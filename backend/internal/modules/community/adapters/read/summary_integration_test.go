//go:build integration

package read

import (
	"context"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
	"time"
)

func TestCitySummaryUsesAllStationsNotFirstPageAndSeparatesExpiryAndConditions(t *testing.T) {
	r, pool, id := freshReader(t)
	ctx := context.Background()
	seedProjection(t, pool, id)
	now := time.Now().UTC()
	r.clock = func() time.Time { return now }
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='MT',municipality_code='5103403' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE community_current_prices SET amount_milli_brl=4000,anchor_received_at=$1::timestamptz-interval '1 hour',expires_at=$1::timestamptz+interval '1 hour'`, now); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 64; i++ {
		sid := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		if _, err := pool.Exec(ctx, `INSERT INTO directory_stations(id,display_name,state,municipality_code) VALUES($1,'Synthetic summary station','MT','5103403')`, sid); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO community_current_prices(station_id,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version) SELECT $1,fuel_product,unit,condition_kind,qualifier_key,$3,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version FROM community_current_prices WHERE station_id=$2`, sid, id, 4000+i); err != nil {
			t.Fatal(err)
		}
	}
	f, _ := application.ValidateFeed("MT", "5103403", "GASOLINE_REGULAR", "cheapest", 20, "")
	summary, err := r.CitySummary(ctx, f)
	if err != nil || len(summary) != 1 || summary[0].StationCount != 65 || summary[0].AverageMilliBrl != 4032 || summary[0].MinimumMilliBrl != 4000 || summary[0].MaximumMilliBrl != 4064 {
		t.Fatalf("summary %+v %v", summary, err)
	}
	for _, order := range []string{"cheapest", "recent", "best", "worst"} {
		f.Order = order
		f.HasCursor = false
		seen := map[string]bool{}
		sum := int64(0)
		for page := 0; page < 5; page++ {
			rows, next, err := r.Feed(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if seen[row.StationID] {
					t.Fatal("duplicate page station")
				}
				seen[row.StationID] = true
				sum += row.AmountMilliBrl
			}
			if next == "" {
				break
			}
			f, err = application.ValidateFeed(f.State, f.Municipality, f.Product, order, 20, next)
			if err != nil {
				t.Fatal(err)
			}
		}
		if len(seen) != 65 || sum != 65*4032 {
			t.Fatalf("%s skipped/repeated stations: %d sum=%d", order, len(seen), sum)
		}
	}
	// Exclude different city, inactive, exact expiry, disputed, nonstandard, future anchor, hidden representative.
	changes := []string{`UPDATE directory_stations SET municipality_code='5107925' WHERE id='00000000-0000-4000-8000-000000000001'`, `UPDATE directory_stations SET status='inactive' WHERE id='00000000-0000-4000-8000-000000000002'`, `UPDATE community_current_prices SET expires_at=$1 WHERE station_id='00000000-0000-4000-8000-000000000003'`, `UPDATE community_current_prices SET availability='DISPUTED' WHERE station_id='00000000-0000-4000-8000-000000000004'`, `UPDATE community_current_prices SET condition_kind='CASH' WHERE station_id='00000000-0000-4000-8000-000000000005'`, `UPDATE community_current_prices SET anchor_received_at=$1::timestamptz+interval '1 minute' WHERE station_id='00000000-0000-4000-8000-000000000006'`}
	for i, q := range changes {
		var args []any
		if i == 2 || i == 5 {
			args = []any{now}
		}
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE community_observations SET hidden_at=$1`, now); err != nil {
		t.Fatal(err)
	}
	summary, err = r.CitySummary(ctx, f)
	if err != nil || len(summary) != 1 || summary[0].StationCount != 58 {
		t.Fatalf("exclusions %+v %v", summary, err)
	}
	// Committed projection changes appear immediately; query-time expiry needs no worker.
	if _, err := pool.Exec(ctx, `UPDATE community_current_prices SET amount_milli_brl=4999 WHERE station_id='00000000-0000-4000-8000-000000000064'`); err != nil {
		t.Fatal(err)
	}
	summary, err = r.CitySummary(ctx, f)
	if err != nil || summary[0].MaximumMilliBrl != 4999 {
		t.Fatalf("live update %+v %v", summary, err)
	}
	r.clock = func() time.Time { return now.Add(2 * time.Hour) }
	summary, err = r.CitySummary(ctx, f)
	if err != nil || len(summary) != 0 {
		t.Fatalf("expiry %+v %v", summary, err)
	}
}

func TestCitySummaryExactRoundingUnitsAndAtomicReadSnapshot(t *testing.T) {
	r, pool, id := freshReader(t)
	ctx := context.Background()
	seedProjection(t, pool, id)
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='MT',municipality_code='5103403' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations(id,display_name,state,municipality_code) VALUES('00000000-0000-4000-8000-000000000001','Synthetic second sample','MT','5103403')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE community_current_prices SET amount_milli_brl=4000 WHERE station_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO community_current_prices(station_id,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version) SELECT '00000000-0000-4000-8000-000000000001',fuel_product,unit,condition_kind,qualifier_key,4001,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version FROM community_current_prices WHERE station_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	f, _ := application.ValidateFeed("MT", "5103403", "GASOLINE_REGULAR", "recent", 20, "")
	rows, err := r.CitySummary(ctx, f)
	if err != nil || len(rows) != 1 || rows[0].AverageMilliBrl != 4001 {
		t.Fatalf("half-up %+v %v", rows, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO community_current_prices(station_id,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version) SELECT station_id,'CNG','M3',condition_kind,qualifier_key,7999,availability,confidence,anchor_received_at,expires_at,algorithm_version,policy_config_version FROM community_current_prices WHERE station_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	rows, err = r.CitySummary(ctx, f)
	if err != nil || len(rows) != 2 || rows[0].FuelProduct != "CNG" || rows[0].Unit != "M3" || rows[0].AverageMilliBrl != 7999 || rows[1].StationCount != 2 {
		t.Fatalf("unit isolation %+v %v", rows, err)
	}
	// A transaction with an intermediate first-row change cannot expose a mixed mean.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE community_current_prices SET amount_milli_brl=6000 WHERE station_id=$1 AND fuel_product='GASOLINE_REGULAR'`, id); err != nil {
		t.Fatal(err)
	}
	rows, err = r.CitySummary(ctx, f)
	if err != nil || rows[1].AverageMilliBrl != 4001 {
		t.Fatalf("dirty partial price %+v %v", rows, err)
	}
	if _, err = tx.Exec(ctx, `UPDATE community_current_prices SET amount_milli_brl=6000 WHERE fuel_product='GASOLINE_REGULAR'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err = r.CitySummary(ctx, f)
	if err != nil || rows[1].AverageMilliBrl != 6000 {
		t.Fatalf("committed snapshot %+v %v", rows, err)
	}
}

type summaryTrace struct {
	calls int
	sql   string
	args  []any
}

func (s *summaryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	s.calls++
	s.sql = data.SQL
	s.args = data.Args
	return ctx
}
func (*summaryTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func TestSummaryActualQueryIsOneRoundTripAndHasAnAggregatePlan(t *testing.T) {
	r, pool, _ := freshReader(t)
	at := time.Now().UTC()
	seedLoad(t, pool, at.Add(-time.Hour).Format(time.RFC3339))
	trace := &summaryTrace{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = trace
	traced, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer traced.Close()
	r.pool = traced
	f, _ := application.ValidateFeed("MT", "5107925", "GASOLINE_REGULAR", "recent", 20, "")
	rows, err := r.CitySummary(context.Background(), f)
	if err != nil || len(rows) != 1 || rows[0].StationCount != 1500 || trace.calls != 1 {
		t.Fatalf("one full population query %+v %v calls=%d", rows, err, trace.calls)
	}
	plan := explainPlan(t, pool, trace.sql, trace.args...)
	if !strings.Contains(plan, "Aggregate") {
		t.Fatalf("actual aggregate plan missing: %s", plan)
	}
}
