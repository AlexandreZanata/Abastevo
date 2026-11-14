//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

// BR-ORG-030/033, SCR-ORG-12, B-BR-EMP-171: members keep discovering and
// revalidating a suspended or archived organization with its notice state,
// without write hints; a closed organization is no longer a context.
func TestAccountContextsKeepSuspendedAndArchivedOrganizationsWithNotice(t *testing.T) {
	fx := legalFixture(t)
	if _, _, e := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); e != nil {
		t.Fatal(e)
	}
	source := contextStore(fx)
	actor := app.CompanyActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
	org := fx.scope.OrgID().String()
	active, e := source.ReadAccountContext(t.Context(), actor, fx.scope, fx.member)
	if e != nil || !slices.Contains(active.Permissions, access.Permission("idn.invitation.manage")) || len(active.InvitationGrants) == 0 || active.Organization.State != "ACTIVE" || active.Organization.SuspensionReason != "" {
		t.Fatal("active context controls", e)
	}
	for _, tc := range []struct{ state, sql, reason string }{
		{"SUSPENDED", `UPDATE organizations SET state='SUSPENDED',suspension_reason='SECURITY',revision=revision+1 WHERE org_id=$1`, "SECURITY"},
		// Archiving keeps the still-open suspension cause for the notice.
		{"ARCHIVED", `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, "SECURITY"},
	} {
		if e := unitSchemaExec(t, fx, tc.sql, org); e != nil {
			t.Fatal(e)
		}
		var reason string
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(suspension_reason,'') FROM organizations WHERE org_id=$1`, org).Scan(&reason); e != nil {
			t.Fatal(e)
		}
		if reason != tc.reason {
			t.Fatal("fixture suspension reason", tc.state, reason)
		}
		page, e := source.ListAccountContexts(t.Context(), actor, 100, "")
		if e != nil || len(page.Organizations) != 1 || page.Organizations[0].ID != org || page.Organizations[0].MembershipID != fx.member {
			t.Fatal("member lost discovery of a readable organization", tc.state, e)
		}
		if page.Organizations[0].State != tc.state || page.Organizations[0].SuspensionReason != tc.reason {
			t.Fatal("discovered context lacks its notice state", tc.state, page.Organizations[0].State)
		}
		view, e := source.ReadAccountContext(t.Context(), actor, fx.scope, fx.member)
		if e != nil || view.Organization.ID != org || !slices.Contains(view.Permissions, access.Permission("idn.employee.read")) {
			t.Fatal("member lost revalidation of a readable organization", tc.state, e)
		}
		if view.Organization.State != tc.state || view.Organization.SuspensionReason != tc.reason {
			t.Fatal("revalidated context lacks its notice state", tc.state, view.Organization.State)
		}
		if slices.Contains(view.Permissions, access.Permission("idn.employee.manage")) || slices.Contains(view.Permissions, access.Permission("idn.invitation.manage")) || len(view.InvitationGrants) != 0 {
			t.Fatal("read-only organization advertised write controls", tc.state)
		}
	}
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='CLOSED',revision=revision+1 WHERE org_id=$1`, org); e != nil {
		t.Fatal(e)
	}
	if _, e := source.ReadAccountContext(t.Context(), actor, fx.scope, fx.member); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("closed organization selected", e)
	}
	if page, e := source.ListAccountContexts(t.Context(), actor, 100, ""); e != nil || len(page.Organizations) != 0 {
		t.Fatal("closed organization discovered", e)
	}
}

// A non-member probing a known organization UUID always sees NOT_FOUND,
// whatever its capability; only a current member learns the capability outcome.
func TestAccountContextNonMemberCannotProbeOrganizationCapability(t *testing.T) {
	fx := legalFixture(t)
	source := contextStore(fx)
	actor := app.CompanyActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
	foreignPerson := transferPersonalActor(t, transferFixtureState{legalFixtureState: fx, actors: map[string]app.CompanyActor{}})
	foreignActor := app.CreationActor{AccountID: foreignPerson.AccountID, SessionID: foreignPerson.SessionID}
	foreign := legalFixtureFor(t, fx.f, fx.onboarding, foreignActor, fx.flags)
	outsider := app.CompanyActor{AccountID: foreignActor.AccountID, SessionID: foreignActor.SessionID}
	_, enabledErr := source.ReadAccountContext(t.Context(), outsider, fx.scope, fx.member)
	_, foreignMemberErr := source.ReadAccountContext(t.Context(), outsider, fx.scope, foreign.member)
	if _, e := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	_, disabledErr := source.ReadAccountContext(t.Context(), outsider, fx.scope, fx.member)
	_, disabledForeignErr := source.ReadAccountContext(t.Context(), outsider, fx.scope, foreign.member)
	for _, e := range []error{enabledErr, foreignMemberErr, disabledErr, disabledForeignErr} {
		if !errors.Is(e, app.ErrNotFound) {
			t.Fatal("non-member distinguished an organization capability", e)
		}
	}
	if _, e := source.ReadAccountContext(t.Context(), actor, fx.scope, fx.member); !errors.Is(e, cap.ErrEntitlementRequired) {
		t.Fatal("current member lost the capability outcome", e)
	}
	if page, e := source.ListAccountContexts(t.Context(), actor, 100, ""); e != nil || len(page.Organizations) != 0 {
		t.Fatal("disabled capability context discovered", e)
	}
}

// Context reads never serialize with an in-flight capability change.
func TestAccountContextCapabilityReadTakesNoLock(t *testing.T) {
	fx := legalFixture(t)
	source := contextStore(fx)
	actor := app.CompanyActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
	blocker, e := fx.f.Admin.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback(context.Background())
	if _, e := blocker.Exec(t.Context(), `SELECT 1 FROM org_capabilities WHERE org_id=$1 AND capability='organizations' FOR UPDATE`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	view, e := source.ReadAccountContext(ctx, actor, fx.scope, fx.member)
	cancel()
	if e != nil || view.Organization.MembershipID != fx.member {
		t.Fatal("current context read waited on a capability lock", e)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 3*time.Second)
	page, e := source.ListAccountContexts(ctx, actor, 100, "")
	cancel()
	if e != nil || len(page.Organizations) != 1 {
		t.Fatal("context discovery waited on a capability lock", e)
	}
}
