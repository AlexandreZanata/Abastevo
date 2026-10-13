package main

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type dispatcherHandler string

func (h dispatcherHandler) Kind() string                         { return string(h) }
func (dispatcherHandler) Version() int                           { return 1 }
func (dispatcherHandler) Handle(context.Context, jobs.Job) error { return nil }

func TestWorkerReservesExactlyFourSerialSafetySlots(t *testing.T) {
	handlers := map[string]jobs.Handler{}
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep"} {
		handlers[kind] = dispatcherHandler(kind)
	}
	dispatchers, err := workerDispatchers(nil, handlers)
	if err != nil || len(dispatchers) != 5 {
		t.Fatal(dispatchers, err)
	}
	general := dispatchers[0]
	if general.Kind != "" || len(general.ExcludedKinds) != 4 {
		t.Fatal("general dispatcher can steal safety work", general)
	}
	seen := map[string]bool{}
	for _, d := range dispatchers[1:] {
		if d.Kind == "" || seen[d.Kind] || len(d.ExcludedKinds) != 0 {
			t.Fatal("duplicate/foreign slot", d)
		}
		seen[d.Kind] = true
	}
	for _, kind := range general.ExcludedKinds {
		if !seen[kind] {
			t.Fatal("safety work lost its slot", kind)
		}
	}
	delete(handlers, "evidence-sweep")
	if _, err := workerDispatchers(nil, handlers); err == nil {
		t.Fatal("missing safety handler accepted")
	}
}

func TestWorkerDispatcherCancellationJoinsEveryLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Nil queues are safe only because every loop must observe cancellation
	// before claiming. Returning proves all five results were joined.
	dispatchers := make([]*jobs.Dispatcher, 5)
	for i := range dispatchers {
		dispatchers[i] = &jobs.Dispatcher{}
	}
	if err := runDispatchers(ctx, dispatchers); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := runDispatchers(context.Background(), nil); err == nil {
		t.Fatal("empty worker set accepted")
	}
	if err := runDispatchers(context.Background(), []*jobs.Dispatcher{nil}); err == nil {
		t.Fatal("nil worker accepted")
	}
}
