package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Consensus handles one community-consensus job: exactly one price
// recomputation, triggered either by an observation write or by a price
// key from the boundary sweeper. Exactly one trigger travels per job;
// a nil result error completes it, anything else retries with backoff
// until the cap, then parks for audited replay.
type Consensus struct {
	ByObservation func(ctx context.Context, observationID string) (domain.Result, error)
	ByKey         func(ctx context.Context, key domain.PriceKey) (domain.Result, error)
}

// Kind implements jobs.Handler.
func (Consensus) Kind() string { return "community-consensus" }

// Version implements jobs.Handler.
func (Consensus) Version() int { return 1 }

type consensusPriceKey struct {
	StationID     string `json:"station_id"`
	Product       string `json:"fuel_product"`
	Unit          string `json:"unit"`
	ConditionKind string `json:"condition_kind"`
	Qualifier     string `json:"qualifier_id"`
}

// Handle parses the versioned envelope and delegates to one trigger.
func (v Consensus) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version       int                `json:"version"`
		ObservationID string             `json:"observation_id"`
		PriceKey      *consensusPriceKey `json:"price_key"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("consensus: bad payload: %w", err)
	}
	if payload.Version != 1 {
		return fmt.Errorf("consensus: bad payload")
	}
	byObs := strings.TrimSpace(payload.ObservationID) != ""
	byKey := payload.PriceKey != nil && strings.TrimSpace(payload.PriceKey.StationID) != ""
	if byObs == byKey {
		return fmt.Errorf("consensus: bad payload")
	}
	if byObs {
		if v.ByObservation == nil {
			return fmt.Errorf("consensus: no observation recompute configured")
		}
		_, err := v.ByObservation(ctx, strings.TrimSpace(payload.ObservationID))
		return err
	}
	if v.ByKey == nil {
		return fmt.Errorf("consensus: no key recompute configured")
	}
	key := payload.PriceKey
	_, err := v.ByKey(ctx, domain.PriceKey{
		StationID: key.StationID, Product: key.Product, Unit: key.Unit,
		ConditionKind: key.ConditionKind, Qualifier: key.Qualifier,
	})
	return err
}

// Boundary handles one consensus-boundary job: every price key past its
// scheduled recompute gets exactly one deduplicated recompute job.
// Empty sweeps complete silently; failures retry the whole sweep.
type Boundary struct {
	Clock   func() time.Time
	Batch   int
	Due     func(ctx context.Context, now time.Time, batch int) ([]domain.PriceKey, error)
	Enqueue func(ctx context.Context, kind string, payload []byte, dedupe string) error
}

// Kind implements jobs.Handler.
func (Boundary) Kind() string { return "consensus-boundary" }

// Version implements jobs.Handler.
func (Boundary) Version() int { return 1 }

// Handle lists due keys and enqueues one recompute job per key with a
// key-scoped dedupe, so concurrent sweeps and restarts converge.
func (v Boundary) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("boundary: bad payload: %w", err)
	}
	if payload.Version != 1 {
		return fmt.Errorf("boundary: bad payload")
	}
	if v.Clock == nil || v.Due == nil || v.Enqueue == nil || v.Batch < 1 {
		return fmt.Errorf("boundary: no sweep configured")
	}
	keys, err := v.Due(ctx, v.Clock(), v.Batch)
	if err != nil {
		return err
	}
	for _, key := range keys {
		raw, err := json.Marshal(map[string]any{
			"version": 1,
			"price_key": map[string]any{
				"station_id": key.StationID, "fuel_product": key.Product,
				"unit": key.Unit, "condition_kind": key.ConditionKind,
				"qualifier_id": key.Qualifier,
			},
		})
		if err != nil {
			return err
		}
		if err := v.Enqueue(ctx, "community-consensus", raw, "consensus:key:"+application.PriceKeyString(key)); err != nil {
			return err
		}
	}
	return nil
}
