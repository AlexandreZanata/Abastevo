//go:build integration

package read

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	parentofficial "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

type rankingFixture struct {
	reader    *Reader
	pool      *pgxpool.Pool
	stations  map[string]string
	weekStart time.Time
	weekEnd   time.Time
}

func freshRankingFixture(t *testing.T) *rankingFixture {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("ranking_official_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	repo := parentdirectory.NewRepository(pool)
	stations := map[string]string{}
	for _, s := range []struct {
		key, cnpj, name, uf, ibge string
	}{
		{"a", "11222333000181", "Synthetic ranking A", "SP", "3550308"},
		{"b", "04218406000104", "Synthetic ranking B", "SP", "3509502"},
		{"c", "99888777000166", "Synthetic ranking C", "RJ", "3304557"},
		{"d", "88777666000155", "Synthetic ranking D", "MG", "3106200"},
	} {
		station, err := repo.ResolveCNPJ(ctx, s.cnpj, s.name, nil)
		if err != nil {
			t.Fatalf("station %s: %v", s.key, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE directory_stations SET state=$2, municipality_code=$3 WHERE id=$1", station.ID, s.uf, s.ibge); err != nil {
			t.Fatalf("geography %s: %v", s.key, err)
		}
		stations[s.key] = station.ID
	}
	unlocated, err := repo.ResolveCNPJ(ctx, "77666555000144", "Synthetic ranking E (no geography)", nil)
	if err != nil {
		t.Fatalf("station e: %v", err)
	}
	stations["e"] = unlocated.ID
	weekStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	weekEnd := time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)
	return &rankingFixture{reader: NewReader(pool), pool: pool, stations: stations, weekStart: weekStart, weekEnd: weekEnd}
}

func (f *rankingFixture) publish(t *testing.T, checksum string, rows []domain.PriceRow) string {
	t.Helper()
	ctx := context.Background()
	im := parentofficial.NewImporter(f.pool)
	run, err := im.BeginRun(ctx, domain.ImportKey{
		SourceURL: "https://www.gov.br/anp/ranking.xlsx", SourceChecksum: checksum, ParserVersion: "mex28-test",
	}, f.weekStart, f.weekEnd)
	if err != nil || run.NoOp {
		t.Fatalf("begin = %+v, %v", run, err)
	}
	var tally domain.QuarantineTally
	if _, err := im.StageBatch(ctx, run.RevisionID, rows, &tally); err != nil {
		t.Fatalf("stage: %v", err)
	}
	fin, err := im.FinishRun(ctx, run.RunID, run.RevisionID, &tally)
	if err != nil || !fin.Published {
		t.Fatalf("finish = %+v, %v", fin, err)
	}
	return run.RevisionID
}

func (f *rankingFixture) row(t *testing.T, station, product, unit string, amount int64, collected string, sourceRow int) domain.PriceRow {
	t.Helper()
	day, err := time.Parse("2006-01-02", collected)
	if err != nil {
		t.Fatal(err)
	}
	return domain.PriceRow{StationID: f.stations[station], Product: product, Unit: unit, AmountMilli: amount, RawText: "synthetic", CollectedOn: day, SourceRow: sourceRow}
}

func rankingQuery(metric, groupBy, sort string, geography application.RankingGeography, limit int) application.RankingQuery {
	rankingMetric := application.RankingMetric{Kind: metric, GroupBy: groupBy, Sort: sort}
	if metric == "AREA_MEAN" {
		rankingMetric.Weighting = "ELIGIBLE_SOURCE_OBSERVATIONS"
	}
	return application.RankingQuery{
		Version: 1, Kind: "PRICE_RANKING",
		Source:        application.RankingSourceQuery{Kind: "ANP_WEEKLY", SurveyWeekStart: "2025-01-06"},
		Product:       application.RankingProduct{Fuel: "GASOLINE_REGULAR", Unit: "L"},
		Condition:     application.RankingCondition{Kind: "STANDARD"},
		Geography:     geography,
		Metric:        rankingMetric,
		Limit:         limit,
		PolicyVersion: "mex-exploration-v1",
	}
}

func TestMEX28RankingExactCoverageAndCorrection(t *testing.T) {
	f := freshRankingFixture(t)
	ctx := context.Background()
	revision := f.publish(t, "sha256:mex28-v1", []domain.PriceRow{
		f.row(t, "a", "GASOLINE_REGULAR", "L", 5900, "2025-01-07", 1),
		f.row(t, "a", "GASOLINE_REGULAR", "L", 5999, "2025-01-08", 2),
		f.row(t, "b", "GASOLINE_REGULAR", "L", 6001, "2025-01-08", 3),
		f.row(t, "c", "GASOLINE_REGULAR", "L", 6100, "2025-01-08", 4),
		f.row(t, "d", "ETHANOL", "L", 4000, "2025-01-08", 5),
		f.row(t, "e", "GASOLINE_REGULAR", "L", 6200, "2025-01-08", 6),
	})
	country := application.RankingGeography{Level: "COUNTRY", Country: "BR"}

	municipalities, err := application.RankPrices(ctx, f.reader, rankingQuery("AREA_MEAN", "MUNICIPALITY", "ASC", country, 10))
	if err != nil {
		t.Fatal(err)
	}
	if len(municipalities.Areas) != 3 ||
		municipalities.Areas[0].MunicipalityCode != "3550308" || municipalities.Areas[0].Mean != "5999" ||
		municipalities.Areas[1].MunicipalityCode != "3509502" || municipalities.Areas[1].Mean != "6001" ||
		municipalities.Areas[2].MunicipalityCode != "3304557" || municipalities.Areas[2].Mean != "6100" {
		t.Fatalf("municipality ranking %+v", municipalities.Areas)
	}
	if municipalities.Coverage.SourceObservations != 5 || municipalities.Coverage.StationsWithPrice != 4 ||
		municipalities.Coverage.SourceStations != 5 || municipalities.Coverage.Municipalities != 3 ||
		municipalities.Coverage.States != 2 || municipalities.Coverage.UnlocatedObservations != 1 ||
		municipalities.Coverage.Sampled != true || municipalities.Coverage.NationalCompletenessClaimed {
		t.Fatalf("municipality coverage %+v", municipalities.Coverage)
	}
	if municipalities.Source.RevisionID != revision {
		t.Fatalf("revision %q != %q", municipalities.Source.RevisionID, revision)
	}

	states, err := application.RankPrices(ctx, f.reader, rankingQuery("AREA_MEAN", "STATE", "ASC", country, 10))
	if err != nil {
		t.Fatal(err)
	}
	if len(states.Areas) != 2 || states.Areas[0].State != "SP" || states.Areas[0].Mean != "6000" ||
		states.Areas[0].Observations != 2 || states.Areas[1].State != "RJ" || states.Areas[1].Mean != "6100" {
		t.Fatalf("state ranking %+v", states.Areas)
	}

	ascending, err := application.RankPrices(ctx, f.reader, rankingQuery("OBSERVATION_PRICE", "STATION", "ASC", country, 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(ascending.Stations) != 2 || ascending.Stations[0].StationID != f.stations["a"] ||
		ascending.Stations[0].AmountMilliBrl != 5999 || ascending.Stations[1].StationID != f.stations["b"] ||
		ascending.Coverage.StationsWithPrice != 4 {
		t.Fatalf("station ranking %+v", ascending.Stations)
	}
	descending, err := application.RankPrices(ctx, f.reader, rankingQuery("OBSERVATION_PRICE", "STATION", "DESC", country, 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(descending.Stations) != 2 || descending.Stations[0].StationID != f.stations["e"] ||
		descending.Stations[1].StationID != f.stations["c"] {
		t.Fatalf("descending station ranking %+v", descending.Stations)
	}

	sp := application.RankingGeography{Level: "STATE", Country: "BR", UF: "SP"}
	spRanking, err := application.RankPrices(ctx, f.reader, rankingQuery("OBSERVATION_PRICE", "STATION", "ASC", sp, 10))
	if err != nil {
		t.Fatal(err)
	}
	if len(spRanking.Stations) != 2 || spRanking.Coverage.SourceObservations != 3 ||
		spRanking.Coverage.SourceStations != 2 || spRanking.Coverage.Municipalities != 2 ||
		spRanking.Coverage.States != 1 {
		t.Fatalf("state-scoped coverage %+v %+v", spRanking.Stations, spRanking.Coverage)
	}

	unknown := rankingQuery("AREA_MEAN", "MUNICIPALITY", "ASC", country, 10)
	unknown.Product.Fuel = "DIESEL_S10"
	unknownRanking, err := application.RankPrices(ctx, f.reader, unknown)
	if err != nil {
		t.Fatal(err)
	}
	if len(unknownRanking.Areas) != 0 || unknownRanking.Coverage.SourceObservations != 0 ||
		unknownRanking.Coverage.StationsWithPrice != 0 || unknownRanking.Coverage.SourceStations != 5 {
		t.Fatalf("unknown product coverage %+v", unknownRanking.Coverage)
	}

	// A corrected revision changes the current pointer without an aggregate
	// write; the previous revision remains retained.
	corrected := f.publish(t, "sha256:mex28-v2", []domain.PriceRow{
		f.row(t, "a", "GASOLINE_REGULAR", "L", 5800, "2025-01-08", 11),
		f.row(t, "b", "GASOLINE_REGULAR", "L", 6001, "2025-01-08", 12),
		f.row(t, "c", "GASOLINE_REGULAR", "L", 6100, "2025-01-08", 13),
		f.row(t, "d", "ETHANOL", "L", 4000, "2025-01-08", 14),
		f.row(t, "e", "GASOLINE_REGULAR", "L", 6200, "2025-01-08", 15),
	})
	if corrected == revision {
		t.Fatal("correction reused revision")
	}
	correctedResult, err := application.RankPrices(ctx, f.reader, rankingQuery("AREA_MEAN", "MUNICIPALITY", "ASC", country, 10))
	if err != nil {
		t.Fatal(err)
	}
	if correctedResult.Source.RevisionID != corrected || correctedResult.Areas[0].Mean != "5800" {
		t.Fatalf("correction not visible without write %+v", correctedResult)
	}
	var summaries int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM official_summary_prices").Scan(&summaries); err != nil || summaries != 0 {
		t.Fatalf("aggregate projection written: %d %v", summaries, err)
	}

	// A pointer to a non-published revision is unavailable, not an empty result.
	if _, err := f.pool.Exec(ctx, "UPDATE official_revisions SET status='failed' WHERE id=$1", corrected); err != nil {
		t.Fatal(err)
	}
	if _, err := application.RankPrices(ctx, f.reader, rankingQuery("AREA_MEAN", "MUNICIPALITY", "ASC", country, 10)); !errors.Is(err, application.ErrRankingUnavailable) {
		t.Fatalf("expired revision admitted: %v", err)
	}
}

func TestMEX28RankingIndexAndPlan(t *testing.T) {
	f := freshRankingFixture(t)
	ctx := context.Background()
	f.publish(t, "sha256:mex28-plan", []domain.PriceRow{
		f.row(t, "a", "GASOLINE_REGULAR", "L", 5999, "2025-01-08", 1),
		f.row(t, "b", "GASOLINE_REGULAR", "L", 6001, "2025-01-08", 2),
	})
	var index string
	if err := f.pool.QueryRow(ctx, "SELECT indexname FROM pg_indexes WHERE tablename='official_station_prices' AND indexname='official_station_prices_ranking_idx'").Scan(&index); err != nil || index == "" {
		t.Fatalf("ranking index missing: %q %v", index, err)
	}
	// Stage a larger revision so the plan is representative of a national
	// source, then analyze. It is not published and never becomes current.
	im := parentofficial.NewImporter(f.pool)
	bulkStart := time.Date(2025, 2, 3, 0, 0, 0, 0, time.UTC)
	bulkEnd := time.Date(2025, 2, 9, 0, 0, 0, 0, time.UTC)
	run, err := im.BeginRun(ctx, domain.ImportKey{
		SourceURL: "https://www.gov.br/anp/ranking.xlsx", SourceChecksum: "sha256:mex28-bulk", ParserVersion: "mex28-test",
	}, bulkStart, bulkEnd)
	if err != nil || run.NoOp {
		t.Fatalf("bulk begin = %+v, %v", run, err)
	}
	// Direct bounded insert avoids the importer's per-row conflict probe while
	// keeping the same table shape; this staging revision is never published.
	// Many stations make DISTINCT ON ordering representative of a national week.
	if _, err := f.pool.Exec(ctx, `
INSERT INTO directory_stations (id, display_name, municipality_code, state)
SELECT gen_random_uuid(), 'Synthetic bulk ' || i, '3550308', 'SP'
FROM generate_series(1, 1000) AS i`); err != nil {
		t.Fatalf("bulk stations: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `
INSERT INTO official_station_prices
    (id, revision_id, station_id, fuel_product, unit, amount_milli_brl,
     raw_price_text, collected_on, source_row)
SELECT gen_random_uuid(), $1::uuid, s.id, 'GASOLINE_REGULAR', 'L',
    5000 + (i % 1000), 'synthetic', DATE '2000-01-01' + (i % 3650), 1000 + i
FROM generate_series(0, 19999) AS i
JOIN (
    SELECT id, row_number() OVER () AS rn
    FROM directory_stations
    WHERE display_name LIKE 'Synthetic bulk %'
) AS s ON s.rn = 1 + (i % 1000)`, run.RevisionID); err != nil {
		t.Fatalf("bulk insert: %v", err)
	}
	if _, err := f.pool.Exec(ctx, "ANALYZE directory_stations"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "VACUUM (ANALYZE) official_station_prices"); err != nil {
		t.Fatal(err)
	}
	plan := rankingPlan(t, f.pool, run.RevisionID)
	if !strings.Contains(plan, "official_station_prices_ranking_idx") {
		t.Fatalf("representative ranking plan does not use index: %s", plan)
	}
	if strings.Contains(plan, "Seq Scan on official_station_prices") {
		t.Fatalf("representative ranking plan scans the whole price table: %s", plan)
	}
}

func rankingPlan(t *testing.T, pool *pgxpool.Pool, revision string) string {
	t.Helper()
	ctx := context.Background()
	var revisionID string
	if err := pool.QueryRow(ctx, "SELECT id::text FROM official_revisions WHERE id=$1::uuid", revision).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	query := `
EXPLAIN (FORMAT JSON)
WITH latest AS (
    SELECT DISTINCT ON (p.station_id)
        p.station_id, p.amount_milli_brl
    FROM official_station_prices AS p
    WHERE p.revision_id = $1::uuid
        AND p.fuel_product = 'GASOLINE_REGULAR'
        AND p.unit = 'L'
    ORDER BY p.station_id, p.collected_on DESC, p.id
)
SELECT d.state, d.municipality_code, SUM(l.amount_milli_brl), COUNT(*)
FROM latest AS l
JOIN directory_stations AS d ON d.id = l.station_id
GROUP BY d.state, d.municipality_code`
	if err := pool.QueryRow(ctx, query, revisionID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var plans []any
	if err := json.Unmarshal(raw, &plans); err != nil {
		t.Fatalf("plan json: %v %s", err, raw)
	}
	encoded, _ := json.Marshal(plans)
	return string(encoded)
}
