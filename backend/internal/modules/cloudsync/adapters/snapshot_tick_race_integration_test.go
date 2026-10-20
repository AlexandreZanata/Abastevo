//go:build integration

package adapters

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pausedTickObjects struct {
	*fakeObjects
	entered chan struct{}
	second  chan struct{}
	release chan struct{}
	puts    atomic.Int32
}

func (o *pausedTickObjects) Put(ctx context.Context, key string, data []byte) error {
	switch o.puts.Add(1) {
	case 1:
		close(o.entered)
	case 2:
		close(o.second)
	}
	select {
	case <-o.release:
		return o.fakeObjects.Put(ctx, key, data)
	case <-ctx.Done():
		return ctx.Err()
	}
}

type tickResult struct {
	count int
	err   error
}

func TestSnapshotConcurrentTickCannotAdmitSecondUploadWhileFirstIsPending(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	first := paidStore(t, pool, p)
	config := pool.Config()
	config.MaxConns = 1
	limited, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	first.Pool = limited
	p = readyDevice(t, first, p)
	objects := &pausedTickObjects{fakeObjects: newFakeObjects(), entered: make(chan struct{}), second: make(chan struct{}), release: make(chan struct{})}
	first.Objects = objects
	second := *first
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := first.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); err != nil {
		t.Fatal(err)
	}
	var release sync.Once
	unblock := func() { release.Do(func() { close(objects.release) }) }
	defer unblock()
	one, two := make(chan tickResult, 1), make(chan tickResult, 1)
	go func() { n, e := first.TickSnapshots(ctx, time.Now(), 25); one <- tickResult{n, e} }()
	select {
	case <-objects.entered:
	case <-ctx.Done():
		t.Fatal("first upload did not start")
	}
	go func() { n, e := second.TickSnapshots(ctx, time.Now(), 25); two <- tickResult{n, e} }()
	var result tickResult
	duplicate := false
	select {
	case result = <-two:
	case <-objects.second:
		duplicate = true
	case <-ctx.Done():
		t.Fatal("concurrent tick neither skipped nor reached upload")
	}
	unblock()
	if duplicate {
		result = <-two
	}
	firstResult := <-one
	if firstResult.err != nil || result.err != nil || firstResult.count+result.count != 1 || objects.puts.Load() != 1 {
		t.Fatalf("duplicate admission: first=%+v second=%+v uploads=%d", firstResult, result, objects.puts.Load())
	}
	var ready int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cloud_snapshots WHERE status='READY'").Scan(&ready); err != nil || ready != 1 {
		t.Fatalf("ready=%d %v", ready, err)
	}
}

func TestSnapshotCancelledUploadReleasesTickLeaseForRetry(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	objects := &pausedTickObjects{fakeObjects: newFakeObjects(), entered: make(chan struct{}), second: make(chan struct{}), release: make(chan struct{})}
	store.Objects = objects
	if _, err := store.Apply(context.Background(), p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan tickResult, 1)
	go func() { n, e := store.TickSnapshots(ctx, time.Now(), 25); done <- tickResult{n, e} }()
	select {
	case <-objects.entered:
	case <-ctx.Done():
		t.Fatal("upload did not start")
	}
	cancel()
	result := <-done
	if result.err == nil || result.count != 0 {
		t.Fatalf("cancelled result %+v", result)
	}
	store.Objects = newFakeObjects()
	n, err := store.TickSnapshots(context.Background(), time.Now(), 25)
	if err != nil || n != 1 {
		t.Fatalf("cancelled lease blocked retry: %d %v", n, err)
	}
}
