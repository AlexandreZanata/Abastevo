//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
)

func restorePayload(t *testing.T, v domain.Vehicle) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := NormalizePayload("vehicles", v.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	return normalized
}

func buildRestoreData(t *testing.T, revision, epoch, consent int64, records []restoreRecord) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"version": 1, "revision": revision, "epoch": epoch, "consent_revision": consent, "records": records})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRestorePreviewForeignCorruptOversizeSchema(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	paidStore(t, pool, foreign)
	p = readyDevice(t, store, p)
	foreign = readyDevice(t, store, foreign)
	ctx := context.Background()
	fake := newFakeObjects()
	store.Objects = fake
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	manifest, err := store.Manifest(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	records := []restoreRecord{{Dataset: "vehicles", ID: v.ID, Revision: 1, Payload: restorePayload(t, v)}}
	data := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, records)
	snapshotID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data); err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewRestore(ctx, p, snapshotID)
	if err != nil || !preview.Compatible || preview.RecordCount != 1 || preview.ConflictCount != 0 {
		t.Fatalf("preview %+v %v", preview, err)
	}
	if preview.ExpiresAt <= time.Now().Unix() || preview.ExpiresAt > time.Now().Add(16*time.Minute).Unix() {
		t.Fatal("preview expiry")
	}
	if _, err = store.PreviewRestore(ctx, foreign, snapshotID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("foreign preview leaked")
	}
	key := snapshotKey(p.AccountID, manifest.Revision, snapshotID)
	fake.mu.Lock()
	original := append([]byte(nil), fake.data[key]...)
	fake.data[key] = []byte(`{"version":1,"revision":1,"corrupt":true}`)
	fake.mu.Unlock()
	if _, err = store.PreviewRestore(ctx, p, snapshotID); err == nil {
		t.Fatal("corrupt preview admitted")
	}
	fake.mu.Lock()
	fake.data[key] = original
	fake.mu.Unlock()
	oldData, _ := json.Marshal(map[string]any{"version": 0, "revision": manifest.Revision, "epoch": manifest.Epoch, "consent_revision": manifest.ConsentRevision, "records": []restoreRecord{}})
	oldID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, oldID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, oldData); err != nil {
		t.Fatal(err)
	}
	oldPreview, err := store.PreviewRestore(ctx, p, oldID)
	if err != nil || oldPreview.Compatible || len(oldPreview.Issues) == 0 {
		t.Fatalf("old schema preview %+v %v", oldPreview, err)
	}
	if _, err = store.StoreSnapshot(ctx, p, newid(t), manifest.Revision, manifest.Epoch, manifest.ConsentRevision, make([]byte, snapshotMaxBytes+1)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("oversize snapshot admitted")
	}
}

func TestRestorePreviewDeletedMaliciousDuplicateStaleCancel(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	fake := newFakeObjects()
	store.Objects = fake
	v := vehicle(t)
	other := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0), vehicleCommand(t, other, 0)}}); err != nil {
		t.Fatal(err)
	}
	manifest, _ := store.Manifest(ctx, p)
	records := []restoreRecord{
		{Dataset: "vehicles", ID: v.ID, Revision: 1, Payload: restorePayload(t, v)},
		{Dataset: "vehicles", ID: other.ID, Revision: 1, Deleted: true},
	}
	data := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, records)
	deletedID := newid(t)
	if _, err := store.StoreSnapshot(ctx, p, deletedID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data); err != nil {
		t.Fatal(err)
	}
	deletedPreview, err := store.PreviewRestore(ctx, p, deletedID)
	if err != nil || !deletedPreview.Compatible || deletedPreview.ExcludedCount != 1 || deletedPreview.RecordCount != 1 {
		t.Fatalf("deleted preview %+v %v", deletedPreview, err)
	}
	malicious := []restoreRecord{{Dataset: "vehicles", ID: v.ID, Revision: 1, Payload: json.RawMessage(`{"id":"` + v.ID + `","display_name":"Evil","capacity_milli":50000,"fuel_product":"ETHANOL","unit":"L","price_source":"CHEAPEST_STATION","password":"secret"}`)}}
	malData := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, malicious)
	malID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, malID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, malData); err != nil {
		t.Fatal(err)
	}
	malPreview, err := store.PreviewRestore(ctx, p, malID)
	if err != nil || malPreview.Compatible {
		t.Fatalf("malicious preview %+v %v", malPreview, err)
	}
	dup := []restoreRecord{
		{Dataset: "vehicles", ID: v.ID, Revision: 1, Payload: restorePayload(t, v)},
		{Dataset: "vehicles", ID: v.ID, Revision: 1, Payload: restorePayload(t, v)},
	}
	dupData := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, dup)
	dupID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, dupID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, dupData); err != nil {
		t.Fatal(err)
	}
	dupPreview, err := store.PreviewRestore(ctx, p, dupID)
	if err != nil || dupPreview.Compatible || !stringsContains(stringsJoin(dupPreview.Issues, ","), "duplicate") {
		t.Fatalf("duplicate preview %+v %v", dupPreview, err)
	}
	before, err := store.PreviewRestore(ctx, p, deletedID)
	if err != nil || before.ConflictCount != 0 {
		t.Fatalf("initial conflicts %+v %v", before, err)
	}
	changed := v
	changed.Name = "Intervening edit"
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, changed, 1)}}); err != nil {
		t.Fatal(err)
	}
	after, err := store.PreviewRestore(ctx, p, deletedID)
	if err != nil || after.ConflictCount == 0 || after.CurrentRevision == before.CurrentRevision {
		t.Fatalf("stale preview %+v %v", after, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	beforeCounts := counts(t, pool)
	if _, err = store.PreviewRestore(cancelled, p, deletedID); err == nil {
		t.Fatal("cancelled preview admitted")
	}
	if afterCounts := counts(t, pool); afterCounts != beforeCounts {
		t.Fatal("cancelled preview wrote")
	}
}

func stringsJoin(parts []string, sep string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += sep
		}
		out += part
	}
	return out
}
