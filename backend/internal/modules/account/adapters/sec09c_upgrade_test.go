//go:build integration

package adapters

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// SEC09-C release-scoped fault/recovery: migration 000060 applies alone
// over a 000059 world with live families, preserves them byte-for-byte,
// leaves history writable, and replays empty. Append-only chain holds.
func TestRefreshHistoryUpgradeAppliesOnly000060(t *testing.T) {
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	previous := fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name == "000060_account_refresh_history.sql" {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: data}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("integration database unreachable: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("sec09c_up_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	// A live pre-upgrade family must survive byte-for-byte.
	if _, err := conn.Exec(ctx, `INSERT INTO accounts (id, alias, status) VALUES ('cccccccc-3333-4333-8333-000000000001','sat-up-1','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO account_session_families (id, account_id, refresh_salt, refresh_hash, access_salt, access_hash, access_expires, issued_at, revoked_at) VALUES ('cccccccc-3333-4333-8333-000000000011','cccccccc-3333-4333-8333-000000000001','s-old','h-old','s-acc','h-acc',now()+interval '15 minutes',now(),NULL)`); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(applied) != 1 || applied[0] != "000060" {
		t.Fatalf("upgrade %v %v", applied, err)
	}
	var tables int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename='account_session_refresh_history'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("tables %d %v", tables, err)
	}
	var kept int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM account_session_families WHERE id='cccccccc-3333-4333-8333-000000000011' AND refresh_hash='h-old' AND revoked_at IS NULL`).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("family rows %d %v", kept, err)
	}
	// History is writable immediately after the upgrade.
	if _, err := conn.Exec(ctx, `INSERT INTO account_session_refresh_history (family_id, refresh_salt, refresh_hash, rotated_at) VALUES ('cccccccc-3333-4333-8333-000000000011','s-older','h-older',now())`); err != nil {
		t.Fatalf("history write: %v", err)
	}
	again, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay %v %v", again, err)
	}
}
