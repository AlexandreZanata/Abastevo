//go:build integration

package adapters

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
)

// TestMigrationUpgrade_AdminBatchAppliesOverPreviousSchema is the recovery
// check for the admin-suite migrations (000140 onward): over a world that
// stopped before them they apply in order, a replay applies nothing, and the
// audit ledger stays append-only after the upgrade.
func TestMigrationUpgrade_AdminBatchAppliesOverPreviousSchema(t *testing.T) {
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("disposable database unavailable")
	}
	name := fmt.Sprintf("admin_upgrade_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Error("cleanup connection failed")
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name

	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	previous, target := fstest.MapFS{}, fstest.MapFS{}
	var expected []string
	for _, file := range names {
		raw, err := fs.ReadFile(migrations.Files, file)
		if err != nil {
			t.Fatal(err)
		}
		target[file] = &fstest.MapFile{Data: raw}
		if file < "000140_" {
			previous[file] = &fstest.MapFile{Data: raw}
		} else {
			expected = append(expected, file[:6])
		}
	}
	if len(expected) == 0 || expected[0] != "000140" {
		t.Fatalf("admin batch migrations missing: %v", expected)
	}
	if _, err := migrate.Apply(ctx, u.String(), previous); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, u.String(), target)
	if err != nil || !slices.Equal(applied, expected) {
		t.Fatalf("upgrade applied=%v expected=%v err=%v", applied, expected, err)
	}
	again, err := migrate.Apply(ctx, u.String(), target)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay applied=%v err=%v", again, err)
	}

	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, table := range []string{"admin_sessions", "admin_login_attempts", "admin_idempotency", "admin_audit_events"} {
		var exists bool
		if err := conn.QueryRow(ctx, "SELECT to_regclass('public.'||$1) IS NOT NULL", table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s exists=%v err=%v", table, exists, err)
		}
	}
	if _, err := conn.Exec(ctx, `INSERT INTO admin_audit_events (action, target_type) VALUES ('admin.synthetic', 'admin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `UPDATE admin_audit_events SET reason = 'changed'`); err == nil {
		t.Fatal("audit ledger accepted an update after upgrade")
	}
}
