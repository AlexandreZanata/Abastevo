//go:build integration

package adapters

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

const projectionStationA = "cccccccc-3333-4333-8333-000000000001"
const projectionStationB = "cccccccc-3333-4333-8333-000000000002"

func projectionAssay(sample, assay, fuel, day, result, conformity, fingerprint string) application.AssayInput {
	return application.AssayInput{
		SampleRef: sample, AssayRef: assay, SourceCNPJ: "11222333000181",
		ObservedOn: day, Fuel: fuel, ResultText: result, Unit: "",
		Conformity: conformity, EvidenceRef: "f#" + sample + "#" + assay,
		Fingerprint: strings.Repeat(fingerprint, 64),
	}
}

// projectionInput covers the FQ-12 judgment outcomes in one revision:
// gasoline aspect failure, conforming gasoline, confirmed diesel turbidity,
// diesel particles cleared by a conforming complementary, and a replica
// conflict; plus one unlinked sample that must never project.
func projectionInput(hashChar string) application.Input {
	return application.Input{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
		Resources: []application.Resource{{
			URL: "https://www.gov.br/anp/pmqc_2026_09.zip", SHA256: strings.Repeat("a", 64), Bytes: 412001,
		}},
		ManifestHash: strings.Repeat(hashChar, 64),
		Assays: []application.AssayInput{
			projectionAssay("s-a1", "Aspecto", "GASOLINE_REGULAR", "2026-09-04", "Límpido com material particulado", "NONCONFORMING", "1"),
			projectionAssay("s-a2", "Aspecto", "GASOLINE_REGULAR", "2026-09-04", "Límpido e isento de impurezas (LII)", "CONFORMING", "2"),
			projectionAssay("s-b1", "Aspecto", "DIESEL_S10", "2026-09-05", "Turvo", "NONCONFORMING", "3"),
			projectionAssay("s-b1", "Teor de Água", "DIESEL_S10", "2026-09-05", "464", "NONCONFORMING", "4"),
			projectionAssay("s-b2", "Aspecto", "DIESEL_S10", "2026-09-06", "Límpido com material particulado", "NONCONFORMING", "5"),
			projectionAssay("s-b2", "Contaminação total", "DIESEL_S10", "2026-09-06", "10", "CONFORMING", "6"),
			projectionAssay("s-b3", "Ponto de Fulgor", "DIESEL_S10", "2026-09-07", "47.0", "CONFORMING", "7"),
			projectionAssay("s-b3", "Ponto de Fulgor", "DIESEL_S10", "2026-09-07", " ", "NONCONFORMING", "8"),
			projectionAssay("s-c1", "Aspecto", "ETHANOL", "2026-09-08", "Turvo", "NONCONFORMING", "9"),
		},
		Counts: domain.Counts{Input: 9, Accepted: 9},
		Links: []application.LinkInput{
			{RecordKey: "pmqc-assay-v1:s-a1", StationID: projectionStationA, Automatic: true, Reason: "exact cnpj", Active: true},
			{RecordKey: "pmqc-assay-v1:s-a2", StationID: projectionStationA, Automatic: true, Reason: "exact cnpj", Active: true},
			{RecordKey: "pmqc-assay-v1:s-b1", StationID: projectionStationB, Automatic: true, Reason: "exact cnpj", Active: true},
			{RecordKey: "pmqc-assay-v1:s-b2", StationID: projectionStationB, Automatic: true, Reason: "exact cnpj", Active: true},
			{RecordKey: "pmqc-assay-v1:s-b3", StationID: projectionStationB, Automatic: true, Reason: "exact cnpj", Active: true},
			{RecordKey: "pmqc-assay-v1:s-c1", Reason: "unmatched cnpj", Active: false},
		},
	}
}

func seedProjectionStations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, id := range []string{projectionStationA, projectionStationB} {
		if _, err := pool.Exec(context.Background(),
			"INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto Projeção')", id); err != nil {
			t.Fatal(err)
		}
	}
}

func readProjections(t *testing.T, pool *pgxpool.Pool, revision string) []quality.ReadStationFuelProjectionsRow {
	t.Helper()
	rev, err := mustUUID(revision)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := quality.New(pool).ReadStationFuelProjections(context.Background(), rev)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestRecomputeRevisionProjectsDeterministicDimensions(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	seedProjectionStations(t, pool)
	maker := &idMaker{}
	rev, err := NewPublisher(pool).Publish(ctx, projectionInput("b"), maker.next)
	if err != nil {
		t.Fatal(err)
	}
	store := NewProjectionStore(pool)
	report, err := store.RecomputeRevision(ctx, "pmqc", "pmqc-assay-v1", rev)
	if err != nil {
		t.Fatal(err)
	}
	if report.Samples != 5 || report.Stations != 2 || report.Fuels != 2 || report.Applied != 2 {
		t.Fatal("projection report diverged", report)
	}
	rows := readProjections(t, pool, rev)
	if len(rows) != 2 {
		t.Fatal("expected one row per station+fuel", rows)
	}
	byFuel := map[string]quality.ReadStationFuelProjectionsRow{}
	for _, row := range rows {
		byFuel[row.Fuel] = row
		if row.ScoreState != string(domain.ScoreUnscored) || row.Score.Valid || row.Methodology != domain.MethodologyVersion || row.Policy != ProjectionPolicy {
			t.Fatal("score invented without calibration", row)
		}
	}
	gas := byFuel["GASOLINE_REGULAR"]
	if gas.Samples != 2 || gas.Conforming != 1 || gas.Nonconforming != 1 || gas.Unresolved != 0 || gas.Visits != 1 {
		t.Fatal("gasoline dimensions wrong", gas)
	}
	if !gas.OldestObservedOn.Time.Equal(gas.NewestObservedOn.Time) {
		t.Fatal("gasoline observation days wrong", gas)
	}
	diesel := byFuel["DIESEL_S10"]
	if diesel.Samples != 3 || diesel.Conforming != 1 || diesel.Nonconforming != 1 || diesel.Unresolved != 1 || diesel.Visits != 3 {
		t.Fatal("diesel dimensions wrong", diesel)
	}
	if diesel.InputDigest != gas.InputDigest || len(diesel.InputDigest) != 64 {
		t.Fatal("digest not revision-wide", diesel.InputDigest, gas.InputDigest)
	}
	firstGeneration := diesel.Generation

	// Identical recomputation is a true no-op: same digest, no rewrite.
	again, err := store.RecomputeRevision(ctx, "pmqc", "pmqc-assay-v1", rev)
	if err != nil {
		t.Fatal(err)
	}
	if again.InputDigest != report.InputDigest || again.Applied != 0 {
		t.Fatal("replay rewrote identical projections", again)
	}
	if got := readProjections(t, pool, rev); got[1].Generation != firstGeneration {
		t.Fatal("idempotent replay bumped generation", got)
	}
}

func TestRecomputeStaleBasisNeverOverwritesNewer(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	seedProjectionStations(t, pool)
	maker := &idMaker{}
	pub := NewPublisher(pool)
	first, err := pub.Publish(ctx, projectionInput("b"), maker.next)
	if err != nil {
		t.Fatal(err)
	}
	store := NewProjectionStore(pool)
	if _, err = store.RecomputeRevision(ctx, "pmqc", "pmqc-assay-v1", first); err != nil {
		t.Fatal(err)
	}
	// Correction of the same edition: new manifest, pointer moves.
	second, err := pub.Publish(ctx, projectionInput("d"), maker.next)
	if err != nil {
		t.Fatal(err)
	}
	// The pointer moved: the derived rows still reflect the old basis and the
	// new revision reads as unknown until recomputed.
	if rows := readProjections(t, pool, second); len(rows) != 0 {
		t.Fatal("unknown basis fabricated projections", rows)
	}
	if _, err = store.RecomputeRevision(ctx, "pmqc", "pmqc-assay-v1", second); err != nil {
		t.Fatal(err)
	}
	// A stale worker finishing late writes nothing.
	if _, err = store.RecomputeRevision(ctx, "pmqc", "pmqc-assay-v1", first); !errors.Is(err, ErrProjectionBasisStale) {
		t.Fatal("stale basis accepted", err)
	}
	rows := readProjections(t, pool, second)
	if len(rows) != 2 || rows[0].BasisRevision.String() == "" {
		t.Fatal("newer basis lost", rows)
	}
	secondRev, _ := mustUUID(second)
	for _, row := range rows {
		if row.BasisRevision != secondRev {
			t.Fatal("row not bound to newer basis", row)
		}
	}
	if rows[0].InputDigest == "" || rows[0].Generation < 2 {
		t.Fatal("correction did not advance the row", rows[0])
	}
}

func TestRecomputeConcurrentConvergesWithoutForking(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	seedProjectionStations(t, pool)
	maker := &idMaker{}
	rev, err := NewPublisher(pool).Publish(ctx, projectionInput("b"), maker.next)
	if err != nil {
		t.Fatal(err)
	}
	store := NewProjectionStore(pool)
	var wg sync.WaitGroup
	digests := make([]string, 4)
	errs := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			report, rerr := store.RecomputeRevision(ctx, "pmqc", "pmqc-assay-v1", rev)
			digests[i], errs[i] = report.InputDigest, rerr
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("concurrent recompute %d: %v", i, errs[i])
		}
		if digests[i] != digests[0] || len(digests[i]) != 64 {
			t.Fatalf("digests diverged: %v", digests)
		}
	}
	rows := readProjections(t, pool, rev)
	if len(rows) != 2 {
		t.Fatal("concurrent recompute forked rows", rows)
	}
}

// TestRecomputeIgnoresReportErasure freezes the ownership boundary: derived
// official dimensions never read or depend on private reports, moderation or
// community data, so report erasure cannot alter them.
func TestRecomputeIgnoresReportErasure(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	seedProjectionStations(t, pool)
	maker := &idMaker{}
	rev, err := NewPublisher(pool).Publish(ctx, projectionInput("b"), maker.next)
	if err != nil {
		t.Fatal(err)
	}
	store := NewProjectionStore(pool)
	if _, err = store.RecomputeRevision(ctx, "pmqc", "pmqc-assay-v1", rev); err != nil {
		t.Fatal(err)
	}
	before := readProjections(t, pool, rev)
	probe, err := mustUUID("eeeeeeee-4444-4444-8444-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = quality.New(pool).EraseQualityReports(ctx, probe); err != nil {
		t.Fatal(err)
	}
	after := readProjections(t, pool, rev)
	if len(before) != len(after) || before[0].InputDigest != after[0].InputDigest || before[0].Samples != after[0].Samples {
		t.Fatal("report erasure touched official projections", before, after)
	}
}
