package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	official "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/official"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
)

// Importer stages and publishes official revisions.
type Importer struct {
	pool *pgxpool.Pool
}

// NewImporter wires the owned generated queries to a pool.
func NewImporter(pool *pgxpool.Pool) *Importer {
	return &Importer{pool: pool}
}

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

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func pgDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

// BeginResult opens one import attempt.
type BeginResult struct {
	RunID            string
	RevisionID       string
	NoOp             bool
	ExistingRevision string
	SurveyStart      time.Time
	SurveyEnd        time.Time
}

// BeginRun records an import run and its staging revision atomically, or
// converges on an already attempted identical triple instead of crashing:
//   - completed: no-op, the accepted revision stands;
//   - failed (terminal verdict recorded): no-op, the recorded outcome stands;
//   - running: resume or adopt the same run: an orphaned run (no revision,
//     e.g. interrupted between the two inserts before this transaction
//     existed) gains its staging revision here, validated against the
//     persisted published state, never just the retry parameters.
//
// Run and revision are created in one transaction fenced by an advisory
// lock on the import identity, so concurrent identical begins converge on
// a single run+revision pair and a cancelled attempt persists nothing.
// Restaging is idempotent; a concurrent duplicate converges on the same
// revision and its FinishRun fails visibly once the first commits.
//
// A failed earlier run never blocks a retry: only new bytes (or a new parser
// version) open a new attempt.
func (im *Importer) BeginRun(ctx context.Context, key domain.ImportKey, surveyStart, surveyEnd time.Time) (BeginResult, error) {
	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return BeginResult{}, err
	}
	// Rollback is a no-op after a successful commit. Every error path
	// below leaves no partial run behind.
	defer func() { _ = tx.Rollback(ctx) }()
	q := official.New(tx)
	identity := key.SourceURL + "\n" + key.SourceChecksum + "\n" + key.ParserVersion
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", identity); err != nil {
		return BeginResult{}, err
	}
	if existing, err := q.GetImportRunByIdentity(ctx, official.GetImportRunByIdentityParams{
		SourceUrl:      key.SourceURL,
		SourceChecksum: key.SourceChecksum,
		ParserVersion:  key.ParserVersion,
	}); err == nil {
		res, err := convergeImportRun(ctx, q, key, existing, surveyStart, surveyEnd)
		if err != nil {
			return BeginResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return BeginResult{}, err
		}
		return res, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return BeginResult{}, err
	}
	runText, err := newUUIDv4()
	if err != nil {
		return BeginResult{}, err
	}
	runID, err := mustUUID(runText)
	if err != nil {
		return BeginResult{}, err
	}
	if _, err := q.CreateImportRun(ctx, official.CreateImportRunParams{
		ID:             runID,
		SourceUrl:      key.SourceURL,
		SourceChecksum: key.SourceChecksum,
		ParserVersion:  key.ParserVersion,
	}); err != nil {
		// Lost the race with an identical concurrent attempt: re-read the
		// winner and converge instead of crashing on the identity.
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			return BeginResult{}, err
		}
		existing, rerr := q.GetImportRunByIdentity(ctx, official.GetImportRunByIdentityParams{
			SourceUrl:      key.SourceURL,
			SourceChecksum: key.SourceChecksum,
			ParserVersion:  key.ParserVersion,
		})
		if rerr != nil {
			return BeginResult{}, err
		}
		res, rerr := convergeImportRun(ctx, q, key, existing, surveyStart, surveyEnd)
		if rerr != nil {
			return BeginResult{}, rerr
		}
		if err := tx.Commit(ctx); err != nil {
			return BeginResult{}, err
		}
		return res, nil
	}
	revText, err := openStagingRevision(ctx, q, runID, surveyStart, surveyEnd)
	if err != nil {
		return BeginResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BeginResult{}, err
	}
	return BeginResult{
		RunID: runText, RevisionID: revText,
		SurveyStart: surveyStart, SurveyEnd: surveyEnd,
	}, nil
}

// openStagingRevision creates the staging revision for a run, binding its
// supersession edge to the persisted published state — never just to the
// retry parameters. A corrected import supersedes the latest published
// revision for the same survey window; an older-week correction never
// supersedes a newer week merely because it was published later.
func openStagingRevision(ctx context.Context, q *official.Queries, runID pgtype.UUID, surveyStart, surveyEnd time.Time) (string, error) {
	revText, err := newUUIDv4()
	if err != nil {
		return "", err
	}
	revID, err := mustUUID(revText)
	if err != nil {
		return "", err
	}
	var supersedes pgtype.UUID
	if latest, err := q.GetLatestPublishedRevisionForWindow(ctx, official.GetLatestPublishedRevisionForWindowParams{
		SurveyStart: pgDate(surveyStart),
		SurveyEnd:   pgDate(surveyEnd),
	}); err == nil {
		supersedes = latest.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if _, err := q.CreateRevision(ctx, official.CreateRevisionParams{
		ID:                   revID,
		ImportRunID:          runID,
		SurveyStart:          pgDate(surveyStart),
		SurveyEnd:            pgDate(surveyEnd),
		SupersedesRevisionID: supersedes,
	}); err != nil {
		return "", err
	}
	return revText, nil
}

// convergeImportRun resolves an already recorded identical import triple to
// a no-op (terminal verdict recorded) or to a resume of its staging run. A
// running attempt without a revision — an orphan left by an interruption
// between the two inserts — is adopted here: the missing staging revision
// is created with its supersession edge validated against the persisted
// published state, so an incomplete state is never silently completed.
func convergeImportRun(ctx context.Context, q *official.Queries, key domain.ImportKey, existing official.GetImportRunByIdentityRow, surveyStart, surveyEnd time.Time) (BeginResult, error) {
	switch existing.Status {
	case domain.RunCompleted, domain.RunFailed:
		return BeginResult{NoOp: true, ExistingRevision: ""}, nil
	default:
		rev, err := q.GetRevisionByRun(ctx, existing.ID)
		if err == nil {
			if rev.Status != domain.RevisionStaging {
				return BeginResult{}, fmt.Errorf("adapters: import of %s already %s", key.SourceURL, rev.Status)
			}
			return BeginResult{
				RunID: uuidString(existing.ID), RevisionID: uuidString(rev.ID),
				SurveyStart: surveyStart, SurveyEnd: surveyEnd,
			}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return BeginResult{}, err
		}
		revText, err := openStagingRevision(ctx, q, existing.ID, surveyStart, surveyEnd)
		if err != nil {
			return BeginResult{}, err
		}
		return BeginResult{
			RunID: uuidString(existing.ID), RevisionID: revText,
			SurveyStart: surveyStart, SurveyEnd: surveyEnd,
		}, nil
	}
}

// StageBatch inserts one batch of validated rows, counting staged rows and
// quarantining conflicting duplicates instead of overwriting. Restaging
// identical rows is a no-op. Tallies accumulate in q for the finish verdict.
func (im *Importer) StageBatch(ctx context.Context, revisionID string, rows []domain.PriceRow, q *domain.QuarantineTally) (int64, error) {
	revUUID, err := mustUUID(revisionID)
	if err != nil {
		return 0, domain.ErrUnknownRevision
	}
	qq := official.New(im.pool)
	var staged int64
	for _, row := range rows {
		stationUUID, err := mustUUID(row.StationID)
		if err != nil {
			return staged, err
		}
		conflicts, err := qq.FindConflictingStationPrice(ctx, official.FindConflictingStationPriceParams{
			RevisionID:     revUUID,
			StationID:      stationUUID,
			FuelProduct:    row.Product,
			Unit:           row.Unit,
			CollectedOn:    pgDate(row.CollectedOn),
			SourceRow:      int32(row.SourceRow),
			AmountMilliBrl: row.AmountMilli,
		})
		if err != nil {
			return staged, err
		}
		if len(conflicts) > 0 {
			q.Add("conflicting-duplicate", row.StationID+"/"+row.Product)
			continue
		}
		idText, err := newUUIDv4()
		if err != nil {
			return staged, err
		}
		id, err := mustUUID(idText)
		if err != nil {
			return staged, err
		}
		if _, err := qq.StageStationPrice(ctx, official.StageStationPriceParams{
			ID:             id,
			RevisionID:     revUUID,
			StationID:      stationUUID,
			FuelProduct:    row.Product,
			Unit:           row.Unit,
			AmountMilliBrl: row.AmountMilli,
			RawPriceText:   row.RawText,
			CollectedOn:    pgDate(row.CollectedOn),
			SourceRow:      int32(row.SourceRow),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Identical restage suppressed by the row identity.
				staged++
				continue
			}
			return staged, err
		}
		staged++
	}
	return staged, nil
}

// FinishResult reports the publication outcome.
type FinishResult struct {
	Verdict    domain.Verdict
	RevisionID string
	Published  bool
}

// FinishRun validates staged counts against the review gate and, on
// publish, marks the run and revision and switches the per-survey pointer
// in a single transaction. Anything else leaves the pointer untouched, so
// the previous revision stays readable and a retry is always safe.
func (im *Importer) FinishRun(ctx context.Context, runID, revisionID string, q *domain.QuarantineTally) (FinishResult, error) {
	revUUID, err := mustUUID(revisionID)
	if err != nil {
		return FinishResult{}, domain.ErrUnknownRevision
	}
	runUUID, err := mustUUID(runID)
	if err != nil {
		return FinishResult{}, err
	}
	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return FinishResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := official.New(tx)
	rev, err := tq.GetRevision(ctx, revUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FinishResult{}, domain.ErrUnknownRevision
		}
		return FinishResult{}, err
	}
	if rev.Status != domain.RevisionStaging {
		return FinishResult{}, errors.New("adapters: revision not staging")
	}
	staged, err := tq.CountRevisionRows(ctx, revUUID)
	if err != nil {
		return FinishResult{}, err
	}
	var prevTotal int64
	hasPrev := false
	if rev.SupersedesRevisionID.Valid {
		if prev, err := tq.GetRevision(ctx, rev.SupersedesRevisionID); err == nil && prev.Status == domain.RevisionPublished {
			if n, err := tq.CountRevisionRows(ctx, rev.SupersedesRevisionID); err == nil {
				prevTotal, hasPrev = n, true
			}
		}
	} else {
		// First publication for this window: compare against the previous
		// chronological week, not whatever happened to publish most recently.
		if prev, err := tq.GetLatestPublishedRevisionBeforeWindow(ctx, rev.SurveyStart); err == nil {
			if n, err := tq.CountRevisionRows(ctx, prev.ID); err == nil {
				prevTotal, hasPrev = n, true
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return FinishResult{}, err
		}
	}
	verdict := domain.ReviewGate(staged, q.Total(), prevTotal, hasPrev)
	counts, _ := json.Marshal(map[string]int64{"staged": staged, "quarantined": q.Total()})
	summary, _ := json.Marshal(map[string]any{"reasons": q.Counts, "samples": q.Samples})
	runStatus := domain.RunCompleted
	revStatus := domain.RevisionPublished
	if !verdict.Publish {
		runStatus = domain.RunFailed
		revStatus = domain.RevisionFailed
		if verdict.Reason == "quarantine-share" || verdict.Reason == "row-drop" {
			revStatus = domain.RevisionNeedsReview
		}
	}
	if err := tq.FinishImportRun(ctx, official.FinishImportRunParams{
		ID:           runUUID,
		Status:       runStatus,
		RowCounts:    counts,
		ErrorSummary: summary,
	}); err != nil {
		return FinishResult{}, err
	}
	if verdict.Publish {
		// A stale competitor must not repoint the window: only the
		// revision built against the still-current state switches the
		// pointer. Anything newer already published retires this revision
		// as superseded, leaving published facts untouched.
		currentPtr, err := tq.GetCurrentPointer(ctx, official.GetCurrentPointerParams{
			SurveyStart: rev.SurveyStart,
			SurveyEnd:   rev.SurveyEnd,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return FinishResult{}, err
		}
		if err == nil && currentPtr != rev.SupersedesRevisionID {
			if err := tq.SetRevisionStatus(ctx, official.SetRevisionStatusParams{
				ID:     revUUID,
				Status: domain.RevisionSuperseded,
			}); err != nil {
				return FinishResult{}, err
			}
			if err := tx.Commit(ctx); err != nil {
				return FinishResult{}, err
			}
			return FinishResult{Verdict: verdict, RevisionID: revisionID, Published: false}, nil
		}
		if err := tq.SetRevisionPublished(ctx, revUUID); err != nil {
			return FinishResult{}, err
		}
		if err := tq.UpsertCurrentPointer(ctx, official.UpsertCurrentPointerParams{
			SurveyStart: rev.SurveyStart,
			SurveyEnd:   rev.SurveyEnd,
			RevisionID:  revUUID,
		}); err != nil {
			return FinishResult{}, err
		}
	} else {
		if err := tq.SetRevisionStatus(ctx, official.SetRevisionStatusParams{
			ID:     revUUID,
			Status: revStatus,
		}); err != nil {
			return FinishResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return FinishResult{}, err
	}
	return FinishResult{Verdict: verdict, RevisionID: revisionID, Published: verdict.Publish}, nil
}

// CurrentRevision returns the published revision for a survey window, or
// ErrUnknownRevision when nothing was ever published for it.
func (im *Importer) CurrentRevision(ctx context.Context, surveyStart, surveyEnd time.Time) (string, error) {
	rev, err := official.New(im.pool).GetCurrentPointer(ctx, official.GetCurrentPointerParams{
		SurveyStart: pgDate(surveyStart),
		SurveyEnd:   pgDate(surveyEnd),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrUnknownRevision
		}
		return "", err
	}
	return uuidString(rev), nil
}
