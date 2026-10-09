//go:build integration

package read

// RST-07 read benchmark: census-shaped synthetic stations (skewed cities,
// dense reviewed cluster) benchmark the exact app read path
// (Reader.Search/Nearby) at 100k and 1M rows, baseline indexes versus one
// candidate composite index. Partitioning is adopted only on measured
// benefit; this file produces the numbers for that decision. Runs only
// with -bench; use -benchtime=1x -v (inner loops are fixed).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
)

// benchMirrorSearchSQL/benchMirrorNearbySQL are benchmark mirrors of
// SearchStations/NearbyStations for EXPLAIN capture. TestBenchSQLMirrors
// fails on drift against stations.sql.
const benchMirrorSearchSQL = `EXPLAIN (ANALYZE, BUFFERS)
SELECT id FROM directory_stations
WHERE ($1::text = '' OR state = $1)
    AND ($2::text = '' OR municipality_code = $2)
    AND ($3::text = '' OR display_name ILIKE '%' || $3 || '%')
    AND ($4::text = '' OR id::text > $4)
ORDER BY id::text ASC
LIMIT $5::int`

const benchMirrorNearbySQL = `EXPLAIN (ANALYZE, BUFFERS)
SELECT id FROM directory_stations
WHERE current_point IS NOT NULL
    AND ST_DWithin(current_point, ST_SetSRID(ST_MakePoint($1::float8, $2::float8), 4326)::geography, $3::int)
ORDER BY ST_Distance(current_point, ST_SetSRID(ST_MakePoint($1::float8, $2::float8), 4326)::geography) ASC
LIMIT $4::int`

func TestBenchSQLMirrorsAppQueries(t *testing.T) {
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
		"municipality_code = @municipality", "ORDER BY id::text ASC",
		"ST_DWithin(current_point", "ORDER BY distance_m ASC",
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("stations.sql lost fragment %q", fragment)
		}
	}
	if !strings.Contains(benchMirrorSearchSQL, "municipality_code = $2") ||
		!strings.Contains(benchMirrorNearbySQL, "ST_DWithin(current_point") {
		t.Fatal("bench mirrors lost their predicates")
	}
}

// benchRng is a deterministic xorshift64* generator: same seed, same city.
type benchRng uint64

func (r *benchRng) next() uint64 {
	x := uint64(*r)
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	*r = benchRng(x)
	return x * 0x2545F4914F6CDD1D
}

func (r *benchRng) below(n uint64) uint64 { return r.next() % n }

// seedCensus COPYs n reviewed stations: 70% across 5 dense cities, the
// rest over 195 sparse ones; 60% of points cluster near the dense query
// center, the rest spread over a Brazil-wide box. Returns dense/sparse
// municipality codes plus the COPY duration.
func seedCensus(b *testing.B, pool *pgxpool.Pool, n int) (dense, sparse string, copyMs int64) {
	b.Helper()
	ctx := context.Background()
	rng := benchRng(0xC3555)
	type stationRow struct {
		id    pgtype.UUID
		name  string
		code  string
		point string
	}
	rows := make([]stationRow, 0, n)
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		var code string
		if rng.below(100) < 70 {
			code = fmt.Sprintf("355000%d", rng.below(5))
		} else {
			code = fmt.Sprintf("3550%03d", 5+rng.below(195))
		}
		counts[code]++
		var id pgtype.UUID
		for j := 0; j < 16; j += 8 {
			v := rng.next()
			for k := 0; k < 8; k++ {
				id.Bytes[j+k] = byte(v >> (8 * k))
			}
		}
		id.Valid = true
		var lat, lon float64
		if rng.below(100) < 60 {
			lat = -23.55 + (float64(rng.below(60000))/100000 - 0.3)
			lon = -46.633 + (float64(rng.below(60000))/100000 - 0.3)
		} else {
			lat = -34 + float64(rng.below(4000))/100
			lon = -74 + float64(rng.below(4600))/100
		}
		rows = append(rows, stationRow{
			id:    id,
			name:  fmt.Sprintf("[RST07-TEST] ESTACAO %08d", i),
			code:  code,
			point: fmt.Sprintf("SRID=4326;POINT(%.6f %.6f)", lon, lat),
		})
	}
	best, worst := "", ""
	for code, count := range counts {
		if best == "" || counts[best] < count {
			best = code
		}
		if worst == "" || counts[worst] > count {
			worst = code
		}
	}
	started := time.Now()
	// Batched text INSERTs (500 rows each): COPY binary would need
	// hand-encoded WKB for the geography column; EWKT text keeps the
	// seed honest and portable.
	const batchSize = 500
	for base := 0; base < len(rows); base += batchSize {
		end := base + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := &pgx.Batch{}
		for _, row := range rows[base:end] {
			batch.Queue(
				`INSERT INTO directory_stations
					(id, display_name, address, municipality_code, state, status, current_point, current_quality)
				VALUES ($1, $2, '{}', $3, 'SP', 'active', $4::geography, 'reviewed')`,
				row.id, row.name, row.code, row.point,
			)
		}
		results := pool.SendBatch(ctx, batch)
		for range rows[base:end] {
			if _, err := results.Exec(); err != nil {
				results.Close()
				b.Fatalf("seed insert: %v", err)
			}
		}
		if err := results.Close(); err != nil {
			b.Fatalf("seed batch: %v", err)
		}
	}
	return best, worst, time.Since(started).Milliseconds()
}

func percentiles(ms []float64) (p50, p95, p99 float64) {
	sorted := append([]float64(nil), ms...)
	sort.Float64s(sorted)
	at := func(q float64) float64 {
		return sorted[int(q*float64(len(sorted)-1))]
	}
	return at(0.50), at(0.95), at(0.99)
}

func benchPhase(b *testing.B, reader *Reader, dense, sparse string, iters int, label string) map[string][3]float64 {
	b.Helper()
	ctx := context.Background()
	workloads := map[string]func() error{
		"city_dense": func() error {
			filter, err := application.ValidateSearch("SP", dense, "", 20, "")
			if err != nil {
				return err
			}
			_, _, err = reader.Search(ctx, filter)
			return err
		},
		"city_sparse": func() error {
			filter, err := application.ValidateSearch("SP", sparse, "", 20, "")
			if err != nil {
				return err
			}
			_, _, err = reader.Search(ctx, filter)
			return err
		},
		"nearby_dense": func() error {
			filter, err := application.ValidateNearby(-23.55, -46.633, 5000, 20, "")
			if err != nil {
				return err
			}
			_, _, err = reader.Nearby(ctx, filter)
			return err
		},
		"nearby_sparse": func() error {
			filter, err := application.ValidateNearby(-15.0, -40.0, 5000, 20, "")
			if err != nil {
				return err
			}
			_, _, err = reader.Nearby(ctx, filter)
			return err
		},
	}
	out := map[string][3]float64{}
	for _, name := range []string{"city_dense", "city_sparse", "nearby_dense", "nearby_sparse"} {
		timings := make([]float64, 0, iters)
		for i := 0; i < iters; i++ {
			started := time.Now()
			if err := workloads[name](); err != nil {
				b.Fatalf("%s %s: %v", label, name, err)
			}
			timings = append(timings, float64(time.Since(started).Microseconds())/1000)
		}
		p50, p95, p99 := percentiles(timings)
		out[name] = [3]float64{p50, p95, p99}
		b.Logf("%s %s p50=%.2fms p95=%.2fms p99=%.2fms", label, name, p50, p95, p99)
	}
	return out
}

func benchSizes(ctx context.Context, pool *pgxpool.Pool, b *testing.B, label string) {
	b.Helper()
	var table, indexes int64
	if err := pool.QueryRow(ctx, `SELECT pg_total_relation_size('directory_stations')`).Scan(&table); err != nil {
		b.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT pg_indexes_size('directory_stations')`).Scan(&indexes); err != nil {
		b.Fatal(err)
	}
	b.Logf("%s table_mib=%.1f indexes_mib=%.1f", label, float64(table)/1048576, float64(indexes)/1048576)
}

func benchExplain(ctx context.Context, pool *pgxpool.Pool, b *testing.B, dense string) {
	b.Helper()
	queries := []struct {
		name string
		text string
		args []any
	}{
		{"city", benchMirrorSearchSQL, []any{"SP", dense, "", "", 21}},
		{"nearby", benchMirrorNearbySQL, []any{-46.633, -23.55, 5000, 21}},
	}
	for _, query := range queries {
		rows, err := pool.Query(ctx, query.text, query.args...)
		if err != nil {
			b.Fatalf("explain %s: %v", query.name, err)
		}
		var summary []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				b.Fatal(err)
			}
			trimmed := strings.TrimSpace(line)
			if strings.Contains(line, "Planning Time") || strings.Contains(line, "Execution Time") ||
				(strings.Contains(line, "Scan") && (strings.Contains(line, "Index") || strings.Contains(line, "Seq"))) {
				summary = append(summary, trimmed)
			}
		}
		rows.Close()
		b.Logf("explain %s: %s", query.name, strings.Join(summary, " | "))
	}
}

func benchmarkReads(b *testing.B, rows, iters int) {
	pool := freshPool(b)
	ctx := context.Background()
	reader := NewReader(pool)
	dense, sparse, copyMs := seedCensus(b, pool, rows)
	b.Logf("seeded %d stations in %dms (dense=%s sparse=%s)", rows, copyMs, dense, sparse)
	benchSizes(ctx, pool, b, "baseline")
	benchExplain(ctx, pool, b, dense)
	before := benchPhase(b, reader, dense, sparse, iters, "baseline")

	started := time.Now()
	if _, err := pool.Exec(ctx, `CREATE INDEX bench_city_covering ON directory_stations (state, municipality_code, status, id)`); err != nil {
		b.Fatalf("candidate index: %v", err)
	}
	b.Logf("candidate index built in %dms", time.Since(started).Milliseconds())
	benchSizes(ctx, pool, b, "candidate")
	benchExplain(ctx, pool, b, dense)
	after := benchPhase(b, reader, dense, sparse, iters, "candidate")
	for _, workload := range []string{"city_dense", "city_sparse", "nearby_dense", "nearby_sparse"} {
		base, got := before[workload], after[workload]
		b.Logf("compare %s baseline_p95=%.2fms candidate_p95=%.2fms", workload, base[1], got[1])
	}
}

func BenchmarkCityReads100k(b *testing.B) { benchmarkReads(b, 100000, 100) }

func BenchmarkCityReads1M(b *testing.B) { benchmarkReads(b, 1000000, 30) }
