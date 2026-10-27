// Package adapters owns entitlement-grant persistence. Every mutation joins
// the caller transaction so the admin audit and idempotency rows commit with
// the grant change.
package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	qent "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/entitlement"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ Pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func uid(raw string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(raw)
	return id
}

func idstr(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	raw := hex.EncodeToString(id.Bytes[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: !t.IsZero()}
}

func optional(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return stamp(*t)
}

func convert(row qent.EntitlementGrant) domain.Grant {
	grant := domain.Grant{
		ID:               idstr(row.ID),
		AccountID:        idstr(row.AccountID),
		Tier:             domain.Tier(row.Tier),
		PolicyVersion:    int(row.PolicyVersion),
		State:            domain.State(row.State),
		StartAt:          row.StartAt.Time,
		Reason:           row.Reason,
		CreatedAt:        row.CreatedAt.Time,
		CreatedSessionID: idstr(row.CreatedSessionID),
	}
	if row.EndAt.Valid {
		end := row.EndAt.Time
		grant.EndAt = &end
	}
	if row.RevokedAt.Valid {
		revoked := row.RevokedAt.Time
		grant.RevokedAt = &revoked
	}
	if row.RevokeReason.Valid {
		grant.RevokeReason = row.RevokeReason.String
	}
	return grant
}

// LockEligibleAccount locks the target account and refuses anything but an
// active status (ENT-02). A missing account is ineligible, not a 404.
func (s *Store) LockEligibleAccount(ctx context.Context, tx pgx.Tx, accountID string) (domain.Grant, error) {
	row, err := qent.New(tx).LockAccount(ctx, uid(accountID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Grant{}, domain.ErrIneligible
		}
		return domain.Grant{}, domain.ErrUnavailable
	}
	if row.Status != "active" {
		return domain.Grant{}, domain.ErrIneligible
	}
	return domain.Grant{AccountID: idstr(row.ID)}, nil
}

// CreateTx inserts one active grant. A partial unique conflict means another
// ACTIVE grant already exists for the tier (ENT-03).
func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, in domain.NewGrant) (domain.Grant, error) {
	if !domain.ValidTier(in.Tier) || in.ID == "" || in.AccountID == "" || in.CreatedSessionID == "" || in.Now.IsZero() || !domain.ValidEndAt(in.Now, in.EndAt) {
		return domain.Grant{}, domain.ErrInvalid
	}
	row, err := qent.New(tx).InsertGrant(ctx, qent.InsertGrantParams{
		ID: uid(in.ID), AccountID: uid(in.AccountID), Tier: string(in.Tier),
		PolicyVersion: domain.PolicyVersion, StartAt: stamp(in.Now),
		EndAt: optional(in.EndAt), Reason: in.Reason,
		CreatedSessionID: uid(in.CreatedSessionID),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return domain.Grant{}, domain.ErrExists
			case "23503":
				return domain.Grant{}, domain.ErrIneligible
			}
		}
		return domain.Grant{}, domain.ErrUnavailable
	}
	return convert(row), nil
}

// RevokeTx revokes one active grant. Revoked grants are never reactivated
// (ENT-07); a second revoke reports already-revoked.
func (s *Store) RevokeTx(ctx context.Context, tx pgx.Tx, id string, now time.Time, reason string) (domain.Grant, error) {
	row, err := qent.New(tx).RevokeGrant(ctx, qent.RevokeGrantParams{ID: uid(id), Now: stamp(now), Reason: pgtype.Text{String: reason, Valid: true}})
	if err == nil {
		return convert(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Grant{}, domain.ErrUnavailable
	}
	existing, getErr := qent.New(tx).GetGrant(ctx, uid(id))
	if errors.Is(getErr, pgx.ErrNoRows) {
		return domain.Grant{}, domain.ErrNotFound
	}
	if getErr != nil {
		return domain.Grant{}, domain.ErrUnavailable
	}
	if existing.State == string(domain.StateRevoked) {
		return domain.Grant{}, domain.ErrAlreadyRevoked
	}
	return domain.Grant{}, domain.ErrNotFound
}

func (s *Store) GetTx(ctx context.Context, tx pgx.Tx, id string) (domain.Grant, error) {
	row, err := qent.New(tx).GetGrant(ctx, uid(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Grant{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Grant{}, domain.ErrUnavailable
	}
	return convert(row), nil
}

func (s *Store) ActiveTx(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) ([]domain.Grant, error) {
	rows, err := qent.New(tx).ActiveGrants(ctx, qent.ActiveGrantsParams{AccountID: uid(accountID), Now: stamp(now)})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	return convertAll(rows), nil
}

func (s *Store) ListByAccountTx(ctx context.Context, tx pgx.Tx, accountID string) ([]domain.Grant, error) {
	rows, err := qent.New(tx).AccountGrants(ctx, uid(accountID))
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	return convertAll(rows), nil
}

func (s *Store) ListTx(ctx context.Context, tx pgx.Tx, filter domain.GrantFilter) ([]domain.Grant, error) {
	limit := filter.Limit
	if limit < 1 || limit > 101 {
		limit = 50
	}
	tier, state := pgtype.Text{}, pgtype.Text{}
	if filter.Tier != "" {
		tier = pgtype.Text{String: string(filter.Tier), Valid: true}
	}
	if filter.State != "" {
		state = pgtype.Text{String: string(filter.State), Valid: true}
	}
	rows, err := qent.New(tx).ListGrants(ctx, qent.ListGrantsParams{
		Tier: tier, State: state, BeforeCreated: stamp(filter.BeforeCreated),
		BeforeID: uid(filter.BeforeID), RowLimit: int32(limit),
	})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	return convertAll(rows), nil
}

func convertAll(rows []qent.EntitlementGrant) []domain.Grant {
	out := make([]domain.Grant, 0, len(rows))
	for _, row := range rows {
		out = append(out, convert(row))
	}
	return out
}
