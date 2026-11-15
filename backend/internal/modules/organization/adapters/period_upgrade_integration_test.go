//go:build integration

package adapters_test

import (
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// periodMigration is the only place naming the EMP-APPR-006 migration; a
// renumbering at integration changes this line only.
const periodMigration = "000203_finance_periods.sql"

// Populated previous head -> period schema: an injected failure changes
// nothing, the ordinary migrator recovers, a repeat is a no-op, the previous
// embedded image stays compatible and the upgraded organization works.
func TestPeriodPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	version := periodMigration[:6]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= periodMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	raw, err := fs.ReadFile(migrations.Files, periodMigration)
	if err != nil {
		t.Fatal("period migration missing", err)
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	other := legalFixtureFor(t, f, onboarding, actor, flags)
	periodUnit(t, fx, "America/Sao_Paulo")
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_units x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,revision) FROM org_settings_versions x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,capability) FROM org_capabilities x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM accounts x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_period_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,org_audit_events,org_memberships,accounts TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	broken := fstest.MapFS{periodMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected period migration failure was not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_finance_periods') IS NULL AND to_regclass('org_finance_period_transitions') IS NULL
	 AND NOT EXISTS(SELECT 1 FROM pg_proc WHERE proname LIKE 'org_finance_period%') AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || snapshot() != before {
		t.Fatal("failed period migration was not atomic", err)
	}
	current := fstest.MapFS{}
	for name, data := range previous {
		current[name] = data
	}
	current[periodMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary period migration recovery", err)
	}
	if snapshot() != before {
		t.Fatal("period upgrade changed prior organization rows")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("period upgrade no-op repeat", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid=ANY(ARRAY['org_finance_periods'::regclass,'org_finance_period_transitions'::regclass])`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("period tables lack ordinary ownership or forced RLS", err)
	}
	// Existing organizations start with every month OPEN; the upgraded
	// commands, history and gate work on the populated data.
	periodGrant(t, f)
	store := periodStoreFor(t, fx)
	if v, err := store.ReadPeriod(t.Context(), fx.scope, periodOwner(fx), fx.member, mustMonth(t, "2026-10")); err != nil || v.State != period.Open || v.Revision != 1 {
		t.Fatal("upgraded organization period read", err)
	}
	if _, _, err := periodWrite(t, fx, store, periodOwner(fx), fx.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); err != nil {
		t.Fatal("upgraded close", err)
	}
	if _, _, err := periodWrite(t, fx, store, periodOwner(fx), fx.member, periodCmd(t, period.ActionReopen, "2026-10", 2, "Synthetic post-upgrade reopening")); err != nil {
		t.Fatal("upgraded reopen", err)
	}
	if fact := periodGate(t, fx, "2026-10-31"); fact.State != period.Reopened || fact.Reopening == nil || !fact.Writable() {
		t.Fatal("upgraded gate")
	}
	if fact := periodGate(t, other, "2026-10-31"); fact.State != period.Open {
		t.Fatal("upgrade or lock leaked into another organization")
	}
}
