package adapters

import (
	"bytes"
	"context"
	"encoding/json"

	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type Handler struct {
	store   *Store
	catalog domain.Catalog
	effects map[string]Effect
}

var _ jobs.Handler = (*Handler)(nil)

// NewHandler requires one source-owned database effect for every catalogued
// consumer. Owning modules check current state/authority using this scoped tx.
// Register the returned jobs.Handler in the existing Dispatcher; no parallel
// queue/worker service is introduced.
func NewHandler(store *Store, catalog domain.Catalog, effects map[string]Effect) (*Handler, error) {
	if store == nil || store.pool == nil {
		return nil, domain.ErrUnavailable
	}
	names := catalog.Consumers()
	if len(names) == 0 || len(names) != len(effects) {
		return nil, domain.ErrInvalid
	}
	copy := make(map[string]Effect, len(effects))
	for _, name := range names {
		if effects[name] == nil {
			return nil, domain.ErrInvalid
		}
		copy[name] = effects[name]
	}
	return &Handler{store: store, catalog: catalog, effects: copy}, nil
}
func (*Handler) Kind() string { return domain.JobKind }
func (*Handler) Version() int { return domain.Version }

// DecodePayload enforces the flat, reference-only envelope even on internal
// queue data: no duplicates, aliases, nulls, unknown fields or extra content.
func DecodePayload(raw []byte) (domain.Envelope, error) {
	var out domain.Envelope
	if len(raw) == 0 || len(raw) > domain.MaxPayloadBytes {
		return out, domain.ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 8 {
		return out, domain.ErrInvalid
	}
	for _, name := range []string{"version", "org_id", "consumer", "event_id", "event_name", "target_type", "target_id", "revision"} {
		if fields[name] == nil || bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
			return out, domain.ErrInvalid
		}
	}
	if httpapi.DecodeJSON(raw, &out) != nil {
		return domain.Envelope{}, domain.ErrInvalid
	}
	return out, nil
}
func (h *Handler) Handle(ctx context.Context, job jobs.Job) error {
	if h == nil || h.store == nil {
		return domain.ErrUnavailable
	}
	if !audit.ValidID(job.ID) || job.Kind != domain.JobKind || job.Status != "leased" || job.LeaseToken < 1 || job.WorkerID == "" ||
		job.MaxAttempts != domain.MaxAttempts || job.Attempts < 1 || job.Attempts > domain.MaxAttempts {
		return domain.ErrInvalid
	}
	env, err := DecodePayload(job.Payload)
	if err != nil {
		return err
	}
	scope, err := h.catalog.ValidateEnvelope(env)
	if err != nil {
		return err
	}
	effect := h.effects[env.Consumer]
	if effect == nil {
		return domain.ErrInvalid
	}
	if job.DedupeKey != env.DedupeKey() {
		return domain.ErrDenied
	}
	return h.store.apply(ctx, scope, env, job, effect)
}
