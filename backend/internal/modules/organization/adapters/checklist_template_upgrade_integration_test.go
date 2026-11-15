//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	ma "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// Populated previous head → 223: a failed attempt changes nothing; an ordinary
// NOSUPERUSER/NOBYPASSRLS migrator recovers; nothing is invented for existing
// assets, categories, reservations or assignments; the previous image stays
// compatible; the guards, the completeness check and the asset-use source
// work after the upgrade.
func TestChecklistTemplatePopulatedUpgradeIsAdditiveAndRecoverable(t *testing.T) {
	version := strings.SplitN(checklistMigration, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= checklistMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	for _, required := range []string{reservationMigration, "000219_organization_state_intake.sql"} {
		if _, ok := previous[required]; !ok {
			t.Fatal("previous image lacks", required)
		}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	assets := reservationStore(t, fx)
	var unit string
	if err := f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, org).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	active, category := checklistAsset(t, fx, assets, unit, "Synthetic upgrade trucks", fleet.Active)
	draft, _ := checklistAsset(t, fx, assets, unit, "Synthetic upgrade drafts", fleet.Draft)
	employee, _ := assignmentPerson(t, fx)
	if _, _, err := assignmentWrite(t, fx, assets, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: active, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{employee}})); err != nil {
		t.Fatal(err)
	}
	now := exactNow(t, fx)
	if _, _, err := reserve(t, fx, assets, fa.ReservationRequestInput{AssetID: active, Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_assets x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_asset_categories x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_asset_assignments x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_asset_reservations x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x),
 (SELECT count(*) FROM job_queue))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	usage := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure)`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before, beforeUsage := snapshot(), usage()
	migrator := f.RuntimeRole + "_checklist_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// The ordinary migrator only references the parents and owns the one
	// function it replaces.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+
		";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,org_audit_events,org_assets,org_asset_categories TO "+role+
		";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, checklistMigration)
	if err != nil {
		t.Fatal("checklist migration missing", err)
	}
	clean := func(label string) {
		t.Helper()
		var ok bool
		if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_checklist_templates') IS NULL AND to_regclass('org_checklist_template_versions') IS NULL AND to_regclass('org_checklist_template_items') IS NULL
 AND to_regclass('org_checklist_template_targets') IS NULL AND to_regclass('org_checklist_policy_revisions') IS NULL AND to_regclass('org_checklist_template_commands') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1) AND to_regprocedure('org_checklist_root(uuid)') IS NULL`, version).Scan(&ok); err != nil || !ok || before != snapshot() || beforeUsage != usage() {
			t.Fatal(label, "changed the previous schema, usage source or data", err)
		}
	}
	broken := fstest.MapFS{checklistMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected checklist migration failure not reached", err)
	}
	clean("failed migration")
	current := fstest.MapFS{}
	for key, data := range previous {
		current[key] = data
	}
	current[checklistMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary checklist migration recovery", err)
	}
	if before != snapshot() {
		t.Fatal("checklist migration rewrote existing assets, categories, reservations, history or jobs")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("checklist migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	var exact bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_checklist_templates) AND NOT EXISTS(SELECT 1 FROM org_checklist_template_versions) AND NOT EXISTS(SELECT 1 FROM org_checklist_template_commands)
 AND(SELECT bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_checklist_templates'::regclass,'org_checklist_template_versions'::regclass,
  'org_checklist_template_items'::regclass,'org_checklist_template_targets'::regclass,'org_checklist_policy_revisions'::regclass,'org_checklist_template_commands'::regclass))
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 AND(SELECT count(*) FROM pg_trigger WHERE tgname IN('org_checklist_template_write','org_checklist_version_write','org_checklist_item_write','org_checklist_target_write','org_checklist_policy_write','org_checklist_version_complete',
  'org_checklist_templates_immutable','org_checklist_template_commands_immutable','org_checklist_template_versions_immutable','org_checklist_template_items_immutable','org_checklist_template_targets_immutable','org_checklist_policy_revisions_immutable'))=12
 AND(SELECT count(*) FROM pg_constraint WHERE conname IN('org_checklist_template_targets_asset_fkey','org_checklist_template_targets_category_fkey','org_checklist_templates_version_fkey','org_checklist_templates_policy_fkey') AND contype='f')=4
 AND position('org_checklist_template_targets_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_asset_reservations_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_asset_blocks_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_fuelings_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_asset_assignments_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0`, migrator).Scan(&exact); err != nil || !exact {
		t.Fatal("checklist migration invented data, weakened RLS or missed a guard", err)
	}
	// Upgraded behaviour with the ordinary runtime grants.
	s, _ := checklistStore(t, fx)
	byAsset, err := createTemplate(t, fx, s, checklist.Departure, assetTarget(active))
	if err != nil {
		t.Fatal("template on the upgraded schema", err)
	}
	if _, err := createTemplate(t, fx, s, checklist.Departure, categoryTarget(category)); err != nil {
		t.Fatal("category template on the upgraded schema", err)
	}
	if _, err := createTemplate(t, fx, s, checklist.Departure, assetTarget(draft)); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("draft target on the upgraded schema", err)
	}
	v, err := s.ReadApplicable(t.Context(), fx.scope, checklistActor(fx), fx.member, active, checklist.Departure)
	if err != nil || v.Match != checklist.MatchAsset || v.Template.TemplateID != byAsset.TemplateID {
		t.Fatal("applicable read on the upgraded schema", v, err)
	}
	if _, _, err := templateRun(t, fx, s, templateCommand(t, ma.TemplateCommandInput{Kind: ma.CommandPolicy, TemplateID: byAsset.TemplateID, ExpectedRevision: 1, Policy: checklist.Policy{Mode: checklist.Warn, ValiditySeconds: 259200}})); err != nil {
		t.Fatal("policy on the upgraded schema", err)
	}
	// The asset-use source accepts the new key (else P7904) with unchanged
	// counts for a targeted asset.
	var use string
	if err := scopedScan(t, fx, `SELECT org_asset_current_usage($1,$2)::text`, &use, org, active); err != nil || !strings.Contains(use, `"operations": 0`) {
		t.Fatal("asset-use source after the upgrade", use, err)
	}
}

func scopedScan(t *testing.T, fx legalFixtureState, query string, out *string, args ...any) error {
	t.Helper()
	tx, err := fx.f.Runtime.Begin(t.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, fx.scope.OrgID().String()); err != nil {
		return err
	}
	return tx.QueryRow(t.Context(), query, args...).Scan(out)
}
