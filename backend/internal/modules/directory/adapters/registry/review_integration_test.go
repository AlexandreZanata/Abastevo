//go:build integration

package registry

// RST-06 review-gate integration on real disposable PostGIS: the CRS
// transform runs through PROJ (never Go math), corroboration policy
// decides on transformed points, and measured datum shifts are logged
// as evidence instead of national promises.

import (
	"context"
	"math"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestTransformPointIntegrationDatumBands(t *testing.T) {
	store := freshStore(t)
	ctx := context.Background()

	// SIRGAS 2000 is definitionally coincident with WGS84 at the metre
	// scale: the transform must execute and stay in datum class. The
	// tight axis check below is the real guard (lon/lat swap breaks it).
	points := [][2]float64{
		{-23.561, -46.656},
		{-23.55, -46.63},
		{-23.40, -46.50},
		{5.27, -28.85},
		{-33.75, -73.99},
	}
	var maxShift float64
	for _, point := range points {
		lat, lon, err := store.Transform(ctx, point[0], point[1], "EPSG:4674")
		if err != nil {
			t.Fatalf("transform %v: %v", point, err)
		}
		if math.Abs(lat-point[0]) > 0.01 || math.Abs(lon-point[1]) > 0.01 {
			t.Fatalf("axis drift: (%v, %v) -> (%v, %v)", point[0], point[1], lat, lon)
		}
		shift := haversineM(point[0], point[1], lat, lon)
		if shift > 5 {
			t.Fatalf("datum shift %.3f m exceeds 5 m", shift)
		}
		maxShift = math.Max(maxShift, shift)
	}
	t.Logf("measured EPSG:4674->EPSG:4326 shift over %d sample points: max %.4f m", len(points), maxShift)

	// 4326 is the identity through the same SQL path.
	lat, lon, err := store.Transform(ctx, -23.561, -46.656, "EPSG:4326")
	if err != nil {
		t.Fatalf("identity transform: %v", err)
	}
	if lat != -23.561 || lon != -46.656 {
		t.Fatalf("identity moved the point: (%v, %v)", lat, lon)
	}
	if _, _, err := store.Transform(ctx, -23.561, -46.656, "MARS-2000"); err == nil {
		t.Fatal("unsupported CRS transformed, want refusal")
	}
	if _, _, err := store.Transform(ctx, math.NaN(), -46.656, "EPSG:4674"); err == nil {
		t.Fatal("NaN transformed, want refusal")
	}
}

func TestHaversineMatchesPostGISScale(t *testing.T) {
	store, dsn := freshStoreWithDSN(t)
	ctx := context.Background()
	latA, lonA := -23.561, -46.656
	latB, lonB := -23.400, -46.500

	goMeters := haversineM(latA, lonA, latB, lonB)
	latA4326, lonA4326, err := store.Transform(ctx, latA, lonA, "EPSG:4674")
	if err != nil {
		t.Fatal(err)
	}
	latB4326, lonB4326, err := store.Transform(ctx, latB, lonB, "EPSG:4674")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("raw connect: %v", err)
	}
	defer conn.Close(ctx)
	var pgMeters float64
	row := conn.QueryRow(ctx,
		`SELECT ST_Distance(
			ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
			ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography)`,
		lonA4326, latA4326, lonB4326, latB4326)
	if err := row.Scan(&pgMeters); err != nil {
		t.Fatalf("postgis distance: %v", err)
	}
	t.Logf("measured corroboration scale: go %.1f m vs postgis %.1f m", goMeters, pgMeters)
	if math.Abs(goMeters-pgMeters)/pgMeters > 0.01 {
		t.Fatalf("haversine diverges from PostGIS: %.1f vs %.1f", goMeters, pgMeters)
	}
}

// reviewFixture builds a single-input PMQC batch: two pointed candidates
// plus one quarantine row, with matching manifest accounting.
func reviewFixture(t *testing.T, first, second BatchCandidate) ([]byte, BatchStreams) {
	t.Helper()
	streams := BatchStreams{
		Candidates: marshalLines(t, first, second),
		Quarantine: marshalLines(t,
			prepQuarantineRow("pmqc", "pmqc-sample.csv:6", "invalid_point", nil)),
	}
	manifest := BatchManifest{
		FormatVersion: FormatBatch,
		RunID:         "44444444-4444-4333-8444-555555555555",
		ParserVersion: "station-prep-v0.2.0",
		PolicyVersion: "station-policy-v1",
		Inputs: []BatchInput{
			{Key: "pmqc", Reference: "pmqc-review.csv", Edition: "synthetic-2024-04", EditionSeq: 7, SHA256: vectorChecksum("raw-review"), Bytes: 0, Rows: 3},
		},
		Counts: map[string]BatchCounts{
			"pmqc": {Input: 3, Accepted: 2, Duplicates: 0, Quarantined: 1},
		},
		Outputs: []BatchOutput{
			streamOutput(PrepAssertionsFile, streams.Assertions),
			streamOutput(PrepCandidatesFile, streams.Candidates),
			streamOutput(PrepQuarantineFile, streams.Quarantine),
		},
	}
	manifest.MunicipalityReference.Reference = "ibge-localidades-v1"
	manifest.MunicipalityReference.ReferenceHash = vectorChecksum("aliases")
	manifest.Completeness.EOFValidated = true
	manifest.Completeness.ExpectedManifest = true
	manifestJSON, err := jsonMarshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return manifestJSON, streams
}

func reviewCandidateRow(sample, cnpj string, lat, lon float64, observed string) BatchCandidate {
	row := prepCandidateRow(sample, cnpj, &lat, &lon)
	row.ObservedAt = observed
	return row
}

func stagedCandidateIDs(t *testing.T, store *PGStore, runID string) map[string]string {
	t.Helper()
	uid, err := mustUUID(runID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.Q.ListRegistryAssertions(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, row := range rows {
		out[row.DisplayName] = uuidString(row.ID)
	}
	return out
}

func TestReviewCandidateIntegrationPromotion(t *testing.T) {
	store := freshStore(t)
	ctx := context.Background()
	manifestJSON, streams := reviewFixture(t,
		reviewCandidateRow("R-101", "48151623000191", -23.561, -46.656, "2024-04-01"),
		reviewCandidateRow("R-102", "48151623000191", -23.5607, -46.6557, "2024-04-10"),
	)
	reports, err := LoadBatch(ctx, store, manifestJSON, streams)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if reports["pmqc"].State != "complete" {
		t.Fatalf("report = %+v", reports["pmqc"])
	}
	ids := stagedCandidateIDs(t, store, reports["pmqc"].RunID)

	decision, err := ReviewCandidate(ctx, store, store, ids["PMQC:R-101"], ids["PMQC:R-102"], "reviewer:int")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if decision.DistanceM <= 0 || decision.DistanceM > ReviewCorroborationM {
		t.Fatalf("distance = %.1f m", decision.DistanceM)
	}
	reviewed, state, err := store.GetAssertion(ctx, decision.AssertionID)
	if err != nil || state != "complete" {
		t.Fatalf("reviewed lookup: %+v, %q, %v", reviewed, state, err)
	}
	if reviewed.LocationQuality != "reviewed" || reviewed.CRS != "EPSG:4326" {
		t.Fatalf("reviewed row = %+v", reviewed)
	}
	if gap := haversineM(-23.561, -46.656, reviewed.Latitude, reviewed.Longitude); gap > 5 {
		t.Fatalf("promoted point %.3f m from candidate evidence", gap)
	}
	run, err := store.GetRun(ctx, SourceReview, "review:"+ids["PMQC:R-101"])
	if err != nil || run.State != "complete" {
		t.Fatalf("review run = %+v, err = %v", run, err)
	}
}

func TestReviewCandidateIntegrationRejections(t *testing.T) {
	cases := map[string]struct {
		second BatchCandidate
	}{
		"far pair": {
			reviewCandidateRow("R-102", "48151623000191", -23.400, -46.500, "2024-04-01"),
		},
		"other establishment": {
			reviewCandidateRow("R-102", "00428184000195", -23.5607, -46.6557, "2024-04-10"),
		},
		"stale pairing": {
			reviewCandidateRow("R-102", "48151623000191", -23.5607, -46.6557, "2024-08-01"),
		},
		"foreign CRS": {
			func() BatchCandidate {
				row := reviewCandidateRow("R-102", "48151623000191", -23.5607, -46.6557, "2024-04-10")
				row.OriginalCRS = "MARS-2000"
				return row
			}(),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := freshStore(t)
			ctx := context.Background()
			manifestJSON, streams := reviewFixture(t,
				reviewCandidateRow("R-101", "48151623000191", -23.561, -46.656, "2024-04-01"),
				tc.second,
			)
			reports, err := LoadBatch(ctx, store, manifestJSON, streams)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			ids := stagedCandidateIDs(t, store, reports["pmqc"].RunID)
			if _, err := ReviewCandidate(ctx, store, store, ids["PMQC:R-101"], ids["PMQC:R-102"], "reviewer:int"); err == nil {
				t.Fatalf("%s promoted, want rejection", name)
			}
			n, err := store.Q.CountCompleteRegistryRuns(ctx, SourceReview)
			if err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Fatalf("%s completed %d review runs, want 0", name, n)
			}
		})
	}
}
