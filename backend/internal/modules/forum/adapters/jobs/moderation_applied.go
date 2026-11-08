package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// ForumModerationAppliedKind is the intent moderation enqueues in the same
// transaction as an operator action on a forum post or comment case.
const ForumModerationAppliedKind = "forum-moderation-applied"

// ModerationAppliedPayload mirrors the moderation action job envelope.
type ModerationAppliedPayload struct {
	Version    int    `json:"version"`
	CaseID     string `json:"case_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	ActionID   string `json:"action_id"`
	Action     string `json:"action"`
}

// ReportSettler moves the RECEIVED reports of one case to a final status.
type ReportSettler interface {
	SettleReportsByCase(ctx context.Context, caseID, status string) (int64, error)
}

// ModerationApplied settles forum report receipts after a moderation
// decision. The target itself was already hidden inside the action
// transaction; this consumer only records the outcome, idempotently.
type ModerationApplied struct {
	Reports ReportSettler
}

func (ModerationApplied) Kind() string { return ForumModerationAppliedKind }
func (ModerationApplied) Version() int { return 1 }

func (h ModerationApplied) Handle(ctx context.Context, job platformjobs.Job) error {
	var payload ModerationAppliedPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("forum-moderation-applied: bad payload: %w", err)
	}
	if payload.Version != 1 || payload.CaseID == "" {
		return fmt.Errorf("forum-moderation-applied: invalid payload")
	}
	if payload.TargetType != "FORUM_POST" && payload.TargetType != "FORUM_COMMENT" {
		return fmt.Errorf("forum-moderation-applied: unsupported target %q", payload.TargetType)
	}
	status, final := reportStatusForAction(payload.Action)
	if !final {
		return nil
	}
	if h.Reports == nil {
		return fmt.Errorf("forum-moderation-applied: report settler not initialized")
	}
	_, err := h.Reports.SettleReportsByCase(ctx, payload.CaseID, status)
	return err
}

// reportStatusForAction maps terminal moderation actions to report status;
// REVIEW only moves the case into review and leaves reports RECEIVED.
func reportStatusForAction(action string) (string, bool) {
	switch action {
	case "INVALIDATE", "RESOLVE":
		return "RESOLVED", true
	case "DISMISS":
		return "DISMISSED", true
	default:
		return "", false
	}
}
