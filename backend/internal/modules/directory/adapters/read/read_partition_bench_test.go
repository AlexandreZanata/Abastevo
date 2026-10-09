//go:build integration

package read

// RST-17 physical design comparison: A is the existing unpartitioned
// indexed catalog, B a rebuildable UF (LIST) read projection and C a
// rebuildable municipality-hash (16 partitions, predeclared; 8/32 stay
// unexplored) read projection. B/C are lab-only DDL on the disposable
// database — never migrations — with identical read columns and the
// same three read indexes as the canonical path, so the comparison
// isolates partition effects. Canonical identities, active-CNPJ
// uniqueness and FKs stay in their owner: projections carry no PK or
// uniqueness, and CNPJ resolution stays canonical by construction.
// Decision spec: read-projection-v1, 15% primary benefit / 10% worst
// regression bounds, repeated trials; keep A without a material
// repeatable total-workload benefit. No per-city tables, no automatic
// production migration.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const projectionSpec = "read-projection-v1"

// projectionDDL builds one lab projection with canonical read columns
// and the same three read indexes (GiST point, city covering,
// municipality btree) as partitioned indexes.
func projectionDDL(table, partitionBy string) []string {
	columns := `(id uuid, display_name text, municipality_code text, state text,
		status text, current_point geography(Point, 4326), current_quality text)`
	statements := []string{"CREATE TABLE " + table + " " + columns + " PARTITION BY " + partitionBy}
	statements = append(statements,
		"CREATE INDEX ON "+table+" USING gist (current_point)",
		"CREATE INDEX ON "+table+" (state, municipality_code, status, id)",
		"CREATE INDEX ON "+table+" (municipality_code, state)",
	)
	return statements
}

func buildProjection(tb testing.TB, pool *pgxpool.Pool, table string, partitions []string, source string) (buildMS, walBytes float64) {
	tb.Helper()
	ctx := context.Background()
	for _, ddl := range partitions {
		if _, err := pool.Exec(ctx, ddl); err != nil {
			tb.Fatalf("partition ddl: %v", err)
		}
	}
	var before string
	if err := pool.QueryRow(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&before); err != nil {
		tb.Fatalf("wal: %v", err)
	}
	started := time.Now()
	if _, err := pool.Exec(ctx, `INSERT INTO `+table+` SELECT id, display_name, municipality_code,
		state, status, current_point, current_quality FROM `+source); err != nil {
		tb.Fatalf("rebuild: %v", err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE "+table); err != nil {
		tb.Fatalf("analyze: %v", err)
	}
	buildMS = float64(time.Since(started).Microseconds()) / 1000
	var after string
	if err := pool.QueryRow(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&after); err != nil {
		tb.Fatalf("wal: %v", err)
	}
	var delta int64
	if err := pool.QueryRow(ctx, `SELECT pg_wal_lsn_diff($1::pg_lsn,$2::pg_lsn)`, after, before).Scan(&delta); err != nil {
		tb.Fatalf("wal diff: %v", err)
	}
	walBytes = float64(delta)
	return buildMS, walBytes
}

func projectionSizes(tb testing.TB, pool *pgxpool.Pool, table string) (tableMiB, indexMiB float64) {
	tb.Helper()
	ctx := context.Background()
	// Parent rows hold no data on partitioned tables (relkind 'p'),
	// so sum children; plain tables sum themselves.
	var tableBytes, indexBytes int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(pg_total_relation_size(c.oid)),0),
		COALESCE(SUM(pg_indexes_size(c.oid)),0) FROM pg_class c WHERE c.oid IN (
		SELECT inhrelid FROM pg_inherits i JOIN pg_class p ON p.oid = i.inhparent AND p.relname = $1
		UNION SELECT oid FROM pg_class WHERE relname = $1 AND relkind = 'r')`,
		table).Scan(&tableBytes, &indexBytes); err != nil {
		tb.Fatalf("sizes: %v", err)
	}
	return float64(tableBytes) / 1048576, float64(indexBytes) / 1048576
}

// Table-parameterized mirrors of SearchStations/NearbyStations/GetStation
// with identical predicates; drift is pinned by TestProjectionMirrors.
const projCitySQL = `SELECT id FROM %s
WHERE ($1::text = '' OR state = $1)
    AND ($2::text = '' OR municipality_code = $2)
    AND ($3::text = '' OR display_name ILIKE '%%' || $3 || '%%')
    AND ($4::text = '' OR id::text > $4::text)
ORDER BY id::text ASC
LIMIT $5::int`

const projNearbySQL = `SELECT id FROM %s
WHERE current_point IS NOT NULL
    AND ST_DWithin(current_point, ST_SetSRID(ST_MakePoint($1::float8, $2::float8), 4326)::geography, $3::int)
ORDER BY ST_Distance(current_point, ST_SetSRID(ST_MakePoint($1::float8, $2::float8), 4326)::geography) ASC
LIMIT $4::int`

const projIDSQL = `SELECT id FROM %s WHERE id = $1::uuid`

func TestProjectionMirrors(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller unavailable")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "db", "queries", "directory", "stations.sql"))
	if err != nil {
		t.Fatalf("read stations.sql: %v", err)
	}
	app := string(raw)
	for _, fragment := range []string{
		"municipality_code = @municipality", "ORDER BY id::text ASC", "ST_DWithin(current_point",
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("stations.sql lost fragment %q", fragment)
		}
	}
	for _, fragment := range []string{
		"municipality_code = $2", "ORDER BY id::text ASC", "ST_DWithin(current_point",
	} {
		if !strings.Contains(projCitySQL, fragment) && !strings.Contains(projNearbySQL, fragment) {
			t.Fatalf("projection mirror lost predicate %q", fragment)
		}
	}
	if !strings.Contains(projIDSQL, "WHERE id = $1::uuid") {
		t.Fatal("id mirror lost its predicate")
	}
}

// projCounts runs the three workload shapes against one table.
func projCounts(ctx context.Context, pool *pgxpool.Pool, table, dense string) (city, nearby, point int, err error) {
	cityRows, err := pool.Query(ctx, fmt.Sprintf(projCitySQL, table), "SP", dense, "", "", 21)
	if err != nil {
		return 0, 0, 0, err
	}
	for cityRows.Next() {
		city++
	}
	cityRows.Close()
	nearbyRows, err := pool.Query(ctx, fmt.Sprintf(projNearbySQL, table), -46.633, -23.55, 5000, 21)
	if err != nil {
		return 0, 0, 0, err
	}
	for nearbyRows.Next() {
		nearby++
	}
	nearbyRows.Close()
	return city, nearby, 0, cityRows.Err()
}

func TestPartitionOracleAgrees(t *testing.T) {
	pool, _ := trafficFreshDB(t)
	dense, _, _ := trafficSeed(t, pool, 5000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='RJ'
		WHERE substr(id::text,1,1)='a' AND state='SP'`); err != nil {
		t.Fatal(err)
	}
	buildProjection(t, pool, "rst17_proj_uf",
		append(projectionDDL("rst17_proj_uf", "LIST (state)"),
			`CREATE TABLE rst17_proj_uf_sp PARTITION OF rst17_proj_uf FOR VALUES IN ('SP')`,
			`CREATE TABLE rst17_proj_uf_rj PARTITION OF rst17_proj_uf FOR VALUES IN ('RJ')`),
		"directory_stations")
	hashParts := projectionDDL("rst17_proj_muni16", "HASH (municipality_code)")
	for i := 0; i < 16; i++ {
		hashParts = append(hashParts, fmt.Sprintf(
			`CREATE TABLE rst17_proj_muni16_p%02d PARTITION OF rst17_proj_muni16 FOR VALUES WITH (modulus 16, remainder %d)`, i, i))
	}
	buildProjection(t, pool, "rst17_proj_muni16", hashParts, "directory_stations")
	for _, table := range []string{"directory_stations", "rst17_proj_uf", "rst17_proj_muni16"} {
		city, nearby, _, err := projCounts(ctx, pool, table, dense)
		if err != nil {
			t.Fatal(err)
		}
		baseCity, baseNearby, _, err := projCounts(ctx, pool, "directory_stations", dense)
		if err != nil {
			t.Fatal(err)
		}
		if city != baseCity || nearby != baseNearby {
			t.Fatalf("%s city=%d nearby=%d, want %d/%d", table, city, nearby, baseCity, baseNearby)
		}
	}
	// One known canonical id resolves on every design.
	var known string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM directory_stations WHERE municipality_code = $1 LIMIT 1`, dense).Scan(&known); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"directory_stations", "rst17_proj_uf", "rst17_proj_muni16"} {
		var id string
		if err := pool.QueryRow(ctx, fmt.Sprintf(projIDSQL, table), known).Scan(&id); err != nil {
			t.Fatalf("%s id lookup: %v", table, err)
		}
		if id != known {
			t.Fatalf("%s id lookup = %s, want %s", table, id, known)
		}
	}
}

// partitionWorkload is one timed shape against all three designs.
type partitionWorkload struct {
	name string
	run  func(ctx context.Context, pool *pgxpool.Pool, table string) error
}

func partitionWorkloads(dense string) []partitionWorkload {
	return []partitionWorkload{
		{"single_city", func(ctx context.Context, pool *pgxpool.Pool, table string) error {
			rows, err := pool.Query(ctx, fmt.Sprintf(projCitySQL, table), "SP", dense, "", "", 21)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
			}
			return rows.Err()
		}},
		{"cross_partition_nearby", func(ctx context.Context, pool *pgxpool.Pool, table string) error {
			rows, err := pool.Query(ctx, fmt.Sprintf(projNearbySQL, table), -46.624, -23.5495, 5000, 21)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
			}
			return rows.Err()
		}},
		{"all_city_uf_only", func(ctx context.Context, pool *pgxpool.Pool, table string) error {
			rows, err := pool.Query(ctx, fmt.Sprintf(projCitySQL, table), "SP", "", "", "", 21)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
			}
			return rows.Err()
		}},
	}
}

// BenchmarkPartitionMatrix builds B/C once on a shared 100k census and
// times every workload on A/B/C plus generic plans, rebuild, vacuum
// and lifecycle. Run with -count=3 for repeated trials.
func BenchmarkPartitionMatrix(b *testing.B) {
	pool, _ := trafficFreshDB(b)
	dense, _, _ := trafficSeed(b, pool, 100000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='RJ'
		WHERE substr(id::text,1,1)='a' AND state='SP'`); err != nil {
		b.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='MG'
		WHERE substr(id::text,1,1)='b' AND state='SP'`); err != nil {
		b.Fatal(err)
	}
	// Level the field: the redistribution UPDATEs bloat A with dead
	// tuples while B/C would otherwise be built clean. Vacuum+analyze
	// A first so the comparison isolates partition effects, not bloat.
	levelStarted := time.Now()
	if _, err := pool.Exec(ctx, `VACUUM (ANALYZE) directory_stations`); err != nil {
		b.Fatal(err)
	}
	b.Logf("level vacuum A: %.0fms", float64(time.Since(levelStarted).Microseconds())/1000)
	tables := map[string]string{"A": "directory_stations"}
	buildMS, wal := buildProjection(b, pool, "rst17_proj_uf",
		append(projectionDDL("rst17_proj_uf", "LIST (state)"),
			`CREATE TABLE rst17_proj_uf_sp PARTITION OF rst17_proj_uf FOR VALUES IN ('SP')`,
			`CREATE TABLE rst17_proj_uf_rj PARTITION OF rst17_proj_uf FOR VALUES IN ('RJ')`,
			`CREATE TABLE rst17_proj_uf_mg PARTITION OF rst17_proj_uf FOR VALUES IN ('MG')`),
		"directory_stations")
	tables["B"] = "rst17_proj_uf"
	b.Logf("build B: %.0fms wal_mib=%.1f", buildMS, wal/1048576)
	buildMS, wal = buildProjection(b, pool, "rst17_proj_muni16",
		func() []string {
			parts := projectionDDL("rst17_proj_muni16", "HASH (municipality_code)")
			for i := 0; i < 16; i++ {
				parts = append(parts, fmt.Sprintf(
					`CREATE TABLE rst17_proj_muni16_p%02d PARTITION OF rst17_proj_muni16 FOR VALUES WITH (modulus 16, remainder %d)`, i, i))
			}
			return parts
		}(),
		"directory_stations")
	tables["C"] = "rst17_proj_muni16"
	b.Logf("build C: %.0fms wal_mib=%.1f", buildMS, wal/1048576)
	for _, name := range []string{"A", "B", "C"} {
		tableMiB, indexMiB := projectionSizes(b, pool, tables[name])
		b.Logf("sizes %s table_mib=%.1f indexes_mib=%.1f", name, tableMiB, indexMiB)
	}
	timeWorkload := func(name string, workload partitionWorkload, iters int) map[string][2]float64 {
		out := map[string][2]float64{}
		for design, table := range tables {
			fn := func(ctx context.Context) error { return workload.run(ctx, pool, table) }
			trafficWarmup(ctx, 10, fn)
			lat, errs := trafficSample(ctx, iters, fn)
			if errs != 0 {
				b.Fatalf("%s %s errors: %d", name, design, errs)
			}
			p50, p95, _ := percentiles(lat)
			out[design] = [2]float64{p50, p95}
			b.ReportMetric(p95, fmt.Sprintf("part_%s_%s_p95_ms", name, design))
		}
		base := out["A"][1]
		for _, design := range []string{"B", "C"} {
			delta := (base - out[design][1]) / base * 100
			b.Logf("workload %s %s p50=%.2fms p95=%.2fms delta_p95=%+.1f%%", name, design, out[design][0], out[design][1], delta)
		}
		b.Logf("workload %s A p50=%.2fms p95=%.2fms", name, out["A"][0], out["A"][1])
		return out
	}
	totals := map[string]float64{}
	for _, workload := range partitionWorkloads(dense) {
		// 60 timed iterations per workload: p95 must survive sporadic
		// host stalls (autovacuum/checkpoint on fresh databases) for a
		// repeatability verdict to mean anything.
		got := timeWorkload(workload.name, workload, 60)
		for design, timings := range got {
			totals[design] += timings[1]
		}
	}
	for design, total := range totals {
		b.Logf("total %s p95_sum=%.1fms", design, total)
	}
	// Generic prepared plans on the single-city shape, all designs.
	for _, digit := range dense {
		if digit < '0' || digit > '9' {
			b.Fatalf("non-numeric city code %q", dense)
		}
	}
	for design, table := range tables {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			b.Fatal(err)
		}
		func() {
			defer conn.Release()
			tx, err := conn.Begin(ctx)
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, "SET LOCAL plan_cache_mode = 'force_generic_plan'"); err != nil {
				b.Fatalf("plan mode: %v", err)
			}
			prepared := "part_city_" + design
			if _, err := tx.Exec(ctx, `PREPARE `+prepared+` AS `+fmt.Sprintf(projCitySQL, table)); err != nil {
				b.Fatalf("prepare: %v", err)
			}
			started := time.Now()
			var count int
			for i := 0; i < 20; i++ {
				rows, err := tx.Query(ctx, `EXECUTE `+prepared+`('SP', '`+dense+`', '', '', 21)`)
				if err != nil {
					b.Fatalf("execute: %v", err)
				}
				for rows.Next() {
					count++
				}
				rows.Close()
			}
			b.Logf("generic %s: 20x p95-track mean=%.2fms rows=%d", design, float64(time.Since(started).Microseconds())/1000/20, count)
			if _, err := tx.Exec(ctx, `DEALLOCATE `+prepared); err != nil {
				b.Fatalf("deallocate: %v", err)
			}
		}()
	}
	// Rebuild amplification, vacuum and lifecycle on B/C.
	for _, table := range []string{"rst17_proj_uf", "rst17_proj_muni16"} {
		started := time.Now()
		if _, err := pool.Exec(ctx, `TRUNCATE `+table); err != nil {
			b.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO `+table+` SELECT id, display_name, municipality_code,
			state, status, current_point, current_quality FROM directory_stations`); err != nil {
			b.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `ANALYZE `+table); err != nil {
			b.Fatal(err)
		}
		b.Logf("rebuild %s: %.0fms", table, float64(time.Since(started).Microseconds())/1000)
		started = time.Now()
		if _, err := pool.Exec(ctx, `VACUUM (ANALYZE) `+table); err != nil {
			b.Fatal(err)
		}
		b.Logf("vacuum %s: %.0fms", table, float64(time.Since(started).Microseconds())/1000)
	}
	started := time.Now()
	if _, err := pool.Exec(ctx, `ALTER TABLE rst17_proj_muni16 DETACH PARTITION rst17_proj_muni16_p00`); err != nil {
		b.Fatal(err)
	}
	b.Logf("lifecycle detach: %.0fms", float64(time.Since(started).Microseconds())/1000)
	started = time.Now()
	if _, err := pool.Exec(ctx, `DROP TABLE rst17_proj_muni16_p00`); err != nil {
		b.Fatal(err)
	}
	b.Logf("lifecycle drop: %.0fms", float64(time.Since(started).Microseconds())/1000)
	started = time.Now()
	if _, err := pool.Exec(ctx, `CREATE TABLE rst17_proj_uf_rs PARTITION OF rst17_proj_uf FOR VALUES IN ('RS')`); err != nil {
		b.Fatal(err)
	}
	b.Logf("lifecycle add-partition: %.0fms", float64(time.Since(started).Microseconds())/1000)
}

// BenchmarkPartitionCold measures first-touch single-city reads per
// design after reopening the pool on the same database (cold client,
// unknown server caches — recorded as such, never claimed cold).
func BenchmarkPartitionCold(b *testing.B) {
	pool, dsn := trafficFreshDB(b)
	dense, _, _ := trafficSeed(b, pool, 100000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='RJ'
		WHERE substr(id::text,1,1)='a' AND state='SP'`); err != nil {
		b.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `VACUUM (ANALYZE) directory_stations`); err != nil {
		b.Fatal(err)
	}
	buildProjection(b, pool, "rst17_proj_uf",
		append(projectionDDL("rst17_proj_uf", "LIST (state)"),
			`CREATE TABLE rst17_proj_uf_sp PARTITION OF rst17_proj_uf FOR VALUES IN ('SP')`,
			`CREATE TABLE rst17_proj_uf_rj PARTITION OF rst17_proj_uf FOR VALUES IN ('RJ')`),
		"directory_stations")
	pool.Close()
	pool = reopenPool(b, dsn)
	tables := map[string]string{"A": "directory_stations", "B": "rst17_proj_uf"}
	for _, name := range []string{"A", "B"} {
		started := time.Now()
		rows, err := pool.Query(ctx, fmt.Sprintf(projCitySQL, tables[name]), "SP", dense, "", "", 21)
		if err != nil {
			b.Fatal(err)
		}
		count := 0
		for rows.Next() {
			count++
		}
		rows.Close()
		b.Logf("cold %s: first-touch single-city=%.1fms rows=%d", name, float64(time.Since(started).Microseconds())/1000, count)
	}
}

// BenchmarkPartitionBuffers compares whole-plan buffers once per design
// (top-level shared hit/read only, never summed child counters) for
// the single-city and cross-partition shapes.
func BenchmarkPartitionBuffers(b *testing.B) {
	pool, _ := trafficFreshDB(b)
	dense, _, _ := trafficSeed(b, pool, 100000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE directory_stations SET state='RJ'
		WHERE substr(id::text,1,1)='a' AND state='SP'`); err != nil {
		b.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `VACUUM (ANALYZE) directory_stations`); err != nil {
		b.Fatal(err)
	}
	buildProjection(b, pool, "rst17_proj_uf",
		append(projectionDDL("rst17_proj_uf", "LIST (state)"),
			`CREATE TABLE rst17_proj_uf_sp PARTITION OF rst17_proj_uf FOR VALUES IN ('SP')`,
			`CREATE TABLE rst17_proj_uf_rj PARTITION OF rst17_proj_uf FOR VALUES IN ('RJ')`),
		"directory_stations")
	hashParts := projectionDDL("rst17_proj_muni16", "HASH (municipality_code)")
	for i := 0; i < 16; i++ {
		hashParts = append(hashParts, fmt.Sprintf(
			`CREATE TABLE rst17_proj_muni16_p%02d PARTITION OF rst17_proj_muni16 FOR VALUES WITH (modulus 16, remainder %d)`, i, i))
	}
	buildProjection(b, pool, "rst17_proj_muni16", hashParts, "directory_stations")
	for _, workload := range []struct {
		name string
		sql  string
		args []any
	}{
		{"single_city", projCitySQL, []any{"SP", dense, "", "", 21}},
		{"nearby", projNearbySQL, []any{-46.624, -23.5495, 5000, 21}},
	} {
		for _, design := range []string{"A", "B", "C"} {
			table := map[string]string{"A": "directory_stations", "B": "rst17_proj_uf", "C": "rst17_proj_muni16"}[design]
			var plan string
			if err := pool.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+fmt.Sprintf(workload.sql, table), workload.args...).Scan(&plan); err != nil {
				b.Fatal(err)
			}
			var outer []struct {
				Plan struct {
					SharedHitBlocks  float64 `json:"Shared Hit Blocks"`
					SharedReadBlocks float64 `json:"Shared Read Blocks"`
				} `json:"Plan"`
				ExecutionTime float64 `json:"Execution Time"`
			}
			if err := json.Unmarshal([]byte(plan), &outer); err != nil || len(outer) == 0 {
				b.Fatalf("buffers %s %s: unparsed", workload.name, design)
			}
			b.Logf("buffers %s %s: exec=%.2fms hit=%.0f read=%.0f", workload.name, design, outer[0].ExecutionTime, outer[0].Plan.SharedHitBlocks, outer[0].Plan.SharedReadBlocks)
		}
	}
}

// BenchmarkPartitionBRIN measures a BRIN index on append-only location
// history with the actual range/retention query shape, separately from
// station semantics: 50k synthetic revisions over 200 stations across
// two years, before/after BRIN, then dropped.
func BenchmarkPartitionBRIN(b *testing.B) {
	pool, _ := trafficFreshDB(b)
	_, _, _ = trafficSeed(b, pool, 20000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO directory_location_revisions
		(id, station_id, point, quality, provider, source_reference, obtained_at)
		SELECT gen_random_uuid(), s.id, s.current_point, 'reviewed', 'rst17', 'seed',
			now() - (g || ' days')::interval
		FROM (SELECT id, current_point FROM directory_stations WHERE current_point IS NOT NULL LIMIT 200) s
		CROSS JOIN generate_series(1, 250) g`); err != nil {
		b.Fatalf("seed revisions: %v", err)
	}
	rangeQuery := `SELECT count(*) FROM directory_location_revisions
		WHERE obtained_at > now() - interval '30 days'`
	timeRange := func() (float64, int64) {
		trafficWarmup(ctx, 5, func(ctx context.Context) error {
			return pool.QueryRow(ctx, rangeQuery).Scan(new(int64))
		})
		lat, errs := trafficSample(ctx, 20, func(ctx context.Context) error {
			return pool.QueryRow(ctx, rangeQuery).Scan(new(int64))
		})
		if errs != 0 {
			b.Fatalf("range errors: %d", errs)
		}
		_, p95, _ := percentiles(lat)
		var count int64
		if err := pool.QueryRow(ctx, rangeQuery).Scan(&count); err != nil {
			b.Fatal(err)
		}
		return p95, count
	}
	var plan string
	if err := pool.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+rangeQuery).Scan(&plan); err != nil {
		b.Fatal(err)
	}
	beforeP95, count := timeRange()
	b.Logf("brin baseline: p95=%.2fms rows=%d plan=%.80s", beforeP95, count, plan)
	started := time.Now()
	if _, err := pool.Exec(ctx, `CREATE INDEX rst17_brin_obtained ON directory_location_revisions USING brin (obtained_at)`); err != nil {
		b.Fatalf("brin: %v", err)
	}
	buildMS := float64(time.Since(started).Microseconds()) / 1000
	if _, err := pool.Exec(ctx, `ANALYZE directory_location_revisions`); err != nil {
		b.Fatal(err)
	}
	afterP95, _ := timeRange()
	var size int64
	if err := pool.QueryRow(ctx, `SELECT pg_relation_size('rst17_brin_obtained')`).Scan(&size); err != nil {
		b.Fatal(err)
	}
	improve := (beforeP95 - afterP95) / beforeP95 * 100
	b.ReportMetric(buildMS, "brin_build_ms")
	b.ReportMetric(float64(size), "brin_bytes")
	b.Logf("brin: build=%.0fms size_kb=%.1f p95=%.2fms delta=%+.1f%%", buildMS, float64(size)/1024, afterP95, improve)
	if _, err := pool.Exec(ctx, `DROP INDEX rst17_brin_obtained`); err != nil {
		b.Fatal(err)
	}
}
