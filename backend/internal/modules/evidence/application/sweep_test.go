package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// fakeSweepStore scripts inventory per method and records every marker,
// so repeated runs prove convergence instead of double work.
type fakeSweepStore struct {
	idle       []domain.SessionRef
	stuck      []domain.SessionRef
	cands      []domain.SessionRef
	finals     []domain.ObjectRef
	expired    []string
	qdel       []string
	fdel       []string
	purged     int64
	failExpire error
}

func (f *fakeSweepStore) ExpireIdleSessions(_ context.Context, _ time.Time, _ int) ([]domain.SessionRef, error) {
	if f.failExpire != nil {
		return nil, f.failExpire
	}
	out := f.idle
	f.idle = nil
	return out, nil
}

func (f *fakeSweepStore) ListStuckVerifying(_ context.Context, _ time.Time, _ int) ([]domain.SessionRef, error) {
	out := f.stuck
	f.stuck = nil
	return out, nil
}

func (f *fakeSweepStore) ExpireStuckSession(_ context.Context, id string, _ time.Time) error {
	f.expired = append(f.expired, id)
	return nil
}

func (f *fakeSweepStore) MarkQuarantineDeleted(_ context.Context, id string) error {
	f.qdel = append(f.qdel, id)
	return nil
}

func (f *fakeSweepStore) QuarantineCandidates(_ context.Context, _ time.Time, _ int) ([]domain.SessionRef, error) {
	out := f.cands
	f.cands = nil
	return out, nil
}

func (f *fakeSweepStore) StaleFinalCandidates(_ context.Context, _ time.Time, _ int) ([]domain.ObjectRef, error) {
	out := f.finals
	f.finals = nil
	return out, nil
}

func (f *fakeSweepStore) MarkFinalDeleted(_ context.Context, id string) error {
	f.fdel = append(f.fdel, id)
	return nil
}

func (f *fakeSweepStore) PurgeObjectsBefore(_ context.Context, _ time.Time) (int64, error) {
	return f.purged, nil
}

type fakeSweepJobs struct {
	live map[string]bool
}

func (f *fakeSweepJobs) HasLiveJob(_ context.Context, sessionID string) (bool, error) {
	return f.live[sessionID], nil
}

type fakeSweepStorage struct {
	keys map[string]bool
	fail map[string]bool
}

func (f *fakeSweepStorage) DeleteQuarantine(_ context.Context, key string) error {
	if f.fail[key] {
		return errors.New("storage down")
	}
	if !f.keys[key] {
		return errors.New("storage: missing key")
	}
	delete(f.keys, key)
	return nil
}

func (f *fakeSweepStorage) DeleteFinal(_ context.Context, key string) error {
	return f.DeleteQuarantine(context.Background(), key)
}

func sweepDeps(store *fakeSweepStore) SweepDeps {
	return SweepDeps{
		Clock: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
		Batch: 100,
		Store: store,
		Jobs:  &fakeSweepJobs{live: map[string]bool{}},
		Storage: &fakeSweepStorage{
			keys: map[string]bool{"q/a": true, "q/b": true, "q/c": true, "f/old": true},
			fail: map[string]bool{},
		},
	}
}

func ref(id, key string) domain.SessionRef {
	return domain.SessionRef{ID: id, QuarantineKey: key, Status: domain.StateIssued, CreatedAt: time.Now()}
}

func TestSweepExpiresIdleAndDeletesQuarantine(t *testing.T) {
	store := &fakeSweepStore{idle: []domain.SessionRef{ref("s1", "q/a"), ref("s2", "q/b")}}
	deps := sweepDeps(store)
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatalf("sweep = %v", err)
	}
	if rep.ExpiredSessions != 2 || rep.QuarantinesDeleted != 2 {
		t.Errorf("report = %+v", rep)
	}
	if len(store.expired) != 0 {
		t.Errorf("idle path must not use stuck expiry: %v", store.expired)
	}
	// Repeated runs converge: inventory is empty, nothing re-deletes.
	rep, err = Sweep(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ExpiredSessions+rep.QuarantinesDeleted+rep.FinalsDeleted != 0 || rep.ObjectsPurged != 0 {
		t.Errorf("repeat report = %+v, want zero work", rep)
	}
}

func TestSweepSkipsActiveLease(t *testing.T) {
	// A VERIFYING session with a live verify job is untouchable: no
	// expiry, no quarantine delete, counted as skipped.
	store := &fakeSweepStore{stuck: []domain.SessionRef{ref("s9", "q/c")}}
	deps := sweepDeps(store)
	deps.Jobs = &fakeSweepJobs{live: map[string]bool{"s9": true}}
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatalf("sweep = %v", err)
	}
	if rep.SkippedActive != 1 || rep.ExpiredSessions != 0 || rep.QuarantinesDeleted != 0 {
		t.Errorf("report = %+v", rep)
	}
	if len(store.expired) != 0 || len(store.qdel) != 0 {
		t.Errorf("active lease mutated: %v %v", store.expired, store.qdel)
	}
}

func TestSweepExpiresStuckWithoutJob(t *testing.T) {
	store := &fakeSweepStore{stuck: []domain.SessionRef{ref("s9", "q/c")}}
	deps := sweepDeps(store)
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatalf("sweep = %v", err)
	}
	if rep.ExpiredSessions != 1 || rep.QuarantinesDeleted != 1 {
		t.Errorf("report = %+v", rep)
	}
	if len(store.expired) != 1 || store.expired[0] != "s9" {
		t.Errorf("expired = %v", store.expired)
	}
}

func TestSweepDeletesTerminalQuarantines(t *testing.T) {
	store := &fakeSweepStore{cands: []domain.SessionRef{
		{ID: "r1", QuarantineKey: "q/a", Status: domain.StateReady},
		{ID: "r2", QuarantineKey: "q/b", Status: domain.StateRejected},
	}}
	deps := sweepDeps(store)
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatalf("sweep = %v", err)
	}
	if rep.QuarantinesDeleted != 2 || len(store.qdel) != 2 {
		t.Errorf("report = %+v qdel = %v", rep, store.qdel)
	}
}

func TestSweepContinuesPastStorageErrors(t *testing.T) {
	// A failing object delete is counted and retried next run; the rest
	// of the sweep still completes in this one.
	store := &fakeSweepStore{cands: []domain.SessionRef{
		{ID: "r1", QuarantineKey: "q/a", Status: domain.StateReady},
		{ID: "r2", QuarantineKey: "q/b", Status: domain.StateReady},
	}}
	deps := sweepDeps(store)
	deps.Storage.(*fakeSweepStorage).fail["q/a"] = true
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatalf("sweep = %v", err)
	}
	if rep.QuarantinesDeleted != 1 || rep.StorageErrors != 1 {
		t.Errorf("report = %+v", rep)
	}
	if len(store.qdel) != 1 || store.qdel[0] != "r2" {
		t.Errorf("qdel = %v", store.qdel)
	}
}

func TestFinalDuePolicy(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	old := func() domain.ObjectRef {
		return domain.ObjectRef{ID: "o", FinalKey: "f/old", CreatedAt: now.Add(-15 * 24 * time.Hour)}
	}
	if !FinalDue(old(), now) {
		t.Error("15-day default not due")
	}
	young := old()
	young.CreatedAt = now.Add(-13 * 24 * time.Hour)
	if FinalDue(young, now) {
		t.Error("13-day default due early")
	}
	extended := old()
	extended.HasExtension = true
	extended.ExtendedUntil = now.Add(5 * 24 * time.Hour)
	if FinalDue(extended, now) {
		t.Error("reviewed extension ignored")
	}
	// The 30-day cap clamps runaway extensions: 31 days old with a
	// +60-day extension is due (clamped to yesterday), while an
	// unclamped reading would keep it forever.
	overlong := old()
	overlong.CreatedAt = now.Add(-31 * 24 * time.Hour)
	overlong.HasExtension = true
	overlong.ExtendedUntil = now.Add(60 * 24 * time.Hour)
	if !FinalDue(overlong, now) {
		t.Error("30-day absolute cap not enforced")
	}
}

func TestSweepDeletesStaleFinalsAndPurgesHashes(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := &fakeSweepStore{
		finals: []domain.ObjectRef{
			{ID: "o1", FinalKey: "f/old", CreatedAt: now.Add(-15 * 24 * time.Hour)},
		},
		purged: 3,
	}
	deps := sweepDeps(store)
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatalf("sweep = %v", err)
	}
	if rep.FinalsDeleted != 1 || len(store.fdel) != 1 || store.fdel[0] != "o1" {
		t.Errorf("report = %+v fdel = %v", rep, store.fdel)
	}
	if rep.ObjectsPurged != 3 {
		t.Errorf("purged = %+v", rep)
	}
}

func TestSweepFailsFastOnDBError(t *testing.T) {
	store := &fakeSweepStore{failExpire: errors.New("db down")}
	if _, err := Sweep(context.Background(), sweepDeps(store)); err == nil {
		t.Error("db failure accepted")
	}
}

func TestSweepRejectsBadBatch(t *testing.T) {
	deps := sweepDeps(&fakeSweepStore{})
	deps.Batch = 0
	if _, err := Sweep(context.Background(), deps); err == nil {
		t.Error("zero batch accepted")
	}
}
