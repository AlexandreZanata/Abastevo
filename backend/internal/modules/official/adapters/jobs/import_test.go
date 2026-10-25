package jobs

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
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
