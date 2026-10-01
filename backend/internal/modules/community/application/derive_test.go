package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

type fakeSignalStore struct {
	obs     map[string]domain.Observation
	bands   map[string]Bands
	upserts int
}

func newFakeSignalStore() *fakeSignalStore {
	return &fakeSignalStore{obs: map[string]domain.Observation{}, bands: map[string]Bands{}}
}

func (f *fakeSignalStore) Observation(_ context.Context, id string) (domain.Observation, error) {
	o, ok := f.obs[id]
	if !ok {
		return domain.Observation{}, errors.New("adapters: unknown observation")
	}
	return o, nil
}

func (f *fakeSignalStore) UpsertSignals(_ context.Context, id string, b Bands, _ time.Time) error {
	f.bands[id] = b
	f.upserts++
	return nil
}

func deriveObs() domain.Observation {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	obs, _, err := domain.NewObservation(domain.Params{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", ContributorRef: "tok-c1",
		ClientSubmissionID: "sub-1", StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD", EvidenceID: "e0000000-0000-4000-8000-000000000001",
		ClaimedCapturedAt: now.Add(-time.Hour), ReceivedAt: now,
	})
	if err != nil {
		panic(err)
	}
	return obs
}

func derivePorts(store *fakeSignalStore) SignalPorts {
	mean := int64(5999)
	return SignalPorts{
		Clock: func() time.Time { return time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC) },
		Store: store,
		Station: func(context.Context, string) (StationSite, error) {
			return SitePrecise, nil
		},
		Photo: func(context.Context, string) (bool, int, error) {
			return true, 1, nil
		},
		Regional: func(context.Context, string, string, string) (int64, bool, error) {
			return mean, true, nil
		},
	}
}

func TestDeriveSignalsPersistsBands(t *testing.T) {
	store := newFakeSignalStore()
	obs := deriveObs()
	store.obs[obs.ID] = obs
	bands, err := DeriveSignals(context.Background(), derivePorts(store), obs.ID)
	if err != nil {
		t.Fatalf("derive = %v", err)
	}
	// Duplicate photo evidence routes to review while staying explicit.
	if bands.Photo != PhotoDuplicate || !bands.NeedsReview {
		t.Errorf("bands = %+v", bands)
	}
	stored, ok := store.bands[obs.ID]
	if !ok || store.upserts != 1 {
		t.Fatalf("persisted = %+v upserts = %d", stored, store.upserts)
	}
	if stored.Photo != bands.Photo || stored.PolicyVersion != SignalsV1 {
		t.Errorf("stored = %+v, want %+v", stored, bands)
	}
	// Recomputation converges: same inputs, one row, no fork.
	again, err := DeriveSignals(context.Background(), derivePorts(store), obs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if store.upserts != 2 || again.Photo != PhotoDuplicate {
		t.Errorf("recompute = %+v upserts = %d", again, store.upserts)
	}
}

func TestDeriveSignalsMergesStoredVerifiedProximity(t *testing.T) {
	// A submit-time verified NEAR band flows into the projection;
	// recomputation converges on the same stored band.
	store := newFakeSignalStore()
	obs := deriveObs()
	obs.LocationVerdict = FixVerified
	obs.LocationProximity = ProximityNear
	store.obs[obs.ID] = obs
	bands, err := DeriveSignals(context.Background(), derivePorts(store), obs.ID)
	if err != nil {
		t.Fatalf("derive = %v", err)
	}
	if bands.Proximity != ProximityNear {
		t.Errorf("proximity = %q, want stored NEAR", bands.Proximity)
	}
	for _, r := range bands.RiskCodes {
		if r == RiskTeleportSuspect {
			t.Errorf("clean verify must not route teleport: %+v", bands)
		}
	}
}

func TestDeriveSignalsIgnoresStoredBandOffPreciseSite(t *testing.T) {
	// The stored band only counts on a precise station: a centroid
	// site keeps the honest nil-claim path even for VERIFIED rows.
	store := newFakeSignalStore()
	obs := deriveObs()
	obs.LocationVerdict = FixVerified
	obs.LocationProximity = ProximityNear
	store.obs[obs.ID] = obs
	ports := derivePorts(store)
	ports.Station = func(context.Context, string) (StationSite, error) {
		return SiteCentroid, nil
	}
	bands, err := DeriveSignals(context.Background(), ports, obs.ID)
	if err != nil {
		t.Fatalf("derive = %v", err)
	}
	if bands.Proximity != ProximityUnknown {
		t.Errorf("proximity = %q, want UNKNOWN off precise site", bands.Proximity)
	}
}

func TestDeriveSignalsRoutesTeleportToReview(t *testing.T) {
	store := newFakeSignalStore()
	obs := deriveObs()
	obs.LocationVerdict = FixVerified
	obs.LocationProximity = ProximityNear
	obs.LocationReason = RiskTeleportSuspect
	store.obs[obs.ID] = obs
	bands, err := DeriveSignals(context.Background(), derivePorts(store), obs.ID)
	if err != nil {
		t.Fatalf("derive = %v", err)
	}
	if bands.Proximity != ProximityNear {
		t.Errorf("teleport keeps bands, only routes: %+v", bands)
	}
	found := false
	for _, r := range bands.RiskCodes {
		if r == RiskTeleportSuspect {
			found = true
		}
	}
	if !found || !bands.NeedsReview {
		t.Errorf("teleport must route to review: %+v", bands)
	}
}

func TestDeriveSignalsHonestUnknowns(t *testing.T) {
	// No station point, no photo, no benchmark: every band abstains and
	// the fact routes to review instead of certifying anything.
	store := newFakeSignalStore()
	obs := deriveObs()
	obs.EvidenceID = ""
	store.obs[obs.ID] = obs
	ports := derivePorts(store)
	ports.Station = func(context.Context, string) (StationSite, error) { return SiteUnknown, nil }
	ports.Photo = func(context.Context, string) (bool, int, error) { return false, 0, nil }
	ports.Regional = func(context.Context, string, string, string) (int64, bool, error) {
		return 0, false, nil
	}
	bands, err := DeriveSignals(context.Background(), ports, obs.ID)
	if err != nil {
		t.Fatalf("derive = %v", err)
	}
	if bands.Proximity != ProximityUnknown || bands.Photo != PhotoNone || bands.Regional != RegionalUnknown {
		t.Errorf("bands overclaim: %+v", bands)
	}
	if !bands.NeedsReview {
		t.Error("unknowns did not route to review")
	}
}

func TestDeriveSignalsPropagatesUnavailable(t *testing.T) {
	// Unavailable signals never count as verified: any port failure
	// aborts before persistence.
	store := newFakeSignalStore()
	obs := deriveObs()
	store.obs[obs.ID] = obs
	boom := errors.New("directory down")
	ports := derivePorts(store)
	ports.Station = func(context.Context, string) (StationSite, error) { return "", boom }
	if _, err := DeriveSignals(context.Background(), ports, obs.ID); !errors.Is(err, boom) {
		t.Errorf("station failure = %v", err)
	}
	ports = derivePorts(store)
	ports.Photo = func(context.Context, string) (bool, int, error) { return false, 0, boom }
	if _, err := DeriveSignals(context.Background(), ports, obs.ID); !errors.Is(err, boom) {
		t.Errorf("photo failure = %v", err)
	}
	ports = derivePorts(store)
	ports.Regional = func(context.Context, string, string, string) (int64, bool, error) {
		return 0, false, boom
	}
	if _, err := DeriveSignals(context.Background(), ports, obs.ID); !errors.Is(err, boom) {
		t.Errorf("regional failure = %v", err)
	}
	if len(store.bands) != 0 {
		t.Errorf("failures persisted bands: %v", store.bands)
	}
	if _, err := DeriveSignals(context.Background(), derivePorts(store), "missing"); err == nil {
		t.Error("missing observation accepted")
	}
}
