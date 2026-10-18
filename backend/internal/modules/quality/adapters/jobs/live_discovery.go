package jobs

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// LiveDiscovery plans bounded chunks from the live official PMQC listing.
// Every run fetches the verified page, inspects exactly one listed archive
// (bounds and member enumeration live behind Inspect), plans its monthly
// members against the ledger and enqueues one import job per new or
// corrected edition. A durable cursor walks the listing archive by archive;
// converged enqueues never charge the budget, so a bounded run can never
// starve later members, and the cursor only advances once the inspected
// archive was fully planned. No listing entry with an unpublished href is
// ever invented.
type LiveDiscovery struct {
	Pool    *pgxpool.Pool
	Source  string
	Stream  string
	List    func(ctx context.Context) ([]native.ArchiveRef, error)
	Inspect func(ctx context.Context, ref native.ArchiveRef) ([]application.InventoryEntry, error)
	Enqueue func(ctx context.Context, payload ImportPayload, dedupe string) (platformjobs.EnqueueResult, error)
	// MaxEnqueues bounds created jobs per run; it must exceed the members of
	// any real archive so one inspection completes, and exists only to keep a
	// pathological listing finite.
	MaxEnqueues int
}

func (LiveDiscovery) Kind() string { return "quality-discovery" }

// Version implements jobs.Handler.
func (LiveDiscovery) Version() int { return 1 }

func (d LiveDiscovery) maxEnqueues() int {
	if d.MaxEnqueues <= 0 {
		return 16
	}
	return d.MaxEnqueues
}

func liveDiscoveryError(code string) error { return errors.New("live discovery: " + code) }

// Handle inspects one archive after the durable cursor and enqueues its
// planned members. The cursor advances only after the whole inspected
// archive was planned; an archive whose planned imports exceeded the
// defensive cap is re-inspected next run, where already-created jobs
// converge and no longer charge the budget.
func (d LiveDiscovery) Handle(ctx context.Context, _ platformjobs.Job) error {
	if d.Pool == nil || d.List == nil || d.Inspect == nil || d.Enqueue == nil || d.Source == "" || d.Stream == "" {
		return liveDiscoveryError("ports unavailable")
	}
	refs, err := d.List(ctx)
	if err != nil {
		return err
	}
	refs, err = uniqueRefs(refs)
	if err != nil {
		return err
	}
	q := quality.New(d.Pool)
	next, err := d.nextRef(ctx, q, refs)
	if err != nil {
		return err
	}
	entries, err := d.Inspect(ctx, next)
	if err != nil {
		return err
	}
	recorded, err := recordedEditions(ctx, q, d.Source, d.Stream)
	if err != nil {
		return err
	}
	enqueued := 0
	held := false
	for _, plan := range application.PlanBackfill(entries, recorded) {
		if plan.Action != application.PlanImportNew && plan.Action != application.PlanCorrect {
			continue
		}
		if enqueued >= d.maxEnqueues() {
			held = true
			break
		}
		entry := findEntry(entries, plan.Label)
		result, eerr := d.Enqueue(ctx, ImportPayload{
			Version: 1, Source: d.Source, Stream: d.Stream,
			Edition: plan.Label, URL: entry.URL, SHA256: entry.SHA256, Member: entry.Member,
		}, discoveryDedupe(d.Source, d.Stream, plan.Label, entry))
		if eerr != nil {
			return eerr
		}
		if result.Created {
			enqueued++
		}
	}
	if held {
		return nil
	}
	return q.SetDiscoveryCursor(ctx, quality.SetDiscoveryCursorParams{Kind: d.Kind(), CursorUrl: next.Cursor()})
}

// nextRef picks the first listed archive after the durable cursor, wrapping
// to the first once the whole listing was covered.
func (d LiveDiscovery) nextRef(ctx context.Context, q *quality.Queries, refs []native.ArchiveRef) (native.ArchiveRef, error) {
	cursor, err := q.GetDiscoveryCursor(ctx, d.Kind())
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return native.ArchiveRef{}, err
	}
	if err == nil && cursor.CursorUrl != "" {
		for _, ref := range refs {
			if ref.Cursor() > cursor.CursorUrl {
				return ref, nil
			}
		}
	}
	return refs[0], nil
}

// uniqueRefs re-checks verified URL uniqueness so a page assembled from
// several sources can never duplicate one archive in a single run.
func uniqueRefs(refs []native.ArchiveRef) ([]native.ArchiveRef, error) {
	if len(refs) == 0 {
		return nil, liveDiscoveryError("listing empty")
	}
	seen := map[string]bool{}
	out := make([]native.ArchiveRef, 0, len(refs))
	for _, ref := range refs {
		if native.ValidateURL(ref.URL) != nil || seen[ref.URL] {
			return nil, liveDiscoveryError("listing reference refused")
		}
		seen[ref.URL] = true
		out = append(out, ref)
	}
	return out, nil
}

func discoveryDedupe(source, stream, label string, entry application.InventoryEntry) string {
	return strings.Join([]string{source, stream, label, application.ResourceIdentity(entry.SHA256, entry.Member)}, "\x1f")
}
