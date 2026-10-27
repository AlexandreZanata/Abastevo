package jobs

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/anp"
	odomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
)

func TestPadSourceCNPJRestoresLostLeadingZeros(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"198451000185", "00198451000185"},
		{"61602199002409", "61602199002409"},
		{"04.218.406/0001-04", "04.218.406/0001-04"},
		{"12ABC34501DE35", "12ABC34501DE35"},
		{"", ""},
	} {
		if got := padSourceCNPJ(tc.raw); got != tc.want {
			t.Errorf("padSourceCNPJ(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
	if _, err := kernel.ParseCNPJ(padSourceCNPJ("198451000185")); err != nil {
		t.Errorf("padded real CNPJ rejected: %v", err)
	}
	if _, err := kernel.ParseCNPJ(padSourceCNPJ("04.218.406/0001-04")); err != nil {
		t.Errorf("formatted CNPJ rejected: %v", err)
	}
}

func TestSourcePriceTextAcceptsANPDecimal(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"5.59", "5,59"},
		{"5,59", "5,59"},
		{"100", "100"},
		{"1.234,56", "1.234,56"},
		{"1.2.3", "1.2.3"},
	} {
		if got := sourcePriceText(tc.raw); got != tc.want {
			t.Errorf("sourcePriceText(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestAdmitFloatNoiseGoldenVectors(t *testing.T) {
	for _, tc := range []struct {
		text, want string
		ok         bool
	}{
		{"4,0199999999999996", "4,020", true},
		{"5,1100000000000003", "5,110", true},
		{"8,0399999999999991", "8,040", true},
		{"5,5999", "", false},
		{"5,9999", "", false},
		{"5,999", "", false},
		{"5,59", "", false},
		{"-4,0199999999999996", "", false},
		{"4,01999999999999960001", "", false},
		{"1.234,0199999999999996", "", false},
		{"4,0199999999999996e3", "", false},
		{"NaN", "", false},
		{"", "", false},
	} {
		got, ok := admitFloatNoise(tc.text)
		if got != tc.want || ok != tc.ok {
			t.Errorf("admitFloatNoise(%q) = %q, %v; want %q, %v", tc.text, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormalizeRowAdmitsWorkbookFloatNoiseWithRawText(t *testing.T) {
	var tally odomain.QuarantineTally
	row := anp.Row{Number: 7, Cells: map[string]string{
		"PRODUTO": "GASOLINA COMUM", "PRECO": "4.0199999999999996",
		"CNPJ": "04.218.406/0001-04", "DATA": "2026-10-20",
	}}
	out, ok := normalizeRow(row, &tally)
	if !ok || out.Row.AmountMilli != 4020 || out.Row.RawText != "4.0199999999999996" {
		t.Fatalf("float noise not admitted exactly: ok=%v row=%+v", ok, out.Row)
	}
	if tally.Total() != 0 || tally.Admitted["float-noise"] != 1 {
		t.Fatalf("float-noise admission not recorded apart from quarantine: %+v", tally)
	}
}

func TestNormalizeRowStillQuarantinesRealOverPrecision(t *testing.T) {
	var tally odomain.QuarantineTally
	row := anp.Row{Number: 8, Cells: map[string]string{
		"PRODUTO": "GASOLINA COMUM", "PRECO": "5.5999",
		"CNPJ": "04.218.406/0001-04", "DATA": "2026-10-20",
	}}
	if _, ok := normalizeRow(row, &tally); ok {
		t.Fatal("real over-precision price admitted")
	}
	if tally.Counts["over-precision"] != 1 || len(tally.Admitted) != 0 {
		t.Fatalf("over-precision not quarantined: %+v", tally)
	}
}
