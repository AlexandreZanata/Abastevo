//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"
)

func contextStore(fx legalFixtureState) *orgpg.AccountContextStore {
	return &orgpg.AccountContextStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Settings: &orgpg.SettingsStore{Pool: fx.f.Runtime}}
}
func TestAccountContextsCurrentOwnSourcePaginationAndScopeIsolation(t *testing.T) {
	fx := legalFixture(t)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	source := contextStore(fx)
	actor := app.CompanyActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
	page, e := source.ListAccountContexts(t.Context(), actor, 1, "")
	if e != nil || len(page.Organizations) != 1 || page.NextAfterOrgID == "" {
		t.Fatal("first current context page", len(page.Organizations), e)
	}
	second, e := source.ListAccountContexts(t.Context(), actor, 1, page.NextAfterOrgID)
	if e != nil || len(second.Organizations) != 1 || second.NextAfterOrgID != "" || second.Organizations[0].ID == page.Organizations[0].ID {
		t.Fatal("next current context page", e)
	}
	want := []string{fx.scope.OrgID().String(), other.scope.OrgID().String()}
	sort.Strings(want)
	if page.Organizations[0].ID != want[0] || second.Organizations[0].ID != want[1] {
		t.Fatal("unstable context order")
	}
	view, e := source.ReadAccountContext(t.Context(), actor, fx.scope, fx.member)
	if e != nil || view.Organization.ID != fx.scope.OrgID().String() || view.Organization.MembershipID != fx.member || !slices.Contains(view.Permissions, access.Permission("idn.employee.read")) || !slices.Contains(view.Permissions, access.Permission("idn.invitation.manage")) || len(view.InvitationGrants) == 0 || view.InvitationValidityDays != 7 {
		t.Fatal("current context controls", e)
	}
	if _, e = source.ReadAccountContext(t.Context(), actor, other.scope, fx.member); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("foreign membership selected", e)
	}
	if _, e = source.ReadAccountContext(t.Context(), app.CompanyActor{AccountID: actor.AccountID, SessionID: onboardingID(t)}, fx.scope, fx.member); !errors.Is(e, app.ErrUnauthenticated) {
		t.Fatal("forged source proof", e)
	}
	foreignPerson := transferPersonalActor(t, transferFixtureState{legalFixtureState: fx, actors: map[string]app.CompanyActor{}})
	foreignActor := app.CreationActor{AccountID: foreignPerson.AccountID, SessionID: foreignPerson.SessionID}
	foreign := legalFixtureFor(t, fx.f, fx.onboarding, foreignActor, fx.flags)
	if _, e = source.ReadAccountContext(t.Context(), actor, foreign.scope, foreign.member); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("another account context read", e)
	}
	foreignPage, e := source.ListAccountContexts(t.Context(), app.CompanyActor{AccountID: foreignActor.AccountID, SessionID: foreignActor.SessionID}, 100, "")
	if e != nil || len(foreignPage.Organizations) != 1 || foreignPage.Organizations[0].ID != foreign.scope.OrgID().String() {
		t.Fatal("another account discovery boundary", e)
	}
	// Ineligible scanned references do not strand pagination.
	firstScope := scopeFor(t, want[0])
	if _, e = fx.flags.Set(t.Context(), firstScope, cap.Actor{AccountID: actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	page, e = source.ListAccountContexts(t.Context(), actor, 1, "")
	if e != nil || len(page.Organizations) != 0 || page.NextAfterOrgID != want[0] {
		t.Fatal("filtered reference broke cursor", e)
	}
	second, e = source.ListAccountContexts(t.Context(), actor, 1, page.NextAfterOrgID)
	if e != nil || len(second.Organizations) != 1 || second.Organizations[0].ID != want[1] {
		t.Fatal("filtered page lost later context", e)
	}
	// Only opaque own account references are visible in this bootstrap. It is
	// read-only and cannot replace an organization scope for writes/private roots.
	e = database.WithOrgBootstrapTx(t.Context(), fx.f.Runtime, func(tx pgx.Tx) error {
		if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.account_context',$1,true)`, actor.AccountID); e != nil {
			return e
		}
		var n int
		if e := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_memberships`).Scan(&n); e != nil || n != 2 {
			t.Fatal("account reference RLS", n, e)
		}
		if e := tx.QueryRow(t.Context(), `SELECT count(*) FROM organizations`).Scan(&n); e != nil || n != 0 {
			t.Fatal("bootstrap leaked private roots", n, e)
		}
		tag, e := tx.Exec(t.Context(), `UPDATE org_memberships SET revision=revision+1`)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 0 {
			t.Fatal("account discovery authorized writes")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	for _, scope := range []kernel.OrgScope{fx.scope, other.scope} {
		if e := database.WithOrgTx(t.Context(), fx.f.Runtime, scope, func(tx pgx.Tx) error {
			var installed string
			e := tx.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.account_context',true),'')`).Scan(&installed)
			if installed != "" {
				t.Fatal("account discovery scope leaked into organization")
			}
			return e
		}); e != nil {
			t.Fatal("scope cleanup", e)
		}
	}
	if _, e := source.ListAccountContexts(t.Context(), actor, 0, ""); e == nil {
		t.Fatal("unbounded page")
	}
	if _, e := source.ListAccountContexts(t.Context(), actor, 1, "INVALID"); e == nil {
		t.Fatal("noncanonical cursor")
	}
	unsafe := *source
	unsafe.Pool = fx.f.Admin
	if _, e := unsafe.ListAccountContexts(t.Context(), actor, 1, ""); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("privileged source accepted", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), "REVOKE SELECT ON org_employee_identities FROM "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	if _, e := source.ListAccountContexts(t.Context(), actor, 100, ""); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("storage failure hidden as empty contexts", e)
	}
}

func TestAccountContextRechecksProofAfterRootWaitAndConcurrentSelection(t *testing.T) {
	fx := legalFixture(t)
	source := contextStore(fx)
	actor := app.CompanyActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := source.ReadAccountContext(t.Context(), actor, fx.scope, fx.member)
			if e != nil {
				failures <- e
			} else if v.Organization.MembershipID != fx.member {
				failures <- errors.New("membership drift")
			}
		}()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Fatal("concurrent selection", e)
	}
	// Source proof expiry after transport authentication/root waiting must not
	// leave sensitive controls enabled or accept an already expired session.
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '15 minutes'+interval '1 second' WHERE id=$1`, actor.SessionID); e != nil {
		t.Fatal(e)
	}
	blocker, e := fx.f.Admin.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback(context.Background())
	if _, e = blocker.Exec(t.Context(), `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	done := make(chan app.AccountContextView, 1)
	errs := make(chan error, 1)
	go func() {
		v, e := source.ReadAccountContext(t.Context(), actor, fx.scope, fx.member)
		done <- v
		errs <- e
	}()
	waitContextRootLock(t, fx)
	timer := time.NewTimer(1100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	if e = blocker.Commit(t.Context()); e != nil {
		t.Fatal(e)
	}
	v := <-done
	e = <-errs
	if e != nil || !slices.Contains(v.Permissions, access.Permission("idn.employee.read")) || slices.Contains(v.Permissions, access.Permission("idn.invitation.manage")) || len(v.InvitationGrants) != 0 {
		t.Fatal("stale proof after wait granted controls", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE account_id=$1`, actor.AccountID); e != nil {
		t.Fatal(e)
	}
	if _, e := source.ReadAccountContext(t.Context(), actor, fx.scope, fx.member); !errors.Is(e, app.ErrUnauthenticated) {
		t.Fatal("revoked source family admitted", e)
	}
}
func waitContextRootLock(t *testing.T, fx legalFixtureState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var blocked int
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock'`, fx.f.RuntimeRole).Scan(&blocked); e != nil {
			t.Fatal(e)
		}
		if blocked > 0 {
			return
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-timer.C:
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
	}
	t.Fatal("current context never reached observed root wait")
}

func TestAccountContextSelectionLosesAuthorityOnActualMembershipReduction(t *testing.T) {
	fx := membershipAccessFixture(t)
	member := accessMember(t, fx)
	actor := fx.transfer.actors[member]
	source := contextStore(fx.legalFixtureState)
	if _, e := source.ReadAccountContext(t.Context(), actor, fx.scope, member); e != nil {
		t.Fatal("current member context", e)
	}
	current := accessRead(t, fx, member)
	paused, e := accessWrite(t, fx, memberapp.MembershipAccessWrite{Action: memberapp.MembershipSuspend, MembershipID: member, ExpectedRevision: current.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic context suspension"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := source.ReadAccountContext(t.Context(), actor, fx.scope, member); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("suspended membership selected", e)
	}
	page, e := source.ListAccountContexts(t.Context(), actor, 100, "")
	if e != nil || len(page.Organizations) != 0 {
		t.Fatal("suspended membership discovered", e)
	}
	if _, e := source.ReadAccountContext(t.Context(), fx.actor, fx.scope, fx.member); e != nil {
		t.Fatal("unrelated current manager lost access", e)
	}
	if _, e = accessWrite(t, fx, memberapp.MembershipAccessWrite{Action: memberapp.MembershipReactivate, MembershipID: member, ExpectedRevision: paused.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic context return"}); e != nil {
		t.Fatal(e)
	}
	if _, e := source.ReadAccountContext(t.Context(), actor, fx.scope, member); e != nil {
		t.Fatal("current explicit return not revalidated", e)
	}
}
