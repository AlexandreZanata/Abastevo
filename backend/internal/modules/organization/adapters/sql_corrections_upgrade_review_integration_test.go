//go:build integration

package adapters_test

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// The review corrections replace guard functions/triggers only. On a populated
// 000178 chain they rewrite no row, a failing correction leaves its previous
// definition and ledger, retry recovers, and the corrected guards then apply.
func TestReviewCorrectionsUpgradePopulatedChainAndRecover(t *testing.T) {
	corrections := []string{
		"000179_team_membership_access_reduction.sql",
		"000180_settings_changed_designation_availability.sql",
		"000181_legal_cnpj_all_zero.sql",
		"000182_membership_bind_while_invited.sql",
		"000183_role_period_not_retroactive.sql",
	}
	previous, current := fstest.MapFS{}, fstest.MapFS{}
	entries, e := fs.ReadDir(migrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		raw, e := fs.ReadFile(migrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		if entry.Name() == "schema_min_supported" || entry.Name() < corrections[0] {
			previous[entry.Name()] = &fstest.MapFile{Data: raw}
			current[entry.Name()] = &fstest.MapFile{Data: raw}
		} else if slices.Contains(corrections, entry.Name()) {
			current[entry.Name()] = &fstest.MapFile{Data: raw}
		}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	grantTeamTables(t, fx)
	f.GrantTeamReductions(t)
	f.GrantSettingsLifecycle(t)
	f.GrantOwnershipLifecycle(t)
	if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	settings, teams := settingsStore(t, fx), teamStore(t, fx)
	designee := reviewAccount(t, fx, "synthetic-upgrade-designee")
	employee, member := teamDriver(t, fx, designee)
	v := settingsRead(t, fx, settings, 0).Values
	v.Responsibilities.ReviewFallback = d.ResponsibilityPair{PrimaryID: member}
	if _, _, e := settings.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 1, v, "")); e != nil {
		t.Fatal(e)
	}
	team, _, e := teamWrite(t, fx, teams, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Upgrade Team"}}))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx, teams, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employee, ExpectedRevision: 1})); e != nil {
		t.Fatal(e)
	}
	if _, _, e := fx.write(t, legalCommand(t, onboardingID(t), false, 0)); e != nil {
		t.Fatal(e)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_teams x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_team_links x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,revision) FROM org_settings_versions x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM org_settings_heads x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_legal_entities x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_establishments x),
		 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x))::text`).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	roleGuard := func() string {
		t.Helper()
		var def string
		if e := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_role_assignment_guard()'::regprocedure)`).Scan(&def); e != nil {
			t.Fatal(e)
		}
		return def
	}
	before, guard := snapshot(), roleGuard()
	broken := fstest.MapFS{}
	for path, data := range current {
		broken[path] = data
	}
	last := corrections[len(corrections)-1]
	broken[last] = &fstest.MapFile{Data: append(append([]byte(nil), current[last].Data...), []byte("\nSELECT 1/0;\n")...)}
	if _, e := migrate.Apply(t.Context(), f.AdminDSN, broken); e == nil || !strings.Contains(e.Error(), "SQLSTATE 22012") {
		t.Fatal("did not reach injected correction outage", e)
	}
	var recorded bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000183')`).Scan(&recorded); e != nil || recorded || roleGuard() != guard || snapshot() != before {
		t.Fatal("failed correction left a partial definition, ledger entry or row change", e)
	}
	if applied, e := migrate.Apply(t.Context(), f.AdminDSN, current); e != nil || len(applied) != 1 || applied[0] != "000183" {
		t.Fatal("correction recovery", applied, e)
	}
	if applied, e := migrate.Apply(t.Context(), f.AdminDSN, current); e != nil || len(applied) != 0 {
		t.Fatal("correction repeated", applied, e)
	}
	if snapshot() != before {
		t.Fatal("corrections rewrote populated rows")
	}
	if roleGuard() == guard {
		t.Fatal("role guard correction not installed")
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("prior image compatibility", e)
	}
	// The corrected settings guard now applies to the upgraded data.
	if _, e := f.Admin.Exec(t.Context(), `UPDATE accounts SET status='suspended' WHERE id=$1`, designee); e != nil {
		t.Fatal(e)
	}
	next := settingsRead(t, fx, settings, 0).Values
	next.ClosurePeriodDays = 60
	if _, _, e := settings.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 2, next, "")); e != nil {
		t.Fatal("upgraded settings guard still blocked an unrelated change", e)
	}
}
