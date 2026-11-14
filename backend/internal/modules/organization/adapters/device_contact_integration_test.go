//go:build integration

package adapters_test

import (
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/jackc/pgx/v5"
)

func deviceContactSelector(t *testing.T, fx deviceFixtureState, record app.DeviceRecord) app.DeviceContactRequest {
	t.Helper()
	s, e := record.Enrollment.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	return app.DeviceContactRequest{RequestID: onboardingID(t), MembershipID: s.MembershipID, EnrollmentID: s.ID}
}

func TestDeviceContactExactCurrentLeaseAndTerminalOwnership(t *testing.T) {
	for _, class := range []d.RevocationClass{d.LostOrCompromised, d.AccessEnded} {
		t.Run(string(class), func(t *testing.T) {
			fx := devicesFixture(t)
			first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic custody contact"))
			if e != nil {
				t.Fatal(e)
			}
			r := deviceContactSelector(t, fx, first.Device)
			before := deviceHistorySnapshot(t, fx)
			at := time.Now().UTC()
			v, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, r)
			if e != nil || v.Validate(fx.person.actor, r) != nil || v.Verdict != app.ContactActive || v.ValiditySeconds != 2592000 || v.AccessModel != d.ExistingAccount || v.ServerTime.Before(at) || v.ServerTime.After(time.Now().UTC()) {
				t.Fatal("actual lease", e)
			}
			for _, mutate := range []func(*app.DeviceContactRequest){func(r *app.DeviceContactRequest) { r.EnrollmentID = onboardingID(t) }, func(r *app.DeviceContactRequest) { r.MembershipID = fx.member }} {
				bad := r
				mutate(&bad)
				if got, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, bad); e == nil || got.Verdict != "" {
					t.Fatal("foreign selector renewed lease", e)
				}
			}
			if deviceHistorySnapshot(t, fx) != before {
				t.Fatal("contact mutated source/sync/audit history")
			}
			other := deviceNewPerson(t, fx)
			if got, e := fx.devices.ContactDevice(t.Context(), fx.scope, other.actor, r); e == nil || got.Verdict != "" {
				t.Fatal("foreign account learned contact", e)
			}
			c := deviceRevokeCommand(t, fx.scope, first.Device, class)
			if _, e := fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, c); e != nil {
				t.Fatal(e)
			}
			before = deviceHistorySnapshot(t, fx)
			v, e = fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, r)
			want := app.ContactLock
			if class == d.LostOrCompromised {
				want = app.ContactErase
			}
			if e != nil || v.Validate(fx.person.actor, r) != nil || v.Verdict != want || v.RevocationClass != class || v.Revision != 2 || v.ValiditySeconds != 0 || v.AccessModel != "" {
				t.Fatal("exact terminal observation", e)
			}
			if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, fx.person.actor); !errors.Is(e, app.ErrDeviceRevoked) {
				t.Fatal("legacy check behavior changed", e)
			}
			if deviceHistorySnapshot(t, fx) != before {
				t.Fatal("contact mutated source/sync/audit history")
			}
		})
	}
}

func TestDeviceContactEndedMembershipOnlyReturnsOwnedTombstone(t *testing.T) {
	fx := devicesFixture(t)
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic ended contact"))
	if e != nil {
		t.Fatal(e)
	}
	r := deviceContactSelector(t, fx, first.Device)
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.person.member); e != nil {
		t.Fatal(e)
	}
	// Terminal custody must not query employee/business projections after access ended.
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), `REVOKE SELECT ON org_employee_profiles,org_employee_identities,organizations,org_onboarding_commands FROM `+role); e != nil {
		t.Fatal(e)
	}
	// The root serialization query is separately required; this test deliberately
	// keeps only its existing source projection grant rather than exposing profiles.
	if _, e := fx.f.Admin.Exec(t.Context(), `GRANT SELECT,UPDATE ON organizations TO `+role); e != nil {
		t.Fatal(e)
	}
	v, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, r)
	if e != nil || v.Verdict != app.ContactLock || v.RevocationClass != d.AccessEnded || v.ValiditySeconds != 0 {
		t.Fatal("ended contact required business read or erased", e)
	}
}
