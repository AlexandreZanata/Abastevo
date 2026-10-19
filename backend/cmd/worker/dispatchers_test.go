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
	if general.Kind != "" || len(general.ExcludedKinds) != 10 {
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
		if !seen[kind] && kind != "quality-import" && kind != "quality-discovery" && kind != "quality-revalidate" && kind != "quality-recompute" && kind != "quality-sanctions-import" && kind != "quality-sanctions-discovery" {
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

func TestWorkerNativeSlotsAreExplicitAndIndependent(t *testing.T) {
	handlers := map[string]jobs.Handler{}
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep", "quality-import", "quality-discovery", "quality-revalidate", "quality-recompute", "quality-sanctions-import", "quality-sanctions-discovery"} {
		handlers[kind] = dispatcherHandler(kind)
	}
	disabled, e := workerDispatchers(nil, handlers)
	if e != nil || len(disabled) != 5 {
		t.Fatal("source handler implicitly claimed work", disabled, e)
	}
	enabled, e := workerDispatchers(nil, handlers, "quality-import", "quality-revalidate", "quality-recompute", "quality-sanctions-import", "quality-sanctions-discovery")
	if e != nil || len(enabled) != 10 || enabled[5].Kind != "quality-import" || enabled[6].Kind != "quality-revalidate" || enabled[7].Kind != "quality-recompute" || enabled[8].Kind != "quality-sanctions-import" || enabled[9].Kind != "quality-sanctions-discovery" {
		t.Fatal("native slot", enabled, e)
	}
	if len(enabled[0].ExcludedKinds) != 10 {
		t.Fatal("general worker can claim source jobs")
	}
	for _, kinds := range [][]string{{"foreign"}, {"quality-import", "quality-import"}} {
		if _, e := workerDispatchers(nil, handlers, kinds...); e == nil {
			t.Fatal("invalid source slots accepted")
		}
	}
	delete(handlers, "quality-import")
	if _, e := workerDispatchers(nil, handlers, "quality-import"); e == nil {
		t.Fatal("missing enabled native handler accepted")
	}
}
