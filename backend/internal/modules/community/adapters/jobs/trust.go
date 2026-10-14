package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// TrustAffected rebuilds at most Batch exact keys per leased job. Continuations
// retain an internal keyset cursor; failures retry, never silently truncate.
type TrustAffected struct {
	Batch     int
	Affected  func(context.Context, string, *domain.PriceKey, int) ([]domain.PriceKey, error)
	Recompute func(context.Context, domain.PriceKey) (domain.Result, error)
	Enqueue   func(context.Context, string, []byte, string) error
}

func (TrustAffected) Kind() string { return "trust-affected-recompute" }
func (TrustAffected) Version() int { return 1 }

type trustAffectedPayload struct {
	Version        int                `json:"version"`
	ContributorRef string             `json:"contributor_ref"`
	After          *consensusPriceKey `json:"after,omitempty"`
}

func (h TrustAffected) Handle(ctx context.Context, job jobs.Job) error {
	var payload trustAffectedPayload
	decoder := json.NewDecoder(bytes.NewReader(job.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("trust recompute: invalid payload")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trust recompute: invalid payload")
	}
	if payload.Version != 1 || strings.TrimSpace(payload.ContributorRef) == "" || len(payload.ContributorRef) > 512 || job.ID == "" {
		return fmt.Errorf("trust recompute: invalid payload")
	}
	if h.Batch < 1 || h.Batch > 100 || h.Affected == nil || h.Recompute == nil || h.Enqueue == nil {
		return fmt.Errorf("trust recompute: missing bounded handler")
	}
	var after *domain.PriceKey
	if payload.After != nil {
		a := payload.After
		if a.StationID == "" || a.Product == "" || a.Unit == "" || a.ConditionKind == "" || a.Qualifier == "" {
			return fmt.Errorf("trust recompute: invalid cursor")
		}
		after = &domain.PriceKey{StationID: a.StationID, Product: a.Product, Unit: a.Unit, ConditionKind: a.ConditionKind, Qualifier: a.Qualifier}
	}
	keys, err := h.Affected(ctx, payload.ContributorRef, after, h.Batch)
	if err != nil {
		return err
	}
	if len(keys) > h.Batch {
		return fmt.Errorf("trust recompute: oversized page")
	}
	for _, key := range keys {
		if _, err := h.Recompute(ctx, key); err != nil {
			return err
		}
	}
	if len(keys) == h.Batch {
		last := keys[len(keys)-1]
		payload.After = &consensusPriceKey{StationID: last.StationID, Product: last.Product, Unit: last.Unit, ConditionKind: last.ConditionKind, Qualifier: last.Qualifier}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		// Deduplication belongs to this immutable page job, not the lifetime
		// of the contributor: a later block/rehabilitation must rebuild again.
		sum := sha256.Sum256([]byte(job.ID + "|" + application.PriceKeyString(last)))
		return h.Enqueue(ctx, h.Kind(), raw, fmt.Sprintf("trust-page:%x", sum))
	}
	return nil
}
