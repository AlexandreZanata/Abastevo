//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/jackc/pgx/v5"
)

func TestDeviceContactRevokeAndMembershipEndAreLinearizable(t *testing.T) {
	for _, ending := range []bool{false, true} {
		t.Run(map[bool]string{false: "lost", true: "ended"}[ending], func(t *testing.T) {
			fx := devicesFixture(t)
			first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic contact race"))
			if e != nil {
				t.Fatal(e)
			}
			r := deviceContactSelector(t, fx, first.Device)
			start := make(chan struct{})
			type result struct {
				v app.DeviceContact
				e error
			}
			contacted := make(chan result, 1)
			changed := make(chan error, 1)
			go func() {
				<-start
				v, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, r)
				contacted <- result{v, e}
			}()
			go func() {
				<-start
				if ending {
					changed <- unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.person.member)
				} else {
					_, e := fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised))
					changed <- e
				}
			}()
			close(start)
			if e := <-changed; e != nil {
				t.Fatal(e)
			}
			got := <-contacted
			want := app.ContactErase
			if ending {
				want = app.ContactLock
			}
			if got.e != nil || got.v.Verdict != app.ContactActive && got.v.Verdict != want {
				t.Fatal("contact race returned untrusted verdict", got.e)
			}
			v, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, r)
			if e != nil || v.Verdict != want || v.ValiditySeconds != 0 {
				t.Fatal("next contact renewed revoked period", e)
			}
		})
	}
}

func TestDeviceContactProofExpiresWhileWaitingForRoot(t *testing.T) {
	fx := devicesFixture(t)
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic contact proof wait"))
	if e != nil {
		t.Fatal(e)
	}
	r := deviceContactSelector(t, fx, first.Device)
	a := fx.person.actor
	a.ProofExpires = time.Now().Add(2 * time.Second).Truncate(time.Second)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	holder, e := fx.f.Admin.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer holder.Rollback(context.Background())
	if _, e := holder.Exec(ctx, `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	type result struct {
		v app.DeviceContact
		e error
	}
	done := make(chan result, 1)
	before := deviceHistorySnapshot(t, fx)
	go func() { v, e := fx.devices.ContactDevice(ctx, fx.scope, a, r); done <- result{v, e} }()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	waited, expired := false, false
	for !waited || !expired {
		select {
		case got := <-done:
			t.Fatal("contact did not wait", got.e)
		case <-ctx.Done():
			t.Fatal("root wait/expiry not observed")
		case <-tick.C:
			if e := fx.f.Admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock' AND query LIKE '%name: LockDeviceContactRoot%'),clock_timestamp()>=$2`, fx.f.RuntimeRole, a.ProofExpires).Scan(&waited, &expired); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := holder.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	got := <-done
	if !errors.Is(got.e, app.ErrEmployeeUnauthenticated) || got.v.Verdict != "" {
		t.Fatal("expired proof exposed provisional lease", got.e)
	}
	if deviceHistorySnapshot(t, fx) != before {
		t.Fatal("expired contact mutated private state")
	}
}

func TestDeviceContactLateCommitFailureNeverExposesProvisionalObservation(t *testing.T) {
	fx := devicesFixture(t)
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic contact commit fault"))
	if e != nil {
		t.Fatal(e)
	}
	r := deviceContactSelector(t, fx, first.Device)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	// A test-only RLS observation queues a deferred failure. This exercises the
	// real transaction's COMMIT error after a valid result was assembled, without
	// adding a product write or a product test hook to this read-only endpoint.
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_contact_failure(n int);
CREATE FUNCTION synthetic_contact_observe() RETURNS boolean LANGUAGE plpgsql AS $$BEGIN INSERT INTO synthetic_contact_failure VALUES(1);RETURN true;END$$;
CREATE FUNCTION synthetic_contact_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic contact commit unavailable';END$$;
CREATE CONSTRAINT TRIGGER synthetic_contact_failure AFTER INSERT ON synthetic_contact_failure DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_contact_commit_failure();
ALTER POLICY org_device_enrollment_scope ON org_device_enrollments USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid AND synthetic_contact_observe());
GRANT INSERT ON synthetic_contact_failure TO `+role); e != nil {
		t.Fatal(e)
	}
	before := deviceHistorySnapshot(t, fx)
	if got, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, r); e == nil || got.Verdict != "" || !got.ServerTime.IsZero() {
		t.Fatal("failed commit exposed contact", e)
	}
	if deviceHistorySnapshot(t, fx) != before {
		t.Fatal("failed contact changed source")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `ALTER POLICY org_device_enrollment_scope ON org_device_enrollments USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)`); e != nil {
		t.Fatal(e)
	}
	if got, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, r); e != nil || got.Verdict != app.ContactActive {
		t.Fatal("contact recovery", e)
	}
}
