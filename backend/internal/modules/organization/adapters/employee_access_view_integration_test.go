//go:build integration

package adapters_test

import (
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"testing"
)

func TestEmployeeAccessViewTracksSourceWithoutChangingEmployeeLifecycle(t *testing.T) {
	fx := invitationFixture(t)
	catalog, e := audit.NewCatalog(memberpg.EmployeeAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	employees := &memberpg.EmployeeStore{Pool: fx.f.Runtime, Authority: &orgpg.EmployeeAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
	command := employeeCommand(t, app.EmployeeWriteInput{Kind: app.CreateEmployee, Profile: d.EmployeeProfile{DisplayName: "Synthetic access view", WorkEmail: "view@example.invalid"}})
	created, _, e := writeEmployee(t, fx.legalFixtureState, employees, command)
	if e != nil {
		t.Fatal(e)
	}
	check := func(want d.EmployeeAccessState) {
		t.Helper()
		record, e := employees.ReadEmployee(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, created.EmployeeID)
		if e != nil {
			t.Fatal(e)
		}
		view, e := record.Snapshot(fx.scope)
		if e != nil || view.AccessState != want {
			t.Fatalf("access view: want %s, got %s (%v)", want, view.AccessState, e)
		}
		identity, _ := view.Employee.Snapshot(fx.scope)
		if identity.State != d.EmployeeActive || identity.Revision != 1 || view.ProfileRevision != 1 {
			t.Fatal("access projection changed employee")
		}
		page, e := employees.ListEmployees(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, app.EmployeeFilter{Limit: 100})
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, record := range page.Employees {
			v, e := record.Snapshot(fx.scope)
			if e != nil {
				t.Fatal(e)
			}
			i, _ := v.Employee.Snapshot(fx.scope)
			if i.ID == created.EmployeeID {
				found = true
				if v.AccessState != want {
					t.Fatal("list disagrees with read")
				}
			}
		}
		if !found {
			t.Fatal("employee missing")
		}
		matches, e := employees.MatchEmployees(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, app.EmployeeMatch{Profile: d.EmployeeProfile{DisplayName: "Synthetic access view", WorkEmail: "view@example.invalid"}, Limit: 10})
		if e != nil || len(matches.Employees) != 1 {
			t.Fatal("match view", e)
		}
		matched, _ := matches.Employees[0].Snapshot(fx.scope)
		if matched.AccessState != want {
			t.Fatal("match disagrees with read")
		}
	}
	check("NONE")
	issue := func() (d.InvitationSnapshot, string) {
		t.Helper()
		v, _, code, e := inviteWrite(t, fx, app.InvitationWrite{Action: app.InvitationIssue, OperationID: onboardingID(t), EmployeeID: created.EmployeeID, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}})
		if e != nil {
			t.Fatal(e)
		}
		snapshot, e := v.Snapshot(fx.scope)
		if e != nil {
			t.Fatal(e)
		}
		return snapshot, code
	}
	invitation, _ := issue()
	check("INVITED")
	if _, _, _, e := inviteWrite(t, fx, app.InvitationWrite{Action: app.InvitationRevoke, OperationID: onboardingID(t), InvitationID: invitation.ID, ExpectedRevision: invitation.Revision}); e != nil {
		t.Fatal(e)
	}
	check("NONE")
	invitation, code := issue()
	redemption := redemptionStore(t, fx)
	person := redemptionActor(t, fx)
	preview, e := redemption.Redeem(t.Context(), person, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if e != nil {
		t.Fatal(e)
	}
	accepted, e := redemption.Redeem(t.Context(), person, app.RedemptionCommand{Action: "ACCEPT", OperationID: onboardingID(t), Code: code, Consent: true, ConsentDigest: preview.ConsentDigest})
	if e != nil || accepted.MembershipState != d.MembershipActive {
		t.Fatal("accept", e)
	}
	check("ACTIVE")
	access := membershipAccessOn(t, fx)
	before := accessRead(t, access, invitation.MembershipID)
	if _, e := accessWrite(t, access, app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: invitation.MembershipID, ExpectedRevision: before.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic source view pause"}); e != nil {
		t.Fatal(e)
	}
	check("SUSPENDED")
	foreign := scopeFor(t, onboardingID(t))
	if _, e := employees.ReadEmployee(t.Context(), foreign, employeeActor(fx.legalFixtureState), fx.member, created.EmployeeID); e == nil {
		t.Fatal("foreign access metadata exposed")
	}
}
