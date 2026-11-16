//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const expenseMigration = "000244_organization_expenses.sql"

// preExpenseCosts is the fueling owner as it ran before 000244: no
// projection existed, so nothing is stored for the cost.
type preExpenseCosts struct{}

func (preExpenseCosts) ProjectFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, int64) error {
	return nil
}

// A populated head before 000244 (two fuelings accepted through the real
// signed intake, an organization cost centre) upgrades to 000244 by an
// ordinary non-superuser migrator: an injected failure leaves schema,
// functions, triggers and data untouched; recovery applies once and
// fabricates no cost; replay is a no-op; the previous embedded image stays
// compatible; the redefined usage sources keep every earlier line and their
// answers; earlier receipts replay; the per-organization catch-up then gives
// each earlier fueling exactly one expense and a second run changes nothing;
// a new fueling commits with its one cost (EMP-EXP-002, EDR-287/288).
func TestExpensePopulatedUpgradeFailureRecoveryAndCatchUp(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= expenseMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000237_organization_trip_conflict_resolutions.sql"]; !ok {
		t.Fatal("previous image does not contain the 000237 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := fuelingFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal))), preExpenseCosts{})
	d := x.syncFixtureState
	org := x.scope.OrgID().String()
	declared := x.declaredNow(t)
	company := x.op(t, d, declared, nil)
	employee := x.op(t, d, declared.Add(time.Second), func(e, p map[string]any) {
		missing("Painel apagado")(e, p)
		p["paid_total"], p["payer"] = "97350", "EMPLOYEE"
	})
	acks := map[string]protocol.Acknowledgement{}
	for _, op := range []protocol.Operation{company, employee} {
		ack, err := x.process(t, d, op)
		if err != nil || ack.Result() != protocol.Accepted {
			t.Fatal("fueling before the upgrade", err, ack.Result(), ack.Code())
		}
		acks[op.ID()] = ack
	}
	centre := onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic centre','SYN-1','ACTIVE')`, org, centre)
		return err
	}); err != nil {
		t.Fatal("cost-centre fixture", err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_fuelings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,fueling_id,code)FROM org_fueling_warnings x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,fueling_id,sequence)FROM org_fueling_review_entries x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_units x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_cost_centres x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_effects x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	usage := func() string {
		t.Helper()
		tx, err := f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		var value string
		if err := tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text||org_unit_current_usage($1,$3)::text||org_cost_centre_current_usage($1,$4)::text`, org, x.vehicle.AssetID, x.unit, centre).Scan(&value); err != nil {
			t.Fatal("usage source refused the populated state", err)
		}
		return value
	}
	sources := []string{"org_asset_current_usage(uuid,uuid)", "org_unit_current_usage(uuid,uuid)", "org_cost_centre_current_usage(uuid,uuid)"}
	definitions := func() []string {
		t.Helper()
		out := make([]string, 0, len(sources))
		for _, source := range sources {
			var v string
			if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef(to_regprocedure($1))`, source).Scan(&v); err != nil {
				t.Fatal(err)
			}
			out = append(out, v)
		}
		return out
	}
	before, beforeUsage, beforeDefinitions := snapshot(), usage(), definitions()
	migrator := f.RuntimeRole + "_expense_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON organizations,org_sync_operations,org_fuelings,org_assets,org_memberships,org_employee_identities,org_device_enrollments,org_units,org_cost_centres TO "+role+
		";GRANT TRIGGER ON org_fuelings TO "+role+
		";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_cost_centre_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, expenseMigration)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{expenseMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected expense migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_expenses') IS NULL AND to_regprocedure('org_expense_source()') IS NULL AND to_regprocedure('org_expense_complete()') IS NULL
 AND to_regprocedure('org_fueling_expense_complete()') IS NULL AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='org_fuelings'::regclass AND tgname='org_fueling_expense_complete')
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000244')`).Scan(&clean); err != nil || !clean || before != snapshot() || strings.Join(definitions(), "") != strings.Join(beforeDefinitions, "") {
		t.Fatal("failed expense migration changed the original schema, functions or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[expenseMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000244" {
		t.Fatal("ordinary expense migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("expense upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("expense migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1) FROM pg_class WHERE oid='org_expenses'::regclass)
 AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='org_fuelings'::regclass AND tgname='org_fueling_expense_complete' AND tgdeferrable AND tginitdeferred)
 AND NOT EXISTS(SELECT 1 FROM org_expenses) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("expense table not owned by the ordinary forced-RLS migrator, or a cost was fabricated", err)
	}
	if usage() != beforeUsage {
		t.Fatal("redefined usage sources changed the populated answers")
	}
	// Each redefined source is its previous body with exactly one added
	// allow-list line: every earlier line is kept verbatim.
	after := definitions()
	for i, line := range []string{"\n OR(c.conrelid='public.org_expenses'::regclass AND c.conname='org_expenses_asset_fkey')",
		"\n OR(c.conrelid='public.org_expenses'::regclass AND c.conname='org_expenses_unit_fkey')",
		"\n   OR(c.conrelid='public.org_expenses'::regclass AND c.conname='org_expenses_cost_centre_fkey')"} {
		if strings.Count(after[i], line) != 1 || strings.Replace(after[i], line, "", 1) != beforeDefinitions[i] {
			t.Fatal("usage source is not its previous body plus the expense key", sources[i])
		}
	}
	expenseGrants(t, x.legalFixtureState)
	// Earlier receipts replay unchanged through the upgraded owner.
	upgraded, err := fuelpg.NewIntakeHandler(x.authority, x.meters, x.store, x.expenses)
	if err != nil {
		t.Fatal(err)
	}
	x.fueling = upgraded
	for _, op := range []protocol.Operation{company, employee} {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), acks[op.ID()].CanonicalBytes()) {
			t.Fatal("earlier receipt did not replay after the upgrade", err)
		}
	}
	// The catch-up, run twice as a job would be, projects each earlier
	// fueling exactly once.
	for run, want := range []int{2, 0} {
		var inserted int
		if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			var err error
			inserted, err = x.expenses.ProjectPendingFuelingsTx(t.Context(), syncapp.Borrow(tx), x.scope)
			return err
		}); err != nil || inserted != want {
			t.Fatal("catch-up run", run, inserted, err)
		}
	}
	var exact bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND bool_and(x.origin_kind='FUELING' AND x.category='FUEL' AND x.paid_centavos=f.paid_centavos AND x.company_centavos=f.paid_centavos
 AND x.payer=f.payer AND x.operation_id=f.operation_id AND x.unit_id=f.unit_id AND x.reporting_day=(f.occurred_at AT TIME ZONE x.reporting_zone)::date AND x.review_state='SUBMITTED'
 AND x.received_at>f.received_at)
 AND(SELECT count(*) FROM org_expenses)=2 AND(SELECT bool_and(payer='EMPLOYEE' AND paid_centavos=97350) FROM org_expenses WHERE operation_id=$2)
 FROM org_fuelings f JOIN org_expenses x ON(x.org_id,x.fueling_id)=(f.org_id,f.id) WHERE f.org_id=$1`, org, employee.ID()).Scan(&exact); err != nil || !exact {
		t.Fatal("catch-up did not give each earlier fueling its one exact cost", err)
	}
	for _, op := range []protocol.Operation{company, employee} {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), acks[op.ID()].CanonicalBytes()) {
			t.Fatal("earlier receipt changed after the catch-up", err)
		}
	}
	fresh := x.op(t, d, x.declaredNow(t), nil)
	ack, err := x.process(t, d, fresh)
	x.accepted(t, fresh, ack, err, 0, 1)
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 244 rehearsal", err)
	}
}
