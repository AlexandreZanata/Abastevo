package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

type fineFixture struct {
	raw      []byte
	files    map[string]string
	identity InputIdentity
}

func canonicalTest(v any) []byte {
	var b strings.Builder
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return []byte(strings.TrimSuffix(b.String(), "\n"))
}
func sampleFine() fineObservation {
	amount, paid, year := int64(15000000), int64(18000001), 2025
	a := fineObservation{Schema: "anp-fine-observation-v1", Source: "sanctions", SourceKey: "11222333000181", Proceeding: "000000000000001", DUF: "SYNTHETIC/1", Department: "SYNTHETIC", Status: "Processo Pago", Due: "2025-02-28", AmountText: "150000,00", Amount: &amount, PaidText: "180000,01", Paid: &paid, Year: &year, Month: "2026-01"}
	sum := sha256.Sum256(canonicalTest(a))
	a.Key = "observation:" + hex.EncodeToString(sum[:])
	a.Evidence = "synthetic.csv:8"
	return a
}
func fineSample(t *testing.T) fineFixture {
	t.Helper()
	f := fineFixture{files: map[string]string{"sanctions.jsonl": string(canonicalTest(sampleFine())) + "\n", "duplicates.jsonl": "", "quarantine.jsonl": ""}, identity: InputIdentity{SHA256: strings.Repeat("a", 64), Bytes: 10}}
	f.seal(t, 1, 0, 0)
	return f
}
func (f *fineFixture) seal(t *testing.T, a, d, q int) {
	t.Helper()
	outputs := []map[string]any{}
	for _, name := range []string{"sanctions.jsonl", "duplicates.jsonl", "quarantine.jsonl"} {
		sum := sha256.Sum256([]byte(f.files[name]))
		outputs = append(outputs, map[string]any{"file": name, "bytes": len(f.files[name]), "sha256": hex.EncodeToString(sum[:])})
	}
	arrays := []string{}
	for _, name := range []string{"sanctions.jsonl", "duplicates.jsonl", "quarantine.jsonl"} {
		arrays = append(arrays, "["+strings.Join(strings.FieldsFunc(strings.TrimSuffix(f.files[name], "\n"), func(r rune) bool { return r == '\n' }), ",")+"]")
	}
	logical := fmt.Sprintf(`[2025,"2026-01",%s,%s,%s,%d,%d,%d,%d]`, arrays[0], arrays[1], arrays[2], a+d+q, a, d, q)
	sum := sha256.Sum256([]byte(logical))
	var err error
	f.raw, err = json.Marshal(map[string]any{"schema_version": "sanctions-prepared-v1", "input_contract": "anp-sicom-cp850-9-v1", "source_file": "synthetic.csv", "input_bytes": f.identity.Bytes, "input_sha256": f.identity.SHA256, "report_year": 2025, "extraction_month": "2026-01", "manifest_hash": hex.EncodeToString(sum[:]), "counts": map[string]int{"input": a + d + q, "accepted": a, "duplicates": d, "quarantined": q}, "outputs": outputs})
	if err != nil {
		t.Fatal(err)
	}
}
func (f fineFixture) open(name string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.files[name])), nil
}
func (f fineFixture) load() (FineBatch, error) {
	return LoadFines(context.Background(), f.raw, f.open, f.identity, 2025)
}
func TestFineTypedBoundaryAndLossCensus(t *testing.T) {
	f := fineSample(t)
	b, e := f.load()
	if e != nil || len(b.Observations) != 1 || b.Counts.Input != 1 || b.ExtractionMonth != "2026-01" || b.ReportYear != 2025 {
		t.Fatal("native typed contract", b.Counts, e)
	}
	a := b.Observations[0]
	if a.DueOn != "2025-02-28" || a.AmountMinor != 15000000 || a.PaidMinor != 18000001 || a.ProceedingRef != "000000000000001" {
		t.Fatal("due/money/source precision changed")
	}
	twin := sampleFine()
	twin.Evidence = "synthetic.csv:9"
	f.files["duplicates.jsonl"] = string(canonicalTest(twin)) + "\n"
	f.files["quarantine.jsonl"] = `{"schema_version":"station-quarantine-v1","source":"sanctions","row_locator":"synthetic.csv:10","reason":"invalid_cnpj","detail":"official fine row refused; raw fields redacted","source_key":null}` + "\n"
	f.seal(t, 1, 1, 1)
	b, e = f.load()
	if e != nil || b.Counts.Input != 3 || b.Counts.Duplicates != 1 || b.Counts.Quarantined != 1 {
		t.Fatal("loss accounting", e)
	}
}
func TestFineBoundaryRejectsTamperWithoutPartialData(t *testing.T) {
	cases := map[string]func(*fineFixture){
		"input identity": func(f *fineFixture) { f.identity.Bytes++ },
		"output tamper":  func(f *fineFixture) { f.files["sanctions.jsonl"] += "corrupt\n" },
		"missing newline resealed": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.TrimSuffix(f.files["sanctions.jsonl"], "\n")
			f.seal(t, 1, 0, 0)
		},
		"CPF": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.ReplaceAll(f.files["sanctions.jsonl"], "11222333000181", "12345678900")
			f.seal(t, 1, 0, 0)
		},
		"unknown field": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.Replace(f.files["sanctions.jsonl"], `"source_key":`, `"name":"private","source_key":`, 1)
			f.seal(t, 1, 0, 0)
		},
		"repeated JSON field": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.Replace(f.files["sanctions.jsonl"], `"status":`, `"Status":"private","status":`, 1)
			f.seal(t, 1, 0, 0)
		},
		"noncanonical JSON": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.Replace(f.files["sanctions.jsonl"], `"status":`, `"status": `, 1)
			f.seal(t, 1, 0, 0)
		},
		"money mismatch": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.Replace(f.files["sanctions.jsonl"], `"amount_minor":15000000`, `"amount_minor":1`, 1)
			f.seal(t, 1, 0, 0)
		},
		"missing zero amount": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.Replace(f.files["sanctions.jsonl"], `"paid_minor":18000001,`, "", 1)
			f.seal(t, 1, 0, 0)
		},
		"calendar": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.ReplaceAll(f.files["sanctions.jsonl"], "2025-02-28", "2025-02-30")
			f.seal(t, 1, 0, 0)
		},
		"wrong year": func(f *fineFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"report_year":2025`, `"report_year":2024`, 1))
		},
		"month precision": func(f *fineFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"extraction_month":"2026-01"`, `"extraction_month":"2026-01-01"`, 1))
		},
		"duplicate accepted": func(f *fineFixture) { f.files["sanctions.jsonl"] += f.files["sanctions.jsonl"]; f.seal(t, 2, 0, 0) },
		"missing locator": func(f *fineFixture) {
			f.files["sanctions.jsonl"] = strings.ReplaceAll(f.files["sanctions.jsonl"], "synthetic.csv:8", "synthetic.csv:9")
			f.seal(t, 1, 0, 0)
		},
		"duplicate without twin": func(f *fineFixture) {
			f.files["duplicates.jsonl"] = f.files["sanctions.jsonl"]
			f.files["sanctions.jsonl"] = ""
			f.seal(t, 0, 1, 0)
		},
		"hash": func(f *fineFixture) {
			var m map[string]any
			_ = json.Unmarshal(f.raw, &m)
			m["manifest_hash"] = strings.Repeat("f", 64)
			f.raw, _ = json.Marshal(m)
		},
		"output path": func(f *fineFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), "sanctions.jsonl", "../sanctions.jsonl", 1))
		},
		"output count": func(f *fineFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"accepted":1`, `"accepted":0`, 1))
		},
		"quarantine private key": func(f *fineFixture) {
			f.files["quarantine.jsonl"] = `{"schema_version":"station-quarantine-v1","source":"sanctions","row_locator":"synthetic.csv:9","reason":"invalid_cnpj","detail":"official fine row refused; raw fields redacted","source_key":"private"}` + "\n"
			f.seal(t, 1, 0, 1)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := fineSample(t)
			mutate(&f)
			b, e := f.load()
			if e == nil || !reflect.DeepEqual(b, FineBatch{}) {
				t.Fatal("failure yielded batch", e)
			}
		})
	}
}
func TestFineCancellationPortAndCloseFailures(t *testing.T) {
	f := fineSample(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	b, e := LoadFines(ctx, f.raw, func(string) (io.ReadCloser, error) { calls++; return nil, errors.New("private raw") }, f.identity, 2025)
	if e == nil || calls != 0 || !reflect.DeepEqual(b, FineBatch{}) {
		t.Fatal("cancelled read", e)
	}
	_, e = LoadFines(context.Background(), f.raw, nil, f.identity, 2025)
	if e == nil {
		t.Fatal("nil port accepted")
	}
	_, e = LoadFines(context.Background(), f.raw, func(string) (io.ReadCloser, error) { return nil, errors.New("private source row") }, f.identity, 2025)
	if e == nil || strings.Contains(e.Error(), "private source row") {
		t.Fatal("raw error leaked")
	}
	_, e = LoadFines(context.Background(), f.raw, func(name string) (io.ReadCloser, error) {
		return badFineClose{Reader: strings.NewReader(f.files[name])}, nil
	}, f.identity, 2025)
	if e == nil {
		t.Fatal("close failure accepted")
	}
}

type badFineClose struct{ io.Reader }

func (badFineClose) Close() error { return errors.New("private close") }
