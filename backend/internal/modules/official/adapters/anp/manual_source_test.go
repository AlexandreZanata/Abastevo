//go:build manual

package anp

import (
	"os"
	"testing"
)

// TestManualRealWeekly is an operator-run bounded smoke over one real ANP
// workbook. It is excluded from CI and never downloads the source itself.
func TestManualRealWeekly(t *testing.T) {
	path := os.Getenv("ANPFUEL_MANUAL_XLSX")
	if path == "" {
		t.Skip("ANPFUEL_MANUAL_XLSX is not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parser := Parser{Limits: DefaultLimits()}
	rows := 0
	products := map[string]int{}
	dates := map[string]int{}
	cnpjs := map[string]bool{}
	var parseErr error
	sheet, headerRow, err := parser.Parse(raw, "", func(row Row) error {
		if rows == 0 {
			for key := range row.Cells {
				t.Logf("column %q", key)
			}
		}
		if rows < 5 {
			t.Logf("sample cnpj=%q len=%d price=%q data=%q product=%q",
				row.Cells["CNPJ"], len(row.Cells["CNPJ"]), row.Cells["PRECO"], row.Cells["DATA"], row.Cells["PRODUTO"])
		}
		rows++
		products[row.Cells["PRODUTO"]]++
		dates[row.Cells["DATA"]]++
		cnpjs[row.Cells["CNPJ"]] = true
		return nil
	})
	if err != nil {
		parseErr = err
	}
	t.Logf("bytes=%d sheet=%q header_row=%d rows=%d products=%d dates=%d cnpjs=%d parse_err=%v",
		len(raw), sheet, headerRow, rows, len(products), len(dates), len(cnpjs), parseErr)
	for product, count := range products {
		t.Logf("product %q rows=%d", product, count)
	}
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if rows < 1000 {
		t.Fatalf("rows = %d, want a real weekly workbook", rows)
	}
	if len(cnpjs) < 100 {
		t.Fatalf("distinct CNPJs = %d", len(cnpjs))
	}
}
