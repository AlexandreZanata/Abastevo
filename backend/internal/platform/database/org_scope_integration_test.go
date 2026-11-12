//go:build integration

package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func organizationTemplate(t *testing.T) fstest.MapFS {
	t.Helper()
	raw, err := os.ReadFile("../../../db/templates/organization_private.sql")
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{"999900_org_scope_template.sql": &fstest.MapFile{Data: raw}}
}

func seedOrganizationTemplate(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	ctx := t.Context()
	if _, err := migrate.Apply(ctx, f.AdminDSN, organizationTemplate(t)); err != nil {
		t.Fatal(err)
	}
	for _, org := range f.Organizations {
		if _, err := f.Admin.Exec(ctx, `INSERT INTO org_example_parent (org_id, id, number) VALUES ($1, $2, $3)`,
			org.ID, org.ParentID, org.Number); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Admin.Exec(ctx, `INSERT INTO org_example_record (org_id, id, parent_id, number) VALUES ($1, $2, $3, $4)`,
			org.ID, org.RecordID, org.ParentID, org.Number); err != nil {
			t.Fatal(err)
		}
	}
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+
		"; GRANT SELECT, INSERT, UPDATE, DELETE ON org_example_parent, org_example_record TO "+role); err != nil {
		t.Fatal(err)
	}
}

func expectSQLState(t *testing.T, err error, state string) {
	t.Helper()
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != state {
		t.Fatalf("expected SQLSTATE %s, got %v", state, err)
	}
}

func assertNoOrganizationScope(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var scope string
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id', true), ''),
		(SELECT count(*) FROM org_example_record)`).Scan(&scope, &rows); err != nil {
		t.Fatal(err)
	}
	if scope != "" || rows != 0 {
		t.Fatalf("pooled connection retained private scope=%q rows=%d", scope, rows)
	}
}

func TestOrganizationTemplateIsolation(t *testing.T) {
	f := orgtest.New(t)
	seedOrganizationTemplate(t, f)
	a, b := f.Organizations[0], f.Organizations[1]
	ctx := t.Context()

	t.Run("ordinary login and absent scope", func(t *testing.T) {
		var role string
		var privileged bool
		if err := f.Runtime.QueryRow(ctx, `SELECT current_user, rolsuper OR rolbypassrls OR rolcreaterole
			FROM pg_roles WHERE rolname = current_user`).Scan(&role, &privileged); err != nil {
			t.Fatal(err)
		}
		if role != f.RuntimeRole || privileged {
			t.Fatal("fixture did not use a separate ordinary login")
		}
		assertNoOrganizationScope(t, f.Runtime)
		_, err := f.Runtime.Exec(ctx, `INSERT INTO org_example_record (org_id, id, parent_id, number)
			VALUES ($1, $2, $3, 'unscoped')`, a.ID, a.RecordID, a.ParentID)
		expectSQLState(t, err, "42501")
	})

	t.Run("unfiltered queries stay within each organization", func(t *testing.T) {
		for _, org := range f.Organizations {
			err := database.WithOrgTx(ctx, f.Runtime, org.Scope(t), func(tx pgx.Tx) error {
				var found string
				var count int
				if err := tx.QueryRow(ctx, `SELECT min(org_id::text), count(*) FROM org_example_record`).Scan(&found, &count); err != nil {
					return err
				}
				if found != org.ID || count != 1 {
					t.Errorf("unfiltered query crossed organization boundary")
				}
				// Explicit predicates remain the first layer, even with RLS.
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_example_record WHERE org_id=$1 AND id=$2`,
					org.ID, org.RecordID).Scan(&count); err != nil {
					return err
				}
				if count != 1 {
					t.Error("scoped record was unavailable")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			assertNoOrganizationScope(t, f.Runtime)
		}
	})

	t.Run("foreign reads updates deletes and joins expose nothing", func(t *testing.T) {
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_example_parent WHERE id=$1`, b.ParentID).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				t.Error("foreign parent was visible")
			}
			for _, query := range []string{
				`UPDATE org_example_record SET revision=revision+1 WHERE org_id=$1`,
				`DELETE FROM org_example_record WHERE org_id=$1`,
			} {
				tag, err := tx.Exec(ctx, query, b.ID)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 0 {
					t.Error("foreign mutation affected rows")
				}
			}
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_example_record r
				JOIN org_example_parent p ON p.org_id=r.org_id AND p.id=r.parent_id
				WHERE p.org_id=$1`, b.ID).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				t.Error("foreign join exposed rows")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("own insert update and delete succeed", func(t *testing.T) {
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `INSERT INTO org_example_record (org_id, id, parent_id, number)
				VALUES ($1, $2, $2, 'own')`, a.ID, a.ParentID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				t.Error("own insert did not affect one row")
			}
			for _, query := range []string{
				`UPDATE org_example_record SET revision=revision+1 WHERE org_id=$1 AND id=$2`,
				`DELETE FROM org_example_record WHERE org_id=$1 AND id=$2`,
			} {
				tag, err := tx.Exec(ctx, query, a.ID, a.ParentID)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					t.Error("own mutation did not affect one row")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("duplicate number within one organization is refused", func(t *testing.T) {
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO org_example_record (org_id, id, parent_id, number)
				VALUES ($1, $2, $2, $3)`, a.ID, a.ParentID, a.Number)
			return err
		})
		expectSQLState(t, err, "23505")
	})

	t.Run("foreign insert is refused", func(t *testing.T) {
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO org_example_record (org_id, id, parent_id, number)
				VALUES ($1, $2, $3, 'foreign')`, b.ID, a.ParentID, b.ParentID)
			return err
		})
		expectSQLState(t, err, "42501")
		assertNoOrganizationScope(t, f.Runtime)
	})

	t.Run("foreign reference is refused by composite key", func(t *testing.T) {
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO org_example_record (org_id, id, parent_id, number)
				VALUES ($1, $2, $3, 'foreign-parent')`, a.ID, a.ParentID, b.ParentID)
			return err
		})
		expectSQLState(t, err, "23503")
		assertNoOrganizationScope(t, f.Runtime)
	})

	t.Run("organization cannot be changed", func(t *testing.T) {
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE org_example_record SET org_id=$1, id=$2`, b.ID, a.ParentID)
			return err
		})
		expectSQLState(t, err, "42501")
	})

	t.Run("runtime cannot alter policy or truncate", func(t *testing.T) {
		_, err := f.Runtime.Exec(ctx, `ALTER TABLE org_example_record DISABLE ROW LEVEL SECURITY`)
		expectSQLState(t, err, "42501")
		_, err = f.Runtime.Exec(ctx, `TRUNCATE org_example_record`)
		expectSQLState(t, err, "42501")
	})

	t.Run("row security off does not bypass policy", func(t *testing.T) {
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SET LOCAL row_security=off`); err != nil {
				return err
			}
			var count int
			return tx.QueryRow(ctx, `SELECT count(*) FROM org_example_record`).Scan(&count)
		})
		expectSQLState(t, err, "42501")
	})

	t.Run("failed callback rolls back writes and scope on same connection", func(t *testing.T) {
		var before, after uint32
		if err := f.Runtime.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&before); err != nil {
			t.Fatal(err)
		}
		failure := errors.New("synthetic callback failure")
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE org_example_record SET revision=99`); err != nil {
				return err
			}
			return failure
		})
		if !errors.Is(err, failure) {
			t.Fatalf("callback failure lost: %v", err)
		}
		assertNoOrganizationScope(t, f.Runtime)
		if err := f.Runtime.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&after); err != nil || before != after {
			t.Fatal("rollback did not reuse the same physical connection")
		}
		var revision int
		if err := f.Admin.QueryRow(ctx, `SELECT revision FROM org_example_record WHERE org_id=$1`, a.ID).Scan(&revision); err != nil || revision != 1 {
			t.Fatal("failed transaction committed its change")
		}
	})

	t.Run("cancellation rolls back writes and releases pool", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		err := database.WithOrgTx(cancelCtx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			if _, err := tx.Exec(cancelCtx, `UPDATE org_example_record SET revision=99`); err != nil {
				return err
			}
			cancel()
			return cancelCtx.Err()
		})
		if err == nil {
			t.Fatal("cancelled work succeeded")
		}
		assertNoOrganizationScope(t, f.Runtime)
		var revision int
		if err := f.Admin.QueryRow(ctx, `SELECT revision FROM org_example_record WHERE org_id=$1`, a.ID).Scan(&revision); err != nil || revision != 1 {
			t.Fatal("cancelled work committed")
		}
	})

	t.Run("privileged migration pool is refused", func(t *testing.T) {
		err := database.WithOrgTx(ctx, f.Admin, a.Scope(t), func(pgx.Tx) error {
			t.Error("privileged login reached private repository work")
			return nil
		})
		if !errors.Is(err, database.ErrUnsafeOrgRole) {
			t.Fatalf("privileged login returned %v", err)
		}
	})

	for _, privilege := range []string{"BYPASSRLS", "CREATEROLE"} {
		t.Run("runtime privilege "+privilege+" is refused", func(t *testing.T) {
			role := pgx.Identifier{f.RuntimeRole}.Sanitize()
			if _, err := f.Admin.Exec(ctx, "ALTER ROLE "+role+" "+privilege); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := f.Admin.Exec(ctx, "ALTER ROLE "+role+" NO"+privilege); err != nil {
					t.Error(err)
				}
			})
			err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(pgx.Tx) error {
				t.Error("privileged runtime reached repository work")
				return nil
			})
			if !errors.Is(err, database.ErrUnsafeOrgRole) {
				t.Fatalf("unsafe runtime returned %v", err)
			}
		})
	}

	t.Run("privileged membership is refused even without inheritance", func(t *testing.T) {
		var migrationRole string
		if err := f.Admin.QueryRow(ctx, `SELECT current_user`).Scan(&migrationRole); err != nil {
			t.Fatal(err)
		}
		from, to := pgx.Identifier{migrationRole}.Sanitize(), pgx.Identifier{f.RuntimeRole}.Sanitize()
		if _, err := f.Admin.Exec(ctx, "GRANT "+from+" TO "+to+" WITH INHERIT FALSE"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := f.Admin.Exec(ctx, "REVOKE "+from+" FROM "+to); err != nil {
				t.Error(err)
			}
		})
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(pgx.Tx) error {
			t.Error("privileged membership reached repository work")
			return nil
		})
		if !errors.Is(err, database.ErrUnsafeOrgRole) {
			t.Fatalf("privileged membership returned %v", err)
		}
	})

	t.Run("forced RLS covers owner and owner login is refused", func(t *testing.T) {
		if _, err := f.Admin.Exec(ctx, "ALTER TABLE org_example_parent OWNER TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := f.Admin.Exec(ctx, "ALTER TABLE org_example_parent OWNER TO CURRENT_USER"); err != nil {
				t.Error(err)
			}
		})
		var count int
		if err := f.Runtime.QueryRow(ctx, `SELECT count(*) FROM org_example_parent`).Scan(&count); err != nil || count != 0 {
			t.Fatal("table owner bypassed forced RLS")
		}
		err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(pgx.Tx) error {
			t.Error("table owner reached repository work")
			return nil
		})
		if !errors.Is(err, database.ErrUnsafeOrgRole) {
			t.Fatalf("owner runtime returned %v", err)
		}
	})

	t.Run("concurrent organizations keep separate scopes", func(t *testing.T) {
		pool := f.RuntimePool(t, 2)
		concurrentCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ready := make(chan struct{}, 2)
		release := make(chan struct{})
		results := make(chan error, 2)
		for _, org := range f.Organizations {
			scope := org.Scope(t)
			go func() {
				results <- database.WithOrgTx(concurrentCtx, pool, scope, func(tx pgx.Tx) error {
					var selected string
					if err := tx.QueryRow(concurrentCtx, `SELECT current_setting('anpfuel.org_id')`).Scan(&selected); err != nil {
						return err
					}
					if selected != org.ID {
						return errors.New("concurrent scope crossed organizations")
					}
					ready <- struct{}{}
					select {
					case <-release:
					case <-concurrentCtx.Done():
						return concurrentCtx.Err()
					}
					tag, err := tx.Exec(concurrentCtx, `UPDATE org_example_record SET revision=revision+1`)
					if err == nil && tag.RowsAffected() != 1 {
						return errors.New("unfiltered concurrent write crossed organizations")
					}
					return err
				})
			}()
		}
		for range 2 {
			select {
			case <-ready:
			case err := <-results:
				t.Fatalf("transaction failed before concurrency barrier: %v", err)
			case <-concurrentCtx.Done():
				t.Fatal("transactions did not reach concurrency barrier")
			}
		}
		close(release)
		for range 2 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		for _, org := range f.Organizations {
			var revision int
			if err := f.Admin.QueryRow(ctx, `SELECT revision FROM org_example_record WHERE org_id=$1`, org.ID).Scan(&revision); err != nil || revision != 2 {
				t.Error("concurrent organization change was lost")
			}
		}
	})
}

func TestOrganizationTemplateMigrationRecovery(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	template := organizationTemplate(t)
	publicID := f.Organizations[0].ParentID
	if _, err := f.Admin.Exec(ctx, `INSERT INTO directory_stations (id, display_name)
		VALUES ($1, 'Synthetic prior catalog row')`, publicID); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Check(ctx, f.Admin, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	// A partially executed file must leave no tables or ledger entry.
	broken := fstest.MapFS{"999900_org_scope_template.sql": &fstest.MapFile{
		Data: append(append([]byte(nil), template["999900_org_scope_template.sql"].Data...), []byte("\nSELECT 1/0;\n")...),
	}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken migration succeeded")
	}
	var absent bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_example_record') IS NULL
		AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version='999900')`).Scan(&absent); err != nil || !absent {
		t.Fatal("partial migration survived rollback")
	}
	seedOrganizationTemplate(t, f)
	applied, err := migrate.Apply(ctx, f.AdminDSN, template)
	if err != nil || len(applied) != 0 {
		t.Fatalf("repeated template migration was not a no-op: %v", err)
	}
	// The unchanged previous application's embedded chain accepts the additive schema.
	if err := migrate.Check(ctx, f.Admin, dbmigrations.Files); err != nil {
		t.Fatalf("previous schema compatibility lost: %v", err)
	}
	var parents, records int
	if err := f.Admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM org_example_parent),
		(SELECT count(*) FROM org_example_record)`).Scan(&parents, &records); err != nil || parents != 2 || records != 2 {
		t.Fatal("retry lost seeded organization rows")
	}
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON directory_stations TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	for _, org := range f.Organizations {
		err := database.WithOrgTx(ctx, f.Runtime, org.Scope(t), func(tx pgx.Tx) error {
			var name string
			var missingPoint bool
			if err := tx.QueryRow(ctx, `SELECT display_name, current_point IS NULL
				FROM directory_stations WHERE id=$1`, publicID).Scan(&name, &missingPoint); err != nil {
				return err
			}
			if name != "Synthetic prior catalog row" || !missingPoint {
				t.Error("additive template changed previous public data")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	assertNoOrganizationScope(t, f.Runtime)
	var publicRows int
	if err := f.Runtime.QueryRow(ctx, `SELECT count(*) FROM directory_stations WHERE id=$1`, publicID).Scan(&publicRows); err != nil || publicRows != 1 {
		t.Fatal("public catalog acquired an organization boundary")
	}
}
