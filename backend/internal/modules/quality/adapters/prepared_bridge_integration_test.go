//go:build integration

package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func TestPreparedRustBridgePublishesAtomicallyAndReplays(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_QUALITY_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_QUALITY_PREPARER_PATH must name the trusted Rust quality CLI")
	}
	_, file, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(file), "../../../../../contracts/testdata/station-prep/quality-assay-sample.csv")
	raw, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	dir := filepath.Join(t.TempDir(), "prepared")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e = exec.CommandContext(ctx, binary, source, dir).Run(); e != nil {
		t.Fatal("trusted preparer failed", e)
	}
	manifest, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	batch, e := prepared.Load(ctx, manifest, func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(dir, name)) }, prepared.InputIdentity{Bytes: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])}, func(_ context.Context, r domain.SourceRecord) (domain.Decision, error) {
		if r.CNPJ != "48151623000191" {
			return domain.Decision{Kind: domain.ResolutionUnmatched, Reason: domain.ReasonUnmatchedID}, nil
		}
		return domain.Decision{Kind: domain.ResolutionBound, Reason: "exact cnpj", Link: domain.StationLink{RecordKey: r.RecordKey, StationID: reportStation, Automatic: true, Active: true}}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	p := freshPool(t)
	if _, e = p.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic bridge station')", reportStation); e != nil {
		t.Fatal(e)
	}
	input := application.Input{Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09", Resources: []application.Resource{{URL: "https://www.gov.br/anp/synthetic-quality.csv", SHA256: hex.EncodeToString(sum[:]), Bytes: uint64(len(raw))}}, Counts: batch.Counts, ManifestHash: batch.ManifestHash, Assays: batch.Assays, Links: batch.Links}
	publisher := NewPublisher(p)
	maker := &idMaker{}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := publisher.Publish(ctx, input, maker.next)
			if e != nil {
				t.Error(e)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	revision := ""
	for id := range ids {
		if revision != "" && id != revision {
			t.Fatal("prepared replay fork")
		}
		revision = id
	}
	if revision == "" {
		t.Fatal("no prepared publication")
	}
	reader := qualityread.NewReader(p)
	rows, _, lease, e := reader.Assays(ctx, time.Now(), "pmqc", "pmqc-assay-v1", reportStation, "", "", "", 100)
	if e != nil || len(rows) != 4 || lease.Counts.Input != 12 || lease.Counts.Accepted != 7 || lease.ManifestHash != batch.ManifestHash || lease.RevisionID != revision {
		t.Fatal("published bridge mismatch", len(rows), lease.Counts, e)
	}
	for _, row := range rows {
		if len(row.SampleRef) != 66 {
			t.Fatal("sample scope lost")
		}
	}
	var revisions, stations int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_revisions").Scan(&revisions); e != nil {
		t.Fatal(e)
	}
	if e = p.QueryRow(ctx, "SELECT count(*) FROM directory_stations").Scan(&stations); e != nil {
		t.Fatal(e)
	}
	if revisions != 1 || stations != 1 {
		t.Fatal("bridge created extra identity/publication", revisions, stations)
	}
}
