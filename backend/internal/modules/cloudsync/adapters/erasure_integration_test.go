//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
)

func TestErasureBlocksReuseAndPurgesObjects(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	fake := newFakeObjects()
	store.Objects = fake
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	manifest, _ := store.Manifest(ctx, p)
	records := []restoreRecord{{Dataset: "vehicles", ID: v.ID, Revision: 1, Payload: restorePayload(t, v)}}
	data := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, records)
	snapshotID := newid(t)
	if _, err := store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data); err != nil {
		t.Fatal(err)
	}
	preview, _ := store.PreviewRestore(ctx, p, snapshotID)
	restoreID := newid(t)
	if _, err := store.RestoreCommit(ctx, p, snapshotID, restoreID, preview.CurrentRevision, preview.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Erase(ctx, p, "all", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PreviewRestore(ctx, p, snapshotID); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("stale device preview leaked %v", err)
	}
	fresh := secondDevice(t, pool, store, p)
	if _, err := store.PreviewRestore(ctx, fresh, snapshotID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("erased snapshot preview leaked %v", err)
	}
	if _, err := store.ReadSnapshot(ctx, fresh, snapshotID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("erased snapshot readable")
	}
	listed, err := store.ListSnapshots(ctx, fresh, 100)
	if err != nil || len(listed) != 0 {
		t.Fatalf("erased inventory listed %d %v", len(listed), err)
	}
	fake.mu.Lock()
	objects := len(fake.data)
	fake.mu.Unlock()
	if objects != 0 {
		t.Fatalf("physical objects retained %d", objects)
	}
	if _, err = store.PutVehicle(ctx, fresh, v, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("erased vehicle resurrected")
	}
	if _, err = store.RestoreCommit(ctx, p, snapshotID, newid(t), preview.CurrentRevision, preview.ExpiresAt); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("stale device restore resurrected %v", err)
	}
	if _, err = store.RestoreCommit(ctx, fresh, snapshotID, newid(t), preview.CurrentRevision, preview.ExpiresAt); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("erased snapshot restore resurrected %v", err)
	}
	again, err := store.Erase(ctx, p, "all", "")
	if err != nil {
		t.Fatal("retry erasure", err)
	}
	_ = again
	raw, _ := json.Marshal(v)
	_ = raw
}

func TestErasureOutageRestoredBytesAndRetry(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	fake := newFakeObjects()
	store.Objects = fake
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	manifest, _ := store.Manifest(ctx, p)
	data := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, []restoreRecord{{Dataset: "vehicles", ID: v.ID, Revision: 1, Payload: restorePayload(t, v)}})
	snapshotID := newid(t)
	if _, err := store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data); err != nil {
		t.Fatal(err)
	}
	fake.failDelete = true
	if _, err := store.Erase(ctx, p, "all", ""); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("outage reported success %v", err)
	}
	listed, err := store.ListSnapshots(ctx, p, 100)
	if err != nil || len(listed) != 1 {
		t.Fatalf("outage purged inventory %d %v", len(listed), err)
	}
	fake.mu.Lock()
	unconfirmed := len(fake.data)
	fake.mu.Unlock()
	if unconfirmed == 0 {
		t.Fatal("unconfirmed bytes lost")
	}
	fake.failDelete = false
	if _, err = store.Erase(ctx, p, "all", ""); err != nil {
		t.Fatal("retry erasure", err)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,'{\"id\":\""+v.ID+"\",\"display_name\":\"Restored bytes\",\"capacity_milli\":50000,\"fuel_product\":\"ETHANOL\",\"unit\":\"L\",\"price_source\":\"CHEAPEST_STATION\",\"price_drop_alert_enabled\":false,\"sort_order\":0,\"archived\":false}') ON CONFLICT DO NOTHING", p.AccountID, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutVehicle(ctx, p, v, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("restored bytes resurrected erased UUID")
	}
	fresh := secondDevice(t, pool, store, p)
	_ = fresh
}
