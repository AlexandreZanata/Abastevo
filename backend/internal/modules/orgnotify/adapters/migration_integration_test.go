//go:build integration

package adapters_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	notifypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// BUC-EMP-111: populated previous chain -> 000204. A failed apply leaves no
// table, function or ledger row; retry converges, the previous chain stays
// compatible and nothing is seeded. The version derives from one file name so
// a coordinator renumbering is a one-line change.
func TestNotificationMigrationPopulatedUpgradeFailureRecovery(t *testing.T) {
	name := notifypg.Migration
	version := strings.SplitN(name, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= name || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	// Populate the previous image with the facts the new tables reference,
	// through the ordinary runtime role and the real organization root store.
	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+r+";GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships TO "+r+";GRANT SELECT,UPDATE ON accounts TO "+r+";GRANT SELECT ON payment_billing_exits TO "+r); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO accounts(id,alias,status) VALUES($1,'synthetic-upgrade','active')`, key(1101)); err != nil {
		t.Fatal(err)
	}
	populated := scopeOf(t, 1401)
	root, err := org.NewPending(populated, org.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orgpg.NewStore(f.Runtime).Create(ctx, populated, root); err != nil {
		t.Fatal(err)
	}
	orgID := populated.OrgID().String()
	if err := database.WithOrgTx(ctx, f.Runtime, populated, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, orgID, key(1201)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, orgID, key(1001), key(1201), key(1101))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO job_queue(id,kind,payload,dedupe_key,max_attempts) VALUES($1,'synthetic-previous','{"version":1}','previous-notify',5)`, key(1301)); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array(
			(SELECT jsonb_agg(to_jsonb(m) ORDER BY org_id,id) FROM org_memberships m),
			(SELECT jsonb_agg(to_jsonb(e) ORDER BY org_id,id) FROM org_employee_identities e),
			(SELECT jsonb_agg(jsonb_build_object('id',id,'kind',kind,'status',status) ORDER BY id) FROM job_queue))::text`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := snapshot()
	raw, err := fs.ReadFile(dbmigrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_notification_requests') IS NULL AND to_regclass('org_notification_notices') IS NULL
		AND to_regclass('org_notification_deliveries') IS NULL AND to_regclass('org_notification_preferences') IS NULL
		AND to_regclass('org_notification_recipients') IS NULL AND to_regclass('org_notification_budgets') IS NULL
		AND to_regclass('org_notification_commands') IS NULL AND to_regprocedure('org_notification_append_only()') IS NULL
		AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean {
		t.Fatal("failed migration left tables, functions or ledger", err)
	}
	if snapshot() != before {
		t.Fatal("failed migration changed previous facts")
	}
	current := fstest.MapFS{}
	for filename, file := range previous {
		current[filename] = file
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("retry failed", applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatal("previous schema chain incompatible", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatal("migration not idempotent", applied, err)
	}
	if snapshot() != before {
		t.Fatal("upgrade changed previous facts")
	}
	var seeded int
	if err := f.Admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM org_notification_requests)+(SELECT count(*) FROM org_notification_recipients)
		+(SELECT count(*) FROM org_notification_notices)+(SELECT count(*) FROM org_notification_deliveries)
		+(SELECT count(*) FROM org_notification_preferences)+(SELECT count(*) FROM org_notification_budgets)
		+(SELECT count(*) FROM org_notification_commands)+(SELECT count(*) FROM job_queue WHERE kind<>'synthetic-previous')`).Scan(&seeded); err != nil || seeded != 0 {
		t.Fatal("migration seeded work", seeded, err)
	}
	// The upgraded schema references the previous facts it must keep.
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_notification_preferences(org_id,membership_id,kind,muted,revision) VALUES($1,$2,'test.budget.alert',true,1)`, orgID, key(1001)); err != nil {
		t.Fatal("upgraded schema rejects a previous membership", err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_notification_preferences(org_id,membership_id,kind,muted,revision) VALUES($1,$2,'test.budget.alert',true,1)`, f.Organizations[1].ID, key(1001)); !sqlState(err, "23503") {
		t.Fatal("foreign membership accepted after upgrade", err)
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("compatibility floor changed")
	}
}
