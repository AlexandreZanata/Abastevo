//go:build integration

package adapters_test

import (
	"context"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

type structuralTargets struct {
	unit, centre, team, active, inactive, disposed, draft, archived string
}

// structuralSchemaFixture is the actual activated organization with every
// structure kind written through its real guards, and one ordinary member.
func structuralSchemaFixture(t *testing.T, fx legalFixtureState) structuralTargets {
	t.Helper()
	s := lifecycleStore(t, fx)
	grantCostCentreTables(t, fx)
	grantTeamTables(t, fx)
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1 ORDER BY created_at LIMIT 1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	targets := structuralTargets{unit: unit, centre: allocationCentre(t, fx, "CC-"+onboardingID(t)[:6]), team: structuralTeam(t, fx, teamStore(t, fx), "Synthetic structural team")}
	targets.active, _ = lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	targets.inactive, _ = lifecycleAssetIn(t, fx, s, unit, fleet.Inactive)
	targets.disposed, _ = lifecycleAssetIn(t, fx, s, unit, fleet.Disposed)
	targets.draft, _ = lifecycleAssetIn(t, fx, s, unit, fleet.Draft)
	targets.archived, _ = lifecycleAssetIn(t, fx, s, unit, fleet.Archived)
	return targets
}

func structuralMemberIn(t *testing.T, fx legalFixtureState) string {
	t.Helper()
	account := onboardingID(t)
	if _, err := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, account, "synthetic-scope-"+account[:8]); err != nil {
		t.Fatal(err)
	}
	_, member := teamDriver(t, fx, account)
	return member
}

func structuralGrantSQL(t *testing.T, fx legalFixtureState, id, member, role, kind, target string) error {
	t.Helper()
	return unitSchemaExec(t, fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp(),$7,'Synthetic structural schema')`, fx.scope.OrgID().String(), id, member, role, kind, target, fx.member)
}

// B-BR-EMP-235 at the database: same-organization kind-checked targets, live
// admission, archive neither blocked nor ending grants, immutable history and
// forced RLS.
func TestStructuralScopeSchemaAdmitsOnlyLiveSameOrganizationTargets(t *testing.T) {
	fx, _, _ := lifecycleFixture(t)
	org := fx.scope.OrgID().String()
	mine := structuralSchemaFixture(t, fx)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	foreign := structuralSchemaFixture(t, other)
	member := structuralMemberIn(t, fx)
	granted := map[string]string{}
	for _, tc := range []struct{ kind, target, column string }{
		{"UNIT", mine.unit, "scope_unit_id"}, {"COST_CENTRE", mine.centre, "scope_cost_centre_id"}, {"TEAM", mine.team, "scope_team_id"},
		{"ASSET", mine.active, "scope_asset_id"}, {"ASSET", mine.inactive, "scope_asset_id"}, {"ASSET", mine.disposed, "scope_asset_id"},
	} {
		id := onboardingID(t)
		if err := structuralGrantSQL(t, fx, id, member, "SUPERVISOR", tc.kind, tc.target); err != nil {
			t.Fatal("live structural target refused", tc.kind, err)
		}
		granted[tc.target] = id
		var exact bool
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT `+tc.column+`=scope_target AND num_nonnulls(scope_unit_id,scope_cost_centre_id,scope_team_id,scope_asset_id)=1 FROM org_role_assignments WHERE org_id=$1 AND id=$2`, org, id).Scan(&exact); err != nil || !exact {
			t.Fatal("generated same-organization reference not exact", tc.kind, err)
		}
	}
	var legacy bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT bool_and(num_nulls(scope_unit_id,scope_cost_centre_id,scope_team_id,scope_asset_id)=4) FROM org_role_assignments WHERE scope_type IN('ORG','SELF')`).Scan(&legacy); err != nil || !legacy {
		t.Fatal("ORG/SELF rows gained a structural reference", err)
	}
	for name, tc := range map[string]struct{ role, kind, target, code string }{
		"foreign unit":              {"SUPERVISOR", "UNIT", foreign.unit, "23503"},
		"foreign cost centre":       {"SUPERVISOR", "COST_CENTRE", foreign.centre, "23503"},
		"foreign team":              {"SUPERVISOR", "TEAM", foreign.team, "23503"},
		"foreign asset":             {"SUPERVISOR", "ASSET", foreign.active, "23503"},
		"cost centre named a unit":  {"SUPERVISOR", "UNIT", mine.centre, "23503"},
		"unit named a team":         {"SUPERVISOR", "TEAM", mine.unit, "23503"},
		"asset named a cost centre": {"SUPERVISOR", "COST_CENTRE", mine.active, "23503"},
		"unknown identifier":        {"SUPERVISOR", "ASSET", onboardingID(t), "23503"},
		"draft asset":               {"SUPERVISOR", "ASSET", mine.draft, "23514"},
		"archived asset":            {"SUPERVISOR", "ASSET", mine.archived, "23514"},
		"owner at a unit":           {"OWNER", "UNIT", mine.unit, "23514"},
		"station role at a unit":    {"STATION_MANAGER", "UNIT", mine.unit, "23514"},
		"organization as unit":      {"SUPERVISOR", "UNIT", org, "23514"},
		"own membership as team":    {"SUPERVISOR", "TEAM", member, "23514"},
		"station scope":             {"SUPERVISOR", "STATION", mine.unit, "23514"},
		"overlapping same target":   {"SUPERVISOR", "UNIT", mine.unit, "23505"},
	} {
		allocationWant(t, name, structuralGrantSQL(t, fx, onboardingID(t), member, tc.role, tc.kind, tc.target), tc.code)
	}
	allocationWant(t, "explicit generated reference", unitSchemaExec(t, fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason,scope_unit_id) VALUES($1,$2,$3,'AUDITOR','UNIT',$4,clock_timestamp(),$5,'Synthetic generated write',$4)`, org, onboardingID(t), member, mine.unit, fx.member), "428C9")
	allocationWant(t, "rewrite structural target", unitSchemaExec(t, fx, `UPDATE org_role_assignments SET scope_target=$3,revision=revision+1,ended_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, granted[mine.unit], mine.centre), "23514")
	// A different role at the same target is not an overlap.
	if err := structuralGrantSQL(t, fx, onboardingID(t), member, "AUDITOR", "UNIT", mine.unit); err != nil {
		t.Fatal(err)
	}
	// Archiving: assignments are rights, not use (BR-ORG-009); they are kept
	// and keep their target, and no new assignment can name it (BR-ORG-008).
	usage := func(fn, target string) string {
		t.Helper()
		var value string
		err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			return tx.QueryRow(t.Context(), `SELECT `+fn+`($1,$2)::text`, org, target).Scan(&value)
		})
		if err != nil {
			t.Fatal("usage source refused a structural grant", fn, err)
		}
		return value
	}
	if got := usage("org_cost_centre_current_usage", mine.centre); got != `{"assets": 0, "budgets": 0, "employees": 0, "operations": 0}` {
		t.Fatal("grant counted as cost-centre use", got)
	}
	if got := usage("org_team_current_usage", mine.team); got != `{"assets": 0, "budgets": 0, "employees": 0, "operations": 0}` {
		t.Fatal("grant counted as team use", got)
	}
	usage("org_asset_current_usage", mine.active)
	usage("org_unit_current_usage", mine.unit)
	west := allocationUnit(t, fx, "WEST")
	westGrant := onboardingID(t)
	if err := structuralGrantSQL(t, fx, westGrant, member, "FINANCE", "UNIT", west); err != nil {
		t.Fatal(err)
	}
	for _, archive := range []struct{ table, id string }{{"org_units", west}, {"org_cost_centres", mine.centre}, {"org_teams", mine.team}} {
		if err := unitSchemaExec(t, fx, `UPDATE `+archive.table+` SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, archive.id); err != nil {
			t.Fatal("structural grant blocked archive", archive.table, err)
		}
	}
	var kept int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND scope_target IN($2,$3,$4) AND ended_at IS NULL`, org, west, mine.centre, mine.team).Scan(&kept); err != nil || kept != 3 {
		t.Fatal("archive ended or rewrote structural grants", kept, err)
	}
	allocationWant(t, "archived unit", structuralGrantSQL(t, fx, onboardingID(t), member, "SUPERVISOR", "UNIT", west), "23514")
	allocationWant(t, "archived cost centre", structuralGrantSQL(t, fx, onboardingID(t), member, "FINANCE", "COST_CENTRE", mine.centre), "23514")
	allocationWant(t, "archived team", structuralGrantSQL(t, fx, onboardingID(t), member, "FINANCE", "TEAM", mine.team), "23514")
	// Ending never re-checks the target, even after it was archived.
	if err := unitSchemaExec(t, fx, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, org, westGrant); err != nil {
		t.Fatal("ending a grant on an archived unit refused", err)
	}
	// An asset with an ASSET grant still completes its lifecycle to archive.
	s := lifecycleStore(t, fx)
	var revision int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_assets WHERE org_id=$1 AND id=$2`, org, mine.disposed).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Archive, mine.disposed, revision))); err != nil {
		t.Fatal("asset grant blocked asset archive", err)
	}
	// Forced RLS: nothing visible or writable without the organization setting,
	// and the other organization never sees these grants.
	if err := pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE scope_type IN('UNIT','COST_CENTRE','TEAM','ASSET')`).Scan(&n); err != nil || n != 0 {
			t.Fatal("structural grants visible without organization setting", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "unscoped structural insert", pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'AUDITOR','UNIT',$4,clock_timestamp(),$5,'Synthetic unscoped write')`, org, onboardingID(t), member, mine.unit, fx.member)
		return err
	}), "42501")
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, other.scope, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1`, member).Scan(&n); err != nil || n != 0 {
			t.Fatal("foreign organization sees structural grants", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Admission, overlap and archive serialize on the organization root lock: a
// waiting grant observes the committed archive or overlap, never a stale one.
func TestStructuralScopeAdmissionSerializesWithArchiveAndOverlap(t *testing.T) {
	fx, _, _ := lifecycleFixture(t)
	org := fx.scope.OrgID().String()
	grantTeamTables(t, fx)
	member := structuralMemberIn(t, fx)
	pool := fx.f.RuntimePool(t, 2)
	insert := `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,'UNIT',$5,clock_timestamp(),$6,'Synthetic concurrent structural grant')`
	race := func(label string, holder, contender func(pgx.Tx) error) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		var holderPID int
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		if err := holder(tx); err != nil {
			t.Fatal(label, "holder write", err)
		}
		done := make(chan error, 1)
		go func() { done <- database.WithOrgTx(ctx, pool, fx.scope, contender) }()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for blocked := false; !blocked; {
			if err := fx.f.Admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND $2=ANY(pg_blocking_pids(pid)))`, fx.f.RuntimeRole, holderPID).Scan(&blocked); err != nil {
				t.Fatal(err)
			}
			if !blocked {
				select {
				case err := <-done:
					t.Fatal(label, "contender finished without waiting for the holder", err)
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal(label, "no lock wait observed")
				}
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(label, "holder commit", err)
		}
		code, _ := allocationCode(<-done)
		return code
	}
	grant := func(role, unit string) func(pgx.Tx) error {
		return func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), insert, org, onboardingID(t), member, role, unit, fx.member)
			return err
		}
	}
	archive := func(unit string) func(pgx.Tx) error {
		return func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, unit)
			return err
		}
	}
	north, south := allocationUnit(t, fx, "NORTH-RACE"), allocationUnit(t, fx, "SOUTH-RACE")
	if code := race("concurrent identical grants", grant("SUPERVISOR", north), grant("SUPERVISOR", north)); code != "23505" {
		t.Fatal("second identical concurrent grant not refused as overlap", code)
	}
	if code := race("archive first", archive(south), grant("FINANCE", south)); code != "23514" {
		t.Fatal("grant admitted a unit archived while it waited", code)
	}
	// Grant first: the archive waits and then succeeds; the grant survives.
	if code := race("grant first", grant("AUDITOR", north), archive(north)); code != "" {
		t.Fatal("archive refused after a structural grant", code)
	}
	var kept int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND scope_type='UNIT' AND scope_target=$2 AND ended_at IS NULL`, org, north).Scan(&kept); err != nil || kept != 2 {
		t.Fatal("concurrent outcome lost or duplicated grants", kept, err)
	}
	// A concurrent team archive is serialized the same way.
	team := structuralTeam(t, fx, teamStore(t, fx), "Synthetic race team")
	code := race("team archive first", func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_teams SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, team)
		return err
	}, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'SUPERVISOR','TEAM',$4,clock_timestamp(),$5,'Synthetic concurrent team grant')`, org, onboardingID(t), member, team, fx.member)
		return err
	})
	if code != "23514" {
		t.Fatal("team grant admitted an archived team", code)
	}
}
