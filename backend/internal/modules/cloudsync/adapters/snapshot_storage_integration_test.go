//go:build integration

package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
)

type fakeObjects struct {
	mu              sync.Mutex
	data            map[string][]byte
	failPut         bool
	lostPutResponse bool
	failHead        bool
	failGet         bool
	corruptGet      bool
	failDelete      bool
	puts            int
}

func newFakeObjects() *fakeObjects {
	return &fakeObjects{data: map[string][]byte{}}
}

func (f *fakeObjects) Put(_ context.Context, key string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts++
	if f.failPut {
		return domain.ErrUnavailable
	}
	f.data[key] = append([]byte(nil), data...)
	if f.lostPutResponse {
		return domain.ErrUnavailable
	}
	return nil
}

func (f *fakeObjects) Head(_ context.Context, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failHead {
		return 0, domain.ErrUnavailable
	}
	data, ok := f.data[key]
	if !ok {
		return 0, domain.ErrNotFound
	}
	return int64(len(data)), nil
}

func (f *fakeObjects) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet {
		return nil, domain.ErrUnavailable
	}
	data, ok := f.data[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if f.corruptGet {
		corrupt := append([]byte(nil), data...)
		if len(corrupt) > 0 {
			corrupt[0] ^= 0xff
		}
		return corrupt, nil
	}
	return append([]byte(nil), data...), nil
}

func (f *fakeObjects) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDelete {
		return domain.ErrUnavailable
	}
	delete(f.data, key)
	return nil
}

func snapshotData(t *testing.T, revision int64) []byte {
	t.Helper()
	return []byte(`{"version":1,"revision":` + itoaTest(revision) + `,"datasets":["vehicles"]}`)
}

func itoaTest(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestSnapshotStorageMissingKeyAndBounds(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	store.Objects = nil
	if _, err := store.StoreSnapshot(ctx, p, newid(t), 0, 1, 1, snapshotData(t, 0)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("missing storage admitted")
	}
	store.Objects = newFakeObjects()
	for _, bad := range [][]byte{nil, {}, []byte("not-json"), make([]byte, snapshotMaxBytes+1)} {
		if _, err := store.StoreSnapshot(ctx, p, newid(t), 0, 1, 1, bad); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("oversize snapshot admitted %d", len(bad))
		}
	}
	if _, err := store.StoreSnapshot(ctx, p, "not-a-uuid", 0, 1, 1, snapshotData(t, 0)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("bad snapshot ID admitted")
	}
}

func TestSnapshotStorageCorruptLostCrashDuplicateLate(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	fake := newFakeObjects()
	store.Objects = fake
	manifest, err := store.Manifest(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	data := snapshotData(t, manifest.Revision)
	snapshotID := newid(t)
	got, err := store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, data)
	if err != nil || got.Status != "READY" {
		t.Fatalf("store %v %+v", err, got)
	}
	sum := sha256.Sum256(data)
	if got.Checksum != hex.EncodeToString(sum[:]) || got.ByteSize != int64(len(data)) {
		t.Fatal("receipt integrity")
	}
	if !stringsContains(got.ObjectKey, "snapshots/") || !stringsContains(got.ObjectKey, p.AccountID) {
		t.Fatalf("object key namespace %q", got.ObjectKey)
	}
	listed, err := store.ListSnapshots(ctx, p, 100)
	if err != nil || len(listed) != 1 {
		t.Fatal("inventory", err)
	}
	read, err := store.ReadSnapshot(ctx, p, snapshotID)
	if err != nil || string(read) != string(data) {
		t.Fatal("readback", err)
	}
	corruptID := newid(t)
	fake.corruptGet = true
	if _, err = store.StoreSnapshot(ctx, p, corruptID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, snapshotData(t, manifest.Revision)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("corrupt bytes marked ready")
	}
	fake.corruptGet = false
	var status string
	if err = pool.QueryRow(ctx, "SELECT status FROM cloud_snapshots WHERE account_id=$1 AND snapshot_id=$2", p.AccountID, corruptID).Scan(&status); err != nil || status != "FAILED" {
		t.Fatalf("orphan not tracked %q %v", status, err)
	}
	if _, err = store.ReadSnapshot(ctx, p, corruptID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("unverified snapshot selectable")
	}
	listed, _ = store.ListSnapshots(ctx, p, 100)
	if len(listed) != 1 {
		t.Fatal("failed snapshot listed")
	}
	lostID := newid(t)
	fake.lostPutResponse = true
	if _, err = store.StoreSnapshot(ctx, p, lostID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, snapshotData(t, manifest.Revision)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("lost PUT admitted")
	}
	fake.lostPutResponse = false
	retry, err := store.StoreSnapshot(ctx, p, lostID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, snapshotData(t, manifest.Revision))
	if err != nil || retry.Status != "READY" {
		t.Fatalf("lost PUT retry %v %+v", err, retry)
	}
	dupID := newid(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := store.StoreSnapshot(ctx, p, dupID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, snapshotData(t, manifest.Revision))
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	lateID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, lateID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, snapshotData(t, manifest.Revision)); err != nil {
		t.Fatal("late writer refused", err)
	}
	listed, _ = store.ListSnapshots(ctx, p, 100)
	if len(listed) != 4 {
		t.Fatalf("late writer overwrote %d", len(listed))
	}
	_ = app.StoredSnapshot{}
}

func TestSnapshotStorageCrossOwnerAndCrash(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	paidStore(t, pool, foreign)
	p = readyDevice(t, store, p)
	foreign = readyDevice(t, store, foreign)
	ctx := context.Background()
	store.Objects = newFakeObjects()
	manifest, err := store.Manifest(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	snapshotID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, snapshotID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, snapshotData(t, manifest.Revision)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ReadSnapshot(ctx, foreign, snapshotID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("cross-owner read leaked")
	}
	listed, err := store.ListSnapshots(ctx, foreign, 100)
	if err != nil || len(listed) != 0 {
		t.Fatal("cross-owner inventory leaked")
	}
	if _, err = pool.Exec(ctx, `CREATE FUNCTION reject_snapshot_ready() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='READY' THEN RAISE EXCEPTION 'synthetic'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_snapshot_ready BEFORE UPDATE ON cloud_snapshots FOR EACH ROW EXECUTE FUNCTION reject_snapshot_ready()`); err != nil {
		t.Fatal(err)
	}
	crashID := newid(t)
	if _, err = store.StoreSnapshot(ctx, p, crashID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, snapshotData(t, manifest.Revision)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("crash reported ready", err)
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER reject_snapshot_ready ON cloud_snapshots"); err != nil {
		t.Fatal(err)
	}
	retry, err := store.StoreSnapshot(ctx, p, crashID, manifest.Revision, manifest.Epoch, manifest.ConsentRevision, snapshotData(t, manifest.Revision))
	if err != nil || retry.Status != "READY" {
		t.Fatalf("crash retry %v %+v", err, retry)
	}
}
