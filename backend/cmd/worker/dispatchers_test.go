package main

import (
	"context"
	"errors"
	"testing"

	enterprisejobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type dispatcherHandler string

func (h dispatcherHandler) Kind() string                         { return string(h) }
func (dispatcherHandler) Version() int                           { return 1 }
func (dispatcherHandler) Handle(context.Context, jobs.Job) error { return nil }

func TestWorkerReservesExactlyFourSerialSafetySlots(t *testing.T) {
	handlers := map[string]jobs.Handler{}
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep", "payment-inbox"} {
		handlers[kind] = dispatcherHandler(kind)
	}
	dispatchers, err := workerDispatchers(nil, handlers)
	if err != nil || len(dispatchers) != 6 {
		t.Fatal(dispatchers, err)
	}
	general := dispatchers[0]
	if general.Kind != "" || len(general.ExcludedKinds) != 13 {
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
		if !seen[kind] && kind != "quality-import" && kind != "quality-discovery" && kind != "quality-revalidate" && kind != "quality-recompute" && kind != "quality-sanctions-import" && kind != "quality-sanctions-discovery" && kind != "quality-enforcement-import" && kind != "quality-enforcement-discovery" {
			t.Fatal("safety work lost its slot", kind)
		}
	}
	if !seen["payment-inbox"] {
		t.Fatal("payment work lost its isolated slot")
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
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep", "payment-inbox", "quality-import", "quality-discovery", "quality-revalidate", "quality-recompute", "quality-sanctions-import", "quality-sanctions-discovery"} {
		handlers[kind] = dispatcherHandler(kind)
	}
	disabled, e := workerDispatchers(nil, handlers)
	if e != nil || len(disabled) != 6 {
		t.Fatal("source handler implicitly claimed work", disabled, e)
	}
	enabled, e := workerDispatchers(nil, handlers, "quality-import", "quality-revalidate", "quality-recompute", "quality-sanctions-import", "quality-sanctions-discovery")
	if e != nil || len(enabled) != 11 || enabled[6].Kind != "quality-import" || enabled[7].Kind != "quality-revalidate" || enabled[8].Kind != "quality-recompute" || enabled[9].Kind != "quality-sanctions-import" || enabled[10].Kind != "quality-sanctions-discovery" {
		t.Fatal("native slot", enabled, e)
	}
	if len(enabled[0].ExcludedKinds) != 13 {
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

func TestEnterpriseExportBuildIsIsolated(t *testing.T) {
	handlers := map[string]jobs.Handler{}
	for _, kind := range []string{
		"evidence-sweep", "account-erasure-reconcile",
		"privacy-training-reconcile", "privacy-training-sweep", "payment-inbox",
		enterprisejobs.BuildKind,
	} {
		handlers[kind] = dispatcherHandler(kind)
	}
	dispatchers, err := workerDispatchers(nil, handlers, enterprisejobs.BuildKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispatchers) != 7 {
		t.Fatalf("dispatchers = %d, want general + four safety + payment inbox + export build", len(dispatchers))
	}
	excluded := false
	for _, kind := range dispatchers[0].ExcludedKinds {
		if kind == enterprisejobs.BuildKind {
			excluded = true
		}
	}
	if !excluded {
		t.Fatal("general dispatcher can claim the export build")
	}
	if dispatchers[6].Kind != enterprisejobs.BuildKind {
		t.Fatalf("dedicated dispatcher = %+v", dispatchers[6])
	}
}

func TestEnterprisePortalRefreshIsIsolated(t *testing.T) {
	handlers := map[string]jobs.Handler{}
	for _, kind := range []string{
		"evidence-sweep", "account-erasure-reconcile",
		"privacy-training-reconcile", "privacy-training-sweep", "payment-inbox",
		enterprisejobs.RefreshKind,
	} {
		handlers[kind] = dispatcherHandler(kind)
	}
	dispatchers, err := workerDispatchers(nil, handlers, enterprisejobs.RefreshKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispatchers) != 7 {
		t.Fatalf("dispatchers = %d, want general + four safety + payment inbox + refresh", len(dispatchers))
	}
	excluded := false
	for _, kind := range dispatchers[0].ExcludedKinds {
		if kind == enterprisejobs.RefreshKind {
			excluded = true
		}
	}
	if !excluded {
		t.Fatal("general dispatcher can claim the portal refresh")
	}
	if dispatchers[6].Kind != enterprisejobs.RefreshKind {
		t.Fatalf("dedicated refresh dispatcher = %+v", dispatchers[6])
	}
}
