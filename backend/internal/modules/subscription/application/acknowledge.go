package application

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func (s *Service) Acknowledge(ctx context.Context, digest string) error {
	if !s.Configured() {
		return domain.ErrUnavailable
	}
	store, ok := s.Store.(JobStore)
	if !ok {
		return domain.ErrUnavailable
	}
	row, err := store.ForJob(ctx, digest)
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrUnauthorized) {
		return nil
	}
	if err != nil {
		return err
	}
	aad := row.Environment + "\x00" + row.PackageName + "\x00" + row.AccountID + "\x00" + digest
	token, err := s.Tokens.Open(row.CipherToken, aad)
	if err != nil {
		return domain.ErrDenied
	}
	req := ProviderRequest{Token: token, ExpectedAccount: s.AccountBinding(row.AccountID), Environment: row.Environment, PackageName: row.PackageName}
	ctx, cancel := context.WithTimeout(ctx, domain.ProviderTimeout)
	defer cancel()
	canonical, err := s.Provider.Verify(ctx, req)
	if errors.Is(err, domain.ErrNotFound) {
		_, e := store.ReceiveNotification(ctx, Notification{Environment: row.Environment, PackageName: row.PackageName, MessageID: "ack-missing:" + digest, Digest: digest, PayloadHash: digest})
		return e
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	if canonical.Snapshot.State != domain.Active && canonical.Snapshot.State != domain.Canceled && canonical.Snapshot.State != domain.Grace {
		return nil
	}
	if !s.now().Before(canonical.Snapshot.ValidUntil) {
		return nil
	}
	// Recheck owner/revision immediately before the external acknowledgement.
	current, err := store.ForJob(ctx, digest)
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrUnauthorized) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.Revision != row.Revision {
		return domain.ErrStale
	}
	if !canonical.Snapshot.Acknowledged {
		provider, ok := s.Provider.(Acknowledger)
		if !ok {
			return domain.ErrUnavailable
		}
		if err = provider.Acknowledge(ctx, req, canonical.Snapshot.ProductID); err != nil {
			return domain.ErrUnavailable
		}
	}
	return store.MarkAcknowledged(ctx, digest, row.Revision)
}
