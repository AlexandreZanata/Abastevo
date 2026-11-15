//go:build integration

package adapters_test

import (
	"testing"
	"time"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// AC5 and B-BR-EMP-245: the actual offboarding that terminates an employee
// record and the actual disposal end every assignment that is not ENDED, in
// the source transaction, with the cause and source revision recorded.
func TestFleetAssignmentsEndWithEmployeeTerminationAndAssetDisposal(t *testing.T) {
	fx := membershipAccessFixture(t)
	lf := fx.legalFixtureState
	s := assignmentStore(t, lf)
	offboard := offboardingStore(t, fx)
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	employeeOf := func(member string) string {
		t.Helper()
		var employee string
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, org, member).Scan(&employee); err != nil {
			t.Fatal(err)
		}
		return employee
	}
	leaver, kept := accessMember(t, fx), accessMember(t, fx)
	leaverEmployee, keptEmployee := employeeOf(leaver), employeeOf(kept)
	colleague, _ := assignmentPerson(t, lf)
	a, _ := lifecycleAssetIn(t, lf, s, unit, fleet.Active)
	b, _ := lifecycleAssetIn(t, lf, s, unit, fleet.Active)
	c, _ := lifecycleAssetIn(t, lf, s, unit, fleet.Inactive)
	create := func(v fa.AssignmentCreateInput) fa.AssignmentResult {
		t.Helper()
		r, _, err := assignmentWrite(t, lf, s, assignmentCreate(t, v))
		if err != nil {
			t.Fatal("fixture assignment", err)
		}
		return r
	}
	now := lifecycleDBNow(t, lf).Truncate(time.Microsecond)
	soon, later := now.Add(time.Hour), now.Add(2*time.Hour)
	active := create(fa.AssignmentCreateInput{AssetID: a, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{leaverEmployee}})
	scheduled := create(fa.AssignmentCreateInput{AssetID: b, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{leaverEmployee}, Start: &soon})
	idle := create(fa.AssignmentCreateInput{AssetID: c, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{leaverEmployee}})
	// A primary replaced at a later start is not ENDED yet; termination ends
	// it earlier with the system cause.
	successor := create(fa.AssignmentCreateInput{AssetID: c, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{colleague}, Start: &later, ReplacePrimary: true})
	untouched := create(fa.AssignmentCreateInput{AssetID: a, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{colleague, keptEmployee}})

	// Retaining the employee record ends the membership only: assignments stay
	// and the ended membership is no longer an assignee.
	retain := offboardingCommand(t, offboardingRead(t, fx, offboard, kept))
	if _, err := offboard.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, retain); err != nil {
		t.Fatal("offboarding that retains the employee", err)
	}
	if rows := assignmentRowsOf(t, lf, a); rows[untouched.AssignmentIDs[1]].to != nil || rows[untouched.AssignmentIDs[1]].cause != "" {
		t.Fatal("retained employee lost an assignment")
	}
	if assignedAt(t, lf, s, kept, a, lifecycleDBNow(t, lf)) {
		t.Fatal("ended membership listed as assignee")
	}

	terminate := offboardingCommand(t, offboardingRead(t, fx, offboard, leaver))
	terminate.TerminateEmployee = true
	before := lifecycleDBNow(t, lf)
	out, err := offboard.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, terminate)
	after := lifecycleDBNow(t, lf)
	if err != nil || out.View.EmployeeState != d.EmployeeTerminated {
		t.Fatal("actual offboarding with termination", err)
	}
	var revision int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_employee_identities WHERE org_id=$1 AND id=$2`, org, leaverEmployee).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	terminated := func(row assignmentRow, empty bool) {
		t.Helper()
		if row.cause != "EMPLOYEE_TERMINATED" || row.source == nil || *row.source != revision || row.endOperation != "" || row.to == nil || row.endedAt == nil {
			t.Fatal("termination cause not recorded", row)
		}
		if empty && !row.to.Equal(row.from) || !empty && (row.to.Before(before.Truncate(time.Microsecond)) || row.to.After(after)) {
			t.Fatal("termination instant", row)
		}
	}
	terminated(assignmentRowsOf(t, lf, a)[active.AssignmentIDs[0]], false)
	terminated(assignmentRowsOf(t, lf, b)[scheduled.AssignmentIDs[0]], true)
	terminated(assignmentRowsOf(t, lf, c)[idle.AssignmentIDs[0]], false)
	if rows := assignmentRowsOf(t, lf, c); rows[successor.AssignmentIDs[0]].cause != "" || !rows[successor.AssignmentIDs[0]].from.Equal(later) {
		t.Fatal("successor primary changed by the termination", rows[successor.AssignmentIDs[0]])
	}
	for _, id := range untouched.AssignmentIDs {
		if row := assignmentRowsOf(t, lf, a)[id]; row.to != nil || row.cause != "" {
			t.Fatal("another employee's assignment ended", row)
		}
	}
	// Rehire never restores the ended assignments.
	if err := allocationExec(t, lf, `UPDATE org_employee_identities SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, leaverEmployee); err != nil {
		t.Fatal(err)
	}
	if row := assignmentRowsOf(t, lf, a)[active.AssignmentIDs[0]]; row.cause != "EMPLOYEE_TERMINATED" {
		t.Fatal("rehire restored an assignment", row)
	}

	// Disposal at the server instant: the current assignments end exactly at
	// the disposal instant (microsecond floor), a scheduled one is emptied.
	d1, revision1 := lifecycleAssetIn(t, lf, s, unit, fleet.Active)
	current := create(fa.AssignmentCreateInput{AssetID: d1, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{colleague}})
	future := create(fa.AssignmentCreateInput{AssetID: d1, Kind: fleet.TemporaryAssignment, EmployeeIDs: []string{colleague}, Start: &soon, End: &later})
	dispose := lifecycleInputFor(t, lf, fleet.Dispose, d1, revision1)
	dispose.At = lifecycleDBNow(t, lf).Add(-500 * time.Nanosecond)
	disposed, _, err := lifecycleWrite(t, lf, s, lifecycleCommand(t, dispose))
	if err != nil {
		t.Fatal("actual disposal", err)
	}
	rows := assignmentRowsOf(t, lf, d1)
	floor := dispose.At.UTC().Truncate(time.Microsecond)
	for id, row := range map[string]assignmentRow{current.AssignmentIDs[0]: rows[current.AssignmentIDs[0]], future.AssignmentIDs[0]: rows[future.AssignmentIDs[0]]} {
		if row.cause != "ASSET_DISPOSED" || row.source == nil || *row.source != disposed.Revision || row.endOperation != "" || row.endedAt == nil {
			t.Fatal("disposal cause not recorded", id, row)
		}
	}
	if !rows[current.AssignmentIDs[0]].to.Equal(floor) || !rows[future.AssignmentIDs[0]].to.Equal(soon) {
		t.Fatal("disposal instant", rows)
	}
	// Reverting the disposal restores nothing; a new assignment needs a new command.
	revert := lifecycleInputFor(t, lf, fleet.RevertDisposal, d1, disposed.Revision)
	if _, _, err := lifecycleWrite(t, lf, s, lifecycleCommand(t, revert)); err != nil {
		t.Fatal("revert disposal", err)
	}
	if again := assignmentRowsOf(t, lf, d1); again[current.AssignmentIDs[0]].cause != "ASSET_DISPOSED" || !again[current.AssignmentIDs[0]].to.Equal(floor) {
		t.Fatal("revert restored an assignment")
	}
	if _, _, err := assignmentWrite(t, lf, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: d1, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{colleague}})); err != nil {
		t.Fatal("new assignment after revert", err)
	}
	// Deactivation keeps assignments (fleet.md §6.1).
	d2, revision2 := lifecycleAssetIn(t, lf, s, unit, fleet.Active)
	kept2 := create(fa.AssignmentCreateInput{AssetID: d2, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{colleague}})
	if _, _, err := lifecycleWrite(t, lf, s, lifecycleCommand(t, lifecycleInputFor(t, lf, fleet.Deactivate, d2, revision2))); err != nil {
		t.Fatal(err)
	}
	if row := assignmentRowsOf(t, lf, d2)[kept2.AssignmentIDs[0]]; row.to != nil || row.cause != "" {
		t.Fatal("deactivation ended an assignment", row)
	}
}

// Schema guards (B-BR-EMP-243/244): forced RLS, append-only rows, ending only
// with a proven cause, the exclusion constraint itself and the usage source.
func TestFleetAssignmentSchemaGuardsAndIsolation(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := assignmentStore(t, fx)
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	employee, _ := assignmentPerson(t, fx)
	org := fx.scope.OrgID().String()
	created, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{employee}}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.AssignmentIDs[0]
	var operation string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT operation_id::text FROM org_asset_assignments WHERE org_id=$1 AND id=$2`, org, id).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) error { return allocationExec(t, fx, query, args...) }
	// DELETE is granted here only to prove the statement guard refuses it.
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT DELETE ON org_asset_assignments TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "delete", exec(`DELETE FROM org_asset_assignments WHERE org_id=$1 AND id=$2`, org, id), "55000")
	allocationWant(t, "change employee", exec(`UPDATE org_asset_assignments SET employee_id=$3 WHERE org_id=$1 AND id=$2`, org, id, onboardingID(t)), "55000")
	allocationWant(t, "change kind", exec(`UPDATE org_asset_assignments SET kind='PRIMARY' WHERE org_id=$1 AND id=$2`, org, id), "55000")
	allocationWant(t, "end without cause", exec(`UPDATE org_asset_assignments SET valid_to=clock_timestamp() WHERE org_id=$1 AND id=$2`, org, id), "55000")
	allocationWant(t, "forged termination", exec(`UPDATE org_asset_assignments SET valid_to=clock_timestamp(),end_cause='EMPLOYEE_TERMINATED',source_revision=1 WHERE org_id=$1 AND id=$2`, org, id), "23514")
	allocationWant(t, "forged disposal", exec(`UPDATE org_asset_assignments SET valid_to=clock_timestamp(),end_cause='ASSET_DISPOSED',source_revision=2 WHERE org_id=$1 AND id=$2`, org, id), "23514")
	allocationWant(t, "end erasing started history", exec(`UPDATE org_asset_assignments SET valid_to=valid_from,end_cause='ENDED',end_operation_id=$3,ended_by=$4 WHERE org_id=$1 AND id=$2`, org, id, operation, fx.actor.AccountID), "23514")
	planned := lifecycleDBNow(t, fx).Truncate(time.Microsecond).Add(time.Hour)
	temporary, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.TemporaryAssignment, EmployeeIDs: []string{employee}, End: &planned}))
	if err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "end later than planned", exec(`UPDATE org_asset_assignments SET valid_to=valid_to+interval '1 hour',end_cause='ENDED',end_operation_id=$3,ended_by=$4 WHERE org_id=$1 AND id=$2`, org, temporary.AssignmentIDs[0], operation, fx.actor.AccountID), "55000")
	allocationWant(t, "replaced without successor", exec(`UPDATE org_asset_assignments SET valid_to=clock_timestamp()+interval '1 hour',end_cause='REPLACED',end_operation_id=$3,ended_by=$4 WHERE org_id=$1 AND id=$2`, org, id, operation, fx.actor.AccountID), "23514")
	allocationWant(t, "temporary without end", exec(`INSERT INTO org_asset_assignments(org_id,id,asset_id,employee_id,kind,valid_from,operation_id,assigned_by) VALUES($1,$2,$3,$4,'TEMPORARY',clock_timestamp(),$5,$6)`, org, onboardingID(t), asset, employee, operation, fx.actor.AccountID), "23514")
	allocationWant(t, "past start", exec(`INSERT INTO org_asset_assignments(org_id,id,asset_id,employee_id,kind,valid_from,operation_id,assigned_by) VALUES($1,$2,$3,$4,'ADDITIONAL',clock_timestamp()-interval '1 day',$5,$6)`, org, onboardingID(t), asset, employee, operation, fx.actor.AccountID), "23514")
	allocationWant(t, "row of another organization", exec(`INSERT INTO org_asset_assignments(org_id,id,asset_id,employee_id,kind,valid_from,operation_id,assigned_by) VALUES($1,$2,$3,$4,'ADDITIONAL',clock_timestamp(),$5,$6)`, onboardingID(t), onboardingID(t), asset, employee, operation, fx.actor.AccountID), "42501")
	// The exclusion constraint alone, inside one transaction (no lock order).
	second, _ := assignmentPerson(t, fx)
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		for _, employee := range []string{employee, second} {
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_asset_assignments(org_id,id,asset_id,employee_id,kind,valid_from,operation_id,assigned_by) VALUES($1,$2,$3,$4,'PRIMARY',date_trunc('microseconds',clock_timestamp())+interval '1 hour',$5,$6)`, org, onboardingID(t), asset, employee, operation, fx.actor.AccountID); err != nil {
				return err
			}
		}
		return nil
	})
	if code, constraint := allocationCode(err); code != "23P01" || constraint != "org_asset_assignments_primary_no_overlap" {
		t.Fatal("overlapping primaries in one transaction", code, constraint, err)
	}
	// Forced RLS: no organization setting, no row and no write.
	if err := pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_asset_assignments)+(SELECT count(*) FROM org_asset_assignment_commands)`).Scan(&n); err != nil || n != 0 {
			t.Fatal("assignments visible without organization setting", n, err)
		}
		tag, err := tx.Exec(t.Context(), `UPDATE org_asset_assignments SET end_reason='x'`)
		if err != nil || tag.RowsAffected() != 0 {
			t.Fatal("unscoped update reached rows", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_asset_assignments(org_id,id,asset_id,employee_id,kind,valid_from,operation_id,assigned_by) VALUES($1,$2,$3,$4,'ADDITIONAL',clock_timestamp(),$5,$6)`, org, onboardingID(t), asset, employee, operation, fx.actor.AccountID)
		return err
	}); err == nil {
		t.Fatal("unscoped insert accepted")
	} else {
		allocationWant(t, "unscoped insert", err, "42501")
	}
	// Assignments are rights, not use: the asset-use source still answers.
	var usage string
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text`, org, asset).Scan(&usage)
	}); err != nil || usage != `{"readings": 0, "operations": 0}` {
		t.Fatal("asset usage source with assignments", usage, err)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_assignments WHERE org_id=$1`, org); n != 2 {
		t.Fatal("refused writes left rows", n)
	}
}
