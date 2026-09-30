package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

type fakeClock struct {
	mu  sync.Mutex
	now int64
}

func (f *fakeClock) NowUnix() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func testService(gate AccountGate, stations StationExists) (*Service, *MemStore) {
	store := NewMemStore()
	var n int
	var mu sync.Mutex
	return &Service{
		Clock:         &fakeClock{now: 1_700_000_000},
		Store:         store,
		CheckAccount:  gate,
		StationExists: stations,
		IDGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			n++
			return "id-" + string(rune('0'+n)), nil
		},
	}, store
}

func allowAll(_ context.Context, _ string) error { return nil }

func knownStations(_ context.Context, stationID string) (bool, error) {
	return stationID == "station-1", nil
}

func TestRateHappyAndStatsExact(t *testing.T) {
	svc, _ := testService(allowAll, knownStations)
	ctx := context.Background()

	got, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 5)
	if err != nil {
		t.Fatalf("Rate: %v", err)
	}
	if !got.Created || got.Rating.Revision != 1 || got.Rating.Stars != 5 {
		t.Errorf("first rating must create revision 1: %+v", got.Rating)
	}
	if got.Stats.Count != 1 || got.Stats.Sum != 5 {
		t.Errorf("stats must be 1/5, got %+v", got.Stats)
	}
	if mean, ok := got.Stats.MeanMilli(); !ok || mean != 5000 {
		t.Errorf("mean must be 5000 milli, got %d %v", mean, ok)
	}

	second, err := svc.Rate(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", 4)
	if err != nil {
		t.Fatal(err)
	}
	if second.Stats.Count != 2 || second.Stats.Sum != 9 {
		t.Errorf("stats must be 2/9, got %+v", second.Stats)
	}
	if mean, _ := second.Stats.MeanMilli(); mean != 4500 {
		t.Errorf("mean must be 4500 milli, got %d", mean)
	}
}

func TestRateConvergesAndEditsRevision(t *testing.T) {
	svc, _ := testService(allowAll, knownStations)
	ctx := context.Background()

	first, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 5)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 5)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || again.Rating.ID != first.Rating.ID || again.Rating.Revision != 1 {
		t.Errorf("equal stars must converge, got %+v", again.Rating)
	}
	if again.Stats.Count != 1 {
		t.Errorf("converge must not inflate counts: %+v", again.Stats)
	}
	edited, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 3)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Created || edited.Rating.Revision != 2 || edited.Rating.Stars != 3 {
		t.Errorf("changed stars must bump revision, got %+v", edited.Rating)
	}
	if edited.Stats.Count != 1 || edited.Stats.Sum != 3 {
		t.Errorf("stats must be 1/3, got %+v", edited.Stats)
	}
}

func TestDeleteTombstonesAndStats(t *testing.T) {
	svc, _ := testService(allowAll, knownStations)
	ctx := context.Background()

	if _, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rate(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", 3); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteRating(ctx, "acc-1", "station-1", "GASOLINE_REGULAR"); err != nil {
		t.Fatalf("DeleteRating: %v", err)
	}
	stats, found, err := svc.Store.Stats(ctx, "station-1", "GASOLINE_REGULAR")
	if err != nil || !found {
		t.Fatalf("stats must persist: %+v %v %v", stats, found, err)
	}
	if stats.Count != 1 || stats.Sum != 3 {
		t.Errorf("deleted rating must leave stats, got %+v", stats)
	}
	if err := svc.DeleteRating(ctx, "acc-1", "station-1", "GASOLINE_REGULAR"); !errors.Is(err, domain.ErrRatingNotFound) {
		t.Errorf("second delete must be not-found, got %v", err)
	}
	if err := svc.DeleteRating(ctx, "ghost", "station-1", "GASOLINE_REGULAR"); !errors.Is(err, domain.ErrRatingNotFound) {
		t.Errorf("unknown key must be not-found, got %v", err)
	}
	// Re-rating after delete starts a fresh revision line.
	fresh, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh.Created || fresh.Rating.Revision != 1 {
		t.Errorf("post-delete rating must recreate, got %+v", fresh.Rating)
	}
}

func TestGateAndTargetRefusals(t *testing.T) {
	ctx := context.Background()
	denied := errors.New("feedback: suspended test")
	svc, _ := testService(func(context.Context, string) error { return denied }, knownStations)
	if _, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 5); !errors.Is(err, denied) {
		t.Errorf("suspended session must refuse, got %v", err)
	}
	if err := svc.DeleteRating(ctx, "acc-1", "station-1", "GASOLINE_REGULAR"); !errors.Is(err, denied) {
		t.Errorf("suspended delete must refuse, got %v", err)
	}

	svc, _ = testService(allowAll, knownStations)
	for _, tc := range []struct {
		account, station, product string
		stars                     int
	}{
		{"", "station-1", "GASOLINE_REGULAR", 5},
		{"acc-1", "unknown-station", "GASOLINE_REGULAR", 5},
		{"acc-1", "station-1", "", 5},
		{"acc-1", "station-1", "GASOLINE_REGULAR", 0},
		{"acc-1", "station-1", "GASOLINE_REGULAR", 6},
	} {
		if _, err := svc.Rate(ctx, tc.account, tc.station, tc.product, tc.stars); err == nil {
			t.Errorf("invalid rate %+v must refuse", tc)
		}
	}

	svc, _ = testService(nil, nil)
	if _, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 5); !errors.Is(err, domain.ErrGateRequired) {
		t.Errorf("missing ports must fail closed, got %v", err)
	}
}

func TestRebuildStatsReconciles(t *testing.T) {
	svc, store := testService(allowAll, knownStations)
	ctx := context.Background()

	if _, err := svc.Rate(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rate(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", 1); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	key := ratingKey("acc-1", "station-1", "GASOLINE_REGULAR")
	rec := store.ratings[key]
	rec.Stars = 2
	store.ratings[key] = rec
	store.stats[targetKey("station-1", "GASOLINE_REGULAR")] = domain.RatingStats{}
	store.mu.Unlock()

	rebuilt, err := svc.RebuildStats(ctx, "station-1", "GASOLINE_REGULAR")
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Count != 2 || rebuilt.Sum != 3 {
		t.Errorf("rebuild must recover 2/3, got %+v", rebuilt)
	}
}
