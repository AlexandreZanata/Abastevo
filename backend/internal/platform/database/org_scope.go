package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// ErrUnsafeOrgRole rejects runtime logins with RLS ownership or privileged roles.
var ErrUnsafeOrgRole = errors.New("database: unsafe organization runtime role")

var ErrInvalidOrgTx = errors.New("database: missing organization transaction")

// WithOrgTx executes private repository work in one transaction, with scope
// limited to that transaction. The caller resolves current authority first.
// Runtime logins must own no RLS tables and have no privileged role memberships.
// The callback uses tx for all database work and propagates failures to roll back.
// The level is pinned to READ COMMITTED whatever the session default: the
// last-owner and overlap guards serialize on the root lock and re-read current
// rows, which a REPEATABLE READ snapshot would not see (write skew).
func WithOrgTx(ctx context.Context, pool *pgxpool.Pool, scope kernel.OrgScope, work func(pgx.Tx) error) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	return pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		if err := requireSafeOrgRole(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_catalog.set_config('anpfuel.org_id', $1, true)`, scope.OrgID().String()); err != nil {
			return fmt.Errorf("database: set organization scope: %w", err)
		}
		return work(tx)
	})
}

// ValidateOrgTx checks a caller-owned composition transaction without changing
// its scope or lifecycle. Explicit-Tx repositories cannot bypass the role guard.
func ValidateOrgTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if tx == nil {
		return ErrInvalidOrgTx
	}
	if err := requireSafeOrgRole(ctx, tx); err != nil {
		return err
	}
	var installed, level string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(current_setting('anpfuel.org_id',true),''),current_setting('transaction_isolation')`).Scan(&installed, &level); err != nil {
		return fmt.Errorf("database: check installed organization scope: %w", err)
	}
	if level != "read committed" {
		return ErrInvalidOrgTx
	}
	if installed != scope.OrgID().String() {
		return kernel.ErrInvalidOrgScope
	}
	return nil
}

func requireSafeOrgRole(ctx context.Context, tx pgx.Tx) error {
	var unsafe bool
	err := tx.QueryRow(ctx, `
			SELECT current_user <> session_user OR EXISTS (
				SELECT 1 FROM pg_catalog.pg_roles r
				WHERE pg_catalog.pg_has_role(session_user, r.oid, 'MEMBER')
				AND (r.rolsuper OR r.rolbypassrls OR r.rolcreaterole)
			) OR EXISTS (
				SELECT 1 FROM pg_catalog.pg_class c
				WHERE c.relrowsecurity
				AND pg_catalog.pg_has_role(session_user, c.relowner, 'MEMBER')
			)`).Scan(&unsafe)
	if err != nil {
		return fmt.Errorf("database: check organization runtime role: %w", err)
	}
	if unsafe {
		return ErrUnsafeOrgRole
	}
	return nil
}
