//go:build integration

package read

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"testing"
)

func TestPremiumFeedAndCurrentPriceStaySeparate(t *testing.T) {
	r, pool, id := freshReader(t)
	ctx := context.Background()
	seedProjection(t, pool, id)
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='MT',municipality_code='5107925' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	for product, amount := range map[string]int64{"GASOLINE_ADDITIVED": 6190, "GASOLINE_PREMIUM_GRADE": 9190} {
		if _, err := pool.Exec(ctx, `INSERT INTO community_current_prices(station_id,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version) SELECT station_id,$2,unit,condition_kind,qualifier_key,$3,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version FROM community_current_prices WHERE station_id=$1 AND fuel_product='GASOLINE_REGULAR'`, id, product, amount); err != nil {
			t.Fatal(err)
		}
	}
	for product, amount := range map[string]int64{"GASOLINE_REGULAR": 5999, "GASOLINE_ADDITIVED": 6190, "GASOLINE_PREMIUM_GRADE": 9190} {
		view, err := r.CurrentPrice(ctx, id, product, "L", "STANDARD", "STANDARD")
		if err != nil || !view.HasAmount || view.AmountMilliBrl != amount {
			t.Fatalf("current %s: %+v %v", product, view, err)
		}
		f, err := application.ValidateFeed("MT", "5107925", product, "cheapest", 20, "")
		if err != nil {
			t.Fatal(err)
		}
		items, _, err := r.Feed(ctx, f)
		if err != nil || len(items) != 1 || items[0].AmountMilliBrl != amount {
			t.Fatalf("feed %s: %+v %v", product, items, err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM official_station_prices WHERE station_id=$1`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invented official price: %d %v", count, err)
	}
}
