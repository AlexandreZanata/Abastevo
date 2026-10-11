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
// session family, captures contributor bindings into durable erasure
// intents and drops address and provider bindings — then enqueues
// owned scope cleanup through RequestErasure (SEC04-B). The account
// row stays as an audit record; later signups mint new accounts
// without reputation carryover. A nil RequestErasure refuses closed:
// deletion without the erasure linkage would repeat AUD-05.
//
// Crash window: intents are durable before enqueue, so a failure
// after revocation replays from ListErasureIntents on retry
// (convergent scope cleanup, SEC04-D). The receipt names the
// recorded intent — acknowledgement is never completion (SEC04-A).
func (s *Service) DeleteAccount(ctx context.Context, accountID string) (DeletionReceipt, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return DeletionReceipt{}, domain.ErrAccountNotFound
	}
	if s.RequestErasure == nil {
		return DeletionReceipt{}, domain.ErrErasureWiringMissing
	}
	contributors, err := s.Store.DeleteAccount(ctx, accountID, s.Clock.NowUnix())
	if err != nil {
		return DeletionReceipt{}, err
	}
	if len(contributors) == 0 {
		// Retry after a crash between revocation and enqueue, or a
		// binding-free account: replay durable intents.
		contributors, err = s.Store.ListErasureIntents(ctx, accountID)
		if err != nil {
			return DeletionReceipt{}, err
		}
	}
	if err := s.RequestErasure(ctx, accountID, contributors); err != nil {
		return DeletionReceipt{}, err
	}
	return DeletionReceipt{AccountID: accountID, ContributorIDs: contributors, ErasureRequested: true}, nil
}
