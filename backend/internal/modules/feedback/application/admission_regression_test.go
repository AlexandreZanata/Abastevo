package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// SEC01-C comment bounds (AUD-01, B-BR-F03/F04): per-account live
// budget admits atomically before any row exists; malformed text
// refuses before any authorize/station/parent IO. Formerly skipped
// SEC01-A regressions, now enforced (GREEN).

func TestSingleAccountCommentFloodUnbounded(t *testing.T) {
	svc, _ := testCommentService(allowAll)
	ctx := context.Background()
	for i := 0; i < domain.MaxLiveCommentsPerAccount; i++ {
		if _, err := svc.SubmitComment(ctx, "acc-flood", "station-1", "GASOLINE_REGULAR", fmt.Sprintf("flood %d", i)); err != nil {
			t.Fatalf("comment %d: %v", i, err)
		}
	}
	if _, err := svc.SubmitComment(ctx, "acc-flood", "station-1", "GASOLINE_REGULAR", "one more"); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("101st comment must refuse quota-exceeded, got %v", err)
	}
	if n, err := svc.Comments.CountLiveCommentsByAccount(ctx, "acc-flood"); err != nil || n != domain.MaxLiveCommentsPerAccount {
		t.Fatalf("live comments = %d (%v), want exactly %d", n, err, domain.MaxLiveCommentsPerAccount)
	}
	// A second legitimate account still writes: per-account budget,
	// not a global lockout.
	if _, err := svc.SubmitComment(ctx, "acc-second", "station-1", "GASOLINE_REGULAR", "legit second"); err != nil {
		t.Fatalf("second account must stay free: %v", err)
	}
}

func TestInvalidCommentCostsPipelineAccess(t *testing.T) {
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
	// Empty shapes also refuse before any lookup.
	if _, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "   "); err == nil {
		t.Fatal("empty comment accepted")
	}
	if got := lookups.Load(); got != 0 {
		t.Fatalf("empty comment cost %d station lookups, want zero", got)
	}
}
