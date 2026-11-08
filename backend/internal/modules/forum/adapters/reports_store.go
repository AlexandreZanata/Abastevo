package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/forum"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Report files a moderation report for a forum post or comment.
// Implements B-BR-FOR-08, B-BR-FOR-10, B-BR-FOR-11.
func (s *Store) Report(ctx context.Context, cmd application.ReportCommand) (*domain.ReportReceipt, error) {
	if cmd.TargetType != "FORUM_POST" && cmd.TargetType != "FORUM_COMMENT" {
		return nil, domain.ErrTargetInvalid
	}
	targetUUID, err := mustUUID(cmd.TargetID)
	if err != nil {
		return nil, domain.ErrTargetInvalid
	}
	reporterUUID, err := mustUUID(cmd.ReporterAccountID)
	if err != nil {
		return nil, domain.ErrMissingAuthor
	}

	cleanReason := strings.TrimSpace(cmd.Reason)
	if cleanReason == "" || len([]rune(cleanReason)) > 200 || !domain.ValidReportReason(cleanReason) {
		return nil, domain.ErrReportInvalid
	}
	cleanDetail := strings.TrimSpace(cmd.Detail)
	if len([]rune(cleanDetail)) > 500 {
		return nil, domain.ErrReportInvalid
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if cmd.Guard != nil {
		if err := cmd.Guard(ctx, tx); err != nil {
			return nil, err
		}
	}

	qtx := s.queries.WithTx(tx)

	// Validate target existence, state and self-report
	switch cmd.TargetType {
	case "FORUM_POST":
		post, err := qtx.GetPostByIDForUpdate(ctx, targetUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrPostNotFound
			}
			return nil, fmt.Errorf("forum: lock post: %w", err)
		}
		if post.PublicationState == string(domain.StateRemoved) {
			return nil, domain.ErrPostNotFound
		}
		if post.AuthorAccountID.Valid && uuidString(post.AuthorAccountID) == cmd.ReporterAccountID {
			return nil, domain.ErrSelfReportForbidden
		}

	case "FORUM_COMMENT":
		comment, err := qtx.GetCommentByIDForUpdate(ctx, targetUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrCommentNotFound
			}
			return nil, fmt.Errorf("forum: lock comment: %w", err)
		}
		if comment.PublicationState == string(domain.StateRemoved) {
			return nil, domain.ErrCommentNotFound
		}
		if comment.AuthorAccountID.Valid && uuidString(comment.AuthorAccountID) == cmd.ReporterAccountID {
			return nil, domain.ErrSelfReportForbidden
		}
	}

	// Quota check: 10 reports per rolling 24 hours per account (B-BR-FOR-11)
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)
	count, err := qtx.CountReportsByAccountSince(ctx, forum.CountReportsByAccountSinceParams{
		AccountID: reporterUUID,
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("forum: count reports: %w", err)
	}
	if count >= 10 {
		return nil, domain.ErrReportQuotaExceeded
	}

	reportUUID := newUUIDBytes()

	inserted, err := qtx.InsertForumReport(ctx, forum.InsertForumReportParams{
		ID:         reportUUID,
		AccountID:  reporterUUID,
		TargetType: cmd.TargetType,
		TargetID:   targetUUID,
		Reason:     cleanReason,
		Detail:     cleanDetail,
		Status:     "RECEIVED",
		CreatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err == nil {
		if s.CaseOpener == nil {
			return nil, fmt.Errorf("forum: case opener unconfigured")
		}
		caseID, _, err := s.CaseOpener(ctx, tx, cmd.TargetType, cmd.TargetID, cleanReason, cleanDetail)
		if err != nil {
			return nil, fmt.Errorf("forum: open case: %w", err)
		}
		caseUUID, err := mustUUID(caseID)
		if err != nil {
			return nil, fmt.Errorf("forum: parse case id: %w", err)
		}
		if err := qtx.UpdateForumReportCase(ctx, forum.UpdateForumReportCaseParams{
			ID:               inserted.ID,
			ModerationCaseID: caseUUID,
		}); err != nil {
			return nil, fmt.Errorf("forum: update report case: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("forum: commit report: %w", err)
		}

		return &domain.ReportReceipt{
			ID:               uuidString(inserted.ID),
			AccountID:        cmd.ReporterAccountID,
			TargetType:       cmd.TargetType,
			TargetID:         cmd.TargetID,
			Reason:           cleanReason,
			Detail:           cleanDetail,
			Status:           "RECEIVED",
			ModerationCaseID: caseID,
			CreatedAt:        now,
			Replayed:         false,
		}, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("forum: insert report: %w", err)
	}

	// Idempotent duplicate: account already reported this target
	existing, err := qtx.GetForumReportByAccountAndTarget(ctx, forum.GetForumReportByAccountAndTargetParams{
		AccountID:  reporterUUID,
		TargetType: cmd.TargetType,
		TargetID:   targetUUID,
	})
	if err != nil {
		return nil, fmt.Errorf("forum: get existing report: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("forum: commit report replay: %w", err)
	}

	caseIDStr := ""
	if existing.ModerationCaseID.Valid {
		caseIDStr = uuidString(existing.ModerationCaseID)
	}

	return &domain.ReportReceipt{
		ID:               uuidString(existing.ID),
		AccountID:        cmd.ReporterAccountID,
		TargetType:       existing.TargetType,
		TargetID:         uuidString(existing.TargetID),
		Reason:           existing.Reason,
		Detail:           existing.Detail,
		Status:           existing.Status,
		ModerationCaseID: caseIDStr,
		CreatedAt:        existing.CreatedAt.Time,
		Replayed:         true,
	}, nil
}

// SettleReportsByCase records a moderation outcome on the RECEIVED reports
// linked to a case. Replays update nothing.
func (s *Store) SettleReportsByCase(ctx context.Context, caseID, status string) (int64, error) {
	if status != "RESOLVED" && status != "DISMISSED" {
		return 0, domain.ErrReportInvalid
	}
	caseUUID, err := mustUUID(caseID)
	if err != nil {
		return 0, domain.ErrTargetInvalid
	}
	return s.queries.SettleForumReportsByCase(ctx, forum.SettleForumReportsByCaseParams{
		Status:           status,
		ModerationCaseID: caseUUID,
	})
}

// InvalidateForumTargetTx executes operator invalidation on a reported forum post or comment.
// Implements BUC-006, B-BR-012, B-BR-FOR-08.
func (s *Store) InvalidateForumTargetTx(ctx context.Context, tx pgx.Tx, targetType, targetID, caseID, reason string, at time.Time) error {
	if tx == nil {
		newTx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("forum: begin tx: %w", err)
		}
		defer newTx.Rollback(ctx)
		if err := s.InvalidateForumTargetTx(ctx, newTx, targetType, targetID, caseID, reason, at); err != nil {
			return err
		}
		return newTx.Commit(ctx)
	}

	qtx := s.queries.WithTx(tx)
	caseUUID, _ := mustUUID(caseID)

	switch targetType {
	case "FORUM_POST":
		postUUID, err := mustUUID(targetID)
		if err != nil {
			return domain.ErrPostNotFound
		}
		post, err := qtx.GetPostByIDForUpdate(ctx, postUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrPostNotFound
			}
			return fmt.Errorf("forum: lock post: %w", err)
		}
		if post.PublicationState == string(domain.StateRemoved) {
			return nil
		}

		rows, err := qtx.SetPostVisibility(ctx, forum.SetPostVisibilityParams{
			ID:         postUUID,
			Visibility: string(domain.VisibilityHidden),
			UpdatedAt:  pgtype.Timestamptz{Time: at, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("forum: set post visibility: %w", err)
		}
		if rows == 0 {
			return domain.ErrPostNotFound
		}

		caseIDStr := uuidString(caseUUID)
		priorState := post.Visibility
		return s.RecordForumChangeTx(ctx, tx, domain.ChangeRecord{
			TargetType:    "POST",
			TargetID:      targetID,
			Revision:      int(post.Revision) + 1,
			Action:        "HIDE",
			ActorKind:     "ADMIN",
			AdminAuditRef: &caseIDStr,
			ReasonCode:    "moderation_invalidate",
			PriorState:    &priorState,
			NewState:      string(domain.VisibilityHidden),
			CreatedAt:     at,
		})

	case "FORUM_COMMENT":
		commentUUID, err := mustUUID(targetID)
		if err != nil {
			return domain.ErrCommentNotFound
		}
		comment, err := qtx.GetCommentByIDForUpdate(ctx, commentUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrCommentNotFound
			}
			return fmt.Errorf("forum: lock comment: %w", err)
		}
		if comment.PublicationState == string(domain.StateRemoved) {
			return nil
		}

		_, err = qtx.SetCommentVisibility(ctx, forum.SetCommentVisibilityParams{
			ID:         commentUUID,
			Visibility: string(domain.VisibilityHidden),
			UpdatedAt:  pgtype.Timestamptz{Time: at, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("forum: set comment visibility: %w", err)
		}

		caseIDStr := uuidString(caseUUID)
		priorState := comment.Visibility
		return s.RecordForumChangeTx(ctx, tx, domain.ChangeRecord{
			TargetType:    "COMMENT",
			TargetID:      targetID,
			Revision:      int(comment.Revision),
			Action:        "HIDE",
			ActorKind:     "ADMIN",
			AdminAuditRef: &caseIDStr,
			ReasonCode:    "moderation_invalidate",
			PriorState:    &priorState,
			NewState:      string(domain.VisibilityHidden),
			CreatedAt:     at,
		})

	default:
		return fmt.Errorf("forum: unknown target type: %s", targetType)
	}
}
