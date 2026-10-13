//go:build integration

package jobs

import (
	"context"
	"testing"
	"time"
)

type blockedGeneralHandler struct{ started, release chan struct{} }

func (blockedGeneralHandler) Kind() string { return "general-import" }
func (blockedGeneralHandler) Version() int { return 1 }
func (h blockedGeneralHandler) Handle(ctx context.Context, _ Job) error {
	close(h.started)
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestSafetyDispatchProgressesWhileGeneralHandlerBlocked(t *testing.T) {
	var calls int
	blocked := blockedGeneralHandler{make(chan struct{}), make(chan struct{})}
	general, pool := freshDispatcher(t, map[string]Handler{
		"general-import": blocked,
		"evidence-sweep": scriptHandler{kind: "evidence-sweep", version: 1, calls: &calls},
	})
	general.ExcludedKinds = []string{"evidence-sweep", "account-erasure-reconcile"}
	// The safety job is older, so a general unfiltered claim would steal it.
	enqueueRaw(t, pool, "evidence-sweep", `{"version":1}`, "safety-first")
	enqueueRaw(t, pool, "general-import", `{"version":1}`, "general-second")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := general.RunOnce(ctx); result <- err }()
	select {
	case <-blocked.started:
	case <-time.After(2 * time.Second):
		t.Fatal("general dispatcher stole reserved safety work")
	}
	// The general job stays blocked throughout the actual safety SQL claim/ack.
	safety := &Dispatcher{Queue: general.Queue, Handlers: general.Handlers, WorkerID: "safety-evidence", Kind: "evidence-sweep"}
	enqueueRaw(t, pool, "account-erasure-reconcile", `{"version":1}`, "foreign-safety")
	if found, err := safety.RunOnce(ctx); err != nil || !found || calls != 1 {
		t.Fatal(found, err, calls)
	}
	if status, _ := jobStatus(t, pool, "safety-first"); status != "done" {
		t.Fatal(status)
	}
	if status, _ := jobStatus(t, pool, "general-second"); status != "leased" {
		t.Fatal("general no longer blocked", status)
	}
	if found, err := safety.RunOnce(ctx); err != nil || found {
		t.Fatal("safety dispatcher claimed foreign kind", found, err)
	}
	if status, attempts := jobStatus(t, pool, "foreign-safety"); status != "queued" || attempts != 0 {
		t.Fatal(status, attempts)
	}
	close(blocked.release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if status, _ := jobStatus(t, pool, "general-second"); status != "done" {
		t.Fatal(status)
	}
}
