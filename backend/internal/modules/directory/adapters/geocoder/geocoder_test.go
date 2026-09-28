package geocoder

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

// memStore is an in-memory domain.Store for unit coverage without a database.
type memStore struct {
	mu       sync.Mutex
	revs     []domain.LocationRevision
	recorded int
}

func (m *memStore) ResolveCNPJ(_ context.Context, _, _ string, _ map[string]string) (domain.Station, error) {
	return domain.Station{}, errors.New("unused")
}

func (m *memStore) Station(_ context.Context, id string) (domain.Station, error) {
	return domain.Station{ID: id}, nil
}

func (m *memStore) RecordLocation(_ context.Context, rev domain.LocationRevision) (domain.LocationRevision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rev.ID = string(rune('a' + len(m.revs)))
	rev.ObtainedAt = time.Now()
	m.revs = append(m.revs, rev)
	m.recorded++
	return rev, nil
}

func (m *memStore) ProjectLocation(_ context.Context, stationID, _ string) (domain.Station, error) {
	return domain.Station{ID: stationID}, nil
}

func (m *memStore) RetireIdentifier(_ context.Context, _, _ string) error { return nil }

func (m *memStore) Revisions(_ context.Context, _ string) ([]domain.LocationRevision, error) {
	return nil, nil
}

func testService(provider *FixtureProvider) (*Service, *memStore) {
	store := &memStore{}
	return &Service{
		Provider: provider,
		Store:    store,
		Limiter:  &Limiter{},
		Cache:    &Cache{TTL: time.Hour},
	}, store
}

func TestCacheAndQuota(t *testing.T) {
	provider := &FixtureProvider{ByStation: map[string]Candidate{
		"s1": {PointWKT: "POINT(-46.6 -23.5)", MatchInfo: "city"},
	}}
	svc, store := testService(provider)
	q := Query{StationID: "s1", Municipality: "SAO PAULO", State: "SP"}
	first, err := svc.Resolve(context.Background(), q)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if first.Quality != domain.QualityCityCentroid {
		t.Errorf("provider answer quality = %q, want city-centroid", first.Quality)
	}
	if first.Provider != "fixture" || first.SourceReference != "city" {
		t.Errorf("attribution missing: %+v", first)
	}
	second, err := svc.Resolve(context.Background(), q)
	if err != nil {
		t.Fatalf("cached resolve: %v", err)
	}
	if second.ID != first.ID {
		t.Error("cache returned a different revision")
	}
	if provider.CallCount() != 1 || store.recorded != 1 {
		t.Errorf("cache missed: calls=%d recorded=%d", provider.CallCount(), store.recorded)
	}
}

func TestQuotaRefusesWithoutProviderCall(t *testing.T) {
	provider := &FixtureProvider{ByStation: map[string]Candidate{
		"s1": {PointWKT: "POINT(-46.6 -23.5)"},
	}}
	now := time.Now()
	clock := now
	svc, _ := testService(provider)
	svc.Limiter = &Limiter{MaxCalls: 1, Window: time.Minute, now: func() time.Time { return clock }}
	svc.Cache = nil
	if _, err := svc.Resolve(context.Background(), Query{StationID: "s1"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	clock = now.Add(time.Second)
	if _, err := svc.Resolve(context.Background(), Query{StationID: "s2"}); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("quota bypassed: %v", err)
	}
	if provider.CallCount() != 1 {
		t.Errorf("refused call reached provider: %d", provider.CallCount())
	}
	clock = now.Add(2 * time.Minute)
	if _, err := svc.Resolve(context.Background(), Query{StationID: "s2"}); err != nil {
		t.Errorf("quota did not reset: %v", err)
	}
}

func TestIntervalSpacing(t *testing.T) {
	now := time.Now()
	lim := &Limiter{MinInterval: time.Minute, now: func() time.Time { return now }}
	if err := lim.Acquire(); err != nil {
		t.Fatal(err)
	}
	if err := lim.Acquire(); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("back-to-back allowed: %v", err)
	}
	now = now.Add(61 * time.Second)
	if err := lim.Acquire(); err != nil {
		t.Errorf("spaced call refused: %v", err)
	}
}

func TestCacheExpiryAndSeparation(t *testing.T) {
	now := time.Now()
	c := &Cache{TTL: time.Minute, now: func() time.Time { return now }}
	rev := domain.LocationRevision{ID: "r1"}
	c.put(Query{StationID: "s1"}, rev)
	if _, ok := c.get(Query{StationID: "s1"}); !ok {
		t.Fatal("fresh entry missed")
	}
	if _, ok := c.get(Query{StationID: "s2"}); ok {
		t.Error("different station hit")
	}
	now = now.Add(61 * time.Second)
	if _, ok := c.get(Query{StationID: "s1"}); ok {
		t.Error("expired entry hit")
	}
}

func TestAmbiguousAndMissing(t *testing.T) {
	provider := &FixtureProvider{ByStation: map[string]Candidate{
		"amb":   {PointWKT: "POINT(-46.6 -23.5)", Ambiguous: true},
		"empty": {PointWKT: "  "},
	}}
	svc, _ := testService(provider)
	amb, err := svc.Resolve(context.Background(), Query{StationID: "amb"})
	if err != nil || amb.Quality != domain.QualityCityCentroid {
		t.Errorf("ambiguous = %+v, %v", amb, err)
	}
	miss, err := svc.Resolve(context.Background(), Query{StationID: "ghost"})
	if err != nil {
		t.Fatalf("miss: %v", err)
	}
	if miss.Quality != domain.QualityUnknown || miss.PointWKT != "" {
		t.Errorf("miss = %+v", miss)
	}
	empty, err := svc.Resolve(context.Background(), Query{StationID: "empty"})
	if err != nil || empty.Quality != domain.QualityUnknown {
		t.Errorf("empty point = %+v, %v", empty, err)
	}
}

func TestTimeoutAndProviderError(t *testing.T) {
	slow := &FixtureProvider{
		ByStation: map[string]Candidate{"s1": {PointWKT: "POINT(0 0)"}},
		Delay:     5 * time.Second,
	}
	svc, store := testService(slow)
	svc.ProviderTimeout = 100 * time.Millisecond
	svc.Cache = nil
	if _, err := svc.Resolve(context.Background(), Query{StationID: "s1"}); err == nil {
		t.Error("slow provider accepted")
	}
	if store.recorded != 0 {
		t.Error("timed-out call recorded a revision")
	}
	boom := errors.New("boom")
	failing := &FixtureProvider{Err: boom}
	svc2, store2 := testService(failing)
	svc2.Cache = nil
	if _, err := svc2.Resolve(context.Background(), Query{StationID: "s1"}); !errors.Is(err, boom) {
		t.Errorf("provider error masked: %v", err)
	}
	if store2.recorded != 0 {
		t.Error("failed call recorded a revision")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc2.Resolve(ctx, Query{StationID: "s1"}); err == nil {
		t.Error("cancelled context accepted")
	}
}
