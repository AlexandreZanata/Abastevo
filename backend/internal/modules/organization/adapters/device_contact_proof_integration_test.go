//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

func TestDeviceContactInvalidCurrentProofNeverReturnsErase(t *testing.T) {
	for _, kind := range []string{"family_revoked", "family_expired", "key_revoked", "token_mismatch"} {
		t.Run(kind, func(t *testing.T) {
			fx := devicesFixture(t)
			first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic terminal proof"))
			if e != nil {
				t.Fatal(e)
			}
			r := deviceContactSelector(t, fx, first.Device)
			if _, e := fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised)); e != nil {
				t.Fatal(e)
			}
			a := fx.person.actor
			switch kind {
			case "family_revoked":
				_, e = fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1`, a.FamilyID)
			case "family_expired":
				_, e = fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET issued_at=clock_timestamp()-interval '31 days' WHERE id=$1`, a.FamilyID)
			case "key_revoked":
				_, e = fx.f.Admin.Exec(t.Context(), `UPDATE identity_keys SET revoked_at=clock_timestamp() WHERE id=$1`, a.KeyID)
			case "token_mismatch":
				a.AccessToken = "synthetic-foreign-token"
			}
			if e != nil {
				t.Fatal(e)
			}
			before := deviceHistorySnapshot(t, fx)
			if got, e := fx.devices.ContactDevice(t.Context(), fx.scope, a, r); !errors.Is(e, app.ErrEmployeeUnauthenticated) || got.Verdict != "" || !got.ServerTime.IsZero() {
				t.Fatal("invalid proof masqueraded as trusted wipe", e)
			}
			if deviceHistorySnapshot(t, fx) != before {
				t.Fatal("invalid proof changed source")
			}
		})
	}
}

func TestDeviceContactRehireCannotRetagPreviousEnrollment(t *testing.T) {
	fx := devicesFixture(t)
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic prior custody period"))
	if e != nil {
		t.Fatal(e)
	}
	old := deviceContactSelector(t, fx, first.Device)
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.person.member); e != nil {
		t.Fatal(e)
	}
	catalog, e := audit.NewCatalog(memberpg.InvitationAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	invites := &memberpg.InvitationStore{Pool: fx.f.Runtime, Authority: &orgpg.InvitationAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Settings: &orgpg.SettingsStore{Pool: fx.f.Runtime}}, Memberships: fx.store.Memberships, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
	invFx := invitationFixtureState{legalFixtureState: fx.legalFixtureState, invites: invites}
	invitation, code := contactInvite(t, invFx, fx.person.employee, "DRIVER")
	redemption := redemptionStore(t, invFx)
	preview, e := redemption.Redeem(t.Context(), fx.person.web, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if e != nil {
		t.Fatal(e)
	}
	accepted, e := redemption.Redeem(t.Context(), fx.person.web, app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest})
	if e != nil || accepted.MembershipState != d.MembershipActive || invitation.MembershipID == old.MembershipID {
		t.Fatal("actual new period", e)
	}
	current, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic new custody period"))
	if e != nil {
		t.Fatal(e)
	}
	next := deviceContactSelector(t, fx, current.Device)
	if got, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, old); e != nil || got.Verdict != app.ContactLock || got.MembershipID != old.MembershipID || got.EnrollmentID != old.EnrollmentID {
		t.Fatal("rehire renewed old content", e)
	}
	if got, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, next); e != nil || got.Verdict != app.ContactActive || got.MembershipID != invitation.MembershipID {
		t.Fatal("fresh period denied", e)
	}
	old.MembershipID = next.MembershipID
	if got, e := fx.devices.ContactDevice(t.Context(), fx.scope, fx.person.actor, old); e == nil || got.Verdict != "" {
		t.Fatal("mixed periods silently rebound", e)
	}
}
