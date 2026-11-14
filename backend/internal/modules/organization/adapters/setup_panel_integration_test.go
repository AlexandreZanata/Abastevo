//go:build integration

package adapters_test

import (
	"errors"
	"sort"
	"sync"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
)

func TestSetupPanelCurrentPersistedReadAndCapabilityOffRecovery(t *testing.T) {
	fx := settingsSchemaFixture(t)
	store := settingsStore(t, fx)
	view, err := store.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0)
	if err != nil {
		t.Fatal(err)
	}
	if view.Organization.ID != fx.scope.OrgID() || view.Organization.State != "PENDING_SETUP" || view.Context.MembershipID != fx.member || view.Context.AccountID != fx.actor.AccountID || !view.Context.Fresh || !view.Context.CapabilityEnabled || !view.TermsRecorded || len(view.Checklist) != 0 {
		t.Fatal("persisted setup binding/checklist missing")
	}
	if len(view.ResponsibilityMembers) != 1 || view.ResponsibilityMembers[0].MembershipID != fx.member || !view.ResponsibilityMembers[0].ProfilePending || !view.ResponsibilityMembers[0].Eligible {
		t.Fatal("named source membership projection missing")
	}
	page, err := store.ListResponsibilityMembers(t.Context(), fx.scope, fx.actor, fx.member, app.SettingsMembersFilter{Limit: 1})
	if err != nil || len(page.Members) != 1 || page.Members[0].MembershipID != fx.member {
		t.Fatal("responsibility picker unavailable", err)
	}
	if _, err := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: 1, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	view, err = store.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0)
	if err != nil || view.Context.CapabilityEnabled {
		t.Fatal("disabled capability hid saved settings or invented write availability", err)
	}
	page, err = store.ListResponsibilityMembers(t.Context(), fx.scope, fx.actor, fx.member, app.SettingsMembersFilter{Limit: 1})
	if err != nil || len(page.Members) != 1 {
		t.Fatal("read-only picker enabled or required a write capability", err)
	}
	directory := fx.onboarding.Repository.(app.CreatedSetupReader)
	owned, err := directory.ListCreatedSetups(t.Context(), fx.actor, 50, "")
	if err != nil || len(owned.Setups) != 1 || owned.Setups[0].ID != fx.scope.OrgID().String() {
		t.Fatal("capability-off creation could not be recovered", err)
	}
	contexts, err := contextStore(fx).ListAccountContexts(t.Context(), fx.actor, 50, "")
	if err != nil || len(contexts.Organizations) != 0 {
		t.Fatal("setup recovery broadened operational native context selection", err)
	}
	if err := unitSchemaExec(t, fx, `UPDATE organizations SET display_name='',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	view, err = store.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0)
	if err != nil || view.Organization.Profile.Name != "" || len(view.Checklist) != 1 || view.Checklist[0] != "NAME" {
		t.Fatal("incomplete persisted profile was hidden or assumed complete", err)
	}
	owned, err = directory.ListCreatedSetups(t.Context(), fx.actor, 50, "")
	if err != nil || len(owned.Setups) != 1 || owned.Setups[0].Name != "" {
		t.Fatal("incomplete saved setup disappeared from recovery", err)
	}
}

func TestCreatedSetupDirectoryFiltersCurrentAuthorityAndRehire(t *testing.T) {
	fx := settingsSchemaFixture(t)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	foreignActor := transferPersonalActor(t, transferFixtureState{legalFixtureState: fx, actors: map[string]app.CompanyActor{}})
	foreign := legalFixtureFor(t, fx.f, fx.onboarding, foreignActor, fx.flags)
	directory := fx.onboarding.Repository.(app.CreatedSetupReader)
	page, e := directory.ListCreatedSetups(t.Context(), fx.actor, 100, "")
	if e != nil || len(page.Setups) != 2 {
		t.Fatal("own-created discovery source boundary", e)
	}
	ids := []string{fx.scope.OrgID().String(), other.scope.OrgID().String()}
	sort.Strings(ids)
	for i, item := range page.Setups {
		if item.ID != ids[i] || item.ID == foreign.scope.OrgID().String() {
			t.Fatal("foreign receipt leaked or UUID ordering changed")
		}
	}
	victim := fx
	if victim.scope.OrgID().String() != ids[0] {
		victim = other
	}
	_, owner := teamDriver(t, victim, foreignActor.AccountID)
	if e := unitSchemaExec(t, victim, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'Synthetic recovery owner')`, victim.scope.OrgID().String(), onboardingID(t), owner, victim.member); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, victim, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, victim.scope.OrgID().String(), victim.member); e != nil {
		t.Fatal(e)
	}
	first, e := directory.ListCreatedSetups(t.Context(), fx.actor, 1, "")
	if e != nil || len(first.Setups) != 0 || first.NextAfterOrgID != ids[0] {
		t.Fatal("unauthorized owned receipt stranded keyset or restored access", e)
	}
	second, e := directory.ListCreatedSetups(t.Context(), fx.actor, 1, first.NextAfterOrgID)
	if e != nil || len(second.Setups) != 1 || second.Setups[0].ID != ids[1] || second.NextAfterOrgID != "" {
		t.Fatal("filtered directory keyset did not advance", e)
	}
	// A new valid period on the same account/employee is resolved from source,
	// never from the membership ID frozen in the original creation receipt.
	var employee string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, victim.scope.OrgID().String(), victim.member).Scan(&employee); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, victim, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, victim.scope.OrgID().String(), victim.member); e != nil {
		t.Fatal(e)
	}
	current := onboardingID(t)
	if e := unitSchemaExec(t, victim, `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, victim.scope.OrgID().String(), current, employee, fx.actor.AccountID); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, victim, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'Synthetic rehire recovery')`, victim.scope.OrgID().String(), onboardingID(t), current, owner); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, victim, `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, victim.scope.OrgID().String(), current); e != nil {
		t.Fatal(e)
	}
	page, e = directory.ListCreatedSetups(t.Context(), fx.actor, 100, "")
	if e != nil || len(page.Setups) != 2 || page.Setups[0].MembershipID != current || page.Setups[0].MembershipID == victim.member {
		t.Fatal("recovery reused historical receipt authority", e)
	}
	for _, bad := range []struct {
		limit int
		after string
	}{{0, ""}, {101, ""}, {1, "not-an-id"}} {
		if _, e := directory.ListCreatedSetups(t.Context(), fx.actor, bad.limit, bad.after); e == nil {
			t.Fatal("unbounded or invalid selector accepted")
		}
	}
	if _, e := directory.ListCreatedSetups(t.Context(), app.CompanyActor{AccountID: fx.actor.AccountID, SessionID: onboardingID(t)}, 10, ""); !errors.Is(e, app.ErrUnauthenticated) {
		t.Fatal("forged proof authorized discovery", e)
	}
}

func TestCreatedSetupDirectoryConcurrentReadsAndStorageFailure(t *testing.T) {
	fx := settingsSchemaFixture(t)
	directory := fx.onboarding.Repository.(app.CreatedSetupReader)
	var group sync.WaitGroup
	failures := make(chan error, 8)
	start := make(chan struct{})
	for range 8 {
		group.Go(func() {
			<-start
			page, e := directory.ListCreatedSetups(t.Context(), fx.actor, 10, "")
			if e != nil {
				failures <- e
			} else if len(page.Setups) != 1 {
				failures <- errors.New("saved setup disappeared")
			}
		})
	}
	close(start)
	group.Wait()
	close(failures)
	for e := range failures {
		t.Fatal("concurrent account/root proof read failed", e)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "REVOKE SELECT ON org_role_assignments FROM "+role); e != nil {
		t.Fatal(e)
	}
	page, e := directory.ListCreatedSetups(t.Context(), fx.actor, 10, "")
	if !errors.Is(e, app.ErrUnavailable) || len(page.Setups) != 0 || page.AccountID != "" {
		t.Fatal("source failure exposed provisional setup metadata or false empty success", e)
	}
}
