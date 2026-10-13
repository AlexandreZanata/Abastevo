//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
)

func odometerRefuel(t *testing.T, id, vehicleID string, expected int64, odometer int64) application.Command {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"id": id, "vehicle_id": vehicleID, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 20000, "total_centavos": 12400, "odometer_metres": odometer, "fill": "full"})
	return application.Command{OperationID: newid(t), Dataset: "refuel", ID: id, ExpectedRevision: expected, Payload: payload}
}

func TestOdometerRegressionIdenticalAndReset(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ctx := context.Background()
	first := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, first, vehicle, 0, 100000)}}); err != nil {
		t.Fatal(err)
	}
	identical := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, identical, vehicle, 0, 100000)}}); err != nil {
		t.Fatal("identical odometer refused", err)
	}
	regressed := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, regressed, vehicle, 0, 90000)}}); err == nil {
		t.Fatal("regression admitted")
	}
	segment, err := store.OpenSegment(ctx, owner, vehicle, "reset")
	if err != nil || segment != 2 {
		t.Fatalf("open segment %d %v", segment, err)
	}
	afterReset := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, afterReset, vehicle, 0, 5000)}}); err != nil {
		t.Fatal("reset fill refused", err)
	}
	follow := newid(t)
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, follow, vehicle, 0, 6000)}}); err != nil {
		t.Fatal("segment fill refused", err)
	}
	crossing, err := func() (string, error) {
		cmd := odometerRefuel(t, first, vehicle, 1, 4000)
		_, e := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{cmd}})
		return "", e
	}()
	if crossing == "" && err == nil {
		t.Fatal("cross-segment edit admitted")
	}
}

func TestMissingGapArchiveAndConcurrent(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ctx := context.Background()
	first := newid(t)
	second := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, first, vehicle, 0, 100000), odometerRefuel(t, second, vehicle, 0, 110000)}}); err != nil {
		t.Fatal(err)
	}
	gapID := newid(t)
	if _, err := store.DeclareGap(ctx, owner, vehicle, gapID, first, second, "missed receipt"); err != nil {
		t.Fatal("declare gap", err)
	}
	correction := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, correction, vehicle, 0, 120000)}}); err != nil {
		t.Fatal("gap correction refused", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE cloud_records SET payload='{\"archived\":true}' WHERE account_id=$1 AND dataset='vehicles' AND id=$2", owner, vehicle); err != nil {
		t.Fatal(err)
	}
	var ledgerCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM ledger_records WHERE account_id=$1", owner).Scan(&ledgerCount); err != nil || ledgerCount != 3 {
		t.Fatalf("archive cascaded history %d %v", ledgerCount, err)
	}
	afterArchive := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, afterArchive, vehicle, 0, 130000)}}); err != nil {
		t.Fatalf("archived history refused %v", err)
	}
	target := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{odometerRefuel(t, target, vehicle, 0, 140000)}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := odometerRefuel(t, target, vehicle, 1, 150000)
			cmd.OperationID = newid(t)
			_, e := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{cmd}})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	wins := 0
	for e := range errs {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent odometer edit %d", wins)
	}
	if _, err := store.OpenSegment(ctx, owner, newid(t), "reset"); err == nil {
		t.Fatal("foreign segment admitted")
	}
	if _, err := store.DeclareGap(ctx, owner, vehicle, newid(t), newid(t), "", "bad"); err == nil {
		t.Fatal("dangling gap admitted")
	}
}
