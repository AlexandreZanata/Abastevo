//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	alertsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/adapters"
	alertsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	ledgeradapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/adapters"
	ledgerapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestBudgetsAbusiveInputsCapsAndFlood(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ledger := ledgeradapters.NewStore(pool)
	alerts := alertsadapters.NewStore(pool)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, application.Batch{Epoch: 1, ConsentRevision: 1, Commands: []application.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := ledger.List(ctx, p.AccountID, "refuel", "", "", "", "", "", "", 102); err == nil {
		t.Fatal("oversize page admitted")
	}
	if _, err := ledger.List(ctx, p.AccountID, "fuel", "", "", "", "", "", "", 10); err == nil {
		t.Fatal("unknown dataset admitted")
	}
	oversize, _ := json.Marshal(map[string]any{"id": newid(t), "vehicle_id": v.ID, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 20000, "total_centavos": 12400, "fill": "full", "note": string(make([]byte, 10000))})
	if _, err := ledger.Apply(ctx, p.AccountID, ledgerapp.Batch{Commands: []ledgerapp.Command{{OperationID: newid(t), Dataset: "refuel", ID: newid(t), Payload: oversize}}}); err == nil {
		t.Fatal("oversize batch admitted")
	}
	t.Logf("abusive refusal elapsed=%v", time.Since(start))
	devices := 0
	for i := 0; i < 5; i++ {
		family := newid(t)
		if _, err := pool.Exec(ctx, "INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at) VALUES($1,$2,'s','h','s','h',clock_timestamp()+interval '1 hour',clock_timestamp())", family, p.AccountID); err != nil {
			t.Fatal(err)
		}
		p2 := p
		p2.FamilyID = family
		if _, err := store.RegisterDevice(ctx, p2); err == nil {
			devices++
		}
	}
	if devices != 2 {
		t.Fatalf("device cap %d", devices)
	}
	station := newid(t)
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic station')", station); err != nil {
		t.Fatal(err)
	}
	rulePayload := func() json.RawMessage {
		raw, _ := json.Marshal(map[string]any{
			"predicates": []any{map[string]any{"kind": "threshold", "fuel": "ETHANOL", "unit": "L", "condition": "STANDARD", "source": "community", "threshold_milli": 6200}},
			"operator":   "AND", "target": map[string]any{"station_ids": []string{station}},
			"schedule": map[string]any{"days": []int{1}, "window_start": "08:00", "window_end": "20:00", "timezone": "UTC", "quiet_start": "00:00", "quiet_end": "00:00", "cooldown_hours": 0},
		})
		return raw
	}
	capped := 0
	for i := 0; i < 22; i++ {
		if _, err := alerts.Apply(ctx, p.AccountID, alertsapp.Batch{Commands: []alertsapp.Command{{OperationID: newid(t), RuleID: newid(t), Payload: rulePayload()}}}); err == nil {
			capped++
		}
	}
	if capped != 20 {
		t.Fatalf("rule cap %d", capped)
	}
	queue := jobs.NewQueue(pool)
	floodStart := time.Now()
	for i := 0; i < 200; i++ {
		if _, err := jobs.Enqueue(ctx, pool, "budget-probe", []byte(`{"n":1}`), "", 1, time.Now()); err != nil {
			t.Fatalf("queue flood %d %v", i, err)
		}
	}
	t.Logf("queue flood 200 elapsed=%v", time.Since(floodStart))
	claimed := 0
	for i := 0; i < 200; i++ {
		if _, err := queue.Claim(ctx, "budget-probe", "budget-worker", time.Minute); err != nil {
			break
		}
		claimed++
	}
	if claimed != 200 {
		t.Fatalf("queue drain %d", claimed)
	}
}

func TestBudgetsLargeHistoriesFreeIsolationErrors(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ledger := ledgeradapters.NewStore(pool)
	ctx := context.Background()
	if _, err := ledger.SetConsent(ctx, p.AccountID, []string{"refuel", "expense"}, true); err != nil {
		t.Fatal(err)
	}
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, application.Batch{Epoch: 1, ConsentRevision: 1, Commands: []application.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ledger_records(account_id,dataset,id,vehicle_id,occurred_at,payload) SELECT $1,'expense',md5(g::text)::uuid,$2,clock_timestamp(),'{"id":"x","vehicle_id":"y","occurred_at":"2026-01-01T00:00:00Z","timezone":"UTC","amount_centavos":100,"category":"tolls"}' FROM generate_series(1,10000)g`, p.AccountID, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE ledger_records"); err != nil {
		t.Fatal(err)
	}
	scanStart := time.Now()
	rows, err := ledger.List(ctx, p.AccountID, "expense", "", "", "", "", "", "", 100)
	if err != nil || len(rows) != 100 {
		t.Fatalf("large scan %+v %v", len(rows), err)
	}
	t.Logf("large history page elapsed=%v", time.Since(scanStart))
	var wg sync.WaitGroup
	errs := make(chan error, 60)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw, _ := json.Marshal(map[string]any{"id": newid(t), "vehicle_id": v.ID, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "amount_centavos": 100, "category": "tolls"})
			_, e := ledger.Apply(ctx, p.AccountID, ledgerapp.Batch{Commands: []ledgerapp.Command{{OperationID: newid(t), Dataset: "expense", ID: newid(t), Payload: raw}}})
			errs <- e
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := ledger.List(ctx, p.AccountID, "expense", "", "", "", "", "", "", 20)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("free isolation %v", e)
		}
	}
	invalid := 0
	for i := 0; i < 50; i++ {
		if _, err = ledger.List(ctx, p.AccountID, "fuel", "", "", "", "", "", "", 10); err != nil {
			invalid++
		}
	}
	if invalid != 50 {
		t.Fatal("error budget leaked")
	}
}
