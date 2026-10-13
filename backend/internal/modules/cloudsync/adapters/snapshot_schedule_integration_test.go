//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
)

func TestSnapshotScheduleRestartOfflineConsentLapsed(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	store.Objects = newFakeObjects()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan int, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, e := store.TickSnapshots(ctx, time.Now(), 25)
			results <- n
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	total := 0
	for n := range results {
		total += n
	}
	if total != 1 {
		t.Fatalf("restart duplicated snapshots %d", total)
	}
	status, err := store.SnapshotStatus(ctx, p)
	if err != nil || status.ReadyCount != 1 || status.UnprotectedChanges != 0 || !status.Enabled {
		t.Fatalf("status %+v %v", status, err)
	}
	updated := v
	updated.Name = "Unprotected change"
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, updated, 1)}}); err != nil {
		t.Fatal(err)
	}
	status, _ = store.SnapshotStatus(ctx, p)
	if status.UnprotectedChanges == 0 || status.LastRevision == 0 {
		t.Fatalf("unprotected not exposed %+v", status)
	}
	emptyOwner := seedOwner(t, pool)
	emptyStore := paidStore(t, pool, emptyOwner)
	emptyOwner = readyDevice(t, emptyStore, emptyOwner)
	emptyStore.Objects = store.Objects
	if n, err := emptyStore.TickSnapshots(ctx, time.Now(), 25); err != nil || n == 0 {
		t.Fatalf("offline unsynced %d %v", n, err)
	}
	if _, err = store.SetConsent(ctx, p, []string{"vehicles", "favorites", "preferences"}, false); err != nil {
		t.Fatal(err)
	}
	if n, err := store.TickSnapshots(ctx, time.Now(), 25); err != nil || n != 0 {
		t.Fatalf("no-consent tick %d %v", n, err)
	}
	store.Paid = nil
	if n, err := store.TickSnapshots(ctx, time.Now(), 25); err != nil || n != 0 {
		t.Fatalf("lapsed tick %d %v", n, err)
	}
}

func TestSnapshotScheduleFailureQuotaCancel(t *testing.T) {
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
	fake.failPut = true
	if n, err := store.TickSnapshots(ctx, time.Now(), 25); !errors.Is(err, domain.ErrUnavailable) || n != 0 {
		t.Fatalf("storage failure %d %v", n, err)
	}
	var pending int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM cloud_snapshots WHERE account_id=$1 AND status='PENDING'", p.AccountID).Scan(&pending); err != nil || pending == 0 {
		t.Fatalf("backlog not tracked %d %v", pending, err)
	}
	fake.failPut = false
	if n, err := store.TickSnapshots(ctx, time.Now(), 25); err != nil || n == 0 {
		t.Fatalf("backlog retry %d %v", n, err)
	}
	for i := 0; i < 7; i++ {
		updated := v
		updated.Name = itoaTest(int64(i))
		rev := int64(2 + i)
		if i == 0 {
			rev = 1
		}
		_ = rev
		if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, updated, int64(1+i))}}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.TickSnapshots(ctx, time.Now(), 25); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := store.ListSnapshots(ctx, p, 100)
	if err != nil || len(listed) != snapshotMaxCount {
		t.Fatalf("quota retention %d %v", len(listed), err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.TickSnapshots(cancelled, time.Now(), 25); err == nil {
		t.Fatal("cancelled upload admitted")
	}
	if _, err := store.TickSnapshots(ctx, time.Now(), 0); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unbounded tick admitted")
	}
}
