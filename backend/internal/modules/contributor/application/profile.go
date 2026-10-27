package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	"github.com/jackc/pgx/v5"
)

type ProfileResult struct {
	Profile domain.Profile
	Links   []domain.Link
	Roles   []string
	Found   bool
}

func (s *Service) ReadProfile(ctx context.Context, accountID string) (ProfileResult, error) {
	if !s.enabled() || !domain.ValidUUID(accountID) {
		return ProfileResult{}, domain.ErrInvalid
	}
	var out ProfileResult
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		profile, found, err := s.Store.GetProfile(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		links, err := s.Store.ListLinks(ctx, tx, accountID)
		if err != nil {
			return err
		}
		out = ProfileResult{Profile: profile, Links: links, Roles: []string{}, Found: true}
		return nil
	})
	return out, err
}

func (s *Service) UpsertProfile(ctx context.Context, accountID, displayName string, links []domain.Link, visibility string, consent, showRoles bool) (ProfileResult, error) {
	if !s.enabled() || !domain.ValidUUID(accountID) || s.RequireCapability == nil {
		return ProfileResult{}, domain.ErrInvalid
	}
	name, ok := domain.NormalizeDisplayName(displayName)
	if !ok {
		return ProfileResult{}, domain.ErrInvalid
	}
	links, ok = domain.ValidateLinks(links)
	if !ok {
		return ProfileResult{}, domain.ErrInvalid
	}
	if visibility == "" {
		visibility = domain.VisibilityHidden
	}
	if !domain.ValidVisibility(visibility) {
		return ProfileResult{}, domain.ErrInvalid
	}
	if visibility == domain.VisibilityPublic && !consent {
		return ProfileResult{}, domain.ErrInvalid
	}
	now := s.now()
	var out ProfileResult
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.RequireCapability(ctx, tx, accountID); err != nil {
			return err
		}
		count, err := s.Store.CountProfileEditsSince(ctx, tx, accountID, now.Add(-24*time.Hour))
		if err != nil {
			return domain.ErrUnavailable
		}
		if count >= domain.MaxProfileEdits {
			return domain.ErrQuota
		}
		profile, err := s.Store.UpsertProfile(ctx, tx, accountID, name, visibility, consent, showRoles)
		if err != nil {
			return err
		}
		if err := s.Store.ReplaceLinks(ctx, tx, accountID, links); err != nil {
			return err
		}
		if err := s.Store.InsertProfileEdit(ctx, tx, accountID, now); err != nil {
			return domain.ErrUnavailable
		}
		out = ProfileResult{Profile: profile, Links: links, Roles: []string{}, Found: true}
		return nil
	})
	return out, err
}

func (s *Service) PublicPage(ctx context.Context, afterSince time.Time, afterID string, limit int) ([]domain.PublicContributor, error) {
	if s == nil || s.Store == nil {
		return nil, domain.ErrUnavailable
	}
	if limit < 1 || limit > 51 {
		limit = 51
	}
	var out []domain.PublicContributor
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.Store.PublicContributors(ctx, tx, s.now(), afterSince, afterID, limit)
		return err
	})
	return out, err
}
