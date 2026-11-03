//go:build integration

package jobs

import (
	"context"
	"testing"
	"time"
)

type cancelOnHandle struct {
	cancel   context.CancelFunc
	complete bool
}

func (cancelOnHandle) Kind() string { return "shutdown-test" }
func (cancelOnHandle) Version() int { return 1 }
func (h cancelOnHandle) Handle(ctx context.Context, _ Job) error {
	h.cancel()
	if h.complete {
		return nil
	}
	return ctx.Err()
}

func TestShutdownFinalizesHeldLease(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "interrupted", true: "completed"}[complete], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h := cancelOnHandle{cancel: cancel, complete: complete}
			d, pool := freshDispatcher(t, map[string]Handler{h.Kind(): h})
			_, err := Enqueue(context.Background(), pool, h.Kind(), []byte(`{"version":1}`), "shutdown", 3, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if found, err := d.RunOnce(ctx); !found || err != nil {
				t.Fatalf("found=%v err=%v", found, err)
			}
			status, attempts := jobStatus(t, pool, "shutdown")
			want := "queued"
			if complete {
				want = "done"
			}
			if status != want || attempts != 1 {
				t.Fatalf("status=%s attempts=%d", status, attempts)
			}
		})
	}
}
