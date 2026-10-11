package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// SEC08-B (AUD-12): one separate synthetic client/key drives the signed
// submit path with fabricated but internally consistent location
// metadata. Geometry cannot detect the fabrication (recorded honestly);
// replay converges, teleport flags review, stale/coarse/missing refuse
// or degrade, unreviewed stations never gain proximity.

type advStore struct {
	mu   sync.Mutex
	rows map[string]domain.Observation
}

func (s *advStore) Submit(_ context.Context, obs domain.Observation, _ func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rows == nil {
		s.rows = map[string]domain.Observation{}
	}
	s.rows[obs.ID] = obs
	return obs.ID, false, nil
}

func (s *advStore) Observation(_ context.Context, id string) (domain.Observation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	obs, ok := s.rows[id]
	if !ok {
		return domain.Observation{}, errors.New("community: not found")
	}
	return obs, nil
}

func (s *advStore) Decisions(_ context.Context, _ string) ([]domain.Decision, error) {
	return nil, nil
}

func (s *advStore) ListByContributor(_ context.Context, _ string, _ int, _ time.Time, _ string, _ bool) ([]domain.Observation, error) {
	return nil, nil
}

func advPorts(store *advStore, locate func(context.Context, string, float64, float64) (float64, StationSite, error)) Ports {
	var mu sync.Mutex
	seen := map[string]Outcome{}
	n := 0
	return Ports{
		Clock: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 30, 0, time.UTC) },
		NewID: func() (string, error) {
			n++
			return fmt.Sprintf("d6c74c23-63db-4c24-a2e5-408cb23bad%02d", 30+n), nil
		},
		Attribution: func(_ context.Context, id string) (string, error) { return "tok-" + id[:8], nil },
		CheckQuota:  func(context.Context, string, string) (time.Duration, error) { return 0, nil },
		Idempotent: func(_ context.Context, key IdempotencyKey, _ []byte, run func(context.Context) (Outcome, error)) (Outcome, error) {
			mu.Lock()
			defer mu.Unlock()
			k := key.ContributorID + "\x00" + key.Method + "\x00" + key.Route + "\x00" + key.Key
			if prev, ok := seen[k]; ok {
				prev.Replayed = true
				return prev, nil
			}
			out, err := run(context.Background())
			if err != nil {
				return Outcome{}, err
			}
			seen[k] = out
			return out, nil
		},
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error { return nil },
		Store:      store,
		Locate:     locate,
		LastSite: func(context.Context, string) (string, time.Time, bool, error) {
			return "", time.Time{}, false, nil
		},
		StationDistance: func(context.Context, string, string) (float64, error) { return 0, nil },
	}
}

func advCaller() Caller {
	return Caller{ContributorID: "c-adv", Fingerprint: "fp:adv", KeyID: "k-adv", Token: "tok-synth-adv"}
}

func advDTO(verdict string, accuracy *float64, captured *time.Time, lat, lon *float64) SubmitDTO {
	return SubmitDTO{
		ClientSubmissionID: "adv-sub-1", StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999, RawText: "5,999",
		ConditionKind: "STANDARD", ClaimedCapturedAt: time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC),
		Location: &LocationEvidence{
			ClaimedVerdict: verdict, PermissionGranted: true, HasFix: true,
			SourceInfoPresent: true, AccuracyMeters: accuracy,
			CapturedAt: captured, Latitude: lat, Longitude: lon,
		},
	}
}

func advNear() func(context.Context, string, float64, float64) (float64, StationSite, error) {
	return func(context.Context, string, float64, float64) (float64, StationSite, error) {
		return 50, SitePrecise, nil
	}
}

func TestAdversarialConsistentFabricationPassesGeometry(t *testing.T) {
	// Honest residual: internally consistent fabrication is
	// indistinguishable from a genuine fix at this layer. The test
	// pins that it lands NEAR (not refused, not silently dropped) so
	// the residual is measured, never hidden.
	store := &advStore{}
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	res, err := Submit(context.Background(), advPorts(store, advNear()), advCaller(),
		"POST", "/v1/observations", "adv-key-1", []byte(`{"adv":1}`),
		advDTO(FixVerified, acc(25), &captured, acc(-23.55052), acc(-46.633309)))
	if err != nil {
		t.Fatalf("consistent fabrication refused: %v", err)
	}
	if res.Replayed {
		t.Error("first submit must not replay")
	}
	stored, err := store.Observation(context.Background(), res.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LocationVerdict != FixVerified || stored.LocationProximity != ProximityNear {
		t.Errorf("bands = %q/%q, want VERIFIED/NEAR", stored.LocationVerdict, stored.LocationProximity)
	}
	if stored.ContributorRef != "tok-synth-adv" {
		t.Errorf("attribution leaked across clients: %q", stored.ContributorRef)
	}
}

func TestAdversarialReplayConverges(t *testing.T) {
	store := &advStore{}
	p := advPorts(store, advNear())
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"adv":2}`)
	first, err := Submit(context.Background(), p, advCaller(), "POST", "/v1/observations", "adv-key-2", body,
		advDTO(FixVerified, acc(25), &captured, acc(-23.55052), acc(-46.633309)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Submit(context.Background(), p, advCaller(), "POST", "/v1/observations", "adv-key-2", body,
		advDTO(FixVerified, acc(25), &captured, acc(-23.55052), acc(-46.633309)))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.ObservationID != first.ObservationID {
		t.Errorf("replay must converge, got %+v vs %+v", first, second)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.rows) != 1 {
		t.Errorf("replay stored %d rows, want 1", len(store.rows))
	}
}

func TestAdversarialTeleportFlagsReview(t *testing.T) {
	store := &advStore{}
	p := advPorts(store, advNear())
	now := time.Date(2026, 9, 30, 12, 0, 30, 0, time.UTC)
	p.LastSite = func(context.Context, string) (string, time.Time, bool, error) {
		return "other-station", now.Add(-time.Minute), true, nil
	}
	p.StationDistance = func(context.Context, string, string) (float64, error) { return 50000, nil }
	captured := now.Add(-30 * time.Second)
	res, err := Submit(context.Background(), p, advCaller(), "POST", "/v1/observations", "adv-key-3", []byte(`{"adv":3}`),
		advDTO(FixVerified, acc(25), &captured, acc(-23.55052), acc(-46.633309)))
	if err != nil {
		t.Fatalf("teleporting submit refused: %v", err)
	}
	stored, _ := store.Observation(context.Background(), res.ObservationID)
	if stored.LocationReason != RiskTeleportSuspect {
		t.Errorf("reason = %q, want teleport-suspect", stored.LocationReason)
	}
	if stored.LocationVerdict != FixVerified || stored.LocationProximity != ProximityNear {
		t.Errorf("teleport keeps bands while flagging review: %+v", stored)
	}
}

func TestAdversarialStaleAndCoarseRefuse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		accuracy float64
		captured time.Time
	}{
		{"stale-fix", 25, time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC)},
		{"coarse-accuracy", 500, time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)},
	} {
		store := &advStore{}
		_, err := Submit(context.Background(), advPorts(store, advNear()), advCaller(),
			"POST", "/v1/observations", "adv-key-"+tc.name, []byte(`{"adv":4}`),
			advDTO(FixVerified, acc(tc.accuracy), &tc.captured, acc(-23.55052), acc(-46.633309)))
		if !errors.Is(err, ErrLocationForged) {
			t.Errorf("%s: err = %v, want location-forged", tc.name, err)
		}
		store.mu.Lock()
		n := len(store.rows)
		store.mu.Unlock()
		if n != 0 {
			t.Errorf("%s: refused submit stored %d rows", tc.name, n)
		}
	}
}

func TestAdversarialMissingAndUnreviewedDegrade(t *testing.T) {
	// Missing coordinates: VERIFIED fix, UNKNOWN proximity — never NEAR.
	store := &advStore{}
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	res, err := Submit(context.Background(), advPorts(store, advNear()), advCaller(),
		"POST", "/v1/observations", "adv-key-5", []byte(`{"adv":5}`),
		advDTO(FixVerified, acc(25), &captured, nil, nil))
	if err != nil {
		t.Fatalf("missing-coords refused: %v", err)
	}
	stored, _ := store.Observation(context.Background(), res.ObservationID)
	if stored.LocationVerdict != FixVerified || stored.LocationProximity != ProximityUnknown {
		t.Errorf("missing coords = %q/%q, want VERIFIED/UNKNOWN", stored.LocationVerdict, stored.LocationProximity)
	}
	// Unreviewed station geometry: same degradation, never consensus.
	centroid := func(context.Context, string, float64, float64) (float64, StationSite, error) {
		return 10, SiteCentroid, nil
	}
	store2 := &advStore{}
	res2, err := Submit(context.Background(), advPorts(store2, centroid), advCaller(),
		"POST", "/v1/observations", "adv-key-6", []byte(`{"adv":6}`),
		advDTO(FixVerified, acc(25), &captured, acc(-23.55052), acc(-46.633309)))
	if err != nil {
		t.Fatalf("unreviewed-site refused: %v", err)
	}
	stored2, _ := store2.Observation(context.Background(), res2.ObservationID)
	if stored2.LocationProximity != ProximityUnknown {
		t.Errorf("unreviewed site = %q, want UNKNOWN", stored2.LocationProximity)
	}
}

func TestAdversarialUnknownVerdictAndSimulatedHonest(t *testing.T) {
	// A garbage verdict can never match the recompute.
	store := &advStore{}
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	_, err := Submit(context.Background(), advPorts(store, advNear()), advCaller(),
		"POST", "/v1/observations", "adv-key-7", []byte(`{"adv":7}`),
		advDTO("TRUSTED", acc(25), &captured, acc(-23.55052), acc(-46.633309)))
	if !errors.Is(err, ErrLocationForged) {
		t.Errorf("garbage verdict = %v, want location-forged", err)
	}
	// An honest simulated flag stores SIMULATED/UNKNOWN, never proximity.
	store2 := &advStore{}
	dto := advDTO(FixSimulated, acc(10), &captured, nil, nil)
	dto.Location.Simulated = true
	res, err := Submit(context.Background(), advPorts(store2, advNear()), advCaller(),
		"POST", "/v1/observations", "adv-key-8", []byte(`{"adv":8}`), dto)
	if err != nil {
		t.Fatalf("honest simulated refused: %v", err)
	}
	stored, _ := store2.Observation(context.Background(), res.ObservationID)
	if stored.LocationVerdict != FixSimulated || stored.LocationProximity != ProximityUnknown {
		t.Errorf("simulated = %q/%q, want SIMULATED/UNKNOWN", stored.LocationVerdict, stored.LocationProximity)
	}
}
