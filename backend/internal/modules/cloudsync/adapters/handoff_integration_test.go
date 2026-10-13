//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	ledgeradapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/adapters"
	ledgerapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	metricsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/adapters"
	metricshttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/adapters/http"
)

func TestHandoffStaleForgedEntitlementFailClosed(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	cloudStore := paidStore(t, pool, owner)
	deviceA := readyDevice(t, cloudStore, owner)
	owners := map[string]string{deviceA.FamilyID: owner.AccountID}
	keys := map[string][2]string{owner.AccountID: {owner.ContributorID, owner.KeyID}}
	cloudHandler := protocolHandler(t, cloudStore, owners, keys)
	staleBody := journeyBatchJSON("99999999-9999-4999-8999-999999999999", deviceA.DeviceID, 1, 1, vehicleCommand(t, vehicle(t), 0))
	if w := journeyCall(t, cloudHandler, "/v1/cloud/sync/push", staleBody); w.Code != 401 {
		t.Fatalf("stale session %d", w.Code)
	}
	forgedBody := journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, vehicle(t), 0))
	forgedBody = strings.Replace(forgedBody, `"access_token":"synthetic"`, `"access_token":"forged-token"`, 1)
	if w := journeyCall(t, cloudHandler, "/v1/cloud/sync/push", forgedBody); w.Code != 401 {
		t.Fatalf("forged token %d", w.Code)
	}
	nilHandler := protocolHandler(t, cloudStore, owners, keys)
	nilHandler.Service = nil
	if w := journeyCall(t, nilHandler, "/v1/cloud/sync/push", journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, vehicle(t), 0))); w.Code != 503 {
		t.Fatalf("nil service %d", w.Code)
	}
	secretBody := journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, vehicle(t), 0))
	secretBody = strings.Replace(secretBody, "synthetic", "synthetic-secret-value", 1)
	w := journeyCall(t, cloudHandler, "/v1/cloud/sync/push", secretBody)
	if w.Code != 401 || strings.Contains(w.Body.String(), "synthetic-secret-value") {
		t.Fatalf("secret echo %d %s", w.Code, w.Body.String())
	}
}

func TestHandoffCrossOwnerCompareNoLeak(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	foreignOwner := seedOwner(t, pool)
	cloudStore := paidStore(t, pool, owner)
	deviceA := readyDevice(t, cloudStore, owner)
	owners := map[string]string{deviceA.FamilyID: owner.AccountID}
	ledgerStore := ledgeradapters.NewStore(pool)
	if _, err := ledgerStore.SetConsent(context.Background(), owner.AccountID, []string{"refuel", "expense"}, true); err != nil {
		t.Fatal(err)
	}
	vehicleAID := newid(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,'{}')", owner.AccountID, vehicleAID); err != nil {
		t.Fatal(err)
	}
	foreignVehicle := newid(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,'{}')", foreignOwner.AccountID, foreignVehicle); err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerStore.SetConsent(context.Background(), foreignOwner.AccountID, []string{"refuel", "expense"}, true); err != nil {
		t.Fatal(err)
	}
	refuel := func(vehicle, id string, odo int64, occurred string, total int64) ledgerapp.Command {
		raw, _ := json.Marshal(map[string]any{"id": id, "vehicle_id": vehicle, "occurred_at": occurred, "timezone": "UTC", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 20000, "total_centavos": total, "odometer_metres": odo, "fill": "full"})
		return ledgerapp.Command{OperationID: newid(t), Dataset: "refuel", ID: id, Payload: raw}
	}
	if _, err := ledgerStore.Apply(context.Background(), owner.AccountID, ledgerapp.Batch{Commands: []ledgerapp.Command{refuel(vehicleAID, newid(t), 100000, "2026-03-15T10:30:00Z", 12400), refuel(vehicleAID, newid(t), 140000, "2026-03-15T11:30:00Z", 12400)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerStore.Apply(context.Background(), foreignOwner.AccountID, ledgerapp.Batch{Commands: []ledgerapp.Command{refuel(foreignVehicle, newid(t), 100000, "2026-03-15T10:30:00Z", 99000), refuel(foreignVehicle, newid(t), 140000, "2026-03-15T11:30:00Z", 99000)}}); err != nil {
		t.Fatal(err)
	}
	metricsHandler := metricshttp.Handler{Service: metricsadapters.NewStore(journeyLedgerReader{store: ledgerStore}), Sessions: journeySessions(owners), SignedOwner: journeySigned}
	foreignBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","baseline":{"vehicle_id":"` + foreignVehicle + `","from":"2026-03-01T00:00:00Z","to":"2026-04-01T00:00:00Z","timezone":"UTC","method":"fuel-purchase-interval"},"current":{"vehicle_id":"` + vehicleAID + `","from":"2026-03-01T00:00:00Z","to":"2026-04-01T00:00:00Z","timezone":"UTC","method":"fuel-purchase-interval"}}`
	w := journeyCall(t, metricsHandler, "/v1/metrics/compare", foreignBody)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"compatible":false`) || strings.Contains(w.Body.String(), "99000") {
		t.Fatalf("foreign compare %d %s", w.Code, w.Body.String())
	}
}
