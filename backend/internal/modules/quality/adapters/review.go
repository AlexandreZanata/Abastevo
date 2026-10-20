package adapters

import (
	"context"
	"errors"
	"math"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ReviewTargetString exposes only the opaque review reference to the job port.
func ReviewTargetString(id pgtype.UUID) string { return uuidString(id) }

// ApplyReviewTx participates in the moderation store's atomic action/status/job
// transaction. A decision fences subsequent owner commands by report revision.
func (s *ReportStore) ApplyReviewTx(ctx context.Context, tx pgx.Tx, targetID, actionID, action, reason string, at time.Time) error {
	target, e := mustUUID(targetID)
	if e != nil {
		return domain.ErrReportInvalid
	}
	audit, e := mustUUID(actionID)
	if e != nil {
		return domain.ErrReportInvalid
	}
	q := quality.New(tx)
	t, e := q.GetReportReviewTarget(ctx, target)
	if errors.Is(e, pgx.ErrNoRows) {
		if action != "DISMISS" || reason != "SUPERSEDED" {
			return domain.ErrReportConflict
		}
		return q.InsertReportReviewDecision(ctx, quality.InsertReportReviewDecisionParams{ActionID: audit, TargetID: target, Action: action, ReasonCode: reason, At: pgtype.Timestamptz{Time: at.UTC(), Valid: true}})
	}
	if e != nil {
		return e
	}
	r, e := q.LockQualityReportReview(ctx, t.ReportID)
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.ErrReportConflict
	}
	if e != nil {
		return e
	}
	stale := t.BindingState != "BOUND" || !t.ReportRevision.Valid || t.ReportRevision.Int32 != r.Revision || r.Status != "PENDING_REVIEW"
	if stale && (action != "DISMISS" || reason != "SUPERSEDED") {
		return domain.ErrReportConflict
	}
	if !stale && reason == "SUPERSEDED" {
		return domain.ErrReportInvalid
	}
	stamp := pgtype.Timestamptz{Time: at.UTC(), Valid: true}
	revision := r.Revision
	if stale && t.ReportRevision.Valid {
		revision = t.ReportRevision.Int32
	}
	if !stale {
		if action == "REVIEW" {
			seen, e := q.HasReportTriage(ctx, target)
			if e != nil {
				return e
			}
			if seen {
				return domain.ErrReportConflict
			}
		}
		d, e := domain.DecideReportReview(action, reason)
		if e != nil {
			return e
		}
		if d.Status != "" {
			if revision == math.MaxInt32 {
				return domain.ErrReportConflict
			}
			revision++
			if e = q.ApplyReportReview(ctx, quality.ApplyReportReviewParams{ID: r.ID, Revision: revision, Status: d.Status, Reason: reason, ClearText: d.ClearText, At: stamp}); e != nil {
				return e
			}
			auditAction := "REVIEW"
			if d.Status == "HIDDEN" {
				auditAction = "HIDE"
			}
			if e = q.InsertReportAudit(ctx, quality.InsertReportAuditParams{ReportID: r.ID, Revision: revision, Action: auditAction, PolicyVersion: "quality-review-v1", At: stamp}); e != nil {
				return e
			}
			if e = q.ClearReportAppeals(ctx, r.ID); e != nil {
				return e
			}
		}
	}
	return q.InsertReportReviewDecision(ctx, quality.InsertReportReviewDecisionParams{ActionID: audit, TargetID: target, ReportRevision: pgtype.Int4{Int32: revision, Valid: !stale || t.ReportRevision.Valid}, Action: action, ReasonCode: reason, At: stamp})
}

// PrivateReview contains user text and must only be called after restricted
// operator authorization. It is never registered on the anonymous API.
type PrivateReview struct {
	TargetID      string        `json:"target_id"`
	BoundRevision int32         `json:"bound_revision"`
	Current       domain.Report `json:"current"`
	AppealText    string        `json:"appeal_text,omitempty"`
	Stale         bool          `json:"stale"`
}

func (s *ReportStore) InspectReview(ctx context.Context, targetID string) (PrivateReview, error) {
	var out PrivateReview
	id, e := mustUUID(targetID)
	if e != nil {
		return out, domain.ErrReportInvalid
	}
	tx, e := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := quality.New(tx)
	t, e := q.GetReportReviewTarget(ctx, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, domain.ErrReportNotFound
	}
	if e != nil {
		return out, e
	}
	// Private read only; no account identifier is included in the view.
	r, e := q.GetReviewReport(ctx, t.ReportID)
	if e != nil {
		return out, e
	}
	out = PrivateReview{TargetID: targetID, BoundRevision: t.ReportRevision.Int32, Current: reportView(r), Stale: t.BindingState != "BOUND" || t.ReportRevision.Int32 != r.Revision || r.Status != "PENDING_REVIEW"}
	out.AppealText, e = q.GetReportAppeal(ctx, quality.GetReportAppealParams{ReportID: r.ID, Revision: r.Revision})
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	return out, tx.Commit(ctx)
}

// PrivateGroupPage is a bounded operator view. Counts describe this page only;
// they are never advertised as station-wide prevalence or public confidence.
type PrivateGroupPage struct {
	Scope       string               `json:"scope"`
	Signal      domain.PrivateSignal `json:"signal"`
	NextAfterID string               `json:"next_after_id,omitempty"`
}

func (s *ReportStore) GroupReview(ctx context.Context, station, fuel, day, after string, activeOwners func(context.Context, pgx.Tx, []string) (map[string]bool, error)) (PrivateGroupPage, error) {
	out := PrivateGroupPage{Scope: "PAGE"}
	sid, e := mustUUID(station)
	if e != nil || activeOwners == nil {
		return out, domain.ErrReportInvalid
	}
	date, e := pgDate(day)
	if e != nil {
		return out, domain.ErrReportInvalid
	}
	valid := false
	for _, f := range domain.AllFuels {
		if string(f) == fuel {
			valid = true
		}
	}
	if !valid {
		return out, domain.ErrReportInvalid
	}
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	}
	cursor, e := mustUUID(after)
	if e != nil {
		return out, domain.ErrReportInvalid
	}
	tx, e := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, e := quality.New(tx).ListReportGroup(ctx, quality.ListReportGroupParams{StationID: sid, Fuel: fuel, Day: date, AfterID: cursor})
	if e != nil {
		return out, e
	}
	if len(rows) > 100 {
		rows = rows[:100]
		out.NextAfterID = uuidString(rows[99].ID)
	}
	owners := make([]string, 0, len(rows))
	for _, r := range rows {
		owners = append(owners, uuidString(r.AccountID))
	}
	active, e := activeOwners(ctx, tx, owners)
	if e != nil {
		return out, e
	}
	claims := make([]domain.SignalClaim, 0, len(rows))
	for _, r := range rows {
		owner := uuidString(r.AccountID)
		claims = append(claims, domain.SignalClaim{Owner: owner, Status: r.Status, OwnerActive: active[owner], OfficialLinkSuperseded: r.Superseded})
	}
	out.Signal = domain.ReviewOnlySignal(claims)
	return out, tx.Commit(ctx)
}
