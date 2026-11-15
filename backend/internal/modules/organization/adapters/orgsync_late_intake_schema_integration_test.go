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
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func lateSQLCode(t *testing.T, err error, want string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != want {
		t.Fatalf("late SQL guard: want %s, got %v", want, err)
	}
}

// lateForgedOutcome replaces only Apply on the actual meter owner, so the
// normal path tries to publish a late code or a late held fact.
type lateForgedOutcome struct {
	syncapp.Handler
	hold  func(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation) (syncapp.Outcome, error)
	proof syncapp.ReviewProof
}

func (h lateForgedOutcome) ReviewProof() syncapp.ReviewProof { return h.proof }
func (h lateForgedOutcome) Apply(ctx context.Context, db syncapp.DBTX, s syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	return h.hold(ctx, db, s, op)
}

// B-BR-EMP-305: the database refuses fabricated, mutated or out-of-path late
// decisions and late codes; ordinary acknowledgements need no new privilege.
func TestLateIntakeSchemaRefusesFabricatedDecisionsAndLateCodes(t *testing.T) {
	x := lateIntakeFixture(t)
	org := x.scope.OrgID().String()
	proof, _ := x.handler.(syncapp.ReviewProofProvider)

	t.Run("normal-path-cannot-publish-late-codes", func(t *testing.T) {
		for _, code := range []string{syncapp.LateMembershipEndedCode, syncapp.LateWindowClosedCode, syncapp.LateDeviceRevokedCode} {
			op := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
			forged := lateForgedOutcome{Handler: x.handler, proof: proof.ReviewProof(), hold: func(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation) (syncapp.Outcome, error) {
				return syncapp.Outcome{Result: protocol.Rejected, Code: code}, nil
			}}
			if ack, err := x.intake.Process(t.Context(), x.source, op, protocol.DefaultLimits(), forged); err == nil || len(ack.CanonicalBytes()) != 0 {
				t.Fatal("a current source published a late refusal code", code, err)
			}
			x.untouched(t, op)
		}
		command := meterObservedCommand(t, x.installed, lifecycleDBNow(t, x.legalFixtureState), 7)
		op := meterReviewOperation(t, x.syncFixtureState, command)
		forged := lateForgedOutcome{Handler: x.handler, proof: proof.ReviewProof(), hold: func(ctx context.Context, db syncapp.DBTX, s syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
			ref, err := x.meters.Reviews.HoldMeterReadingTx(ctx, db, s, op, command, syncapp.LateHeldCode)
			if err != nil {
				return syncapp.Outcome{}, err
			}
			return syncapp.Outcome{Result: protocol.AcceptedForReview, Code: syncapp.LateHeldCode, ReviewItemID: ref.ReviewItemID}, nil
		}}
		if ack, err := x.intake.Process(t.Context(), x.source, op, protocol.DefaultLimits(), forged); err == nil || len(ack.CanonicalBytes()) != 0 {
			t.Fatal("a current source held a late fact", err)
		}
		lateSQLCode(t, lateHeldAttempt(t, x, x.op(t, lifecycleDBNow(t, x.legalFixtureState)), lifecycleDBNow(t, x.legalFixtureState)), "23514")
		x.untouched(t, op)
	})

	ended := x.end(t)
	held := x.op(t, ended.Add(-time.Hour))
	ack, err := x.late(t, held)
	x.held(t, held, ack, err, ended, string(syncapp.LateHoldDeclared))

	// The held-meter guard itself: a late code needs the exact ended source and
	// a declaration (or an earlier claim) before the end instant.
	t.Run("held-meter-late-code-guard", func(t *testing.T) {
		if ended.IsZero() {
			t.Fatal("end instant unavailable")
		}
		lateSQLCode(t, lateHeldAttempt(t, x, x.op(t, ended), ended), "23514")
		lateSQLCode(t, lateHeldAttempt(t, x, x.op(t, ended.Add(time.Hour)), ended.Add(time.Hour)), "23514")
	})

	t.Run("append-only-forced-rls", func(t *testing.T) {
		role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
		if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_sync_late_intakes TO "+role); err != nil {
			t.Fatal(err)
		}
		var forced bool
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_sync_late_intakes'::regclass`).Scan(&forced); err != nil || !forced {
			t.Fatal("late decisions are not forced-RLS", err)
		}
		for _, verb := range []string{`UPDATE org_sync_late_intakes SET window_days=60`, `DELETE FROM org_sync_late_intakes`, `TRUNCATE org_sync_late_intakes`} {
			lateSQLCode(t, unitSchemaExec(t, x.legalFixtureState, verb), "55000")
		}
		var visible int
		if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_sync_late_intakes`).Scan(&visible); err != nil || visible != 0 {
			t.Fatal("late decisions visible without an organization scope", err, visible)
		}
	})

	t.Run("fabricated-rows", func(t *testing.T) {
		var revision *int64
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT settings_revision FROM org_sync_late_window($1,$2)`, org, ended).Scan(&revision); err != nil {
			t.Fatal(err)
		}
		rev := int64(0)
		if revision != nil {
			rev = *revision
		}
		insert := func(op protocol.Operation, end string, endedAt time.Time, window int, settings int64, declared time.Time, disposition, result, code string) error {
			return database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
				v := op.Snapshot()
				if _, err := tx.Exec(t.Context(), `INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest) VALUES($1,$2,$3,$4,$5,$6,$7,1,'meter_reading',$8,$9)`, org, op.ID(), v.OwnerAccountID, v.APIOrigin, v.MembershipID, v.EnrollmentID, v.Type, v.Entity.ID, v.PayloadDigest); err != nil {
					return err
				}
				_, err := tx.Exec(t.Context(), `INSERT INTO org_sync_late_intakes(org_id,operation_id,membership_id,enrollment_id,end_source,ended_at,window_days,settings_revision,declared_at,declared_submicro,received_at,disposition,result,code) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8::bigint,0),$9,0,clock_timestamp(),$10,$11,$12)`, org, op.ID(), v.MembershipID, v.EnrollmentID, end, endedAt, window, settings, declared, disposition, result, code)
				return err
			})
		}
		before := ended.Add(-time.Hour)
		for name, attempt := range map[string]func(protocol.Operation) error{
			"wrong-end-instant": func(op protocol.Operation) error {
				return insert(op, "MEMBERSHIP", ended.Add(time.Hour), 15, rev, before, "HOLD_DECLARED", protocol.AcceptedForReview, syncapp.LateHeldCode)
			},
			"wrong-end-source": func(op protocol.Operation) error {
				return insert(op, "DEVICE", ended, 15, rev, before, "ENDED", protocol.Rejected, syncapp.LateDeviceRevokedCode)
			},
			"widened-window": func(op protocol.Operation) error {
				return insert(op, "MEMBERSHIP", ended, 60, rev, before, "HOLD_DECLARED", protocol.AcceptedForReview, syncapp.LateHeldCode)
			},
			"wrong-disposition": func(op protocol.Operation) error {
				return insert(op, "MEMBERSHIP", ended, 15, rev, before, "ENDED", protocol.Rejected, syncapp.LateMembershipEndedCode)
			},
			"declared-after-end-held": func(op protocol.Operation) error {
				return insert(op, "MEMBERSHIP", ended, 15, rev, ended, "HOLD_DECLARED", protocol.AcceptedForReview, syncapp.LateHeldCode)
			},
		} {
			t.Run(name, func(t *testing.T) {
				lateSQLCode(t, attempt(x.op(t, before)), "23514")
			})
		}
		t.Run("consistent-row-without-acknowledgement", func(t *testing.T) {
			op := x.op(t, before)
			err := insert(op, "MEMBERSHIP", ended, 15, rev, before, "HOLD_DECLARED", protocol.Rejected, "MALFORMED")
			var pe *pgconn.PgError
			if !errors.As(err, &pe) || pe.Code != "23503" && pe.Code != "23514" {
				t.Fatal("a late decision committed without its acknowledgement", err)
			}
			x.untouched(t, op)
		})
	})
}

// A populated head before 000218 (ended member with its ACCESS_ENDED period
// and an accepted signed reading) upgrades through an ordinary non-superuser
// migrator: an injected failure changes nothing, recovery applies once, replay
// is a no-op, the previous image stays compatible and the redefined held-meter
// guard keeps every 201 check. The populated ended period then delivers late.
func TestLateIntakePopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000218_organization_late_intake.sql"
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
	if _, ok := previous["000215_organization_fuelings.sql"]; !ok {
		t.Fatal("previous image lacks the 000215 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	x := lateIntakeFrom(t, syncFixtureOn(t, devicesFixtureOn(t, legalFixtureFor(t, f, onboarding, actor, flags))))
	accepted := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	if ack, err := x.intake.Process(t.Context(), x.source, accepted, protocol.DefaultLimits(), x.handler); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("previous-image accepted reading", err)
	}
	ended := x.end(t)
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_device_enrollments x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,revision)FROM org_settings_versions x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_late_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_sync_operations,org_sync_acknowledgements,org_memberships,org_device_enrollments,org_settings_versions TO "+role+
		";GRANT TRIGGER ON org_sync_acknowledgements TO "+role+";ALTER FUNCTION org_meter_held_source() OWNER TO "+role); err != nil {
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
		t.Fatal("injected late-intake migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_sync_late_intakes') IS NULL AND to_regprocedure('org_sync_late_disposition(timestamptz,integer,timestamptz,timestamptz,timestamptz)') IS NULL
 AND to_regprocedure('org_sync_ack_late_source()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000218')
 AND pg_get_functiondef('org_meter_held_source()'::regprocedure) NOT LIKE '%ACCESS_ENDED_BEFORE_RECEIPT%'`).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed late-intake migration changed the original schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000218" {
		t.Fatal("ordinary late-intake migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("late-intake upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("late-intake migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1 AND NOT rolsuper AND NOT rolbypassrls)
 AND pg_get_functiondef('org_meter_held_source()'::regprocedure) LIKE '%held asset unavailability unproven%' AND pg_get_functiondef('org_meter_held_source()'::regprocedure) LIKE '%e.state=''ENROLLED'' AND p.state=''ACTIVE''%'
 AND pg_get_functiondef('org_meter_held_source()'::regprocedure) LIKE '%org_sync_late_end%' FROM pg_class WHERE oid='org_sync_late_intakes'::regclass`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("late table/guard ownership or the preserved 201 checks are missing", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 218 rehearsal", err)
	}
	f.GrantLateIntake(t)
	// The populated, pre-upgrade ended period now delivers through the late path.
	op := x.op(t, ended.Add(-30*time.Minute))
	ack, err := x.late(t, op)
	x.held(t, op, ack, err, ended, string(syncapp.LateHoldDeclared))
	if row, ok := x.row(t, accepted); ok {
		t.Fatal("upgrade invented a late decision for an existing receipt", row)
	}
}

// lateHeldAttempt inserts a claim, a late-code review item and a held meter
// reading directly, so only the 218 held-meter guard can refuse it.
func lateHeldAttempt(t *testing.T, x lateFixture, op protocol.Operation, declared time.Time) error {
	t.Helper()
	org := x.scope.OrgID().String()
	return database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		v := op.Snapshot()
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest) VALUES($1,$2,$3,$4,$5,$6,$7,1,'meter_reading',$8,$9)`, org, op.ID(), v.OwnerAccountID, v.APIOrigin, v.MembershipID, v.EnrollmentID, v.Type, v.Entity.ID, v.PayloadDigest); err != nil {
			return err
		}
		item := onboardingID(t)
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_sync_review_items(org_id,id,operation_id,operation_type,operation_schema,payload_digest,code,author_employee_id) SELECT o.org_id,$2,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest,'ACCESS_ENDED_BEFORE_RECEIPT',m.employee_id FROM org_sync_operations o JOIN org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id) WHERE o.org_id=$1 AND o.operation_id=$3`, org, item, op.ID()); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_meter_held_readings(org_id,review_item_id,operation_id,reading_id,asset_id,meter_id,segment_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,origin,method,request_hash,unit_id)
 SELECT a.org_id,$2,$3,$4,a.id,$6,$7,7,$8,0,'America/Cuiaba',-14400,'STANDALONE','TYPED',repeat('b',64),a.unit_id FROM org_assets a WHERE a.org_id=$1 AND a.id=$5`, org, item, op.ID(), v.Entity.ID, x.installed.AssetID, x.installed.MeterID, x.installed.SegmentID, declared.Truncate(time.Microsecond))
		return err
	})
}
