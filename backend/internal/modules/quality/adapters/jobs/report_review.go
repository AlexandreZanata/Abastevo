package jobs

import (
	"context"
	"encoding/json"
	"errors"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReportReview consumes identifiers only. Withdrawn/edited/erased commands
// cannot recreate actionable review from a stale or replayed durable job.
type ReportReview struct {
	Pool *pgxpool.Pool
	Open func(context.Context, pgx.Tx, string, string) error
}

func (ReportReview) Kind() string { return "quality-report-review" }
func (ReportReview) Version() int { return 1 }
func (h ReportReview) Handle(ctx context.Context, job platformjobs.Job) error {
	var p adapters.ReportReviewPayload
	if json.Unmarshal(job.Payload, &p) != nil || p.Version != 1 || p.Revision < 1 {
		return errors.New("quality: invalid review payload")
	}
	var id pgtype.UUID
	if id.Scan(p.ReportID) != nil || !id.Valid {
		return errors.New("quality: invalid review target")
	}
	if h.Open == nil {
		return errors.New("quality: review unavailable")
	}
	tx, e := h.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, e := quality.New(tx).LockQualityReportReview(ctx, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if r.Revision != p.Revision || r.Status != "PENDING_REVIEW" {
		return nil
	}
	targetID, e := platformjobs.NewUUIDv4()
	if e != nil {
		return e
	}
	var target pgtype.UUID
	if e = target.Scan(targetID); e != nil {
		return e
	}
	target, e = quality.New(tx).EnsureReportReviewTarget(ctx, quality.EnsureReportReviewTargetParams{ID: target, ReportID: id, ReportRevision: pgtype.Int4{Int32: p.Revision, Valid: true}})
	if e != nil {
		return e
	}
	if e = h.Open(ctx, tx, adapters.ReviewTargetString(target), r.Kind); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
