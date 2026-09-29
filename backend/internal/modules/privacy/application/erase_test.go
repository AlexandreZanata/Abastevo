package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

type fakeEraseStore struct {
	*fakeExportStore
	ledger  map[string]domain.LedgerEntry
	replays map[string]time.Time
	purged  int64
}

func newFakeEraseStore() *fakeEraseStore {
	return &fakeEraseStore{
		fakeExportStore: newFakeExportStore(),
		ledger:          map[string]domain.LedgerEntry{},
		replays:         map[string]time.Time{},
	}
}

func (f *fakeEraseStore) RequestDeletion(_ context.Context, r domain.Request, _ string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error) {
	id, replayed, err := f.RequestExport(context.Background(), r, enqueue)
	return id, replayed, err
}

func (f *fakeEraseStore) RecordLedger(_ context.Context, e domain.LedgerEntry) (string, bool, error) {
	for _, existing := range f.ledger {
		if existing.ContributorID == e.ContributorID && existing.Scope == e.Scope {
			return existing.ID, true, nil
		}
	}
	f.ledger[e.ID] = e
	return e.ID, false, nil
}

func (f *fakeEraseStore) ListLedger(_ context.Context, contributorID string) ([]domain.LedgerEntry, error) {
	var out []domain.LedgerEntry
	for _, e := range f.ledger {
		if e.ContributorID == contributorID {
			if t, ok := f.replays[e.ID]; ok {
				e.ReplayedAt = t
			}
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeEraseStore) MarkLedgerReplayed(_ context.Context, id string, at time.Time) error {
	if _, ok := f.ledger[id]; !ok {
		return domain.ErrNotFound
	}
	f.replays[id] = at
	return nil
}

func (f *fakeEraseStore) PurgeOwnerArchives(_ context.Context, contributorID string) (int64, error) {
	var n int64
	for id, r := range f.requests {
		if r.ContributorID == contributorID && len(f.archives[id]) > 0 {
			delete(f.archives, id)
			n++
		}
	}
	f.purged += n
	return n, nil
}

func (f *fakeEraseStore) CompleteDeletionRequest(_ context.Context, id string, at time.Time) error {
	r, ok := f.requests[id]
	if !ok || r.Status != domain.StatusRequested || r.Type != domain.TypeDeletion {
		return domain.ErrBadState
	}
	r.Status = domain.StatusReady
	r.ReadyAt = at
	r.CompletedAt = at
	f.requests[id] = r
	return nil
}

type fakeIdentity struct{ deleted map[string]bool }

func (f *fakeIdentity) DeleteContributor(_ context.Context, id string) (bool, int64, error) {
	if f.deleted == nil {
		f.deleted = map[string]bool{}
	}
	if f.deleted[id] {
		return false, 0, nil
	}
	f.deleted[id] = true
	return true, 2, nil
}

type fakeCommunity struct {
	unlinked map[string]string
	jobs     []string
	fail     bool
}

func (f *fakeCommunity) EraseContributor(_ context.Context, ref, anon string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (int64, int64, int, error) {
	if f.fail {
		return 0, 0, 0, errors.New("community down")
	}
	if f.unlinked == nil {
		f.unlinked = map[string]string{}
	}
	if _, ok := f.unlinked[ref]; ok {
		return 0, 0, 0, nil
	}
	f.unlinked[ref] = anon
	if enqueue != nil {
		_ = enqueue(context.Background(), nil, "community-consensus", []byte(`{}`), "dedupe")
		f.jobs = append(f.jobs, "community-consensus")
	}
	return 3, 1, 2, nil
}

type fakeEvidence struct {
	purged  map[string]bool
	deleted [][2]string
}

func (f *fakeEvidence) EraseContributor(_ context.Context, ref string, deleteKey func(ctx context.Context, key, namespace string) error) (int64, int64, error) {
	if f.purged == nil {
		f.purged = map[string]bool{}
	}
	if f.purged[ref] {
		return 0, 0, nil
	}
	f.purged[ref] = true
	if deleteKey != nil {
		_ = deleteKey(context.Background(), "q/aaa", "q/")
		f.deleted = append(f.deleted, [2]string{"q/aaa", "q/"})
	}
	return 1, 1, nil
}

type fakeTrust struct{ erased map[string]bool }

func (f *fakeTrust) EraseContributor(_ context.Context, ref, _ string) (int64, error) {
	if f.erased == nil {
		f.erased = map[string]bool{}
	}
	if f.erased[ref] {
		return 0, nil
	}
	f.erased[ref] = true
	return 2, nil
}

func erasePorts(store *fakeEraseStore) ErasePorts {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	n := 100
	return ErasePorts{
		Clock: func() time.Time { return base },
		NewID: func() (string, error) {
			n++
			return "d0000000-0000-4000-8000-00000000010" + string(rune('0'+n%10)), nil
		},
		Attribution: func(_ context.Context, contributorID string) (string, error) {
			return "tok-" + contributorID, nil
		},
		Store:     store,
		Identity:  &fakeIdentity{},
		Community: &fakeCommunity{},
		Evidence:  &fakeEvidence{},
		Trust:     &fakeTrust{},
		DeleteObject: func(context.Context, string, string) error {
			return nil
		},
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error {
			return nil
		},
	}
}

func TestEraseRunsAllScopesWithLedger(t *testing.T) {
	store := newFakeEraseStore()
	report, err := Erase(context.Background(), erasePorts(store), "c1", EraseDTO{ClientSubmissionID: "erase-1", Reason: "owner request"})
	if err != nil {
		t.Fatalf("erase = %v", err)
	}
	if !report.IdentityRevoked || report.KeysRevoked != 2 {
		t.Errorf("identity = %+v", report)
	}
	if report.ObsUnlinked != 3 || report.VotesUnlinked != 1 || report.KeysRecomputed != 2 {
		t.Errorf("community = %+v", report)
	}
	if report.SessionsPurged != 1 || report.ObjectsPurged != 1 {
		t.Errorf("evidence = %+v", report)
	}
	if report.TrustUnlinked != 2 {
		t.Errorf("trust = %+v", report)
	}
	if len(store.ledger) != 5 {
		t.Errorf("ledger rows = %d, want 5", len(store.ledger))
	}
	got, err := store.Get(context.Background(), report.RequestID)
	if err != nil || got.Status != domain.StatusReady || got.Type != domain.TypeDeletion {
		t.Errorf("request = %+v, %v", got, err)
	}
}

func TestEraseConvergesCompletedRequest(t *testing.T) {
	store := newFakeEraseStore()
	p := erasePorts(store)
	first, err := Erase(context.Background(), p, "c1", EraseDTO{ClientSubmissionID: "erase-1", Reason: "owner request"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Erase(context.Background(), p, "c1", EraseDTO{ClientSubmissionID: "erase-1", Reason: "owner request"})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.RequestID != first.RequestID {
		t.Errorf("completed erasure did not converge: %+v", second)
	}
	if len(store.ledger) != 5 {
		t.Errorf("replay duplicated ledger rows: %d", len(store.ledger))
	}
}

func TestEraseFailsScopeMarksRequest(t *testing.T) {
	store := newFakeEraseStore()
	p := erasePorts(store)
	p.Community = &fakeCommunity{fail: true}
	if _, err := Erase(context.Background(), p, "c1", EraseDTO{ClientSubmissionID: "erase-1", Reason: "r"}); err == nil {
		t.Error("scope failure accepted")
	}
	var failed bool
	for _, r := range store.requests {
		if r.Status == domain.StatusFailed {
			failed = true
		}
	}
	if !failed {
		t.Error("failed erasure left no FAILED receipt")
	}
}

func TestEraseRejectsAnonymous(t *testing.T) {
	store := newFakeEraseStore()
	if _, err := Erase(context.Background(), erasePorts(store), "", EraseDTO{ClientSubmissionID: "x", Reason: "r"}); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anonymous = %v", err)
	}
}

func TestReplayLedgerReappliesScopes(t *testing.T) {
	store := newFakeEraseStore()
	p := erasePorts(store)
	if _, err := Erase(context.Background(), p, "c1", EraseDTO{ClientSubmissionID: "erase-1", Reason: "owner request"}); err != nil {
		t.Fatal(err)
	}
	// Simulate a stale restore: fresh scope states behind the same refs.
	p2 := erasePorts(store)
	report, err := ReplayLedger(context.Background(), p2, "c1")
	if err != nil {
		t.Fatalf("replay = %v", err)
	}
	_ = report
	entries, err := store.ListLedger(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.ReplayedAt.IsZero() {
			t.Errorf("scope %s not stamped", e.Scope)
		}
	}
	// Second replay converges without work.
	if _, err := ReplayLedger(context.Background(), erasePorts(store), "c1"); err != nil {
		t.Errorf("second replay = %v", err)
	}
}

func TestReplayLedgerUnknownContributor(t *testing.T) {
	store := newFakeEraseStore()
	if _, err := ReplayLedger(context.Background(), erasePorts(store), "ghost"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown replay = %v", err)
	}
}
