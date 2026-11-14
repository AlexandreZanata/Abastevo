//go:build integration

package adapters_test

import (
	"bytes"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// structuralTeam creates one ACTIVE team through the real team command.
func structuralTeam(t *testing.T, fx legalFixtureState, s *orgpg.TeamStore, name string) string {
	t.Helper()
	team, _, err := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: name}}))
	if err != nil {
		t.Fatal(err)
	}
	return team.TeamID
}

// structuralLink adds or removes an employee link through the real command.
func structuralLink(t *testing.T, fx legalFixtureState, s *orgpg.TeamStore, kind app.TeamWriteKind, team, employee string) {
	t.Helper()
	var revision int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_teams WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), team).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if _, _, err := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: kind, TeamID: team, EmployeeID: employee, ExpectedRevision: revision})); err != nil {
		t.Fatal("team link", kind, err)
	}
}

// 07-permissions §10 example 3 and BR-AUTH-004 at the actual composed company
// entry points: the held meter reading review (a record that stores the unit
// and asset it had when held) and the asset routes (current allocation via
// the FLEET-005 port). Every grant uses the real role API.
func TestStructuralScopesActualCompanyHTTPExampleThree(t *testing.T) {
	fx := syncFixture(t)
	legal := fx.legalFixtureState
	org := fx.scope.OrgID().String()
	s := lifecycleStore(t, legal)
	grantCostCentreTables(t, legal)
	grantTeamTables(t, legal)
	runtime := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE ON org_asset_unit_periods,org_asset_cost_centre_periods TO "+runtime+";GRANT SELECT,INSERT ON org_role_commands TO "+runtime+";GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+runtime); err != nil {
		t.Fatal(err)
	}
	teams := teamStore(t, legal)
	var norte string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, org).Scan(&norte); err != nil {
		t.Fatal(err)
	}
	sul, leste := allocationUnit(t, legal, "SUL"), allocationUnit(t, legal, "LESTE")
	centreNorte, centreSul := allocationCentre(t, legal, "CC-NORTE"), allocationCentre(t, legal, "CC-SUL")
	teamA, teamB := structuralTeam(t, legal, teams, "Synthetic team A"), structuralTeam(t, legal, teams, "Synthetic team B")
	// The device author belongs to team A when the reading is held.
	structuralLink(t, legal, teams, app.AddTeamEmployee, teamA, fx.person.employee)

	// Vehicle V: ACTIVE in Norte with cost centre Norte, meter installed.
	v, revision := lifecycleAssetIn(t, legal, s, norte, fleet.Active)
	draft, _ := lifecycleAssetIn(t, legal, s, norte, fleet.Draft)
	if err := allocationExec(t, legal, `INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,$4)`, org, v, centreNorte, allocationStart(t, legal, v)); err != nil {
		t.Fatal(err)
	}
	meters := meterStore(t, legal)
	meters.Authority = &orgpg.MeterAuthority{Memberships: legal.store.Memberships, Roles: legal.store.Roles, Capabilities: legal.flags, Devices: fx.devices}
	install := meterInstallInput(t, v, lifecycleDBNow(t, legal).Add(-time.Minute))
	install.ExpectedAssetRevision = revision
	installed, _, err := meterInstall(t, legal, meters, meterInstallCommand(t, install))
	if err != nil {
		t.Fatal("meter on the active vehicle", err)
	}
	reviews := meterReviewStore(t, fx)
	observed := lifecycleDBNow(t, legal).Add(-time.Second).Truncate(time.Microsecond)
	_, held, _, err := storeMeterHeldReview(t, fx, reviews, meterObservedCommand(t, installed, observed, 7))
	if err != nil {
		t.Fatal("held March reading", err)
	}
	var storedUnit, storedAsset string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT unit_id::text,asset_id::text FROM org_meter_held_readings WHERE org_id=$1 AND review_item_id=$2`, org, held.ReviewItemID).Scan(&storedUnit, &storedAsset); err != nil || storedUnit != norte || storedAsset != v {
		t.Fatal("held reading did not store the Norte snapshot", err)
	}

	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{67}, 32), Pool: fx.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	base := "/v1/orgs/" + org
	owner := matrixNewPerson(t, fx.deviceFixtureState, router)
	matrixAssign(t, fx.deviceFixtureState, owner, "OWNER", "ORG", "ACTIVE")
	assign := func(t *testing.T, member, role, scope, target string, want int, code string) map[string]any {
		t.Helper()
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "membership_id": member, "role": role, "scope": scope, "target_id": target, "reason": "Synthetic structural grant"}
		return matrixCall(t, fx.deviceFixtureState, router, owner, base+"/roles/assign", body, want, code, "", nil)
	}
	person := func(role, scope, target string) matrixPerson {
		t.Helper()
		p := matrixNewPerson(t, fx.deviceFixtureState, router)
		matrixAssign(t, fx.deviceFixtureState, p, "", "", "NONE")
		if scope == "SELF" {
			target = p.member
		}
		if role != "" {
			ack := assign(t, p.member, role, scope, target, 201, "")
			if ack["scope_type"] != scope || ack["scope_target"] != target || ack["replayed"] != false {
				t.Fatal("structural grant acknowledgement changed", ack["scope_type"], ack["scope_target"])
			}
		}
		return p
	}
	// ADMIN carries O for meter.reading.correct and fleet.asset.read and is
	// grantable by OWNER (BR-AUTH-007); SUPERVISOR/FINANCE are not (their O
	// cells exceed OWNER's S cells), with or without a structural scope.
	norteSup, sulSup := person("ADMIN", "UNIT", norte), person("ADMIN", "UNIT", sul)
	centreNorteSup, centreSulSup := person("ADMIN", "COST_CENTRE", centreNorte), person("ADMIN", "COST_CENTRE", centreSul)
	assetSup := person("ADMIN", "ASSET", v)
	teamASup, teamBSup := person("ADMIN", "TEAM", teamA), person("ADMIN", "TEAM", teamB)
	// Team membership alone: linked to team A, holding only a team-B grant or
	// a SELF grant, neither of which names team A.
	teamMember, selfMember := person("ADMIN", "TEAM", teamB), person("ADMIN", "SELF", "")
	structuralLink(t, legal, teams, app.AddTeamEmployee, teamA, teamMember.employee)
	structuralLink(t, legal, teams, app.AddTeamEmployee, teamA, selfMember.employee)

	reviewPath := base + "/sync/reviews/meter-readings/" + held.ReviewItemID + "/read"
	assetPath := base + "/fleet/assets/" + v + "/read"
	call := func(t *testing.T, actor matrixPerson, path string, want int, permission string) map[string]any {
		t.Helper()
		code := ""
		if want == 403 {
			code = "FORBIDDEN"
		}
		if want == 404 {
			code = "NOT_FOUND"
		}
		return matrixCall(t, fx.deviceFixtureState, router, actor, path, map[string]any{}, want, code, permission, nil)
	}
	review := func(t *testing.T, actor matrixPerson, want int) {
		t.Helper()
		key := ""
		if want == 403 {
			key = "meter.reading.correct"
		}
		v := call(t, actor, reviewPath, want, key)
		if want == 200 && v["review_item_id"] != held.ReviewItemID {
			t.Fatal("structurally covered review not served")
		}
	}
	asset := func(t *testing.T, actor matrixPerson, want int) {
		t.Helper()
		key := ""
		if want == 403 {
			key = "fleet.asset.read"
		}
		call(t, actor, assetPath, want, key)
	}
	t.Run("march", func(t *testing.T) {
		for _, tc := range []struct {
			actor          matrixPerson
			review, assetW int
		}{
			{norteSup, 200, 200}, {sulSup, 403, 403}, {centreNorteSup, 403, 200}, {centreSulSup, 403, 403},
			{assetSup, 200, 200}, {teamASup, 200, 403}, {teamBSup, 403, 403}, {teamMember, 403, 403}, {selfMember, 403, 403},
		} {
			review(t, tc.actor, tc.review)
			asset(t, tc.actor, tc.assetW)
		}
	})
	// "April": V moves to Sul and cost centre Sul (test-only reallocation;
	// the public transfer command is EMP-FLEET-008).
	april := lifecycleDBNow(t, legal).Truncate(time.Microsecond)
	if err := allocationReallocate(t, legal, "org_asset_unit_periods", "unit_id", v, sul, april); err != nil {
		t.Fatal(err)
	}
	if err := allocationReallocate(t, legal, "org_asset_cost_centre_periods", "cost_centre_id", v, centreSul, april); err != nil {
		t.Fatal(err)
	}
	t.Run("april", func(t *testing.T) {
		for _, tc := range []struct {
			actor          matrixPerson
			review, assetW int
		}{
			// The March record keeps Norte; the reassigned asset never widens it.
			{norteSup, 200, 403}, {sulSup, 403, 200}, {centreNorteSup, 403, 403}, {centreSulSup, 403, 200}, {assetSup, 200, 200},
		} {
			review(t, tc.actor, tc.review)
			asset(t, tc.actor, tc.assetW)
		}
		history := base + "/fleet/assets/" + v + "/unit-allocations/list"
		if page := call(t, sulSup, history, 200, ""); len(page["items"].([]any)) != 2 {
			t.Fatal("Sul supervisor cannot read the moved asset's history")
		}
		call(t, norteSup, history, 403, "fleet.asset.read")
	})
	t.Run("author changes team", func(t *testing.T) {
		structuralLink(t, legal, teams, app.RemoveTeamEmployee, teamA, fx.person.employee)
		structuralLink(t, legal, teams, app.AddTeamEmployee, teamB, fx.person.employee)
		review(t, teamASup, 200)
		review(t, teamBSup, 403)
	})
	t.Run("organization-level resources carry no structure", func(t *testing.T) {
		call(t, norteSup, base+"/sync/reviews/meter-readings/list", 403, "meter.reading.correct")
		call(t, sulSup, base+"/fleet/assets/list", 403, "fleet.asset.read")
		call(t, norteSup, base+"/sync/reviews/meter-readings/"+onboardingID(t)+"/read", 404, "")
		call(t, sulSup, base+"/fleet/assets/"+onboardingID(t)+"/read", 404, "")
		call(t, norteSup, base+"/fleet/assets/"+draft+"/read", 403, "fleet.asset.read")
		// A structurally scoped auth.role.assign covers no role management.
		call(t, norteSup, base+"/roles/list", 403, "auth.role.assign")
		matrixCall(t, fx.deviceFixtureState, router, norteSup, base+"/roles/assign", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "membership_id": sulSup.member, "role": "DRIVER", "scope": "UNIT", "target_id": norte, "reason": "Synthetic unit admin grant"}, 403, "FORBIDDEN", "auth.role.assign", nil)
	})
	t.Run("grant command negatives", func(t *testing.T) {
		target := matrixNewPerson(t, fx.deviceFixtureState, router)
		other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
		var foreignUnit string
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, other.scope.OrgID().String()).Scan(&foreignUnit); err != nil {
			t.Fatal(err)
		}
		assign(t, target.member, "ADMIN", "UNIT", foreignUnit, 404, "NOT_FOUND")
		assign(t, target.member, "ADMIN", "UNIT", centreNorte, 404, "NOT_FOUND")
		assign(t, target.member, "ADMIN", "TEAM", sul, 404, "NOT_FOUND")
		assign(t, target.member, "ADMIN", "ASSET", draft, 422, "VALIDATION_FAILED")
		if err := unitSchemaExec(t, legal, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, leste); err != nil {
			t.Fatal(err)
		}
		assign(t, target.member, "ADMIN", "UNIT", leste, 422, "VALIDATION_FAILED")
		assign(t, target.member, "OWNER", "UNIT", norte, 403, "FORBIDDEN")
		assign(t, target.member, "ADMIN", "UNIT", org, 403, "FORBIDDEN")
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "membership_id": target.member, "role": "APPROVER", "scope": "UNIT", "target_id": norte, "reason": "Synthetic replayed structural grant"}
		first := matrixCall(t, fx.deviceFixtureState, router, owner, base+"/roles/assign", body, 201, "", "", nil)
		replay := matrixCall(t, fx.deviceFixtureState, router, owner, base+"/roles/assign", body, 201, "", "", nil)
		if replay["replayed"] != true || replay["id"] != first["id"] || replay["scope_target"] != norte {
			t.Fatal("replayed structural grant did not return the original result")
		}
		body["target_id"] = sul
		matrixCall(t, fx.deviceFixtureState, router, owner, base+"/roles/assign", body, 409, "OPERATION_ID_REUSED", "", nil)
		body["operation_id"], body["target_id"] = onboardingID(t), norte
		matrixCall(t, fx.deviceFixtureState, router, owner, base+"/roles/assign", body, 409, "CONFLICT", "", nil)
		var count int
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, org, target.member).Scan(&count); err != nil || count != 2 {
			t.Fatal("refused structural grants left effects", count, err)
		}
	})
	t.Run("owner self-change keeps or narrows its scope", func(t *testing.T) {
		own := func(scope, target string) string {
			t.Helper()
			id := onboardingID(t)
			if err := unitSchemaExec(t, legal, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'ADMIN',$4,$5,clock_timestamp(),$6,'Synthetic own admin scope')`, org, id, owner.member, scope, target, legal.member); err != nil {
				t.Fatal(err)
			}
			return id
		}
		change := func(id, scope, target string, want int, code string) {
			t.Helper()
			matrixCall(t, fx.deviceFixtureState, router, owner, base+"/roles/"+id+"/change", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "role": "ADMIN", "scope": scope, "target_id": target, "reason": "Synthetic own scope change"}, want, code, "", nil)
		}
		unit := own("UNIT", norte)
		change(unit, "ORG", org, 403, "FORBIDDEN")
		change(unit, "UNIT", sul, 403, "FORBIDDEN")
		change(own("ORG", org), "UNIT", sul, 200, "")
	})
	t.Run("change and resolve under a structural scope", func(t *testing.T) {
		var id string
		var rev int64
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text,revision FROM org_role_assignments WHERE org_id=$1 AND membership_id=$2 AND scope_type='TEAM' AND ended_at IS NULL`, org, teamBSup.member).Scan(&id, &rev); err != nil {
			t.Fatal(err)
		}
		changed := matrixCall(t, fx.deviceFixtureState, router, owner, base+"/roles/"+id+"/change", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": rev, "role": "ADMIN", "scope": "TEAM", "target_id": teamA, "reason": "Synthetic team change"}, 200, "", "", nil)
		if changed["new_assignment"].(map[string]any)["scope_target"] != teamA {
			t.Fatal("structural change not applied")
		}
		review(t, teamBSup, 200)
		resolve := base + "/sync/reviews/meter-readings/" + held.ReviewItemID + "/resolve"
		decision := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "decision": "ACCEPT", "reason": ""}
		matrixCall(t, fx.deviceFixtureState, router, sulSup, resolve, decision, 403, "FORBIDDEN", "meter.reading.correct", nil)
		accepted := matrixCall(t, fx.deviceFixtureState, router, norteSup, resolve, decision, 200, "", "", nil)
		if accepted["resolution"].(map[string]any)["outcome"] != "ACCEPTED" {
			t.Fatal("Norte-scoped decision not recorded")
		}
	})
}
