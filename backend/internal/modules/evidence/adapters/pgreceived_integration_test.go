//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"
)

// TestPGReceivedAtAnchorsDeadline proves the forward stamp persists
// and drives selection: an object received 25 h ago is stale and
// overdue at a now-minus-24 h cutoff, while a fresh one is neither.
func TestPGReceivedAtAnchorsDeadline(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	now := time.Now()

	oldSess := testSession("e0000000-0000-4000-8000-000000000011", "upl-old")
	if _, _, err := s.ReserveSession(ctx, oldSess); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, oldSess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	oldObj := testObject(oldSess.ID)
	oldObj.ReceivedAt = now.Add(-25 * time.Hour)
	if _, err := s.RecordVerified(ctx, oldSess.ID, oldObj); err != nil {
		t.Fatalf("record old: %v", err)
	}

	freshSess := testSession("e0000000-0000-4000-8000-000000000012", "upl-fresh")
	if _, _, err := s.ReserveSession(ctx, freshSess); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteSession(ctx, freshSess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	freshObj := testObject(freshSess.ID)
	freshObj.ReceivedAt = now.Add(-time.Hour)
	if _, err := s.RecordVerified(ctx, freshSess.ID, freshObj); err != nil {
		t.Fatalf("record fresh: %v", err)
	}

	cutoff := now.Add(-24 * time.Hour)
	stale, err := s.StaleFinalCandidates(ctx, cutoff, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].ID != oldObj.ID {
		t.Fatalf("stale = %+v, want only the 25-hour object", stale)
	}
	if stale[0].ReceivedAt.IsZero() {
		t.Error("stale row must carry its received stamp")
	}

	overdue, err := s.OverdueFinals(ctx, cutoff, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(overdue) != 1 || overdue[0].ID != oldObj.ID {
		t.Fatalf("overdue = %+v, want only the 25-hour object", overdue)
	}

	// Restore-replay convergence: deleting the bytes converges, and
	// both selections go quiet on repeat.
	if err := s.MarkFinalDeleted(ctx, oldObj.ID); err != nil {
		t.Fatal(err)
	}
	stale, err = s.StaleFinalCandidates(ctx, cutoff, 10)
	if err != nil || len(stale) != 0 {
		t.Fatalf("repeat stale = %+v, %v", stale, err)
	}
	overdue, err = s.OverdueFinals(ctx, cutoff, 10)
	if err != nil || len(overdue) != 0 {
		t.Fatalf("repeat overdue = %+v, %v", overdue, err)
	}
}

// TestPGReceivedAtFallsBackToCreated proves the migration backfill
// path: a NULL stamp still selects by creation time, so pre-migration
// rows go overdue on the first sweep pass instead of living forever.
func TestPGReceivedAtFallsBackToCreated(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess := testSession("e0000000-0000-4000-8000-000000000013", "upl-backfill")
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, sess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	obj := testObject(sess.ID)
	if _, err := s.RecordVerified(ctx, sess.ID, obj); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_objects SET received_at = NULL WHERE id = $1`, obj.ID); err != nil {
		t.Fatal(err)
	}
	// Backdate creation 25 h to simulate a pre-migration row.
	if _, err := pool.Exec(ctx, `UPDATE evidence_objects SET created_at = now() - interval '25 hours' WHERE id = $1`, obj.ID); err != nil {
		t.Fatal(err)
	}
	overdue, err := s.OverdueFinals(ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range overdue {
		if o.ID == obj.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("NULL-stamp row must fall back to created_at, overdue = %+v", overdue)
	}
}
