package application

import (
	"context"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// Report reasons stay short and reviewable, mirroring the moderation
// reason bound without importing that module.
const ReportMaxReasonChars = 200

// ValidVisibility reports whether a visibility transition target is
// operator-settable. Author tombstones are not a visibility state.
func ValidVisibility(visibility string) bool {
	switch visibility {
	case domain.VisibilityVisible, domain.VisibilityFlagged, domain.VisibilityHidden:
		return true
	default:
		return false
	}
}

// ReportComment files a moderation report against one live comment
// (F07). The reporter needs only an active account; reports never
// grant moderation power and carry no reporter identity into the
// case (privacy by design — reporter deletion cannot cascade).
// First reports flag the comment; quota bounds report volume.
func (s *Service) ReportComment(ctx context.Context, accountID, commentID, reason string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrTargetInvalid
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > ReportMaxReasonChars {
		return domain.ErrReportInvalid
	}
	if err := s.authorize(ctx, accountID); err != nil {
		return err
	}
	target, found, err := s.Comments.GetComment(ctx, strings.TrimSpace(commentID))
	if err != nil {
		return err
	}
	if !found || !target.Live() {
		return domain.ErrCommentNotFound
	}
	if s.ReportQuota == nil || s.OpenCase == nil {
		return domain.ErrGateRequired
	}
	if _, err := s.ReportQuota(ctx, accountID, "report"); err != nil {
		return err
	}
	if _, err := s.OpenCase(ctx, accountID, target.ID, reason); err != nil {
		return err
	}
	return s.Comments.FlagComment(ctx, target.ID)
}

// SetVisibility moves one comment along the moderator lane. Operator
// authority arrives from the restricted CLI environment, never from a
// public request; tombstoned rows refuse so author deletion wins over
// later un-hides.
func (s *Service) SetVisibility(ctx context.Context, commentID, visibility string) error {
	if !ValidVisibility(visibility) {
		return domain.ErrVisibilityInvalid
	}
	target, found, err := s.Comments.GetComment(ctx, strings.TrimSpace(commentID))
	if err != nil {
		return err
	}
	if !found || !target.Live() {
		return domain.ErrCommentNotFound
	}
	return s.Comments.SetVisibility(ctx, target.ID, visibility)
}

// ReportQuota bounds report volume per reporter. The composition root
// injects the shared limiter; handlers map denials onto 429.
type ReportQuota func(ctx context.Context, subject, operation string) (time.Duration, error)

// OpenCase files the moderation case for a reported comment and
// returns its ID. The composition root injects the moderation-owned
// opener; cases converge on already-open targets instead of
// flooding the queue.
type OpenCase func(ctx context.Context, reporterAccountID, commentID, reason string) (string, error)
