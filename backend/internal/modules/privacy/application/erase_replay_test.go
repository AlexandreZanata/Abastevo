package application

import (
	"context"
	"testing"
)

// SEC04-D lost responses and partial restores (AUD-05, B-BR-016): a
// retried intent converges on the recorded request; a restored ledger
// holding only some scopes replays exactly those scopes and stamps
// them; worker failure in one scope marks the request without
// touching the others (covered in erase_test.go).

func TestLostResponseConvergesOnRecordedIntent(t *testing.T) {
	store := newFakeEraseStore()
	p := erasePorts(store)
	dto := EraseDTO{ClientSubmissionID: "erase-lost-1", Reason: "owner request"}
	first, err := RequestDeletion(context.Background(), p, "c-lost", dto)
	if err != nil || first.Replayed {
		t.Fatalf("first intent = %+v, %v", first, err)
	}
	// The response never arrived: the identical retry converges on the
	// recorded request instead of minting a second job.
	second, err := RequestDeletion(context.Background(), p, "c-lost", dto)
	if err != nil || !second.Replayed || second.RequestID != first.RequestID {
		t.Fatalf("retry = %+v, %v, want replay of %q", second, err, first.RequestID)
	}
}

func TestPartialRestoreReplaysRecordedScopesOnly(t *testing.T) {
	store := newFakeEraseStore()
	p := erasePorts(store)
	dto := EraseDTO{ClientSubmissionID: "erase-partial-1", Reason: "owner request"}
	if _, err := RequestDeletion(context.Background(), p, "c-partial", dto); err != nil {
		t.Fatal(err)
	}
	if _, err := Erase(context.Background(), p, "c-partial", dto); err != nil {
		t.Fatalf("erase: %v", err)
	}
	// Simulate a partial restore: only the identity and community
	// ledger rows survived. Replay runs exactly those scopes and
	// stamps them; missing scopes are not invented.
	for id, e := range store.ledger {
		if e.Scope != "IDENTITY" && e.Scope != "COMMUNITY" {
			delete(store.ledger, id)
		}
	}
	report, err := ReplayLedger(context.Background(), erasePorts(store), "c-partial")
	if err != nil {
		t.Fatalf("partial replay: %v", err)
	}
	if !report.IdentityRevoked || report.ObsUnlinked == 0 {
		t.Errorf("partial report = %+v, want recorded scopes applied", report)
	}
	if report.SessionsPurged != 0 || report.TrainingRevoked != 0 || report.TrustUnlinked != 0 {
		t.Errorf("partial report = %+v, want unrestored scopes untouched", report)
	}
	entries, err := store.ListLedger(context.Background(), "c-partial")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.ReplayedAt.IsZero() {
			t.Errorf("scope %q unstamped after replay", e.Scope)
		}
	}
}
