package application

import (
	"context"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// accountUsable maps a stored account status onto its auth verdict.
// Status leads every check (P13-T04A): a suspended or deleted account
// refuses even with otherwise live sessions or proofs. Unknown status
// strings fail closed as suspended.
func accountUsable(acc domain.Account) error {
	switch acc.Status {
	case domain.StatusActive:
		return nil
	case domain.StatusDeleted:
		return domain.ErrAccountDeleted
	default:
		return domain.ErrAccountSuspended
	}
}

// SuspendAccount marks the account suspended and revokes every session
// family immediately. Operator-driven; there is no self-suspend route.
func (s *Service) SuspendAccount(ctx context.Context, accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrAccountNotFound
	}
	return s.Store.SuspendAccount(ctx, accountID, s.Clock.NowUnix())
}

// ReactivateAccount returns a suspended account to active. Revoked
// sessions stay revoked; the owner logs in again for fresh families.
// Reactivating a deleted account refuses: deletion is terminal.
func (s *Service) ReactivateAccount(ctx context.Context, accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrAccountNotFound
	}
	return s.Store.ReactivateAccount(ctx, accountID)
}

// DeleteAccount marks the caller's account deleted, revokes every
// session family and drops address and provider bindings. The account
// row stays as an audit record; later signups mint new accounts without
// reputation carryover.
func (s *Service) DeleteAccount(ctx context.Context, accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrAccountNotFound
	}
	return s.Store.DeleteAccount(ctx, accountID, s.Clock.NowUnix())
}
