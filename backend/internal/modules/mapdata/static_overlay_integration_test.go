//go:build integration

package mapdata

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildStaticOverlayRealPostGIS(t *testing.T) {
	pool := freshDB(t)
	ctx := context.Background()
	statements := []string{
		`INSERT INTO directory_stations (id, display_name, state, status)
		 VALUES ('11111111-1111-4111-8111-111111111111', 'Posto Teste', 'PR', 'active')`,
		`INSERT INTO directory_identifiers (id, station_id, kind, normalized_value)
		 VALUES ('22222222-2222-4222-8222-222222222222',
		         '11111111-1111-4111-8111-111111111111', 'CNPJ', '01122233300000')`,
		`INSERT INTO official_import_runs (id, source_url, source_checksum, parser_version, status, finished_at)
		 VALUES ('55555555-5555-4555-8555-555555555555', 'https://example.test/anp.xlsx',
		         'sha256:test', 'test', 'completed', now())`,
		`INSERT INTO official_revisions (id, import_run_id, survey_start, survey_end, published_at, status)
		 VALUES ('66666666-6666-4666-8666-666666666666',
		         '55555555-5555-4555-8555-555555555555', '2026-10-11', '2026-10-17', now(), 'published')`,
		`INSERT INTO official_current_revisions (survey_start, survey_end, revision_id)
		 VALUES ('2026-10-11', '2026-10-17', '66666666-6666-4666-8666-666666666666')`,
		`INSERT INTO official_station_prices
		 (id, revision_id, station_id, fuel_product, unit, amount_milli_brl, raw_price_text, collected_on, source_row)
		 VALUES ('77777777-7777-4777-8777-777777777777',
		         '66666666-6666-4666-8666-666666666666',
		         '11111111-1111-4111-8111-111111111111',
		         'GASOLINE_REGULAR', 'L', 5890, '5,89', '2026-10-15', 1)`,
		`INSERT INTO map_generations
		 (id, generation_key, format, product_class, visibility, profile_id, profile_sha256,
		  rights_sha256, source_revision, assertions_sha256, content_sha256, input_count,
		  accepted_count, duplicate_count, quarantined_count, status, activated_at)
		 VALUES ('33333333-3333-4333-8333-333333333333', 'test-gen', 'abastevo.map-prepared-v1',
		         'PUBLIC_OVERLAY', 'PUBLIC', 'test', repeat('a', 64), repeat('b', 64),
		         'test-source', repeat('c', 64), repeat('d', 64), 1, 1, 0, 0, 'ACTIVE', now())`,
		`INSERT INTO map_assertions
		 (generation_id, assertion_id, source, kind, name, cnpj, osm_type, osm_id,
		  latitude_deg, longitude_deg, point, quality, display_point, services, clocks,
		  rights, capabilities, assertion_sha256)
		 VALUES ('33333333-3333-4333-8333-333333333333',
		         '44444444-4444-4444-8444-444444444444', 'OSM', 'FUEL_STATION', 'Posto Teste',
		         '01122233300000', 'node', 1, -25.4, -49.2,
		         ST_SetSRID(ST_MakePoint(-49.2, -25.4), 4326)::geography, 'osm-mapped',
		         'osm_node', ARRAY['FUEL'],
		         '{"observed_at":"2026-10-25T21:21:21Z","published_at":"2026-10-25T21:21:21Z","fetched_at":"2026-10-25T21:21:21Z"}',
		         '{"source":"OSM","license":"ADMITTED","attribution":"test","source_sha256":"' || repeat('e', 64) || '"}',
		         '{"price_known":false,"pin_eligible":true,"entrance_known":false,"live_status":false}',
		         repeat('f', 64))`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	root := t.TempDir()
	result, err := BuildStaticOverlay(ctx, pool, StaticOverlayOptions{OutputRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if result.Counts.Assertions != 1 || result.Counts.WithPrices != 1 || result.Counts.UFs != 1 {
		t.Fatalf("counts=%+v", result.Counts)
	}
	if result.PriceWeek == nil || result.PriceWeek.SurveyStart != "2026-10-11" {
		t.Fatalf("price week=%+v", result.PriceWeek)
	}
	current, err := os.Readlink(filepath.Join(root, "current"))
	if err != nil || current != result.Release {
		t.Fatalf("current=%q err=%v release=%q", current, err, result.Release)
	}
	raw, err := os.ReadFile(filepath.Join(root, result.Release, "PR.json"))
	if err != nil {
		t.Fatal(err)
	}
	var overlay staticOverlayUF
	if err := json.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	if len(overlay.Assertions) != 1 || len(overlay.Assertions[0].Prices) != 1 {
		t.Fatalf("overlay=%+v", overlay)
	}
	if overlay.Assertions[0].Prices[0].AmountMilliBRL != 5890 {
		t.Fatalf("price=%+v", overlay.Assertions[0].Prices[0])
	}
}
