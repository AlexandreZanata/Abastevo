package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Discover runs one bounded registry API discovery as a job. Fetching
// stays snapshot-scoped and idempotent: a complete snapshot replays
// without touching the network, so restarts and duplicate enqueues
// converge instead of hammering the provider. Terminal run outcomes
// (complete or quarantined) complete the job; transport and provider
// failures return an error so the dispatcher backs off and eventually
// parks the job dead. No Rust networking is involved: Go owns fetch
// under the existing allowlisted discovery policy.
type Discover struct {
	Store  registry.Store
	Config registry.APIConfig
}

// Kind implements jobs.Handler.
func (Discover) Kind() string { return "registry-discover" }

// Version implements jobs.Handler.
func (Discover) Version() int { return 1 }

// DiscoverPayload is the versioned discovery envelope. Snapshot
// identifies one scope period (for example "api:SP:2026-10-09"); scope
// fields bound the traversal.
type DiscoverPayload struct {
	Version      int    `json:"version"`
	Snapshot     string `json:"snapshot"`
	CNPJ         string `json:"cnpj"`
	UF           string `json:"uf"`
	Municipality string `json:"municipality"`
}

// EncodeDiscoverPayload builds the versioned envelope for scheduling.
func EncodeDiscoverPayload(snapshot, cnpj, uf, municipality string) ([]byte, error) {
	return json.Marshal(DiscoverPayload{Version: 1, Snapshot: snapshot, CNPJ: cnpj, UF: uf, Municipality: municipality})
}

// DiscoverDedupeKey converges duplicate enqueues for one snapshot.
func DiscoverDedupeKey(snapshot string) string {
	return "registry-discover:" + snapshot
}

// Handle implements jobs.Handler.
func (d Discover) Handle(ctx context.Context, job jobs.Job) error {
	var payload DiscoverPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("discover: bad payload: %w", err)
	}
	if payload.Version != 1 {
		return fmt.Errorf("discover: unsupported payload version %d", payload.Version)
	}
	if payload.Snapshot == "" {
		return errors.New("discover: snapshot is required")
	}
	if d.Store == nil {
		return errors.New("discover: store is required")
	}
	cfg := d.Config
	cfg.CNPJ, cfg.UF, cfg.Municipality = payload.CNPJ, payload.UF, payload.Municipality
	report, err := registry.Discover(ctx, d.Store, payload.Snapshot, cfg)
	if err != nil {
		return err
	}
	switch report.State {
	case "complete", "quarantined":
		return nil
	default:
		return fmt.Errorf("discover: snapshot %q finished %s (%s)", payload.Snapshot, report.State, report.ErrorCode)
	}
}
