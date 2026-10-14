package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// Publisher stages and publishes quality assay revisions.
type Publisher struct {
	pool *pgxpool.Pool
}

// NewPublisher wires the owned generated queries to a pool.
func NewPublisher(pool *pgxpool.Pool) *Publisher {
	return &Publisher{pool: pool}
}

// editionRequest is the shared edition/manifest envelope for assay and
// event publications. Row staging differs per stream and arrives as stage.
type editionRequest struct {
	Source       string
	Stream       string
	Edition      string
	Predecessor  string
	Resources    []application.Resource
	Counts       domain.Counts
	ManifestHash string
}

// stageFunc persists one revision's rows and asserts the staged counts.
// Any error rolls the whole publication back.
type stageFunc func(q *quality.Queries, editionID, revisionID pgtype.UUID) error

func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32], nil
}

func mustUUID(text string) (pgtype.UUID, error) {
	clean := strings.ReplaceAll(text, "-", "")
	raw, err := hex.DecodeString(clean)
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("adapters: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func nullUUID() pgtype.UUID {
	return pgtype.UUID{}
}

func uuidOrNull(text string) (pgtype.UUID, error) {
	if strings.TrimSpace(text) == "" {
		return nullUUID(), nil
	}
	return mustUUID(text)
}

func pgDate(layout string) (pgtype.Date, error) {
	parsed, err := time.Parse("2006-01-02", layout)
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: parsed, Valid: true}, nil
}

func pgText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// Publish validates one manifest plus staged rows and publishes exactly
// one revision atomically: edition, resources, manifest, staging rows,
// supersession of the previous published revision and the current-pointer
// switch commit together or not at all. Replaying an identical manifest
// returns the existing published revision without new rows; a concurrent
// loser of the published-per-edition index converges to the winner.
func (p *Publisher) Publish(ctx context.Context, in application.Input, newID func() string) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	req := editionRequest{
		Source: in.Source, Stream: in.Stream, Edition: in.Edition,
		Predecessor: in.Predecessor, Resources: in.Resources,
		Counts: in.Counts, ManifestHash: in.ManifestHash,
	}
	stage := func(q *quality.Queries, editionID, revisionID pgtype.UUID) error {
		for _, a := range in.Assays {
			observed, derr := pgDate(a.ObservedOn)
			if derr != nil {
				return derr
			}
			rowID, rerr := mustUUID(newID())
			if rerr != nil {
				return rerr
			}
			if _, serr := q.StageAssay(ctx, quality.StageAssayParams{
				ID: rowID, RevisionID: revisionID,
				SampleRef: a.SampleRef, AssayRef: a.AssayRef,
				SourceCnpj: a.SourceCNPJ, ObservedOn: observed,
				Fuel: a.Fuel, ResultText: a.ResultText, Unit: a.Unit,
				Conformity: a.Conformity, EvidenceRef: a.EvidenceRef,
				Fingerprint: a.Fingerprint,
			}); serr != nil {
				return serr
			}
		}
		staged, cerr := q.CountRevisionAssays(ctx, revisionID)
		if cerr != nil {
			return cerr
		}
		if staged != int64(len(in.Assays)) {
			return errors.New("adapters: staged assay count diverged; rolled back")
		}
		for _, l := range in.Links {
			station, serr := uuidOrNull(l.StationID)
			if serr != nil {
				return serr
			}
			linkID, rerr := mustUUID(newID())
			if rerr != nil {
				return rerr
			}
			if _, serr := q.StageLink(ctx, quality.StageLinkParams{
				ID: linkID, RevisionID: revisionID, RecordKey: l.RecordKey,
				StationID: station, Automatic: l.Automatic,
				Reason: l.Reason, Active: l.Active,
			}); serr != nil {
				return serr
			}
		}
		return nil
	}
	// A concurrent publisher may win the edition key between our lookup
	// and insert; the loser retries once into the replay/correction path
	// and converges to the winner instead of surfacing a conflict.
	for attempt := 0; ; attempt++ {
		rev, retry, err := p.publishTx(ctx, req, newID, stage)
		if !retry || attempt >= 2 {
			return rev, err
		}
	}
}

func (p *Publisher) publishTx(ctx context.Context, req editionRequest, newID func() string, stage stageFunc) (string, bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	q := quality.New(p.pool).WithTx(tx)

	editionID, err := mustUUID(newID())
	if err != nil {
		return "", false, err
	}
	revisionID, err := mustUUID(newID())
	if err != nil {
		return "", false, err
	}
	existing, err := q.GetEditionByKey(ctx, quality.GetEditionByKeyParams{
		Source: req.Source, Stream: req.Stream, Edition: req.Edition,
	})
	if err == nil {
		editionID = existing.ID
		manifest, merr := q.GetManifest(ctx, existing.ID)
		if merr != nil {
			return "", false, merr
		}
		if manifest.ManifestHash == req.ManifestHash {
			published, perr := q.GetPublishedRevisionByEdition(ctx, existing.ID)
			if perr != nil {
				return "", false, perr
			}
			if cerr := tx.Commit(ctx); cerr != nil {
				return "", false, cerr
			}
			committed = true
			return uuidString(published.ID), false, nil
		}
		if uerr := q.UpdateManifest(ctx, quality.UpdateManifestParams{
			EditionID:    existing.ID,
			Input:        int64(req.Counts.Input),
			Accepted:     int64(req.Counts.Accepted),
			Duplicates:   int64(req.Counts.Duplicates),
			Quarantined:  int64(req.Counts.Quarantined),
			ManifestHash: req.ManifestHash,
		}); uerr != nil {
			return "", false, uerr
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		if _, cerr := q.CreateEdition(ctx, quality.CreateEditionParams{
			ID: editionID, Source: req.Source, Stream: req.Stream,
			Edition: req.Edition, Predecessor: pgText(req.Predecessor),
		}); cerr != nil {
			if isEditionConflict(cerr) {
				return "", true, nil
			}
			return "", false, cerr
		}
		for _, r := range req.Resources {
			if rerr := q.AddResource(ctx, quality.AddResourceParams{
				EditionID: editionID, Url: r.URL, Sha256: r.SHA256,
				Bytes: int64(r.Bytes),
			}); rerr != nil {
				return "", false, rerr
			}
		}
		if _, merr := q.RecordManifest(ctx, quality.RecordManifestParams{
			EditionID:    editionID,
			Input:        int64(req.Counts.Input),
			Accepted:     int64(req.Counts.Accepted),
			Duplicates:   int64(req.Counts.Duplicates),
			Quarantined:  int64(req.Counts.Quarantined),
			ManifestHash: req.ManifestHash,
		}); merr != nil {
			return "", false, merr
		}
	} else {
		return "", false, err
	}

	var supersedes pgtype.UUID
	if prev, perr := q.GetPublishedRevisionByEdition(ctx, editionID); perr == nil {
		supersedes = prev.ID
	} else if !errors.Is(perr, pgx.ErrNoRows) {
		return "", false, perr
	}
	if _, cerr := q.CreateRevision(ctx, quality.CreateRevisionParams{
		ID: revisionID, EditionID: editionID, SupersedesRevisionID: supersedes,
	}); cerr != nil {
		return "", false, cerr
	}
	if serr := stage(q, editionID, revisionID); serr != nil {
		return "", false, serr
	}
	if supersedes.Valid {
		if serr := q.SetRevisionSuperseded(ctx, supersedes); serr != nil {
			return "", false, serr
		}
	}
	if perr := q.SetRevisionPublished(ctx, revisionID); perr != nil {
		if isPublishedConflict(perr) {
			// A concurrent publisher won: converge to its revision.
			_ = tx.Rollback(ctx)
			committed = true
			ro := quality.New(p.pool)
			edition, gerr := ro.GetEditionByKey(ctx, quality.GetEditionByKeyParams{
				Source: req.Source, Stream: req.Stream, Edition: req.Edition,
			})
			if gerr != nil {
				return "", false, gerr
			}
			published, gerr := ro.GetPublishedRevisionByEdition(ctx, edition.ID)
			if gerr != nil {
				return "", false, gerr
			}
			return uuidString(published.ID), false, nil
		}
		return "", false, perr
	}
	if serr := q.SetCurrentRevision(ctx, quality.SetCurrentRevisionParams{
		Source: req.Source, Stream: req.Stream, RevisionID: revisionID,
	}); serr != nil {
		return "", false, serr
	}
	if cerr := tx.Commit(ctx); cerr != nil {
		return "", false, cerr
	}
	committed = true
	return uuidString(revisionID), false, nil
}

func isPublishedConflict(err error) bool {
	return strings.Contains(err.Error(), "quality_revisions_one_published_per_edition")
}

func isEditionConflict(err error) bool {
	return strings.Contains(err.Error(), "quality_editions_source_stream_edition_key")
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}
