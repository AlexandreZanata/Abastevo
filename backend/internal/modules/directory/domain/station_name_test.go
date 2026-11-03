package domain

import "testing"

func TestStationDisplayNamePrecedenceAndMissingTradeNames(t *testing.T) {
	const legal = " [P25-TEST] LEGAL Name LTDA "
	const cnpj = "91000000000066"
	for _, tc := range []struct{ name, trade, correction, want string }{
		{"trade", " [P25-TEST] Minha   Marca ", "", "[P25-TEST] Minha Marca"},
		{"correction", "[P25-TEST] Minha Marca", "[P25-TEST] Approved", "[P25-TEST] Approved"},
		{"blank", " \t", "", "[P25-TEST] LEGAL Name LTDA"},
		{"punctuation", "—...*!", "", "[P25-TEST] LEGAL Name LTDA"},
		{"missing", "NAO POSSUI", "", "[P25-TEST] LEGAL Name LTDA"},
		{"accented", "Não possui", "", "[P25-TEST] LEGAL Name LTDA"},
		{"no-name", "SEM NOME", "", "[P25-TEST] LEGAL Name LTDA"},
		{"not-informed", "NAO INFORMADO", "", "[P25-TEST] LEGAL Name LTDA"},
		{"dash", "-", "", "[P25-TEST] LEGAL Name LTDA"},
		{"dot", ".", "", "[P25-TEST] LEGAL Name LTDA"},
		{"star", "*", "", "[P25-TEST] LEGAL Name LTDA"},
		{"cnpj", cnpj, "", "[P25-TEST] LEGAL Name LTDA"},
		{"formatted-cnpj", "91.000.000/0000-66", "", "[P25-TEST] LEGAL Name LTDA"},
		{"same-legal", "[p25-test] legal\tname ltda", "", "[P25-TEST] LEGAL Name LTDA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := StationDisplayName(legal, tc.trade, cnpj, tc.correction); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
