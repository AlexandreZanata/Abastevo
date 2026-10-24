package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func workerDispatchers(queue *jobs.Queue, handlers map[string]jobs.Handler, nativeKinds ...string) ([]*jobs.Dispatcher, error) {
	// Fixed serial slots in this process: no per-job concurrency expansion.
	kinds := []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep"}
	fixedExcluded := []string{"quality-import", "quality-discovery", "quality-revalidate", "quality-recompute", "quality-sanctions-import", "quality-sanctions-discovery", "quality-enforcement-import", "quality-enforcement-discovery"}
	excluded := append(append([]string(nil), kinds...), fixedExcluded...)
	excluded = append(excluded, nativeKinds...)
	general := &jobs.Dispatcher{Queue: queue, Handlers: handlers, WorkerID: "worker-1-general", LeaseTTL: 5 * time.Minute, RetryDelay: time.Minute, ExcludedKinds: dedupeStrings(excluded)}
	dispatchers := []*jobs.Dispatcher{general}
	for _, kind := range kinds {
		if handler := handlers[kind]; handler == nil || handler.Kind() != kind {
			return nil, fmt.Errorf("worker: required safety handler unavailable: %s", kind)
		}
		dispatchers = append(dispatchers, &jobs.Dispatcher{Queue: queue, Handlers: handlers, WorkerID: "worker-1-" + kind, Kind: kind, LeaseTTL: 5 * time.Minute, RetryDelay: time.Minute})
	}
	seen := map[string]bool{}
	for _, kind := range nativeKinds {
		if !validNativeKind(kind) || seen[kind] {
			return nil, errors.New("worker: invalid native serial slot")
		}
		seen[kind] = true
		if h := handlers[kind]; h == nil || h.Kind() != kind {
			return nil, errors.New("worker: native handler unavailable")
		}
		dispatchers = append(dispatchers, &jobs.Dispatcher{Queue: queue, Handlers: handlers, WorkerID: "worker-1-" + kind, Kind: kind, LeaseTTL: 5 * time.Minute, RetryDelay: time.Minute})
	}
	return dispatchers, nil
}

func validNativeKind(kind string) bool {
	switch kind {
	case "quality-import", "quality-discovery", "quality-revalidate", "quality-recompute",
		"quality-sanctions-import", "quality-sanctions-discovery",
		"quality-enforcement-import", "quality-enforcement-discovery",
		"enterprise-export-build":
		return true
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// runDispatchers cancels and joins all fixed loops on the first exit. A real
// failure is preserved rather than hidden inside a sibling's cancellation.
func runDispatchers(ctx context.Context, dispatchers []*jobs.Dispatcher) error {
	if len(dispatchers) == 0 {
		return errors.New("worker: dispatchers required")
	}
	for _, d := range dispatchers {
		if d == nil {
			return errors.New("worker: nil dispatcher")
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, len(dispatchers))
	for _, d := range dispatchers {
		go func(d *jobs.Dispatcher) { results <- d.Run(runCtx) }(d)
	}
	var failure error
	for range dispatchers {
		err := <-results
		if err != nil && !errors.Is(err, context.Canceled) && failure == nil {
			failure = err
		}
		if err == nil && ctx.Err() == nil && failure == nil {
			failure = errors.New("worker: dispatcher stopped unexpectedly")
		}
		cancel()
	}
	if failure != nil {
		return failure
	}
	return ctx.Err()
}
