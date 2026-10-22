package jobs

import (
	"context"
	"encoding/json"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

const EnforcementDiscoveryKind = "quality-enforcement-discovery"

type EnforcementDiscovery struct {
	Pool    *pgxpool.Pool
	List    func(context.Context) error
	Inspect func(context.Context) (application.InventoryEntry, error)
	Enqueue func(context.Context, ImportPayload, string) (platformjobs.EnqueueResult, error)
	Serial  func(context.Context, func(context.Context) error) error
}

func (EnforcementDiscovery) Kind() string { return EnforcementDiscoveryKind }
func (EnforcementDiscovery) Version() int { return 1 }
func (d EnforcementDiscovery) Handle(ctx context.Context, _ platformjobs.Job) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if d.Pool == nil || d.List == nil || d.Inspect == nil || d.Enqueue == nil || d.Serial == nil {
		return enforcementImportError("discovery ports unavailable")
	}
	return d.Serial(ctx, func(ctx context.Context) error {
		if d.List(ctx) != nil {
			return enforcementImportError("approved listing unavailable")
		}
		entry, e := d.Inspect(ctx)
		if e != nil {
			return enforcementImportError("resource inspection unavailable")
		}
		p := ImportPayload{Version: 1, Source: "enforcement", Stream: "enforcement-v1", Edition: application.EnforcementSourceEdition, URL: entry.URL, SHA256: entry.SHA256, Member: entry.Member}
		raw, e := json.Marshal(p)
		if e != nil || entry.Label != p.Edition || entry.Bytes == 0 || entry.Bytes > uint64(prepared.MaxBytes) {
			return enforcementImportError("resource identity refused")
		}
		if _, e = decodeEnforcementPayload(raw); e != nil {
			return e
		}
		recorded, e := recordedEditions(ctx, quality.New(d.Pool), p.Source, p.Stream)
		if e != nil {
			return enforcementImportError("edition ledger unavailable")
		}
		for _, plan := range application.PlanBackfill([]application.InventoryEntry{entry}, recorded) {
			if plan.Action != application.PlanImportNew && plan.Action != application.PlanCorrect {
				continue
			}
			if _, e = d.Enqueue(ctx, p, discoveryDedupe(p.Source, p.Stream, p.Edition, entry)); e != nil {
				return enforcementImportError("enqueue acknowledgement unavailable")
			}
		}
		return nil
	})
}
