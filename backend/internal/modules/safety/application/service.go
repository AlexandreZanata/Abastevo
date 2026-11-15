// Package application validates community safety intents (B-BR-SAFE-01…09)
// and hands normalized commands to the transactional store port.
package application

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/domain"
)

// Guard re-checks the session, signing key binding and account status
// inside the write transaction (B-BR-SAFE-03).
type Guard func(ctx context.Context, tx pgx.Tx) error

// BlockCommand blocks the author of one public content reference.
type BlockCommand struct {
	AccountID string
	Ref       domain.ContentRef
	Guard     Guard
}

// UnblockCommand removes one of the owner's blocks.
type UnblockCommand struct {
	AccountID string
	BlockID   string
	Guard     Guard
}

// ReportCommand reports the author of one public content reference.
type ReportCommand struct {
	AccountID string
	Ref       domain.ContentRef
	Reason    domain.ReportReason
	Detail    string
	Guard     Guard
}

// Store is the transactional persistence port owned by safety.
type Store interface {
	Block(ctx context.Context, cmd BlockCommand) (domain.BlockResult, error)
	Unblock(ctx context.Context, cmd UnblockCommand) error
	ListBlocks(ctx context.Context, accountID string) ([]domain.Block, error)
	Hidden(ctx context.Context, accountID string, refs []domain.ContentRef) ([]domain.ContentRef, error)
	ReportUser(ctx context.Context, cmd ReportCommand) (domain.ReportReceipt, error)
}

// Service validates wire input before any store work.
type Service struct {
	Store Store
}

// Block validates the reference and blocks its author.
func (s Service) Block(ctx context.Context, accountID, kind, id string, guard Guard) (domain.BlockResult, error) {
	if strings.TrimSpace(accountID) == "" {
		return domain.BlockResult{}, domain.ErrInvalid
	}
	ref, err := domain.ParseContentRef(kind, id)
	if err != nil {
		return domain.BlockResult{}, err
	}
	return s.Store.Block(ctx, BlockCommand{AccountID: accountID, Ref: ref, Guard: guard})
}

// Unblock validates the block id and removes the owner's block.
func (s Service) Unblock(ctx context.Context, accountID, blockID string, guard Guard) error {
	if strings.TrimSpace(accountID) == "" {
		return domain.ErrInvalid
	}
	id, err := domain.ParseID(blockID)
	if err != nil {
		// A malformed id names no block: refuse like a missing one.
		return domain.ErrNotFound
	}
	return s.Store.Unblock(ctx, UnblockCommand{AccountID: accountID, BlockID: id, Guard: guard})
}

// ListBlocks returns the owner's blocks, newest first.
func (s Service) ListBlocks(ctx context.Context, accountID string) ([]domain.Block, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, domain.ErrInvalid
	}
	return s.Store.ListBlocks(ctx, accountID)
}

// Hidden validates 1..100 references and returns those written by
// accounts the owner blocked.
func (s Service) Hidden(ctx context.Context, accountID string, raw []domain.RawRef) ([]domain.ContentRef, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, domain.ErrInvalid
	}
	refs, err := domain.ParseHiddenQuery(raw)
	if err != nil {
		return nil, err
	}
	return s.Store.Hidden(ctx, accountID, refs)
}

// ReportUser validates reason/detail/reference and files the report.
func (s Service) ReportUser(ctx context.Context, accountID, kind, id, reason, detail string, guard Guard) (domain.ReportReceipt, error) {
	if strings.TrimSpace(accountID) == "" {
		return domain.ReportReceipt{}, domain.ErrInvalid
	}
	ref, err := domain.ParseContentRef(kind, id)
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	code, err := domain.ParseReportReason(reason)
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	clean, err := domain.NormalizeDetail(detail)
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	return s.Store.ReportUser(ctx, ReportCommand{AccountID: accountID, Ref: ref, Reason: code, Detail: clean, Guard: guard})
}
