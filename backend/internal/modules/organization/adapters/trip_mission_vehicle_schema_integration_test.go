//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

const insertVehicleTransition = `INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,assignee_employee_id,assignee_membership_id)
 VALUES($1,$2,$3,'VEHICLE',$4,$5,'COMPANY',$6,$7,$8,$9,$10)`

const insertVehicleRow = `INSERT INTO org_trip_mission_vehicles(org_id,mission_id,revision,operation_id,previous_asset_id,asset_id,reason,changed_by,changed_by_membership_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`

// B-BR-EMP-388/389, EDR-251: forced RLS and insert-only change records; the
// database refuses a record without its VEHICLE transition, by another
// planner, whose previous vehicle is not the mission's, naming an
// unavailable or busy vehicle or the same one, a change while the assignee
// runs a trip on the previous vehicle, a VEHICLE transition that moves the
// state or reaches a terminal mission, a head whose vehicle moves without its
// record, and moving a reading or a trip to another asset; the asset usage
// source recognises both keys without counting them.
func TestTripMissionVehicleSchemaIsForcedRLSGuardedAndAppendOnly(t *testing.T) {
	m := missionFixture(t)
	org := m.scope.OrgID().String()
	d := m.syncFixtureState
	free, spare := m.otherVehicle(t, ""), m.otherVehicle(t, "")
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
		var first, second string
		if err := tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text,org_asset_current_usage($1,$3)::text`, org, m.vehicle.AssetID, free.AssetID).Scan(&first, &second); err != nil {
			t.Fatal("asset usage refused the change keys", err)
		}
		return first + second
	}
	changed := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.plan(t, changed, 1, cargoPlan("")); err != nil {
		t.Fatal(err)
	}
	before := usage()
	if _, err := m.changeVehicle(t, changed, 2, free.AssetID); err != nil {
		t.Fatal(err)
	}
	if usage() != before {
		t.Fatal("a vehicle change changed an asset usage answer")
	}
	var forced bool
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_trip_mission_vehicles'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("change records without forced RLS", err)
	}
	var visible int
	if err := m.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_mission_vehicles`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("change records visible without the organization setting", visible, err)
	}
	other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
	if err := database.WithOrgTx(t.Context(), m.f.Runtime, other.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_mission_vehicles`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("another organization sees change records", visible, err)
	}
	if _, err := m.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_mission_vehicles TO "+pgx.Identifier{m.f.RuntimeRole}.Sanitize()+";GRANT UPDATE ON org_meter_readings,org_trips TO "+pgx.Identifier{m.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	for name, query := range map[string]string{
		"record-update":   `UPDATE org_trip_mission_vehicles SET reason='Reescrito' WHERE org_id=$1`,
		"record-delete":   `DELETE FROM org_trip_mission_vehicles WHERE org_id=$1`,
		"record-truncate": `TRUNCATE org_trip_mission_vehicles`,
	} {
		t.Run(name, func(t *testing.T) {
			args := []any{org}
			if name == "record-truncate" {
				args = nil
			}
			planSchemaCode(t, unitSchemaExec(t, m.legalFixtureState, query, args...), "55000", "")
		})
	}

	// The assignee d runs a trip on the first vehicle.
	assigned := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.assignTo(t, assigned, 1, m.person.employee); err != nil {
		t.Fatal(err)
	}
	running := m.startOp(t, d, m.declaredNow(t), nil)
	ack, err := m.process(t, d, running)
	m.accepted(t, running, ack, err, 1, 1)
	onSpare := m.create(t, m.unit, spare.AssetID).MissionID
	if _, err := m.plan(t, onSpare, 1, cargoPlan("")); err != nil {
		t.Fatal(err)
	}
	draft := m.create(t, m.unit, spare.AssetID).MissionID
	type vehicleCase struct {
		mission, state, previous, asset string
		revision                        int64
		assignee                        bool
		transition, record              bool
		edit                            func(record []any)
		code, msg                       string
	}
	for name, c := range map[string]vehicleCase{
		"record-without-transition": {onSpare, "PLANNED", spare.AssetID, free.AssetID, 3, false, false, true, nil, "23514", "mission vehicle source unavailable"},
		"previous-not-current":      {onSpare, "PLANNED", m.vehicle.AssetID, free.AssetID, 3, false, true, true, nil, "23514", "mission vehicle source unavailable"},
		"by-another-planner":        {onSpare, "PLANNED", spare.AssetID, free.AssetID, 3, false, true, true, func(r []any) { r[7], r[8] = m.person.auth.Account.ID, m.person.member }, "23514", "mission vehicle source unavailable"},
		"implement":                 {onSpare, "PLANNED", spare.AssetID, m.installed.AssetID, 3, false, true, true, nil, "23514", "mission vehicle unavailable"},
		"draft-asset":               {onSpare, "PLANNED", spare.AssetID, m.draftAsset(t), 3, false, true, true, nil, "23514", "mission vehicle unavailable"},
		"busy-vehicle":              {onSpare, "PLANNED", spare.AssetID, m.vehicle.AssetID, 3, false, true, true, nil, "23514", "mission vehicle in use"},
		"same-vehicle":              {onSpare, "PLANNED", spare.AssetID, spare.AssetID, 3, false, true, true, nil, "23514", "org_trip_mission_vehicles_change"},
		"assignee-runs-a-trip":      {assigned, "ASSIGNED", m.vehicle.AssetID, spare.AssetID, 3, true, true, true, nil, "23514", "mission trip in progress"},
		"transition-without-record": {onSpare, "PLANNED", spare.AssetID, free.AssetID, 3, false, true, false, nil, "23514", "mission publication unavailable"},
		"vehicle-on-a-draft":        {draft, "DRAFT", spare.AssetID, free.AssetID, 2, false, true, true, nil, "23514", "mission transition unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			op := onboardingID(t)
			record := []any{org, c.mission, c.revision, op, c.previous, c.asset, "Troca", m.actor.AccountID, m.member}
			if c.edit != nil {
				c.edit(record)
			}
			err := database.WithOrgTx(t.Context(), m.f.Runtime, m.scope, func(tx pgx.Tx) error {
				if c.transition {
					employee, member := any(nil), any(nil)
					if c.assignee {
						employee, member = m.person.employee, m.person.member
					}
					if _, err := tx.Exec(t.Context(), insertVehicleTransition, org, c.mission, c.revision, c.state, c.state, m.actor.AccountID, m.member, op, employee, member); err != nil {
						return err
					}
				}
				if !c.record {
					return nil
				}
				_, err := tx.Exec(t.Context(), insertVehicleRow, record...)
				if err == nil {
					err = errors.New("the change record was admitted")
				}
				return err
			})
			planSchemaCode(t, err, c.code, c.msg)
		})
	}
	t.Run("head-vehicle-without-record", func(t *testing.T) {
		err := database.WithOrgTx(t.Context(), m.f.Runtime, m.scope, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), insertVehicleTransition, org, onSpare, int64(3), "PLANNED", "PLANNED", m.actor.AccountID, m.member, onboardingID(t), nil, nil); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `UPDATE org_trip_missions SET revision=revision+1,asset_id=$3 WHERE org_id=$1 AND id=$2`, org, onSpare, free.AssetID)
			return err
		})
		planSchemaCode(t, err, "55000", "mission history is append-only")
	})
	t.Run("vehicle-transition-moves-state", func(t *testing.T) {
		err := unitSchemaExec(t, m.legalFixtureState, insertVehicleTransition, org, assigned, int64(3), "ASSIGNED", "ACCEPTED", m.actor.AccountID, m.member, onboardingID(t), m.person.employee, m.person.member)
		planSchemaCode(t, err, "23514", "mission transition unavailable")
	})
	t.Run("vehicle-on-a-terminal-mission", func(t *testing.T) {
		cancelled := m.create(t, m.unit, spare.AssetID).MissionID
		if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripappCancel(cancelled, 1))); err != nil {
			t.Fatal(err)
		}
		err := unitSchemaExec(t, m.legalFixtureState, insertVehicleTransition, org, cancelled, int64(3), "CANCELLED", "CANCELLED", m.actor.AccountID, m.member, onboardingID(t), nil, nil)
		planSchemaCode(t, err, "23514", "mission transition unavailable")
	})
	t.Run("reading-or-trip-moved-to-another-asset", func(t *testing.T) {
		var readings int
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2 AND meter_id=$3`, org, running.ID(), m.vehicle.MeterID).Scan(&readings); err != nil || readings != 1 {
			t.Fatal("the running trip's reading is not where the test moves it from", readings, err)
		}
		planSchemaCode(t, unitSchemaExec(t, m.legalFixtureState, `UPDATE org_meter_readings SET meter_id=$2,segment_id=$3 WHERE org_id=$1 AND operation_id=$4`, org, spare.MeterID, spare.SegmentID, running.ID()), "55000", "")
		planSchemaCode(t, unitSchemaExec(t, m.legalFixtureState, `UPDATE org_trips SET asset_id=$2 WHERE org_id=$1 AND id=$3`, org, spare.AssetID, tripOf(running)), "55000", "")
	})
	t.Run("sql-table", func(t *testing.T) {
		var table string
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT string_agg(s||'>'||COALESCE(org_trip_mission_next(s,'VEHICLE'),'-'),',' ORDER BY s) FROM unnest(ARRAY['DRAFT','PLANNED','ASSIGNED','ACCEPTED','DECLINED','IN_PROGRESS','COMPLETED','CANCELLED']) s`).Scan(&table); err != nil ||
			table != "ACCEPTED>ACCEPTED,ASSIGNED>ASSIGNED,CANCELLED>-,COMPLETED>-,DECLINED>DECLINED,DRAFT>-,IN_PROGRESS>-,PLANNED>PLANNED" {
			t.Fatal("the SQL VEHICLE rows", table, err)
		}
	})
	t.Run("usage-source-recognises-the-keys", func(t *testing.T) {
		var defined bool
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT position('org_trip_mission_vehicles_previous_fkey' IN a)>0 AND position('org_trip_mission_vehicles_asset_fkey' IN a)>0
 AND position('org_trip_missions_asset_fkey' IN a)>0 AND position('org_checklist_template_targets_asset_fkey' IN a)>0
 FROM(SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) AS a) s`).Scan(&defined); err != nil || !defined {
			t.Fatal("the asset allow-list lost a line or misses a change key", err)
		}
	})
}
