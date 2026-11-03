package registry

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSNA01CSVPreservesBothNamesAndAddressComponents(t *testing.T) {
	store := newFakeStore()
	csv := "CNPJ;RAZAO_SOCIAL;NOME_FANTASIA;COD_IBGE;UF;SITUACAO;LOGRADOURO;NUMERO;COMPLEMENTO;BAIRRO;CEP;MUNICIPIO\n91000000000066; [P25-TEST] Legal LTDA ;[P25-TEST] Trade;3550308;SP;ATIVA;Av. Sintetica;123;Loja A;Centro;00000-000;Cidade Sintetica\n"
	report, err := StageCSV(context.Background(), store, "names", strings.NewReader(csv), DefaultLimits())
	if err != nil || report.Accepted != 1 {
		t.Fatal(report, err)
	}
	rows, _ := store.ListAssertions(context.Background(), report.RunID)
	a := rows[0]
	if a.DisplayName != "[P25-TEST] Trade" || a.LegalName != " [P25-TEST] Legal LTDA " || a.TradeName != "[P25-TEST] Trade" {
		t.Fatalf("source names lost: %+v", a)
	}
	for key, want := range map[string]string{"street": "Av. Sintetica", "number": "123", "complement": "Loja A", "district": "Centro", "postal_code": "00000-000", "municipio": "Cidade Sintetica", "municipio_ibge": "3550308", "uf": "SP"} {
		if a.Address[key] != want {
			t.Fatalf("%s=%q want %q", key, a.Address[key], want)
		}
	}
}

func TestSNA01CSVDoesNotReplayOldParserCompletion(t *testing.T) {
	store := newFakeStore()
	ctx := context.Background()
	_, _, _ = store.CreateRun(ctx, "old", SourceCSV, "names", "")
	_ = store.FinishRun(ctx, "old", "complete", 0, 0, 0, "")
	report, err := StageCSV(ctx, store, "names", strings.NewReader("CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO\n91000000000066;[P25-TEST] Legal;3550308;SP;ATIVA\n"), DefaultLimits())
	if err != nil || report.RunID == "old" || report.Accepted != 1 {
		t.Fatal("old parser masked new source", report, err)
	}
}

func TestSNA01OfficialAPITradeNameAndLegalFallback(t *testing.T) {
	for _, trade := range []string{"[P25-TEST] Trade", "NAO POSSUI", ""} {
		var raw map[string]any
		_ = json.Unmarshal([]byte(officialSyntheticRecord), &raw)
		raw["cnpj"] = "91000000000066"
		raw["razaoSocial"] = " [P25-TEST] Legal LTDA "
		raw["nomeFantasia"] = trade
		body, _ := json.Marshal(raw)
		a, ok := parseOfficialAPIRecord(body)
		want := "[P25-TEST] Legal LTDA"
		if trade == "[P25-TEST] Trade" {
			want = trade
		}
		if !ok || a.DisplayName != want || a.LegalName != " [P25-TEST] Legal LTDA " || a.TradeName != trade {
			t.Fatal("source names lost", a, ok)
		}
	}
}
