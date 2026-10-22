//! Bounded streaming XML pull parser for spreadsheet parts.
//!
//! Only the subset a SpreadsheetML part needs is accepted: elements,
//! attributes and character data. A document type declaration is refused
//! instead of being expanded, and an unknown entity reference is refused
//! instead of being resolved, so a hostile part cannot smuggle content
//! into the reader. Depth, attribute, name and text sizes are bounded.

use std::io::{self, Read};

/// Refusal reasons of the XML reader.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum XmlError {
    Truncated,
    InvalidSyntax,
    DtdRefused,
    EntityRefused,
    DepthExceeded,
    AttributeLimit,
    TextLimit,
    NameLimit,
    Encoding,
}

impl XmlError {
    fn message(self) -> &'static str {
        match self {
            XmlError::Truncated => "truncated xml part",
            XmlError::InvalidSyntax => "invalid xml syntax",
            XmlError::DtdRefused => "xml document type declaration refused",
            XmlError::EntityRefused => "unknown xml entity reference refused",
            XmlError::DepthExceeded => "xml nesting depth exceeded",
            XmlError::AttributeLimit => "xml attribute budget exceeded",
            XmlError::TextLimit => "xml character data budget exceeded",
            XmlError::NameLimit => "xml name budget exceeded",
            XmlError::Encoding => "unsupported xml encoding",
        }
    }
}

impl From<io::Error> for XmlError {
    fn from(_: io::Error) -> Self {
        XmlError::Truncated
    }
}

impl From<XmlError> for io::Error {
    fn from(error: XmlError) -> Self {
        io::Error::new(io::ErrorKind::InvalidData, error.message())
    }
}

/// One parsed XML event.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum XmlEvent {
    Start {
        name: String,
        attributes: Vec<(String, String)>,
    },
    End {
        name: String,
    },
    Text(String),
}

/// Bounds applied to one XML part.
#[derive(Debug, Clone, Copy)]
pub struct XmlLimits {
    pub max_depth: usize,
    pub max_attributes: usize,
    pub max_attribute_bytes: usize,
    pub max_text_bytes: usize,
    pub max_name_bytes: usize,
}

impl Default for XmlLimits {
    fn default() -> Self {
        Self {
            max_depth: 64,
            max_attributes: 64,
            max_attribute_bytes: 64 * 1024,
            max_text_bytes: 4 * 1024 * 1024,
            max_name_bytes: 256,
        }
    }
}

/// Streaming XML reader over any byte source.
pub struct XmlReader<R: Read> {
    input: R,
    buffer: Vec<u8>,
    position: usize,
    input_done: bool,
    limits: XmlLimits,
    depth: usize,
    pending_text: String,
    pending_end: Option<String>,
    failed: bool,
}

impl<R: Read> XmlReader<R> {
    pub fn new(input: R, limits: XmlLimits) -> Self {
        Self {
            input,
            buffer: Vec::new(),
            position: 0,
            input_done: false,
            limits,
            depth: 0,
            pending_text: String::new(),
            pending_end: None,
            failed: false,
        }
    }

    fn fill(&mut self) -> io::Result<bool> {
        if self.input_done {
            return Ok(false);
        }
        if self.position > 0 {
            self.buffer.drain(..self.position);
            self.position = 0;
        }
        let mut chunk = [0u8; 32 * 1024];
        let read = self.input.read(&mut chunk)?;
        if read == 0 {
            self.input_done = true;
            Ok(false)
        } else {
            self.buffer.extend_from_slice(&chunk[..read]);
            Ok(true)
        }
    }

    fn ensure(&mut self, count: usize) -> io::Result<bool> {
        while self.buffer.len() - self.position < count {
            if !self.fill()? {
                return Ok(false);
            }
        }
        Ok(true)
    }

    /// Next buffered byte, refilling the buffer when it is exhausted.
    /// `None` means the input really ended.
    fn peek_filled(&mut self) -> io::Result<Option<u8>> {
        if self.position >= self.buffer.len() && !self.fill()? {
            return Ok(None);
        }
        Ok(self.buffer.get(self.position).copied())
    }

    fn starts_with(&self, prefix: &[u8]) -> bool {
        self.buffer[self.position..].starts_with(prefix)
    }

    /// Make sure at least `prefix.len()` bytes are buffered, so a marker
    /// split across two reads is still recognized.
    fn ensure_prefix(&mut self, prefix: &[u8]) -> bool {
        while self.buffer.len() - self.position < prefix.len() {
            match self.fill() {
                Ok(true) => {}
                Ok(false) => return false,
                Err(_) => return false,
            }
        }
        true
    }

    fn skip_until(&mut self, needle: &[u8]) -> Result<(), XmlError> {
        loop {
            if let Some(found) = find(&self.buffer[self.position..], needle) {
                self.position += found + needle.len();
                return Ok(());
            }
            self.position = self.buffer.len();
            if !self.fill().map_err(|_| XmlError::Truncated)? {
                return Err(XmlError::Truncated);
            }
        }
    }

    fn read_name(&mut self) -> Result<String, XmlError> {
        let mut raw: Vec<u8> = Vec::new();
        while let Some(byte) = self.peek_filled()? {
            if !is_name_byte(byte) {
                break;
            }
            raw.push(byte);
            self.position += 1;
            if raw.len() > self.limits.max_name_bytes {
                return Err(XmlError::NameLimit);
            }
        }
        let name = String::from_utf8_lossy(&raw).into_owned();
        if name.is_empty() {
            return Err(XmlError::NameLimit);
        }
        Ok(name)
    }

    fn skip_whitespace(&mut self) -> Result<(), XmlError> {
        while let Some(byte) = self.peek_filled()? {
            if byte.is_ascii_whitespace() {
                self.position += 1;
            } else {
                break;
            }
        }
        Ok(())
    }

    fn read_attribute_value(&mut self) -> Result<String, XmlError> {
        let quote = self.peek_filled()?.ok_or(XmlError::Truncated)?;
        if quote != b'"' && quote != b'\'' {
            return Err(XmlError::InvalidSyntax);
        }
        self.position += 1;
        let mut raw: Vec<u8> = Vec::new();
        loop {
            let byte = self.peek_filled()?.ok_or(XmlError::Truncated)?;
            if byte == quote {
                break;
            }
            raw.push(byte);
            self.position += 1;
            if raw.len() > self.limits.max_attribute_bytes {
                return Err(XmlError::AttributeLimit);
            }
            if !self.ensure(1)? {
                return Err(XmlError::Truncated);
            }
        }
        self.position += 1;
        let text = String::from_utf8(raw).map_err(|_| XmlError::Encoding)?;
        decode_entities(&text)
    }

    fn read_start_tag(&mut self) -> Result<XmlEvent, XmlError> {
        self.depth += 1;
        if self.depth > self.limits.max_depth {
            return Err(XmlError::DepthExceeded);
        }
        let name = self.read_name()?;
        let mut attributes: Vec<(String, String)> = Vec::new();
        loop {
            self.skip_whitespace()?;
            match self.peek_filled()? {
                None => return Err(XmlError::Truncated),
                Some(b'>') => {
                    self.position += 1;
                    return Ok(XmlEvent::Start { name, attributes });
                }
                Some(b'/') => {
                    self.position += 1;
                    if self.peek_filled()? != Some(b'>') {
                        return Err(XmlError::InvalidSyntax);
                    }
                    self.position += 1;
                    self.depth -= 1;
                    self.pending_end = Some(name.clone());
                    return Ok(XmlEvent::Start { name, attributes });
                }
                Some(_) => {
                    if attributes.len() >= self.limits.max_attributes {
                        return Err(XmlError::AttributeLimit);
                    }
                    let attribute = self.read_name()?;
                    self.skip_whitespace()?;
                    if self.peek_filled()? != Some(b'=') {
                        return Err(XmlError::InvalidSyntax);
                    }
                    self.position += 1;
                    self.skip_whitespace()?;
                    let value = self.read_attribute_value()?;
                    if attribute.len() + value.len() > self.limits.max_attribute_bytes {
                        return Err(XmlError::AttributeLimit);
                    }
                    attributes.push((attribute, value));
                }
            }
        }
    }

    fn read_end_tag(&mut self) -> Result<XmlEvent, XmlError> {
        self.position += 2;
        let name = self.read_name()?;
        self.skip_whitespace()?;
        if self.peek_filled()? != Some(b'>') {
            return Err(XmlError::InvalidSyntax);
        }
        self.position += 1;
        if self.depth == 0 {
            return Err(XmlError::InvalidSyntax);
        }
        self.depth -= 1;
        Ok(XmlEvent::End { name })
    }

    fn read_text(&mut self) -> Result<String, XmlError> {
        let mut raw: Vec<u8> = Vec::new();
        loop {
            match self.peek_filled()? {
                None => return Err(XmlError::Truncated),
                Some(b'<') => break,
                Some(byte) => {
                    raw.push(byte);
                    self.position += 1;
                    if raw.len() > self.limits.max_text_bytes {
                        return Err(XmlError::TextLimit);
                    }
                    if !self.ensure(1)? {
                        return Err(XmlError::Truncated);
                    }
                }
            }
        }
        let text = String::from_utf8(raw).map_err(|_| XmlError::Encoding)?;
        decode_entities(&text)
    }

    fn read_cdata(&mut self) -> Result<String, XmlError> {
        self.position += 9;
        let mut raw: Vec<u8> = Vec::new();
        loop {
            let window = &self.buffer[self.position..];
            if let Some(found) = find(window, b"]]>") {
                raw.extend_from_slice(&window[..found]);
                self.position += found + 3;
                if raw.len() > self.limits.max_text_bytes {
                    return Err(XmlError::TextLimit);
                }
                return String::from_utf8(raw).map_err(|_| XmlError::Encoding);
            }
            raw.extend_from_slice(window);
            self.position = self.buffer.len();
            if raw.len() > self.limits.max_text_bytes {
                return Err(XmlError::TextLimit);
            }
            if !self.fill().map_err(|_| XmlError::Truncated)? {
                return Err(XmlError::Truncated);
            }
        }
    }

    fn read_declaration(&mut self) -> Result<(), XmlError> {
        let start = self.position;
        self.skip_until(b"?>")?;
        let raw = String::from_utf8_lossy(&self.buffer[start..self.position]).into_owned();
        if let Some(index) = raw.find("encoding") {
            let rest = &raw[index + "encoding".len()..];
            if let Some(equal) = rest.find('=') {
                let value = rest[equal + 1..].trim_start();
                let quote = value.chars().next().unwrap_or('"');
                if quote == '"' || quote == '\'' {
                    let quoted = &value[1..];
                    if let Some(end) = quoted.find(quote) {
                        let encoding = quoted[..end].to_ascii_lowercase();
                        if !encoding.is_empty() && encoding != "utf-8" && encoding != "us-ascii" {
                            return Err(XmlError::Encoding);
                        }
                    }
                }
            }
        }
        Ok(())
    }
}

impl<R: Read> Iterator for XmlReader<R> {
    type Item = Result<XmlEvent, XmlError>;

    fn next(&mut self) -> Option<Self::Item> {
        if self.failed {
            return None;
        }
        loop {
            if let Some(name) = self.pending_end.take() {
                return Some(Ok(XmlEvent::End { name }));
            }
            if !self.pending_text.is_empty() {
                let text = std::mem::take(&mut self.pending_text);
                return Some(Ok(XmlEvent::Text(text)));
            }
            let byte = match self.peek_filled() {
                Ok(Some(byte)) => byte,
                Ok(None) => return None,
                Err(_) => {
                    self.failed = true;
                    return Some(Err(XmlError::Truncated));
                }
            };
            if byte != b'<' {
                match self.read_text() {
                    Ok(text) => {
                        self.pending_text = text;
                        continue;
                    }
                    Err(error) => {
                        self.failed = true;
                        return Some(Err(error));
                    }
                }
            }
            if self.ensure_prefix(b"<?") && self.starts_with(b"<?") {
                let outcome = if self.starts_with(b"<?xml") {
                    self.read_declaration()
                } else {
                    self.skip_until(b"?>")
                };
                if let Err(error) = outcome {
                    self.failed = true;
                    return Some(Err(error));
                }
                continue;
            }
            if self.ensure_prefix(b"<!--") && self.starts_with(b"<!--") {
                self.position += 4;
                if let Err(error) = self.skip_until(b"-->") {
                    self.failed = true;
                    return Some(Err(error));
                }
                continue;
            }
            if self.ensure_prefix(b"<![CDATA[") && self.starts_with(b"<![CDATA[") {
                match self.read_cdata() {
                    Ok(text) => {
                        self.pending_text = text;
                        continue;
                    }
                    Err(error) => {
                        self.failed = true;
                        return Some(Err(error));
                    }
                }
            }
            if self.ensure_prefix(b"<!") && self.starts_with(b"<!") {
                self.failed = true;
                return Some(Err(XmlError::DtdRefused));
            }
            if self.ensure_prefix(b"</") && self.starts_with(b"</") {
                let event = self.read_end_tag();
                if event.is_err() {
                    self.failed = true;
                }
                return Some(event);
            }
            self.position += 1;
            let event = self.read_start_tag();
            if event.is_err() {
                self.failed = true;
            }
            return Some(event);
        }
    }
}

fn find(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    if needle.is_empty() || haystack.len() < needle.len() {
        return None;
    }
    haystack
        .windows(needle.len())
        .position(|window| window == needle)
}

fn is_name_byte(byte: u8) -> bool {
    byte.is_ascii_alphanumeric()
        || byte == b'_'
        || byte == b'-'
        || byte == b'.'
        || byte == b':'
        || byte >= 0x80
}

/// Decode the five predefined entities and numeric character references.
/// Every other reference is refused: a SpreadsheetML part never needs one.
fn decode_entities(raw: &str) -> Result<String, XmlError> {
    if !raw.contains('&') {
        return Ok(raw.to_string());
    }
    let mut out = String::with_capacity(raw.len());
    let mut rest = raw;
    while let Some(index) = rest.find('&') {
        out.push_str(&rest[..index]);
        let tail = &rest[index..];
        let end = tail.find(';').ok_or(XmlError::EntityRefused)?;
        let reference = &tail[1..end];
        match reference {
            "amp" => out.push('&'),
            "lt" => out.push('<'),
            "gt" => out.push('>'),
            "quot" => out.push('"'),
            "apos" => out.push('\''),
            _ => {
                let code = if let Some(hex) = reference.strip_prefix("#x") {
                    u32::from_str_radix(hex, 16).ok().or(Some(0)).unwrap_or(0)
                } else if let Some(decimal) = reference.strip_prefix('#') {
                    decimal.parse::<u32>().ok().or(Some(0)).unwrap_or(0)
                } else {
                    0
                };
                let character = char::from_u32(code).ok_or(XmlError::EntityRefused)?;
                if code == 0 {
                    return Err(XmlError::EntityRefused);
                }
                out.push(character);
            }
        }
        rest = &tail[end + 1..];
    }
    out.push_str(rest);
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::{decode_entities, XmlError, XmlEvent, XmlLimits, XmlReader};

    fn events(input: &str) -> Result<Vec<XmlEvent>, XmlError> {
        XmlReader::new(input.as_bytes(), XmlLimits::default()).collect()
    }

    #[test]
    fn reads_elements_attributes_and_text() {
        let parsed = events("<a x=\"1\"><b>value</b><c/></a>").unwrap();
        assert_eq!(
            parsed,
            vec![
                XmlEvent::Start {
                    name: "a".to_string(),
                    attributes: vec![("x".to_string(), "1".to_string())]
                },
                XmlEvent::Start {
                    name: "b".to_string(),
                    attributes: Vec::new()
                },
                XmlEvent::Text("value".to_string()),
                XmlEvent::End {
                    name: "b".to_string()
                },
                XmlEvent::Start {
                    name: "c".to_string(),
                    attributes: Vec::new()
                },
                XmlEvent::End {
                    name: "c".to_string()
                },
                XmlEvent::End {
                    name: "a".to_string()
                },
            ]
        );
    }

    #[test]
    fn decodes_predefined_entities_and_numeric_references() {
        assert_eq!(
            decode_entities("a&amp;b&lt;c&#65;&#x42;").unwrap(),
            "a&b<cAB"
        );
        assert_eq!(
            decode_entities("no entities"),
            Ok("no entities".to_string())
        );
        assert_eq!(decode_entities("&unknown;"), Err(XmlError::EntityRefused));
    }

    #[test]
    fn refuses_document_type_declarations() {
        assert_eq!(
            events("<!DOCTYPE sheet [<!ENTITY x SYSTEM \"file:///etc/passwd\">]><sheet/>"),
            Err(XmlError::DtdRefused)
        );
    }

    #[test]
    fn refuses_nesting_beyond_the_bound() {
        let limits = XmlLimits {
            max_depth: 2,
            ..XmlLimits::default()
        };
        let parsed =
            XmlReader::new("<a><b><c/></b></a>".as_bytes(), limits).collect::<Result<Vec<_>, _>>();
        assert_eq!(parsed, Err(XmlError::DepthExceeded));
    }

    #[test]
    fn refuses_unsupported_encodings_and_truncated_parts() {
        assert_eq!(
            events("<?xml version=\"1.0\" encoding=\"UTF-16\"?><a/>"),
            Err(XmlError::Encoding)
        );
        assert_eq!(events("<a>text"), Err(XmlError::Truncated));
    }

    #[test]
    fn reads_character_data_and_cdata() {
        let parsed = events("<a><![CDATA[raw & text]]></a>").unwrap();
        assert!(parsed.contains(&XmlEvent::Text("raw & text".to_string())));
    }
}
