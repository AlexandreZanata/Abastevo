//go:build integration

package migrate

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
)

func TestAccountProfileMigrationRecoveryAndPreviousReads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dsn := freshDB(t, testDSN(t))
	const upgrade = "000302_account_recovery_profile.sql"
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	previous := fstest.MapFS{}
	for _, name := range files {
		if name >= upgrade {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: data}
	}
	if _, err := Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	const id = "0871ff80-78ca-4128-be6c-9d6512759301"
	if _, err := conn.Exec(ctx,
		"INSERT INTO accounts (id, alias, status) VALUES ($1, 'legacy-handle', 'active')", id); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO account_key_credentials
		(account_id, username_hash, key_lookup, key_salt, key_hash, created_at)
		VALUES ($1, 'synthetic-username-hash', 'synthetic-lookup', 'salt', 'synthetic-key-hash', 1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO account_addresses (address_hash, account_id) VALUES ('synthetic-email-hash', $1)", id); err != nil {
		t.Fatal(err)
	}
	migration, err := fs.ReadFile(dbmigrations.Files, upgrade)
	if err != nil {
		t.Fatal(err)
	}
	previous[upgrade] = &fstest.MapFile{Data: append(migration, []byte("\nSELECT 1 / 0;\n")...)}
	if _, err := Apply(ctx, dsn, previous); err == nil {
		t.Fatal("fault injected at the end of migration must roll back")
	}
	var clean bool
	err = conn.QueryRow(ctx, `SELECT
		to_regclass('account_recovery_audit') IS NULL
		AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE
			(table_name = 'accounts' AND column_name = 'display_name') OR
			(table_name = 'account_addresses' AND column_name = 'email_hint'))
		AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '000302')`).Scan(&clean)
	if err != nil || !clean {
		t.Fatalf("migration failure left partial state: clean=%v err=%v", clean, err)
	}
	applied, err := Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(applied) != 1 || applied[0] != "000302" {
		t.Fatalf("retry did not solely apply 000302: %v, %v", applied, err)
	}
	var alias, usernameHash string
	err = conn.QueryRow(ctx, `SELECT a.alias, k.username_hash
		FROM accounts a JOIN account_key_credentials k ON k.account_id = a.id WHERE a.id = $1`, id).
		Scan(&alias, &usernameHash)
	if err != nil || alias != "legacy-handle" || usernameHash != "synthetic-username-hash" {
		t.Fatalf("previous image read changed: %q, %q, %v", alias, usernameHash, err)
	}
	if _, err := conn.Exec(ctx, "UPDATE accounts SET display_name = 'Synthetic Author' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "UPDATE accounts SET status = 'deleted' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	var cleared bool
	if err := conn.QueryRow(ctx, "SELECT display_name IS NULL FROM accounts WHERE id = $1", id).Scan(&cleared); err != nil || !cleared {
		t.Fatalf("previous image deletion left the new name: %v, %v", cleared, err)
	}
}
