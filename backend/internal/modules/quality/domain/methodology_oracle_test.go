package domain

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMethodologyOracleKeepsFullSourceSampleScope(t *testing.T) {
	path := t.TempDir() + "/synthetic.csv"
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(f)
	w.Comma = ';'
	header := strings.Split("DataColeta;IdNumeric;GrupoProduto;Produto;RazaoSocialPosto;CnpjPosto;Distribuidora;Endereço;Complemento;Bairro;Município;Latitude;Longitude;Uf;RegiaoPolitica;Ensaio;Resultado;UnidadeEnsaio;Conforme", ";")
	if err = w.Write(header); err != nil {
		t.Fatal(err)
	}
	for _, scope := range [][2]string{{"2025-01-01 01:00:00", "11222333000181"}, {"2025-01-01 02:00:00", "11222333000181"}, {"2025-01-01 01:00:00", "11222333000262"}} {
		row := make([]string, 19)
		row[0], row[1], row[3], row[5], row[15], row[16], row[18] = scope[0], "1", "ETANOL HIDRATADO COMUM", scope[1], "Aspecto", "LII", "Sim"
		if err = w.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	w.Flush()
	if err = w.Error(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readPMQCSamples(t, path); len(got) != 3 {
		t.Fatalf("full CNPJ/clock sample scopes collapsed: %d", len(got))
	}
}

// TestPMQCMethodologyAgainstRealOpenData reconciles the domain judgment with
// the independent Python oracle over the official September 2026 open file
// (verified hashes in FQ-02C/FQ-05W2 evidence). Frozen counts below were
// produced by the oracle first and then reproduced here; any divergence in
// policy or data fails loudly.
func TestPMQCMethodologyAgainstRealOpenData(t *testing.T) {
	path := os.Getenv("ANPFUEL_TEST_PMQC_CSV")
	if path == "" {
		t.Skip("ANPFUEL_TEST_PMQC_CSV not set")
	}
	type counters struct{ conforming, nonconforming, unresolved int }
	families := map[string]*counters{}
	reasons := map[string]int{}
	total := 0
	for _, sample := range readPMQCSamples(t, path) {
		judgment := JudgeSample(sample.facts, sample.assays)
		family := countingKey(sample.facts.Fuel)
		if families[family] == nil {
			families[family] = &counters{}
		}
		total++
		switch judgment.Verdict {
		case SampleConforming:
			families[family].conforming++
		case SampleNonconforming:
			families[family].nonconforming++
		case SampleUnresolved:
			families[family].unresolved++
		}
		if len(judgment.Reasons) > 0 {
			reasons[judgment.Reasons[0]]++
		}
	}
	for family, c := range families {
		t.Logf("%s conforming=%d nonconforming=%d unresolved=%d", family, c.conforming, c.nonconforming, c.unresolved)
	}
	t.Logf("samples=%d reasons=%v", total, reasons)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	january := digest == "de013f3d7455236ec0c2846426e4beee072d3a0dc7f38c5c3a421da69302cafd"
	if !january && digest != "cecf8ac7d4eadddc65c7238f19c7a63498af3fc0273abbfc9828803a2cf67b7d" {
		t.Fatal("unrecognized official fixture hash")
	}
	want := map[string]counters{
		"gasoline":    {1457, 129, 0},
		"ethanol":     {1049, 29, 1},
		"diesel_s10":  {1018, 39, 3},
		"diesel_s500": {424, 15, 0},
	}
	wantTotal := 4164
	if january {
		wantTotal = 5776
		want = map[string]counters{"gasoline": {2068, 51, 0}, "ethanol": {1621, 44, 0}, "diesel_s10": {1312, 109, 2}, "diesel_s500": {522, 47, 0}}
	}
	if total != wantTotal {
		t.Fatalf("sample total = %d, want %d", total, wantTotal)
	}
	for family, w := range want {
		got := families[family]
		if got == nil || *got != w {
			t.Fatalf("%s = %+v, want %+v", family, got, w)
		}
	}
	if january {
		if reasons[ReasonReplicaConflict] != 2 || reasons[ReasonAspectoUnconfirmed] != 2 {
			t.Fatalf("January reason histogram diverged: %v", reasons)
		}
		return
	}
	if reasons[ReasonReplicaConflict] != 3 || reasons[ReasonDieselAspectoCleared] != 5 || reasons[ReasonDefectUnclassified] != 1 {
		t.Fatalf("reason histogram diverged: %v", reasons)
	}
}

type realSample struct {
	facts  SampleFacts
	assays []AssayObservation
}

// countingKey groups the oracle families the same way the Python reference
// does: gasoline and ethanol by family, diesel split by grade.
func countingKey(fuel Fuel) string {
	switch fuel {
	case FuelEthanol:
		return "ethanol"
	case FuelDieselS10:
		return "diesel_s10"
	case FuelDieselS500:
		return "diesel_s500"
	default:
		return "gasoline"
	}
}

// readPMQCSamples parses one official open-data CSV with the published 19
// columns. The mapping is test-only; production preparation stays in the
// reviewed native contract.
func readPMQCSamples(t *testing.T, path string) []realSample {
	t.Helper()
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = ';'
	r.FieldsPerRecord = 19
	r.LazyQuotes = true
	header, e := r.Read()
	if e != nil {
		t.Fatal(e)
	}
	index := map[string]int{}
	for i, name := range header {
		index[strings.TrimSpace(name)] = i
	}
	type key struct{ id, product, cnpj, clock string }
	grouped := map[key]*realSample{}
	order := []key{}
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		fuel, ok := realFuel(row[index["Produto"]])
		if !ok {
			t.Fatalf("unknown source product %q", row[index["Produto"]])
		}
		k := key{row[index["IdNumeric"]], row[index["Produto"]], row[index["CnpjPosto"]], row[index["DataColeta"]]}
		sample := grouped[k]
		if sample == nil {
			sample = &realSample{facts: SampleFacts{
				SampleKey:  "pmqc-assay-v1:" + strings.Join([]string{k.cnpj, k.clock, k.product, k.id}, "\x1f"),
				SourceCNPJ: row[index["CnpjPosto"]],
				ObservedOn: row[index["DataColeta"]],
				Fuel:       fuel,
			}}
			grouped[k] = sample
			order = append(order, k)
		}
		conformity, _ := ParseConformity(row[index["Conforme"]])
		sample.assays = append(sample.assays, AssayObservation{
			Ensaio:     row[index["Ensaio"]],
			Result:     row[index["Resultado"]],
			Conformity: conformity,
		})
	}
	out := make([]realSample, 0, len(order))
	for _, k := range order {
		out = append(out, *grouped[k])
	}
	return out
}

func realFuel(product string) (Fuel, bool) {
	switch product {
	case "GASOLINA C COMUM":
		return FuelGasolineRegular, true
	case "GASOLINA C COMUM ADITIVADA":
		return FuelGasolineAdditived, true
	case "GASOLINA C PREMIUM":
		return FuelGasolinePremiumGrade, true
	case "ETANOL HIDRATADO COMUM", "ETANOL HIDRATADO ADITIVADO":
		return FuelEthanol, true
	case "ÓLEO DIESEL B S10 - COMUM":
		return FuelDieselS10, true
	case "ÓLEO DIESEL B S10 - ADITIVADO":
		return FuelDieselS10, true
	case "ÓLEO DIESEL B S500 - COMUM", "ÓLEO DIESEL B S500 - ADITIVADO":
		return FuelDieselS500, true
	default:
		return "", false
	}
}
