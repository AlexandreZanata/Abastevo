package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/account"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func lockProviderSubject(ctx context.Context, tx pgx.Tx, provider, subject string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('provider-login:' || $1 || ':' || $2))`, provider, subject)
	return err
}

func (s *PGStore) LinkGoogleWithProof(ctx context.Context, proof domain.SessionProof, link domain.ProviderLink) (domain.ProviderLink, error) {
	if link.Provider != "google" || link.Issuer != domain.IssuerGoogle || link.Subject == "" || link.AccountID != proof.AccountID {
		return domain.ProviderLink{}, domain.ErrOIDCWrongAudience
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ProviderLink{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockProviderSubject(ctx, tx, link.Provider, link.Subject); err != nil {
		return domain.ProviderLink{}, err
	}
	q := account.New(tx)
	owner, err := lockProfileAccess(ctx, q, proof, true)
	if err != nil {
		return domain.ProviderLink{}, err
	}
	if existing, err := q.GetProviderLink(ctx, account.GetProviderLinkParams{AccountID: owner.ID, Provider: link.Provider}); err == nil {
		if existing.Subject == link.Subject && existing.Issuer == link.Issuer {
			link.LinkedAt = unix(existing.LinkedAt)
			return link, nil
		}
		err = q.UpdateProviderLink(ctx, account.UpdateProviderLinkParams{
			AccountID: owner.ID, Provider: link.Provider, Issuer: link.Issuer,
			Subject: link.Subject, LinkedAt: stamp(link.LinkedAt),
		})
		if err != nil {
			return domain.ProviderLink{}, googleLinkError(err)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.ProviderLink{}, err
	} else if err := q.InsertProviderLink(ctx, account.InsertProviderLinkParams{
		AccountID: owner.ID, Provider: link.Provider, Issuer: link.Issuer,
		Subject: link.Subject, LinkedAt: stamp(link.LinkedAt),
	}); err != nil {
		return domain.ProviderLink{}, googleLinkError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ProviderLink{}, err
	}
	return link, nil
}

func googleLinkError(err error) error {
	if isUniqueViolation(err) {
		return domain.ErrLinkCrossAccount
	}
	return err
}
