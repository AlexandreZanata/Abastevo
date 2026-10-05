package application

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type fakeProfileStore struct {
	mu        sync.Mutex
	profiles  map[string]StoredProfile
	operators map[string]StoredOperator
	closed    int
}

func (f *fakeProfileStore) EnsureUnclaimed(_ context.Context, stationID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.profiles == nil {
		f.profiles = map[string]StoredProfile{}
	}
	if _, ok := f.profiles[stationID]; !ok {
		f.profiles[stationID] = StoredProfile{PolicyVersion: "profile-v1", Revision: 1, Business: map[string]string{}}
	}
	return nil
}

func (f *fakeProfileStore) Profile(_ context.Context, stationID string) (StoredProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	profile, ok := f.profiles[stationID]
	if !ok {
		return StoredProfile{}, errors.New("no profile")
	}
	return profile, nil
}

func (f *fakeProfileStore) RecordOperator(_ context.Context, id, stationID, cnpj, source, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, op := range f.operators {
		_ = op
	}
	if f.operators == nil {
		f.operators = map[string]StoredOperator{}
	}
	f.operators[stationID] = StoredOperator{ID: id, CNPJ: cnpj, Source: source, Reference: ref}
	return nil
}

func (f *fakeProfileStore) CloseOperator(_ context.Context, id string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for station, op := range f.operators {
		if op.ID == id {
			delete(f.operators, station)
			f.closed++
			return 1, nil
		}
	}
	return 0, nil
}

func (f *fakeProfileStore) CurrentOperator(_ context.Context, stationID string) (StoredOperator, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.operators[stationID]
	return op, ok, nil
}

func testReader(display string) StationReader {
	lat, lon := -23.55, -46.63
	return func(context.Context, string) (string, string, *float64, *float64, error) {
		return display, "reviewed", &lat, &lon, nil
	}
}

func TestEnsureUnclaimedIsIdempotent(t *testing.T) {
	store := &fakeProfileStore{}
	if err := EnsureUnclaimed(context.Background(), store, "station-1"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := EnsureUnclaimed(context.Background(), store, "station-1"); err != nil {
		t.Fatalf("replay: %v", err)
	}
	profile, err := ReadProfile(context.Background(), store, testReader("Posto"), "station-1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if profile.HasBadge || len(profile.Business) != 0 || profile.OperatorCNPJ != "" {
		t.Fatalf("unclaimed must be empty and badgeless: %+v", profile)
	}
	if profile.PolicyVersion != "profile-v1" {
		t.Fatalf("policy = %q", profile.PolicyVersion)
	}
	if err := EnsureUnclaimed(context.Background(), store, ""); err == nil {
		t.Fatal("blank station must fail")
	}
}

func TestLinkOperatorRotatesAndConverges(t *testing.T) {
	store := &fakeProfileStore{}
	newID := func() string { return "op-1" }
	if err := LinkOperator(context.Background(), store, newID, "station-1", "04218406000104", "registry", "PRC-1"); err != nil {
		t.Fatalf("link: %v", err)
	}
	// Same CNPJ replays without a new revision.
	if err := LinkOperator(context.Background(), store, newID, "station-1", "04218406000104", "registry", "PRC-1"); err != nil {
		t.Fatalf("replay: %v", err)
	}
	// New CNPJ closes the old link and opens exactly one revision.
	if err := LinkOperator(context.Background(), store, func() string { return "op-2" }, "station-1", "00428184000195", "dou", "ANP-9"); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if store.closed != 1 {
		t.Fatalf("closed = %d, want 1", store.closed)
	}
	current, found, err := store.CurrentOperator(context.Background(), "station-1")
	if err != nil || !found || current.CNPJ != "00428184000195" {
		t.Fatalf("current = %+v, %v, %v", current, found, err)
	}
	// Permitted sources only; blank identity refused.
	if err := LinkOperator(context.Background(), store, newID, "station-1", "04218406000104", "suggestion", "x"); err == nil {
		t.Fatal("suggestion source must fail")
	}
	if err := LinkOperator(context.Background(), store, newID, "", "04218406000104", "registry", "x"); err == nil {
		t.Fatal("blank station must fail")
	}
}
