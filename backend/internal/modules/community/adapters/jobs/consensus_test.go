package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestConsensusHandleRoutesTriggers(t *testing.T) {
	var byObs, byKey string
	h := Consensus{
		ByObservation: func(_ context.Context, id string) (domain.Result, error) {
			byObs = id
			return domain.Result{}, nil
		},
		ByKey: func(_ context.Context, key domain.PriceKey) (domain.Result, error) {
			byKey = key.StationID
			return domain.Result{}, nil
		},
	}
	obsPayload := `{"version":1,"observation_id":"d6c74c23-63db-4c24-a2e5-408cb23bad27"}`
	if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(obsPayload)}); err != nil {
		t.Fatalf("observation trigger = %v", err)
	}
	if byObs != "d6c74c23-63db-4c24-a2e5-408cb23bad27" || byKey != "" {
		t.Errorf("routed obs=%q key=%q", byObs, byKey)
	}
	keyPayload := `{"version":1,"price_key":{"station_id":"s1","fuel_product":"GASOLINE_REGULAR","unit":"L","condition_kind":"STANDARD","qualifier_id":"STANDARD"}}`
	if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(keyPayload)}); err != nil {
		t.Fatalf("key trigger = %v", err)
	}
	if byKey != "s1" {
		t.Errorf("key routed = %q", byKey)
	}
}

func TestConsensusHandleRejectsBadPayload(t *testing.T) {
	h := Consensus{
		ByObservation: func(context.Context, string) (domain.Result, error) { return domain.Result{}, nil },
		ByKey:         func(context.Context, domain.PriceKey) (domain.Result, error) { return domain.Result{}, nil },
	}
	for name, raw := range map[string]string{
		"malformed": `{`,
		"version":   `{"version":2,"observation_id":"o"}`,
		"empty":     `{"version":1}`,
		"both":      `{"version":1,"observation_id":"o","price_key":{"station_id":"s"}}`,
		"key-empty": `{"version":1,"price_key":{"station_id":""}}`,
	} {
		if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(raw)}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := (Consensus{}).Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"observation_id":"o"}`)}); err == nil {
		t.Error("nil orchestration accepted")
	}
	boom := errors.New("store down")
	failing := Consensus{
		ByObservation: func(context.Context, string) (domain.Result, error) { return domain.Result{}, boom },
	}
	if err := failing.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"observation_id":"o"}`)}); !errors.Is(err, boom) {
		t.Errorf("failure = %v", err)
	}
}

func boundaryHandler(due []domain.PriceKey, enqueued *[]string) Boundary {
	return Boundary{
		Clock: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
		Batch: 100,
		Due: func(context.Context, time.Time, int) ([]domain.PriceKey, error) {
			return due, nil
		},
		Enqueue: func(_ context.Context, kind string, payload []byte, dedupe string) error {
			if kind != "community-consensus" {
				return errors.New("wrong kind " + kind)
			}
			*enqueued = append(*enqueued, dedupe)
			return nil
		},
	}
}

func boundaryKey(suffix string) domain.PriceKey {
	return domain.PriceKey{
		StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		Product:   "GASOLINE_REGULAR", Unit: "L",
		ConditionKind: "STANDARD", Qualifier: suffix,
	}
}

func TestBoundaryEnqueuesPerKey(t *testing.T) {
	var enqueued []string
	h := boundaryHandler([]domain.PriceKey{boundaryKey("STANDARD"), boundaryKey("Q1")}, &enqueued)
	if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1}`)}); err != nil {
		t.Fatalf("handle = %v", err)
	}
	if len(enqueued) != 2 || enqueued[0] == enqueued[1] {
		t.Errorf("dedupes = %v, want one distinct recompute job per key", enqueued)
	}
	var none []string
	h = boundaryHandler(nil, &none)
	if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1}`)}); err != nil {
		t.Fatalf("empty sweep = %v", err)
	}
	if len(none) != 0 {
		t.Errorf("empty sweep enqueued: %v", none)
	}
}

func TestBoundaryRejectsBadPayload(t *testing.T) {
	var enqueued []string
	h := boundaryHandler(nil, &enqueued)
	for name, raw := range map[string]string{
		"malformed": `{`,
		"version":   `{"version":2}`,
	} {
		if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(raw)}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := (Boundary{}).Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1}`)}); err == nil {
		t.Error("nil dependencies accepted")
	}
	boom := errors.New("db down")
	failing := boundaryHandler(nil, &enqueued)
	failing.Due = func(context.Context, time.Time, int) ([]domain.PriceKey, error) {
		return nil, boom
	}
	if err := failing.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1}`)}); !errors.Is(err, boom) {
		t.Errorf("due failure = %v", err)
	}
}
