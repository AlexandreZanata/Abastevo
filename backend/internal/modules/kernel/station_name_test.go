package kernel

import "testing"

func TestPublicStationNameDropsCPFShapedTokens(t *testing.T) {
	for _, tc := range []struct{ name, cnpj, want string }{
		{"JOAO DA SILVA 12345678909", "11222333000181", "JOAO DA SILVA"},
		{"MARIA SOUZA 123.456.789-09 COMBUSTIVEIS", "11222333000181", "MARIA SOUZA COMBUSTIVEIS"},
		{"A 12345678909 12345678909 B", "", "A B"},
		{"12345678909", "11222333000181", "Posto 11222333000181"},
		{"12345678909", "", "Posto"},
		{"  ", "", ""},
		{"POSTO BR 163 KM 12345", "", "POSTO BR 163 KM 12345"},
		{"AUTO POSTO 04218406000104", "", "AUTO POSTO 04218406000104"},
		{"POSTO A1234567890B", "", "POSTO A1234567890B"},
		{"POSTO SÃO JOSÉ", "", "POSTO SÃO JOSÉ"},
	} {
		if got := PublicStationName(tc.name, tc.cnpj); got != tc.want {
			t.Errorf("PublicStationName(%q, %q) = %q, want %q", tc.name, tc.cnpj, got, tc.want)
		}
	}
}

func TestHasCPFShapedTokenMatchesThePortalPrivacyShape(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"Ligue 12345678909 hoje", true},
		{"CPF 123.456.789-09.", true},
		{"12345678909", true},
		{"linha\n12345678909\n", true},
		{"(65) 99999-9999", false},
		{"CNPJ 04218406000104", false},
		{"A1234567890B", false},
		{"R$ 6,199 em 1.234 postos", false},
		{"", false},
	} {
		if got := HasCPFShapedToken(tc.text); got != tc.want {
			t.Errorf("HasCPFShapedToken(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
