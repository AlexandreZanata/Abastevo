//go:build integration

package adapters

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

func projectionKey() domain.PriceKey {
	return domain.PriceKey{
		StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		Product:   "GASOLINE_REGULAR", Unit: "L",
		ConditionKind: "STANDARD", Qualifier: "STANDARD",
	}
}

func priceResult() domain.Result {
	return domain.Result{
		Verdict: domain.VerdictPrice, AmountMilli: 5999,
		Confidence: domain.ConfLow, Freshness: domain.FreshFresh,
		ExpiresAt:  time.Date(2026, 10, 2, 11, 0, 0, 0, time.UTC),
		ComputedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
		Supporters: 1, Groups: 1,
		Reasons:              []string{},
		WinnerObservationIDs: []string{"d6c74c23-63db-4c24-a2e5-408cb23bad27"},
		AlgorithmVersion:     domain.ConsensusV1,
	}
}

func TestSaveProjectionVersionsForward(t *testing.T) {
	s, pool, stationID := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, stationID, "sub-1")
	key := projectionKey()
	key.StationID = stationID
	anchorAt := time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC)
	next := time.Date(2026, 9, 30, 17, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if err := s.SaveProjection(ctx, key, priceResult(), obs.ID, anchorAt, cutoff, &next, "consensus-v1"); err != nil {
		t.Fatalf("save: %v", err)
	}
	var version int64
	var availability, confidence string
	var amount int64
	if err := pool.QueryRow(ctx, `SELECT projection_version, availability, confidence, amount_milli_brl FROM community_current_prices WHERE station_id = $1`, stationID).Scan(&version, &availability, &confidence, &amount); err != nil {
		t.Fatal(err)
	}
	if version != 1 || availability != "AVAILABLE" || confidence != "LOW" || amount != 5999 {
		t.Errorf("row = %d %q %q %d", version, availability, confidence, amount)
	}
	var inputs int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_projection_inputs`).Scan(&inputs); err != nil {
		t.Fatal(err)
	}
	if inputs != 1 {
		t.Errorf("audit inputs = %d, want one versioned row", inputs)
	}
	// A second recompute moves the version forward, never sideways.
	res := priceResult()
	res.AmountMilli = 6099
	later := time.Date(2026, 9, 30, 18, 0, 0, 0, time.UTC)
	if err := s.SaveProjection(ctx, key, res, obs.ID, anchorAt, cutoff, &later, "consensus-v1"); err != nil {
		t.Fatalf("resave: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT projection_version, amount_milli_brl FROM community_current_prices WHERE station_id = $1`, stationID).Scan(&version, &amount); err != nil {
		t.Fatal(err)
	}
	if version != 2 || amount != 6099 {
		t.Errorf("row = %d %d, want version 2 with the new amount", version, amount)
	}
}

func TestConcurrentRecomputeSerializes(t *testing.T) {
	s, pool, stationID := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, stationID, "sub-1")
	key := projectionKey()
	key.StationID = stationID
	anchorAt := time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC)
	next := time.Date(2026, 9, 30, 17, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res := priceResult()
			res.AmountMilli = int64(5900 + i)
			errs[i] = s.SaveProjection(ctx, key, res, obs.ID, anchorAt, cutoff, &next, "consensus-v1")
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("worker: %v", err)
		}
	}
	// Serialized writers converge on exactly eight versions: no lost
	// update, no stale overwrite, no error.
	var version int64
	var amount int64
	if err := pool.QueryRow(ctx, `SELECT projection_version, amount_milli_brl FROM community_current_prices WHERE station_id = $1`, stationID).Scan(&version, &amount); err != nil {
		t.Fatal(err)
	}
	if version != workers {
		t.Fatalf("version = %d, want %d serialized writes", version, workers)
	}
	if amount < 5900 || amount >= 5900+workers {
		t.Errorf("amount = %d outside writer range", amount)
	}
}

func TestDueProjectionsListsWakeups(t *testing.T) {
	s, _, stationID := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, stationID, "sub-1")
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	cutoff := now.Add(-48 * time.Hour)
	anchorAt := now.Add(-time.Hour)
	due := projectionKey()
	due.StationID = stationID
	if err := s.SaveProjection(ctx, due, priceResult(), obs.ID, anchorAt, cutoff, &past, "consensus-v1"); err != nil {
		t.Fatal(err)
	}
	calm := projectionKey()
	calm.StationID = stationID
	calm.Product = "ETHANOL"
	if err := s.SaveProjection(ctx, calm, priceResult(), obs.ID, anchorAt, cutoff, &future, "consensus-v1"); err != nil {
		t.Fatal(err)
	}
	dueKeys, err := s.DueProjections(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(dueKeys) != 1 || dueKeys[0].Product != "GASOLINE_REGULAR" {
		t.Fatalf("due = %+v, want only the past-due key", dueKeys)
	}
	// Clearing the wake removes the key without touching the price.
	if err := s.SaveProjection(ctx, due, priceResult(), obs.ID, anchorAt, cutoff, nil, "consensus-v1"); err != nil {
		t.Fatal(err)
	}
	dueKeys, err = s.DueProjections(ctx, now, 10)
	if err != nil || len(dueKeys) != 0 {
		t.Fatalf("due after clear = %+v, %v", dueKeys, err)
	}
}

func TestEligibleAnchorsFilterState(t *testing.T) {
	s, pool, stationID := freshStore(t)
	ctx := context.Background()
	good := validatedFixture(t, s, stationID, "sub-1")
	pending := testObs(stationID, "sub-2")
	pending.ID = "d6c74c23-63db-4c24-a2e5-408cb23cee01"
	var jobs [][]byte
	if _, _, err := s.Submit(ctx, pending, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	bad := testObs(stationID, "sub-3")
	bad.ID = "d6c74c23-63db-4c24-a2e5-408cb23cee02"
	if _, _, err := s.Submit(ctx, bad, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	claim, err := domain.ClaimValidation(bad, domain.StateReceived, "job-9", domain.ActorWorker, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(ctx, claim); err != nil {
		t.Fatal(err)
	}
	reject, err := domain.Reject(bad, domain.StateValidating, domain.ActorWorker, []string{"invalid-station"}, now, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(ctx, reject); err != nil {
		t.Fatal(err)
	}
	old := validatedFixture(t, s, stationID, "sub-4")
	if _, err := pool.Exec(ctx, "UPDATE community_observations SET received_at = $1 WHERE id = $2", now.Add(-50*time.Hour), old.ID); err != nil {
		t.Fatal(err)
	}
	key := projectionKey()
	key.StationID = stationID
	anchors, err := s.EligibleAnchors(ctx, key, now.Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(anchors) != 1 || anchors[0].ID != good.ID {
		t.Fatalf("anchors = %+v, want only the validated fresh fact", anchors)
	}
	if anchors[0].AmountMilli != 5999 || anchors[0].ContributorRef != "ref-1" {
		t.Errorf("anchor = %+v", anchors[0])
	}
}
