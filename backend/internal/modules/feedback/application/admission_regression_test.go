package application

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

// SEC01-A admission regressions (AUD-01): one account can create
// unlimited comments, and malformed comments cost authorize +
// station lookups before the text refusal. Each test asserts the
// future bound and skips until SEC01-C implements it — committed
// skipped (not blessed, not breaking CI).

func TestSingleAccountCommentFloodUnbounded(t *testing.T) {
	t.Skip("SEC01-C: per-account comment budget pending")
	svc, _ := testCommentService(allowAll)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if _, err := svc.SubmitComment(ctx, "acc-flood", "station-1", "GASOLINE_REGULAR", fmt.Sprintf("flood %d", i)); err != nil {
			t.Fatalf("comment %d: %v", i, err)
		}
	}
	if _, err := svc.SubmitComment(ctx, "acc-flood", "station-1", "GASOLINE_REGULAR", "one more"); err == nil {
		t.Fatal("101st comment for one account accepted without a budget")
	}
}

func TestInvalidCommentCostsPipelineAccess(t *testing.T) {
	t.Skip("SEC01-C: validate comment text before store IO")
	svc, _ := testCommentService(allowAll)
	ctx := context.Background()
	var lookups atomic.Int64
	svc.StationExists = func(ctx context.Context, stationID string) (bool, error) {
		lookups.Add(1)
		return knownStations(ctx, stationID)
	}
	oversized := strings.Repeat("x", 10<<10)
	if _, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", oversized); err == nil {
		t.Fatal("oversized comment accepted")
	}
	if got := lookups.Load(); got != 0 {
		t.Fatalf("malformed comment cost %d station lookups, want zero", got)
	}
}
