package application

import (
	"context"
	"errors"
	"time"
)

var ErrErasurePending = errors.New("account: erasure has remaining scopes")

// ErasureClaim is one session-independent repair unit. One contributor per
// claim bounds expansion even if historical accounts exceed binding limits.
type ErasureClaim struct{ AccountID, ContributorID, Nonce string }
type ErasureRecoveryStore interface {
	ClaimErasure(context.Context, string, int64) (ErasureClaim, bool, error)
	FinishErasure(context.Context, ErasureClaim, bool, int64) error
}
type ErasureRecovery struct {
	Store            ErasureRecoveryStore
	NewNonce         func() (string, error)
	Now              func() time.Time
	EraseFeedback    func(context.Context, string) error
	EraseContributor func(context.Context, string, string) error
}

// Reconcile repairs one account with a one-minute execution budget, below
// the durable two-minute lease. Lost hints and dead queue jobs cannot lose
// intent: the recurring scheduler reclaims due requests independently.
func (r ErasureRecovery) Reconcile(ctx context.Context) error {
	if r.Store == nil || r.NewNonce == nil || r.Now == nil || r.EraseFeedback == nil || r.EraseContributor == nil {
		return errors.New("account: erasure recovery not configured")
	}
	nonce, err := r.NewNonce()
	if err != nil {
		return err
	}
	claim, found, err := r.Store.ClaimErasure(ctx, nonce, r.Now().Unix())
	if err != nil || !found {
		return err
	}
	work, cancel := context.WithTimeout(ctx, time.Minute)
	err = r.EraseFeedback(work, claim.AccountID)
	if err == nil && claim.ContributorID != "" {
		err = r.EraseContributor(work, claim.AccountID, claim.ContributorID)
	}
	cancel()
	// A bounded acknowledgement survives cancellation of execution. Lost
	// acknowledgements are harmless: scope execution is idempotent.
	ack, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	finish := r.Store.FinishErasure(ack, claim, err == nil, r.Now().Unix())
	return errors.Join(err, finish)
}
