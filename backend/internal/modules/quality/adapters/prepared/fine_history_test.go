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

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

type historyFixture struct {
	raw      []byte
	files    map[string]string
	identity InputIdentity
}

func historySample(t *testing.T) historyFixture {
	t.Helper()
	paid := int64(0)
	row := application.HistoricalFineObservation{Schema: "anp-fine-history-observation-v1", Source: "sanctions", SourceCNPJ: "11222333000181", ProceedingRef: "000000000000001", ReferenceLabel: "Auto de Infração", ReferenceRef: "SYNTHETIC/1", Status: "Unknown source status", DueOn: "2020-02-28", AmountText: " R$ 1.234,56 ", AmountMinor: 123456, PaidText: " R$ 0,00 ", PaidMinor: &paid, SourcePeriod: "2020", Clock: application.FineHistoryClock{Precision: "MONTH", Value: "2021-02"}, InputContract: "anp-sicom-utf8bom-2020-v1", EvidenceRef: "multas-aplicadas-2020.csv:8"}
	row.ObservationKey = row.ObservationIdentity()
	accepted := string(canonicalTest(row)) + "\n"
	row.EvidenceRef = "multas-aplicadas-2020.csv:9"
	duplicate := string(canonicalTest(row)) + "\n"
	quarantine := `{"schema_version":"station-quarantine-v1","source":"sanctions","row_locator":"multas-aplicadas-2020.csv:10","reason":"invalid_cnpj","detail":"historical fine row classified; raw fields redacted","source_key":null}` + "\n" + `{"schema_version":"station-quarantine-v1","source":"sanctions","row_locator":"multas-aplicadas-2020.csv:11","reason":"source_metadata","detail":"historical fine row classified; raw fields redacted","source_key":null}` + "\n"
	note := application.FineHistoryNote{Schema: "anp-fine-source-note-v1", Kind: "BLANK_SEPARATOR", EvidenceRef: "multas-aplicadas-2020.csv:11"}
	f := historyFixture{files: map[string]string{"history.jsonl": accepted, "duplicates.jsonl": duplicate, "quarantine.jsonl": quarantine, "source_notes.jsonl": string(canonicalTest(note)) + "\n"}, identity: InputIdentity{SHA256: strings.Repeat("a", 64), Bytes: 100}}
	f.seal(t)
	return f
}
func (f *historyFixture) seal(t *testing.T) {
	t.Helper()
	var outputs []map[string]any
	var arrays []string
	var counts []int
	for _, name := range []string{"history.jsonl", "duplicates.jsonl", "quarantine.jsonl", "source_notes.jsonl"} {
		body := f.files[name]
		sum := sha256.Sum256([]byte(body))
		outputs = append(outputs, map[string]any{"file": name, "bytes": len(body), "sha256": hex.EncodeToString(sum[:])})
		lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
		if body == "" {
			lines = nil
		}
		arrays = append(arrays, "["+strings.Join(lines, ",")+"]")
		counts = append(counts, len(lines))
	}
	logical := fmt.Sprintf(`["anp-sicom-utf8bom-2020-v1","2020",{"precision":"MONTH","value":"2021-02"},7,%s,%s,%s,%s,%d,%d,%d,%d]`, arrays[0], arrays[1], arrays[2], arrays[3], counts[0]+counts[1]+counts[2], counts[0], counts[1], counts[2])
	hash := sha256.Sum256([]byte(logical))
	var e error
	f.raw, e = json.Marshal(map[string]any{"schema_version": "fine-history-prepared-v1", "input_contract": "anp-sicom-utf8bom-2020-v1", "source_file": "multas-aplicadas-2020.csv", "input_bytes": f.identity.Bytes, "input_sha256": f.identity.SHA256, "source_period": "2020", "source_clock": map[string]string{"precision": "MONTH", "value": "2021-02"}, "header_record": 7, "metadata_records": counts[3], "manifest_hash": hex.EncodeToString(hash[:]), "counts": map[string]int{"input": counts[0] + counts[1] + counts[2], "accepted": counts[0], "duplicates": counts[1], "quarantined": counts[2]}, "outputs": outputs})
	if e != nil {
		t.Fatal(e)
	}
}
func (f historyFixture) open(name string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.files[name])), nil
}
func (f historyFixture) load() (FineHistoryBatch, error) {
	return LoadFineHistory(context.Background(), f.raw, f.open, f.identity, "2020")
}
func TestFineHistoryTypedLossMetadataAndOriginalRowCensus(t *testing.T) {
	f := historySample(t)
	b, e := f.load()
	if e != nil || b.Counts.Input != 4 || b.Counts.Accepted != 1 || b.Counts.Duplicates != 1 || b.Counts.Quarantined != 2 || len(b.Notes) != 1 || b.MetadataRecords != 1 || b.HeaderRecord != 7 {
		t.Fatal("typed history loss/metadata contract", e)
	}
	r := b.Observations[0]
	if r.ReferenceLabel != "Auto de Infração" || r.Department != "" || r.PaidMinor == nil || *r.PaidMinor != 0 || r.Clock.Precision != "MONTH" || r.Clock.Value != "2021-02" {
		t.Fatal("source facts changed")
	}
}
func TestFineHistoryTamperAlwaysReturnsNoPartialBatch(t *testing.T) {
	cases := map[string]func(*historyFixture){
		"valid row after footer": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], ".csv:8", ".csv:11", 1)
			f.files["quarantine.jsonl"] = strings.Replace(f.files["quarantine.jsonl"], ".csv:11", ".csv:8", 1)
			f.files["source_notes.jsonl"] = strings.Replace(f.files["source_notes.jsonl"], ".csv:11", ".csv:8", 1)
			f.seal(t)
		},
		"input bytes":   func(f *historyFixture) { f.identity.Bytes++ },
		"input hash":    func(f *historyFixture) { f.identity.SHA256 = strings.Repeat("b", 64) },
		"stream tamper": func(f *historyFixture) { f.files["history.jsonl"] += "tamper\n" },
		"truncated resealed": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.TrimSuffix(f.files["history.jsonl"], "\n")
			f.seal(t)
		},
		"noncanonical": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], `"status":`, `"status": `, 1)
			f.seal(t)
		},
		"unknown field": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], `"status":`, `"name":"private","status":`, 1)
			f.seal(t)
		},
		"repeated field": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], `"status":`, `"Status":"private","status":`, 1)
			f.seal(t)
		},
		"missing nullable field": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], `"source_transit_on":null,`, "", 1)
			f.seal(t)
		},
		"CPF": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], "11222333000181", "00000000000", 1)
			f.seal(t)
		},
		"AI becomes DUF": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], "Auto de Infração", "Número do DUF", 1)
			f.seal(t)
		},
		"money mismatch": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], `"amount_minor":123456`, `"amount_minor":123457`, 1)
			f.seal(t)
		},
		"duplicate locator": func(f *historyFixture) {
			f.files["duplicates.jsonl"] = strings.Replace(f.files["duplicates.jsonl"], ".csv:9", ".csv:8", 1)
			f.seal(t)
		},
		"locator hole": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], ".csv:8", ".csv:12", 1)
			f.seal(t)
		},
		"locator leading zero": func(f *historyFixture) {
			f.files["history.jsonl"] = strings.Replace(f.files["history.jsonl"], ".csv:8", ".csv:08", 1)
			f.seal(t)
		},
		"duplicate no twin": func(f *historyFixture) { f.files["history.jsonl"] = ""; f.seal(t) },
		"note on CPF": func(f *historyFixture) {
			f.files["source_notes.jsonl"] = strings.Replace(f.files["source_notes.jsonl"], ".csv:11", ".csv:10", 1)
			f.seal(t)
		},
		"missing note":  func(f *historyFixture) { f.files["source_notes.jsonl"] = ""; f.seal(t) },
		"repeated note": func(f *historyFixture) { f.files["source_notes.jsonl"] += f.files["source_notes.jsonl"]; f.seal(t) },
		"private note": func(f *historyFixture) {
			f.files["source_notes.jsonl"] = strings.Replace(f.files["source_notes.jsonl"], `"text":""`, `"text":"private body"`, 1)
			f.seal(t)
		},
		"quarantine key": func(f *historyFixture) {
			f.files["quarantine.jsonl"] = strings.Replace(f.files["quarantine.jsonl"], `"source_key":null`, `"source_key":"private"`, 1)
			f.seal(t)
		},
		"quarantine detail": func(f *historyFixture) {
			f.files["quarantine.jsonl"] = strings.ReplaceAll(f.files["quarantine.jsonl"], "historical fine row classified; raw fields redacted", "private source body")
			f.seal(t)
		},
		"path": func(f *historyFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), "history.jsonl", "../history.jsonl", 1))
		},
		"wrong source file": func(f *historyFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), "multas-aplicadas-2020.csv", "invented.csv", 1))
		},
		"wrong header": func(f *historyFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"header_record":7`, `"header_record":6`, 1))
		},
		"clock precision": func(f *historyFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"precision":"MONTH"`, `"precision":"DAY"`, 1))
		},
		"wrong period": func(f *historyFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"source_period":"2020"`, `"source_period":"2021"`, 1))
		},
		"missing metadata count": func(f *historyFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"metadata_records":1,`, "", 1))
		},
		"negative count": func(f *historyFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"duplicates":1`, `"duplicates":-1`, 1))
		},
		"wrong logical digest": func(f *historyFixture) {
			var m map[string]any
			_ = json.Unmarshal(f.raw, &m)
			m["manifest_hash"] = strings.Repeat("f", 64)
			f.raw, _ = json.Marshal(m)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := historySample(t)
			change(&f)
			b, e := f.load()
			if e == nil || !reflect.DeepEqual(b, FineHistoryBatch{}) {
				t.Fatal("tamper returned partial typed facts", e)
			}
		})
	}
}
func TestFineHistoryCancellationPortsAndCloseFailureRedact(t *testing.T) {
	f := historySample(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	b, e := LoadFineHistory(ctx, f.raw, func(string) (io.ReadCloser, error) { calls++; return nil, errors.New("private") }, f.identity, "2020")
	if e == nil || calls != 0 || !reflect.DeepEqual(b, FineHistoryBatch{}) {
		t.Fatal("cancelled input read")
	}
	for _, open := range []OpenStream{nil, func(string) (io.ReadCloser, error) { return nil, errors.New("private body") }, func(name string) (io.ReadCloser, error) {
		return badFineClose{Reader: strings.NewReader(f.files[name])}, nil
	}} {
		b, e := LoadFineHistory(context.Background(), f.raw, open, f.identity, "2020")
		if e == nil || strings.Contains(e.Error(), "private") || !reflect.DeepEqual(b, FineHistoryBatch{}) {
			t.Fatal("port/close failure leaked or returned facts", e)
		}
	}
}

func TestFineHistoryInvalidManifestRefusesBeforeOpeningAnyStream(t *testing.T) {
	for name, change := range map[string]func(*historyFixture){
		"manifest bytes":       func(f *historyFixture) { f.raw = []byte(strings.Repeat("x", (1<<20)+1)) },
		"input bytes zero":     func(f *historyFixture) { f.identity.Bytes = 0 },
		"input bytes oversize": func(f *historyFixture) { f.identity.Bytes = MaxBytes + 1 },
		"header missing":       func(f *historyFixture) { f.raw = []byte(strings.Replace(string(f.raw), `"header_record":7,`, "", 1)) },
		"metadata negative": func(f *historyFixture) {
			f.raw = []byte(strings.Replace(string(f.raw), `"metadata_records":1`, `"metadata_records":-1`, 1))
		},
		"artifact aggregate": func(f *historyFixture) {
			var m map[string]any
			_ = json.Unmarshal(f.raw, &m)
			for _, o := range m["outputs"].([]any) {
				o.(map[string]any)["bytes"] = 55 << 20
			}
			f.raw, _ = json.Marshal(m)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := historySample(t)
			change(&f)
			calls := 0
			b, e := LoadFineHistory(context.Background(), f.raw, func(string) (io.ReadCloser, error) { calls++; return nil, errors.New("private") }, f.identity, "2020")
			if e == nil || calls != 0 || !reflect.DeepEqual(b, FineHistoryBatch{}) {
				t.Fatal("invalid manifest opened ports or returned facts", calls, e)
			}
		})
	}
}

type historyTrackingClose struct {
	io.Reader
	closed *bool
}

func (c historyTrackingClose) Close() error { *c.closed = true; return nil }
func TestFineHistoryOpenErrorStillClosesReturnedOwnedStream(t *testing.T) {
	f := historySample(t)
	closed := false
	b, e := LoadFineHistory(context.Background(), f.raw, func(string) (io.ReadCloser, error) {
		return historyTrackingClose{Reader: strings.NewReader(""), closed: &closed}, errors.New("private stream failure")
	}, f.identity, "2020")
	if e == nil || !closed || strings.Contains(e.Error(), "private") || !reflect.DeepEqual(b, FineHistoryBatch{}) {
		t.Fatal("failed open leaked its owned stream or facts", closed, e)
	}
}

type historyCancelClose struct {
	io.Reader
	cancel context.CancelFunc
	closed *bool
}

func (c historyCancelClose) Close() error { *c.closed = true; c.cancel(); return nil }

type historyReadFailure struct{}

func (historyReadFailure) Read([]byte) (int, error) { return 0, errors.New("private read failure") }
func (historyReadFailure) Close() error             { return nil }
func TestFineHistoryLateCancellationReadAndCloseFailuresReturnNoFacts(t *testing.T) {
	f := historySample(t)
	for _, mode := range []string{"late close failure", "late read failure", "late cancellation"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			closed := false
			open := func(name string) (io.ReadCloser, error) {
				if name != "source_notes.jsonl" {
					return f.open(name)
				}
				switch mode {
				case "late close failure":
					return badFineClose{Reader: strings.NewReader(f.files[name])}, nil
				case "late read failure":
					return historyReadFailure{}, nil
				default:
					return historyCancelClose{Reader: strings.NewReader(f.files[name]), cancel: cancel, closed: &closed}, nil
				}
			}
			b, e := LoadFineHistory(ctx, f.raw, open, f.identity, "2020")
			if e == nil || strings.Contains(e.Error(), "private") || !reflect.DeepEqual(b, FineHistoryBatch{}) {
				t.Fatal("late failure returned facts or raw error", e)
			}
			if mode == "late cancellation" && !closed {
				t.Fatal("cancelled stream was not closed")
			}
		})
	}
}
