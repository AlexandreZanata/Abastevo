//go:build integration

package adapters_test

import (
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestCompanyMigrationUpgradeFailureRetryAndPreviousCompatibility(t *testing.T) {
	const name = "000167_company_web_sessions.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= name {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	owner, family, station := f.Organizations[0].ID, f.Organizations[0].ParentID, f.Organizations[0].RecordID
	now := time.Now().UTC().Truncate(time.Second)
	salt := strings.Repeat("a", 32)
	hash := strings.Repeat("b", 64)
	if _, err := f.Admin.Exec(ctx, "INSERT INTO accounts(id,alias,status) VALUES($1,'synthetic-previous-company','active')", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at)
	VALUES($1,$2,$3,$4,$3,$4,$5,$6)`, family, owner, salt, hash, now.Add(15*time.Minute), now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic previous station')", station); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('company_web_sessions') IS NULL
		AND to_regclass('account_session_families_company_owner_idx') IS NULL
		AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000167')`).Scan(&clean); err != nil || !clean {
		t.Fatal("failed migration left schema or ledger", err)
	}
	current := fstest.MapFS{}
	for name, file := range previous {
		current[name] = file
	}
	current[name] = &fstest.MapFile{Data: raw}
	applied, err := migrate.Apply(ctx, f.AdminDSN, current)
	if err != nil || len(applied) != 1 || applied[0] != "000167" {
		t.Fatal(applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatal("previous chain refused new schema", err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatal("migration repeated work", applied, err)
	}
	var count int
	if err := f.Admin.QueryRow(ctx, "SELECT count(*) FROM company_web_sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("migration seeded credentials", err)
	}
	// The original family/account records survive, and its former DELETE
	// remains valid with the new child table through the composite cascade.
	if _, err := f.Admin.Exec(ctx, `INSERT INTO company_web_sessions
	 (id,account_id,family_id,token_lookup,token_salt,token_hash,csrf_salt,csrf_hash,authenticated_at,created_at,last_seen_at,expires_at)
	 VALUES($1,$2,$3,$4,$5,$4,$5,$4,$6,$6,$6,$7)`, station, owner, family, hash, salt, now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	other := f.Organizations[1].ID
	if _, err := f.Admin.Exec(ctx, "INSERT INTO accounts(id,alias,status) VALUES($1,'synthetic-other-owner','active')", other); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, "UPDATE company_web_sessions SET account_id=$1", other); err == nil {
		t.Fatal("cross-account family reference accepted")
	}
	if _, err := f.Admin.Exec(ctx, "DELETE FROM account_session_families WHERE id=$1", family); err != nil {
		t.Fatal("previous family cleanup became incompatible", err)
	}
	if err := f.Admin.QueryRow(ctx, "SELECT count(*) FROM company_web_sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("orphaned cookie after family cleanup", err)
	}
	var label string
	if err := f.Admin.QueryRow(ctx, "SELECT display_name FROM directory_stations WHERE id=$1", station).Scan(&label); err != nil || label != "Synthetic previous station" {
		t.Fatal("public data changed", err)
	}
	floor, err := fs.ReadFile(migrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("compatibility floor changed", err)
	}
}
