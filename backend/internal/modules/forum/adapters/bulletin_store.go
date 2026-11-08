package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// GetSourceByCode resolves the seeded Abastevo editorial source.
func (s *Store) GetSourceByCode(ctx context.Context, code string) (*domain.Source, error) {
	row, err := s.queries.GetSourceByCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMissingSource
		}
		return nil, fmt.Errorf("forum: get source by code: %w", err)
	}
	return mapSourceRowToDomain(row), nil
}

// FindLatestBulletinGroup returns the newest bulletin for one correction group.
func (s *Store) FindLatestBulletinGroup(ctx context.Context, groupKey string) (*application.BulletinGroup, error) {
	row, err := s.queries.FindLatestBulletinGroup(ctx, pgtype.Text{String: groupKey, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("forum: find latest bulletin group: %w", err)
	}
	return &application.BulletinGroup{
		PostID:           uuidString(row.ID),
		PublicationState: domain.PublicationState(row.PublicationState),
	}, nil
}

// CreateBulletinDraft inserts one deterministic bulletin draft. A duplicate
// bulletin key is a converged no-op so concurrent/restarted generators cannot
// fork the same revision pair.
func (s *Store) CreateBulletinDraft(ctx context.Context, post *domain.Post) (bool, error) {
	if err := s.insertEditorialPost(ctx, s.queries, post); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
			pgErr.ConstraintName == "forum_posts_bulletin_key_uniq" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
