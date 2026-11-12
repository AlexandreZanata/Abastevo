// Package adapters stores scoped role periods with current database owner guards.
package adapters

import (
	"context"
	"errors"
	"math"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaccess"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

var _ app.Repository = (*Store)(nil)
var _ app.TransactionRepository = (*Store)(nil)

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
func safe(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, kernel.ErrInvalidOrgScope) {
		return d.ErrAssignmentScope
	}
	for _, known := range []error{d.ErrAssignmentScope, d.ErrAssignmentInvalid, d.ErrAssignmentConflict, d.ErrAssignmentEnded, d.ErrUnknownRole, app.ErrNotFound, kernel.ErrLastOrgOwner, kernel.ErrOrgAccessPolicyUnavailable} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P7501":
			return kernel.ErrLastOrgOwner
		case "P7502":
			return kernel.ErrOrgAccessPolicyUnavailable
		case "23505":
			return d.ErrAssignmentConflict
		case "23503":
			return app.ErrNotFound
		case "23514", "22P02", "22008":
			return d.ErrAssignmentInvalid
		case "42501":
			return d.ErrAssignmentScope
		case "55000":
			return d.ErrAssignmentEnded
		}
	}
	return app.ErrUnavailable
}
func validateID(scope kernel.OrgScope, raw string) error {
	if scope.Validate() != nil {
		return d.ErrAssignmentScope
	}
	if _, err := kernel.ParseOrgID(raw); err != nil {
		return d.ErrAssignmentInvalid
	}
	return nil
}
func check(ctx context.Context, tx pgx.Tx, s kernel.OrgScope) error {
	return safe(ctx, database.ValidateOrgTx(ctx, tx, s))
}
func run[T any](ctx context.Context, s *Store, scope kernel.OrgScope, work func(pgx.Tx) (T, error)) (T, error) {
	var zero, result T
	if scope.Validate() != nil {
		return zero, d.ErrAssignmentScope
	}
	if s == nil || s.pool == nil {
		return zero, app.ErrUnavailable
	}
	err := database.WithOrgTx(ctx, s.pool, scope, func(tx pgx.Tx) error { var err error; result, err = work(tx); return err })
	if err != nil {
		return zero, safe(ctx, err)
	}
	return result, nil
}
func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC().Truncate(time.Microsecond), Valid: !t.IsZero()}
}
func value(scope kernel.OrgScope, row q.GetAssignmentRow) (d.Assignment, error) {
	if !row.ValidFrom.Valid || row.ValidFrom.InfinityModifier != pgtype.Finite || row.ValidTo.InfinityModifier != pgtype.Finite || row.EndedAt.InfinityModifier != pgtype.Finite {
		return d.Assignment{}, app.ErrUnavailable
	}
	v := d.AssignmentSnapshot{OrgID: scope.OrgID(), ID: row.ID, MembershipID: row.MembershipID, AssignedBy: row.AssignedBy, Role: d.Role(row.Role), Scope: d.ScopeType(row.ScopeType), TargetID: row.ScopeTarget, ValidFrom: row.ValidFrom.Time, Reason: row.Reason, Revision: row.Revision}
	if row.ValidTo.Valid {
		v.ValidTo = row.ValidTo.Time
	}
	if row.EndedAt.Valid {
		v.EndedAt = row.EndedAt.Time
	}
	result, err := d.RehydrateAssignment(scope, v)
	if err != nil {
		return d.Assignment{}, app.ErrUnavailable
	}
	return result, nil
}
func lock(ctx context.Context, tx pgx.Tx, s kernel.OrgScope, member string, requireBound bool) error {
	queries := q.New(tx)
	account, err := queries.AssignmentAccount(ctx, q.AssignmentAccountParams{OrgID: s.OrgID().String(), MembershipID: member})
	if err != nil {
		return safe(ctx, err)
	}
	// A concurrent first binding could otherwise make the INSERT trigger
	// acquire a newly bound account after this adapter already locked the root.
	if account == "" && requireBound {
		return d.ErrAssignmentInvalid
	}
	if account != "" {
		if err := queries.LockAssignmentAccount(ctx, account); err != nil {
			return safe(ctx, err)
		}
	}
	_, err = queries.LockAssignmentRoot(ctx, s.OrgID().String())
	return safe(ctx, err)
}
func (s *Store) Get(ctx context.Context, scope kernel.OrgScope, id string) (d.Assignment, error) {
	if err := validateID(scope, id); err != nil {
		return d.Assignment{}, err
	}
	return run(ctx, s, scope, func(tx pgx.Tx) (d.Assignment, error) { return s.GetTx(ctx, tx, scope, id) })
}
func (s *Store) GetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.Assignment, error) {
	if err := validateID(scope, id); err != nil {
		return d.Assignment{}, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return d.Assignment{}, err
	}
	row, err := q.New(tx).GetAssignment(ctx, q.GetAssignmentParams{OrgID: scope.OrgID().String(), ID: id})
	if err != nil {
		return d.Assignment{}, safe(ctx, err)
	}
	return value(scope, row)
}
func (s *Store) Create(ctx context.Context, scope kernel.OrgScope, a d.Assignment) (bool, error) {
	if _, err := initial(scope, a); err != nil {
		return false, err
	}
	return run(ctx, s, scope, func(tx pgx.Tx) (bool, error) { return s.CreateTx(ctx, tx, scope, a) })
}
func initial(scope kernel.OrgScope, a d.Assignment) (d.AssignmentSnapshot, error) {
	v, err := a.Snapshot(scope)
	if err != nil {
		return d.AssignmentSnapshot{}, err
	}
	if v.Revision != 1 || !v.EndedAt.IsZero() {
		return d.AssignmentSnapshot{}, d.ErrAssignmentInvalid
	}
	return v, nil
}
func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a d.Assignment) (bool, error) {
	v, err := initial(scope, a)
	if err != nil {
		return false, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return false, err
	}
	// Original intent is immutable. An exact historical replay is read-only,
	// including after account erasure; it neither binds nor grants access.
	if found, err := s.creationReplay(ctx, tx, scope, v); found || err != nil {
		return false, err
	}
	if err := lock(ctx, tx, scope, v.MembershipID, true); err != nil {
		return false, err
	}
	// Recheck after the root lock to converge concurrent first creates.
	if found, err := s.creationReplay(ctx, tx, scope, v); found || err != nil {
		return false, err
	}
	changed, err := q.New(tx).InsertAssignment(ctx, q.InsertAssignmentParams{OrgID: scope.OrgID().String(), ID: v.ID, MembershipID: v.MembershipID, Role: string(v.Role), ScopeType: string(v.Scope), ScopeTarget: v.TargetID, ValidFrom: stamp(v.ValidFrom), ValidTo: stamp(v.ValidTo), AssignedBy: v.AssignedBy, Reason: v.Reason})
	if err != nil {
		return false, safe(ctx, err)
	}
	return changed == 1, nil
}
func (s *Store) creationReplay(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, v d.AssignmentSnapshot) (bool, error) {
	existing, err := s.GetTx(ctx, tx, scope, v.ID)
	if err == nil {
		before, err := existing.Snapshot(scope)
		if err != nil {
			return false, app.ErrUnavailable
		}
		if sameIntent(before, v) {
			return true, nil
		}
		return false, d.ErrAssignmentConflict
	}
	if err != app.ErrNotFound {
		return false, err
	}
	return false, nil
}
func sameIntent(a, b d.AssignmentSnapshot) bool {
	return a.ID == b.ID && a.MembershipID == b.MembershipID && a.Role == b.Role && a.Scope == b.Scope && a.TargetID == b.TargetID && a.AssignedBy == b.AssignedBy && a.Reason == b.Reason && stamp(a.ValidFrom) == stamp(b.ValidFrom) && stamp(a.ValidTo) == stamp(b.ValidTo)
}

type endResult struct {
	assignment d.Assignment
	changed    bool
}

func (s *Store) End(ctx context.Context, scope kernel.OrgScope, id string, expected int64) (d.Assignment, bool, error) {
	if err := endInput(scope, id, expected); err != nil {
		return d.Assignment{}, false, err
	}
	v, err := run(ctx, s, scope, func(tx pgx.Tx) (endResult, error) {
		a, changed, err := s.EndTx(ctx, tx, scope, id, expected)
		return endResult{a, changed}, err
	})
	return v.assignment, v.changed, err
}
func endInput(s kernel.OrgScope, id string, expected int64) error {
	if err := validateID(s, id); err != nil {
		return err
	}
	if expected < 1 || expected == math.MaxInt64 {
		return d.ErrAssignmentInvalid
	}
	return nil
}
func (s *Store) EndTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string, expected int64) (d.Assignment, bool, error) {
	if err := endInput(scope, id, expected); err != nil {
		return d.Assignment{}, false, err
	}
	a, err := s.GetTx(ctx, tx, scope, id)
	if err != nil {
		return d.Assignment{}, false, err
	}
	v, err := a.Snapshot(scope)
	if err != nil {
		return d.Assignment{}, false, app.ErrUnavailable
	}
	if err := lock(ctx, tx, scope, v.MembershipID, false); err != nil {
		return d.Assignment{}, false, err
	}
	// Re-read after the root lock; a concurrent end may have committed.
	a, err = s.GetTx(ctx, tx, scope, id)
	if err != nil {
		return d.Assignment{}, false, err
	}
	v, err = a.Snapshot(scope)
	if err != nil {
		return d.Assignment{}, false, app.ErrUnavailable
	}
	if !v.EndedAt.IsZero() {
		if v.Revision == expected+1 {
			return a, false, nil
		}
		return d.Assignment{}, false, d.ErrAssignmentConflict
	}
	if v.Revision != expected {
		return d.Assignment{}, false, d.ErrAssignmentConflict
	}
	changed, err := q.New(tx).EndAssignment(ctx, q.EndAssignmentParams{OrgID: scope.OrgID().String(), ID: id, ExpectedRevision: expected})
	if err != nil {
		return d.Assignment{}, false, safe(ctx, err)
	}
	if changed != 1 {
		return d.Assignment{}, false, d.ErrAssignmentConflict
	}
	a, err = s.GetTx(ctx, tx, scope, id)
	if err != nil {
		return d.Assignment{}, false, err
	}
	return a, true, nil
}
func (s *Store) Candidates(ctx context.Context, scope kernel.OrgScope, member string) ([]d.Assignment, error) {
	if err := validateID(scope, member); err != nil {
		return nil, err
	}
	return run(ctx, s, scope, func(tx pgx.Tx) ([]d.Assignment, error) { return s.CandidatesTx(ctx, tx, scope, member) })
}
func (s *Store) CandidatesTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member string) ([]d.Assignment, error) {
	if err := validateID(scope, member); err != nil {
		return nil, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return nil, err
	}
	rows, err := q.New(tx).CandidateAssignments(ctx, q.CandidateAssignmentsParams{OrgID: scope.OrgID().String(), MembershipID: member})
	if err != nil {
		return nil, safe(ctx, err)
	}
	out := make([]d.Assignment, 0, len(rows))
	for _, row := range rows {
		a, err := value(scope, q.GetAssignmentRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
