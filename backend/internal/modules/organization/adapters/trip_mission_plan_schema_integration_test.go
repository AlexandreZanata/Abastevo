//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// planSchemaCode asserts the SQLSTATE and, when given, the guard message or
// constraint, so a refusal is attributed to the intended guard.
func planSchemaCode(t *testing.T, err error, code, message string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != code || message != "" && pe.Message != message && pe.ConstraintName != message {
		t.Fatalf("want %s %q, got %v", code, message, err)
	}
}

const insertPlanTransition = `INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,assignee_employee_id,assignee_membership_id)
 VALUES($1,$2,$3,'PLAN',$4,'PLANNED','COMPANY',$5,$6,$7,$8,$9)`

const insertPlanRow = `INSERT INTO org_trip_mission_plans(org_id,mission_id,revision,operation_id,destination_text,destination_unit_id,reason,cost_centre_id,load_kind,cargo_description,passenger_count,planned_by,planned_by_membership_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

// B-BR-EMP-386, EDR-248: forced RLS and an insert-only planning history; the
// database refuses a plan without its PLAN transition, of another kind, by
// another planner, with sensitive or unapproved content or an archived or
// foreign reference, a PLAN that keeps an assignee or leaves a non-open
// state, and a head that reaches PLANNED without its plan; the unit and
// cost-centre usage sources recognise the plan keys without counting them.
func TestTripMissionPlanSchemaIsForcedRLSGuardedAndAppendOnly(t *testing.T) {
	m := missionFixture(t)
	org := m.scope.OrgID().String()
	centre, archived := m.costCentre(t, "ACTIVE"), m.costCentre(t, "ARCHIVED")
	south := m.secondUnit(t)
	usage := func() string {
		t.Helper()
		tx, err := m.f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		var unit, destination, cost string
		if err := tx.QueryRow(t.Context(), `SELECT org_unit_current_usage($1,$2)::text,org_unit_current_usage($1,$3)::text,org_cost_centre_current_usage($1,$4)::text`, org, m.unit, south, centre).Scan(&unit, &destination, &cost); err != nil {
			t.Fatal("usage sources refused the plan keys", err)
		}
		return unit + destination + cost
	}
	beforeUsage := usage()
	mission := m.create(t, m.unit, "").MissionID
	unitPlan := cargoPlan(centre)
	unitPlan.DestinationText, unitPlan.DestinationUnitID = nil, south
	if _, err := m.plan(t, mission, 1, unitPlan); err != nil {
		t.Fatal(err)
	}
	if after := usage(); after != beforeUsage {
		t.Fatal("a plan changed a unit or cost-centre usage answer", beforeUsage, after)
	}
	var forced bool
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_trip_mission_plans'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("planning revisions without forced RLS", err)
	}
	var visible int
	if err := m.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_mission_plans`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("plans visible without the organization setting", visible, err)
	}
	other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
	if err := database.WithOrgTx(t.Context(), m.f.Runtime, other.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_mission_plans`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("another organization sees plans", visible, err)
	}
	if _, err := m.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_mission_plans TO "+pgx.Identifier{m.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	// EMP-TRIP-010: the planned stops (000256) reference the planning
	// revisions, so the truncation cascades to them and must reach both
	// append-only guards (the EMP-TRIP-019 checklist precedent).
	if _, err := m.f.Admin.Exec(t.Context(), "GRANT TRUNCATE ON org_trip_mission_plan_stops TO "+pgx.Identifier{m.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	for name, query := range map[string]string{
		"plan-update":   `UPDATE org_trip_mission_plans SET reason='Reescrito' WHERE org_id=$1`,
		"plan-delete":   `DELETE FROM org_trip_mission_plans WHERE org_id=$1`,
		"plan-truncate": `TRUNCATE org_trip_mission_plans CASCADE`,
	} {
		t.Run(name, func(t *testing.T) {
			args := []any{org}
			if name == "plan-truncate" {
				args = nil
			}
			planSchemaCode(t, unitSchemaExec(t, m.legalFixtureState, query, args...), "55000", "")
		})
	}

	driver := m.driver(t)
	if _, err := m.assignTo(t, mission, 2, driver.person.employee); err != nil {
		t.Fatal(err)
	}
	var assignOp string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT operation_id::text FROM org_trip_mission_transitions WHERE org_id=$1 AND mission_id=$2 AND revision=3`, org, mission).Scan(&assignOp); err != nil {
		t.Fatal(err)
	}
	// One transaction per case: the PLAN transition of revision 4 (unless
	// skipped) and then the planning row; every case rolls back.
	type planCase struct {
		transition bool
		assignee   bool
		edit       func(args []any)
		code, msg  string
	}
	row := func(op string) []any {
		return []any{org, mission, int64(4), op, "Centro de distribuição Norte", nil, "Entrega de peças", centre, "CARGO", "Peças", nil, m.actor.AccountID, m.member}
	}
	for name, c := range map[string]planCase{
		"plan-without-transition":    {false, false, nil, "23514", "mission plan source unavailable"},
		"plan-of-assign-transition":  {false, false, func(a []any) { a[2], a[3] = int64(3), assignOp }, "23514", "mission plan source unavailable"},
		"plan-by-another-planner":    {true, false, func(a []any) { a[11], a[12] = driver.person.auth.Account.ID, driver.person.member }, "23514", "mission plan source unavailable"},
		"cpf-in-destination":         {true, false, func(a []any) { a[4] = "Casa do cliente 12345678909" }, "23514", "org_trip_mission_plans_destination_text_check"},
		"formatted-cpf-in-reason":    {true, false, func(a []any) { a[6] = "Cliente 123.456.789-09" }, "23514", "org_trip_mission_plans_reason_check"},
		"cpf-in-cargo":               {true, false, func(a []any) { a[9] = "Pacote 12345678909" }, "23514", "org_trip_mission_plans_cargo_description_check"},
		"passenger-description":      {true, false, func(a []any) { a[8], a[10] = "PASSENGERS", 3 }, "23514", "org_trip_mission_plans_shape"},
		"cargo-with-count":           {true, false, func(a []any) { a[10] = 2 }, "23514", "org_trip_mission_plans_shape"},
		"passengers-above-bound":     {true, false, func(a []any) { a[8], a[9], a[10] = "PASSENGERS", nil, 1000 }, "23514", "org_trip_mission_plans_passenger_count_check"},
		"both-destinations":          {true, false, func(a []any) { a[5] = south }, "23514", "org_trip_mission_plans_shape"},
		"archived-cost-centre":       {true, false, func(a []any) { a[7] = archived }, "23514", "mission plan cost centre unavailable"},
		"unknown-cost-centre":        {true, false, func(a []any) { a[7] = onboardingID(t) }, "23514", "mission plan cost centre unavailable"},
		"plan-transition-assignee":   {true, true, nil, "23514", "mission transition unavailable"},
		"plan-transition-no-receipt": {true, false, func(a []any) { a[0] = nil }, "23514", "mission publication unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			args := row(onboardingID(t))
			if c.edit != nil {
				c.edit(args)
			}
			commitOnly := args[0] == nil
			args[0] = org
			err := database.WithOrgTx(t.Context(), m.f.Runtime, m.scope, func(tx pgx.Tx) error {
				if c.transition {
					employee, member := any(nil), any(nil)
					if c.assignee {
						employee, member = driver.person.employee, driver.person.member
					}
					if _, err := tx.Exec(t.Context(), insertPlanTransition, org, mission, int64(4), "ASSIGNED", m.actor.AccountID, m.member, args[3], employee, member); err != nil {
						return err
					}
				}
				if commitOnly {
					return nil // the deferred check refuses a PLAN without its receipt and plan
				}
				_, err := tx.Exec(t.Context(), insertPlanRow, args...)
				if err == nil {
					err = errors.New("the plan row was admitted")
				}
				return err
			})
			planSchemaCode(t, err, c.code, c.msg)
		})
	}
	t.Run("head-planned-without-its-plan", func(t *testing.T) {
		op := onboardingID(t)
		err := database.WithOrgTx(t.Context(), m.f.Runtime, m.scope, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), insertPlanTransition, org, mission, int64(4), "ASSIGNED", m.actor.AccountID, m.member, op, nil, nil); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `UPDATE org_trip_missions SET state='PLANNED',revision=revision+1,assignee_employee_id=NULL,assignee_membership_id=NULL WHERE org_id=$1 AND id=$2`, org, mission)
			return err
		})
		planSchemaCode(t, err, "55000", "mission history is append-only")
	})
	t.Run("plan-from-a-terminal-state", func(t *testing.T) {
		cancelled := m.create(t, m.unit, "").MissionID
		if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripappCancel(cancelled, 1))); err != nil {
			t.Fatal(err)
		}
		err := unitSchemaExec(t, m.legalFixtureState, insertPlanTransition, org, cancelled, int64(3), "CANCELLED", m.actor.AccountID, m.member, onboardingID(t), nil, nil)
		planSchemaCode(t, err, "23514", "mission transition unavailable")
	})
	t.Run("sql-table", func(t *testing.T) {
		var table string
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT string_agg(s||'>'||COALESCE(org_trip_mission_next(s,'PLAN'),'-'),',' ORDER BY s) FROM unnest(ARRAY['DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED','IN_PROGRESS','COMPLETED','CANCELLED']) s`).Scan(&table); err != nil ||
			table != "ACCEPTED>-,ASSIGNED>PLANNED,CANCELLED>-,COMPLETED>-,DECLINED>PLANNED,DRAFT>PLANNED,IN_PROGRESS>-,PLANNED>PLANNED" {
			t.Fatal("the SQL PLAN rows", table, err)
		}
	})
	t.Run("usage-sources-recognise-the-keys", func(t *testing.T) {
		var defined bool
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT position('org_trip_mission_plans_destination_unit_fkey' IN pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_trip_missions_unit_fkey' IN pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_trip_mission_plans_cost_centre_fkey' IN pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_trips_cost_centre_fkey' IN pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure))>0`).Scan(&defined); err != nil || !defined {
			t.Fatal("usage allow-lists lost a line or miss a plan key", err)
		}
		// Associations, not use: the planned destination unit and cost centre
		// can still be archived and the history keeps them.
		if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_cost_centres SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, centre); err != nil {
			t.Fatal("a planned cost centre cannot be archived", err)
		}
		if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, south); err != nil {
			t.Fatal("a planned destination unit cannot be archived", err)
		}
		var kept int
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_mission_plans WHERE org_id=$1 AND destination_unit_id=$2 AND cost_centre_id=$3`, org, south, centre).Scan(&kept); err != nil || kept != 1 {
			t.Fatal("archiving rewrote the planning history", kept, err)
		}
	})
}
