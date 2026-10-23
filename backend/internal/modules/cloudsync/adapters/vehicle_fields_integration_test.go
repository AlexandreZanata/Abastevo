//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"testing"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func mhevVehicle() domain.Vehicle {
	propulsion := domain.PropulsionMHEV
	return domain.Vehicle{
		ID: "aaaaaaaa-1111-4111-8111-000000000010", Name: "Mild",
		CapacityMilli: 45000, Fuel: kernel.GasolineRegular, Unit: kernel.Liter,
		PriceSource: "CHEAPEST_STATION", SortOrder: 1, Propulsion: &propulsion,
	}
}

func TestVehicleMHEVAndUnknownFieldPreservation(t *testing.T) {
	pool := freshDB(t)
	ctx := context.Background()
	owner := seedOwner(t, pool)
	store := paidStore(t, pool, owner)

	vehicle := mhevVehicle()
	record, err := store.PutVehicle(ctx, owner, vehicle, 0)
	if err != nil {
		t.Fatal("MHEV admission refused", err)
	}
	stored := fetchVehicle(t, pool, owner.AccountID, vehicle.ID)
	if stored.Propulsion == nil || *stored.Propulsion != domain.PropulsionMHEV {
		t.Fatal("MHEV not stored as its own kind", stored.Propulsion)
	}

	// Simulate a field written by a newer server/client, then a legacy
	// partial update that omits every energy/future field.
	if _, err := pool.Exec(ctx, `UPDATE cloud_records SET payload = payload || '{"future_energy":{"usable_wh":30000},"charge_connector":"TYPE2"}'::jsonb
		WHERE account_id=$1 AND dataset='vehicles' AND id=$2`, owner.AccountID, vehicle.ID); err != nil {
		t.Fatal(err)
	}
	legacy := vehicle
	legacy.Name = "Mild renamed"
	legacy.Propulsion = nil
	if _, err := store.PutVehicle(ctx, owner, legacy, record.Revision); err != nil {
		t.Fatal("legacy update refused", err)
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM cloud_records WHERE account_id=$1 AND dataset='vehicles' AND id=$2`,
		owner.AccountID, vehicle.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["propulsion"]) != `"MHEV"` {
		t.Fatal("legacy update erased MHEV", string(raw))
	}
	if !jsonEqual(t, got["future_energy"], `{"usable_wh":30000}`) || !jsonEqual(t, got["charge_connector"], `"TYPE2"`) {
		t.Fatal("legacy update erased unknown newer fields", string(raw))
	}

	// The sync Apply path (not only PutVehicle) preserves them too.
	owner = readyDevice(t, store, owner)
	command := vehicleCommand(t, vehicle, record.Revision+1)
	command.Payload = []byte(`{"id":"` + vehicle.ID + `","display_name":"Apply renamed","capacity_milli":45000,"fuel_product":"GASOLINE_REGULAR","unit":"L","price_source":"CHEAPEST_STATION","price_drop_alert_enabled":false,"sort_order":1,"archived":false}`)
	batch := application.Batch{Epoch: 1, ConsentRevision: 1, Commands: []application.Command{command}}
	if _, err := store.Apply(ctx, owner, batch); err != nil {
		t.Fatal("apply refused", err)
	}
	if err := pool.QueryRow(ctx, `SELECT payload FROM cloud_records WHERE account_id=$1 AND dataset='vehicles' AND id=$2`,
		owner.AccountID, vehicle.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(t, got["future_energy"], `{"usable_wh":30000}`) || !jsonEqual(t, got["charge_connector"], `"TYPE2"`) {
		t.Fatal("sync apply erased unknown newer fields", string(raw))
	}
}

func jsonEqual(t *testing.T, raw json.RawMessage, want string) bool {
	t.Helper()
	var a, b any
	if json.Unmarshal(raw, &a) != nil || json.Unmarshal([]byte(want), &b) != nil {
		return false
	}
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
