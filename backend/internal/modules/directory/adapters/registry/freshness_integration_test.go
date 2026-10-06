//go:build integration

package registry

import (
	"context"
	"strings"
	"testing"
)

func TestLastCompleteRunTracksFreshness(t *testing.T) {
	store := freshStore(t)
	ctx := context.Background()
	limits := Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10}

	if _, err := StageCSV(ctx, store, "fresh-1", strings.NewReader(cleanCSV), limits); err != nil {
		t.Fatalf("stage: %v", err)
	}
	last, err := store.Q.LastCompleteRegistryRun(ctx, SourceCSV)
	if err != nil {
		t.Fatalf("last complete: %v", err)
	}
	if last.SnapshotIdentity != "fresh-1" || last.State != "complete" || last.Accepted != 3 {
		t.Fatalf("last = %+v", last)
	}
}
