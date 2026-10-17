//go:build integration

package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

const fileSourceURL = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_09.zip"

func fileJob(t *testing.T, raw []byte) platformjobs.Job {
	t.Helper()
	sum := sha256.Sum256(raw)
	payload, e := json.Marshal(ImportPayload{Version: 1, Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09", URL: fileSourceURL, SHA256: hex.EncodeToString(sum[:]), Member: "PMQC_2026_09.csv"})
	if e != nil {
		t.Fatal(e)
	}
	return platformjobs.Job{Payload: payload}
}
func TestFileImportIntegrityReplayAndCleanup(t *testing.T) {
	p := freshJobsDB(t)
	raw := []byte("synthetic pinned archive bytes")
	job := fileJob(t, raw)
	root := t.TempDir()
	fetches, prepares, guards, leases := 0, 0, 0, 0
	staleGuard := false
	cleanupFailure := false
	im := FileImport{Pool: p, Publish: adapters.NewPublisher(p), Serial: func(ctx context.Context, run func(context.Context) error) error { leases++; return run(ctx) }, Fetch: func(context.Context, string) (FileArtifact, error) {
		fetches++
		filename := filepath.Join(root, "input.zip")
		if e := os.WriteFile(filename, raw, 0600); e != nil {
			return FileArtifact{}, e
		}
		sum := sha256.Sum256(raw)
		return FileArtifact{Path: filename, Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}, Close: func() error {
			e := os.Remove(filename)
			if e == nil && cleanupFailure {
				return errors.New("synthetic cleanup failure")
			}
			return e
		}}, nil
	}, Prepare: func(ctx context.Context, payload ImportPayload, file FileArtifact) (PreparedBatch, error) {
		prepares++
		b, e := fakePrepare(strings.Repeat("e", 64))(nil)
		b.MemberSHA256 = strings.Repeat("a", 64)
		b.MemberBytes = 10
		b.Guard = func(context.Context, pgx.Tx) error {
			guards++
			if staleGuard {
				return errors.New("synthetic source binding changed")
			}
			return nil
		}
		return b, e
	}}
	for i := 0; i < 2; i++ {
		if e := im.Handle(context.Background(), job); e != nil {
			t.Fatal(e)
		}
	}
	if fetches != 2 || prepares != 2 || guards != 2 || leases != 2 {
		t.Fatal("file replay skipped integrity/live guard", fetches, prepares, guards, leases)
	}
	staleGuard = true
	if e := im.Handle(context.Background(), job); e == nil {
		t.Fatal("stale file replay bypassed live guard")
	}
	staleGuard = false
	cleanupFailure = true
	if e := im.Handle(context.Background(), job); e == nil || strings.Contains(e.Error(), "synthetic") {
		t.Fatal("cleanup failure leaked/acknowledged", e)
	}
	cleanupFailure = false
	if e := im.Handle(context.Background(), job); e != nil {
		t.Fatal("cleanup-failure retry did not converge", e)
	}
	raw = []byte("changed remote archive")
	if e := im.Handle(context.Background(), job); e == nil {
		t.Fatal("changed source resurrected old payload")
	}

	var n int
	if e := p.QueryRow(context.Background(), "SELECT count(*) FROM quality_revisions").Scan(&n); e != nil || n != 1 {
		t.Fatal("file replay forked", n, e)
	}
	mark, e := quality.New(p).GetWatermark(context.Background(), quality.GetWatermarkParams{Source: "pmqc", Stream: "pmqc-assay-v1"})
	if e != nil || mark.LastEdition != "2026-09" {
		t.Fatal(mark, e)
	}
	children, e := os.ReadDir(root)
	if e != nil || len(children) != 0 {
		t.Fatal("file cleanup", children, e)
	}
}
