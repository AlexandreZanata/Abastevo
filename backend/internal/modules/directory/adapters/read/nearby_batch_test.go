//go:build integration

package read

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

type nearbyQueryCounter struct{ count atomic.Int32 }

func (c *nearbyQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.count.Add(1)
	return ctx
}

func (c *nearbyQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// TestNearbyEnrichmentHasConstantRoundTrips pins the Android budget for the
// transient nearby lookup: one page must cost a constant number of round
// trips, like Search, never one pair of lookups per row.
func TestNearbyEnrichmentHasConstantRoundTrips(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	repo := parent.NewRepository(pool)
	for i := 0; i < 10; i++ {
		cnpj := fmt.Sprintf("%014d", 90000000000000+i)
		st, err := repo.ResolveCNPJ(ctx, cnpj, fmt.Sprintf("Posto Nearby %d", i), map[string]string{"municipio": "3550308"})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		rev, err := repo.RecordLocation(ctx, domain.LocationRevision{
			StationID: st.ID,
			PointWKT:  fmt.Sprintf("POINT(-46.634%d -23.551%d)", i%10, i%10),
			Quality:   domain.QualityReviewed,
			Provider:  "review",
		})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if _, err := repo.ProjectLocation(ctx, st.ID, rev.ID); err != nil {
			t.Fatalf("project: %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='SP', municipality_code='3550308' WHERE id=$1`, st.ID); err != nil {
			t.Fatal(err)
		}
	}
	counter := &nearbyQueryCounter{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = counter
	cfg.MaxConns = 2
	counted, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer counted.Close()
	reader := NewReader(counted)
	counter.count.Store(0)
	items, _, err := reader.Nearby(ctx, application.NearbyFilter{Lat: -23.551, Lon: -46.634, RadiusM: 3000, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 {
		t.Fatalf("nearby rows = %d, want 10", len(items))
	}
	if n := counter.count.Load(); n > 2 {
		t.Errorf("nearby page: %d queries, want at most two", n)
	}
	for _, item := range items {
		detail, err := reader.Detail(ctx, item.ID)
		if err != nil || !reflect.DeepEqual(item.Station, detail) {
			t.Fatalf("batch changed detail for %s: %v", item.ID, err)
		}
	}
}
