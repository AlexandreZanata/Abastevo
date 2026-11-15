//go:build integration

package adapters_test

import (
	"context"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// B-BR-EMP-358, EDR-230: forced RLS on the three mission tables, an
// insert-only history and receipts, a head that moves only with the
// transition of its new revision, the creation and transition guards, and
// unit and asset usage sources that recognise the mission keys without
// counting them.
func TestTripMissionSchemaIsForcedRLSGuardedAndAppendOnly(t *testing.T) {
	m := missionFixture(t)
	org := m.scope.OrgID().String()
	driver := m.driver(t)
	mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.assignTo(t, mission, 1, driver.person.employee); err != nil {
		t.Fatal(err)
	}
	var forced bool
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity) AND count(*)=3 FROM pg_class WHERE oid=ANY(ARRAY['org_trip_missions'::regclass,'org_trip_mission_transitions'::regclass,'org_trip_mission_commands'::regclass])`).Scan(&forced); err != nil || !forced {
		t.Fatal("mission tables without forced RLS", err)
	}
	var visible int
	if err := m.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_missions`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("rows visible without the organization setting", visible, err)
	}
	other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
	if err := database.WithOrgTx(t.Context(), m.f.Runtime, other.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_missions`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("another organization sees missions", visible, err)
	}
	// The guards, not missing privileges, must refuse every rewrite.
	if _, err := m.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_missions,org_trip_mission_transitions,org_trip_mission_commands TO "+pgx.Identifier{m.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	refused := func(name, want, query string, args ...any) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			unitSchemaCode(t, unitSchemaExec(t, m.legalFixtureState, query, args...), want)
		})
	}
	refused("head-update-without-transition", "55000", `UPDATE org_trip_missions SET state='CANCELLED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, mission)
	refused("head-content-change", "55000", `UPDATE org_trip_missions SET title='Outro título',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, mission)
	refused("head-delete", "55000", `DELETE FROM org_trip_missions WHERE org_id=$1 AND id=$2`, org, mission)
	refused("transition-update", "55000", `UPDATE org_trip_mission_transitions SET reason='Reescrito' WHERE org_id=$1 AND mission_id=$2`, org, mission)
	refused("transition-delete", "55000", `DELETE FROM org_trip_mission_transitions WHERE org_id=$1 AND mission_id=$2`, org, mission)
	refused("receipt-update", "55000", `UPDATE org_trip_mission_commands SET result_state='DRAFT' WHERE org_id=$1 AND mission_id=$2`, org, mission)
	refused("receipt-truncate", "55000", `TRUNCATE org_trip_mission_commands`)
	refused("head-born-assigned", "23514", `INSERT INTO org_trip_missions(org_id,id,unit_id,title,starts_at,ends_at,state,revision,created_by,created_by_membership_id,create_operation_id)
 VALUES($1,$2,$3,'Missão',clock_timestamp(),clock_timestamp()+interval '1 hour','DRAFT',2,$4,$5,$6)`, org, onboardingID(t), m.unit, m.actor.AccountID, m.member, onboardingID(t))
	refused("head-on-implement", "23514", `INSERT INTO org_trip_missions(org_id,id,unit_id,asset_id,title,starts_at,ends_at,state,revision,created_by,created_by_membership_id,create_operation_id)
 VALUES($1,$2,$3,$4,'Missão',clock_timestamp(),clock_timestamp()+interval '1 hour','DRAFT',1,$5,$6,$7)`, org, onboardingID(t), m.unit, m.installed.AssetID, m.actor.AccountID, m.member, onboardingID(t))
	refused("head-without-create-transition", "23514", `INSERT INTO org_trip_missions(org_id,id,unit_id,title,starts_at,ends_at,state,revision,created_by,created_by_membership_id,create_operation_id)
 VALUES($1,$2,$3,'Missão',clock_timestamp(),clock_timestamp()+interval '1 hour','DRAFT',1,$4,$5,$6)`, org, onboardingID(t), m.unit, m.actor.AccountID, m.member, onboardingID(t))
	refused("window-reversed", "23514", `INSERT INTO org_trip_missions(org_id,id,unit_id,title,starts_at,ends_at,state,revision,created_by,created_by_membership_id,create_operation_id)
 VALUES($1,$2,$3,'Missão',clock_timestamp(),clock_timestamp()-interval '1 hour','DRAFT',1,$4,$5,$6)`, org, onboardingID(t), m.unit, m.actor.AccountID, m.member, onboardingID(t))
	refused("transition-skips-revision", "23514", `INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,reason)
 VALUES($1,$2,4,'CANCEL','ASSIGNED','CANCELLED','COMPANY',$3,$4,$5,'Pulo')`, org, mission, m.actor.AccountID, m.member, onboardingID(t))
	refused("response-without-signed-operation", "23514", `INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,enrollment_id,assignee_employee_id,assignee_membership_id)
 VALUES($1,$2,3,'ACCEPT','ASSIGNED','ACCEPTED','DEVICE',$3,$4,$5,$6,$7,$4)`, org, mission, driver.person.auth.Account.ID, driver.person.member, onboardingID(t), driver.source.EnrollmentID, driver.person.employee)
	refused("company-response", "23514", `INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,assignee_employee_id,assignee_membership_id)
 VALUES($1,$2,3,'ACCEPT','ASSIGNED','ACCEPTED','COMPANY',$3,$4,$5,$6,$7)`, org, mission, m.actor.AccountID, m.member, onboardingID(t), driver.person.employee, driver.person.member)
	refused("reassign-same-member", "23514", `INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,assignee_employee_id,assignee_membership_id)
 VALUES($1,$2,3,'ASSIGN','ASSIGNED','ASSIGNED','COMPANY',$3,$4,$5,$6,$7)`, org, mission, m.actor.AccountID, m.member, onboardingID(t), driver.person.employee, driver.person.member)
	refused("cross-organization-head", "42501", `INSERT INTO org_trip_missions(org_id,id,unit_id,title,starts_at,ends_at,state,revision,created_by,created_by_membership_id,create_operation_id)
 VALUES($1,$2,$3,'Missão',clock_timestamp(),clock_timestamp()+interval '1 hour','DRAFT',1,$4,$5,$6)`, other.scope.OrgID().String(), onboardingID(t), m.unit, m.actor.AccountID, m.member, onboardingID(t))
	t.Run("usage-sources-recognise-the-keys", func(t *testing.T) {
		tx, err := m.f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		var asset, unit string
		if err := tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text,org_unit_current_usage($1,$3)::text`, org, m.vehicle.AssetID, m.unit).Scan(&asset, &unit); err != nil {
			t.Fatal("usage sources refused the mission keys", err)
		}
		var defined bool
		if err := tx.QueryRow(t.Context(), `SELECT position('org_trip_missions_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_trip_missions_unit_fkey' IN pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_checklist_template_targets_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_trips_unit_fkey' IN pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure))>0`).Scan(&defined); err != nil || !defined {
			t.Fatal("usage allow-lists lost a line", err)
		}
	})
}
