//go:build integration

package adapters_test

import (
	"encoding/json"
	"errors"
	"fmt"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"strings"
	"testing"
)

type memberAccessFixture struct {
	invitationFixtureState
	access   *memberpg.MembershipAccessStore
	transfer transferFixtureState
}

func membershipAccessFixture(t *testing.T) memberAccessFixture {
	t.Helper()
	return membershipAccessOn(t, invitationFixture(t))
}
func membershipAccessOn(t *testing.T, fx invitationFixtureState) memberAccessFixture {
	t.Helper()
	fx.f.GrantMembershipAccessCommands(t)
	catalog, e := audit.NewCatalog(memberpg.MembershipAccessAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	roles := accesspg.NewStore(fx.f.Runtime)
	s := &memberpg.MembershipAccessStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Periods: roles, Authority: &orgpg.MembershipAccessAuthority{Memberships: fx.store.Memberships, Roles: roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
	tf := transferFixtureState{legalFixtureState: fx.legalFixtureState, actors: map[string]orgapp.CompanyActor{fx.member: fx.actor}}
	return memberAccessFixture{fx, s, tf}
}
func accessActor(fx memberAccessFixture) app.EmployeeActor {
	return app.EmployeeActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
}
func accessMember(t *testing.T, fx memberAccessFixture) string {
	t.Helper()
	id, _ := createTransferMember(t, fx.transfer, false)
	return id
}
func accessRead(t *testing.T, fx memberAccessFixture, member string) app.MembershipAccessResult {
	t.Helper()
	out, e := fx.access.ReadMembershipAccess(t.Context(), fx.scope, accessActor(fx), fx.member, member)
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func accessWrite(t *testing.T, fx memberAccessFixture, c app.MembershipAccessWrite) (app.MembershipAccessResult, error) {
	t.Helper()
	return fx.access.ChangeMembershipAccess(t.Context(), fx.scope, accessActor(fx), fx.member, c)
}
func TestMembershipAccessSuspendsReactivatesWithOriginalAcknowledgements(t *testing.T) {
	fx := membershipAccessFixture(t)
	target := accessMember(t, fx)
	before := accessRead(t, fx, target)
	var roles string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(r)ORDER BY id)::text FROM org_role_assignments r WHERE org_id=$1 AND membership_id=$2`, fx.scope.OrgID().String(), target).Scan(&roles); e != nil {
		t.Fatal(e)
	}
	c := app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: before.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic pause"}
	paused, e := accessWrite(t, fx, c)
	if e != nil || paused.State.State != d.MembershipSuspended || paused.State.Suspension != d.ByManager || paused.Replayed {
		t.Fatal("suspend", e)
	}
	candidates, e := accesspg.NewStore(fx.f.Runtime).Candidates(t.Context(), fx.scope, target)
	if e != nil || len(candidates) != 0 {
		t.Fatal("suspended authority", e)
	}
	again, e := accessWrite(t, fx, c)
	if e != nil || !again.Replayed || again.OccurredAt != paused.OccurredAt {
		t.Fatal("suspend retry", e)
	}
	c.Reason = "Changed intent"
	if _, e := accessWrite(t, fx, c); !errors.Is(e, app.ErrAccessOperationReused) {
		t.Fatal("reason reuse", e)
	}
	c.Reason = "Synthetic pause"
	resume := app.MembershipAccessWrite{Action: app.MembershipReactivate, MembershipID: target, ExpectedRevision: paused.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic resume"}
	active, e := accessWrite(t, fx, resume)
	if e != nil || active.State.State != d.MembershipActive || active.PreviousSuspension != d.ByManager {
		t.Fatal("reactivate", e)
	}
	original, e := accessWrite(t, fx, c)
	if e != nil || !original.Replayed || original.State.State != d.MembershipSuspended || original.OccurredAt != paused.OccurredAt {
		t.Fatal("original after later state", e)
	}
	if accessRead(t, fx, target).State.State != d.MembershipActive {
		t.Fatal("retry resuspended target")
	}
	var after string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(r)ORDER BY id)::text FROM org_role_assignments r WHERE org_id=$1 AND membership_id=$2`, fx.scope.OrgID().String(), target).Scan(&after); e != nil || after != roles {
		t.Fatal("assignment periods rewritten", e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_membership_access_commands WHERE membership_id=$1`, 2, target)
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_membership_access_notices WHERE membership_id=$1`, 2, target)
	encoded, _ := json.Marshal(paused)
	if strings.Contains(string(encoded), target) || strings.Contains(fmt.Sprintf("%+v", paused), target) {
		t.Fatal("private metadata formatting")
	}
}
