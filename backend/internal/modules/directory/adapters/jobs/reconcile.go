package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Reconcile publishes one staged registry run to canonical identities.
// Staging itself stays operator-triggered until live source access is
// verified (the daily schedule below is disabled for the same reason);
// the job only ever publishes COMPLETE runs, so partial snapshots,
// worker death mid-run and retry exhaustion conserve IDs and the
// last-good publication by construction. There is no publication
// pointer to roll back: projections are append-only and revocation
// sticks via status (P25-T04).
type Reconcile struct {
	Store registry.Store
	Canon registry.Canonicalizer
}

// Kind implements jobs.Handler.
func (Reconcile) Kind() string { return "registry-reconcile" }

// Version implements jobs.Handler.
func (Reconcile) Version() int { return 1 }

// Payload is the versioned reconcile envelope.
type Payload struct {
	Version  int    `json:"version"`
	Source   string `json:"source"`
	Snapshot string `json:"snapshot"`
}

// EncodePayload builds the versioned envelope for scheduling.
func EncodePayload(source, snapshot string) ([]byte, error) {
	return json.Marshal(Payload{Version: 1, Source: source, Snapshot: snapshot})
}

// DedupeKey converges duplicate enqueues for one snapshot.
func DedupeKey(source, snapshot string) string {
	return "registry-reconcile:" + source + ":" + snapshot
}

// Handle implements jobs.Handler.
func (r Reconcile) Handle(ctx context.Context, job jobs.Job) error {
	var payload Payload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("reconcile: bad payload: %w", err)
	}
	if payload.Version != 1 {
		return fmt.Errorf("reconcile: unsupported payload version %d", payload.Version)
	}
	if payload.Source == "" || payload.Snapshot == "" {
		return errors.New("reconcile: source and snapshot are required")
	}
	if r.Store == nil || r.Canon == nil {
		return errors.New("reconcile: store and canonicalizer are required")
	}
	_, err := registry.ReconcileRun(ctx, r.Store, r.Canon, payload.Source, payload.Snapshot)
	return err
}
