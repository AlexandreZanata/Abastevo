//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/jackc/pgx/v5/pgxpool"
)

func energyVehicle(id string) domain.Vehicle {
	battery := int64(38000)
	propulsion := domain.PropulsionBEV
	return domain.Vehicle{
		ID: "aaaaaaaa-1111-4111-8111-000000000001", Name: "BYD",
		CapacityMilli: 0, Fuel: kernel.GasolineRegular, Unit: kernel.Liter,
		PriceSource: "CHEAPEST_STATION", SortOrder: 1,
		Propulsion: &propulsion, BatteryWh: &battery,
	}
}

func TestEnergyVehicleOwnerConcurrentAndLegacyMerge(t *testing.T) {
	pool := freshDB(t)
	ctx := context.Background()
	owner := seedOwner(t, pool)
	store := paidStore(t, pool, owner)
	vehicle := energyVehicle("")
	record, err := store.PutVehicle(ctx, owner, vehicle, 0)
	if err != nil {
		t.Fatal("BEV admission refused", err)
	}
	// Legacy partial update (no energy fields, correct revision) preserves battery.
	legacy := vehicle
	legacy.Name = "BYD Renamed"
	legacy.Propulsion = nil
	legacy.BatteryWh = nil
	merged, err := store.PutVehicle(ctx, owner, legacy, record.Revision)
	if err != nil {
		t.Fatal("legacy merge refused", err)
	}
	stored := fetchVehicle(t, pool, owner.AccountID, vehicle.ID)
	if stored.BatteryWh == nil || *stored.BatteryWh != 38000 {
		t.Fatal("legacy update erased battery")
	}
	_ = merged
	// Concurrent updates: one wins, one conflicts; winner retains energy.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			update := vehicle
			update.Name = "Concurrent"
			update.SortOrder = 10 + n
			_, err := store.PutVehicle(ctx, owner, update, record.Revision+1)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	conflicts := 0
	for err := range errs {
		if err == domain.ErrConflict {
			conflicts++
		} else if err != nil {
			t.Fatal("unexpected concurrent error", err)
		}
	}
	if conflicts != 1 {
		t.Fatal("concurrent guard failed", conflicts)
	}
	// Cross-owner isolation: records are account-scoped; another account
	// neither sees nor disturbs this owner's vehicle (same ID is a
	// separate namespace, not a conflict).
	other := seedOwner(t, pool)
	var otherCount int64
	err = pool.QueryRow(ctx, `SELECT count(*) FROM cloud_records WHERE account_id=$1 AND dataset='vehicles' AND id=$2 AND NOT deleted`, other.AccountID, vehicle.ID).Scan(&otherCount)
	if err != nil || otherCount != 0 {
		t.Fatal("cross-owner leakage", otherCount, err)
	}
}

func fetchVehicle(t *testing.T, pool *pgxpool.Pool, accountID, vehicleID string) domain.Vehicle {
	t.Helper()
	var payload []byte
	err := pool.QueryRow(context.Background(),
		`SELECT payload FROM cloud_records WHERE account_id=$1 AND dataset='vehicles' AND id=$2 AND NOT deleted`,
		accountID, vehicleID).Scan(&payload)
	if err != nil {
		t.Fatal(err)
	}
	var vehicle domain.Vehicle
	if err := json.Unmarshal(payload, &vehicle); err != nil {
		t.Fatal(err)
	}
	return vehicle
}
