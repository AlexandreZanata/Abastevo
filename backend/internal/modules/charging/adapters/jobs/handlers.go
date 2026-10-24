package jobs

import (
	"context"
	"encoding/json"

	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// ImportHandler runs verified charging imports via the queue with leases,
// retry and dead budgets. Schedules stay off; operators enqueue manually
// and retain the last accepted state on any failure.
type ImportHandler struct {
	Runner Runner
}

func (ImportHandler) Kind() string { return ChargingImportKind }
func (ImportHandler) Version() int { return 1 }

func (h ImportHandler) Handle(ctx context.Context, job platformjobs.Job) error {
	if h.Runner.Store == nil {
		return importError("store unavailable")
	}
	var payload ImportPayload
	if len(job.Payload) > 4096 || json.Unmarshal(job.Payload, &payload) != nil {
		return importError("invalid payload")
	}
	// Merge/split review runs before admission: compare incoming place
	// IDs with stored ones and refuse auto-linking on divergence. The
	// runner stores; review flags ride the result (never auto-bind).
	_, err := h.Runner.Run(ctx, payload)
	return err
}

// DiscoveryHandler checks allowlisted sources for new revisions without
// downloading. It never auto-imports, auto-closes or auto-binds.
type DiscoveryHandler struct{}

func (DiscoveryHandler) Kind() string { return ChargingDiscoveryKind }
func (DiscoveryHandler) Version() int { return 1 }

func (DiscoveryHandler) Handle(ctx context.Context, job platformjobs.Job) error {
	var request struct {
		Version int    `json:"version"`
		URL     string `json:"url"`
	}
	if len(job.Payload) > 4096 || json.Unmarshal(job.Payload, &request) != nil || request.Version != 1 {
		return importError("invalid payload")
	}
	_, err := Discover(ctx, request.URL)
	return err
}
