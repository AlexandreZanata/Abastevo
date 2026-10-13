//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	alertsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/adapters"
	alertsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	dirread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	ledgeradapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/adapters"
	ledgerapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
)

type campaignObjects struct {
	mu         sync.Mutex
	data       map[string][]byte
	failDelete bool
}

func newCampaignObjects() *campaignObjects { return &campaignObjects{data: map[string][]byte{}} }

func (f *campaignObjects) Put(_ context.Context, key string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = append([]byte(nil), data...)
	return nil
}

func (f *campaignObjects) Head(_ context.Context, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.data[key]
	if !ok {
		return 0, errors.New("campaign: service unavailable")
	}
	return int64(len(data)), nil
}

func (f *campaignObjects) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.data[key]
	if !ok {
		return nil, errors.New("campaign: service unavailable")
	}
	return append([]byte(nil), data...), nil
}

func (f *campaignObjects) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDelete {
		return errors.New("campaign: service unavailable")
	}
	delete(f.data, key)
	return nil
}

func TestPrivacyCampaignDeleteDuringActivityAndRestore(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	store.Station = dirread.NewReader(pool).ActiveReference
	objects := newCampaignObjects()
	store.Objects = objects
	ledger := ledgeradapters.NewStore(pool)
	alerts := alertsadapters.NewStore(pool)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, application.Batch{Epoch: 1, ConsentRevision: 1, Commands: []application.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	refuelID := newid(t)
	refuelRaw, _ := json.Marshal(map[string]any{"id": refuelID, "vehicle_id": v.ID, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 20000, "total_centavos": 12400, "fill": "full"})
	if _, err := ledger.SetConsent(ctx, p.AccountID, []string{"refuel", "expense"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(ctx, p.AccountID, ledgerapp.Batch{Commands: []ledgerapp.Command{{OperationID: newid(t), Dataset: "refuel", ID: refuelID, Payload: refuelRaw}}}); err != nil {
		t.Fatal(err)
	}
	station := newid(t)
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic station')", station); err != nil {
		t.Fatal(err)
	}
	ruleID := newid(t)
	ruleRaw, _ := json.Marshal(map[string]any{
		"predicates": []any{map[string]any{"kind": "threshold", "fuel": "ETHANOL", "unit": "L", "condition": "STANDARD", "source": "community", "threshold_milli": 6200}},
		"operator":   "AND", "target": map[string]any{"station_ids": []string{station}},
		"schedule": map[string]any{"days": []int{0, 1, 2, 3, 4, 5, 6}, "window_start": "00:00", "window_end": "00:00", "timezone": "UTC", "quiet_start": "00:00", "quiet_end": "00:00", "cooldown_hours": 0},
	})
	if _, err := alerts.Apply(ctx, p.AccountID, alertsapp.Batch{Commands: []alertsapp.Command{{OperationID: newid(t), RuleID: ruleID, Payload: ruleRaw}}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	status, err := store.Status(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	snapshotData := []byte(`{"version":1,"revision":` + itoaCampaign(status.Revision) + `,"epoch":1,"consent_revision":1,"records":[]}`)
	snapshotID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, snapshotID, status.Revision, status.Epoch, status.ConsentRevision, snapshotData); err != nil {
		t.Fatal(err)
	}
	var purchasesBefore int
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM subscription_purchases WHERE account_id=$1", p.AccountID).Scan(&purchasesBefore); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	eraseErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = store.Pull(ctx, p, application.Position{Version: 1}, 10)
		_, _ = ledger.List(ctx, p.AccountID, "refuel", "", "", "", "", "", "", 100)
		_, _ = alerts.Evaluate(ctx, p.AccountID, ruleID, "", 100, now)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, e := store.Erase(ctx, p, "all", ""); e != nil {
			eraseErr <- e
			return
		}
		if e := ledger.EraseAll(ctx, p.AccountID); e != nil {
			eraseErr <- e
			return
		}
		eraseErr <- alerts.EraseAll(ctx, p.AccountID)
	}()
	wg.Wait()
	close(eraseErr)
	for e := range eraseErr {
		if e != nil {
			t.Fatalf("campaign erasure %v", e)
		}
	}
	var cloudRecords, ledgerRecords, rules, inbox, snapshots, purchasesAfter, revoked int
	if err = pool.QueryRow(ctx, "SELECT (SELECT COUNT(*) FROM cloud_records WHERE account_id=$1 AND NOT deleted),(SELECT COUNT(*) FROM ledger_records WHERE account_id=$1 AND NOT deleted),(SELECT COUNT(*) FROM alert_rules WHERE account_id=$1 AND NOT deleted),(SELECT COUNT(*) FROM alert_inbox WHERE account_id=$1),(SELECT COUNT(*) FROM cloud_snapshots WHERE account_id=$1),(SELECT COUNT(*) FROM subscription_purchases WHERE account_id=$1),(SELECT COUNT(*) FROM cloud_devices WHERE account_id=$1 AND revoked_at IS NOT NULL)", p.AccountID).Scan(&cloudRecords, &ledgerRecords, &rules, &inbox, &snapshots, &purchasesAfter, &revoked); err != nil {
		t.Fatal(err)
	}
	if cloudRecords != 0 || ledgerRecords != 0 || rules != 0 || inbox != 0 || snapshots != 0 {
		t.Fatalf("erasure inventory cloud=%d ledger=%d rules=%d inbox=%d snapshots=%d", cloudRecords, ledgerRecords, rules, inbox, snapshots)
	}
	if purchasesAfter != purchasesBefore {
		t.Fatalf("purchase metadata disturbed %d want %d", purchasesAfter, purchasesBefore)
	}
	if revoked == 0 {
		t.Fatal("devices not revoked")
	}
	objects.mu.Lock()
	remaining := len(objects.data)
	objects.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("physical objects retained %d", remaining)
	}
	if _, err = store.PreviewRestore(ctx, p, snapshotID); err == nil {
		t.Fatal("erased snapshot preview leaked")
	}
	if _, err = store.Pull(ctx, p, application.Position{Version: 1}, 10); err == nil {
		t.Fatal("stale device reconnected")
	}
}

func TestPrivacyCampaignOutageParallelDeadlines(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	objects := newCampaignObjects()
	store.Objects = objects
	ledger := ledgeradapters.NewStore(pool)
	alerts := alertsadapters.NewStore(pool)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, application.Batch{Epoch: 1, ConsentRevision: 1, Commands: []application.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	status, err := store.Status(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.StoreSnapshot(ctx, p, newid(t), status.Revision, status.Epoch, status.ConsentRevision, []byte(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	objects.failDelete = true
	if _, err = store.Erase(ctx, p, "all", ""); err == nil {
		t.Fatal("outage falsely completed")
	}
	var snapshots int
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM cloud_snapshots WHERE account_id=$1", p.AccountID).Scan(&snapshots); err != nil || snapshots == 0 {
		t.Fatalf("outage purged inventory %d %v", snapshots, err)
	}
	objects.mu.Lock()
	unconfirmed := len(objects.data)
	objects.mu.Unlock()
	if unconfirmed == 0 {
		t.Fatal("unconfirmed bytes lost")
	}
	objects.failDelete = false
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := store.Erase(ctx, p, "all", ""); e != nil {
				errs <- e
				return
			}
			if e := ledger.EraseAll(ctx, p.AccountID); e != nil {
				errs <- e
				return
			}
			errs <- alerts.EraseAll(ctx, p.AccountID)
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("parallel erasure %v", e)
		}
	}
	start := time.Now()
	if _, err = store.Erase(ctx, p, "all", ""); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Fatalf("deletion deadline %v", elapsed)
	}
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM cloud_snapshots WHERE account_id=$1", p.AccountID).Scan(&snapshots); err != nil || snapshots != 0 {
		t.Fatalf("physical deadline snapshots=%d %v", snapshots, err)
	}
}

func itoaCampaign(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
