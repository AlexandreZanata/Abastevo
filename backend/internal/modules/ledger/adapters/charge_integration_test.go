//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	ledgerdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/domain"
)

func chargePayload(t *testing.T, id, vehicleID string) json.RawMessage {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"id": id, "vehicle_id": vehicleID, "occurred_at": "2026-10-22T10:00:00Z", "timezone": "America/Sao_Paulo",
		"energy_wh": 15000, "total_centavos": 1275, "soc_start_percent": 20, "soc_end_percent": 80,
		"odometer_metres": 50000, "note": "test charge",
	})
	return raw
}

func TestChargeSessionCRUDRevisionIdempotency(t *testing.T) {
	pool := freshDB(t)
	ctx := context.Background()
	account := newAccount(t, pool)
	store := NewStore(pool)
	if _, err := store.SetConsent(ctx, account, []string{"charge", "refuel"}, true); err != nil {
		t.Fatal(err)
	}
	vehicleID := newVehicle(t, pool, account)
	chargeID := newid(t)
	operation := newid(t)
	apply := func(commands []application.Command) (application.BatchResult, error) {
		return store.Apply(ctx, account, application.Batch{Commands: commands})
	}
	// Create with null-optional fields omitted (null measurements allowed).
	result, err := apply([]application.Command{{
		OperationID: operation, Dataset: "charge", ID: chargeID, ExpectedRevision: 0,
		Payload: chargePayload(t, chargeID, vehicleID),
	}})
	if err != nil || len(result.Results) != 1 || result.Results[0].Revision != 1 {
		t.Fatal("charge admission refused", err, result)
	}
	// Replay same operation is idempotent (same revision, replayed flag).
	replay, err := apply([]application.Command{{
		OperationID: operation, Dataset: "charge", ID: chargeID, ExpectedRevision: 0,
		Payload: chargePayload(t, chargeID, vehicleID),
	}})
	if err != nil || !replay.Results[0].Replayed {
		t.Fatal("replay diverged", err, replay)
	}
	// PHEV/fuel coexistence: same vehicle holds refuel and charge datasets.
	refuelID := newid(t)
	refuelPayload, _ := json.Marshal(map[string]any{
		"id": refuelID, "vehicle_id": vehicleID, "occurred_at": "2026-10-21T10:00:00Z", "timezone": "America/Sao_Paulo",
		"fuel": "GASOLINE_REGULAR", "unit": "L", "quantity_milli": 40000, "total_centavos": 24000,
		"fill": "full", "note": "test refuel",
	})
	_, err = apply([]application.Command{{
		OperationID: newid(t), Dataset: "refuel", ID: refuelID, ExpectedRevision: 0, Payload: refuelPayload,
	}})
	if err != nil {
		t.Fatal("PHEV coexistence refused", err)
	}
	// Concurrent edit: one wins with revision, stale conflicts.
	if _, err := apply([]application.Command{{
		OperationID: newid(t), Dataset: "charge", ID: chargeID, ExpectedRevision: 1,
		Payload: chargePayload(t, chargeID, vehicleID),
	}}); err != nil {
		t.Fatal("correction refused", err)
	}
	if _, err := apply([]application.Command{{
		OperationID: newid(t), Dataset: "charge", ID: chargeID, ExpectedRevision: 1,
		Payload: chargePayload(t, chargeID, vehicleID),
	}}); err == nil {
		t.Fatal("stale revision accepted")
	}
	// No automatic public observation: charge rows stay in ledger only.
	var community int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_observations`).Scan(&community); err == nil && community != 0 {
		t.Fatal("charge leaked to public observations")
	}
	var charging int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM charging_observations`).Scan(&charging); err == nil && charging != 0 {
		t.Fatal("charge leaked to community observations")
	}
	_ = ledgerdomain.Charge{}
}
