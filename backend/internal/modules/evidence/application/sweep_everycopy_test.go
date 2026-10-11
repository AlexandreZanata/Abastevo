package application

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// SEC05-E every-copy deadline compliance in compressed time (AUD-06,
// B-BR-M01/M04): one sweep at now = first-receipt + 25 h deletes and
// marks every app-owned copy — idle quarantine original, terminal
// quarantine original, stale sanitized final and aged-out object row
// — with zero storage errors and a quiet post-pass signal. The wall
// clock observation this compresses rides the seeded real soak
// (SOAK_RUNNING, proof due after the wall clock).

func TestSweepEveryCopyMeetsDeadline(t *testing.T) {
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
