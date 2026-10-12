package application

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// This injected-clock test covers post-deadline recovery, not within-deadline
// compliance. At first receipt +25h it deletes/marks all scripted copies.
// Actual elapsed-time acceptance requires the separate autonomous soak.

func TestSweepEveryCopyRecoversAfterDeadline(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	old := now.Add(-25 * time.Hour)
	store := &fakeSweepStore{
		idle: []domain.SessionRef{
			{ID: "s-idle", QuarantineKey: "q/idle", Status: domain.StateIssued, CreatedAt: old},
		},
		cands: []domain.SessionRef{
			{ID: "s-terminal", QuarantineKey: "q/terminal", Status: domain.StateReady, CreatedAt: old},
		},
		finals: []domain.ObjectRef{
			{ID: "o-stale", FinalKey: "f/stale", CreatedAt: old, ReceivedAt: old},
		},
		purged: 1,
	}
	deps := sweepDeps(store)
	deps.Clock = func() time.Time { return now }
	deps.Storage = &fakeSweepStorage{
		keys: map[string]bool{"q/idle": true, "q/terminal": true, "f/stale": true},
		fail: map[string]bool{},
	}
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ExpiredSessions != 1 {
		t.Errorf("expired sessions = %d, want 1 (idle original)", rep.ExpiredSessions)
	}
	if rep.QuarantinesDeleted != 2 {
		t.Errorf("quarantines deleted = %d, want idle + terminal originals", rep.QuarantinesDeleted)
	}
	if rep.FinalsDeleted != 1 || rep.ObjectsPurged != 1 {
		t.Errorf("finals/purged = %d/%d, want 1/1", rep.FinalsDeleted, rep.ObjectsPurged)
	}
	if rep.StorageErrors != 0 {
		t.Errorf("storage errors = %d, want zero", rep.StorageErrors)
	}
	if rep.Overdue.Unhealthy() {
		t.Errorf("post-pass signal = %+v, want quiet zero", rep.Overdue)
	}
	if len(store.qdel) != 2 || len(store.fdel) != 1 {
		t.Errorf("markers q=%v f=%v", store.qdel, store.fdel)
	}
}
