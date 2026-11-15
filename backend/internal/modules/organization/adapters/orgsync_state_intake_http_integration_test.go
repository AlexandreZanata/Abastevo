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
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// VAL-22 part 2 through the actual signed route and default runtime: while the
// organization is suspended an empty batch keeps ORG_SUSPENDED; a batch with
// operations is assessed per operation, resends replay byte for byte.
func TestStateIntakeSignedRouteWhileSuspended(t *testing.T) {
	fx := meterIntakeHTTP(t)
	fx.f.GrantStateIntake(t)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_sync_review_items,org_meter_held_readings,org_sync_review_resolutions TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	ops := suspensionOnFixture(t, fx.legalFixtureState)
	pre := fx.operation(t, "7", nil)
	if _, err := ops.change(t, app.SuspendOrganization, ops.root(t).Revision, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, code := lateHTTPBatch(t, fx, 403); code != "ORG_SUSPENDED" {
		t.Fatal("an empty batch was admitted while suspended", code)
	}
	post := fx.operation(t, "8", nil)
	acks, body, _ := lateHTTPBatch(t, fx, 200, pre, post)
	if acks[0].Result() != protocol.AcceptedForReview || acks[0].Code() != syncapp.StateGraceCode {
		t.Fatal("pre-suspension reading was not held in grace", acks[0].Result(), acks[0].Code())
	}
	if acks[1].Result() != protocol.Rejected || acks[1].Code() != syncapp.StateSuspendedCode {
		t.Fatal("post-suspension reading was not refused", acks[1].Code())
	}
	_, again, _ := lateHTTPBatch(t, fx, 200, pre, post)
	if !bytes.Equal(body, again) {
		t.Fatal("state resend did not replay byte for byte")
	}
	if _, err := ops.change(t, app.ReactivateOrganization, ops.root(t).Revision, ""); err != nil {
		t.Fatal(err)
	}
	if ack := fx.dispatch(t, fx.operation(t, "9", nil), 200); ack.Result() != protocol.Accepted {
		t.Fatal("new work after reactivation did not apply")
	}
	_, after, _ := lateHTTPBatch(t, fx, 200, pre, post)
	if !bytes.Equal(body, after) {
		t.Fatal("reactivation rewrote a state acknowledgement")
	}
}

// A populated 218 head (a late decision and an applied reading) upgrades to
// 219 through an ordinary migrator: failure changes nothing, recovery applies
// once, replay is a no-op, the previous image stays compatible, and state
// intake then works on the upgraded organization.
func TestStateIntakePopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000219_organization_state_intake.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= name || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000218_organization_late_intake.sql"]; !ok {
		t.Fatal("previous image lacks the 000218 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	base := syncFixtureOn(t, devicesFixtureOn(t, legalFixtureFor(t, f, onboarding, actor, flags)))
	x := lateIntakeFrom(t, base)
	applied := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	if ack, err := x.intake.Process(t.Context(), x.source, applied, protocol.DefaultLimits(), x.handler); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("previous-image accepted reading", err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,capability)FROM org_capabilities x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_late_intakes x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_state_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_sync_operations,org_sync_acknowledgements,org_capabilities TO "+role+";GRANT TRIGGER ON org_sync_acknowledgements TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected state-intake migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_sync_state_intakes') IS NULL AND to_regprocedure('org_sync_ack_state_source()') IS NULL AND to_regprocedure('org_sync_plan_disposition(text,bigint,timestamptz,timestamptz,timestamptz)') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000219')`).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed state-intake migration changed the schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if got, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(got) != 1 || got[0] != "000219" {
		t.Fatal("ordinary state-intake migration recovery", got, err)
	}
	if before != snapshot() {
		t.Fatal("state-intake upgrade rewrote existing state")
	}
	if got, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(got) != 0 {
		t.Fatal("state-intake migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1 AND NOT rolsuper AND NOT rolbypassrls) FROM pg_class WHERE oid='org_sync_state_intakes'::regclass`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("state table not owned by the ordinary forced-RLS migrator", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 219 rehearsal", err)
	}
	f.GrantStateIntake(t)
	ops := suspensionOnFixture(t, x.legalFixtureState)
	state := stateFixture{lateFixture: x, ops: ops}
	state.state = newStateStore(x)
	pre := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	state.suspend(t)
	ack, err := state.process(t, pre)
	state.heldState(t, pre, ack, err, syncapp.StateSuspension, syncapp.StateGraceCode)
	if _, _, _, ok := state.stateRow(t, applied); ok {
		t.Fatal("upgrade invented a state decision for an existing receipt")
	}
}
