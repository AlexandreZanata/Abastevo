//go:build integration

package read

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
)

func TestFeedStationNamesArePublicDistinctFactsWithAllowlistedAddress(t *testing.T) {
	r, pool, id := freshReader(t)
	ctx := context.Background()
	r.clock = func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) }
	seedProjection(t, pool, id)
	if _, err := pool.Exec(ctx, `UPDATE community_current_prices SET anchor_received_at=$1::timestamptz-interval '1 hour',expires_at=$1::timestamptz+interval '24 hours' WHERE station_id=$2`, r.clock(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='MT',municipality_code='5107925' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	f, err := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "recent", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := r.Feed(ctx, f)
	if err != nil || len(before) != 1 || before[0].StationLegalName != nil || len(before[0].StationAddress) != 0 {
		t.Fatalf("legacy %v %+v", err, before)
	}
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET display_name='Synthetic Trade',legal_name='Synthetic Legal 123.456.789-09',address='{"street":"Synthetic Avenue","number":"1","municipio":"Synthetic City","uf":"MT","private":"hidden","latitude":"not-an-address"}'::jsonb WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	rows, _, err := r.Feed(ctx, f)
	if err != nil || len(rows) != 1 {
		t.Fatal(err, rows)
	}
	row := rows[0]
	if row.StationName != "Synthetic Trade" || row.StationLegalName == nil || strings.TrimSpace(*row.StationLegalName) != "Synthetic Legal" {
		t.Fatalf("names %+v", row)
	}
	if len(row.StationAddress) != 4 || row.StationAddress["street"] != "Synthetic Avenue" || row.AmountMilliBrl != before[0].AmountMilliBrl {
		t.Fatalf("public facts %+v", row)
	}
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET legal_name='12345678909' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	rows, _, err = r.Feed(ctx, f)
	if err != nil || rows[0].StationLegalName != nil {
		t.Fatal("personal identifier survived", err)
	}
}
