package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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

// BeginRun records an import run and its staging revision, or reports a
// no-op when the identical bytes plus parser version already completed.
// A failed earlier run never blocks a retry: only completed triples match.
func (im *Importer) BeginRun(ctx context.Context, key domain.ImportKey, surveyStart, surveyEnd time.Time) (BeginResult, error) {
	q := official.New(im.pool)
	if existing, err := q.GetImportRunByIdentity(ctx, official.GetImportRunByIdentityParams{
		SourceUrl:      key.SourceURL,
		SourceChecksum: key.SourceChecksum,
		ParserVersion:  key.ParserVersion,
	}); err == nil {
		if existing.Status == domain.RunCompleted {
			return BeginResult{NoOp: true, ExistingRevision: ""}, nil
		}
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
		return BeginResult{}, err
	}
	revText, err := newUUIDv4()
	if err != nil {
		return BeginResult{}, err
	}
	revID, err := mustUUID(revText)
	if err != nil {
		return BeginResult{}, err
	}
	// A corrected import supersedes the latest published revision when one
	// exists; the first import stands alone.
	var supersedes pgtype.UUID
	if latest, err := q.GetLatestPublishedRevision(ctx); err == nil {
		supersedes = latest.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return BeginResult{}, err
	}
	if _, err := q.CreateRevision(ctx, official.CreateRevisionParams{
		ID:                   revID,
		ImportRunID:          runID,
		SurveyStart:          pgDate(surveyStart),
		SurveyEnd:            pgDate(surveyEnd),
		SupersedesRevisionID: supersedes,
	}); err != nil {
		return BeginResult{}, err
	}
	return BeginResult{
		RunID: runText, RevisionID: revText,
		SurveyStart: surveyStart, SurveyEnd: surveyEnd,
	}, nil
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
	}
	if !hasPrev {
		if latest, err := tq.GetLatestPublishedRevision(ctx); err == nil {
			if n, err := tq.CountRevisionRows(ctx, latest.ID); err == nil {
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
