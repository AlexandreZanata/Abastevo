//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNativeSourceSerialWaitDoesNotOccupyPublicationPool(t *testing.T) {
	base := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := base.Config()
	cfg.MaxConns = 1
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	serial := NativeSourceSerial(p)
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	second := make(chan error, 1)
	var calls atomic.Int32
	go func() {
		first <- serial(ctx, func(context.Context) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-entered
	go func() {
		second <- serial(ctx, func(context.Context) error { calls.Add(1); return errors.New("synthetic attempt failed") })
	}()
	blocked := 0
	for ctx.Err() == nil && blocked == 0 {
		if e = base.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name='quality-native-source-serial' AND wait_event_type='Lock'").Scan(&blocked); e != nil {
			break
		}
	}
	if blocked != 1 || calls.Load() != 0 {
		t.Fatal("source fetch/prepare ran outside the serial lease", blocked, calls.Load(), e)
	}
	var one int
	if e = p.QueryRow(ctx, "SELECT 1").Scan(&one); e != nil || one != 1 {
		t.Fatal("lease waiter occupied publication pool", e)
	}
	close(release)
	if e = <-first; e != nil {
		t.Fatal(e)
	}
	if e = <-second; e == nil || calls.Load() != 1 {
		t.Fatal("serial callback outcome lost", e, calls.Load())
	}
	if e = serial(ctx, func(context.Context) error { return nil }); e != nil {
		t.Fatal("failed attempt leaked advisory lease", e)
	}
}
func TestNativeSourceSerialCancellationReleasesWaitingConnection(t *testing.T) {
	p := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serial := NativeSourceSerial(p)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- serial(ctx, func(context.Context) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-entered
	waiting, stop := context.WithCancel(ctx)
	second := make(chan error, 1)
	called := false
	go func() { second <- serial(waiting, func(context.Context) error { called = true; return nil }) }()
	blocked := 0
	for ctx.Err() == nil && blocked == 0 {
		if e := p.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name='quality-native-source-serial' AND wait_event_type='Lock'").Scan(&blocked); e != nil {
			t.Fatal(e)
		}
	}
	if blocked != 1 {
		t.Fatal("no SQL advisory wait barrier")
	}
	stop()
	if e := <-second; e == nil || called {
		t.Fatal("cancelled lease ran callback", e, called)
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := serial(ctx, func(context.Context) error { return nil }); e != nil {
		t.Fatal("cancelled waiter leaked lease", e)
	}
}
