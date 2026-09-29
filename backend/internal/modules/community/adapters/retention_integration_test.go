//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"
)

func TestOldestObservationMetric(t *testing.T) {
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	empty, err := s.OldestObservation(ctx)
	if err != nil || !empty.IsZero() {
		t.Errorf("empty oldest = %v, %v", empty, err)
	}
	base := time.Now().Truncate(time.Millisecond)
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name)
		VALUES ('d2222222-0000-4000-8000-000000000001', 'Metric Station')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO community_observations
		(id, contributor_ref, client_submission_id, station_id, fuel_product, unit,
		 amount_milli_brl, condition_kind, qualifier_key, received_at, policy_version)
		VALUES ('b3333333-0000-4000-8000-000000000001', 'tok-m', 'm1',
		 'd2222222-0000-4000-8000-000000000001', 'L', 'L', 100,
		 'STANDARD', 'STANDARD', $1, 'community-v1')`, base.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	oldest, err := s.OldestObservation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if oldest.After(base.Add(-time.Hour).Add(time.Minute)) || oldest.Before(base.Add(-time.Hour).Add(-time.Minute)) {
		t.Errorf("oldest = %v", oldest)
	}
}
