//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/jackc/pgx/v5"
)

func denyPaid() func(context.Context, pgx.Tx, string) error {
	return func(context.Context, pgx.Tx, string) error { return errDenied() }
}

func TestLedgerConsentSeparateAndRevoke(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ctx := context.Background()
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{refuelCommand(t, newid(t), vehicle, 0)}}); err == nil {
		t.Fatal("upload without opt-in admitted")
	}
	status, err := store.SetConsent(ctx, owner, []string{"refuel"}, true)
	if err != nil || len(status.Datasets) != 1 {
		t.Fatalf("opt-in %+v %v", status, err)
	}
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{refuelCommand(t, newid(t), vehicle, 0)}}); err != nil {
		t.Fatal("opted-in upload refused", err)
	}
	if _, err = store.SetConsent(ctx, owner, []string{"refuel"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{refuelCommand(t, newid(t), vehicle, 0)}}); err == nil {
		t.Fatal("revoked upload admitted")
	}
	rows, err := store.List(ctx, owner, "refuel", "", "", "", "", "", "", 100)
	if err != nil || len(rows) == 0 {
		t.Fatal("revoke hid held data", err)
	}
	if _, err = store.SetConsent(ctx, owner, []string{"refuel", "refuel"}, true); err == nil {
		t.Fatal("duplicate consent admitted")
	}
	if _, err = store.SetConsent(ctx, owner, []string{"professional"}, true); err == nil {
		t.Fatal("unregistered dataset admitted")
	}
}

func TestLedgerTwoDeviceConflictDeleteReplay(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ensureConsent(t, store, owner)
	ctx := context.Background()
	id := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{refuelCommand(t, id, vehicle, 0)}}); err != nil {
		t.Fatal(err)
	}
	stale := refuelCommand(t, id, vehicle, 1)
	stale.Payload, _ = json.Marshal(map[string]any{"id": id, "vehicle_id": vehicle, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 25000, "total_centavos": 15000, "fill": "full"})
	stale.OperationID = newid(t)
	current := refuelCommand(t, id, vehicle, 1)
	current.OperationID = newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{current}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{stale}}); err == nil {
		t.Fatal("stale device overwrote")
	}
	delID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{expenseCommand(t, delID, vehicle, 0)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Delete(ctx, owner, "expense", delID); err != nil {
		t.Fatal("delete", err)
	}
	recreate := expenseCommand(t, delID, vehicle, 0)
	recreate.OperationID = newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{recreate}}); err == nil {
		t.Fatal("deleted record resurrected")
	}
	replay := refuelCommand(t, newid(t), vehicle, 0)
	op := replay.OperationID
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{replay}}); err != nil {
		t.Fatal(err)
	}
	again, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{replay}})
	if err != nil || !again.Results[0].Replayed {
		t.Fatal("offline replay", err)
	}
	_ = op
	_ = errors.Is
}

func TestLedgerStaleCascadeExpiryForeign(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	foreign := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ensureConsent(t, store, owner)
	ensureConsent(t, store, foreign)
	ctx := context.Background()
	id := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{refuelCommand(t, id, vehicle, 0)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Delete(ctx, owner, "refuel", id); err != nil {
		t.Fatal(err)
	}
	stale := refuelCommand(t, id, vehicle, 0)
	stale.OperationID = newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{stale}}); err == nil {
		t.Fatal("stale restore resurrected deleted")
	}
	if _, err := pool.Exec(ctx, "UPDATE cloud_records SET payload='{\"archived\":true}' WHERE account_id=$1 AND dataset='vehicles' AND id=$2", owner, vehicle); err != nil {
		t.Fatal(err)
	}
	var ledgerCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM ledger_records WHERE account_id=$1", owner).Scan(&ledgerCount); err != nil || ledgerCount == 0 {
		t.Fatal("archive cascaded history")
	}
	store.Paid = denyPaid()
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{refuelCommand(t, newid(t), vehicle, 0)}}); err == nil {
		t.Fatal("expired upload admitted")
	}
	rows, err := store.List(ctx, owner, "refuel", "", "", "", "", "", "", 100)
	if err != nil {
		t.Fatal("expired read refused", err)
	}
	_ = rows
	delID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{expenseCommand(t, delID, vehicle, 0)}}); err == nil {
		t.Fatal("expired write admitted with paid deny")
	}
	store.Paid = nil
	foreignVehicle := newVehicle(t, pool, foreign)
	foreignCmd := refuelCommand(t, newid(t), foreignVehicle, 0)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{foreignCmd}}); err == nil {
		t.Fatal("foreign reference admitted")
	}
	if err := store.EraseAll(ctx, owner); err != nil {
		t.Fatal(err)
	}
	erased, err := store.List(ctx, owner, "refuel", "", "", "", "", "", "", 100)
	if err != nil || len(erased) != 0 {
		t.Fatalf("erasure inventory %+v %v", erased, err)
	}
}
