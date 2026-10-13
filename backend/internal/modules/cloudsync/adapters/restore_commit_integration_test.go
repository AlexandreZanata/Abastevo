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

func commitSnapshot(t *testing.T, store *Store, p app.Principal, revision int64, records []restoreRecord) (string, app.RestorePreview) {
	t.Helper()
	ctx := context.Background()
	manifest, err := store.Manifest(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	_ = revision
	data := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, records)
	snapshotID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data); err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewRestore(ctx, p, snapshotID)
	if err != nil || !preview.Compatible {
		t.Fatalf("preview %+v %v", preview, err)
	}
	return snapshotID, preview
}

func TestRestoreCommitDoubleConfirmAndStale(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	store.Objects = newFakeObjects()
	v := vehicle(t)
	other := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	manifest, _ := store.Manifest(ctx, p)
	records := []restoreRecord{{Dataset: "vehicles", ID: other.ID, Revision: 1, Payload: restorePayload(t, other)}}
	data := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, records)
	snapshotID := newid(t)
	if _, err := store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data); err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewRestore(ctx, p, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	restoreID := newid(t)
	first, err := store.RestoreCommit(ctx, p, snapshotID, restoreID, preview.CurrentRevision, preview.ExpiresAt)
	if err != nil || first.PostRevision == first.PreRevision {
		t.Fatalf("commit %+v %v", first, err)
	}
	before := counts(t, pool)
	again, err := store.RestoreCommit(ctx, p, snapshotID, restoreID, preview.CurrentRevision, preview.ExpiresAt)
	if err != nil || !again.Replayed || again.PostRevision != first.PostRevision {
		t.Fatalf("double confirm %+v %v", again, err)
	}
	if after := counts(t, pool); after != before {
		t.Fatal("duplicate restore effects")
	}
	changed := v
	changed.Name = "Intervening edit"
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, changed, 1)}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RestoreCommit(ctx, p, snapshotID, newid(t), preview.CurrentRevision, preview.ExpiresAt); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("stale preview committed")
	}
	if _, err = store.RestoreCommit(ctx, p, snapshotID, newid(t), preview.CurrentRevision, time.Now().Add(-time.Minute).Unix()); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("expired preview committed")
	}
	var receipt string
	if err = pool.QueryRow(ctx, "SELECT receipt::text FROM cloud_restores WHERE account_id=$1 AND restore_id=$2", p.AccountID, restoreID).Scan(&receipt); err != nil || len(receipt) == 0 {
		t.Fatal("receipt", err)
	}
	var decoded map[string]any
	if err = json.Unmarshal([]byte(receipt), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["restore_id"]; !ok {
		t.Fatal("receipt metadata")
	}
	receiptText := receipt
	for _, forbidden := range []string{"password", "credential"} {
		if len(receiptText) > 0 && containsFold(receiptText, forbidden) {
			t.Fatal("receipt leaked private content")
		}
	}
}

func TestRestoreCommitCrashConcurrentRevokeExpiry(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	other := secondDevice(t, pool, store, p)
	ctx := context.Background()
	store.Objects = newFakeObjects()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	manifest, _ := store.Manifest(ctx, p)
	fresh := vehicle(t)
	records := []restoreRecord{{Dataset: "vehicles", ID: fresh.ID, Revision: 1, Payload: restorePayload(t, fresh)}}
	data := buildRestoreData(t, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, records)
	snapshotID := newid(t)
	if _, err := store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data); err != nil {
		t.Fatal(err)
	}
	preview, _ := store.PreviewRestore(ctx, p, snapshotID)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_restore_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic'; END $$; CREATE TRIGGER reject_restore_receipt BEFORE INSERT ON cloud_restores FOR EACH ROW EXECUTE FUNCTION reject_restore_receipt()`); err != nil {
		t.Fatal(err)
	}
	before := counts(t, pool)
	if _, err := store.RestoreCommit(ctx, p, snapshotID, newid(t), preview.CurrentRevision, preview.ExpiresAt); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("failed rollback reported success", err)
	}
	if after := counts(t, pool); after != before {
		t.Fatal("failed restore left effects")
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER reject_restore_receipt ON cloud_restores"); err != nil {
		t.Fatal(err)
	}
	conflicting := v
	conflicting.Name = "Concurrent edit"
	if _, err := store.Apply(ctx, other, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, conflicting, 1)}}); err != nil {
		t.Fatal(err)
	}
	stalePreview, _ := store.PreviewRestore(ctx, p, snapshotID)
	_ = stalePreview
	if _, err := store.RestoreCommit(ctx, p, snapshotID, newid(t), preview.CurrentRevision, preview.ExpiresAt); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("concurrent edit committed")
	}
	freshPreview, _ := store.PreviewRestore(ctx, p, snapshotID)
	if err := store.RevokeDevice(ctx, p, p.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RestoreCommit(ctx, p, snapshotID, newid(t), freshPreview.CurrentRevision, freshPreview.ExpiresAt); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("revoked restore admitted %v", err)
	}
	p = secondDevice(t, pool, store, p)
	freshPreview, _ = store.PreviewRestore(ctx, p, snapshotID)
	store.Paid = nil
	if _, err := store.RestoreCommit(ctx, p, snapshotID, newid(t), freshPreview.CurrentRevision, freshPreview.ExpiresAt); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("expired restore admitted %v", err)
	}
}

func containsFold(haystack, needle string) bool {
	haystack = fold(haystack)
	needle = fold(needle)
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func fold(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}
