//go:build integration

package database_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

// The last-owner and period-overlap guards serialize on the organization root
// lock and re-read under READ COMMITTED. Under REPEATABLE READ two transactions
// could each end a different owner from one snapshot, so organization work
// pins the level and refuses a caller transaction at any other level.
func TestOrganizationTransactionsRunAtReadCommitted(t *testing.T) {
	f := orgtest.New(t)
	scope := f.Organizations[0].Scope(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "ALTER ROLE "+role+" SET default_transaction_isolation='repeatable read'"); err != nil {
		t.Fatal(err)
	}
	pool := f.RuntimePool(t, 2)
	var level string
	if err := database.WithOrgTx(t.Context(), pool, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT current_setting('transaction_isolation')`).Scan(&level)
	}); err != nil || level != "read committed" {
		t.Fatal("organization transaction isolation", level, err)
	}
	for _, iso := range []pgx.TxIsoLevel{pgx.RepeatableRead, pgx.Serializable} {
		tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: iso})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(t.Context(), `SELECT pg_catalog.set_config('anpfuel.org_id',$1,true)`, scope.OrgID().String()); err != nil {
			t.Fatal(err)
		}
		if err := database.ValidateOrgTx(t.Context(), tx, scope); !errors.Is(err, database.ErrInvalidOrgTx) {
			t.Fatal("caller transaction accepted at", iso, err)
		}
		_ = tx.Rollback(t.Context())
	}
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err := tx.Exec(t.Context(), `SELECT pg_catalog.set_config('anpfuel.org_id',$1,true)`, scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if err := database.ValidateOrgTx(t.Context(), tx, scope); err != nil {
		t.Fatal("read committed caller transaction refused", err)
	}
}
