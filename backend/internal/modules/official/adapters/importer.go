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

// lockSurveyWindow serializes every decision that can publish or repoint one
// survey window. A transaction-scoped advisory lock covers the first
// publication too: FOR UPDATE alone cannot lock a pointer row that does not
// exist yet. Begin, Stage and Finish all take this lock before the revision
// row, so their lock order has no cycle.
func lockSurveyWindow(ctx context.Context, tx pgx.Tx, surveyStart, surveyEnd time.Time) error {
	key := "official-window:" + pgDate(surveyStart).Time.Format("2006-01-02") + ":" + pgDate(surveyEnd).Time.Format("2006-01-02")
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", key)
	return err
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
		res, err := convergeImportRun(ctx, tx, q, key, existing, surveyStart, surveyEnd)
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
	created, err := q.CreateImportRunIfAbsent(ctx, official.CreateImportRunIfAbsentParams{
		ID:             runID,
		SourceUrl:      key.SourceURL,
		SourceChecksum: key.SourceChecksum,
		ParserVersion:  key.ParserVersion,
	})
	if err != nil {
		return BeginResult{}, err
	}
	if created == 0 {
		// The identity lock normally makes this unreachable, but an
		// ON CONFLICT no-op never aborts the transaction: re-read the
		// winner and converge on its persisted state.
		existing, rerr := q.GetImportRunByIdentity(ctx, official.GetImportRunByIdentityParams{
			SourceUrl:      key.SourceURL,
			SourceChecksum: key.SourceChecksum,
			ParserVersion:  key.ParserVersion,
		})
		if rerr != nil {
			return BeginResult{}, rerr
		}
		res, rerr := convergeImportRun(ctx, tx, q, key, existing, surveyStart, surveyEnd)
		if rerr != nil {
			return BeginResult{}, rerr
		}
		if err := tx.Commit(ctx); err != nil {
			return BeginResult{}, err
		}
		return res, nil
	}
	if err := lockSurveyWindow(ctx, tx, surveyStart, surveyEnd); err != nil {
		return BeginResult{}, err
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
func convergeImportRun(ctx context.Context, tx pgx.Tx, q *official.Queries, key domain.ImportKey, existing official.GetImportRunByIdentityRow, surveyStart, surveyEnd time.Time) (BeginResult, error) {
	switch existing.Status {
	case domain.RunCompleted, domain.RunFailed:
		return BeginResult{NoOp: true, ExistingRevision: ""}, nil
	default:
		rev, err := q.GetRevisionByRun(ctx, existing.ID)
		if err == nil {
			if err := requireSameSurveyWindow(rev, surveyStart, surveyEnd); err != nil {
				return BeginResult{}, err
			}
			if err := lockSurveyWindow(ctx, tx, rev.SurveyStart.Time, rev.SurveyEnd.Time); err != nil {
				return BeginResult{}, err
			}
			locked, err := q.LockRevision(ctx, rev.ID)
			if err != nil {
				return BeginResult{}, err
			}
			if locked.Status != domain.RevisionStaging {
				return BeginResult{}, fmt.Errorf("adapters: import of %s already %s", key.SourceURL, locked.Status)
			}
			return BeginResult{
				RunID: uuidString(existing.ID), RevisionID: uuidString(locked.ID),
				SurveyStart: locked.SurveyStart.Time, SurveyEnd: locked.SurveyEnd.Time,
			}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return BeginResult{}, err
		}
		if err := lockSurveyWindow(ctx, tx, surveyStart, surveyEnd); err != nil {
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

// requireSameSurveyWindow rejects a retry that reuses the import identity for
// a different period. The persisted revision owns the window; returning the
// caller's dates would silently stage facts under the wrong week.
func requireSameSurveyWindow(rev official.OfficialRevision, surveyStart, surveyEnd time.Time) error {
	start := pgDate(surveyStart)
	end := pgDate(surveyEnd)
	if rev.SurveyStart.Time.Format("2006-01-02") != start.Time.Format("2006-01-02") ||
		rev.SurveyEnd.Time.Format("2006-01-02") != end.Time.Format("2006-01-02") {
		return fmt.Errorf("adapters: import already has survey window %s..%s, requested %s..%s",
			rev.SurveyStart.Time.Format("2006-01-02"), rev.SurveyEnd.Time.Format("2006-01-02"),
			start.Time.Format("2006-01-02"), end.Time.Format("2006-01-02"))
	}
	return nil
}

// StageBatch inserts one batch of validated rows, counting staged rows and
// quarantining conflicting duplicates instead of overwriting. Restaging
// identical rows is a no-op. Tallies accumulate in q for the finish verdict.
func (im *Importer) StageBatch(ctx context.Context, revisionID string, rows []domain.PriceRow, q *domain.QuarantineTally) (int64, error) {
	revUUID, err := mustUUID(revisionID)
	if err != nil {
		return 0, domain.ErrUnknownRevision
	}
	head, err := official.New(im.pool).GetRevision(ctx, revUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrUnknownRevision
		}
		return 0, err
	}
	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSurveyWindow(ctx, tx, head.SurveyStart.Time, head.SurveyEnd.Time); err != nil {
		return 0, err
	}
	qq := official.New(tx)
	rev, err := qq.LockRevision(ctx, revUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrUnknownRevision
		}
		return 0, err
	}
	if rev.Status != domain.RevisionStaging {
		return 0, fmt.Errorf("%w: revision %s is %s", domain.ErrRevisionFrozen, revisionID, rev.Status)
	}
	// A batch either commits whole or leaves the revision untouched. Tallies
	// are merged only after commit so a rejected batch cannot inflate the
	// caller's review counts.
	var local domain.QuarantineTally
	var staged int64
	for _, row := range rows {
		stationUUID, err := mustUUID(row.StationID)
		if err != nil {
			return 0, err
		}
		existing, err := qq.GetRevisionRowBySourceRow(ctx, official.GetRevisionRowBySourceRowParams{
			RevisionID: revUUID,
			SourceRow:  int32(row.SourceRow),
		})
		if err == nil {
			if !sameStagedRow(existing, stationUUID, row) {
				return 0, fmt.Errorf("%w: revision %s source row %d", domain.ErrDivergentRestage, revisionID, row.SourceRow)
			}
			staged++
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
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
			return 0, err
		}
		if len(conflicts) > 0 {
			local.Add("conflicting-duplicate", row.StationID+"/"+row.Product)
			continue
		}
		idText, err := newUUIDv4()
		if err != nil {
			return 0, err
		}
		id, err := mustUUID(idText)
		if err != nil {
			return 0, err
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
			return 0, err
		}
		staged++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	mergeTally(q, local)
	return staged, nil
}

func sameStagedRow(existing official.OfficialStationPrice, station pgtype.UUID, row domain.PriceRow) bool {
	return existing.StationID == station &&
		existing.FuelProduct == row.Product &&
		existing.Unit == row.Unit &&
		existing.AmountMilliBrl == row.AmountMilli &&
		existing.RawPriceText == row.RawText &&
		existing.CollectedOn.Valid &&
		existing.CollectedOn.Time.Format("2006-01-02") == pgDate(row.CollectedOn).Time.Format("2006-01-02")
}

func mergeTally(dst *domain.QuarantineTally, src domain.QuarantineTally) {
	if len(src.Counts) > 0 {
		if dst.Counts == nil {
			dst.Counts = map[string]int64{}
		}
		for reason, count := range src.Counts {
			dst.Counts[reason] += count
		}
	}
	for _, sample := range src.Samples {
		if len(dst.Samples) >= 20 {
			break
		}
		dst.Samples = append(dst.Samples, sample)
	}
}

// FinishResult reports the publication outcome.
type FinishResult struct {
	Verdict    domain.Verdict
	RevisionID string
	Published  bool
}

// FinishRun validates staged counts against the review gate and, on
// publish, marks the run and revision and switches the per-survey pointer
// in a single transaction. The survey-window advisory lock serializes the
// read of the predecessor, eligibility, publication and pointer switch, so
// two competing finishes can never both publish against the same state.
// Staging writes take the same lock and revision row, freezing the reviewed
// set once the terminal decision commits. Anything that does not publish
// leaves the pointer untouched; an interrupted transaction leaves no
// partial run, revision or pointer change.
func (im *Importer) FinishRun(ctx context.Context, runID, revisionID string, q *domain.QuarantineTally) (FinishResult, error) {
	revUUID, err := mustUUID(revisionID)
	if err != nil {
		return FinishResult{}, domain.ErrUnknownRevision
	}
	runUUID, err := mustUUID(runID)
	if err != nil {
		return FinishResult{}, err
	}
	head, err := official.New(im.pool).GetRevision(ctx, revUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FinishResult{}, domain.ErrUnknownRevision
		}
		return FinishResult{}, err
	}
	if head.ImportRunID != runUUID {
		return FinishResult{}, fmt.Errorf("adapters: revision %s does not belong to run %s", revisionID, runID)
	}
	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return FinishResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSurveyWindow(ctx, tx, head.SurveyStart.Time, head.SurveyEnd.Time); err != nil {
		return FinishResult{}, err
	}
	tq := official.New(tx)
	rev, err := tq.LockRevision(ctx, revUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FinishResult{}, domain.ErrUnknownRevision
		}
		return FinishResult{}, err
	}
	if rev.ImportRunID != runUUID {
		return FinishResult{}, fmt.Errorf("adapters: revision %s does not belong to run %s", revisionID, runID)
	}
	run, err := tq.GetImportRunByID(ctx, runUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FinishResult{}, fmt.Errorf("adapters: run %s does not exist", runID)
		}
		return FinishResult{}, err
	}
	switch rev.Status {
	case domain.RevisionPublished:
		// A duplicate finish of the same revision is idempotent only while
		// it is still the window's current facts.
		if err := requireTerminalRun(run.Status, domain.RunCompleted); err != nil {
			return FinishResult{}, err
		}
		current, currentExists, err := currentPointer(ctx, tq, rev)
		if err != nil {
			return FinishResult{}, err
		}
		if currentExists && current == revUUID {
			if err := tx.Commit(ctx); err != nil {
				return FinishResult{}, err
			}
			return FinishResult{Verdict: domain.Verdict{Publish: true}, RevisionID: revisionID, Published: true}, nil
		}
		return FinishResult{Verdict: domain.Verdict{Reason: "not-current"}, RevisionID: revisionID, Published: false}, nil
	case domain.RevisionSuperseded:
		if err := requireTerminalRun(run.Status, domain.RunCompleted); err != nil {
			return FinishResult{}, err
		}
		return FinishResult{Verdict: domain.Verdict{Reason: "superseded"}, RevisionID: revisionID, Published: false}, nil
	case domain.RevisionFailed, domain.RevisionNeedsReview:
		if err := requireTerminalRun(run.Status, domain.RunFailed); err != nil {
			return FinishResult{}, err
		}
		return FinishResult{Verdict: domain.Verdict{Reason: "revision-" + rev.Status}, RevisionID: revisionID, Published: false}, nil
	case domain.RevisionStaging:
		if run.Status != domain.RunRunning {
			return FinishResult{}, fmt.Errorf("adapters: run %s is %s while revision %s is staging", runID, run.Status, revisionID)
		}
	default:
		return FinishResult{}, fmt.Errorf("adapters: revision %s has unknown status %q", revisionID, rev.Status)
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
	current, currentExists, err := currentPointer(ctx, tq, rev)
	if err != nil {
		return FinishResult{}, err
	}
	// Only a revision built against the still-current predecessor may
	// switch the pointer. A first publication requires an empty pointer; a
	// correction requires the pointer to still name the revision it read at
	// Begin. Anything newer retires this revision as superseded instead of
	// repointing accepted facts.
	stale := false
	if rev.SupersedesRevisionID.Valid {
		stale = !currentExists || current != rev.SupersedesRevisionID
	} else {
		stale = currentExists
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
	if verdict.Publish && stale {
		runStatus = domain.RunCompleted
		revStatus = domain.RevisionSuperseded
	}
	affected, err := tq.FinishImportRun(ctx, official.FinishImportRunParams{
		ID:           runUUID,
		Status:       runStatus,
		RowCounts:    counts,
		ErrorSummary: summary,
	})
	if err := checkedRows(affected, err, "finish import run"); err != nil {
		return FinishResult{}, err
	}
	switch {
	case !verdict.Publish:
		affected, err = tq.RejectStagingRevision(ctx, official.RejectStagingRevisionParams{
			ID:     revUUID,
			Status: revStatus,
		})
		if err := checkedRows(affected, err, "reject revision"); err != nil {
			return FinishResult{}, err
		}
	case stale:
		affected, err = tq.RetireStagingRevision(ctx, revUUID)
		if err := checkedRows(affected, err, "retire revision"); err != nil {
			return FinishResult{}, err
		}
		verdict = domain.Verdict{Reason: "superseded"}
	default:
		affected, err = tq.PublishStagingRevision(ctx, revUUID)
		if err := checkedRows(affected, err, "publish revision"); err != nil {
			return FinishResult{}, err
		}
		affected, err = tq.SwitchCurrentRevision(ctx, official.SwitchCurrentRevisionParams{
			SurveyStart: rev.SurveyStart,
			SurveyEnd:   rev.SurveyEnd,
			RevisionID:  revUUID,
		})
		if err := checkedRows(affected, err, "switch current revision"); err != nil {
			return FinishResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return FinishResult{}, err
	}
	return FinishResult{Verdict: verdict, RevisionID: revisionID, Published: verdict.Publish && !stale}, nil
}

func currentPointer(ctx context.Context, q *official.Queries, rev official.OfficialRevision) (pgtype.UUID, bool, error) {
	current, err := q.GetCurrentPointer(ctx, official.GetCurrentPointerParams{
		SurveyStart: rev.SurveyStart,
		SurveyEnd:   rev.SurveyEnd,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, false, nil
		}
		return pgtype.UUID{}, false, err
	}
	return current, true, nil
}

func checkedRows(affected int64, err error, action string) error {
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("adapters: %s affected %d rows, want 1", action, affected)
	}
	return nil
}

func requireTerminalRun(status, want string) error {
	if status != want {
		return fmt.Errorf("adapters: run is %s while revision is terminal (%s expected)", status, want)
	}
	return nil
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
