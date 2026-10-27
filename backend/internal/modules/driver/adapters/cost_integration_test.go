//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	ledgeradapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/adapters"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedLedgerExpense(t *testing.T, p *pgxpool.Pool, owner, vehicle string, amount int64) string {
	t.Helper()
	id := newDriverID(t)
	payload := fmt.Sprintf(`{"amount_centavos":%d}`, amount)
	if _, e := p.Exec(context.Background(), "INSERT INTO ledger_records(account_id,dataset,id,vehicle_id,occurred_at,payload) VALUES($1,'expense',$2,$3,now(),$4)", owner, id, vehicle, payload); e != nil {
		t.Fatal(e)
	}
	return id
}

func costByID(profile domain.VehicleCostProfile, id string) domain.ProfileCost {
	for _, cost := range profile.Costs {
		if cost.ID == id {
			return cost
		}
	}
	return domain.ProfileCost{}
}

func costProfile(vehicle string, version int64, ledgerID string, actualAmount, modeledAmount *int64) domain.VehicleCostProfile {
	costs := []domain.ProfileCost{{ID: "modeled-1", Kind: "RESERVE", Provenance: "MODELED", From: "2026-01-01", Amount: modeledAmount}}
	if ledgerID != "" {
		costs = append(costs, domain.ProfileCost{ID: "actual-1", Kind: "MAINTENANCE", Provenance: "ACTUAL", LedgerSourceID: ledgerID, SourceRevision: 1, From: "2026-01-01", Amount: actualAmount})
	}
	return domain.VehicleCostProfile{Vehicle: vehicle, Ownership: "OWNED", Timezone: "America/Cuiaba", From: "2026-01-01", Version: version, Costs: costs}
}

func TestDriverCostProfileVersionsLinksRestartAndErasure(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	ledgerID := seedLedgerExpense(t, p, owner, car, 12_345)
	actual, zero := int64(12_345), int64(0)
	s := NewWithLedger(p, cloud.RequireDriverOwnedVehicle, ledgeradapters.RequireDriverLedgerCost)
	first := app.SaveCostProfile{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Profile: costProfile(car, 1, ledgerID, &actual, nil)}
	receipt, e := s.SaveCostProfile(ctx, owner, first)
	if e != nil || receipt.Version != 1 || receipt.Replayed {
		t.Fatal("create", receipt, e)
	}
	var ledgerCount int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM ledger_records WHERE account_id=$1", owner).Scan(&ledgerCount); e != nil || ledgerCount != 1 {
		t.Fatal("duplicate ledger writer", ledgerCount, e)
	}
	got, e := s.GetCostProfile(ctx, owner, car, 1)
	if e != nil || len(got.Costs) != 2 || costByID(got, "modeled-1").Amount != nil || costByID(got, "actual-1").Amount == nil || *costByID(got, "actual-1").Amount != 12_345 || costByID(got, "actual-1").LedgerSourceID != ledgerID {
		t.Fatalf("read %+v %v", got, e)
	}
	second := app.SaveCostProfile{OperationID: newDriverID(t), ExpectedVersion: 1, ExpectedEpoch: 0, Profile: costProfile(car, 2, ledgerID, &actual, &zero)}
	if receipt, e = s.SaveCostProfile(ctx, owner, second); e != nil || receipt.Version != 2 {
		t.Fatal("edit", receipt, e)
	}
	if replay, e := s.SaveCostProfile(ctx, owner, second); e != nil || !replay.Replayed || replay.Version != 2 {
		t.Fatal("replay", replay, e)
	}
	stale := second
	stale.OperationID = newDriverID(t)
	if _, e = s.SaveCostProfile(ctx, owner, stale); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale version", e)
	}
	list, e := s.ListCostProfiles(ctx, owner, car, 0, 10)
	if e != nil || len(list) != 2 || list[0].Version != 1 || list[1].Version != 2 || costByID(list[0], "modeled-1").Amount != nil || *costByID(list[1], "modeled-1").Amount != 0 {
		t.Fatalf("versions %+v %v", list, e)
	}
	if _, e = p.Exec(ctx, "UPDATE ledger_records SET revision=revision+1 WHERE account_id=$1 AND id=$2", owner, ledgerID); e != nil {
		t.Fatal(e)
	}
	third := app.SaveCostProfile{OperationID: newDriverID(t), ExpectedVersion: 2, ExpectedEpoch: 0, Profile: costProfile(car, 3, ledgerID, &actual, &zero)}
	if _, e = s.SaveCostProfile(ctx, owner, third); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale ledger revision", e)
	}
	if e = s.EraseCostProfile(ctx, owner, car, 2, 0); e != nil {
		t.Fatal(e)
	}
	if _, e = s.GetCostProfile(ctx, owner, car, 2); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("erased profile readable", e)
	}
	if _, e = s.SaveCostProfile(ctx, owner, second); !errors.Is(e, app.ErrErased) {
		t.Fatal("erased profile recreated", e)
	}
	if e = s.EraseCostOwner(ctx, owner, 0); e != nil {
		t.Fatal(e)
	}
	first.OperationID = newDriverID(t)
	first.ExpectedEpoch = 0
	if _, e = s.SaveCostProfile(ctx, owner, first); !errors.Is(e, app.ErrErased) {
		t.Fatal("old epoch accepted", e)
	}
	if list, e = s.ListCostProfiles(ctx, owner, car, 0, 10); e != nil || len(list) != 0 {
		t.Fatal("owner erase left profiles", list, e)
	}
}

func TestDriverCostRejectsForeignDeletedVoidedAndAmbiguousLinks(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	foreignOwner := seedDriverOwner(t, p)
	foreignCar := seedDriverVehicle(t, p, foreignOwner)
	s := NewWithLedger(p, cloud.RequireDriverOwnedVehicle, ledgeradapters.RequireDriverLedgerCost)

	foreign := seedLedgerExpense(t, p, foreignOwner, foreignCar, 100)
	amount := int64(100)
	in := app.SaveCostProfile{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Profile: costProfile(car, 1, foreign, &amount, nil)}
	if _, e := s.SaveCostProfile(ctx, owner, in); !errors.Is(e, app.ErrDenied) {
		t.Fatal("foreign ledger source", e)
	}
	wrongVehicle := seedLedgerExpense(t, p, owner, foreignCar, 100)
	in.OperationID = newDriverID(t)
	in.Profile = costProfile(car, 1, wrongVehicle, &amount, nil)
	if _, e := s.SaveCostProfile(ctx, owner, in); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("vehicle mismatch", e)
	}
	deleted := seedLedgerExpense(t, p, owner, car, 100)
	if _, e := p.Exec(ctx, "UPDATE ledger_records SET deleted=true,revision=revision+1 WHERE account_id=$1 AND id=$2", owner, deleted); e != nil {
		t.Fatal(e)
	}
	in.OperationID = newDriverID(t)
	in.Profile = costProfile(car, 1, deleted, &amount, nil)
	if _, e := s.SaveCostProfile(ctx, owner, in); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("deleted ledger source", e)
	}
	voided := seedLedgerExpense(t, p, owner, car, 100)
	if _, e := p.Exec(ctx, "UPDATE ledger_records SET voided=true WHERE account_id=$1 AND id=$2", owner, voided); e != nil {
		t.Fatal(e)
	}
	in.OperationID = newDriverID(t)
	in.Profile = costProfile(car, 1, voided, &amount, nil)
	if _, e := s.SaveCostProfile(ctx, owner, in); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("voided ledger source", e)
	}
	ambiguous := newDriverID(t)
	if _, e := p.Exec(ctx, "INSERT INTO ledger_records(account_id,dataset,id,vehicle_id,occurred_at,payload) VALUES($1,'expense',$2,$3,now(),'{\"amount_centavos\":100}'),($1,'refuel',$2,$3,now(),'{\"total_centavos\":100}')", owner, ambiguous, car); e != nil {
		t.Fatal(e)
	}
	in.OperationID = newDriverID(t)
	in.Profile = costProfile(car, 1, ambiguous, &amount, nil)
	if _, e := s.SaveCostProfile(ctx, owner, in); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("ambiguous ledger source", e)
	}
	in.OperationID = newDriverID(t)
	in.Profile = costProfile(car, 1, seedLedgerExpense(t, p, owner, car, 100), &amount, nil)
	if _, e := New(p, cloud.RequireDriverOwnedVehicle).SaveCostProfile(ctx, owner, in); !errors.Is(e, app.ErrDenied) {
		t.Fatal("missing ledger port", e)
	}
}

func TestDriverCostLedgerLockFencesConcurrentUpdate(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	ledgerID := seedLedgerExpense(t, p, owner, car, 12_345)
	amount := int64(12_345)
	locked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	port := func(ctx context.Context, tx pgx.Tx, owner, vehicle, source string, revision int64) (*int64, string, error) {
		value, status, e := ledgeradapters.RequireDriverLedgerCost(ctx, tx, owner, vehicle, source, revision)
		if status == "ok" {
			once.Do(func() { close(locked) })
			<-release
		}
		return value, status, e
	}
	in := app.SaveCostProfile{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Profile: costProfile(car, 1, ledgerID, &amount, nil)}
	done := make(chan error, 1)
	go func() {
		_, e := NewWithLedger(p, cloud.RequireDriverOwnedVehicle, port).SaveCostProfile(ctx, owner, in)
		done <- e
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("ledger port not reached")
	}
	updateStarted, updateDone := make(chan struct{}), make(chan error, 1)
	go func() {
		close(updateStarted)
		_, e := p.Exec(ctx, "UPDATE ledger_records SET revision=revision+1 WHERE account_id=$1 AND id=$2", owner, ledgerID)
		updateDone <- e
	}()
	<-updateStarted
	select {
	case e := <-updateDone:
		t.Fatal("ledger update bypassed the atomic link", e)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := <-updateDone; e != nil {
		t.Fatal(e)
	}
	if _, e := New(p, cloud.RequireDriverOwnedVehicle).GetCostProfile(ctx, owner, car, 1); e != nil {
		t.Fatal("profile missing after atomic link", e)
	}
	var revision int64
	if e := p.QueryRow(ctx, "SELECT revision FROM ledger_records WHERE account_id=$1 AND id=$2", owner, ledgerID).Scan(&revision); e != nil || revision != 2 {
		t.Fatal("ledger update not committed", revision, e)
	}
}

func maintenance(vehicle string, version, segment int64, interval, baseline, nextDue *int64) domain.MaintenanceSchedule {
	return domain.MaintenanceSchedule{
		ID: "11111111-1111-4111-8111-111111111111", Vehicle: vehicle, Version: version, Timezone: "America/Cuiaba",
		Kind: "OIL", Basis: "ODOMETER", EffectiveFrom: "2026-01-01", IntervalMetres: interval,
		OdometerSegment: segment, BaselineMetres: baseline, NextDueMetres: nextDue,
	}
}

func TestDriverMaintenanceVersionsZeroUnknownErasure(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	interval, zero := int64(10_000_000), int64(0)
	first := app.SaveMaintenance{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Schedule: maintenance(car, 1, 1, &interval, &zero, nil)}
	if receipt, e := s.SaveMaintenance(ctx, owner, first); e != nil || receipt.Version != 1 {
		t.Fatal("create", receipt, e)
	}
	if got, e := s.GetMaintenance(ctx, owner, "11111111-1111-4111-8111-111111111111", 1); e != nil || got.BaselineMetres == nil || *got.BaselineMetres != 0 || got.NextDueMetres != nil || got.OdometerSegment != 1 {
		t.Fatalf("v1 %+v %v", got, e)
	}
	second := app.SaveMaintenance{OperationID: newDriverID(t), ExpectedVersion: 1, ExpectedEpoch: 0, Schedule: maintenance(car, 2, 2, &interval, nil, &zero)}
	if receipt, e := s.SaveMaintenance(ctx, owner, second); e != nil || receipt.Version != 2 {
		t.Fatal("edit", receipt, e)
	}
	if got, e := s.GetMaintenance(ctx, owner, "11111111-1111-4111-8111-111111111111", 2); e != nil || got.BaselineMetres != nil || got.NextDueMetres == nil || *got.NextDueMetres != 0 || got.OdometerSegment != 2 {
		t.Fatalf("v2 %+v %v", got, e)
	}
	if list, e := s.ListMaintenance(ctx, owner, car, 0, 10); e != nil || len(list) != 2 {
		t.Fatal("list", list, e)
	}
	if replay, e := s.SaveMaintenance(ctx, owner, second); e != nil || !replay.Replayed {
		t.Fatal("replay", replay, e)
	}
	if e := s.EraseMaintenance(ctx, owner, "11111111-1111-4111-8111-111111111111", 2, 0); e != nil {
		t.Fatal(e)
	}
	second.OperationID = newDriverID(t)
	if _, e := s.SaveMaintenance(ctx, owner, second); !errors.Is(e, app.ErrErased) {
		t.Fatal("erased schedule recreated", e)
	}
	if e := s.EraseMaintenanceOwner(ctx, owner, 0); e != nil {
		t.Fatal(e)
	}
	first.OperationID = newDriverID(t)
	if _, e := s.SaveMaintenance(ctx, owner, first); !errors.Is(e, app.ErrErased) {
		t.Fatal("old maintenance epoch accepted", e)
	}
	if list, e := s.ListMaintenance(ctx, owner, car, 0, 10); e != nil || len(list) != 0 {
		t.Fatal("owner erase left schedules", list, e)
	}
}

func TestDriverCostAndMaintenanceSoftDeleteCleanup(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	ledgerID := seedLedgerExpense(t, p, owner, car, 100)
	amount := int64(100)
	s := NewWithLedger(p, cloud.RequireDriverOwnedVehicle, ledgeradapters.RequireDriverLedgerCost)
	in := app.SaveCostProfile{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Profile: costProfile(car, 1, ledgerID, &amount, nil)}
	if _, e := s.SaveCostProfile(ctx, owner, in); e != nil {
		t.Fatal(e)
	}
	interval := int64(1_000)
	schedule := app.SaveMaintenance{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Schedule: maintenance(car, 1, 1, &interval, nil, nil)}
	if _, e := s.SaveMaintenance(ctx, owner, schedule); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", owner); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"driver_cost_profiles", "driver_cost_profile_items", "driver_cost_commands", "driver_cost_tombstones", "driver_maintenance_schedules", "driver_maintenance_commands", "driver_maintenance_tombstones"} {
		if driverCount(t, p, table) != 0 {
			t.Fatal("soft deletion left cost data", table)
		}
	}
}
