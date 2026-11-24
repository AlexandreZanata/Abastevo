package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func (m *MemStore) LinkGoogleWithProof(_ context.Context, proof domain.SessionProof, link domain.ProviderLink) (domain.ProviderLink, error) {
	if link.Provider != "google" || link.Issuer != domain.IssuerGoogle || link.Subject == "" || link.AccountID != proof.AccountID {
		return domain.ProviderLink{}, domain.ErrOIDCWrongAudience
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.verifyProfileAccessLocked(proof, true); err != nil {
		return domain.ProviderLink{}, err
	}
	if existing := m.links[proof.AccountID][link.Provider]; existing.Subject == link.Subject && existing.Issuer == link.Issuer {
		link.LinkedAt = existing.LinkedAt
		return link, nil
	}
	if err := m.linkProviderLocked(link); err != nil {
		return domain.ProviderLink{}, err
	}
	return link, nil
}
