//go:build integration

package read

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	parentofficial "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
)

func seedBulletinStations(t *testing.T, pool *pgxpool.Pool, state string, count int) []string {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		INSERT INTO directory_stations (id, display_name, state, municipality_code, status)
		SELECT md5($1 || '-' || g::text)::uuid, $1 || ' synthetic ' || g::text,
			$1, CASE WHEN $1 = 'SP' THEN '3550308' WHEN $1 = 'MG' THEN '3106200' ELSE '3304557' END,
			'active'
		FROM generate_series(1, $2) AS g
		RETURNING id::text
	`, state, count)
	if err != nil {
		t.Fatalf("seed %s stations: %v", state, err)
	}
	defer rows.Close()
	out := make([]string, 0, count)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan station id: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("station rows: %v", err)
	}
	return out
}

func bulletinRows(stations []string, amount func(int) int64, collected string, startRow int) []domain.PriceRow {
	day, _ := time.Parse("2006-01-02", collected)
	rows := make([]domain.PriceRow, 0, len(stations))
	for i, station := range stations {
		rows = append(rows, domain.PriceRow{
			StationID:   station,
			Product:     "GASOLINE_REGULAR",
			Unit:        "L",
			AmountMilli: amount(i),
			RawText:     "synthetic",
			CollectedOn: day,
			SourceRow:   startRow + i,
		})
	}
	return rows
}

func publishBulletinRevision(t *testing.T, pool *pgxpool.Pool, checksum string, start, end time.Time, rows []domain.PriceRow) string {
	t.Helper()
	ctx := context.Background()
	importer := parentofficial.NewImporter(pool)
	run, err := importer.BeginRun(ctx, domain.ImportKey{
		SourceURL:      "https://www.gov.br/anp/forum-bulletin-synthetic.xlsx",
		SourceChecksum: checksum,
		ParserVersion:  "forum-bulletin-test-v1",
	}, start, end)
	if err != nil || run.NoOp {
		t.Fatalf("begin %s = %+v, %v", checksum, run, err)
	}
	var tally domain.QuarantineTally
	if _, err := importer.StageBatch(ctx, run.RevisionID, rows, &tally); err != nil {
		t.Fatalf("stage %s: %v", checksum, err)
	}
	finish, err := importer.FinishRun(ctx, run.RunID, run.RevisionID, &tally)
	if err != nil || !finish.Published {
		t.Fatalf("finish %s = %+v, %v", checksum, finish, err)
	}
	return run.RevisionID
}

func TestForumBulletinAggregatePair(t *testing.T) {
	f := freshRankingFixture(t)
	ctx := context.Background()
	sp := seedBulletinStations(t, f.pool, "SP", 35)
	mg := seedBulletinStations(t, f.pool, "MG", 31)
	rj := seedBulletinStations(t, f.pool, "RJ", 5)

	prevStart := time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC)
	prevEnd := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
	curStart := f.weekStart
	curEnd := f.weekEnd

	previousRows := append(append(
		bulletinRows(sp, func(i int) int64 { return int64(5500 + i + 1) }, "2025-01-02", 1),
		bulletinRows(mg, func(i int) int64 { return int64(4500 + i + 1) }, "2025-01-02", 100)...),
		bulletinRows(rj, func(i int) int64 { return int64(6500 + i + 1) }, "2025-01-02", 200)...)
	previousRevision := publishBulletinRevision(t, f.pool, "sha256:bulletin-previous", prevStart, prevEnd, previousRows)

	// A first current-week import is superseded by the corrected one below.
	firstRows := append(append(
		bulletinRows(sp, func(i int) int64 { return 1 }, "2025-01-08", 1),
		bulletinRows(mg, func(i int) int64 { return 1 }, "2025-01-08", 100)...),
		bulletinRows(rj, func(i int) int64 { return 1 }, "2025-01-08", 200)...)
	publishBulletinRevision(t, f.pool, "sha256:bulletin-current-v1", curStart, curEnd, firstRows)

	currentRows := append(append(
		bulletinRows(sp, func(i int) int64 { return int64(6000 + i + 1) }, "2025-01-08", 1),
		bulletinRows(mg, func(i int) int64 { return int64(5000 + i + 1) }, "2025-01-08", 100)...),
		bulletinRows(rj, func(i int) int64 { return int64(7000 + i + 1) }, "2025-01-08", 200)...)
	// Duplicate current observation for the first SP station: the later
	// collected_on must win instead of double-counting the earlier row.
	duplicateDay, _ := time.Parse("2006-01-02", "2025-01-09")
	currentRows = append(currentRows, domain.PriceRow{
		StationID: sp[0], Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 9999,
		RawText: "synthetic duplicate", CollectedOn: duplicateDay, SourceRow: 999,
	})
	currentRevision := publishBulletinRevision(t, f.pool, "sha256:bulletin-current-v2", curStart, curEnd, currentRows)

	set, err := application.ReadWeeklyBulletins(ctx, f.reader, application.BulletinQuery{
		MethodologyVersion: application.BulletinMethodologyVersion,
		Fuel:               "GASOLINE_REGULAR",
		Unit:               "L",
	})
	if err != nil {
		t.Fatalf("read bulletins: %v", err)
	}
	if set.CurrentRevision.RevisionID != currentRevision || set.PreviousRevision.RevisionID != previousRevision {
		t.Fatalf("revision pair current=%s previous=%s, want %s/%s",
			set.CurrentRevision.RevisionID, set.PreviousRevision.RevisionID, currentRevision, previousRevision)
	}
	if set.CurrentRevision.SourceChecksum != "sha256:bulletin-current-v2" ||
		set.PreviousRevision.SourceChecksum != "sha256:bulletin-previous" {
		t.Fatalf("manifest checksums %+v", set)
	}
	if len(set.Pairs) != 2 || set.Pairs[0].UF != "MG" || set.Pairs[1].UF != "SP" {
		t.Fatalf("eligible pairs %+v", set.Pairs)
	}
	if len(set.Skipped) != 1 || set.Skipped[0].UF != "RJ" || set.Skipped[0].Reason != application.BulletinSkipBelowMinStations {
		t.Fatalf("skips %+v", set.Skipped)
	}

	spPair := set.Pairs[1]
	if spPair.Current.SumMilliBrl != 214628 || spPair.Current.ObservationCount != 35 ||
		spPair.Current.MeanNumerator != 214628 || spPair.Current.MeanDenominator != 35 ||
		spPair.Current.Mean != "214628/35" {
		t.Fatalf("SP current aggregate %+v", spPair.Current)
	}
	if spPair.Previous.SumMilliBrl != 193130 || spPair.Previous.ObservationCount != 35 ||
		spPair.Previous.Mean != "5518" {
		t.Fatalf("SP previous aggregate %+v", spPair.Previous)
	}
	if spPair.Current.Coverage.SourceObservations != 36 ||
		spPair.Current.Coverage.SampledStations != 35 ||
		spPair.Current.Coverage.SourceStations != 35 ||
		!spPair.Current.Coverage.Sampled ||
		spPair.Current.Coverage.NationalCompletenessClaimed {
		t.Fatalf("SP coverage %+v", spPair.Current.Coverage)
	}

	mgPair := set.Pairs[0]
	if mgPair.Current.SumMilliBrl != 155496 || mgPair.Current.Mean != "5016" ||
		mgPair.Previous.SumMilliBrl != 139996 || mgPair.Previous.Mean != "4516" {
		t.Fatalf("MG aggregate %+v", mgPair)
	}
}

func TestForumBulletinUnavailableWithoutPair(t *testing.T) {
	f := freshRankingFixture(t)
	ctx := context.Background()
	sp := seedBulletinStations(t, f.pool, "SP", 30)
	publishBulletinRevision(t, f.pool, "sha256:only-one", f.weekStart, f.weekEnd,
		bulletinRows(sp, func(i int) int64 { return int64(5000 + i) }, "2025-01-08", 1))

	_, err := application.ReadWeeklyBulletins(ctx, f.reader, application.BulletinQuery{
		MethodologyVersion: application.BulletinMethodologyVersion,
		Fuel:               "GASOLINE_REGULAR",
		Unit:               "L",
	})
	if !errors.Is(err, application.ErrBulletinUnavailable) {
		t.Fatalf("expected ErrBulletinUnavailable, got %v", err)
	}
}

func TestForumBulletinRejectsSumOverflow(t *testing.T) {
	f := freshRankingFixture(t)
	ctx := context.Background()
	sp := seedBulletinStations(t, f.pool, "SP", 30)
	prevStart := time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC)
	prevEnd := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
	publishBulletinRevision(t, f.pool, "sha256:overflow-previous", prevStart, prevEnd,
		bulletinRows(sp, func(i int) int64 { return int64(4000 + i) }, "2025-01-02", 1))
	publishBulletinRevision(t, f.pool, "sha256:overflow-current", f.weekStart, f.weekEnd,
		bulletinRows(sp, func(i int) int64 { return math.MaxInt64 }, "2025-01-08", 1))

	_, err := application.ReadWeeklyBulletins(ctx, f.reader, application.BulletinQuery{
		MethodologyVersion: application.BulletinMethodologyVersion,
		Fuel:               "GASOLINE_REGULAR",
		Unit:               "L",
	})
	if !errors.Is(err, application.ErrBulletinOverflow) {
		t.Fatalf("expected ErrBulletinOverflow, got %v", err)
	}
}

func TestForumBulletinValidateRefusesUnknownFuelUnit(t *testing.T) {
	f := freshRankingFixture(t)
	_, err := application.ReadWeeklyBulletins(context.Background(), f.reader, application.BulletinQuery{
		MethodologyVersion: application.BulletinMethodologyVersion,
		Fuel:               "GASOLINE_REGULAR",
		Unit:               "M3",
	})
	if !errors.Is(err, application.ErrBulletinInvalid) {
		t.Fatalf("expected ErrBulletinInvalid, got %v", err)
	}
}

func TestForumBulletinSampleQueryUsesLatestObservation(t *testing.T) {
	f := freshRankingFixture(t)
	ctx := context.Background()
	sp := seedBulletinStations(t, f.pool, "SP", 31)
	rows := bulletinRows(sp, func(i int) int64 { return int64(5000 + i) }, "2025-01-07", 1)
	rows = append(rows, domain.PriceRow{
		StationID: sp[0], Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 7777,
		RawText: "synthetic", CollectedOn: time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC), SourceRow: 500,
	})
	revision := publishBulletinRevision(t, f.pool, "sha256:sample-latest", f.weekStart, f.weekEnd, rows)

	samples, err := f.reader.ReadBulletinSamples(ctx, revision, "GASOLINE_REGULAR", "L")
	if err != nil {
		t.Fatalf("read samples: %v", err)
	}
	if len(samples) != 1 || samples[0].UF != "SP" {
		t.Fatalf("samples %+v", samples)
	}
	if samples[0].ObservationCount != 31 || samples[0].Coverage.SampledStations != 31 ||
		samples[0].Coverage.SourceObservations != 32 {
		t.Fatalf("sample coverage %+v", samples[0])
	}
	wantSum := int64(31*5000 + 30*31/2)
	// The duplicate overrides the first station's latest amount from 5000 to 7777.
	wantSum += 7777 - 5000
	if samples[0].SumMilliBrl != wantSum {
		t.Fatalf("sum %d, want %d", samples[0].SumMilliBrl, wantSum)
	}
}

func TestForumBulletinSQLRejectsMixedUnit(t *testing.T) {
	f := freshRankingFixture(t)
	ctx := context.Background()
	sp := seedBulletinStations(t, f.pool, "SP", 30)
	rows := bulletinRows(sp, func(i int) int64 { return int64(5000 + i) }, "2025-01-08", 1)
	for i := range rows {
		rows[i].Unit = "M3"
	}
	revision := publishBulletinRevision(t, f.pool, "sha256:mixed-unit", f.weekStart, f.weekEnd, rows)
	if _, err := f.reader.ReadBulletinSamples(ctx, revision, "GASOLINE_REGULAR", "L"); err != nil {
		t.Fatalf("wrong-unit rows must simply be absent, got %v", err)
	}
	samples, err := f.reader.ReadBulletinSamples(ctx, revision, "GASOLINE_REGULAR", "L")
	if err != nil || len(samples) != 0 {
		t.Fatalf("mixed unit leaked: %+v %v", samples, err)
	}
}
