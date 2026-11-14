// Package adapters persists scoped identity and membership history under RLS.
package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
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
		return d.ErrScope
	}
	for _, known := range []error{d.ErrScope, d.ErrInvalid, d.ErrConflict, d.ErrRevision, d.ErrEnded, app.ErrNotFound, kernel.ErrLastOrgOwner, kernel.ErrOrgAccessPolicyUnavailable} {
		if errors.Is(err, known) {
			return known
		}
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P7501":
			return kernel.ErrLastOrgOwner
		case "P7502":
			return kernel.ErrOrgAccessPolicyUnavailable
		case "23505":
			return d.ErrConflict
		case "23503":
			return app.ErrNotFound
		case "23514", "22P02":
			return d.ErrInvalid
		case "42501":
			return d.ErrScope
		case "55000":
			return d.ErrEnded
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return app.ErrUnavailable
}
func inScope[T any](ctx context.Context, s *Store, scope kernel.OrgScope, work func(pgx.Tx) (T, error)) (T, error) {
	var zero, result T
	if scope.Validate() != nil {
		return zero, d.ErrScope
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
func id(scope kernel.OrgScope, raw string) (string, error) {
	if scope.Validate() != nil {
		return "", d.ErrScope
	}
	value, err := kernel.ParseOrgID(raw)
	if err != nil {
		return "", d.ErrInvalid
	}
	return value.String(), nil
}
func check(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope) error {
	return safe(ctx, database.ValidateOrgDBTX(ctx, tx, scope))
}
func employeeInitial(scope kernel.OrgScope, e d.Employee) (d.EmployeeSnapshot, error) {
	v, err := e.Snapshot(scope)
	if err != nil {
		return d.EmployeeSnapshot{}, err
	}
	if v.State != d.EmployeeActive || v.Revision != 1 || v.MergedInto != "" {
		return d.EmployeeSnapshot{}, d.ErrInvalid
	}
	return v, nil
}
func initial(scope kernel.OrgScope, m d.Membership) (d.MembershipSnapshot, error) {
	v, err := m.Snapshot(scope)
	if err != nil {
		return d.MembershipSnapshot{}, err
	}
	if v.State != d.MembershipInvited || v.Revision != 1 || v.AccountTombstone != "" {
		return d.MembershipSnapshot{}, d.ErrInvalid
	}
	return v, nil
}
func advance(scope kernel.OrgScope, expected int64, m d.Membership) (d.MembershipSnapshot, error) {
	v, err := m.Snapshot(scope)
	if err != nil {
		return d.MembershipSnapshot{}, err
	}
	if expected == math.MaxInt64 {
		return d.MembershipSnapshot{}, d.ErrRevision
	}
	if expected < 1 || v.Revision != expected+1 || v.AccountTombstone != "" {
		return d.MembershipSnapshot{}, d.ErrInvalid
	}
	return v, nil
}
func accountID(scope kernel.OrgScope, v d.MembershipSnapshot) string {
	if v.Account == (d.AccountRef{}) {
		return ""
	}
	raw, _, _ := v.Account.Reference(scope)
	return raw
}

func (s *Store) GetEmployee(ctx context.Context, scope kernel.OrgScope, raw string) (d.Employee, error) {
	key, err := id(scope, raw)
	if err != nil {
		return d.Employee{}, err
	}
	return inScope(ctx, s, scope, func(tx pgx.Tx) (d.Employee, error) { return s.GetEmployeeTx(ctx, tx, scope, key) })
}
func (s *Store) GetEmployeeTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, raw string) (d.Employee, error) {
	return s.GetEmployeeDBTX(ctx, tx, scope, raw)
}

func (s *Store) GetEmployeeDBTX(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, raw string) (d.Employee, error) {
	key, err := id(scope, raw)
	if err != nil {
		return d.Employee{}, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return d.Employee{}, err
	}
	row, err := q.New(tx).GetEmployee(ctx, q.GetEmployeeParams{OrgID: scope.OrgID().String(), ID: key})
	if err != nil {
		return d.Employee{}, safe(ctx, err)
	}
	result, err := d.RehydrateEmployee(scope, d.EmployeeSnapshot{OrgID: scope.OrgID(), ID: row.ID, State: d.EmployeeState(row.State), Revision: row.Revision, MergedInto: row.MergedInto})
	if err != nil {
		return d.Employee{}, app.ErrUnavailable
	}
	return result, nil
}
func (s *Store) CreateEmployee(ctx context.Context, scope kernel.OrgScope, e d.Employee) (bool, error) {
	if _, err := employeeInitial(scope, e); err != nil {
		return false, err
	}
	return inScope(ctx, s, scope, func(tx pgx.Tx) (bool, error) { return s.CreateEmployeeTx(ctx, tx, scope, e) })
}
func (s *Store) CreateEmployeeTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, e d.Employee) (bool, error) {
	v, err := employeeInitial(scope, e)
	if err != nil {
		return false, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return false, err
	}
	n, err := q.New(tx).InsertEmployee(ctx, q.InsertEmployeeParams{OrgID: scope.OrgID().String(), ID: v.ID})
	if err != nil {
		return false, safe(ctx, err)
	}
	if n == 1 {
		return true, nil
	}
	prior, err := s.GetEmployeeTx(ctx, tx, scope, v.ID)
	if err != nil {
		return false, err
	}
	before, _ := prior.Snapshot(scope)
	if before == v {
		return false, nil
	}
	return false, d.ErrConflict
}
func (s *Store) Get(ctx context.Context, scope kernel.OrgScope, raw string) (d.Membership, error) {
	key, err := id(scope, raw)
	if err != nil {
		return d.Membership{}, err
	}
	return inScope(ctx, s, scope, func(tx pgx.Tx) (d.Membership, error) { return s.GetTx(ctx, tx, scope, key) })
}
func (s *Store) GetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, raw string) (d.Membership, error) {
	key, err := id(scope, raw)
	if err != nil {
		return d.Membership{}, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return d.Membership{}, err
	}
	return read(ctx, tx, scope, key)
}
func read(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, key string) (d.Membership, error) {
	row, err := q.New(tx).GetMembership(ctx, q.GetMembershipParams{OrgID: scope.OrgID().String(), ID: key})
	if err != nil {
		return d.Membership{}, safe(ctx, err)
	}
	var account d.AccountRef
	if row.AccountID != "" {
		switch d.AccessModel(row.AccessModel) {
		case d.ExistingAccount:
			account, err = d.PersonalAccount(row.AccountID)
		case d.ManagedAccess:
			account, err = d.ManagedAccount(scope, row.AccountID)
		default:
			err = d.ErrInvalid
		}
		if err != nil {
			return d.Membership{}, app.ErrUnavailable
		}
	}
	result, err := d.RehydrateMembership(scope, d.MembershipSnapshot{OrgID: scope.OrgID(), ID: row.ID, EmployeeID: row.EmployeeID, Account: account, Model: d.AccessModel(row.AccessModel), State: d.MembershipState(row.State), Revision: row.Revision, Suspension: d.SuspensionOrigin(row.SuspensionOrigin), EndReason: d.EndReason(row.EndReason), AccountTombstone: row.AccountTombstone})
	if err != nil {
		return d.Membership{}, app.ErrUnavailable
	}
	return result, nil
}
func (s *Store) ActiveByAccount(ctx context.Context, scope kernel.OrgScope, raw string) (d.Membership, error) {
	key, err := id(scope, raw)
	if err != nil {
		return d.Membership{}, err
	}
	return inScope(ctx, s, scope, func(tx pgx.Tx) (d.Membership, error) { return s.ActiveByAccountTx(ctx, tx, scope, key) })
}
func (s *Store) ActiveByAccountTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, raw string) (d.Membership, error) {
	return s.ActiveByAccountDBTX(ctx, tx, scope, raw)
}

func (s *Store) ActiveByAccountDBTX(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, raw string) (d.Membership, error) {
	key, err := id(scope, raw)
	if err != nil {
		return d.Membership{}, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return d.Membership{}, err
	}
	member, err := q.New(tx).ActiveMembershipID(ctx, q.ActiveMembershipIDParams{OrgID: scope.OrgID().String(), AccountID: key})
	if err != nil {
		return d.Membership{}, safe(ctx, err)
	}
	return read(ctx, tx, scope, member)
}
func (s *Store) Create(ctx context.Context, scope kernel.OrgScope, m d.Membership) (bool, error) {
	if _, err := initial(scope, m); err != nil {
		return false, err
	}
	return inScope(ctx, s, scope, func(tx pgx.Tx) (bool, error) { return s.CreateTx(ctx, tx, scope, m) })
}
func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, m d.Membership) (bool, error) {
	v, err := initial(scope, m)
	if err != nil {
		return false, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return false, err
	}
	n, err := q.New(tx).InsertMembership(ctx, q.InsertMembershipParams{OrgID: scope.OrgID().String(), ID: v.ID, EmployeeID: v.EmployeeID, AccountID: accountID(scope, v), AccessModel: string(v.Model)})
	if err != nil {
		return false, safe(ctx, err)
	}
	if n == 1 {
		return true, nil
	}
	prior, err := read(ctx, tx, scope, v.ID)
	if err != nil {
		return false, err
	}
	before, _ := prior.Snapshot(scope)
	if before == v {
		return false, nil
	}
	return false, d.ErrConflict
}
func (s *Store) Save(ctx context.Context, scope kernel.OrgScope, expected int64, m d.Membership) (bool, error) {
	if _, err := advance(scope, expected, m); err != nil {
		return false, err
	}
	return inScope(ctx, s, scope, func(tx pgx.Tx) (bool, error) { return s.SaveTx(ctx, tx, scope, expected, m) })
}
func (s *Store) SaveTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, expected int64, m d.Membership) (bool, error) {
	v, err := advance(scope, expected, m)
	if err != nil {
		return false, err
	}
	if err := check(ctx, tx, scope); err != nil {
		return false, err
	}
	n, err := q.New(tx).AdvanceMembership(ctx, q.AdvanceMembershipParams{OrgID: scope.OrgID().String(), ID: v.ID, AccountID: accountID(scope, v), State: string(v.State), Revision: v.Revision, SuspensionOrigin: string(v.Suspension), EndReason: string(v.EndReason), ExpectedRevision: expected})
	if err != nil {
		return false, safe(ctx, err)
	}
	if n == 1 {
		return true, nil
	}
	prior, err := read(ctx, tx, scope, v.ID)
	if err != nil {
		return false, err
	}
	before, _ := prior.Snapshot(scope)
	if before == v {
		return false, nil
	}
	if before.State == d.MembershipEnded {
		return false, d.ErrEnded
	}
	return false, d.ErrConflict
}
