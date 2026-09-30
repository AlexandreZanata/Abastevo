package application

import (
	"context"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// BindContributor links one anonymous device contributor to the
// session-owned account (P13-T04B, B-BR-A04). Both proofs are required:
// a live account session (fresh by construction — access tokens live at
// most 900s) and a contributor key proof verified against the
// contributor's registered identity keys. The account derives
// server-side from the session; caller-supplied account IDs are never
// trusted. A contributor bound to another account refuses with
// ErrBindingCrossAccount, so recovery can never transfer another
// contributor's observations or reputation. Same-account relinks
// converge idempotently.
func (s *Service) BindContributor(ctx context.Context, familyID, accessToken, contributorID, proof string) (domain.ContributorBinding, error) {
	contributorID = strings.TrimSpace(contributorID)
	if contributorID == "" || strings.TrimSpace(proof) == "" {
		return domain.ContributorBinding{}, domain.ErrBindingInvalid
	}
	if s.KeyProver == nil {
		return domain.ContributorBinding{}, domain.ErrKeyUnavailable
	}
	accountID, err := s.ValidateAccess(ctx, familyID, accessToken)
	if err != nil {
		return domain.ContributorBinding{}, err
	}
	// Status pre-check fires before key verification, mirroring the
	// provider-link ceremony.
	if acc, found, err := s.Store.GetAccount(ctx, accountID); err != nil {
		return domain.ContributorBinding{}, err
	} else if !found {
		return domain.ContributorBinding{}, domain.ErrAccountNotFound
	} else if err := accountUsable(acc); err != nil {
		return domain.ContributorBinding{}, err
	}
	fingerprint, err := s.KeyProver.VerifyKeyProof(ctx, accountID, contributorID, proof)
	if err != nil {
		return domain.ContributorBinding{}, err
	}
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return domain.ContributorBinding{}, domain.ErrBindingInvalid
	}
	auditID, err := s.IDGen()
	if err != nil {
		return domain.ContributorBinding{}, err
	}
	binding := domain.ContributorBinding{
		AccountID:      accountID,
		ContributorID:  contributorID,
		KeyFingerprint: fingerprint,
		BoundAt:        s.Clock.NowUnix(),
	}
	if err := s.Store.BindContributor(ctx, binding, auditID); err != nil {
		return domain.ContributorBinding{}, err
	}
	return binding, nil
}

// UnbindContributor removes one contributor binding from the
// session-owned account. Unlike login methods there is no last-binding
// rule: unbinding every contributor only revokes the account's social
// write scope.
func (s *Service) UnbindContributor(ctx context.Context, familyID, accessToken, contributorID string) error {
	contributorID = strings.TrimSpace(contributorID)
	if contributorID == "" {
		return domain.ErrBindingInvalid
	}
	accountID, err := s.ValidateAccess(ctx, familyID, accessToken)
	if err != nil {
		return err
	}
	auditID, err := s.IDGen()
	if err != nil {
		return err
	}
	return s.Store.UnbindContributor(ctx, accountID, contributorID, auditID, s.Clock.NowUnix())
}

// ListBindings returns the contributor bindings of the session-owned
// account.
func (s *Service) ListBindings(ctx context.Context, familyID, accessToken string) ([]domain.ContributorBinding, error) {
	accountID, err := s.ValidateAccess(ctx, familyID, accessToken)
	if err != nil {
		return nil, err
	}
	return s.Store.ListBindings(ctx, accountID)
}

// BindingBlocked reports whether a contributor's social writes must
// refuse for account reasons (P13-T04D). Unbound contributors keep the
// anonymous baseline (false); bound contributors follow their owner's
// status. Deleted accounts drop bindings at deletion, so their former
// contributors read as unbound here — key revocation through the
// privacy erasure flow is what finally retires those keys.
func BindingBlocked(ctx context.Context, s Store, contributorID string) (bool, error) {
	if strings.TrimSpace(contributorID) == "" {
		return false, nil
	}
	owner, found, err := s.FindBindingOwner(ctx, contributorID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	acc, found, err := s.GetAccount(ctx, owner)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return !acc.Active(), nil
}
