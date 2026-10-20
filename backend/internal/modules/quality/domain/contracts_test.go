package domain

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestDomainStdlibOnly(t *testing.T) {
	// Same architecture rule as kernel/directory: no I/O, SQL or network in
	// quality grouping/identifier decisions. Test files are exempt.
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, ".") {
				t.Errorf("%s imports non-stdlib %q", name, path)
			}
		}
	}
}

func TestFuelParityWithKernel(t *testing.T) {
	// FQ-01 reuses the kernel wire vocabulary: every quality fuel must be a
	// kernel product with a fixed unit, and ANP source labels must map to
	// the same values instead of a parallel enum.
	if len(AllFuels) != 8 {
		t.Fatalf("closed v1 vocabulary changed: %d fuels", len(AllFuels))
	}
	for _, f := range AllFuels {
		if _, err := kernel.Product(string(f)).Unit(); err != nil {
			t.Errorf("fuel %q is not a kernel product: %v", f, err)
		}
	}
	mapped := map[string]Fuel{
		"ETANOL":             FuelEthanol,
		"GASOLINA COMUM":     FuelGasolineRegular,
		"GASOLINA ADITIVADA": FuelGasolineAdditived,
		"OLEO DIESEL S500":   FuelDieselS500,
		"OLEO DIESEL S10":    FuelDieselS10,
		"GNV":                FuelCNG,
		"GLP P13":            FuelLPGP13,
	}
	for label, want := range mapped {
		got, err := kernel.ParseProduct(label)
		if err != nil || string(got) != string(want) {
			t.Errorf("label %q maps to %q, want %q (err %v)", label, got, want, err)
		}
	}
	// Unknown source fuels (e.g. hydrogen, unmapped premium text) fail in
	// the kernel so importers quarantine with ReasonUnmappedFuel.
	if _, err := kernel.ParseProduct("HIDROGENIO"); err == nil {
		t.Error("unknown fuel accepted")
	}
	if _, err := kernel.ParseProduct("GASOLINA PREMIUM"); err == nil {
		t.Error("unverified premium label accepted before FQ-02 vocabulary freeze")
	}
}

func TestParseConformity(t *testing.T) {
	for text, want := range map[string]Conformity{
		"Conforme":     Conforming,
		"Sim":          Conforming,
		"Não":          Nonconforming,
		"CONFORME":     Conforming,
		"Não Conforme": Nonconforming,
		"NAO CONFORME": Nonconforming,
		"não conforme": Nonconforming,
		"Inconclusivo": Inconclusive,
		"INCONCLUSIVO": Inconclusive,
	} {
		if got, err := ParseConformity(text); err != nil || got != want {
			t.Errorf("conformity %q -> %q, want %q (err %v)", text, got, want, err)
		}
	}
	for _, text := range []string{"", "  ", "PENDENTE", "EM ANALISE", "OK"} {
		if got, err := ParseConformity(text); err == nil || got != UnknownStatus {
			t.Errorf("status %q -> %q without explicit unknown (err %v)", text, got, err)
		}
	}
}

func sample(ref string) SampleID {
	return SampleID{Stream: StreamPMQCAssay, NativeRef: ref}
}

func TestVerifiedColumnsGroup(t *testing.T) {
	// Every verified coordinate column rides the sample identity; assay
	// content groups beneath it without loss.
	rows := []Assay{
		{Sample: sample("AMOSTRA-001"), AssayRef: "ENSAIO-ASPECTO", ResultText: "conforme", Unit: "-", Status: Conforming},
		{Sample: sample("AMOSTRA-001"), AssayRef: "ENSAIO-DESTILACAO", ResultText: "181,5", Unit: "°C", Status: Conforming},
	}
	g := GroupAssays(rows, []string{"pmqc.csv:2", "pmqc.csv:3"})
	if g.Accepted() != 2 || g.Duplicates != 0 || len(g.Quarantined) != 0 {
		t.Fatalf("grouped = %+v", g)
	}
	if len(g.Samples) != 1 {
		t.Fatalf("one sample with two assays grouped as %d samples", len(g.Samples))
	}
}

func TestRepeatedAssaysRetained(t *testing.T) {
	rows := []Assay{
		{Sample: sample("S-1"), AssayRef: "A", ResultText: "x", Status: Conforming},
		{Sample: sample("S-1"), AssayRef: "B", ResultText: "y", Status: Nonconforming},
		{Sample: sample("S-1"), AssayRef: "C", ResultText: "texto livre", Status: UnknownStatus},
	}
	g := GroupAssays(rows, nil)
	if g.Accepted() != 3 || g.Duplicates != 0 || len(g.Quarantined) != 0 {
		t.Fatalf("distinct assays must all be retained: %+v", g)
	}
}

func TestExactDuplicateDeduped(t *testing.T) {
	row := Assay{Sample: sample("S-1"), AssayRef: "A", ResultText: "181,5", Unit: "°C", Status: Conforming}
	g := GroupAssays([]Assay{row, row}, []string{"f:2", "f:3"})
	if g.Accepted() != 1 || g.Duplicates != 1 || len(g.Quarantined) != 0 {
		t.Fatalf("exact repeat must dedupe with count: %+v", g)
	}
	// Same key but different result is a different determination, not a repeat.
	changed := row
	changed.ResultText = "182,0"
	g = GroupAssays([]Assay{row, changed}, nil)
	if g.Accepted() != 2 || g.Duplicates != 0 {
		t.Fatalf("changed result must not dedupe: %+v", g)
	}
}

func TestSameDaySeparateSamples(t *testing.T) {
	rows := []Assay{
		{Sample: sample("S-MANHA"), AssayRef: "A", ResultText: "ok", Status: Conforming},
		{Sample: sample("S-TARDE"), AssayRef: "A", ResultText: "ok", Status: Conforming},
	}
	g := GroupAssays(rows, nil)
	if len(g.Samples) != 2 || g.Accepted() != 2 {
		t.Fatalf("same-day separate native refs must stay distinct: %+v", g)
	}
}

func TestMissingIdentifiersQuarantined(t *testing.T) {
	rows := []Assay{
		{Sample: SampleID{Stream: StreamPMQCAssay}, AssayRef: "A", ResultText: "x"},
		{Sample: sample("S-1"), ResultText: "x"},
		{Sample: SampleID{}, AssayRef: "A"},
	}
	g := GroupAssays(rows, []string{"f:2", "f:3", "f:4"})
	if g.Accepted() != 0 || g.Duplicates != 0 || len(g.Quarantined) != 3 {
		t.Fatalf("missing identifiers must quarantine all rows: %+v", g)
	}
	if g.Quarantined[0].Reason != ReasonMissingSample {
		t.Errorf("empty sample ref -> %q, want %q", g.Quarantined[0].Reason, ReasonMissingSample)
	}
	if g.Quarantined[1].Reason != ReasonMissingAssay {
		t.Errorf("empty assay ref -> %q, want %q", g.Quarantined[1].Reason, ReasonMissingAssay)
	}
}

func TestLossAccountingHolds(t *testing.T) {
	row := func(s, a string) Assay {
		return Assay{Sample: sample(s), AssayRef: a, ResultText: "x", Status: Conforming}
	}
	rows := []Assay{
		row("S-1", "A"), row("S-1", "B"), row("S-1", "A"),
		row("S-2", "A"), {Sample: sample("S-3")},
		{}, row("S-2", "A"),
	}
	g := GroupAssays(rows, nil)
	input := len(rows)
	if g.Accepted()+g.Duplicates+len(g.Quarantined) != input {
		t.Fatalf("input %d != accepted %d + duplicates %d + quarantined %d",
			input, g.Accepted(), g.Duplicates, len(g.Quarantined))
	}
}

func TestCorrectionAppendOnly(t *testing.T) {
	history := []Revision{{Supersedes: "rev-1", Note: "initial import"}}
	known := []string{"rev-1", "rev-2"}
	next, err := ApplyCorrection(history, Revision{Supersedes: "rev-2", Note: "fix station link"}, known)
	if err != nil || len(next) != 2 || len(history) != 1 {
		t.Fatalf("correction must append without mutating: %+v, err %v", next, err)
	}
	if history[0].Note != "initial import" {
		t.Fatal("original revision rewritten")
	}
	if _, err := ApplyCorrection(history, Revision{Supersedes: "rev-2"}, known); err == nil {
		t.Error("reasonless correction accepted")
	}
	if _, err := ApplyCorrection(history, Revision{Supersedes: "rev-9", Note: "x"}, known); err == nil {
		t.Error("correction superseding unknown revision accepted")
	}
}

func TestLinkCollisionUnderReview(t *testing.T) {
	links := []StationLink{
		{StationID: "st-a", RecordKey: "pmqc:S-1", Automatic: true, Reason: "exact cnpj", Active: true},
		{StationID: "st-b", RecordKey: "pmqc:S-1", Automatic: false, Reason: "reviewed succession", Active: true},
		{StationID: "st-c", RecordKey: "pmqc:S-2", Automatic: true, Reason: "exact cnpj", Active: true},
	}
	out := ReviewLinks(links)
	active := 0
	for _, l := range out {
		if l.Active {
			active++
			if l.RecordKey == "pmqc:S-1" {
				t.Errorf("collided record %q stays active on %s", l.RecordKey, l.StationID)
			}
		}
		if l.RecordKey == "pmqc:S-1" && !strings.Contains(l.Reason, ReasonLinkCollision) {
			t.Errorf("collision reason missing on %+v", l)
		}
	}
	if active != 1 {
		t.Fatalf("exactly the undisputed link must survive, got %d active", active)
	}
	// No input mutation: the caller's slice keeps its flags.
	if !links[0].Active || !links[1].Active {
		t.Fatal("ReviewLinks mutated its input")
	}
}

type manifestVector struct {
	ID     string `json:"id"`
	Schema string `json:"schema"`
	Valid  bool   `json:"valid"`
	Value  struct {
		Source      string `json:"source"`
		Stream      string `json:"stream"`
		Edition     string `json:"edition"`
		Resources   []Resource
		Counts      Counts
		Predecessor string `json:"predecessor"`
	} `json:"value"`
}

func TestManifestFixtures(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts", "testdata", "quality", "manifest-*.json"))
	if err != nil || len(matches) != 2 {
		t.Fatalf("expected 2 manifest vectors: %v %v", matches, err)
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var v manifestVector
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
		if v.Schema != "QualityManifest" {
			t.Errorf("%s: schema %q", filepath.Base(path), v.Schema)
		}
		m := Manifest{
			Source: v.Value.Source, Stream: v.Value.Stream,
			Edition: v.Value.Edition, Resources: v.Value.Resources,
			Counts: v.Value.Counts, Predecessor: v.Value.Predecessor,
		}
		err = m.Reconcile()
		if v.Valid && err != nil {
			t.Errorf("%s: valid manifest refused: %v", filepath.Base(path), err)
		}
		if !v.Valid && err == nil {
			t.Errorf("%s: invalid manifest accepted", filepath.Base(path))
		}
	}
}

func TestManifestRefusals(t *testing.T) {
	good := Manifest{
		Source: "pmqc", Stream: StreamPMQCAssay, Edition: "2026-09",
		Resources: []Resource{{URL: "https://example.invalid/p.zip", SHA256: "abc", Bytes: 1}},
		Counts:    Counts{Input: 10, Accepted: 7, Duplicates: 2, Quarantined: 1},
	}
	if err := good.Reconcile(); err != nil {
		t.Fatalf("balanced manifest refused: %v", err)
	}
	empty := good
	empty.Resources = nil
	if err := empty.Reconcile(); err == nil {
		t.Error("resourceless manifest accepted")
	}
	bad := good
	bad.Counts.Input = 9
	if err := bad.Reconcile(); err == nil {
		t.Error("mismatched counts accepted")
	}
}

func TestValidDayUsesGregorianCalendar(t *testing.T) {
	for _, day := range []string{"2024-02-29", "2000-02-29", "2026-09-30"} {
		if !ValidDay(day) {
			t.Fatal("valid calendar day refused", day)
		}
	}
	for _, day := range []string{"2026-02-30", "2026-02-29", "1900-02-29", "2026-13-01", "2026-00-01", "2026-09-00", "2026-9-01", "0000-01-01"} {
		if ValidDay(day) {
			t.Fatal("invalid calendar day accepted", day)
		}
	}
}
