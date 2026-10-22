//go:build integration

package prepared

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// syntheticEnforcementWorkbook builds a minimal XLSX with the pinned
// profile: the eleven exact headers, one company row and one personal
// row. The archive is produced by the standard library, independently
// of the Rust reader.
func syntheticEnforcementWorkbook() []byte {
	index := map[string]int{}
	var shared strings.Builder
	shared.WriteString(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	position := func(value string) int {
		if i, ok := index[value]; ok {
			return i
		}
		i := len(index)
		index[value] = i
		shared.WriteString("<si><t>" + value + "</t></si>")
		return i
	}
	cell := func(letter string, number int, value string, numeric bool) string {
		if numeric {
			return fmt.Sprintf(`<c r="%s%d"><v>%s</v></c>`, letter, number, value)
		}
		return fmt.Sprintf(`<c r="%s%d" t="s"><v>%d</v></c>`, letter, number, position(value))
	}
	letters := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K"}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	rows := [][]string{
		{"UF", "Município", "Bairro", "ENDEREÇO", "CNPJ/CPF", "Agente Econômico", "Segmento Fiscalizado", "DATA DO DF", "Número do Documento", "Procedimento de Fiscalização", "Resultado"},
		{"DF", "BRASILIA", "Centro Sintético", "Rua Sintética 1", "36754752000195", "Empresa Sintética Ltda", "Revenda de Combustíveis", "43467", "712508", "Procedimento sintético", "Resultado sintético"},
		{"SP", "CAMPINAS", "Centro Sintético", "Rua Sintética 2", "12345678901", "Pessoa Sintética", "Revenda de Combustíveis", "43468", "712509", "Procedimento sintético", "Resultado curto"},
	}
	for i, row := range rows {
		number := i + 1
		sheet.WriteString(fmt.Sprintf(`<row r="%d">`, number))
		for j, value := range row {
			numeric := i > 0 && j == 7
			sheet.WriteString(cell(letters[j], number, value, numeric))
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	shared.WriteString(`</sst>`)
	workbook := `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><workbookPr defaultThemeVersion="202300"/><sheets><sheet name="v_dados_abertos_dfs" sheetId="1" r:id="rId1"/></sheets></workbook>`
	rels := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, member := range []struct{ name, body string }{
		{"xl/workbook.xml", workbook},
		{"xl/_rels/workbook.xml.rels", rels},
		{"xl/sharedStrings.xml", shared.String()},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	} {
		f, err := w.Create(member.name)
		if err != nil {
			panic(err)
		}
		if _, err := io.WriteString(f, member.body); err != nil {
			panic(err)
		}
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestRustEnforcementPreparerRoundTrip(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH must name the built trusted Rust enforcement CLI")
	}
	raw := syntheticEnforcementWorkbook()
	source := filepath.Join(t.TempDir(), "synthetic.xlsx")
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	dir := filepath.Join(t.TempDir(), "prepared")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, binary, source, dir).Run(); err != nil {
		t.Fatal("trusted enforcement preparer failed", err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadEnforcement(ctx, manifest,
		func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(dir, name)) },
		InputIdentity{Bytes: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])})
	if err != nil {
		t.Fatal("actual Rust contract mismatch", err)
	}
	if b.Counts.Input != 2 || b.Counts.Accepted != 2 || len(b.Records) != 2 {
		t.Fatal("round-trip counts", b.Counts, len(b.Records))
	}
	if b.Records[0].SourceCNPJ != "36754752000195" || b.Records[0].DocumentDate != "2019-01-02" {
		t.Fatal("company record mistyped", b.Records[0])
	}
	if b.Records[1].SourceCNPJ != "" || b.Records[1].SubjectKind != "personal-shaped" {
		t.Fatal("personal subject leaked an identifier", b.Records[1])
	}
	for _, r := range b.Records {
		if strings.Contains(r.Result, "Sintética") && strings.Contains(r.Result, "Rua") {
			t.Fatal("address column leaked into result", r)
		}
	}
}

func TestRustEnforcementFullSourceOracle(t *testing.T) {
	workbook := os.Getenv("ANPFUEL_TEST_ENFORCEMENT_XLSX_PATH")
	binary := os.Getenv("ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH")
	if workbook == "" || binary == "" {
		t.Skip("private enforcement workbook or preparer unavailable")
	}
	raw, err := os.ReadFile(workbook)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	dir := filepath.Join(t.TempDir(), "prepared")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := exec.CommandContext(ctx, binary, workbook, dir).Run(); err != nil {
		t.Fatal("trusted enforcement preparer failed", err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadEnforcement(ctx, manifest,
		func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(dir, name)) },
		InputIdentity{Bytes: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])})
	if err != nil {
		t.Fatal("full-source Rust contract mismatch", err)
	}
	if b.Counts.Input != 244247 || b.Counts.Accepted != 244247 || len(b.Records) != 244247 {
		t.Fatal("full-source counts", b.Counts, len(b.Records))
	}
	company, personal, other := 0, 0, 0
	for _, r := range b.Records {
		switch r.SubjectKind {
		case "company-shaped":
			company++
			if r.SourceCNPJ == "" {
				t.Fatal("admitted company without identifier", r.RecordKey)
			}
		case "personal-shaped":
			personal++
			if r.SourceCNPJ != "" {
				t.Fatal("personal subject carries an identifier", r.RecordKey)
			}
		case "other":
			other++
			if r.SourceCNPJ != "" {
				t.Fatal("unknown subject carries an identifier", r.RecordKey)
			}
		default:
			t.Fatal("unknown subject kind", r.SubjectKind)
		}
		if len(r.DocumentDate) != 10 {
			t.Fatal("inexact date precision", r.RecordKey)
		}
	}
	if company != 242948 || personal != 1294 || other != 5 {
		t.Fatal("subject census", company, personal, other)
	}
}
