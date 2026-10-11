//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

// CS05: CONTRIBUTION reports queue through the same append-only
// pipeline with the same one-open-case-per-target rule: duplicate
// reports converge instead of flooding, for the new target exactly
// like for observations.
func TestContributionOneOpenCase(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	target := "d6c74c23-63db-4c24-a2e5-408cb23bad27"
	first := mustCase(t, "c0000000-0000-4000-8000-000000000011",
		domain.TargetContribution, target, domain.PriorityP2,
		domain.ContributionReasonSpam, base)
	id, replayed, err := s.OpenCase(ctx, first)
	if err != nil || replayed || id != first.ID {
		t.Fatalf("first open = %q %v %v", id, replayed, err)
	}
	second := mustCase(t, "c0000000-0000-4000-8000-000000000012",
		domain.TargetContribution, target, domain.PriorityP1,
		domain.ContributionReasonVoteGaming, base.Add(time.Second))
	id, replayed, err = s.OpenCase(ctx, second)
	if err != nil || !replayed || id != first.ID {
		t.Fatalf("second open = %q %v %v, want converge onto %q", id, replayed, err, first.ID)
	}
	got, err := s.Get(ctx, first.ID)
	if err != nil || got.Status != domain.StatusOpen || got.TargetType != domain.TargetContribution {
		t.Fatalf("open case = %+v %v", got, err)
	}
}
