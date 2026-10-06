//go:build integration

package read

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
)

// Best/worst orders rank the same station+fuel by the community star
// average; unrated rows sort last in both and stay pageable across the
// rated/unrated boundary.
func TestCityFeedBestWorstOrdersByCommunityStars(t *testing.T) {
	r, pool, base := freshReader(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	r.clock = func() time.Time { return now }
	seedProjection(t, pool, base)
	stations := []string{
		base,
		"00000000-0000-0000-0000-000000000011",
		"00000000-0000-0000-0000-000000000012",
	}
	for _, sid := range stations[1:] {
		if _, err := pool.Exec(ctx, `INSERT INTO directory_stations(id,display_name,state,municipality_code) VALUES($1,'Synthetic station','MT','5107925')`, sid); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO community_current_prices(station_id,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version) SELECT $1,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version FROM community_current_prices WHERE station_id=$2`, sid, base); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='MT',municipality_code='5107925' WHERE id=$1`, base); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE community_current_prices SET anchor_received_at=$1::timestamptz-interval '7 hours', expires_at=$1::timestamptz+interval '24 hours'`, now); err != nil {
		t.Fatal(err)
	}
	// base: avg 5.0 over 2 ratings; stations[1]: avg 3.0; stations[2]: unrated.
	for _, seed := range []struct {
		station string
		count   int64
		sum     int64
	}{
		{base, 2, 10},
		{stations[1], 4, 12},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO feedback_rating_stats(station_id,product,ratings_count,stars_sum) VALUES($1,'GASOLINE_REGULAR',$2,$3)`, seed.station, seed.count, seed.sum); err != nil {
			t.Fatal(err)
		}
	}
	best, err := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "best", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	page, _, err := r.Feed(ctx, best)
	if err != nil || len(page) != 3 {
		t.Fatalf("best page %+v %v", page, err)
	}
	if page[0].StationID != base || page[0].RatingsAvg == nil || *page[0].RatingsAvg != 5.0 || page[0].RatingsCount != 2 {
		t.Fatalf("best first %+v", page[0])
	}
	if page[1].StationID != stations[1] || page[1].RatingsAvg == nil || *page[1].RatingsAvg != 3.0 {
		t.Fatalf("best second %+v", page[1])
	}
	if page[2].StationID != stations[2] || page[2].RatingsAvg != nil || page[2].RatingsCount != 0 {
		t.Fatalf("unrated sorts last %+v", page[2])
	}
	worst, err := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "worst", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	page, _, err = r.Feed(ctx, worst)
	if err != nil || len(page) != 3 || page[0].StationID != stations[1] || page[1].StationID != base || page[2].StationID != stations[2] {
		t.Fatalf("worst order %+v %v", page, err)
	}
	// Page across the rated/unrated boundary: limit 2 then continue.
	first, err := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "best", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	p1, cursor, err := r.Feed(ctx, first)
	if err != nil || len(p1) != 2 || cursor == "" {
		t.Fatalf("best first page %+v %q %v", p1, cursor, err)
	}
	second, err := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "best", 2, cursor)
	if err != nil {
		t.Fatal(err)
	}
	p2, _, err := r.Feed(ctx, second)
	if err != nil || len(p2) != 1 || p2[0].StationID != stations[2] {
		t.Fatalf("best tail page %+v %v", p2, err)
	}
}
