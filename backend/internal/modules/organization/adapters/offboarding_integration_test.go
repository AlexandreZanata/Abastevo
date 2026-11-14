//go:build integration

package adapters_test

import (
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	orgd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
)

func offboardingStore(t *testing.T, fx memberAccessFixture) *memberpg.OffboardingStore {
	t.Helper()
	fx.f.GrantOffboardingCommands(t)
	grantTeamTables(t, fx.legalFixtureState)
	catalog, e := audit.NewCatalog(memberpg.OffboardingAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &memberpg.OffboardingStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Audit: auditpg.NewStore(fx.f.Runtime, catalog), Authority: &orgpg.OffboardingAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}}}
}
func offboardingCommand(t *testing.T, v app.OffboardingView) app.OffboardingCommand {
	t.Helper()
	flags := make([]string, 0, len(v.Items))
	for _, i := range v.Items {
		flags = append(flags, i.Key)
	}
	return app.OffboardingCommand{OperationID: onboardingID(t), MembershipID: v.State.ID, ExpectedRevision: v.State.Revision, EmployeeRevision: v.EmployeeRevision, Reason: "Synthetic access ending", HandoverDigest: v.Digest, Flags: flags}
}
func offboardingRead(t *testing.T, fx memberAccessFixture, s *memberpg.OffboardingStore, target string) app.OffboardingView {
	t.Helper()
	v, e := s.ReadOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, target, false)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestOffboardingEndsActualAccessAndRetainsOriginalOutcome(t *testing.T) {
	for _, terminate := range []bool{false, true} {
		t.Run(map[bool]string{false: "retain", true: "terminate"}[terminate], func(t *testing.T) {
			fx := membershipAccessFixture(t)
			s := offboardingStore(t, fx)
			target := accessMember(t, fx)
			v := offboardingRead(t, fx, s, target)
			c := offboardingCommand(t, v)
			c.TerminateEmployee = terminate
			out, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c)
			if e != nil || out.View.State.EndReason != d.Offboarded || out.Replayed {
				t.Fatal("offboard", e)
			}
			if terminate && out.View.EmployeeState != d.EmployeeTerminated || !terminate && out.View.EmployeeState != d.EmployeeActive {
				t.Fatal("employee choice")
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND ended_at IS NULL`, 0, target)
			again, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c)
			if e != nil || !again.Replayed || again.OccurredAt != out.OccurredAt {
				t.Fatal("original retry", e)
			}
			c.Reason = "Changed intent"
			if _, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c); !errors.Is(e, app.ErrAccessOperationReused) {
				t.Fatal("reuse", e)
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM account_session_families f JOIN org_memberships m ON m.account_id=f.account_id WHERE m.id=$1 AND f.revoked_at IS NULL`, 1, target)
		})
	}
}
func TestOffboardingSelfLeaveSuspendedOrNoRoleAndLastOwner(t *testing.T) {
	for _, suspended := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_role", true: "suspended"}[suspended], func(t *testing.T) {
			fx := membershipAccessFixture(t)
			s := offboardingStore(t, fx)
			target := accessMember(t, fx)
			actor := fx.transfer.actors[target]
			a := app.EmployeeActor{AccountID: actor.AccountID, SessionID: actor.SessionID}
			if suspended {
				v := accessRead(t, fx, target)
				if _, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: v.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic pause"}); e != nil {
					t.Fatal(e)
				}
			} else {
				if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE membership_id=$1 AND ended_at IS NULL`, target); e != nil {
					t.Fatal(e)
				}
			}
			v, e := s.ReadOffboarding(t.Context(), fx.scope, a, "", target, true)
			if e != nil {
				t.Fatal(e)
			}
			c := offboardingCommand(t, v)
			c.Self = true
			out, e := s.CompleteOffboarding(t.Context(), fx.scope, a, "", c)
			if e != nil || out.View.State.EndReason != d.Left || out.View.EmployeeState != d.EmployeeActive {
				t.Fatal("self leave", e)
			}
			again, e := s.CompleteOffboarding(t.Context(), fx.scope, a, "", c)
			if e != nil || !again.Replayed || again.OccurredAt != out.OccurredAt {
				t.Fatal("ended self retry", e)
			}
			if _, e := s.ReadOffboarding(t.Context(), fx.scope, a, "", fx.member, true); !errors.Is(e, app.ErrEmployeeNotFound) {
				t.Fatal("self entry reaches another account", e)
			}
		})
	}
	fx := membershipAccessFixture(t)
	s := offboardingStore(t, fx)
	v, e := s.ReadOffboarding(t.Context(), fx.scope, accessActor(fx), "", fx.member, true)
	if e != nil {
		t.Fatal(e)
	}
	c := offboardingCommand(t, v)
	c.Self = true
	if _, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), "", c); !errors.Is(e, kernel.ErrLastOrgOwner) {
		t.Fatal("last owner self leave", e)
	}
}
func TestOffboardingReviewChangesAndRetainedSourceFlags(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := offboardingStore(t, fx)
	target := accessMember(t, fx)
	before := offboardingRead(t, fx, s, target)
	old := offboardingCommand(t, before)

	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	settingsCatalog, e := audit.NewCatalog(orgpg.SettingsAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	settings := &orgpg.SettingsStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Audit: auditpg.NewStore(fx.f.Runtime, settingsCatalog)}
	settingsRead, e := settings.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0)
	if e != nil {
		t.Fatal(e)
	}
	settingsValue, e := settingsRead.Settings.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	values := settingsValue.Values
	values.Responsibilities.PlatformNotices.PrimaryID = target
	if _, _, e := settings.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, settingsValue.Revision, values, "")); e != nil {
		t.Fatal(e)
	}
	team := teamStore(t, fx.legalFixtureState)
	created, _, e := teamWrite(t, fx.legalFixtureState, team, teamCommand(t, orgapp.TeamWriteInput{Kind: orgapp.CreateTeam, Profile: orgd.TeamProfile{Name: "Synthetic handover"}}))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx.legalFixtureState, team, teamCommand(t, orgapp.TeamWriteInput{Kind: orgapp.AddTeamEmployee, TeamID: created.TeamID, EmployeeID: before.State.EmployeeID, ExpectedRevision: 1})); e != nil {
		t.Fatal(e)
	}
	if _, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, old); !errors.Is(e, d.ErrOpenResponsibilities) {
		t.Fatal("stale handover", e)
	}
	v := offboardingRead(t, fx, s, target)
	if len(v.Items) != 2 {
		t.Fatal("actual team inventory", len(v.Items))
	}
	c := offboardingCommand(t, v)
	c.Flags = nil
	if _, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c); !errors.Is(e, d.ErrOpenResponsibilities) {
		t.Fatal("unreviewed flag", e)
	}
	c.Flags = offboardingCommand(t, v).Flags
	out, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c)
	if e != nil || len(out.View.Items) != 2 {
		t.Fatal("flagged source", e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_team_links WHERE employee_id=$1 AND end_reason='MEMBERSHIP_ENDED' AND source_membership_id=$2`, 1, before.State.EmployeeID, target)
	invitationCount(t, fx.invitationFixtureState, `SELECT jsonb_array_length(result->'items') FROM org_offboarding_commands WHERE operation_id=$1`, 2, c.OperationID)
	visible := offboardingRead(t, fx, s, target)
	if len(visible.Items) != 2 {
		t.Fatal("pending flags hidden after reduction")
	}
}
func TestOffboardingTwelveWayRetryCASAndDeferredFailures(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := offboardingStore(t, fx)
	target := accessMember(t, fx)
	c := offboardingCommand(t, offboardingRead(t, fx, s, target))
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	results := make(chan app.OffboardingResult, 12)
	for range 12 {
		wg.Go(func() {
			out, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c)
			errs <- e
			results <- out
		})
	}
	wg.Wait()
	close(errs)
	close(results)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	first := 0
	for out := range results {
		if !out.Replayed {
			first++
		}
	}
	if first != 1 {
		t.Fatal("multiple effects", first)
	}

	nextTarget := accessMember(t, fx)
	base := offboardingCommand(t, offboardingRead(t, fx, s, nextTarget))
	conflicts := make(chan error, 12)
	start := make(chan struct{})
	var contenders sync.WaitGroup
	for range 12 {
		candidate := base
		candidate.OperationID = onboardingID(t)
		contenders.Go(func() {
			<-start
			_, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, candidate)
			conflicts <- e
		})
	}
	close(start)
	contenders.Wait()
	close(conflicts)
	won, lost := 0, 0
	for e := range conflicts {
		if e == nil {
			won++
		} else if errors.Is(e, d.ErrConflict) {
			lost++
		} else {
			t.Fatal("CAS contender", e)
		}
	}
	if won != 1 || lost != 11 {
		t.Fatal("distinct CAS", won, lost)
	}
	for _, table := range []string{"org_offboarding_commands", "org_offboarding_notices", "org_audit_events"} {
		t.Run(table, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			s := offboardingStore(t, fx)
			target := accessMember(t, fx)
			c := offboardingCommand(t, offboardingRead(t, fx, s, target))
			c.TerminateEmployee = true
			if _, e := fx.f.Admin.Exec(t.Context(), "CREATE FUNCTION syn_offboard_failure()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic offboarding failure';END $$;CREATE CONSTRAINT TRIGGER syn_offboard_failure AFTER INSERT ON "+pgx.Identifier{table}.Sanitize()+" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION syn_offboard_failure()"); e != nil {
				t.Fatal(e)
			}
			out, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c)
			if !errors.Is(e, app.ErrRegisterUnavailable) || out.OperationID != "" {
				t.Fatal("provisional success", e)
			}
			if unitState(t, fx, target) != "ACTIVE" {
				t.Fatal("partial end")
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND ended_at IS NULL`, 1, target)
			if _, e := fx.f.Admin.Exec(t.Context(), "DROP TRIGGER syn_offboard_failure ON "+pgx.Identifier{table}.Sanitize()); e != nil {
				t.Fatal(e)
			}
			if _, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c); e != nil {
				t.Fatal("recovery", e)
			}
		})
	}
}
