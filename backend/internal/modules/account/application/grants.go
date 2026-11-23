package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// GrantExchanger redeems a native authorization code at the provider
// and returns the provider refresh token already sealed under aad. The
// plaintext token never leaves the adapter. The identity the provider
// returns with the token must equal subject, so a code belonging to
// another user can never be attached to this account.
type GrantExchanger interface {
	Exchange(ctx context.Context, provider, code, subject, aad string) ([]byte, error)
}

// GrantRevoker opens a sealed refresh token under aad and revokes it at
// the provider. A provider answer that the token is already invalid
// counts as revoked.
type GrantRevoker interface {
	Revoke(ctx context.Context, provider string, sealed []byte, aad string) error
}

// GrantOutcome is the result of one revocation attempt.
type GrantOutcome int

const (
	// GrantRevoked deletes the queued row after provider acceptance.
	GrantRevoked GrantOutcome = iota
	// GrantRetry keeps the row with backoff.
	GrantRetry
	// GrantAbandoned deletes the row after the retention window.
	GrantAbandoned
)

// GrantRevocationStore leases due revocations one at a time and settles
// them under the same lease nonce.
type GrantRevocationStore interface {
	ClaimGrantRevocation(ctx context.Context, nonce string, nowUnix int64) (domain.GrantRevocation, bool, error)
	FinishGrantRevocation(ctx context.Context, rev domain.GrantRevocation, outcome GrantOutcome, nowUnix int64) error
}

// ErrGrantRevocationIncomplete reports attempts that did not reach the
// provider's acceptance in one run. Detail is counts only.
var ErrGrantRevocationIncomplete = errors.New("account: provider grant revocation incomplete")

// GrantRevocations is the worker-side reconciler of the revocation queue.
// A nil Revoker (credentials not configured on this process) treats every
// attempt as failed: rows back off and drain at the retention window
// instead of lingering.
type GrantRevocations struct {
	Store    GrantRevocationStore
	Revoker  GrantRevoker
	NewNonce func() (string, error)
	Now      func() time.Time
}

// Reconcile settles up to domain.GrantRevocationBatch due revocations.
// It returns ErrGrantRevocationIncomplete (wrapped with counts) when any
// attempt failed or was abandoned, so the job surfaces the backlog.
func (r GrantRevocations) Reconcile(ctx context.Context) error {
	if r.Store == nil || r.NewNonce == nil || r.Now == nil {
		return errors.New("account: grant revocation not configured")
	}
	var retried, abandoned int
	for i := 0; i < domain.GrantRevocationBatch; i++ {
		nonce, err := r.NewNonce()
		if err != nil {
			return err
		}
		rev, found, err := r.Store.ClaimGrantRevocation(ctx, nonce, r.Now().Unix())
		if err != nil {
			return err
		}
		if !found {
			break
		}
		outcome := GrantRevoked
		if r.Revoker == nil {
			outcome = GrantRetry
		} else if err := r.revoke(ctx, rev); err != nil {
			outcome = GrantRetry
		}
		now := r.Now().Unix()
		if outcome == GrantRetry && domain.GrantRevocationExpired(rev.RequestedAt, now) {
			outcome = GrantAbandoned
		}
		switch outcome {
		case GrantRetry:
			retried++
		case GrantAbandoned:
			abandoned++
		}
		// A bounded settlement survives cancellation of the attempt; a
		// lost settlement only repeats an idempotent revocation.
		settle, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		err = r.Store.FinishGrantRevocation(settle, rev, outcome, now)
		stop()
		if err != nil {
			return err
		}
	}
	if retried > 0 || abandoned > 0 {
		return fmt.Errorf("%w: %d retrying, %d abandoned", ErrGrantRevocationIncomplete, retried, abandoned)
	}
	return nil
}

func (r GrantRevocations) revoke(ctx context.Context, rev domain.GrantRevocation) error {
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return r.Revoker.Revoke(work, rev.Provider, rev.Sealed, domain.GrantAAD(rev.ID))
}

// grantCaptureTimeout bounds the provider round trip a sign-in waits for.
const grantCaptureTimeout = 5 * time.Second

// captureGrant redeems the native authorization code and stores the
// sealed refresh token for accountID. Best-effort by design: sign-in
// never fails because the provider token endpoint is unavailable or the
// code is stale; OnGrantFailure observes the failure without the code or
// any token. With Grants nil nothing is exchanged or recorded.
func (s *Service) captureGrant(ctx context.Context, accountID, provider, subject, code string, now int64) {
	if s.Grants == nil || code == "" {
		return
	}
	grantID, err := s.IDGen()
	if err == nil {
		var sealed []byte
		exchange, cancel := context.WithTimeout(ctx, grantCaptureTimeout)
		sealed, err = s.Grants.Exchange(exchange, provider, code, subject, domain.GrantAAD(grantID))
		cancel()
		if err == nil {
			err = s.Store.SaveProviderGrant(ctx, domain.ProviderGrant{
				ID:         grantID,
				AccountID:  accountID,
				Provider:   provider,
				Subject:    subject,
				Sealed:     sealed,
				CapturedAt: now,
			})
		}
	}
	if err != nil && s.OnGrantFailure != nil {
		s.OnGrantFailure(ctx, provider, err)
	}
}
