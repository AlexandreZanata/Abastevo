package domain

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if filepath.Base(path) != "PMQC_2026_09.csv" {
		return
	}
	want := map[string]counters{
		"gasoline":    {1457, 129, 0},
		"ethanol":     {1049, 29, 1},
		"diesel_s10":  {1018, 39, 3},
		"diesel_s500": {424, 15, 0},
	}
	if total != 4164 {
		t.Fatalf("sample total = %d, want 4164", total)
	}
	for family, w := range want {
		got := families[family]
		if got == nil || *got != w {
			t.Fatalf("%s = %+v, want %+v", family, got, w)
		}
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
	type key struct{ id, product string }
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
		k := key{row[index["IdNumeric"]], row[index["Produto"]]}
		sample := grouped[k]
		if sample == nil {
			sample = &realSample{facts: SampleFacts{
				SampleKey:  "pmqc-assay-v1:" + k.id,
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
