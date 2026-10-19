package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const NativeFineDiscoveryKind = "quality-sanctions-discovery"

// FineDiscovery inspects at most one listed CSV per run. The persistent cursor
// and enqueue acknowledgment share the native source lease. A crash before the
// cursor update safely retries the same deduplicated job, including dead jobs;
// it never replenishes their attempt budget.
type FineDiscovery struct {
	Pool    *pgxpool.Pool
	List    func(context.Context) ([]native.FineRef, error)
	Inspect func(context.Context, native.FineRef) (application.InventoryEntry, error)
	Enqueue func(context.Context, ImportPayload, string) (platformjobs.EnqueueResult, error)
	Serial  func(context.Context, func(context.Context) error) error
}

func (FineDiscovery) Kind() string { return NativeFineDiscoveryKind }
func (FineDiscovery) Version() int { return 1 }

func (d FineDiscovery) Handle(ctx context.Context, _ platformjobs.Job) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if d.Pool == nil || d.List == nil || d.Inspect == nil || d.Enqueue == nil || d.Serial == nil {
		return fineImportError("discovery ports unavailable")
	}
	return d.Serial(ctx, func(ctx context.Context) error {
		refs, e := d.List(ctx)
		if e != nil || len(refs) == 0 || len(refs) > 1000 {
			return fineImportError("listing unavailable")
		}
		refs = append([]native.FineRef(nil), refs...)
		seen := map[string]bool{}
		for _, ref := range refs {
			if native.ValidateFineSelection(ref.URL, ref.Year) != nil || seen[ref.URL] {
				return fineImportError("listing reference refused")
			}
			seen[ref.URL] = true
		}
		sort.Slice(refs, func(i, j int) bool { return refs[i].URL < refs[j].URL })
		q := quality.New(d.Pool)
		cursor, e := q.GetDiscoveryCursor(ctx, d.Kind())
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return fineImportError("discovery cursor unavailable")
		}
		next := refs[0]
		for _, ref := range refs {
			if ref.URL > cursor.CursorUrl {
				next = ref
				break
			}
		}
		entry, e := d.Inspect(ctx, next)
		if e != nil {
			return fineImportError("resource inspection unavailable")
		}
		payload := ImportPayload{Version: 1, Source: "sanctions", Stream: "sanction-v1", Edition: next.Year, URL: entry.URL, SHA256: entry.SHA256, Member: entry.Member}
		raw, e := json.Marshal(payload)
		if e != nil || entry.URL != next.URL || entry.Label != next.Year || entry.Bytes == 0 || entry.Bytes > uint64(prepared.MaxBytes) {
			return fineImportError("inspected resource identity refused")
		}
		if _, e = decodeFinePayload(raw); e != nil {
			return e
		}
		recorded, e := recordedEditions(ctx, q, payload.Source, payload.Stream)
		if e != nil {
			return fineImportError("edition ledger unavailable")
		}
		for _, plan := range application.PlanBackfill([]application.InventoryEntry{entry}, recorded) {
			if plan.Action != application.PlanImportNew && plan.Action != application.PlanCorrect {
				continue
			}
			if _, e = d.Enqueue(ctx, payload, discoveryDedupe(payload.Source, payload.Stream, payload.Edition, entry)); e != nil {
				return fineImportError("import enqueue unavailable")
			}
		}
		if e = q.SetDiscoveryCursor(ctx, quality.SetDiscoveryCursorParams{Kind: d.Kind(), CursorUrl: next.URL}); e != nil {
			return fineImportError("discovery cursor acknowledgement unavailable")
		}
		return nil
	})
}
