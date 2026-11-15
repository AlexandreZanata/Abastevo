//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// allocationFixture is the actual activated organization with its first unit,
// lifecycle store and the disposable test-only period write privileges. The
// periods are still written only through their real guards.
func allocationFixture(t *testing.T) (legalFixtureState, *fleetpg.AssetStore, string) {
	t.Helper()
	fx, s, unit := lifecycleFixture(t)
	grantCostCentreTables(t, fx)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE ON org_asset_unit_periods,org_asset_cost_centre_periods TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	return fx, s, unit
}

func allocationExec(t *testing.T, fx legalFixtureState, query string, args ...any) error {
	t.Helper()
	return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), query, args...); return err })
}

func allocationCode(err error) (string, string) {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code, pe.ConstraintName
	}
	if err == nil {
		return "", ""
	}
	return "other", ""
}

func allocationWant(t *testing.T, label string, err error, code string) {
	t.Helper()
	if got, _ := allocationCode(err); got != code {
		t.Fatalf("%s: SQL refusal %q, want %q (%v)", label, got, code, err)
	}
}

// allocationUnit adds one more ACTIVE operating unit through the real guard.
func allocationUnit(t *testing.T, fx legalFixtureState, code string) string {
	t.Helper()
	id := onboardingID(t)
	if err := allocationExec(t, fx, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,$3,$4,'UTC','ACTIVE')`, fx.scope.OrgID().String(), id, "Synthetic unit "+code, code); err != nil {
		t.Fatal(err)
	}
	return id
}

// allocationCentre adds one ACTIVE cost centre through the real guard.
func allocationCentre(t *testing.T, fx legalFixtureState, code string) string {
	t.Helper()
	id := onboardingID(t)
	if err := allocationExec(t, fx, `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,$3,$4,'ACTIVE')`, fx.scope.OrgID().String(), id, "Synthetic centre "+code, code); err != nil {
		t.Fatal(err)
	}
	return id
}

// allocationReallocate is the test-only change of allocation (the public
// transfer command is EMP-FLEET-008): it closes the open period at the
// instant and opens the successor at the same instant in one transaction.
func allocationReallocate(t *testing.T, fx legalFixtureState, table, column, asset, target string, at time.Time) error {
	t.Helper()
	return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE `+table+` SET valid_to=$3 WHERE org_id=$1 AND asset_id=$2 AND valid_to IS NULL`, fx.scope.OrgID().String(), asset, at); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO `+table+`(org_id,asset_id,`+column+`,valid_from) VALUES($1,$2,$3,$4)`, fx.scope.OrgID().String(), asset, target, at)
		return err
	})
}

func allocationStart(t *testing.T, fx legalFixtureState, asset string) time.Time {
	t.Helper()
	var at time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT started_at FROM org_asset_unit_allocations WHERE org_id=$1 AND asset_id=$2`, fx.scope.OrgID().String(), asset).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at.UTC()
}

type allocationRow struct {
	target         string
	from, recorded time.Time
	to             *time.Time
}

func (r allocationRow) same(o allocationRow) bool {
	return r.target == o.target && r.from.Equal(o.from) && r.recorded.Equal(o.recorded) && (r.to == nil) == (o.to == nil) && (r.to == nil || r.to.Equal(*o.to))
}

func allocationRows(t *testing.T, fx legalFixtureState, table, column, asset string) []allocationRow {
	t.Helper()
	rows, err := fx.f.Admin.Query(t.Context(), `SELECT `+column+`::text,valid_from,valid_to,recorded_at FROM `+table+` WHERE org_id=$1 AND asset_id=$2 ORDER BY valid_from`, fx.scope.OrgID().String(), asset)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []allocationRow
	for rows.Next() {
		var r allocationRow
		if err := rows.Scan(&r.target, &r.from, &r.to, &r.recorded); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	return out
}

func allocationAt(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, asset string, at time.Time) (fa.AllocationSnapshot, error) {
	t.Helper()
	return s.UnitAndCostCentreAt(t.Context(), fx.scope, asset, at)
}

func allocationHistory(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, asset string, history fa.AllocationHistory, filter fa.AllocationFilter) fa.AllocationPage {
	t.Helper()
	page, err := s.ListAssetAllocations(t.Context(), fx.scope, fleetActor(fx), fx.member, asset, history, filter)
	if err != nil {
		t.Fatal("allocation history", history, err)
	}
	return page
}

// AC2 and B-BR-EMP-224: activation opens exactly one unit period at the exact
// initial allocation instant; no lifecycle command opens or closes one.
func TestFleetAllocationActivationOpensOneUnitPeriodThroughTheLifecycle(t *testing.T) {
	fx, s, unit := allocationFixture(t)
	draft, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Draft)
	if rows := allocationRows(t, fx, "org_asset_unit_periods", "unit_id", draft); len(rows) != 0 {
		t.Fatal("draft asset received an invented unit period")
	}
	if _, err := allocationAt(t, fx, s, draft, lifecycleDBNow(t, fx)); !errors.Is(err, fa.ErrUnallocated) {
		t.Fatal("draft asset answered a unit", err)
	}
	if page := allocationHistory(t, fx, s, draft, fa.UnitAllocationHistory, fa.AllocationFilter{Limit: 20}); len(page.Entries) != 0 || page.Next != nil {
		t.Fatal("draft unit history not empty")
	}
	input := fleetCreateInput()
	input.UnitID = unit
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	activation := fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: 1})
	if _, _, err := fleetWrite(t, fx, s, activation); err != nil {
		t.Fatal("actual activation", err)
	}
	asset := created.AssetID
	start := allocationStart(t, fx, asset)
	rows := allocationRows(t, fx, "org_asset_unit_periods", "unit_id", asset)
	if len(rows) != 1 || rows[0].target != unit || !rows[0].from.Equal(start) || rows[0].to != nil || rows[0].recorded.Before(start) {
		t.Fatal("activation did not open exactly one unit period at the initial allocation instant", rows)
	}
	if replay, replayed, err := fleetWrite(t, fx, s, activation); err != nil || !replayed || replay.Revision != 2 || len(allocationRows(t, fx, "org_asset_unit_periods", "unit_id", asset)) != 1 {
		t.Fatal("activation replay changed the unit history", err)
	}
	// AC4: the boundary instant belongs to the period; sub-microsecond floors.
	for name, tc := range map[string]struct {
		at   time.Time
		want error
	}{
		"before activation":    {start.Add(-time.Microsecond), fa.ErrUnallocated},
		"sub-microsecond less": {start.Add(-time.Nanosecond), fa.ErrUnallocated},
		"activation instant":   {start, nil},
		"sub-microsecond more": {start.Add(999 * time.Nanosecond), nil},
		"far future":           {time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC), nil},
	} {
		got, err := allocationAt(t, fx, s, asset, tc.at)
		if !errors.Is(err, tc.want) || err == nil && (got.UnitID != unit || got.CostCentreID != "" || got.AssetID != asset || !got.At.Equal(tc.at.Truncate(time.Microsecond))) {
			t.Fatal("snapshot port at", name, got, err)
		}
	}
	revision := int64(2)
	for _, command := range []fleet.LifecycleCommand{fleet.Deactivate, fleet.Reactivate, fleet.Deactivate, fleet.Dispose, fleet.RevertDisposal, fleet.Dispose, fleet.Archive} {
		result, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, command, asset, revision)))
		if err != nil {
			t.Fatal("lifecycle command", command, err)
		}
		revision = result.Revision
		after := allocationRows(t, fx, "org_asset_unit_periods", "unit_id", asset)
		if len(after) != 1 || !after[0].same(rows[0]) {
			t.Fatal("lifecycle command changed the unit history", command, after)
		}
	}
	if got, err := allocationAt(t, fx, s, asset, lifecycleDBNow(t, fx)); err != nil || got.UnitID != unit {
		t.Fatal("archived asset lost its historical unit", err)
	}
	page := allocationHistory(t, fx, s, asset, fa.UnitAllocationHistory, fa.AllocationFilter{Limit: 20})
	if len(page.Entries) != 1 || page.Entries[0].TargetID != unit || !page.Entries[0].Period.From.Equal(start) || page.Entries[0].Period.To != nil || page.Next != nil {
		t.Fatal("unit history read changed the stored period", page.Entries)
	}
	if page := allocationHistory(t, fx, s, asset, fa.CostCentreAllocationHistory, fa.AllocationFilter{Limit: 20}); len(page.Entries) != 0 {
		t.Fatal("cost centre invented for an asset that never had one")
	}
	if _, err := allocationAt(t, fx, s, onboardingID(t), start); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("missing asset answered", err)
	}
}

// 07-permissions §10 example 3 with the port (AC1, AC3, AC4; BR-AUTH-005):
// V is in Norte until the change and in Sul from it. A record created before
// the change stores Norte, and the port keeps answering Norte for its instant.
func TestFleetAllocationPortReproducesPermissionsExampleThree(t *testing.T) {
	fx, s, norte := allocationFixture(t)
	sul := allocationUnit(t, fx, "SUL")
	centreNorte, centreSul := allocationCentre(t, fx, "CC-NORTE"), allocationCentre(t, fx, "CC-SUL")
	v, _ := lifecycleAssetIn(t, fx, s, norte, fleet.Active)
	w, _ := lifecycleAssetIn(t, fx, s, norte, fleet.Active)
	march := allocationStart(t, fx, v)
	if err := allocationExec(t, fx, `INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,$4)`, fx.scope.OrgID().String(), v, centreNorte, march); err != nil {
		t.Fatal("first cost-centre period", err)
	}
	// A "March" fueling is created now and stores the snapshot it receives.
	type fueling struct {
		at               time.Time
		unit, costCentre string
	}
	fuelingAt := march.Add(1500 * time.Nanosecond)
	before, err := allocationAt(t, fx, s, v, fuelingAt)
	if err != nil || before.UnitID != norte || before.CostCentreID != centreNorte {
		t.Fatal("March snapshot", before, err)
	}
	stored := fueling{at: fuelingAt, unit: before.UnitID, costCentre: before.CostCentreID}
	// "April": the change instant is a past instant later than the fueling.
	april := lifecycleDBNow(t, fx).Truncate(time.Microsecond)
	if !april.After(fuelingAt) {
		t.Fatal("clock did not advance past the fueling instant")
	}
	if got, err := allocationAt(t, fx, s, v, april); err != nil || got.UnitID != norte {
		t.Fatal("open Norte period does not cover April before the change", err)
	}
	if err := allocationReallocate(t, fx, "org_asset_unit_periods", "unit_id", v, sul, april); err != nil {
		t.Fatal("test-only unit change", err)
	}
	if err := allocationReallocate(t, fx, "org_asset_cost_centre_periods", "cost_centre_id", v, centreSul, april); err != nil {
		t.Fatal("test-only cost-centre change", err)
	}
	for name, tc := range map[string]struct {
		at         time.Time
		unit, cost string
	}{
		"stored fueling instant": {stored.at, norte, centreNorte},
		"last Norte microsecond": {april.Add(-time.Microsecond), norte, centreNorte},
		"sub-microsecond before": {april.Add(-time.Nanosecond), norte, centreNorte},
		"change instant":         {april, sul, centreSul},
		"after change":           {april.Add(time.Hour), sul, centreSul},
	} {
		got, err := allocationAt(t, fx, s, v, tc.at)
		if err != nil || got.UnitID != tc.unit || got.CostCentreID != tc.cost {
			t.Fatal("example 3 snapshot", name, got.UnitID, got.CostCentreID, err)
		}
	}
	after, err := allocationAt(t, fx, s, v, stored.at)
	if err != nil || after.UnitID != stored.unit || after.CostCentreID != stored.costCentre || stored.unit != norte || stored.unit == sul {
		t.Fatal("the March record no longer matches its stored Norte snapshot", err)
	}
	// AC3: an asset without a cost-centre period answers none, never invented.
	if got, err := allocationAt(t, fx, s, w, april); err != nil || got.UnitID != norte || got.CostCentreID != "" {
		t.Fatal("cost centre invented", got.CostCentreID, err)
	}
	// Both histories read in start order with the exact keyset.
	first := allocationHistory(t, fx, s, v, fa.UnitAllocationHistory, fa.AllocationFilter{Limit: 1})
	if len(first.Entries) != 1 || first.Entries[0].TargetID != norte || first.Entries[0].Period.To == nil || !first.Entries[0].Period.To.Equal(april) || first.Next == nil || !first.Next.Equal(march) {
		t.Fatal("first unit history page", first.Entries)
	}
	second := allocationHistory(t, fx, s, v, fa.UnitAllocationHistory, fa.AllocationFilter{Limit: 1, After: first.Next})
	if len(second.Entries) != 1 || second.Entries[0].TargetID != sul || !second.Entries[0].Period.From.Equal(april) || !second.Entries[0].Period.Open() || second.Next != nil {
		t.Fatal("second unit history page", second.Entries)
	}
	centres := allocationHistory(t, fx, s, v, fa.CostCentreAllocationHistory, fa.AllocationFilter{Limit: 20})
	if len(centres.Entries) != 2 || centres.Entries[0].TargetID != centreNorte || centres.Entries[1].TargetID != centreSul || !centres.Entries[1].Period.From.Equal(april) {
		t.Fatal("cost-centre history", centres.Entries)
	}
	// The in-transaction port reads the caller's own uncommitted change.
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		got, err := s.UnitAndCostCentreAtTx(t.Context(), tx, fx.scope, v, stored.at)
		if err != nil || got.UnitID != norte {
			t.Fatal("in-transaction snapshot", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Negative cases on real PostGIS: archived structures, DRAFT assets, starts
// before activation, deletion, reopening, broken chains and missing org scope.
func TestFleetAllocationRefusesArchivedDraftAndBrokenPeriods(t *testing.T) {
	fx, s, unit := allocationFixture(t)
	org := fx.scope.OrgID().String()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT DELETE,TRUNCATE ON org_asset_unit_periods,org_asset_cost_centre_periods TO "+role); err != nil {
		t.Fatal(err)
	}
	other := allocationUnit(t, fx, "OTHER")
	archivedUnit := allocationUnit(t, fx, "OLD")
	if err := allocationExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, archivedUnit); err != nil {
		t.Fatal(err)
	}
	centre, archivedCentre := allocationCentre(t, fx, "CC-LIVE"), allocationCentre(t, fx, "CC-OLD")
	if err := allocationExec(t, fx, `UPDATE org_cost_centres SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, archivedCentre); err != nil {
		t.Fatal(err)
	}
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	draft, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Draft)
	start := allocationStart(t, fx, asset)
	baseline := allocationRows(t, fx, "org_asset_unit_periods", "unit_id", asset)
	now := lifecycleDBNow(t, fx).Truncate(time.Microsecond)
	insertUnit := `INSERT INTO org_asset_unit_periods(org_id,asset_id,unit_id,valid_from,valid_to) VALUES($1,$2,$3,$4,$5)`
	insertCentre := `INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from,valid_to) VALUES($1,$2,$3,$4,$5)`
	allocationWant(t, "archived unit", allocationReallocate(t, fx, "org_asset_unit_periods", "unit_id", asset, archivedUnit, now), "23503")
	allocationWant(t, "missing unit", allocationReallocate(t, fx, "org_asset_unit_periods", "unit_id", asset, onboardingID(t), now), "23503")
	allocationWant(t, "archived cost centre", allocationExec(t, fx, insertCentre, org, asset, archivedCentre, now, nil), "23503")
	allocationWant(t, "missing cost centre", allocationExec(t, fx, insertCentre, org, asset, onboardingID(t), now, nil), "23503")
	allocationWant(t, "draft unit period", allocationExec(t, fx, insertUnit, org, draft, unit, now, nil), "23514")
	allocationWant(t, "draft cost centre", allocationExec(t, fx, insertCentre, org, draft, centre, now, nil), "23514")
	allocationWant(t, "cost centre before activation", allocationExec(t, fx, insertCentre, org, asset, centre, start.Add(-time.Microsecond), nil), "23514")
	allocationWant(t, "unit before activation", allocationExec(t, fx, insertUnit, org, asset, other, start.Add(-time.Hour), start), "23514")
	allocationWant(t, "empty period", allocationExec(t, fx, insertCentre, org, asset, centre, now, now), "23514")
	allocationWant(t, "reversed period", allocationExec(t, fx, insertCentre, org, asset, centre, now, now.Add(-time.Hour)), "23514")
	allocationWant(t, "infinite start", allocationExec(t, fx, `INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,'infinity')`, org, asset, centre), "23514")
	allocationWant(t, "second open unit period", allocationExec(t, fx, insertUnit, org, asset, other, now, nil), "23P01")
	// AC2: the only open unit period of an ACTIVE asset can be neither removed
	// nor closed without a successor starting at the same instant.
	allocationWant(t, "delete only period", allocationExec(t, fx, `DELETE FROM org_asset_unit_periods WHERE org_id=$1 AND asset_id=$2`, org, asset), "55000")
	allocationWant(t, "truncate", allocationExec(t, fx, `TRUNCATE org_asset_unit_periods`), "55000")
	allocationWant(t, "close only period", allocationExec(t, fx, `UPDATE org_asset_unit_periods SET valid_to=$3 WHERE org_id=$1 AND asset_id=$2`, org, asset, now), "23514")
	allocationWant(t, "gap after close", database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE org_asset_unit_periods SET valid_to=$3 WHERE org_id=$1 AND asset_id=$2`, org, asset, now); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), insertUnit, org, asset, other, now.Add(time.Second), nil)
		return err
	}), "23514")
	allocationWant(t, "change open unit", allocationExec(t, fx, `UPDATE org_asset_unit_periods SET unit_id=$3 WHERE org_id=$1 AND asset_id=$2`, org, asset, other), "55000")
	allocationWant(t, "move start", allocationExec(t, fx, `UPDATE org_asset_unit_periods SET valid_from=valid_from-interval '1 day' WHERE org_id=$1 AND asset_id=$2`, org, asset), "55000")
	allocationWant(t, "rewrite recording", allocationExec(t, fx, `UPDATE org_asset_unit_periods SET recorded_at=recorded_at-interval '1 day' WHERE org_id=$1 AND asset_id=$2`, org, asset), "55000")
	if rows := allocationRows(t, fx, "org_asset_unit_periods", "unit_id", asset); len(rows) != 1 || !rows[0].same(baseline[0]) {
		t.Fatal("refused write changed the unit history", rows)
	}
	// A closed period is final: it cannot be reopened or moved.
	if err := allocationReallocate(t, fx, "org_asset_unit_periods", "unit_id", asset, other, now); err != nil {
		t.Fatal("valid test-only reallocation", err)
	}
	allocationWant(t, "reopen closed period", allocationExec(t, fx, `UPDATE org_asset_unit_periods SET valid_to=NULL WHERE org_id=$1 AND asset_id=$2 AND valid_to IS NOT NULL`, org, asset), "55000")
	allocationWant(t, "move closed end", allocationExec(t, fx, `UPDATE org_asset_unit_periods SET valid_to=valid_to+interval '1 hour' WHERE org_id=$1 AND asset_id=$2 AND valid_to IS NOT NULL`, org, asset), "55000")
	if err := allocationExec(t, fx, insertCentre, org, asset, centre, now, nil); err != nil {
		t.Fatal("valid cost-centre period", err)
	}
	allocationWant(t, "delete cost centre period", allocationExec(t, fx, `DELETE FROM org_asset_cost_centre_periods WHERE org_id=$1`, org), "55000")
	allocationWant(t, "truncate cost centre", allocationExec(t, fx, `TRUNCATE org_asset_cost_centre_periods`), "55000")
	allocationWant(t, "close cost centre empty", allocationExec(t, fx, `UPDATE org_asset_cost_centre_periods SET valid_to=valid_from WHERE org_id=$1`, org), "23514")
	if err := allocationExec(t, fx, `UPDATE org_asset_cost_centre_periods SET valid_to=$2 WHERE org_id=$1 AND valid_to IS NULL`, org, now.Add(time.Hour)); err != nil {
		t.Fatal("closing a cost-centre period without successor refused (cost centre is optional)", err)
	}
	// Forced RLS: without the organization setting nothing is visible or writable.
	var visible int
	err := pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_asset_unit_periods)+(SELECT count(*) FROM org_asset_cost_centre_periods)`).Scan(&visible); err != nil {
			return err
		}
		tag, err := tx.Exec(t.Context(), `UPDATE org_asset_cost_centre_periods SET valid_to=valid_to+interval '1 hour'`)
		if err != nil || tag.RowsAffected() != 0 {
			t.Fatal("unscoped update reached a period", err)
		}
		return nil
	})
	if err != nil || visible != 0 {
		t.Fatal("periods visible without organization setting", visible, err)
	}
	unscoped := pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), insertCentre, org, asset, centre, now.Add(2*time.Hour), nil)
		return err
	})
	allocationWant(t, "unscoped insert", unscoped, "42501")
	if _, err := s.UnitAndCostCentreAt(t.Context(), fx.scope, asset, time.Time{}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("unset instant reached the port", err)
	}
	if _, err := s.UnitAndCostCentreAt(t.Context(), fx.scope, "bad", now); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("invalid asset reached the port", err)
	}
	err = pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := s.UnitAndCostCentreAtTx(t.Context(), tx, fx.scope, asset, now); err == nil {
			t.Fatal("port read through an unscoped transaction")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Overlapping periods under real concurrent transactions: the exclusion
// constraint decides; no sleep orders the writers, a lock wait is observed.
func TestFleetAllocationOverlapUnderConcurrencyExclusionWins(t *testing.T) {
	fx, s, unit := allocationFixture(t)
	org := fx.scope.OrgID().String()
	sul, west := allocationUnit(t, fx, "SUL"), allocationUnit(t, fx, "WEST")
	first, second := allocationCentre(t, fx, "CC-A"), allocationCentre(t, fx, "CC-B")
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	now := lifecycleDBNow(t, fx).Truncate(time.Microsecond)
	pool := fx.f.RuntimePool(t, 2)
	race := func(label string, holder, contender func(pgx.Tx) error) (string, string) {
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
		go func() {
			done <- database.WithOrgTx(ctx, pool, fx.scope, contender)
		}()
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
		return allocationCode(<-done)
	}
	code, constraint := race("cost centres", func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,$4)`, org, asset, first, now)
		return err
	}, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,$4)`, org, asset, second, now.Add(time.Second))
		return err
	})
	if code != "23P01" || constraint != "org_asset_cost_centre_periods_no_overlap" {
		t.Fatal("concurrent overlapping cost centres were not decided by the exclusion constraint", code, constraint)
	}
	transfer := func(target string, at time.Time) func(pgx.Tx) error {
		return func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `UPDATE org_asset_unit_periods SET valid_to=$3 WHERE org_id=$1 AND asset_id=$2 AND valid_to IS NULL`, org, asset, at); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `INSERT INTO org_asset_unit_periods(org_id,asset_id,unit_id,valid_from) VALUES($1,$2,$3,$4)`, org, asset, target, at)
			return err
		}
	}
	code, constraint = race("unit changes", transfer(sul, now), transfer(west, now.Add(time.Second)))
	if code != "23P01" || constraint != "org_asset_unit_periods_no_overlap" {
		t.Fatal("concurrent overlapping unit changes were not decided by the exclusion constraint", code, constraint)
	}
	units := allocationRows(t, fx, "org_asset_unit_periods", "unit_id", asset)
	centres := allocationRows(t, fx, "org_asset_cost_centre_periods", "cost_centre_id", asset)
	if len(units) != 2 || units[0].target != unit || units[0].to == nil || !units[0].to.Equal(now) || units[1].target != sul || units[1].to != nil || len(centres) != 1 || centres[0].target != first {
		t.Fatal("losing writer left an effect", units, centres)
	}
}

// The unit and cost-centre archive guards still block archiving a structure
// with a current or future allocation of an ACTIVE asset (B-BR-EMP-224).
// Fresh units keep employee/team usage out of the counted sources.
func TestFleetAllocationArchiveGuardsCountCurrentAndFuturePeriods(t *testing.T) {
	fx, s, _ := allocationFixture(t)
	org := fx.scope.OrgID().String()
	east, west, later, idle := allocationUnit(t, fx, "EAST"), allocationUnit(t, fx, "WEST"), allocationUnit(t, fx, "LATER"), allocationUnit(t, fx, "IDLE")
	centre := allocationCentre(t, fx, "CC-USED")
	v, _ := lifecycleAssetIn(t, fx, s, east, fleet.Active)
	w, _ := lifecycleAssetIn(t, fx, s, west, fleet.Active)
	if _, revision := lifecycleAssetIn(t, fx, s, idle, fleet.Inactive); revision != 3 {
		t.Fatal("inactive fixture", revision)
	}
	archiveUnit := func(id string) error {
		return allocationExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, id)
	}
	allocationWant(t, "unit with open period", archiveUnit(east), "P7703")
	if err := archiveUnit(idle); err != nil {
		t.Fatal("out-of-service asset blocked unit archive (FLEET-003 semantics)", err)
	}
	now := lifecycleDBNow(t, fx).Truncate(time.Microsecond)
	if err := allocationReallocate(t, fx, "org_asset_unit_periods", "unit_id", v, west, now); err != nil {
		t.Fatal(err)
	}
	if err := archiveUnit(east); err != nil {
		t.Fatal("ended period still blocks unit archive", err)
	}
	if err := allocationReallocate(t, fx, "org_asset_unit_periods", "unit_id", w, later, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "unit with future period", archiveUnit(later), "P7703")
	allocationWant(t, "unit current until a future change", archiveUnit(west), "P7703")
	var usage string
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT org_unit_current_usage($1,$2)::text`, org, west).Scan(&usage)
	}); err != nil || usage != `{"assets": 2, "budgets": 0, "employees": 0, "operations": 0}` {
		t.Fatal("unit usage does not count both the current and the ending allocation", usage, err)
	}
	if err := allocationExec(t, fx, `INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,$4)`, org, v, centre, now); err != nil {
		t.Fatal(err)
	}
	archiveCentre := func() error {
		return allocationExec(t, fx, `UPDATE org_cost_centres SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, centre)
	}
	allocationWant(t, "cost centre with open period", archiveCentre(), "P7803")
	if err := allocationExec(t, fx, `UPDATE org_asset_cost_centre_periods SET valid_to=$3 WHERE org_id=$1 AND asset_id=$2`, org, v, now.Add(time.Microsecond)); err != nil {
		t.Fatal(err)
	}
	if err := archiveCentre(); err != nil {
		t.Fatal("ended cost-centre period blocks archive", err)
	}
}
