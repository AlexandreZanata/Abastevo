package jobs

import (
	"context"
	"encoding/json"
	"errors"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReportReviewed acknowledges only an atomically audited quality decision.
// It never creates alerts, scores or evidence from an unverified user claim.
type ReportReviewed struct{ Pool *pgxpool.Pool }

func (ReportReviewed) Kind() string { return "quality-report-reviewed" }
func (ReportReviewed) Version() int { return 1 }
func (h ReportReviewed) Handle(ctx context.Context, job platformjobs.Job) error {
	var p struct {
		Version  int    `json:"version"`
		TargetID string `json:"target_id"`
		ActionID string `json:"action_id"`
	}
	if json.Unmarshal(job.Payload, &p) != nil || p.Version != 1 {
		return errors.New("quality: invalid decision payload")
	}
	var target, action pgtype.UUID
	if target.Scan(p.TargetID) != nil || action.Scan(p.ActionID) != nil || !target.Valid || !action.Valid {
		return errors.New("quality: invalid decision reference")
	}
	_, e := quality.New(h.Pool).GetReportReviewDecision(ctx, quality.GetReportReviewDecisionParams{ActionID: action, TargetID: target})
	return e
}
