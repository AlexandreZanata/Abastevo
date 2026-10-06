//go:build integration

package read

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
)

func TestCityFeedIsolationExpiryMoneyPaginationAndConcurrency(t *testing.T) {
	r, pool, id := freshReader(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	r.clock = func() time.Time { return now }
	seedProjection(t, pool, id)
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='MT',municipality_code='5107925' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE community_current_prices SET anchor_received_at=$1::timestamptz-interval '7 hours', expires_at=$1::timestamptz+interval '24 hours',confidence='HIGH'`, now); err != nil {
		t.Fatal(err)
	}
	// All fixtures are synthetic. Copy projection without observation attribution.
	ids := []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003", "00000000-0000-0000-0000-000000000004", "00000000-0000-0000-0000-000000000005", "00000000-0000-0000-0000-000000000006"}
	for _, sid := range ids {
		if _, err := pool.Exec(ctx, `INSERT INTO directory_stations(id,display_name,state,municipality_code) VALUES($1,'Synthetic station','MT','5107925')`, sid); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO community_current_prices(station_id,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version) SELECT $1,fuel_product,unit,condition_kind,qualifier_key,3981,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version FROM community_current_prices WHERE station_id=$2`, sid, id); err != nil {
			t.Fatal(err)
		}
	}
	mutations := []struct {
		sql string
		id  string
	}{
		{`UPDATE directory_stations SET municipality_code='5103403' WHERE id=$1`, ids[1]},
		{`UPDATE directory_stations SET status='inactive' WHERE id=$1`, ids[2]},
		{`UPDATE community_current_prices SET expires_at=$2 WHERE station_id=$1`, ids[3]},
		{`UPDATE community_current_prices SET availability='DISPUTED' WHERE station_id=$1`, ids[4]},
		{`UPDATE community_current_prices SET condition_kind='CASH' WHERE station_id=$1`, ids[5]},
	}
	for _, m := range mutations {
		args := []any{m.id}
		if m.id == ids[3] {
			args = append(args, now)
		}
		if _, err := pool.Exec(ctx, m.sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	f, err := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "cheapest", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	page, key, err := r.Feed(ctx, f)
	if err != nil || len(page) != 1 || page[0].StationID != ids[0] || page[0].AmountMilliBrl != 3981 || page[0].Confidence != "MEDIUM" || key == "" {
		t.Fatalf("first %+v %q %v", page, key, err)
	}
	f, err = application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "cheapest", 1, key)
	if err != nil {
		t.Fatal(err)
	}
	page, key, err = r.Feed(ctx, f)
	if err != nil || len(page) != 1 || page[0].StationID != id || key != "" {
		t.Fatalf("second %+v %q %v", page, key, err)
	}
	f, _ = application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "recent", 20, "")
	page, _, err = r.Feed(ctx, f)
	if err != nil || len(page) != 2 || page[0].StationID != ids[0] {
		t.Fatalf("recent ties %+v %v", page, err)
	}
	// Committed updates appear immediately; reads cannot see partially written rows.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				rows, _, e := r.Feed(ctx, f)
				if e != nil || len(rows) != 2 {
					t.Errorf("concurrent read %v %+v", e, rows)
				}
			}
		}()
	}
	if _, err := pool.Exec(ctx, `UPDATE community_current_prices SET amount_milli_brl=3980,anchor_received_at=$2 WHERE station_id=$1`, id, now); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	page, _, err = r.Feed(ctx, f)
	if err != nil || page[0].StationID != id || page[0].AmountMilliBrl != 3980 {
		t.Fatalf("update not visible %+v %v", page, err)
	}
	r.clock = func() time.Time { return now.Add(49 * time.Hour) }
	page, _, err = r.Feed(ctx, f)
	if err != nil || len(page) != 0 {
		t.Fatalf("expiry independent of worker %+v %v", page, err)
	}
}
