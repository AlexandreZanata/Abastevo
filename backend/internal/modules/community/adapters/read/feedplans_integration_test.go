//go:build integration

package read

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
)

// seedLoad inserts 1500 synthetic stations with identical prices plus one
// measured helpfulness target, so EXPLAIN shapes reflect index choice
// instead of tiny-table sequential scans. All fixtures are synthetic.
func seedLoad(t *testing.T, pool *pgxpool.Pool, now string) string {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name, state, municipality_code)
		SELECT ('00000000-0000-0000-0000-' || lpad(g::text, 12, '0'))::uuid, 'Load station', 'MT', '5107925'
		FROM generate_series(1, 1500) g`); err != nil {
		t.Fatalf("stations: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO community_current_prices
		(station_id, fuel_product, unit, condition_kind, qualifier_key, amount_milli_brl,
		 availability, confidence, anchor_received_at, expires_at, algorithm_version, policy_config_version)
		SELECT id, 'GASOLINE_REGULAR', 'L', 'STANDARD', 'STANDARD', 5999,
		 'AVAILABLE', 'MEDIUM', $1::timestamptz, $1::timestamptz + interval '24 hours',
		 'load-v1', 'load-v1' FROM directory_stations WHERE state = 'MT'`, now); err != nil {
		t.Fatalf("prices: %v", err)
	}
	target := "00000000-0000-0000-0000-ffffffffffff"
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name, state, municipality_code)
		VALUES ($1, 'Helpfulness target station', 'MT', '5107925')`, target); err != nil {
		t.Fatalf("target station: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO community_observations
		(id, contributor_ref, client_submission_id, station_id, fuel_product, unit,
		 amount_milli_brl, condition_kind, qualifier_key, received_at, policy_version)
		SELECT gen_random_uuid(), 'tok-load', 'load-' || row_number() OVER (), s.id, 'GASOLINE_REGULAR', 'L',
		 5999, 'STANDARD', 'STANDARD', now(), 'community-pricing-v1'
		FROM (SELECT id FROM directory_stations WHERE state = 'MT' LIMIT 1500) s`); err != nil {
		t.Fatalf("observations: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO community_observation_decisions
		(id, observation_id, sequence, to_state, policy_version, actor_ref)
		SELECT gen_random_uuid(), o.id, 1, 'VALIDATED', 'community-pricing-v1', 'worker'
		FROM community_observations o JOIN directory_stations s ON s.id = o.station_id
		WHERE s.state = 'MT' LIMIT 1500`); err != nil {
		t.Fatalf("decisions: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO community_helpfulness_votes
		(id, contributor_ref, observation_id, decision_sequence, choice, client_submission_id)
		SELECT gen_random_uuid(), 'tok-voter', o.id, 1, 'HELPFUL', 'load-vote-' || row_number() OVER ()
		FROM community_observations o JOIN directory_stations s ON s.id = o.station_id
		WHERE s.state = 'MT' LIMIT 1500`); err != nil {
		t.Fatalf("votes: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO community_helpfulness_tallies
		(observation_id, decision_sequence, helpful, not_helpful)
		SELECT o.id, 1, 1, 0 FROM community_observations o
		JOIN directory_stations s ON s.id = o.station_id WHERE s.state = 'MT' LIMIT 1500`); err != nil {
		t.Fatalf("tallies: %v", err)
	}
	return target
}

func explainPlan(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `EXPLAIN (COSTS OFF) `+query, args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	plan := strings.Join(lines, "\n")
	if plan == "" {
		t.Fatal("empty plan")
	}
	return plan
}

func TestFeedCheapestPlanUsesIndex(t *testing.T) {
	r, pool, _ := freshReader(t)
	seedLoad(t, pool, "2026-10-06 12:00:00+00")
	plan := explainPlan(t, pool, `SELECT p.station_id FROM community_current_prices p
		JOIN directory_stations s ON s.id = p.station_id
		WHERE s.state = 'MT' AND s.municipality_code = '5107925'
		AND p.fuel_product = 'GASOLINE_REGULAR' AND p.unit = 'L'
		AND p.condition_kind = 'STANDARD' AND p.availability = 'AVAILABLE'
		AND p.expires_at > now()
		ORDER BY p.amount_milli_brl ASC, p.station_id ASC LIMIT 21`)
	if strings.Contains(plan, "Seq Scan") {
		t.Errorf("seq scan in cheapest feed plan:\n%s", plan)
	}
	if !strings.Contains(plan, "Index") {
		t.Errorf("no index in cheapest feed plan:\n%s", plan)
	}
	_ = r
}

func TestHelpfulnessTallyPlanUsesPrimaryKey(t *testing.T) {
	_, pool, _ := freshReader(t)
	target := seedLoad(t, pool, "2026-10-06 12:00:00+00")
	var obsID string
	if err := pool.QueryRow(context.Background(), `SELECT o.id::text FROM community_observations o
		JOIN directory_stations s ON s.id = o.station_id WHERE s.state = 'MT' LIMIT 1`).Scan(&obsID); err != nil {
		t.Fatal(err)
	}
	plan := explainPlan(t, pool, `SELECT observation_id, decision_sequence, helpful, not_helpful
		FROM community_helpfulness_tallies WHERE observation_id = $1::uuid AND decision_sequence = 1`, obsID)
	if strings.Contains(plan, "Seq Scan") {
		t.Errorf("seq scan in tally PK lookup:\n%s", plan)
	}
	if !strings.Contains(plan, "community_helpfulness_tallies_pkey") {
		t.Errorf("tally lookup misses pkey:\n%s", plan)
	}
	plan = explainPlan(t, pool, `SELECT COUNT(*) FILTER (WHERE choice = 'HELPFUL'),
		COUNT(*) FILTER (WHERE choice = 'NOT_HELPFUL')
		FROM community_helpfulness_votes
		WHERE observation_id = $1::uuid AND decision_sequence = 1 AND deleted_at IS NULL`, obsID)
	if strings.Contains(plan, "Seq Scan") {
		t.Errorf("seq scan in rebuild aggregate:\n%s", plan)
	}
	if !strings.Contains(plan, "community_helpfulness_target_idx") {
		t.Errorf("rebuild misses target index:\n%s", plan)
	}
	_ = target
}

func TestFeedTiebreakDeterministicAcrossPages(t *testing.T) {
	r, pool, _ := freshReader(t)
	anchor := "2026-10-06 12:00:00+00"
	seedLoad(t, pool, anchor)
	now, err := time.Parse("2006-01-02 15:04:05-07", anchor)
	if err != nil {
		t.Fatal(err)
	}
	r.clock = func() time.Time { return now }
	// All 1500 rows share amount and anchor: only the station_id
	// tiebreak orders them, on every page including the second+.
	var got []string
	key := ""
	for {
		f, err := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "cheapest", 50, key)
		if err != nil {
			t.Fatal(err)
		}
		page, next, err := r.Feed(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page {
			got = append(got, item.StationID)
		}
		if next == "" {
			break
		}
		key = next
	}
	if len(got) != 1500 {
		t.Fatalf("pages = %d rows, want 1500", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("tiebreak not ascending at %d: %s >= %s", i, got[i-1], got[i])
		}
	}
}

func TestFeedOrdersServeSeededLoad(t *testing.T) {
	r, pool, _ := freshReader(t)
	anchor := "2026-10-06 12:00:00+00"
	seedLoad(t, pool, anchor)
	now, err := time.Parse("2006-01-02 15:04:05-07", anchor)
	if err != nil {
		t.Fatal(err)
	}
	r.clock = func() time.Time { return now }
	for _, order := range []string{"recent", "cheapest", "best", "worst"} {
		f, err := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", order, 5, "")
		if err != nil {
			t.Fatal(err)
		}
		page, _, err := r.Feed(context.Background(), f)
		if err != nil || len(page) == 0 {
			t.Fatalf("order %s serves nothing: %+v %v", order, page, err)
		}
	}
}
