//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
)

func TestOrganizationDevicesHonorVerifiedIdentityClockWindow(t *testing.T) {
	fx := devicesFixture(t)
	c := deviceEnrollCommand(t, "Synthetic identity clock window")
	expires := time.Now().Add(8 * time.Minute).Truncate(time.Second)
	r := deviceSignedRequest(t, fx.registrar, fx.person.signer, "/v1/orgs/"+fx.scope.OrgID().String()+"/devices/enroll", map[string]any{"family_id": fx.person.auth.Session.FamilyID, "access_token": fx.person.auth.Session.AccessToken, "schema_version": 1, "operation_id": c.OperationID, "label": c.Label}, expires)
	r.Header.Set(auth.HeaderCreated, fmt.Sprint(time.Now().Add(4*time.Minute).Unix()))
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		t.Fatal(e)
	}
	lines := auth.BaseLines(r, deviceTestHost, r.Header.Get(auth.HeaderCreated), r.Header.Get(auth.HeaderExpires), fx.person.signer.fingerprint, r.Header.Get(auth.HeaderNonce), raw, true)
	r.Header.Set(auth.HeaderSignature, deviceSignLines(t, fx.person.signer.key, lines))
	r.Body = io.NopCloser(strings.NewReader(string(raw)))
	proof, e := (auth.Verifier{Pool: fx.f.Admin, Authority: deviceTestHost}).Verify(t.Context(), r)
	if e != nil {
		t.Fatal("existing identity profile refused fixture", e)
	}
	a := fx.person.actor
	a.KeyID, a.ContributorID, a.Fingerprint, a.ProofExpires = proof.KeyID, proof.ContributorID, proof.Fingerprint, expires
	if _, e := fx.devices.EnrollDevice(t.Context(), fx.scope, a, c); e != nil {
		t.Fatal("device narrowed the verified identity clock window", e)
	}
}

func TestOrganizationDevicesEnrollmentAndRevocationRaceWithMembershipEnding(t *testing.T) {
	for _, action := range []string{"enroll", "revoke"} {
		t.Run(action, func(t *testing.T) {
			fx := devicesFixture(t)
			var cmd app.DeviceCommand
			if action == "revoke" {
				first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic ending race"))
				if e != nil {
					t.Fatal(e)
				}
				cmd = deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised)
			} else {
				cmd = deviceEnrollCommand(t, "Synthetic new ending race")
			}
			start := make(chan struct{})
			changed, ended := make(chan error, 1), make(chan error, 1)
			go func() {
				<-start
				var e error
				if action == "enroll" {
					_, e = fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, cmd)
				} else {
					_, e = fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, cmd)
				}
				changed <- e
			}()
			go func() {
				<-start
				ended <- unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.person.member)
			}()
			close(start)
			commandError, endError := <-changed, <-ended
			if endError != nil {
				t.Fatal("ending race", endError)
			}
			if commandError != nil && !errors.Is(commandError, app.ErrEmployeeNotFound) && !errors.Is(commandError, app.ErrDeviceRevoked) && !errors.Is(commandError, d.ErrConflict) {
				t.Fatal("source race", commandError)
			}
			if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, fx.person.actor); e == nil {
				t.Fatal("ended membership retained device use")
			}
			page, e := fx.devices.ListDevices(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, "", 20)
			if e != nil {
				t.Fatal(e)
			}
			if action == "revoke" && len(page.Devices) != 1 || action == "enroll" && commandError == nil && len(page.Devices) != 1 || action == "enroll" && commandError != nil && len(page.Devices) != 0 {
				t.Fatal("race left partial enrollment")
			}
			for _, record := range page.Devices {
				s, e := record.Enrollment.Snapshot(fx.scope)
				if e != nil || s.State != d.DeviceRevoked {
					t.Fatal("race left live key", e)
				}
				expected := d.AccessEnded
				if action == "revoke" && commandError == nil {
					expected = d.LostOrCompromised
				}
				if s.Revocation != expected {
					t.Fatal("race lost original classification")
				}
			}
			if _, e := fx.accounts.ValidateAccess(t.Context(), fx.person.auth.Session.FamilyID, fx.person.auth.Session.AccessToken); e != nil {
				t.Fatal("ending race crossed personal boundary", e)
			}
		})
	}
}

func TestOrganizationDevicesCheckAndRevokeAreLinearizable(t *testing.T) {
	fx := devicesFixture(t)
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic check race"))
	if e != nil {
		t.Fatal(e)
	}
	c := deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised)
	start := make(chan struct{})
	checked, revoked := make(chan error, 1), make(chan error, 1)
	go func() { <-start; _, e := fx.devices.CheckDevice(t.Context(), fx.scope, fx.person.actor); checked <- e }()
	go func() {
		<-start
		_, e := fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, c)
		revoked <- e
	}()
	close(start)
	if e := <-revoked; e != nil {
		t.Fatal(e)
	}
	if e := <-checked; e != nil && !errors.Is(e, app.ErrDeviceRevoked) {
		t.Fatal("check race", e)
	}
	if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, fx.person.actor); !errors.Is(e, app.ErrDeviceRevoked) {
		t.Fatal("next request after committed revoke", e)
	}
}

func TestOrganizationDevicesExpireProofAfterProjectionWithoutProvisionalResult(t *testing.T) {
	fx := devicesFixture(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_device_projection_wait() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN PERFORM pg_advisory_xact_lock(921015);RETURN NEW;END$$;CREATE TRIGGER zz_synthetic_device_projection_wait AFTER INSERT ON org_device_enrollments FOR EACH ROW EXECUTE FUNCTION synthetic_device_projection_wait()`); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	holder, e := fx.f.Admin.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer holder.Rollback(context.Background())
	if _, e := holder.Exec(ctx, `SELECT pg_advisory_xact_lock(921015)`); e != nil {
		t.Fatal(e)
	}
	actor := deviceVerifyActor(t, fx, fx.person, fx.person.signer, time.Now().Add(2*time.Second))
	c := deviceEnrollCommand(t, "Synthetic waited projection")
	before := deviceHistorySnapshot(t, fx)
	type response struct {
		value app.DeviceResult
		err   error
	}
	done := make(chan response, 1)
	go func() { v, e := fx.devices.EnrollDevice(ctx, fx.scope, actor, c); done <- response{v, e} }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waited, expired := false, false
	for !waited || !expired {
		select {
		case r := <-done:
			t.Fatal("projection did not wait", r.err)
		case <-ctx.Done():
			t.Fatal("projection clock/lock not observed")
		case <-ticker.C:
			if e := fx.f.Admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock' AND query LIKE '%name: InsertDevice%'),clock_timestamp()>=$2`, fx.f.RuntimeRole, actor.ProofExpires).Scan(&waited, &expired); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := holder.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case r := <-done:
		if !errors.Is(r.err, app.ErrEmployeeUnauthenticated) || r.value.OperationID != "" {
			t.Fatal("expired projection acknowledged enrollment", r.err)
		}
	case <-ctx.Done():
		t.Fatal("projection did not finish")
	}
	if deviceHistorySnapshot(t, fx) != before {
		t.Fatal("expired projection left device, receipt or audit")
	}
}
