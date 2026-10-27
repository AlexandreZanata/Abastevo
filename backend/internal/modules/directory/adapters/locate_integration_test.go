//go:build integration

package adapters

// B-BR-MAP-02 on real disposable PostGIS: the bulk tier loader projects the
// best proven tier, never downgrades, is idempotent, rolls back on dry-run,
// reports unknown/inactive stations and serializes concurrent batches.

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

const (
	locStationA = "a1000000-0000-4000-8000-000000000001"
	locStationB = "a1000000-0000-4000-8000-000000000002"
	locStationC = "a1000000-0000-4000-8000-000000000003"
	locCNPJA    = "11222333000181"
	locCNPJB    = "05938540000134"
	locCNPJC    = "08065008000148"
	locUnknown  = "33000167000101"
)

func seedLocateStations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []struct{ id, cnpj, status string }{
		{locStationA, locCNPJA, "active"},
		{locStationB, locCNPJB, "active"},
		{locStationC, locCNPJC, "suspended"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name, state, status)
			VALUES ($1, 'Posto Teste', 'SP', $2)`, s.id, s.status); err != nil {
			t.Fatalf("seed station: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO directory_identifiers (id, station_id, kind, normalized_value)
			VALUES (gen_random_uuid(), $1, 'CNPJ', $2)`, s.id, s.cnpj); err != nil {
			t.Fatalf("seed identifier: %v", err)
		}
	}
}

func projection(t *testing.T, pool *pgxpool.Pool, id string) (quality string, lat, lon float64, revisions int) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT coalesce(current_quality, ''),
			coalesce(ST_Y(current_point::geometry), 0), coalesce(ST_X(current_point::geometry), 0)
		FROM directory_stations WHERE id = $1`, id).Scan(&quality, &lat, &lon); err != nil {
		t.Fatalf("projection: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM directory_location_revisions WHERE station_id = $1`, id).Scan(&revisions); err != nil {
		t.Fatalf("revisions: %v", err)
	}
	return quality, lat, lon, revisions
}

func loc(cnpj, quality string, lat, lon float64) TierLocation {
	return TierLocation{CNPJ: cnpj, Quality: quality, Latitude: lat, Longitude: lon,
		Provider: "test", SourceReference: "[LOCATE-TEST]"}
}

func TestApplyTierLocationsLadderIntegration(t *testing.T) {
	pool, _, _ := freshRegistryDB(t)
	seedLocateStations(t, pool)
	ctx := context.Background()
	batch := []TierLocation{
		loc(locCNPJA, domain.QualityANPReported, -23.55, -46.63),
		loc(locCNPJB, domain.QualityReviewed, -25.40, -49.20),
		loc(locCNPJC, domain.QualityAddressPoint, -15.60, -56.10),
		loc(locUnknown, domain.QualityOSMMapped, -22.90, -43.20),
	}

	dry, err := ApplyTierLocations(ctx, pool, batch, true)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if dry.Outcomes[LocateApplied] != 2 || dry.Outcomes[LocateUnknown] != 1 || dry.Outcomes[LocateInactive] != 1 {
		t.Fatalf("dry-run outcomes %+v", dry.Outcomes)
	}
	if q, _, _, revs := projection(t, pool, locStationA); q != "" || revs != 0 {
		t.Fatalf("dry-run wrote: quality=%q revisions=%d", q, revs)
	}

	first, err := ApplyTierLocations(ctx, pool, batch, false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if first.Applied[domain.QualityANPReported] != 1 || first.Applied[domain.QualityReviewed] != 1 {
		t.Fatalf("applied %+v", first.Applied)
	}
	if len(first.Skipped) != 2 || first.Skipped[0].CNPJ != locCNPJC || first.Skipped[1].CNPJ != locUnknown {
		t.Fatalf("skipped %+v", first.Skipped)
	}
	if q, lat, lon, revs := projection(t, pool, locStationA); q != domain.QualityANPReported || lat != -23.55 || lon != -46.63 || revs != 1 {
		t.Fatalf("station A = %q (%v,%v) revs=%d", q, lat, lon, revs)
	}
	if q, _, _, _ := projection(t, pool, locStationC); q != "" {
		t.Fatalf("inactive station projected %q", q)
	}

	// Idempotent rerun: same tier, same point.
	again, err := ApplyTierLocations(ctx, pool, batch, false)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if again.Outcomes[LocateApplied] != 0 || again.Outcomes[LocateUnchanged] != 2 {
		t.Fatalf("rerun outcomes %+v", again.Outcomes)
	}

	// A lower tier never replaces reviewed; an equal tier refreshes the
	// point; a higher tier upgrades.
	next := []TierLocation{
		loc(locCNPJB, domain.QualityAddressPoint, -25.41, -49.21),
		loc(locCNPJA, domain.QualityOSMMapped, -23.551, -46.631),
	}
	res, err := ApplyTierLocations(ctx, pool, next, false)
	if err != nil {
		t.Fatalf("ladder: %v", err)
	}
	if res.Outcomes[LocateHigherTier] != 1 || res.Applied[domain.QualityOSMMapped] != 1 {
		t.Fatalf("ladder outcomes %+v applied %+v", res.Outcomes, res.Applied)
	}
	if q, lat, _, revs := projection(t, pool, locStationB); q != domain.QualityReviewed || lat != -25.40 || revs != 1 {
		t.Fatalf("reviewed station downgraded: %q %v revs=%d", q, lat, revs)
	}
	if q, _, _, revs := projection(t, pool, locStationA); q != domain.QualityOSMMapped || revs != 2 {
		t.Fatalf("upgrade missing: %q revs=%d", q, revs)
	}
	refresh, err := ApplyTierLocations(ctx, pool, []TierLocation{loc(locCNPJA, domain.QualityOSMMapped, -23.552, -46.632)}, false)
	if err != nil || refresh.Applied[domain.QualityOSMMapped] != 1 {
		t.Fatalf("equal-tier refresh: %+v %v", refresh, err)
	}
	var supersedes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM directory_location_revisions
		WHERE station_id = $1 AND supersedes_id IS NOT NULL`, locStationA).Scan(&supersedes); err != nil || supersedes != 2 {
		t.Fatalf("supersede chain = %d, %v", supersedes, err)
	}
}

func TestApplyTierLocationsRejectsBadBatchesIntegration(t *testing.T) {
	pool, _, _ := freshRegistryDB(t)
	seedLocateStations(t, pool)
	ctx := context.Background()
	for name, batch := range map[string][]TierLocation{
		"centroid":  {loc(locCNPJA, domain.QualityCityCentroid, -23.5, -46.6)},
		"unknown":   {loc(locCNPJA, domain.QualityUnknown, -23.5, -46.6)},
		"duplicate": {loc(locCNPJA, domain.QualityANPReported, -23.5, -46.6), loc(locCNPJA, domain.QualityReviewed, -23.5, -46.6)},
	} {
		if _, err := ApplyTierLocations(ctx, pool, batch, false); err == nil {
			t.Errorf("%s batch accepted", name)
		}
	}
	// The table constraint also refuses a non-pin projection with a point.
	if _, err := pool.Exec(ctx, `UPDATE directory_stations
		SET current_point = ST_SetSRID(ST_MakePoint(-46.6, -23.5), 4326)::geography, current_quality = 'city-centroid'
		WHERE id = $1`, locStationA); err == nil {
		t.Error("city-centroid point accepted by directory_stations")
	}
}

func TestApplyTierLocationsConcurrentBatchesIntegration(t *testing.T) {
	pool, _, _ := freshRegistryDB(t)
	seedLocateStations(t, pool)
	ctx := context.Background()
	batch := []TierLocation{
		loc(locCNPJA, domain.QualityANPReported, -23.55, -46.63),
		loc(locCNPJB, domain.QualityAddressPoint, -25.40, -49.20),
	}
	var wg sync.WaitGroup
	results := make([]LocateResult, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = ApplyTierLocations(ctx, pool, batch, false)
		}(i)
	}
	wg.Wait()
	applied := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
		applied += results[i].Outcomes[LocateApplied]
	}
	if applied != 2 {
		t.Fatalf("concurrent batches applied %d rows, want exactly 2", applied)
	}
	for _, id := range []string{locStationA, locStationB} {
		if _, _, _, revs := projection(t, pool, id); revs != 1 {
			t.Fatalf("station %s has %d revisions after concurrent batches", id, revs)
		}
	}
}
