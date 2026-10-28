// Package adapters owns staff assignment persistence. Roles are code-frozen;
// only assignments join the caller transaction.
package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	qstaff "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/staff"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/domain"
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

func convert(row qstaff.StaffAssignment) domain.Assignment {
	out := domain.Assignment{
		ID: idstr(row.ID), AccountID: idstr(row.AccountID), Role: row.Role,
		Label: domain.Label(row.Role), State: row.State,
		StartAt: row.StartAt.Time, Reason: row.Reason, CreatedAt: row.CreatedAt.Time,
	}
	if row.EndAt.Valid {
		end := row.EndAt.Time
		out.EndAt = &end
	}
	if row.RevokedAt.Valid {
		revoked := row.RevokedAt.Time
		out.RevokedAt = &revoked
	}
	if row.RevokeReason.Valid {
		out.RevokeReason = row.RevokeReason.String
	}
	return out
}

func (s *Store) LockEligibleAccount(ctx context.Context, tx pgx.Tx, accountID string) error {
	row, err := qstaff.New(tx).LockAccount(ctx, uid(accountID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrIneligible
		}
		return domain.ErrUnavailable
	}
	if row.Status != "active" {
		return domain.ErrIneligible
	}
	return nil
}

func (s *Store) GetActiveAssignment(ctx context.Context, tx pgx.Tx, accountID, role string) (domain.Assignment, bool, error) {
	row, err := qstaff.New(tx).GetActiveAssignment(ctx, qstaff.GetActiveAssignmentParams{
		AccountID: uid(accountID), Role: role,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Assignment{}, false, nil
	}
	if err != nil {
		return domain.Assignment{}, false, domain.ErrUnavailable
	}
	return convert(row), true, nil
}

func (s *Store) Insert(ctx context.Context, tx pgx.Tx, in domain.Assignment) (domain.Assignment, error) {
	row, err := qstaff.New(tx).InsertAssignment(ctx, qstaff.InsertAssignmentParams{
		ID: uid(in.ID), AccountID: uid(in.AccountID), Role: in.Role,
		StartAt: stamp(in.StartAt), EndAt: optional(in.EndAt), Reason: in.Reason,
		CreatedSessionID: uid(in.CreatedSessionID),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return domain.Assignment{}, domain.ErrConflict
			case "23503":
				return domain.Assignment{}, domain.ErrIneligible
			}
		}
		return domain.Assignment{}, domain.ErrUnavailable
	}
	return convert(row), nil
}

func (s *Store) Get(ctx context.Context, tx pgx.Tx, id string) (domain.Assignment, error) {
	row, err := qstaff.New(tx).GetAssignment(ctx, uid(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Assignment{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Assignment{}, domain.ErrUnavailable
	}
	return convert(row), nil
}

func (s *Store) Revoke(ctx context.Context, tx pgx.Tx, id string, now time.Time, reason string) (domain.Assignment, error) {
	row, err := qstaff.New(tx).RevokeAssignment(ctx, qstaff.RevokeAssignmentParams{
		ID: uid(id), Now: stamp(now), Reason: pgtype.Text{String: reason, Valid: true},
	})
	if err == nil {
		return convert(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Assignment{}, domain.ErrUnavailable
	}
	existing, getErr := qstaff.New(tx).GetAssignment(ctx, uid(id))
	if errors.Is(getErr, pgx.ErrNoRows) {
		return domain.Assignment{}, domain.ErrNotFound
	}
	if getErr != nil {
		return domain.Assignment{}, domain.ErrUnavailable
	}
	if existing.State == string(domain.StateRevoked) {
		return domain.Assignment{}, domain.ErrAlreadyRevoked
	}
	return domain.Assignment{}, domain.ErrNotFound
}

func (s *Store) Active(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) ([]domain.Assignment, error) {
	rows, err := qstaff.New(tx).ActiveAssignments(ctx, qstaff.ActiveAssignmentsParams{
		AccountID: uid(accountID), Now: stamp(now),
	})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.Assignment, 0, len(rows))
	for _, row := range rows {
		out = append(out, convert(row))
	}
	return out, nil
}

func (s *Store) ActiveLabels(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) ([]string, error) {
	rows, err := s.Active(ctx, tx, accountID, now)
	if err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Label != "" {
			labels = append(labels, row.Label)
		}
	}
	return labels, nil
}
