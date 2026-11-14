//go:build integration

package adapters_test

import (
	"errors"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	orgd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
)

func employeeMergeStore(t *testing.T, fx memberAccessFixture) *memberpg.EmployeeMergeStore {
	t.Helper()
	fx.f.GrantEmployeeMergeCommands(t)
	grantTeamTables(t, fx.legalFixtureState)
	catalog, e := audit.NewCatalog(memberpg.EmployeeMergeAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &memberpg.EmployeeMergeStore{Pool: fx.f.Runtime, Authority: &orgpg.EmployeeMergeAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}}, Teams: orgpg.EmployeeMergeTeamProjector{NewID: jobs.NewUUIDv4}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func mergeProfileStore(t *testing.T, fx memberAccessFixture) *memberpg.EmployeeStore {
	t.Helper()
	catalog, e := audit.NewCatalog(memberpg.EmployeeAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &memberpg.EmployeeStore{Pool: fx.f.Runtime, Authority: &orgpg.EmployeeAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func mergeEmployee(t *testing.T, fx memberAccessFixture, name string) string {
	t.Helper()
	v, _, e := writeEmployee(t, fx.legalFixtureState, mergeProfileStore(t, fx), employeeCommand(t, app.EmployeeWriteInput{Kind: app.CreateEmployee, AllowPossibleDuplicate: true, Profile: d.EmployeeProfile{DisplayName: name, WorkEmail: "merge@example.invalid"}}))
	if e != nil {
		t.Fatal(e)
	}
	return v.EmployeeID
}
func mergePreview(t *testing.T, fx memberAccessFixture, s *memberpg.EmployeeMergeStore, source, survivor string) app.EmployeeMergeView {
	t.Helper()
	v, e := s.PreviewEmployeeMerge(t.Context(), fx.scope, accessActor(fx), fx.member, source, survivor)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func mergeCommand(t *testing.T, fx memberAccessFixture, v app.EmployeeMergeView) app.EmployeeMergeCommand {
	t.Helper()
	a, _ := v.Source.Snapshot(fx.scope)
	b, _ := v.Survivor.Snapshot(fx.scope)
	av, _ := a.Employee.Snapshot(fx.scope)
	bv, _ := b.Employee.Snapshot(fx.scope)
	return app.EmployeeMergeCommand{OperationID: onboardingID(t), SourceID: av.ID, SurvivorID: bv.ID, SourceRevision: av.Revision, SurvivorRevision: bv.Revision, Reviewed: true, ReviewDigest: v.Digest, Reason: "Synthetic reviewed duplicate"}
}
func mergeWrite(t *testing.T, fx memberAccessFixture, s *memberpg.EmployeeMergeStore, c app.EmployeeMergeCommand) (app.EmployeeMergeResult, error) {
	t.Helper()
	return s.MergeEmployees(t.Context(), fx.scope, accessActor(fx), fx.member, c)
}
func mergeAddTeam(t *testing.T, fx memberAccessFixture, source, survivor, name string) string {
	t.Helper()
	store := teamStore(t, fx.legalFixtureState)
	team, _, e := teamWrite(t, fx.legalFixtureState, store, teamCommand(t, orgapp.TeamWriteInput{Kind: orgapp.CreateTeam, Profile: orgd.TeamProfile{Name: name}}))
	if e != nil {
		t.Fatal(e)
	}
	for n, employee := range []string{source, survivor} {
		if employee == "" {
			continue
		}
		if _, _, e := teamWrite(t, fx.legalFixtureState, store, teamCommand(t, orgapp.TeamWriteInput{Kind: orgapp.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employee, ExpectedRevision: int64(n + 1)})); e != nil {
			t.Fatal(e)
		}
	}
	return team.TeamID
}
func mergeStableData(t *testing.T, fx memberAccessFixture) string {
	t.Helper()
	var out string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(m)ORDER BY org_id,id)FROM org_memberships m),(SELECT jsonb_agg(to_jsonb(r)ORDER BY org_id,id)FROM org_role_assignments r),(SELECT jsonb_agg(to_jsonb(p)ORDER BY org_id,employee_id)FROM org_employee_profiles p),(SELECT jsonb_agg(to_jsonb(f)ORDER BY id)FROM account_session_families f))::text`).Scan(&out); e != nil {
		t.Fatal(e)
	}
	return out
}
func TestEmployeeMergeTransfersTeamsPreservesHistoryAndResolvesChains(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := employeeMergeStore(t, fx)
	source := mergeEmployee(t, fx, "Synthetic Duplicate")
	survivor := mergeEmployee(t, fx, "Synthetic Duplicate")
	one := mergeAddTeam(t, fx, source, "", "Synthetic first")
	two := mergeAddTeam(t, fx, source, survivor, "Synthetic shared")
	before := mergeStableData(t, fx)
	c := mergeCommand(t, fx, mergePreview(t, fx, s, source, survivor))
	out, e := mergeWrite(t, fx, s, c)
	if e != nil || len(out.Teams) != 2 || out.Replayed {
		t.Fatal("merge", e)
	}
	created, existing := 0, 0
	for _, effect := range out.Teams {
		if effect.Disposition == "CREATED" {
			created++
		} else {
			existing++
		}
	}
	if created != 1 || existing != 1 {
		t.Fatal("dedup", created, existing)
	}
	if mergeStableData(t, fx) != before {
		t.Fatal("membership/profile/account rewrite")
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_team_links WHERE org_id=$1 AND employee_id=$2 AND ended_at IS NULL`, 0, fx.scope.OrgID().String(), source)
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_team_links WHERE org_id=$1 AND employee_id=$2 AND team_id IN($3,$4) AND ended_at IS NULL`, 2, fx.scope.OrgID().String(), survivor, one, two)
	again, e := mergeWrite(t, fx, s, c)
	if e != nil || !again.Replayed || again.OccurredAt != out.OccurredAt {
		t.Fatal("original receipt", e)
	}
	changed := c
	changed.Reason = "Changed reason"
	if _, e := mergeWrite(t, fx, s, changed); !errors.Is(e, app.ErrAccessOperationReused) {
		t.Fatal("intent reuse", e)
	}
	third := mergeEmployee(t, fx, "Synthetic Canonical")
	later := mergeCommand(t, fx, mergePreview(t, fx, s, survivor, third))
	if _, e := mergeWrite(t, fx, s, later); e != nil {
		t.Fatal("chained merge", e)
	}
	resolved, e := s.ResolveEmployee(t.Context(), fx.scope, accessActor(fx), fx.member, source)
	if e != nil || len(resolved.Chain) != 3 || resolved.Chain[0] != source || resolved.Chain[1] != survivor || resolved.Chain[2] != third {
		t.Fatal("historical resolution", e)
	}
	again, e = mergeWrite(t, fx, s, c)
	if e != nil || again.SurvivorID != survivor || again.SurvivorState != d.EmployeeActive || again.OccurredAt != out.OccurredAt {
		t.Fatal("later state changed receipt", e)
	}
}
func TestEmployeeMergeConflictReviewAndLiveOrientation(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := employeeMergeStore(t, fx)
	first, second := accessMember(t, fx), accessMember(t, fx)
	a, b := accessRead(t, fx, first).State.EmployeeID, accessRead(t, fx, second).State.EmployeeID
	for _, pair := range [][2]string{{a, b}, {a, a}} {
		if _, e := s.PreviewEmployeeMerge(t.Context(), fx.scope, accessActor(fx), fx.member, pair[0], pair[1]); !errors.Is(e, d.ErrMergeConflict) {
			t.Fatal("live/self conflict", e)
		}
	}
	source := mergeEmployee(t, fx, "Synthetic no login")
	if _, e := s.PreviewEmployeeMerge(t.Context(), fx.scope, accessActor(fx), fx.member, a, source); !errors.Is(e, d.ErrMergeConflict) {
		t.Fatal("live source orientation", e)
	}
	v := mergePreview(t, fx, s, source, a)
	c := mergeCommand(t, fx, v)
	if _, _, e := writeEmployee(t, fx.legalFixtureState, mergeProfileStore(t, fx), employeeCommand(t, app.EmployeeWriteInput{Kind: app.ChangeEmployee, EmployeeID: source, ExpectedRevision: 1, Profile: d.EmployeeProfile{DisplayName: "Synthetic corrected", WorkEmail: "merge@example.invalid"}})); e != nil {
		t.Fatal(e)
	}
	if _, e := mergeWrite(t, fx, s, c); !errors.Is(e, d.ErrMergeConflict) {
		t.Fatal("stale review", e)
	}
	c = mergeCommand(t, fx, mergePreview(t, fx, s, source, a))
	before := mergeStableData(t, fx)
	if _, e := mergeWrite(t, fx, s, c); e != nil {
		t.Fatal("live survivor", e)
	}
	if mergeStableData(t, fx) != before {
		t.Fatal("unrelated login altered")
	}
}
func TestEmployeeMergeRejectsChangedTeamReview(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := employeeMergeStore(t, fx)
	a, b := mergeEmployee(t, fx, "Synthetic A"), mergeEmployee(t, fx, "Synthetic B")
	c := mergeCommand(t, fx, mergePreview(t, fx, s, a, b))
	mergeAddTeam(t, fx, a, "", "Synthetic newly assigned team")
	if _, e := mergeWrite(t, fx, s, c); !errors.Is(e, d.ErrMergeConflict) {
		t.Fatal("stale team review", e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_employee_identities WHERE id=$1 AND merged_into IS NULL AND state='ACTIVE'`, 1, a)
}

func TestEmployeeMergeTwelveWayRetriesAndOppositeRace(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := employeeMergeStore(t, fx)
	a, b := mergeEmployee(t, fx, "Synthetic A"), mergeEmployee(t, fx, "Synthetic B")
	c := mergeCommand(t, fx, mergePreview(t, fx, s, a, b))
	results := make(chan app.EmployeeMergeResult, 12)
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() { out, e := mergeWrite(t, fx, s, c); results <- out; errs <- e })
	}
	wg.Wait()
	close(results)
	close(errs)
	effects := 0
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	for out := range results {
		if !out.Replayed {
			effects++
		}
	}
	if effects != 1 {
		t.Fatal("repeated effects", effects)
	}
	first, last := mergeEmployee(t, fx, "Synthetic distinct source"), mergeEmployee(t, fx, "Synthetic distinct target")
	base := mergeCommand(t, fx, mergePreview(t, fx, s, first, last))
	contenders := make(chan error, 12)
	for range 12 {
		candidate := base
		candidate.OperationID = onboardingID(t)
		wg.Go(func() { _, e := mergeWrite(t, fx, s, candidate); contenders <- e })
	}
	wg.Wait()
	close(contenders)
	success, conflict := 0, 0
	for e := range contenders {
		if e == nil {
			success++
		} else if errors.Is(e, d.ErrMergeConflict) || errors.Is(e, d.ErrConflict) {
			conflict++
		} else {
			t.Fatal("distinct CAS", e)
		}
	}
	if success != 1 || conflict != 11 {
		t.Fatal("distinct effects", success, conflict)
	}

	x, y := mergeEmployee(t, fx, "Synthetic X"), mergeEmployee(t, fx, "Synthetic Y")
	left := mergeCommand(t, fx, mergePreview(t, fx, s, x, y))
	right := mergeCommand(t, fx, mergePreview(t, fx, s, y, x))
	race := make(chan error, 2)
	for _, cmd := range []app.EmployeeMergeCommand{left, right} {
		wg.Go(func() { _, e := mergeWrite(t, fx, s, cmd); race <- e })
	}
	wg.Wait()
	close(race)
	won, lost := 0, 0
	for e := range race {
		if e == nil {
			won++
		} else if errors.Is(e, d.ErrMergeConflict) {
			lost++
		} else {
			t.Fatal(e)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatal("cycle race", won, lost)
	}
}
func TestEmployeeMergeLateFailureRollsBackProjectionPointerAndAudit(t *testing.T) {
	for _, table := range []string{"org_employee_merge_commands", "org_team_links", "org_audit_events"} {
		t.Run(table, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			s := employeeMergeStore(t, fx)
			a, b := mergeEmployee(t, fx, "Synthetic A"), mergeEmployee(t, fx, "Synthetic B")
			mergeAddTeam(t, fx, a, "", "Synthetic failure team")
			c := mergeCommand(t, fx, mergePreview(t, fx, s, a, b))
			var links string
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(l)ORDER BY id)::text FROM org_team_links l`).Scan(&links); e != nil {
				t.Fatal(e)
			}
			if _, e := fx.f.Admin.Exec(t.Context(), "CREATE FUNCTION syn_merge_failure()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic merge failure';END $$;CREATE CONSTRAINT TRIGGER syn_merge_failure AFTER INSERT ON "+pgx.Identifier{table}.Sanitize()+" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION syn_merge_failure()"); e != nil {
				t.Fatal(e)
			}
			out, e := mergeWrite(t, fx, s, c)
			if !errors.Is(e, app.ErrRegisterUnavailable) || out.OperationID != "" {
				t.Fatal("provisional success", e)
			}
			var after string
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(l)ORDER BY id)::text FROM org_team_links l`).Scan(&after); e != nil || after != links {
				t.Fatal("partial team change", e)
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_employee_identities WHERE id=$1 AND merged_into IS NULL AND state='ACTIVE'`, 1, a)
			if _, e := fx.f.Admin.Exec(t.Context(), "DROP TRIGGER syn_merge_failure ON "+pgx.Identifier{table}.Sanitize()); e != nil {
				t.Fatal(e)
			}
			if _, e := mergeWrite(t, fx, s, c); e != nil {
				t.Fatal("recovery", e)
			}
		})
	}
}
