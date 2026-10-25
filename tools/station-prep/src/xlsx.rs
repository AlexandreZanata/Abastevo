//! Bounded SpreadsheetML reader: workbook profile, shared strings and
//! sheet rows.
//!
//! Only what a pinned source workbook needs is read: the workbook date
//! system, the first worksheet and its shared strings. Cells are
//! resolved to their text exactly as the source states it, a cell
//! reference decides the column and a worksheet wider than the expected
//! profile is refused instead of being truncated.

use crate::xml::{XmlError, XmlEvent, XmlLimits, XmlReader};
use crate::zip::{EntryReader, ZipArchive, ZipError};
use std::io::Read;

/// Refusal reasons of the spreadsheet reader.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum XlsxError {
    Archive(ZipError),
    Xml(XmlError),
    MissingPart,
    ProfileRefused,
    CellRefused,
    SharedStringRefused,
}

impl From<ZipError> for XlsxError {
    fn from(error: ZipError) -> Self {
        XlsxError::Archive(error)
    }
}

impl From<XmlError> for XlsxError {
    fn from(error: XmlError) -> Self {
        XlsxError::Xml(error)
    }
}

/// Workbook facts a pinned source profile depends on.
#[derive(Debug, Clone)]
pub struct Workbook {
    /// `true` when the workbook counts days from 1904-01-01.
    pub date1904: bool,
    /// Name of the first worksheet as the source states it.
    pub sheet_name: String,
    /// Archive path of the first worksheet.
    pub sheet_path: String,
}

/// One worksheet row with the cell texts placed by their reference.
#[derive(Debug, Clone)]
pub struct SheetRow {
    pub number: u64,
    pub cells: Vec<String>,
}

fn local_name(name: &str) -> &str {
    match name.rsplit_once(':') {
        Some((_, local)) => local,
        None => name,
    }
}

fn attribute<'a>(attributes: &'a [(String, String)], wanted: &str) -> Option<&'a str> {
    attributes
        .iter()
        .find(|(name, _)| local_name(name) == wanted)
        .map(|(_, value)| value.as_str())
}

/// Column index of a cell reference such as `AB12`.
pub fn column_index(reference: &str) -> Option<usize> {
    let letters: String = reference
        .chars()
        .take_while(|character| character.is_ascii_alphabetic())
        .collect();
    if letters.is_empty() {
        return None;
    }
    let mut index = 0usize;
    for character in letters.chars() {
        if !character.is_ascii_uppercase() {
            return None;
        }
        index = index * 26 + usize::from(character as u8 - b'A' + 1);
    }
    Some(index - 1)
}

/// Read the workbook profile: date system and first worksheet.
pub fn read_workbook(archive: &ZipArchive) -> Result<Workbook, XlsxError> {
    let mut date1904 = false;
    let mut sheet_name = String::new();
    let mut relationship = String::new();
    let reader = XmlReader::new(archive.open_entry("xl/workbook.xml")?, XmlLimits::default());
    for event in reader {
        match event? {
            XmlEvent::Start { name, attributes } => match local_name(&name) {
                "workbookPr" => {
                    date1904 = attribute(&attributes, "date1904")
                        .map(|value| value == "1" || value.eq_ignore_ascii_case("true"))
                        .unwrap_or(false);
                }
                "sheet" if sheet_name.is_empty() => {
                    sheet_name = attribute(&attributes, "name")
                        .unwrap_or_default()
                        .to_string();
                    relationship = attribute(&attributes, "id").unwrap_or_default().to_string();
                }
                _ => {}
            },
            XmlEvent::End { .. } | XmlEvent::Text(_) => {}
        }
    }
    if sheet_name.is_empty() || relationship.is_empty() {
        return Err(XlsxError::ProfileRefused);
    }
    let target = read_relationship(archive, &relationship)?;
    let target = target.strip_prefix('/').unwrap_or(&target);
    let target = target.strip_prefix("xl/").unwrap_or(target);
    if !target.starts_with("worksheets/") {
        return Err(XlsxError::ProfileRefused);
    }
    Ok(Workbook {
        date1904,
        sheet_name,
        sheet_path: format!("xl/{target}"),
    })
}

fn read_relationship(archive: &ZipArchive, identifier: &str) -> Result<String, XlsxError> {
    let reader = XmlReader::new(
        archive.open_entry("xl/_rels/workbook.xml.rels")?,
        XmlLimits::default(),
    );
    for event in reader {
        if let XmlEvent::Start { name, attributes } = event? {
            if local_name(&name) == "Relationship"
                && attribute(&attributes, "Id") == Some(identifier)
            {
                return Ok(attribute(&attributes, "Target")
                    .unwrap_or_default()
                    .to_string());
            }
        }
    }
    Err(XlsxError::MissingPart)
}

/// Read the shared string table of a workbook.
pub fn read_shared_strings(archive: &ZipArchive) -> Result<Vec<String>, XlsxError> {
    if !archive
        .entries()
        .iter()
        .any(|entry| entry.name == "xl/sharedStrings.xml")
    {
        return Ok(Vec::new());
    }
    let mut strings: Vec<String> = Vec::new();
    let mut current = String::new();
    let mut inside = false;
    let reader = XmlReader::new(
        archive.open_entry("xl/sharedStrings.xml")?,
        XmlLimits::default(),
    );
    for event in reader {
        match event? {
            XmlEvent::Start { name, .. } => {
                if local_name(&name) == "si" {
                    inside = true;
                    current.clear();
                }
            }
            XmlEvent::Text(text) => {
                if inside {
                    current.push_str(&text);
                }
            }
            XmlEvent::End { name } => {
                if local_name(&name) == "si" && inside {
                    inside = false;
                    strings.push(std::mem::take(&mut current));
                }
            }
        }
    }
    Ok(strings)
}

/// Streaming reader over the rows of one worksheet.
pub struct SheetRows<'a> {
    reader: XmlReader<EntryReader<'a>>,
    strings: &'a [String],
    max_columns: usize,
    inside_row: bool,
    cells: Vec<String>,
    row_number: u64,
    cell: Option<CellState>,
    capture: Option<Capture>,
    text: String,
    row_index: u64,
}

#[derive(Debug)]
struct CellState {
    column: usize,
    kind: String,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Capture {
    Value,
    Inline,
}

impl<'a> SheetRows<'a> {
    /// Read the rows of `path`, resolving shared strings and refusing a
    /// worksheet wider than `max_columns`.
    pub fn new(
        archive: &'a ZipArchive,
        path: &str,
        strings: &'a [String],
        max_columns: usize,
    ) -> Result<Self, XlsxError> {
        Ok(Self {
            reader: XmlReader::new(archive.open_entry(path)?, XmlLimits::default()),
            strings,
            max_columns,
            inside_row: false,
            cells: Vec::new(),
            row_number: 0,
            cell: None,
            capture: None,
            text: String::new(),
            row_index: 0,
        })
    }
}

impl Iterator for SheetRows<'_> {
    type Item = Result<SheetRow, XlsxError>;

    fn next(&mut self) -> Option<Self::Item> {
        loop {
            let event = self.reader.next()?;
            match event {
                Err(error) => return Some(Err(error.into())),
                Ok(XmlEvent::Start { name, attributes }) => match local_name(&name) {
                    "row" => {
                        self.inside_row = true;
                        self.cells.clear();
                        self.row_index += 1;
                        self.row_number = attribute(&attributes, "r")
                            .and_then(|value| value.parse::<u64>().ok())
                            .unwrap_or(self.row_index);
                    }
                    "c" => {
                        if !self.inside_row {
                            continue;
                        }
                        let reference = attribute(&attributes, "r").unwrap_or_default();
                        let column = match column_index(reference) {
                            Some(column) => column,
                            None => return Some(Err(XlsxError::CellRefused)),
                        };
                        if column >= self.max_columns {
                            return Some(Err(XlsxError::CellRefused));
                        }
                        self.cell = Some(CellState {
                            column,
                            kind: attribute(&attributes, "t").unwrap_or("n").to_string(),
                        });
                        self.text.clear();
                    }
                    "v" => {
                        if self.cell.is_some() {
                            self.capture = Some(Capture::Value);
                            self.text.clear();
                        }
                    }
                    "is" => {
                        if self.cell.is_some() {
                            self.capture = Some(Capture::Inline);
                            self.text.clear();
                        }
                    }
                    "f" if self.cell.is_some() => {
                        self.capture = None;
                    }
                    _ => {}
                },
                Ok(XmlEvent::Text(text)) => {
                    if self.capture.is_some() {
                        self.text.push_str(&text);
                    }
                }
                Ok(XmlEvent::End { name }) => match local_name(&name) {
                    "v" | "is" => {
                        self.capture = None;
                    }
                    "c" => {
                        let cell = match self.cell.take() {
                            Some(cell) => cell,
                            None => continue,
                        };
                        let value = match cell.kind.as_str() {
                            "s" => {
                                let index = match self.text.trim().parse::<usize>() {
                                    Ok(index) => index,
                                    Err(_) => return Some(Err(XlsxError::SharedStringRefused)),
                                };
                                match self.strings.get(index) {
                                    Some(value) => value.clone(),
                                    None => return Some(Err(XlsxError::SharedStringRefused)),
                                }
                            }
                            "inlineStr" => self.text.clone(),
                            _ => self.text.clone(),
                        };
                        if self.cells.len() <= cell.column {
                            self.cells.resize(cell.column + 1, String::new());
                        }
                        self.cells[cell.column] = value;
                    }
                    "row" => {
                        if !self.inside_row {
                            continue;
                        }
                        self.inside_row = false;
                        return Some(Ok(SheetRow {
                            number: self.row_number,
                            cells: std::mem::take(&mut self.cells),
                        }));
                    }
                    _ => {}
                },
            }
        }
    }
}

/// Read one whole part into memory, bounded by the caller.
pub fn read_part(archive: &ZipArchive, path: &str) -> Result<Vec<u8>, XlsxError> {
    let mut reader = archive.open_entry(path)?;
    let mut payload = Vec::new();
    reader
        .read_to_end(&mut payload)
        .map_err(|_| XlsxError::Archive(ZipError::Truncated))?;
    Ok(payload)
}

#[cfg(test)]
mod tests {
    use super::{column_index, read_shared_strings, read_workbook, SheetRows};
    use crate::zip::ZipArchive;

    const WORKBOOK: &str = r#"<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><workbookPr defaultThemeVersion="202300"/><sheets><sheet name="v_dados_abertos_dfs" sheetId="1" r:id="rId1"/></sheets></workbook>"#;
    const RELS: &str = r#"<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>"#;
    const SHARED: &str = r#"<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="2" uniqueCount="2"><si><t>UF</t></si><si><t>São Paulo</t></si></sst>"#;
    const SHEET: &str = r#"<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row><row r="2"><c r="A2" s="2"><v>43467</v></c><c r="B2" s="2"/></row></sheetData></worksheet>"#;

    fn archive() -> ZipArchive {
        let mut members: Vec<(&str, &[u8])> = vec![
            ("xl/workbook.xml", WORKBOOK.as_bytes()),
            ("xl/_rels/workbook.xml.rels", RELS.as_bytes()),
            ("xl/sharedStrings.xml", SHARED.as_bytes()),
            ("xl/worksheets/sheet1.xml", SHEET.as_bytes()),
        ];
        members.sort_by(|left, right| left.0.cmp(right.0));
        ZipArchive::open(
            crate::zip::tests::stored_archive(&members),
            Default::default(),
        )
        .unwrap()
    }

    #[test]
    fn reads_the_workbook_profile() {
        let workbook = read_workbook(&archive()).unwrap();
        assert!(!workbook.date1904);
        assert_eq!(workbook.sheet_name, "v_dados_abertos_dfs");
        assert_eq!(workbook.sheet_path, "xl/worksheets/sheet1.xml");
    }

    #[test]
    fn reads_shared_strings_and_rows() {
        let opened = archive();
        let strings = read_shared_strings(&opened).unwrap();
        assert_eq!(strings, vec!["UF".to_string(), "São Paulo".to_string()]);
        let rows: Vec<_> = SheetRows::new(&opened, "xl/worksheets/sheet1.xml", &strings, 11)
            .unwrap()
            .collect::<Result<Vec<_>, _>>()
            .unwrap();
        assert_eq!(rows.len(), 2);
        assert_eq!(
            rows[0].cells,
            vec!["UF".to_string(), "São Paulo".to_string()]
        );
        assert_eq!(rows[1].cells, vec!["43467".to_string(), String::new()]);
    }

    #[test]
    fn refuses_a_worksheet_wider_than_the_profile() {
        let opened = archive();
        let strings = read_shared_strings(&opened).unwrap();
        let outcome = SheetRows::new(&opened, "xl/worksheets/sheet1.xml", &strings, 1)
            .unwrap()
            .collect::<Result<Vec<_>, _>>();
        assert!(outcome.is_err());
    }

    #[test]
    fn maps_cell_references_to_columns() {
        assert_eq!(column_index("A1"), Some(0));
        assert_eq!(column_index("K2"), Some(10));
        assert_eq!(column_index("AB12"), Some(27));
        assert_eq!(column_index("12"), None);
    }
}
