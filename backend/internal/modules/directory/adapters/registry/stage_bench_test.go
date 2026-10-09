//go:build integration

package registry

// RST-07 parse benchmark: Go StageCSV over the deterministic normalized
// snapshot shared with the Rust bench (same seed, same identity/duplicate
// sequence), asserting identical accounting and logging the accepted CNPJ
// multiset hash for cross-implementation comparison. Runs only with
// -bench; the file comes from tools/station-prep bench_generate:
//
//	./target/release/bench_generate 100000 7 normalized > /tmp/bench-100k-norm.csv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func BenchmarkStageCSV100k(b *testing.B) {
	path := os.Getenv("STATION_PREP_BENCH_CSV")
	if path == "" {
		path = "/tmp/bench-100k-norm.csv"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Skipf("bench snapshot missing (generate it first): %v", err)
	}
	for range make([]struct{}, b.N) {
		store := newFakeStore()
		limits := Limits{MaxBytes: 1 << 28, MaxRows: 2000000, BatchSize: 500}
		started := time.Now()
		report, err := StageCSV(context.Background(), store, "bench-100k", strings.NewReader(string(raw)), limits)
		if err != nil {
			b.Fatalf("stage: %v", err)
		}
		elapsed := time.Since(started)
		// Identical semantics with station-prep on the same seed: the
		// raw layout parses to 98004/1996/0, so the normalized twin must
		// stage exactly that accounting.
		if report.Accepted != 98004 || report.Duplicates != 1996 || report.Rejected != 0 {
			b.Fatalf("accounting = %+v, want 98004/1996/0", report)
		}
		keys := make([]string, 0, len(store.assertions))
		for _, assertion := range store.assertions {
			keys = append(keys, assertion.SourceKey)
		}
		sort.Strings(keys)
		// Same framing as station-prep bench_parse: key plus newline per
		// entry, so the digests are directly comparable.
		digest := sha256.New()
		for _, key := range keys {
			digest.Write([]byte(key))
			digest.Write([]byte("\n"))
		}
		sum := digest.Sum(nil)
		var mem runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&mem)
		b.ReportMetric(float64(len(raw))/elapsed.Seconds(), "bytes_per_sec")
		b.ReportMetric(float64(mem.HeapAlloc)/1048576, "heap_mib")
		b.Logf("counts=%d/%d/%d rows_per_sec=%.0f cnpjset=%s heap_mib=%.1f",
			report.Accepted, report.Duplicates, report.Rejected,
			float64(report.Accepted+report.Duplicates+report.Rejected)/elapsed.Seconds(),
			hex.EncodeToString(sum[:]), float64(mem.HeapAlloc)/1048576)
	}
}
