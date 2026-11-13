//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

func deviceHistorySnapshot(t *testing.T, fx deviceFixtureState) string {
	t.Helper()
	var out string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_device_enrollments x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_device_commands x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM account_session_families x))::text`).Scan(&out); e != nil {
		t.Fatal(e)
	}
	return out
}

func TestOrganizationDevicesForeignOwnBindingAndNonManagerDenials(t *testing.T) {
	fx := devicesFixture(t)
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic scoped device"))
	if e != nil {
		t.Fatal(e)
	}
	other := deviceNewPerson(t, fx)
	c := deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised)
	before := deviceHistorySnapshot(t, fx)
	if _, e := fx.devices.ListDevices(t.Context(), fx.scope, other.web, "", fx.person.member, true, "", 20); e == nil {
		t.Fatal("foreign self list")
	}
	if _, e := fx.devices.RevokeDevice(t.Context(), fx.scope, other.web, "", fx.person.member, true, c); e == nil {
		t.Fatal("foreign self revoke")
	}
	if _, e := fx.devices.ListDevices(t.Context(), fx.scope, other.web, other.member, fx.person.member, false, "", 20); e == nil {
		t.Fatal("non-manager list")
	}
	if _, e := fx.devices.RevokeDevice(t.Context(), fx.scope, other.web, other.member, fx.person.member, false, c); e == nil {
		t.Fatal("non-manager revoke")
	}
	foreign := fx.f.Organizations[1].Scope(t)
	if _, e := fx.devices.CheckDevice(t.Context(), foreign, fx.person.actor); e == nil {
		t.Fatal("foreign check")
	}
	if _, e := fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, other.member, false, c); e == nil {
		t.Fatal("wrong target member revoke")
	}
	if deviceHistorySnapshot(t, fx) != before {
		t.Fatal("denied device command changed source history")
	}
}

func TestOrganizationDevicesDefaultLimitAndPageIsolation(t *testing.T) {
	fx := devicesFixture(t)
	actors := []app.DeviceActor{fx.person.actor}
	for range 3 {
		_, a := deviceAddSigner(t, fx, fx.person)
		actors = append(actors, a)
	}
	for i, a := range actors {
		_, e := fx.devices.EnrollDevice(t.Context(), fx.scope, a, deviceEnrollCommand(t, "Synthetic page device"))
		if i < 3 && e != nil || i == 3 && !errors.Is(e, app.ErrDeviceLimit) {
			t.Fatal("default scoped device limit", i, e)
		}
	}
	manager := employeeActor(fx.legalFixtureState)
	page, e := fx.devices.ListDevices(t.Context(), fx.scope, manager, fx.member, fx.person.member, false, "", 2)
	if e != nil || len(page.Devices) != 2 || page.NextID == "" {
		t.Fatal("device page", e)
	}
	tail, e := fx.devices.ListDevices(t.Context(), fx.scope, manager, fx.member, fx.person.member, false, page.NextID, 2)
	if e != nil || len(tail.Devices) != 1 || tail.NextID != "" {
		t.Fatal("device page tail", e)
	}
	ids := map[string]bool{}
	for _, r := range append(page.Devices, tail.Devices...) {
		s, e := r.Enrollment.Snapshot(fx.scope)
		if e != nil || s.MembershipID != fx.person.member || ids[s.ID] {
			t.Fatal("device cursor crossed or duplicated scope", e)
		}
		ids[s.ID] = true
	}
}

func TestOrganizationDevicesMembershipEndingPreservesLostAndSuspension(t *testing.T) {
	fx := devicesFixture(t)
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic lost device"))
	if e != nil {
		t.Fatal(e)
	}
	_, secondActor := deviceAddSigner(t, fx, fx.person)
	second, e := fx.devices.EnrollDevice(t.Context(), fx.scope, secondActor, deviceEnrollCommand(t, "Synthetic ended-access device"))
	if e != nil {
		t.Fatal(e)
	}
	manager := employeeActor(fx.legalFixtureState)
	lost, e := fx.devices.RevokeDevice(t.Context(), fx.scope, manager, fx.member, fx.person.member, false, deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised))
	if e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.person.member); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, secondActor); e == nil {
		t.Fatal("suspended membership used enrolled device")
	}
	var current string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_device_enrollments WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), deviceRecordID(t, fx, second.Device)).Scan(&current); e != nil || current != "ENROLLED" {
		t.Fatal("suspension invented ending", e)
	}
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.person.member); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, secondActor); e != nil {
		t.Fatal("reactivated original device", e)
	}
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.person.member); e != nil {
		t.Fatal(e)
	}
	for _, a := range []app.DeviceActor{fx.person.actor, secondActor} {
		if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, a); e == nil {
			t.Fatal("ended membership device used")
		}
	}
	page, e := fx.devices.ListDevices(t.Context(), fx.scope, manager, fx.member, fx.person.member, false, "", 20)
	if e != nil || len(page.Devices) != 2 {
		t.Fatal("retained ended device history", e)
	}
	for _, r := range page.Devices {
		s, _ := r.Enrollment.Snapshot(fx.scope)
		if s.State != d.DeviceRevoked {
			t.Fatal("membership left enrolled device")
		}
		if s.ID == deviceRecordID(t, fx, lost.Device) {
			if s.Revocation != d.LostOrCompromised || r.SourceType != "COMMAND" || !r.RevokedAt.Equal(*lost.Device.RevokedAt) {
				t.Fatal("ending overwrote lost classification")
			}
		} else if s.Revocation != d.AccessEnded || r.SourceType != "MEMBERSHIP" || r.SourceID != fx.person.member || r.SourceRevision != 5 {
			t.Fatal("ending source invented")
		}
	}
	if _, e := fx.accounts.ValidateAccess(t.Context(), fx.person.auth.Session.FamilyID, fx.person.auth.Session.AccessToken); e != nil {
		t.Fatal("membership ending revoked personal family", e)
	}
}

func deviceRecordID(t *testing.T, fx deviceFixtureState, r app.DeviceRecord) string {
	t.Helper()
	s, e := r.Enrollment.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	return s.ID
}

func TestOrganizationDevicesDeferredAuditReceiptRollbackAndRecovery(t *testing.T) {
	for _, table := range []string{"org_audit_events", "org_device_commands"} {
		for _, revoke := range []bool{false, true} {
			t.Run(table+map[bool]string{false: "_enroll", true: "_revoke"}[revoke], func(t *testing.T) {
				fx := devicesFixture(t)
				c := deviceEnrollCommand(t, "Synthetic atomic device")
				if revoke {
					r, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, c)
					if e != nil {
						t.Fatal(e)
					}
					c = deviceRevokeCommand(t, fx.scope, r.Device, d.LostOrCompromised)
				}
				before := deviceHistorySnapshot(t, fx)
				if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_device_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic deferred private device failure';END$$;CREATE CONSTRAINT TRIGGER synthetic_device_failure AFTER INSERT ON `+table+` DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_device_failure()`); e != nil {
					t.Fatal(e)
				}
				var r app.DeviceResult
				var e error
				if revoke {
					r, e = fx.devices.RevokeDevice(t.Context(), fx.scope, fx.person.web, "", fx.person.member, true, c)
				} else {
					r, e = fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, c)
				}
				if !errors.Is(e, app.ErrRegisterUnavailable) || r.OperationID != "" || r.Replayed {
					t.Fatal("provisional device result on commit failure", e)
				}
				if deviceHistorySnapshot(t, fx) != before {
					t.Fatal("device failure left partial audit, receipt or enrollment")
				}
				if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_device_failure ON `+table); e != nil {
					t.Fatal(e)
				}
				if revoke {
					r, e = fx.devices.RevokeDevice(t.Context(), fx.scope, fx.person.web, "", fx.person.member, true, c)
				} else {
					r, e = fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, c)
				}
				if e != nil || r.Device.Validate(fx.scope) != nil {
					t.Fatal("device recovery", e)
				}
			})
		}
	}
}

func TestOrganizationDevicesCurrentProofAfterRootWait(t *testing.T) {
	fx := devicesFixture(t)
	actor := deviceVerifyActor(t, fx, fx.person, fx.person.signer, time.Now().Add(2*time.Second))
	c := deviceEnrollCommand(t, "Synthetic waited device")
	before := deviceHistorySnapshot(t, fx)
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	holder, e := fx.f.Admin.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer holder.Rollback(context.Background())
	if _, e := holder.Exec(ctx, `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := fx.devices.EnrollDevice(ctx, fx.scope, actor, c); done <- e }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waited, expired := false, false
	for !waited || !expired {
		select {
		case e := <-done:
			t.Fatal("device enrollment did not wait", e)
		case <-ctx.Done():
			t.Fatal("device source lock/expiry not observed")
		case <-ticker.C:
			if e := fx.f.Admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock' AND query LIKE '%FROM organizations WHERE org_id=%FOR UPDATE%'),clock_timestamp()>=$2`, fx.f.RuntimeRole, actor.ProofExpires).Scan(&waited, &expired); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := holder.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, app.ErrEmployeeUnauthenticated) {
			t.Fatal("expired signed proof after root wait", e)
		}
	case <-ctx.Done():
		t.Fatal("device wait did not finish")
	}
	if deviceHistorySnapshot(t, fx) != before {
		t.Fatal("expired device proof changed source history")
	}
}

func TestOrganizationDevicesRejectRevokedFamilyIdentityKeyAndForgedProof(t *testing.T) {
	for _, kind := range []string{"family", "key", "token", "fingerprint", "unbound"} {
		t.Run(kind, func(t *testing.T) {
			fx := devicesFixture(t)
			c := deviceEnrollCommand(t, "Synthetic proof denial device")
			if _, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, c); e != nil {
				t.Fatal(e)
			}
			actor := fx.person.actor
			switch kind {
			case "family":
				if e := fx.accounts.Store.RevokeFamily(t.Context(), actor.FamilyID, time.Now().Unix()); e != nil {
					t.Fatal(e)
				}
			case "key":
				if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE identity_keys SET revoked_at=clock_timestamp() WHERE id=$1`, actor.KeyID); e != nil {
					t.Fatal(e)
				}
			case "token":
				actor.AccessToken = "synthetic-incorrect-personal-access"
			case "fingerprint":
				actor.Fingerprint = "fp:" + strings.Repeat("b", 64)
			case "unbound":
				if e := fx.accounts.Store.UnbindContributor(t.Context(), actor.AccountID, actor.ContributorID, onboardingID(t), time.Now().Unix()); e != nil {
					t.Fatal(e)
				}
			}
			before := deviceHistorySnapshot(t, fx)
			if _, e := fx.devices.CheckDevice(t.Context(), fx.scope, actor); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
				t.Fatal("invalid current device proof used enrollment", e)
			}
			if _, e := fx.devices.EnrollDevice(t.Context(), fx.scope, actor, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
				t.Fatal("invalid current device proof replayed receipt", e)
			}
			if deviceHistorySnapshot(t, fx) != before {
				t.Fatal("invalid proof changed device facts")
			}
		})
	}
}

func TestOrganizationDevicesFreshInvitationCreatesNewAccessEndedPeriodButKeepsLostBan(t *testing.T) {
	for _, class := range []d.RevocationClass{d.AccessEnded, d.LostOrCompromised} {
		t.Run(string(class), func(t *testing.T) {
			fx := devicesFixture(t)
			c := deviceEnrollCommand(t, "Synthetic previous access period")
			first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, c)
			if e != nil {
				t.Fatal(e)
			}
			if _, e := fx.devices.RevokeDevice(t.Context(), fx.scope, fx.person.web, "", fx.person.member, true, deviceRevokeCommand(t, fx.scope, first.Device, class)); e != nil {
				t.Fatal(e)
			}
			if _, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic same-period return")); !errors.Is(e, app.ErrDeviceRevoked) {
				t.Fatal("same period revocation bypass", e)
			}
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
			r := redemptionStore(t, invFx)
			preview, e := r.Redeem(t.Context(), fx.person.web, app.RedemptionCommand{Action: "PREVIEW", Code: code})
			if e != nil {
				t.Fatal(e)
			}
			accepted, e := r.Redeem(t.Context(), fx.person.web, app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest})
			if e != nil {
				t.Fatal(e)
			}
			if invitation.MembershipID == fx.person.member || accepted.MembershipState != d.MembershipActive {
				t.Fatal("new invitation did not create distinct active access")
			}
			newDevice, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic newly invited period"))
			if class == d.LostOrCompromised {
				if !errors.Is(e, app.ErrDeviceRevoked) {
					t.Fatal("fresh invitation restored compromised key", e)
				}
				return
			}
			if e != nil {
				t.Fatal("fresh membership could not enroll valid personal key", e)
			}
			snap, e := newDevice.Device.Enrollment.Snapshot(fx.scope)
			if e != nil || snap.MembershipID != invitation.MembershipID || snap.ID == deviceRecordID(t, fx, first.Device) || snap.State != d.DeviceEnrolled {
				t.Fatal("new period rewrote original enrollment", e)
			}
		})
	}
}

func TestOrganizationDevicesManagerReductionSurvivesRootAndCapabilityRestrictions(t *testing.T) {
	for _, state := range []string{"ACTIVE", "SUSPENDED", "ARCHIVED"} {
		t.Run(state, func(t *testing.T) {
			fx := devicesFixture(t)
			first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic reduction device"))
			if e != nil {
				t.Fatal(e)
			}
			if _, e := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: 1, Enabled: false}); e != nil {
				t.Fatal(e)
			}
			// These ordinary source-valid transitions exercise the same persisted
			// states as lifecycle commands without inventing device/membership facts.
			if state == "SUSPENDED" {
				if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='SUSPENDED',suspension_reason='SECURITY',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
					t.Fatal(e)
				}
			} else if state == "ARCHIVED" {
				if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
					t.Fatal(e)
				}
			}
			manager := employeeActor(fx.legalFixtureState)
			page, e := fx.devices.ListDevices(t.Context(), fx.scope, manager, fx.member, fx.person.member, false, "", 20)
			if e != nil || len(page.Devices) != 1 {
				t.Fatal("restricted manager could not inspect enrolled devices", e)
			}
			r, e := fx.devices.RevokeDevice(t.Context(), fx.scope, manager, fx.member, fx.person.member, false, deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised))
			if e != nil || r.Device.Validate(fx.scope) != nil {
				t.Fatal("root/capability restriction blocked access reduction", e)
			}
			snap, _ := r.Device.Enrollment.Snapshot(fx.scope)
			if snap.State != d.DeviceRevoked || snap.Revocation != d.LostOrCompromised {
				t.Fatal("restricted reduction not committed")
			}
		})
	}
}
