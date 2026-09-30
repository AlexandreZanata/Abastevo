package application

import (
	"context"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// LinkProvider binds a verified external subject to an existing FREE
// account (P13-T03B, B-BR-A03). The OIDC token is the only proof: equal
// email strings never merge accounts (no code path consults Email as a
// key). Cross-account binds refuse with ErrLinkCrossAccount; same-account
// relinks converge idempotently.
func (s *Service) LinkProvider(ctx context.Context, accountID, provider, rawToken, audience, nonce string) (domain.ProviderLink, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ProviderLink{}, domain.ErrAccountNotFound
	}
	if !domain.ValidProvider(provider) {
		return domain.ProviderLink{}, domain.ErrOIDCUnknownIssuer
	}
	if s.Verifier == nil {
		return domain.ProviderLink{}, domain.ErrOIDCUnavailable
	}
	// Pre-check refuses dead accounts before burning the single-use
	// nonce or fetching JWKS; the store re-checks atomically with the
	// bind.
	if acc, found, err := s.Store.GetAccount(ctx, accountID); err != nil {
		return domain.ProviderLink{}, err
	} else if !found {
		return domain.ProviderLink{}, domain.ErrAccountNotFound
	} else if err := accountUsable(acc); err != nil {
		return domain.ProviderLink{}, err
	}
	subject, err := s.Verifier.Verify(ctx, provider, rawToken, audience, nonce)
	if err != nil {
		return domain.ProviderLink{}, err
	}
	now := s.Clock.NowUnix()
	link := domain.ProviderLink{
		AccountID: accountID,
		Provider:  provider,
		Issuer:    subject.Issuer,
		Subject:   subject.Subject,
		Email:     subject.Email,
		LinkedAt:  now,
	}
	if link.Issuer == "" || link.Subject == "" {
		return domain.ProviderLink{}, domain.ErrOIDCWrongAudience
	}
	if err := s.Store.LinkProvider(ctx, link); err != nil {
		return domain.ProviderLink{}, err
	}
	return link, nil
}

// UnlinkProvider removes one provider binding. The store refuses the last
// login method (ErrLastLoginMethod) and unknown bindings
// (ErrProviderNotLinked) atomically with the delete.
func (s *Service) UnlinkProvider(ctx context.Context, accountID, provider string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrAccountNotFound
	}
	if !domain.ValidProvider(provider) {
		return domain.ErrOIDCUnknownIssuer
	}
	return s.Store.UnlinkProvider(ctx, accountID, provider)
}

// ListProviders returns the provider bindings of one account.
func (s *Service) ListProviders(ctx context.Context, accountID string) ([]domain.ProviderLink, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, domain.ErrAccountNotFound
	}
	return s.Store.ListProviders(ctx, accountID)
}
