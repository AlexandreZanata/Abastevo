package application

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func (s *Service) Refresh(ctx context.Context, digest, operationID string) error {
	if !s.Configured() {
		return domain.ErrUnavailable
	}
	if !domain.ValidID(operationID) {
		return domain.ErrInvalid
	}
	jobs, ok := s.Store.(JobStore)
	if !ok {
		return domain.ErrUnavailable
	}
	store, ok := s.Store.(RefreshStore)
	if !ok {
		return domain.ErrUnavailable
	}
	row, err := jobs.ForJob(ctx, digest)
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrUnauthorized) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.Environment != s.Environment || row.PackageName != s.PackageName {
		return domain.ErrDenied
	}
	a, err := store.BeginRefresh(ctx, row, operationID)
	if errors.Is(err, domain.ErrDenied) || errors.Is(err, domain.ErrUnauthorized) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.Replay {
		return nil
	}
	aad := row.Environment + "\x00" + row.PackageName + "\x00" + row.AccountID + "\x00" + digest
	token, err := s.Tokens.Open(row.CipherToken, aad)
	if err != nil {
		return domain.ErrDenied
	}
	reqCtx, cancel := context.WithTimeout(ctx, domain.ProviderTimeout)
	canonical, err := s.Provider.Verify(reqCtx, ProviderRequest{Token: token, ExpectedAccount: s.AccountBinding(row.AccountID), Environment: row.Environment, PackageName: row.PackageName})
	cancel()
	if errors.Is(err, domain.ErrNotFound) {
		// Only a previously bound token can translate canonical absence into revoke.
		canonical = ProviderPurchase{Snapshot: row.Snapshot}
		canonical.Snapshot.State = domain.Revoked
		canonical.Snapshot.Acknowledged = true
	} else if err != nil {
		if errors.Is(err, domain.ErrDenied) {
			if e := s.Store.Reject(ctx, a); e != nil {
				return e
			}
			return domain.ErrDenied
		}
		return domain.ErrUnavailable
	}
	canonical.Snapshot.VerifiedAt = s.now()
	linked := ""
	if canonical.LinkedToken != "" {
		linked = domain.TokenDigest(row.Environment, row.PackageName, canonical.LinkedToken)
	}
	return s.Store.Commit(ctx, a, VerifiedPurchase{Snapshot: canonical.Snapshot, LinkedDigest: linked})
}
