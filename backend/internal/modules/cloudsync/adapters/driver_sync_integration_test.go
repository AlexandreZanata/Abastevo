//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	driverdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
)

func driverEarning(t *testing.T, vehicle string) (string, []byte) {
	t.Helper()
	id := newid(t)
	net := int64(45_000)
	r := driverdomain.EarningRecord{
		ID: id, VehicleID: vehicle, Platform: "UBER", SourceHash: strings.Repeat("a", 64),
		Coverage: "SUMMARY", Currency: "BRL",
		From: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC),
		Timezone: "America/Cuiaba", Money: driverdomain.Earnings{Basis: "INCLUSIVE_NET", Net: &net},
	}
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	return id, raw
}

func TestDriverDatasetSyncConflictForeignAndErasure(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	if _, e := pool.Exec(ctx, "UPDATE cloud_state SET consent_mask=15,consent_revision=1 WHERE account_id=$1", p.AccountID); e != nil {
		t.Fatal(e)
	}
	v := vehicle(t)
	if _, e := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); e != nil {
		t.Fatal(e)
	}
	id, payload := driverEarning(t, v.ID)
	cmd := app.Command{OperationID: newid(t), Dataset: "driver_earnings", ID: id, Payload: payload}
	batch := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{cmd}}
	if result, e := store.Apply(ctx, p, batch); e != nil || result.Results[0].Revision != 1 {
		t.Fatal("driver create", result, e)
	}
	d2, e := store.RegisterDevice(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	p2 := p
	p2.DeviceID = d2.ID
	feed, e := store.Pull(ctx, p2, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10)
	if e != nil || len(feed.Changes) != 2 || feed.Changes[1].Dataset != "driver_earnings" {
		t.Fatalf("driver pull %+v %v", feed, e)
	}
	if replay, e := store.Apply(ctx, p, batch); e != nil || !replay.Results[0].Replayed {
		t.Fatal("driver replay", replay, e)
	}
	stale := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{{OperationID: newid(t), Dataset: "driver_earnings", ID: id, ExpectedRevision: 0, Payload: payload}}}
	var conflict *app.ConflictError
	if _, e := store.Apply(ctx, p, stale); !errors.As(e, &conflict) || conflict.Conflict.CurrentRevision != 1 || conflict.Conflict.Erased {
		t.Fatal("driver conflict", e)
	}
	if rows, e := store.Snapshot(ctx, p2, app.SnapshotQuery{Dataset: "driver_earnings"}, 10); e != nil || len(rows.Records) != 1 || rows.Records[0].ID != id {
		t.Fatalf("driver snapshot %+v %v", rows, e)
	}
	store.Objects = newFakeObjects()
	manifest, e := store.Manifest(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	normalized, e := NormalizePayload("driver_earnings", id, payload)
	if e != nil {
		t.Fatal(e)
	}
	data := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, []restoreRecord{{Dataset: "driver_earnings", ID: id, Revision: 1, Payload: normalized}})
	snapshotID := newid(t)
	if _, e = store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data); e != nil {
		t.Fatal(e)
	}
	preview, e := store.PreviewRestore(ctx, p2, snapshotID)
	if e != nil || preview.RecordCount != 1 || preview.ConflictCount != 0 {
		t.Fatalf("driver restore preview %+v %v", preview, e)
	}
	if _, e = store.RestoreCommit(ctx, p2, snapshotID, newid(t), preview.CurrentRevision, preview.ExpiresAt); e != nil {
		t.Fatal("driver second-device restore", e)
	}
	bad, _ := json.Marshal(map[string]any{"id": id, "vehicle_id": v.ID, "gps_trace": []int{1}})
	if _, e := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{{OperationID: newid(t), Dataset: "driver_earnings", ID: newid(t), Payload: bad}}}); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("raw gps admitted", e)
	}
	foreign := seedOwner(t, pool)
	foreignStore := paidStore(t, pool, foreign)
	foreign = readyDevice(t, foreignStore, foreign)
	if _, e := pool.Exec(ctx, "UPDATE cloud_state SET consent_mask=15,consent_revision=1 WHERE account_id=$1", foreign.AccountID); e != nil {
		t.Fatal(e)
	}
	if rows, e := foreignStore.Snapshot(ctx, foreign, app.SnapshotQuery{Dataset: "driver_earnings"}, 10); e != nil || len(rows.Records) != 0 {
		t.Fatal("foreign driver snapshot", rows, e)
	}
	if _, e := foreignStore.PreviewRestore(ctx, foreign, snapshotID); e == nil {
		t.Fatal("foreign driver restore preview leaked")
	}
	epoch, e := store.Erase(ctx, p, "driver_earnings", "")
	if e != nil || epoch <= 1 {
		t.Fatal("driver erase", epoch, e)
	}
	if _, e := store.Pull(ctx, p2, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10); !errors.Is(e, domain.ErrResnapshot) {
		t.Fatal("old cursor survived erasure", e)
	}
	if _, e := store.PreviewRestore(ctx, p2, snapshotID); e == nil {
		t.Fatal("pre-erasure snapshot resurrected")
	}
	status, e := store.Status(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	status, e = store.SetConsent(ctx, p, []string{"driver_earnings"}, true)
	if e != nil {
		t.Fatal(e)
	}
	newID, newPayload := driverEarning(t, v.ID)
	if _, e := store.Apply(ctx, p, app.Batch{Epoch: status.Epoch, ConsentRevision: status.ConsentRevision, Commands: []app.Command{{OperationID: newid(t), Dataset: "driver_earnings", ID: newID, Payload: newPayload}}}); e != nil {
		t.Fatal("post-erase new data", e)
	}
	rows, e := store.Snapshot(ctx, p, app.SnapshotQuery{Dataset: "driver_earnings"}, 10)
	if e != nil || len(rows.Records) != 1 || rows.Records[0].ID != newID {
		t.Fatal("post-erase snapshot", rows, e)
	}
}
