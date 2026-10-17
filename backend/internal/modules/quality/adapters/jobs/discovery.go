package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// ImportPayload is the versioned import job envelope. The content hash
// pins the bytes the discovery planned; a mismatch fails the job.
type ImportPayload struct {
	Member  string `json:"member,omitempty"`
	Version int    `json:"version"`
	Source  string `json:"source"`
	Stream  string `json:"stream"`
	Edition string `json:"edition"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

// Discovery plans one bounded chunk of editions and enqueues imports.
// Listing and enqueueing are injected; production wires the ANP page
// lister and the platform queue. Guardrails: at most MaxEditions import
// jobs per run, oldest labels first, watermark cursor honored.
type Discovery struct {
	Source      string
	Stream      string
	Pool        *pgxpool.Pool
	List        func(ctx context.Context) ([]application.InventoryEntry, error)
	Enqueue     func(ctx context.Context, payload ImportPayload, dedupe string) (string, error)
	MaxEditions int
}

func (Discovery) Kind() string { return "quality-discovery" }

// Version implements jobs.Handler.
func (Discovery) Version() int { return 1 }

func (d Discovery) maxEditions() int {
	if d.MaxEditions <= 0 {
		return 4
	}
	return d.MaxEditions
}

// Handle plans from the observed inventory against recorded editions and
// enqueues one import job per new or corrected edition. Stable replays,
// aliases and missing resources enqueue nothing; the ledger itself is the
// resume state, so late or republished labels always flow while recorded
// content never double-imports. The watermark records completed imports
// on the Import side and stays untouched here.
func (d Discovery) Handle(ctx context.Context, job platformjobs.Job) error {
	if d.List == nil {
		return errors.New("discovery: no lister configured")
	}
	if d.Enqueue == nil {
		return errors.New("discovery: no enqueue configured")
	}
	entries, err := d.List(ctx)
	if err != nil {
		return err
	}
	q := quality.New(d.Pool)
	recorded := make([]application.RecordedEdition, 0, len(entries))
	hashes, err := q.ListStreamResourceHashes(ctx, quality.ListStreamResourceHashesParams{
		Source: d.Source, Stream: d.Stream,
	})
	if err != nil {
		return err
	}
	byEdition := map[string][]string{}
	byEditionMembers := map[string][]string{}
	for _, h := range hashes {
		byEdition[h.Edition] = append(byEdition[h.Edition], h.Sha256)
		byEditionMembers[h.Edition] = append(byEditionMembers[h.Edition], h.MemberName)
	}
	for _, e := range entries {
		rec := application.RecordedEdition{Label: e.Label}
		if edition, gerr := q.GetEditionByKey(ctx, quality.GetEditionByKeyParams{
			Source: d.Source, Stream: d.Stream, Edition: e.Label,
		}); gerr == nil {
			if manifest, merr := q.GetManifest(ctx, edition.ID); merr == nil {
				rec.ManifestHash = manifest.ManifestHash
			}
			rec.ResourceHashes = byEdition[e.Label]
			rec.ResourceMembers = byEditionMembers[e.Label]
		} else if !errors.Is(gerr, pgx.ErrNoRows) {
			return gerr
		}
		recorded = append(recorded, rec)
	}
	enqueued := 0
	for _, p := range application.PlanBackfill(entries, recorded) {
		if p.Action != application.PlanImportNew && p.Action != application.PlanCorrect {
			continue
		}
		if enqueued >= d.maxEditions() {
			break
		}
		entry := findEntry(entries, p.Label)
		if _, eerr := d.Enqueue(ctx, ImportPayload{
			Version: 1, Source: d.Source, Stream: d.Stream,
			Edition: p.Label, URL: entry.URL, SHA256: entry.SHA256, Member: entry.Member,
		}, strings.Join([]string{d.Source, d.Stream, p.Label, application.ResourceIdentity(entry.SHA256, entry.Member)}, "\x1f")); eerr != nil {
			return eerr
		}
		enqueued++
	}
	return nil
}

func findEntry(entries []application.InventoryEntry, label string) application.InventoryEntry {
	for _, e := range entries {
		if e.Label == label {
			return e
		}
	}
	return application.InventoryEntry{}
}

// PreparedBatch is one prepared edition ready to publish. Preparation
// itself is injected (Rust assay preparation today); the handler only
// pins bytes to plan and publishes atomically.
type PreparedBatch struct {
	Assays       []application.AssayInput
	Links        []application.LinkInput
	Enforcement  []application.EnforcementInput
	Sanctions    []application.SanctionInput
	Periods      []application.PeriodInput
	Counts       domain.Counts
	ManifestHash string
	MemberSHA256 string
	MemberBytes  uint64
	Guard        func(context.Context, pgx.Tx) error `json:"-"`
}

// Import fetches, prepares and publishes a single edition, then advances
// the watermark. Any failure before the atomic publish leaves watermark
// and publication untouched for retry.
type Import struct {
	Pool    *pgxpool.Pool
	Publish *adapters.Publisher
	Fetch   func(ctx context.Context, url string) ([]byte, error)
	Prepare func(raw []byte) (PreparedBatch, error)

	MaxBytes uint64
}

func (Import) Kind() string { return "quality-import" }

// Version implements jobs.Handler.
func (Import) Version() int { return 1 }

func (im Import) maxBytes() uint64 {
	if im.MaxBytes == 0 {
		return 100 << 20
	}
	return im.MaxBytes
}

// Handle publishes exactly the planned bytes: hash mismatch, oversize,
// preparation failure or publish failure all fail the job with nothing
// partially activated.
func (im Import) Handle(ctx context.Context, job platformjobs.Job) error {
	var payload ImportPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	if payload.Version != 1 || payload.Source == "" || payload.Stream == "" ||
		payload.Edition == "" || payload.URL == "" || payload.SHA256 == "" {
		return errors.New("import: malformed payload")
	}
	if im.Fetch == nil {
		return errors.New("import: no fetcher configured")
	}
	if im.Prepare == nil {
		return errors.New("import: no preparation configured")
	}
	raw, err := im.Fetch(ctx, payload.URL)
	if err != nil {
		return err
	}
	if uint64(len(raw)) > im.maxBytes() {
		return fmt.Errorf("import: %d bytes exceed %d budget", len(raw), im.maxBytes())
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, payload.SHA256) {
		return fmt.Errorf("import: content hash mismatch for %s", payload.Edition)
	}
	prepared, err := im.Prepare(raw)
	if err != nil {
		return err
	}
	return im.publishPrepared(ctx, payload, prepared, uint64(len(raw)))
}

func (im Import) publishPrepared(ctx context.Context, payload ImportPayload, prepared PreparedBatch, rawBytes uint64) error {
	publisher := im.Publish
	if prepared.Guard != nil {
		publisher = publisher.WithGuard(prepared.Guard)
	}
	q := quality.New(im.Pool)
	predecessor := ""
	if mark, gerr := q.GetWatermark(ctx, quality.GetWatermarkParams{
		Source: payload.Source, Stream: payload.Stream,
	}); gerr == nil {
		predecessor = mark.LastEdition
	} else if !errors.Is(gerr, pgx.ErrNoRows) {
		return gerr
	}
	resources := []application.Resource{{
		URL: payload.URL, SHA256: payload.SHA256, Bytes: rawBytes, Member: payload.Member, MemberSHA256: prepared.MemberSHA256, MemberBytes: prepared.MemberBytes,
	}}
	switch payload.Source {
	case "pmqc":
		if _, perr := publisher.Publish(ctx, application.Input{
			Source: payload.Source, Stream: payload.Stream, Edition: payload.Edition,
			Predecessor: predecessor, Resources: resources, Counts: prepared.Counts,
			ManifestHash: prepared.ManifestHash, Assays: prepared.Assays, Links: prepared.Links, Periods: prepared.Periods,
		}, platformjobs.NewUUIDv4); perr != nil {
			if errors.Is(perr, application.ErrSupersededPublication) {
				return nil
			}
			return perr
		}
	case "enforcement", "sanctions":
		if _, perr := publisher.PublishEvents(ctx, application.EventsInput{
			Source: payload.Source, Stream: payload.Stream, Edition: payload.Edition,
			Predecessor: predecessor, Resources: resources, Counts: prepared.Counts,
			ManifestHash: prepared.ManifestHash, Periods: prepared.Periods,
			Enforcement: prepared.Enforcement, Sanctions: prepared.Sanctions,
		}, platformjobs.NewUUIDv4); perr != nil {
			if errors.Is(perr, application.ErrSupersededPublication) {
				return nil
			}
			return perr
		}
	default:
		return fmt.Errorf("import: unknown source %q", payload.Source)
	}
	return q.SetWatermark(ctx, quality.SetWatermarkParams{
		Source: payload.Source, Stream: payload.Stream,
		LastEdition: payload.Edition, LastHash: prepared.ManifestHash,
	})
}
