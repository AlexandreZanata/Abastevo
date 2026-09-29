//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

func TestPurgeClosedCases(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	old := now.Add(-400 * 24 * time.Hour)
	// One long-closed case with audit rows (due), one recent closed
	// case (kept), one old but still open case (never touched).
	seed := func(id, target, status string, opened time.Time) {
		if _, err := pool.Exec(ctx, `INSERT INTO moderation_cases
			(id, target_type, target_id, status, priority, reason, opened_at, policy_version)
			VALUES ($1::uuid, 'OBSERVATION', $2, $3, 'P2', 'r', $4, 'moderation-v1')`,
			id, target, status, opened); err != nil {
			t.Fatalf("seed case: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO moderation_actions
			(id, case_id, actor_id, action, reason, occurred_at, policy_version)
			VALUES (gen_random_uuid(), $1::uuid, 'op-7', 'RESOLVE', 'r', $2, 'moderation-v1')`, id, opened); err != nil {
			t.Fatalf("seed action: %v", err)
		}
	}
	seed("c0000000-0000-4000-8000-000000000001", "b0000000-0000-4000-8000-000000000001", domain.StatusResolved, old)
	seed("c0000000-0000-4000-8000-000000000002", "b0000000-0000-4000-8000-000000000002", domain.StatusResolved, now)
	seed("c0000000-0000-4000-8000-000000000003", "b0000000-0000-4000-8000-000000000003", domain.StatusOpen, old)

	cases, actions, oldest, err := s.PurgeClosedCases(ctx, now.Add(-365*24*time.Hour), 500)
	if err != nil {
		t.Fatalf("purge = %v", err)
	}
	if cases != 1 || actions != 1 {
		t.Errorf("purged cases=%d actions=%d, want 1/1", cases, actions)
	}
	if oldest.IsZero() || oldest.After(old) || oldest.Before(old.Add(-time.Minute)) {
		t.Errorf("oldest = %v want ~%v", oldest, old)
	}
	var leftCases, leftActions int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM moderation_cases`).Scan(&leftCases); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM moderation_actions`).Scan(&leftActions); err != nil {
		t.Fatal(err)
	}
	if leftCases != 2 || leftActions != 2 {
		t.Errorf("remaining cases=%d actions=%d, want 2/2", leftCases, leftActions)
	}
	// Converges: nothing due anymore.
	cases, actions, oldest, err = s.PurgeClosedCases(ctx, now.Add(-365*24*time.Hour), 500)
	if err != nil || cases != 0 || actions != 0 || !oldest.IsZero() {
		t.Errorf("second purge = %d/%d %v %v", cases, actions, oldest, err)
	}
}
