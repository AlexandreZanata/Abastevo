//go:build integration

package adapters

// Maintained regression for orphaned begin recovery: a run row committed
// without its revision (interruption between the two inserts) is adopted by
// the next identical begin instead of deadlocking on a wait. No source
// bytes or PII involved.

import (
	"context"
	"testing"
	"time"

	official "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/official"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
)

func TestESPFollowupOrphanedBeginCanRecover(t *testing.T) {
	importer, pool := freshImporter(t)
	key := domain.ImportKey{SourceURL: "https://www.gov.br/anp/review-crash.xlsx", SourceChecksum: "sha256:review-synthetic", ParserVersion: "review-orphan"}
	id, err := newUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	uid, err := mustUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	// This is the durable state after CreateImportRun commits and the
	// process stops before CreateRevision. No source bytes or PII involved.
	_, err = official.New(pool).CreateImportRun(context.Background(), official.CreateImportRunParams{ID: uid, SourceUrl: key.SourceURL, SourceChecksum: key.SourceChecksum, ParserVersion: key.ParserVersion})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, err := importer.BeginRun(ctx, key, weekStart, weekEnd)
		cancel()
		if err != nil {
			t.Errorf("attempt %d: orphaned running import did not recover: %v", attempt+1, err)
			continue
		}
		if result.NoOp || result.RevisionID == "" {
			t.Errorf("attempt %d: orphaned import has no resumable revision", attempt+1)
		}
	}
	var revisions int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM official_revisions WHERE import_run_id=$1", uid).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 1 {
		t.Errorf("orphan recovery revisions=%d, want one", revisions)
	}
}
