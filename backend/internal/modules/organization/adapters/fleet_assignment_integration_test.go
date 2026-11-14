//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// assignmentStore is the lifecycle store plus the exact EMP-FLEET-006 grants.
func assignmentStore(t *testing.T, fx legalFixtureState) *fleetpg.AssetStore {
	t.Helper()
	s := lifecycleStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_asset_assignments TO "+role+";GRANT SELECT,INSERT ON org_asset_assignment_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	return s
}

// assignmentPerson is a synthetic ACTIVE employee record with an ACTIVE
// membership (DRIVER, SELF) and its own synthetic account.
func assignmentPerson(t *testing.T, fx legalFixtureState) (string, string) {
	t.Helper()
	account, employee, member := onboardingID(t), onboardingID(t), onboardingID(t)
	if _, err := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, account, "synthetic-assignee-"+account[:8]); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, []any{org, employee}},
		{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, member, employee, account}},
		{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'DRIVER','SELF',$3,clock_timestamp()-interval '1 minute',$4,'Synthetic assignee')`, []any{org, onboardingID(t), member, fx.member}},
		{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, member}},
	} {
		if err := allocationExec(t, fx, q.sql, q.args...); err != nil {
			t.Fatal("assignee fixture", err)
		}
	}
	return employee, member
}

// assignmentEmployeeIn is an employee record without access in a given state.
func assignmentEmployeeIn(t *testing.T, fx legalFixtureState, state string) string {
	t.Helper()
	employee := onboardingID(t)
	org := fx.scope.OrgID().String()
	if err := allocationExec(t, fx, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, org, employee); err != nil {
		t.Fatal(err)
	}
	if state != "ACTIVE" {
		if err := allocationExec(t, fx, `UPDATE org_employee_identities SET state=$3,revision=revision+1 WHERE org_id=$1 AND id=$2`, org, employee, state); err != nil {
			t.Fatal(err)
		}
	}
	return employee
}

func assignmentCreate(t *testing.T, v fa.AssignmentCreateInput) fa.AssignmentCreate {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := fa.NewAssignmentCreate(v)
	if err != nil {
		t.Fatal("invalid fixture assignment command", err)
	}
	return c
}

func assignmentWrite(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, c fa.AssignmentCreate) (fa.AssignmentResult, bool, error) {
	t.Helper()
	v, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(v.EmployeeIDs))
	for i := range ids {
		ids[i] = onboardingID(t)
	}
	return s.CreateAssignments(t.Context(), fx.scope, fleetActor(fx), fx.member, ids, c)
}

func assignmentEnd(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, v fa.AssignmentEndInput) (fa.AssignmentResult, bool, error) {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := fa.NewAssignmentEnd(v)
	if err != nil {
		t.Fatal("invalid fixture end command", err)
	}
	return s.EndAssignment(t.Context(), fx.scope, fleetActor(fx), fx.member, c)
}

type assignmentRow struct {
	id, employee, kind, cause, endOperation string
	from                                    time.Time
	to, endedAt                             *time.Time
	source                                  *int64
}

func assignmentRowsOf(t *testing.T, fx legalFixtureState, asset string) map[string]assignmentRow {
	t.Helper()
	rows, err := fx.f.Admin.Query(t.Context(), `SELECT id::text,employee_id::text,kind,end_cause,COALESCE(end_operation_id::text,''),valid_from,valid_to,ended_at,source_revision FROM org_asset_assignments WHERE org_id=$1 AND asset_id=$2`, fx.scope.OrgID().String(), asset)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]assignmentRow{}
	for rows.Next() {
		var r assignmentRow
		if err := rows.Scan(&r.id, &r.employee, &r.kind, &r.cause, &r.endOperation, &r.from, &r.to, &r.endedAt, &r.source); err != nil {
			t.Fatal(err)
		}
		out[r.id] = r
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	return out
}

func assignedAt(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, member, asset string, at time.Time) bool {
	t.Helper()
	var assigned bool
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		var err error
		// The borrowed intake port, exactly as the signed meter intake holds it.
		assigned, err = s.AssignedAtDBTX(t.Context(), syncapp.Borrow(tx), fx.scope, member, asset, at)
		return err
	}); err != nil {
		t.Fatal("assigned-asset port", err)
	}
	return assigned
}

func assignmentCount(t *testing.T, fx legalFixtureState, query string, args ...any) int {
	t.Helper()
	var n int
	if err := fx.f.Admin.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// AC1, AC3, AC4, replay and the assigned-asset port (B-BR-EMP-243/244/246).
func TestFleetAssignmentStoreDerivesStatesAtServerTimeAndKeepsReceipts(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := assignmentStore(t, fx)
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	other, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	e1, m1 := assignmentPerson(t, fx)
	e2, m2 := assignmentPerson(t, fx)
	e3, m3 := assignmentPerson(t, fx)
	org := fx.scope.OrgID().String()

	// AC4: one assignment per employee, in command order, with one receipt
	// and one audit event; the same intent replays the first result.
	bulk := assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{e1, e2}, Reason: "Synthetic shared vehicle"})
	before := lifecycleDBNow(t, fx)
	created, replayed, err := assignmentWrite(t, fx, s, bulk)
	if err != nil || replayed || created.AssetID != asset || len(created.AssignmentIDs) != 2 || created.ReplacedID != "" {
		t.Fatal("bulk create", err, created)
	}
	rows := assignmentRowsOf(t, fx, asset)
	if len(rows) != 2 || rows[created.AssignmentIDs[0]].employee != e1 || rows[created.AssignmentIDs[1]].employee != e2 || rows[created.AssignmentIDs[0]].from.Before(before.Truncate(time.Microsecond)) || rows[created.AssignmentIDs[0]].to != nil {
		t.Fatal("bulk rows", rows)
	}
	again, replayed, err := assignmentWrite(t, fx, s, bulk)
	if err != nil || !replayed || !slices.Equal(again.AssignmentIDs, created.AssignmentIDs) {
		t.Fatal("bulk replay", err)
	}
	bulkInput, _ := bulk.Snapshot()
	bulkInput.Reason = "Synthetic changed intent"
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, bulkInput)); !errors.Is(err, fa.ErrOperationReused) {
		t.Fatal("operation reuse", err)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2 AND action='fleet.assignment.created' AND target_id=$3`, org, bulkInput.OperationID, asset); n != 1 {
		t.Fatal("create audit event", n)
	}
	// AC4 negative: one non-ACTIVE employee leaves nothing at all.
	inactive := assignmentEmployeeIn(t, fx, "INACTIVE")
	failed := assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{e3, inactive}})
	if _, _, err := assignmentWrite(t, fx, s, failed); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("bulk with an inactive employee", err)
	}
	failedInput, _ := failed.Snapshot()
	if len(assignmentRowsOf(t, fx, asset)) != 2 || assignmentCount(t, fx, `SELECT count(*) FROM org_asset_assignment_commands WHERE org_id=$1 AND operation_id=$2`, org, failedInput.OperationID) != 0 || assignmentCount(t, fx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2`, org, failedInput.OperationID) != 0 {
		t.Fatal("partial bulk effect")
	}
	// AC1: a future start is SCHEDULED, inside ACTIVE, ended ENDED.
	now := lifecycleDBNow(t, fx).Truncate(time.Microsecond)
	start, end := now.Add(time.Hour), now.Add(3*time.Hour)
	scheduled, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.TemporaryAssignment, EmployeeIDs: []string{e3}, Start: &start, End: &end}))
	if err != nil {
		t.Fatal("scheduled temporary", err)
	}
	past := now.Add(-time.Hour)
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{e3}, Start: &past})); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("past start accepted", err)
	}
	states := func() map[string]fleet.AssignmentState {
		t.Helper()
		page, err := s.ListAssetAssignments(t.Context(), fx.scope, fleetActor(fx), fx.member, asset, fa.AssignmentFilter{Limit: 10})
		if err != nil {
			t.Fatal("asset assignment list", err)
		}
		out := map[string]fleet.AssignmentState{}
		for _, v := range page.Assignments {
			if v.State != v.Period.StateAt(page.At) {
				t.Fatal("state not derived at the page instant")
			}
			out[v.ID] = v.State
		}
		return out
	}
	if got := states(); got[created.AssignmentIDs[0]] != fleet.AssignmentActive || got[created.AssignmentIDs[1]] != fleet.AssignmentActive || got[scheduled.AssignmentIDs[0]] != fleet.AssignmentScheduled || len(got) != 3 {
		t.Fatal("derived states", got)
	}
	// Ending: ACTIVE ends now, SCHEDULED keeps an empty period; an ENDED one
	// refuses, a replay returns the first result.
	endFirst := fa.AssignmentEndInput{OperationID: onboardingID(t), AssetID: asset, AssignmentID: created.AssignmentIDs[0], Reason: "Synthetic return"}
	if r, replayed, err := assignmentEnd(t, fx, s, endFirst); err != nil || replayed || r.AssignmentIDs[0] != created.AssignmentIDs[0] {
		t.Fatal("end active", err)
	}
	if r, replayed, err := assignmentEnd(t, fx, s, endFirst); err != nil || !replayed || r.AssignmentIDs[0] != created.AssignmentIDs[0] {
		t.Fatal("end replay", err)
	}
	if _, _, err := assignmentEnd(t, fx, s, fa.AssignmentEndInput{AssetID: asset, AssignmentID: created.AssignmentIDs[0]}); !errors.Is(err, fleet.ErrAssignmentEnded) {
		t.Fatal("ended assignment ended again", err)
	}
	if _, _, err := assignmentEnd(t, fx, s, fa.AssignmentEndInput{AssetID: asset, AssignmentID: scheduled.AssignmentIDs[0]}); err != nil {
		t.Fatal("end scheduled", err)
	}
	if _, _, err := assignmentEnd(t, fx, s, fa.AssignmentEndInput{AssetID: other, AssignmentID: created.AssignmentIDs[1]}); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("assignment of another asset", err)
	}
	if _, _, err := assignmentEnd(t, fx, s, fa.AssignmentEndInput{AssetID: asset, AssignmentID: onboardingID(t)}); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("unknown assignment", err)
	}
	rows = assignmentRowsOf(t, fx, asset)
	first, cancelled := rows[created.AssignmentIDs[0]], rows[scheduled.AssignmentIDs[0]]
	if first.cause != "ENDED" || first.to == nil || first.to.Before(now) || first.endOperation != endFirst.OperationID || first.endedAt == nil || first.source != nil {
		t.Fatal("ended row", first)
	}
	if cancelled.cause != "ENDED" || cancelled.to == nil || !cancelled.to.Equal(start) || !cancelled.from.Equal(start) {
		t.Fatal("scheduled end keeps an empty period", cancelled)
	}
	if got := states(); got[created.AssignmentIDs[0]] != fleet.AssignmentEnded || got[scheduled.AssignmentIDs[0]] != fleet.AssignmentEnded || got[created.AssignmentIDs[1]] != fleet.AssignmentActive {
		t.Fatal("states after ending", got)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='fleet.assignment.ended'`, org); n != 2 {
		t.Fatal("end audit events", n)
	}
	// B-BR-EMP-246: the port is half-open on both periods.
	tstart, tend := lifecycleDBNow(t, fx).Truncate(time.Microsecond).Add(time.Hour), lifecycleDBNow(t, fx).Truncate(time.Microsecond).Add(2*time.Hour)
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: other, Kind: fleet.TemporaryAssignment, EmployeeIDs: []string{e3}, Start: &tstart, End: &tend})); err != nil {
		t.Fatal(err)
	}
	for at, want := range map[time.Time]bool{tstart.Add(-time.Microsecond): false, tstart: true, tstart.Add(999 * time.Nanosecond): true, tend.Add(-time.Microsecond): true, tend: false} {
		if assignedAt(t, fx, s, m3, other, at) != want {
			t.Fatal("half-open assigned-asset port", at, want)
		}
	}
	if assignedAt(t, fx, s, m1, other, tstart) || assignedAt(t, fx, s, m3, asset, tstart) {
		t.Fatal("port answered for another member or asset")
	}
	var assets []string
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		var err error
		assets, err = s.AssetsAssignedAtDBTX(t.Context(), tx, fx.scope, m3, tstart)
		return err
	}); err != nil || !slices.Equal(assets, []string{other}) {
		t.Fatal("assets assigned at an instant", assets, err)
	}
	// The own list belongs to the requesting member (the owner here): only its
	// ACTIVE assignments, never a scheduled one or another member's.
	var ownerEmployee string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, org, fx.member).Scan(&ownerEmployee); err != nil {
		t.Fatal(err)
	}
	mine, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{ownerEmployee}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: other, Kind: fleet.TemporaryAssignment, EmployeeIDs: []string{ownerEmployee}, Start: &tstart, End: &tend})); err != nil {
		t.Fatal(err)
	}
	own, err := s.ListOwnAssignments(t.Context(), fx.scope, fleetActor(fx), fx.member, fa.OwnAssignmentFilter{Limit: 10})
	if err != nil || len(own.Assignments) != 1 || own.Assignments[0].ID != mine.AssignmentIDs[0] || own.Assignments[0].State != fleet.AssignmentActive {
		t.Fatal("own ACTIVE assignments", err, own)
	}
	if _, err := s.ListOwnAssignments(t.Context(), fx.scope, fleetActor(fx), m2, fa.OwnAssignmentFilter{Limit: 10}); err == nil {
		t.Fatal("another member's own list read")
	}
	// An ended membership is never listed as assignee, although its employee
	// record (not terminated) keeps the assignment; its past stays answerable.
	beforeEnd := lifecycleDBNow(t, fx)
	if err := allocationExec(t, fx, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, m2); err != nil {
		t.Fatal(err)
	}
	if assignedAt(t, fx, s, m2, asset, lifecycleDBNow(t, fx)) || !assignedAt(t, fx, s, m2, asset, beforeEnd) {
		t.Fatal("ended membership listed as assignee or history lost")
	}
	if rows := assignmentRowsOf(t, fx, asset); rows[created.AssignmentIDs[1]].to != nil {
		t.Fatal("membership end changed the assignment")
	}
	_ = e2
}

// AC2 and the concurrency negative (B-BR-EMP-244): the exclusion constraint
// refuses an overlapping primary; replace ends the previous one at the new
// start in the same transaction; two concurrent primaries leave one.
func TestFleetAssignmentPrimaryOverlapReplaceAndConcurrency(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := assignmentStore(t, fx)
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	e1, _ := assignmentPerson(t, fx)
	e2, _ := assignmentPerson(t, fx)
	e3, _ := assignmentPerson(t, fx)
	org := fx.scope.OrgID().String()
	p1, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{e1}}))
	if err != nil {
		t.Fatal("first primary", err)
	}
	now := lifecycleDBNow(t, fx).Truncate(time.Microsecond)
	later := now.Add(time.Hour)
	overlap := assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{e2}, Start: &later})
	if _, _, err := assignmentWrite(t, fx, s, overlap); !errors.Is(err, fa.ErrAssignmentOverlap) {
		t.Fatal("overlapping primary", err)
	}
	if len(assignmentRowsOf(t, fx, asset)) != 1 {
		t.Fatal("refused primary left a row")
	}
	// Other kinds never conflict with the primary.
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.TemporaryAssignment, EmployeeIDs: []string{e3}, End: &later})); err != nil {
		t.Fatal("substitute beside the primary", err)
	}
	replace := assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{e2}, Start: &later, ReplacePrimary: true})
	p2, _, err := assignmentWrite(t, fx, s, replace)
	if err != nil || p2.ReplacedID != p1.AssignmentIDs[0] {
		t.Fatal("replace primary", err, p2)
	}
	replaceInput, _ := replace.Snapshot()
	rows := assignmentRowsOf(t, fx, asset)
	old, successor := rows[p1.AssignmentIDs[0]], rows[p2.AssignmentIDs[0]]
	if old.cause != "REPLACED" || old.to == nil || !old.to.Equal(later) || old.endOperation != replaceInput.OperationID || !successor.from.Equal(later) || successor.to != nil {
		t.Fatal("replace did not end the previous primary at the new start", old, successor)
	}
	if again, replayed, err := assignmentWrite(t, fx, s, replace); err != nil || !replayed || again.ReplacedID != p1.AssignmentIDs[0] {
		t.Fatal("replace replay", err)
	}
	// A replace whose new period reaches a later primary is refused whole: the
	// primary it would have shortened keeps its end.
	soon, until := now.Add(30*time.Minute), now.Add(90*time.Minute)
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{e3}, Start: &soon, End: &until, ReplacePrimary: true})); !errors.Is(err, fa.ErrAssignmentOverlap) {
		t.Fatal("replace into a later primary", err)
	}
	if rows := assignmentRowsOf(t, fx, asset); !rows[p1.AssignmentIDs[0]].to.Equal(later) || rows[p1.AssignmentIDs[0]].endOperation != replaceInput.OperationID || len(rows) != 3 {
		t.Fatal("refused replace changed the previous primary")
	}
	// Two concurrent primaries on a fresh asset: both wait behind a held root
	// lock (observed, no sleep), then exactly one commits.
	contested, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	holder, err := fx.f.Runtime.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(ctx)
	var holderPID int
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, employee := range []string{e1, e2} {
		c := assignmentCreate(t, fa.AssignmentCreateInput{AssetID: contested, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{employee}})
		id := onboardingID(t)
		go func() {
			_, _, err := s.CreateAssignments(ctx, fx.scope, fleetActor(fx), fx.member, []string{id}, c)
			results <- err
		}()
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for blocked := false; !blocked; {
		if err := fx.f.Admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND $2=ANY(pg_blocking_pids(pid)))`, fx.f.RuntimeRole, holderPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if !blocked {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				t.Fatal("no lock wait observed")
			}
		}
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	first, second := <-results, <-results
	if (first == nil) == (second == nil) || !errors.Is(errors.Join(first, second), fa.ErrAssignmentOverlap) {
		t.Fatal("concurrent primaries", first, second)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_assignments WHERE org_id=$1 AND asset_id=$2 AND kind='PRIMARY'`, org, contested); n != 1 {
		t.Fatal("concurrent primaries left", n)
	}
}

// Negative references (B-BR-EMP-243): another organization's employee or
// asset is not found; a non-ACTIVE employee or a non-assignable asset is a
// validation refusal; an INACTIVE asset may still be assigned.
func TestFleetAssignmentRefusesForeignInactiveAndUnassignableReferences(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := assignmentStore(t, fx)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var otherUnit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, other.scope.OrgID().String()).Scan(&otherUnit); err != nil {
		t.Fatal(err)
	}
	foreignEmployee, _ := assignmentPerson(t, other)
	foreignAsset, _ := lifecycleAssetIn(t, other, s, otherUnit, fleet.Active)
	employee, _ := assignmentPerson(t, fx)
	active, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	create := func(asset string, kind fleet.AssignmentKind, employee string) error {
		_, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: kind, EmployeeIDs: []string{employee}}))
		return err
	}
	if err := create(active, fleet.AdditionalAssignment, foreignEmployee); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("employee of another organization", err)
	}
	if err := create(active, fleet.AdditionalAssignment, onboardingID(t)); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("unknown employee", err)
	}
	if err := create(foreignAsset, fleet.AdditionalAssignment, employee); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("asset of another organization", err)
	}
	for _, state := range []string{"INACTIVE", "TERMINATED"} {
		if err := create(active, fleet.AdditionalAssignment, assignmentEmployeeIn(t, fx, state)); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("employee not ACTIVE", state, err)
		}
	}
	for _, state := range []fleet.State{fleet.Draft, fleet.Disposed, fleet.Archived} {
		asset, _ := lifecycleAssetIn(t, fx, s, unit, state)
		if err := create(asset, fleet.PrimaryAssignment, employee); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("asset not assignable", state, err)
		}
	}
	inactive, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Inactive)
	if err := create(inactive, fleet.PrimaryAssignment, employee); err != nil {
		t.Fatal("INACTIVE asset refused", err)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_assignments WHERE org_id=$1`, fx.scope.OrgID().String()); n != 1 {
		t.Fatal("refused references left rows", n)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_assignments WHERE org_id=$1`, other.scope.OrgID().String()); n != 0 {
		t.Fatal("foreign organization changed", n)
	}
}
