package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPreparedManifestRefusedBeforeResolution(t *testing.T) {
	calls := 0
	_, e := Load(context.Background(), []byte(`{"schema_version":"unknown"}`), func(string) (io.ReadCloser, error) { calls++; return io.NopCloser(strings.NewReader("")), nil }, InputIdentity{}, nil)
	if e == nil || calls != 0 {
		t.Fatal("malformed manifest touched streams", e, calls)
	}
}

type fixture struct {
	raw      []byte
	files    map[string]string
	identity InputIdentity
}

func fixtureRows(t *testing.T, rows ...map[string]any) fixture {
	t.Helper()
	f := fixture{files: map[string]string{"assays.jsonl": "", "duplicates.jsonl": "", "quarantine.jsonl": ""}, identity: InputIdentity{SHA256: strings.Repeat("a", 64), Bytes: 10}}
	for _, row := range rows {
		raw, e := json.Marshal(row)
		if e != nil {
			t.Fatal(e)
		}
		f.files["assays.jsonl"] += string(raw) + "\n"
	}
	f.reseal(t, len(rows), 0, 0)
	return f
}
func (f *fixture) reseal(t *testing.T, accepted, duplicates, quarantine int) {
	t.Helper()
	c := map[string]any{"input": accepted + duplicates + quarantine, "accepted": accepted, "duplicates": duplicates, "quarantined": quarantine}
	outputs := []map[string]any{}
	for _, name := range []string{"assays.jsonl", "duplicates.jsonl", "quarantine.jsonl"} {
		raw := f.files[name]
		sum := sha256.Sum256([]byte(raw))
		outputs = append(outputs, map[string]any{"file": name, "bytes": len(raw), "sha256": hex.EncodeToString(sum[:])})
	}
	logical := strings.TrimSuffix(f.files["assays.jsonl"], "\n")
	if logical != "" {
		logical += "\n"
	}
	logical += fmt.Sprintf("counts:%d:%d:%d:%d", accepted+duplicates+quarantine, accepted, duplicates, quarantine)
	sum := sha256.Sum256([]byte(logical))
	var e error
	f.raw, e = json.Marshal(map[string]any{"schema_version": "quality-prepared-v1", "input_contract": "pmqc-native-19-v2026-10", "source_file": "synthetic.csv", "input_bytes": f.identity.Bytes, "input_sha256": f.identity.SHA256, "manifest_hash": hex.EncodeToString(sum[:]), "counts": c, "outputs": outputs})
	if e != nil {
		t.Fatal(e)
	}
}
func (f fixture) open(name string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.files[name])), nil
}
func row() map[string]any {
	return map[string]any{"schema_version": "quality-assay-v1", "source": "pmqc-assay", "source_key": "11222333000181", "sample_ref": "7|ETANOL HIDRATADO COMUM", "observed_at": "2026-09-04", "fuel": "ETHANOL", "assay_ref": "ASPECTO", "result_text": "<1,0 Límpido\u2028Ω", "unit": "", "conformity": "CONFORMING", "evidence_ref": "synthetic.csv#7#ASPECTO"}
}
func unbound(_ context.Context, r domain.SourceRecord) (domain.Decision, error) {
	return domain.Decision{Kind: domain.ResolutionUnmatched, Reason: "unmatched_cnpj", Link: domain.StationLink{RecordKey: r.RecordKey}}, nil
}
func (f fixture) load() (Batch, error) {
	return Load(context.Background(), f.raw, f.open, f.identity, unbound)
}

func TestPreparedLossAccountingEmptyValuesAndNoPrematureResolution(t *testing.T) {
	a := row()
	a["result_text"] = ""
	f := fixtureRows(t, a)
	f.files["duplicates.jsonl"] = f.files["assays.jsonl"]
	f.files["quarantine.jsonl"] = `{"schema_version":"station-quarantine-v1","source":"pmqc-assay","row_locator":"synthetic.csv:4","reason":"invalid_cnpj","detail":"Synthetic malformed identifier","source_key":null}` + "\n"
	f.reseal(t, 1, 1, 1)
	calls := 0
	resolve := func(ctx context.Context, r domain.SourceRecord) (domain.Decision, error) {
		calls++
		if r.CNPJ != "11222333000181" || r.ObservedAt != "2026-09-04" {
			t.Fatal("identity changed")
		}
		return unbound(ctx, r)
	}
	b, e := Load(context.Background(), f.raw, f.open, f.identity, resolve)
	if e != nil || len(b.Assays) != 1 || len(b.Links) != 1 || b.Counts.Input != 3 || b.Counts.Duplicates != 1 || b.Assays[0].ResultText != "" || b.Links[0].Active || calls != 1 {
		t.Fatal("round trip", len(b.Assays), b.Counts, calls, e)
	}
	if strings.Contains(b.Assays[0].SampleRef, "11222333000181") || len(b.Assays[0].SampleRef) != 66 {
		t.Fatal("public sample identifier")
	}
	f.files["duplicates.jsonl"] += "corrupt\n"
	calls = 0
	b, e = Load(context.Background(), f.raw, f.open, f.identity, resolve)
	if e == nil || calls != 0 || len(b.Assays) != 0 {
		t.Fatal("corruption resolved/published", calls, e)
	}
}

func TestPreparedFailuresAreClosedAndDoNotResolve(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*fixture)
	}{
		{"input hash", func(f *fixture) { f.identity.SHA256 = strings.Repeat("b", 64) }},
		{"input bytes", func(f *fixture) { f.identity.Bytes++ }},
		{"unknown field", func(f *fixture) {
			f.raw = append(append([]byte{}, f.raw[:len(f.raw)-1]...), []byte(`,"account_id":"private"}`)...)
		}},
		{"duplicate field", func(f *fixture) {
			f.raw = append(append([]byte{}, f.raw[:len(f.raw)-1]...), []byte(`,"input_bytes":10}`)...)
		}},
		{"path traversal", func(f *fixture) { f.raw = []byte(strings.Replace(string(f.raw), "assays.jsonl", "../assays.jsonl", 1)) }},
		{"oversize", func(f *fixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"input_bytes":10`, `"input_bytes":104857601`, 1))
			f.identity.Bytes = 104857601
		}},
		{"logical hash", func(f *fixture) {
			var m map[string]any
			_ = json.Unmarshal(f.raw, &m)
			m["manifest_hash"] = strings.Repeat("f", 64)
			f.raw, _ = json.Marshal(m)
		}},
		{"missing count", func(f *fixture) {
			var m map[string]any
			_ = json.Unmarshal(f.raw, &m)
			delete(m["counts"].(map[string]any), "duplicates")
			f.raw, _ = json.Marshal(m)
		}},
		{"unbalanced", func(f *fixture) { f.raw = []byte(strings.Replace(string(f.raw), `"input":1`, `"input":2`, 1)) }},
		{"CRLF output", func(f *fixture) {
			f.files["assays.jsonl"] = strings.ReplaceAll(f.files["assays.jsonl"], "\n", "\r\n")
			f.reseal(t, 1, 0, 0)
		}},
		{"recomputed truncated output", func(f *fixture) {
			f.files["assays.jsonl"] = strings.TrimSuffix(f.files["assays.jsonl"], "\n")
			f.reseal(t, 1, 0, 0)
		}},
		{"truncated stream", func(f *fixture) { f.files["assays.jsonl"] = strings.TrimSuffix(f.files["assays.jsonl"], "\n") }},
		{"duplicate without twin", func(f *fixture) { f.files["duplicates.jsonl"] = "{}\n"; f.reseal(t, 1, 1, 0) }},
		{"accepted duplicate", func(f *fixture) { f.files["assays.jsonl"] += f.files["assays.jsonl"]; f.reseal(t, 2, 0, 0) }},
		{"unknown row field", func(f *fixture) {
			f.files["assays.jsonl"] = strings.TrimSuffix(f.files["assays.jsonl"], "}\n") + `,"account_id":"private"}` + "\n"
			f.reseal(t, 1, 0, 0)
		}},
		{"invalid UTF-8", func(f *fixture) {
			f.files["assays.jsonl"] = strings.Replace(f.files["assays.jsonl"], "Límpido", string([]byte{0xff}), 1)
			f.reseal(t, 1, 0, 0)
		}},
		{"unknown fuel", func(f *fixture) {
			f.files["assays.jsonl"] = strings.Replace(f.files["assays.jsonl"], `"ETHANOL"`, `"UNKNOWN_FUEL"`, 1)
			f.reseal(t, 1, 0, 0)
		}},
		{"bad date", func(f *fixture) {
			f.files["assays.jsonl"] = strings.Replace(f.files["assays.jsonl"], "2026-09-04", "2026-02-30", 1)
			f.reseal(t, 1, 0, 0)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fixtureRows(t, row())
			tc.mutate(&f)
			calls := 0
			_, e := Load(context.Background(), f.raw, f.open, f.identity, func(ctx context.Context, r domain.SourceRecord) (domain.Decision, error) {
				calls++
				return unbound(ctx, r)
			})
			if e == nil || calls != 0 {
				t.Fatal("bad artifact admitted", calls, e)
			}
			if strings.Contains(e.Error(), "11222333000181") || strings.Contains(e.Error(), "private") {
				t.Fatal("source content in error")
			}
		})
	}
}

func TestPreparedSampleScopeAndResolution(t *testing.T) {
	rows := []map[string]any{row(), row(), row(), row()}
	rows[1]["observed_at"] = "2026-09-05"
	rows[2]["source_key"] = "04218406000104"
	rows[3]["fuel"] = "GASOLINE_REGULAR"
	// Rust ordering does not include fuel; the same-name/date fuel tie is valid.
	rows = []map[string]any{rows[2], rows[0], rows[3], rows[1]}
	f := fixtureRows(t, rows...)
	calls := 0
	resolve := func(_ context.Context, r domain.SourceRecord) (domain.Decision, error) {
		calls++
		return domain.Decision{Kind: domain.ResolutionBound, Reason: "exact cnpj", Link: domain.StationLink{RecordKey: r.RecordKey, StationID: "11111111-1111-4111-8111-111111111111", Automatic: true, Active: true}}, nil
	}
	b, e := Load(context.Background(), f.raw, f.open, f.identity, resolve)
	if e != nil || calls != 4 || len(b.Links) != 4 {
		t.Fatal(calls, e)
	}
	seen := map[string]bool{}
	for _, a := range b.Assays {
		if seen[a.SampleRef] {
			t.Fatal("different sample mixed")
		}
		seen[a.SampleRef] = true
	}
	replay, e := Load(context.Background(), f.raw, f.open, f.identity, resolve)
	if e != nil || !reflect.DeepEqual(b, replay) {
		t.Fatal("unstable replay", e)
	}
	_, e = Load(context.Background(), f.raw, f.open, f.identity, func(context.Context, domain.SourceRecord) (domain.Decision, error) {
		return domain.Decision{}, errors.New("synthetic failure")
	})
	if e == nil {
		t.Fatal("resolution failure hidden")
	}
}

func TestPreparedRejectsSeparatorCollisionAndOversizedFields(t *testing.T) {
	for _, bad := range []string{"x\x1fy", "x\x00y", strings.Repeat("a", 1025)} {
		a := row()
		a["result_text"] = bad
		f := fixtureRows(t, a)
		if _, e := f.load(); e == nil {
			t.Fatal("invalid field accepted")
		}
	}
	f := fixtureRows(t)
	b, e := f.load()
	if e != nil || len(b.Assays) != 0 || b.Counts.Input != 0 {
		t.Fatal("empty complete preparation", e)
	}
}

// Run explicitly with the verified private local September CSV/artifact paths.
// This is a repeatable manual source/size measurement, never a default CI input.
func BenchmarkNativePreparedBridge(b *testing.B) {
	dir, csv := os.Getenv("ANPFUEL_NATIVE_QUALITY_PREPARED"), os.Getenv("ANPFUEL_NATIVE_QUALITY_CSV")
	if dir == "" || csv == "" {
		b.Fatal("explicit verified native fixture paths required")
	}
	source, e := os.Open(csv)
	if e != nil {
		b.Fatal("native CSV unavailable")
	}
	sum := sha256.New()
	n, e := io.Copy(sum, io.LimitReader(source, MaxBytes+1))
	_ = source.Close()
	if e != nil || n > MaxBytes || hex.EncodeToString(sum.Sum(nil)) != "cecf8ac7d4eadddc65c7238f19c7a63498af3fc0273abbfc9828803a2cf67b7d" {
		b.Fatal("verified September fixture identity mismatch")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		b.Fatal("native artifacts unavailable")
	}
	identity := InputIdentity{Bytes: n, SHA256: hex.EncodeToString(sum.Sum(nil))}
	open := func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(dir, name)) }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch, e := Load(context.Background(), raw, open, identity, unbound)
		if e != nil {
			b.Fatal(e)
		}
		if batch.Counts.Input != 66225 || batch.Counts.Accepted != 34158 || batch.Counts.Duplicates != 32067 || batch.Counts.Quarantined != 0 || len(batch.Links) != 4164 {
			b.Fatal("native reconciliation differs")
		}
	}
}

func TestPreparedReviewDecisionsNeverActivateCandidates(t *testing.T) {
	f := fixtureRows(t, row())
	for _, kind := range []domain.ResolutionKind{domain.ResolutionUnmatched, domain.ResolutionAmbiguous, domain.ResolutionWrongBranch, domain.ResolutionSuccession} {
		b, e := Load(context.Background(), f.raw, f.open, f.identity, func(_ context.Context, r domain.SourceRecord) (domain.Decision, error) {
			return domain.Decision{Kind: kind, Reason: string(kind), Link: domain.StationLink{RecordKey: r.RecordKey, StationID: "11111111-1111-4111-8111-111111111111"}}, nil
		})
		if e != nil || len(b.Links) != 1 || b.Links[0].Active || b.Links[0].StationID != "" {
			t.Fatal("candidate became attribution", kind, e)
		}
	}
	for _, decision := range []domain.Decision{{Kind: domain.ResolutionBound, Reason: "exact cnpj", Link: domain.StationLink{StationID: "bad", Active: true}}, {Kind: domain.ResolutionUnmatched, Reason: "unknown", Link: domain.StationLink{Active: true}}, {Kind: "invented", Reason: "bad"}} {
		if b, e := Load(context.Background(), f.raw, f.open, f.identity, func(context.Context, domain.SourceRecord) (domain.Decision, error) { return decision, nil }); e == nil || len(b.Assays) != 0 {
			t.Fatal("invalid resolver contract", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opens := 0
	if _, e := Load(ctx, f.raw, func(string) (io.ReadCloser, error) { opens++; return nil, nil }, f.identity, unbound); !errors.Is(e, context.Canceled) || opens != 0 {
		t.Fatal("cancelled preparation did work", e, opens)
	}
}
