package anp

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// cellSpec describes one test cell: inline string, shared-string index or
// raw numeric, with optional formula source (cached value only).
type cellSpec struct {
	str     string
	shared  int
	num     string
	formula string
	hasNum  bool
	hasShr  bool
	isErr   bool
}

func strCell(s string) cellSpec { return cellSpec{str: s} }
func numCell(s string) cellSpec { return cellSpec{num: s, hasNum: true} }
func sharedCell(i int) cellSpec { return cellSpec{shared: i, hasShr: true} }
func errCell(s string) cellSpec { return cellSpec{num: s, hasNum: true, isErr: true} }
func formulaCell(f, cached string) cellSpec {
	return cellSpec{formula: f, num: cached, hasNum: cached != ""}
}

func colName(i int) string {
	name := ""
	for i >= 0 {
		name = string(rune('A'+i%26)) + name
		i = i/26 - 1
	}
	return name
}

// buildXLSX assembles a minimal workbook in memory: one sheet plus an
// optional shared-string table. Rows are 1-based; row 0 entries are skipped
// so headers can sit at any offset (header-shift cases).
func buildXLSX(t *testing.T, sheet string, shared []string, rows map[int][]cellSpec) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	add := func(name, body string) {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	add("xl/workbook.xml", `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="`+sheet+`" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	if shared != nil {
		var sb strings.Builder
		sb.WriteString(`<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
		for _, s := range shared {
			sb.WriteString("<si><t>" + s + "</t></si>")
		}
		sb.WriteString("</sst>")
		add("xl/sharedStrings.xml", sb.String())
	}
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	maxRow := 0
	for r := range rows {
		if r > maxRow {
			maxRow = r
		}
	}
	for r := 1; r <= maxRow; r++ {
		cells, ok := rows[r]
		if !ok {
			continue
		}
		fmt.Fprintf(&sb, `<row r="%d">`, r)
		for i, c := range cells {
			ref := fmt.Sprintf("%s%d", colName(i), r)
			switch {
			case c.isErr:
				fmt.Fprintf(&sb, `<c r="%s" t="e"><v>%s</v></c>`, ref, c.num)
			case c.formula != "":
				fmt.Fprintf(&sb, `<c r="%s"><f>%s</f>`, ref, c.formula)
				if c.hasNum {
					fmt.Fprintf(&sb, `<v>%s</v>`, c.num)
				}
				sb.WriteString(`</c>`)
			case c.hasShr:
				fmt.Fprintf(&sb, `<c r="%s" t="s"><v>%d</v></c>`, ref, c.shared)
			case c.hasNum:
				fmt.Fprintf(&sb, `<c r="%s"><v>%s</v></c>`, ref, c.num)
			default:
				fmt.Fprintf(&sb, `<c r="%s" t="str"><v>%s</v></c>`, ref, c.str)
			}
		}
		sb.WriteString(`</row>`)
	}
	sb.WriteString(`</sheetData></worksheet>`)
	add("xl/worksheets/sheet1.xml", sb.String())
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func stationRows() map[int][]cellSpec {
	return map[int][]cellSpec{
		7: {strCell("MUNICIPIO"), strCell("CNPJ"), strCell("PRODUTO"), strCell("PREÇO"), strCell("DATA")},
		8: {strCell("SAO PAULO"), strCell("04.218.406/0001-04"), strCell("GASOLINA COMUM"), strCell("5,999"), numCell("45658")},
		9: {strCell("SAO PAULO"), strCell("12ABC345/01DE-35"), strCell("GASOLINA PODIUM"), strCell("7,999"), numCell("45658")},
	}
}

func collect(t *testing.T, p Parser, data []byte, hint string) (string, int, []Row) {
	t.Helper()
	var rows []Row
	sheet, hdr, err := p.Parse(data, hint, func(r Row) error {
		rows = append(rows, r)
		return nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return sheet, hdr, rows
}

func testParser() Parser {
	lim := DefaultLimits()
	lim.MaxRows = 1000
	return Parser{Limits: lim}
}

func TestHeaderShiftAndExactDecimal(t *testing.T) {
	// Headers at row 11 (A07 drift), values byte-exact, no float anywhere.
	rows := map[int][]cellSpec{
		11: {strCell("CNPJ"), strCell("PRODUTO"), strCell("PREÇO"), strCell("DATA")},
		12: {strCell("04.218.406/0001-04"), strCell("GASOLINA COMUM"), strCell("5,999"), numCell("45658")},
	}
	data := buildXLSX(t, "DPC", nil, rows)
	sheet, hdr, got := collect(t, testParser(), data, "DPC")
	if sheet != "DPC" || hdr != 11 {
		t.Errorf("sheet=%q hdr=%d, want DPC/11", sheet, hdr)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
	if got[0].Cells["PRECO"] != "5,999" {
		t.Errorf("decimal not preserved: %q", got[0].Cells["PRECO"])
	}
	if got[0].Cells["DATA"] != "45658" {
		t.Errorf("serial not preserved: %q", got[0].Cells["DATA"])
	}
	if got[0].Number != 12 {
		t.Errorf("row number = %d, want 12", got[0].Number)
	}
}

func TestUnknownLabelPassesThrough(t *testing.T) {
	data := buildXLSX(t, "DPC", nil, stationRows())
	_, _, got := collect(t, testParser(), data, "DPC")
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2", len(got))
	}
	if got[1].Cells["PRODUTO"] != "GASOLINA PODIUM" {
		t.Errorf("unknown label coerced: %q", got[1].Cells["PRODUTO"])
	}
}

func TestSharedStringsAndFormulasNeverEvaluate(t *testing.T) {
	shared := []string{"GASOLINA COMUM", "RESUMO"}
	rows := map[int][]cellSpec{
		1: {strCell("CNPJ"), strCell("PRODUTO"), strCell("PREÇO"), strCell("DATA")},
		2: {numCell("123"), sharedCell(0), formulaCell("SUM(A1:A9)", "6"), numCell("45658")},
		3: {numCell("124"), sharedCell(1), formulaCell("SUM(A1:A9)", ""), numCell("45658")},
	}
	data := buildXLSX(t, "DPC", shared, rows)
	_, _, got := collect(t, testParser(), data, "DPC")
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2", len(got))
	}
	if got[0].Cells["PRODUTO"] != "GASOLINA COMUM" {
		t.Errorf("shared string unresolved: %q", got[0].Cells["PRODUTO"])
	}
	if got[0].Cells["PRECO"] != "6" {
		t.Errorf("cached formula value lost: %q", got[0].Cells["PRECO"])
	}
	if got[1].Cells["PRECO"] != "" {
		t.Errorf("uncached formula evaluated or filled: %q", got[1].Cells["PRECO"])
	}
}

func TestMissingSheetAndColumns(t *testing.T) {
	rows := map[int][]cellSpec{
		1: {strCell("CNPJ"), strCell("PRODUTO")},
		2: {strCell("x"), strCell("GASOLINA COMUM")},
	}
	data := buildXLSX(t, "RESUMO", nil, rows)
	p := testParser()
	var out []Row
	_, _, err := p.Parse(data, "DPC", func(r Row) error {
		out = append(out, r)
		return nil
	})
	if err == nil {
		t.Fatal("missing sheet/columns accepted")
	}
	if !errors.Is(err, ErrMissingColumns) && !errors.Is(err, ErrMissingSheet) {
		t.Errorf("wrong error: %v", err)
	}
}

func TestMalformedZIPAndXML(t *testing.T) {
	p := testParser()
	if _, _, err := p.Parse([]byte("not a zip"), "DPC", func(Row) error { return nil }); !errors.Is(err, ErrMalformed) {
		t.Errorf("garbage accepted: %v", err)
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("xl/workbook.xml")
	f.Write([]byte(`<?xml version="1.0"?><workbook><sheets><sheet name="DPC"/></sheets>`))
	w.Close()
	if _, _, err := p.Parse(buf.Bytes(), "DPC", func(Row) error { return nil }); err == nil {
		t.Error("truncated XML accepted")
	}
	var evil bytes.Buffer
	w = zip.NewWriter(&evil)
	f, _ = w.Create("xl/workbook.xml")
	f.Write([]byte(`<?xml version="1.0"?><!DOCTYPE x [<!ENTITY a "b">]><workbook/>`))
	w.Close()
	if _, _, err := p.Parse(evil.Bytes(), "DPC", func(Row) error { return nil }); !errors.Is(err, ErrMalformed) {
		t.Errorf("doctype accepted: %v", err)
	}
	// A doctype inside an otherwise VALID workbook must still refuse: this
	// proves the guard itself, not a coincidental missing-sheet failure.
	valid := buildXLSX(t, "DPC", nil, stationRows())
	withDecl := splicePart(t, valid, "xl/workbook.xml", `<?xml version="1.0"?><!DOCTYPE workbook [<!ENTITY a "b">]>`)
	if _, _, err := p.Parse(withDecl, "DPC", func(Row) error { return nil }); !errors.Is(err, ErrMalformed) {
		t.Errorf("doctype in valid workbook accepted: %v", err)
	}
	var trav bytes.Buffer
	w = zip.NewWriter(&trav)
	f, _ = w.Create("../../evil.xml")
	f.Write([]byte(`x`))
	w.Close()
	if _, _, err := p.Parse(trav.Bytes(), "DPC", func(Row) error { return nil }); !errors.Is(err, ErrUnsafeEntry) {
		t.Errorf("traversal accepted: %v", err)
	}
}

// splicePart rewrites one entry of an existing workbook, preserving the
// rest byte-identical.
func splicePart(t *testing.T, data []byte, part, newPrefix string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == part {
			idx := bytes.Index(raw, []byte("?>"))
			if idx < 0 {
				t.Fatalf("no XML prolog in %s", part)
			}
			raw = append([]byte(newPrefix), raw[idx+2:]...)
		}
		out, err := w.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSizeLimitsEnforced(t *testing.T) {
	p := Parser{Limits: Limits{
		MaxCompressedBytes: 1 << 20, MaxUncompressedBytes: 1 << 20,
		MaxSheets: 2, MaxRows: 3, MaxHeaderScanRows: 25, MaxSharedStrings: 100,
	}}
	rows := map[int][]cellSpec{
		1: {strCell("CNPJ"), strCell("PRODUTO"), strCell("PREÇO"), strCell("DATA")},
	}
	for r := 2; r <= 8; r++ {
		rows[r] = []cellSpec{strCell("x"), strCell("GASOLINA COMUM"), strCell("5,999"), numCell("45658")}
	}
	data := buildXLSX(t, "DPC", nil, rows)
	var n int
	_, _, err := p.Parse(data, "DPC", func(Row) error { n++; return nil })
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversize accepted: %v (rows=%d)", err, n)
	}
	if _, _, err := p.Parse(make([]byte, (1<<20)+1), "DPC", func(Row) error { return nil }); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversize blob accepted: %v", err)
	}
}

func TestExcelSerialDates(t *testing.T) {
	cases := []struct {
		serial int
		iso    string
	}{
		{45658, "2025-01-01"},
		{45351, "2024-02-29"},
		{61, "1900-03-01"},
	}
	for _, c := range cases {
		got, err := ExcelSerialToDate(c.serial)
		if err != nil || got != c.iso {
			t.Errorf("serial %d = %q, %v; want %q", c.serial, got, err, c.iso)
		}
	}
	for _, serial := range []int{0, -5, 60, 2958466} {
		if _, err := ExcelSerialToDate(serial); err == nil {
			t.Errorf("serial %d accepted", serial)
		}
	}
}

func TestErrorCellsFlagged(t *testing.T) {
	rows := map[int][]cellSpec{
		1: {strCell("CNPJ"), strCell("PRODUTO"), strCell("PREÇO"), strCell("DATA")},
		2: {strCell("x"), strCell("GASOLINA COMUM"), errCell("#N/A"), numCell("45658")},
	}
	data := buildXLSX(t, "DPC", nil, rows)
	_, _, got := collect(t, testParser(), data, "DPC")
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
	if !strings.HasPrefix(got[0].Cells["PRECO"], "!<err>:") {
		t.Errorf("error cell not flagged: %q", got[0].Cells["PRECO"])
	}
}
