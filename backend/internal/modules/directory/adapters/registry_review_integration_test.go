//go:build integration

package adapters

// RST-06 end to end on real disposable PostGIS: prepared candidates load,
// corroborated promotion stages a reviewed assertion, and the existing
// reconciliation projects it as the station's reviewed point. Vectors are
// synthetic with owned [RST06-E2E-TEST] markers.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
)

func e2eCandidate(sample, cnpj string, lat, lon float64, observed string) registry.BatchCandidate {
	return registry.BatchCandidate{
		SchemaVersion: "station-coordinate-candidate-v1",
		Source:        "pmqc",
		SourceKey:     cnpj,
		SampleRef:     sample,
		ObservedAt:    observed,
		Latitude:      &lat,
		Longitude:     &lon,
		OriginalCRS:   "EPSG:4674",
		EvidenceRef:   "pmqc-e2e.csv#" + sample,
		ReviewState:   "pending",
	}
}

func e2eOutput(path string, raw []byte) registry.BatchOutput {
	sum := sha256.Sum256(raw)
	return registry.BatchOutput{
		Path:   path,
		SHA256: hex.EncodeToString(sum[:]),
		Bytes:  uint64(len(raw)),
		Rows:   strings.Count(string(raw), "\n"),
	}
}

func TestReviewReconcileIntegrationProjectsReviewedPoint(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()

	latA, lonA := -23.561, -46.656
	latB, lonB := -23.5607, -46.6557
	lines := func(rows ...registry.BatchCandidate) []byte {
		var out []byte
		for _, row := range rows {
			raw, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, raw...)
			out = append(out, '\n')
		}
		return out
	}
	quarantine := registry.BatchQuarantine{
		SchemaVersion: "station-quarantine-v1",
		Source:        "pmqc",
		RowLocator:    "pmqc-e2e.csv:4",
		Reason:        "invalid_point",
		Detail:        "synthetic triage",
	}
	quarantineRaw, err := json.Marshal(quarantine)
	if err != nil {
		t.Fatal(err)
	}
	quarantineRaw = append(quarantineRaw, '\n')
	streams := registry.BatchStreams{
		Candidates: lines(
			e2eCandidate("E-201", "48151623000191", latA, lonA, "2024-04-01"),
			e2eCandidate("E-202", "48151623000191", latB, lonB, "2024-04-10"),
		),
		Quarantine: quarantineRaw,
	}
	manifest := registry.BatchManifest{
		FormatVersion: "station-batch-v1",
		RunID:         "55555555-5555-4333-8444-555555555555",
		ParserVersion: "station-prep-v0.2.0",
		PolicyVersion: "station-policy-v1",
		Inputs: []registry.BatchInput{
			{Key: "pmqc", Reference: "pmqc-e2e.csv", Edition: "synthetic-e2e", EditionSeq: 9, SHA256: strings.Repeat("e2", 32), Bytes: 0, Rows: 3},
		},
		Counts: map[string]registry.BatchCounts{
			"pmqc": {Input: 3, Accepted: 2, Duplicates: 0, Quarantined: 1},
		},
		Outputs: []registry.BatchOutput{
			e2eOutput("assertions.jsonl", streams.Assertions),
			e2eOutput("candidates.jsonl", streams.Candidates),
			e2eOutput("quarantine.jsonl", streams.Quarantine),
		},
	}
	manifest.MunicipalityReference.Reference = "ibge-localidades-v1"
	manifest.MunicipalityReference.ReferenceHash = strings.Repeat("f2", 32)
	manifest.Completeness.EOFValidated = true
	manifest.Completeness.ExpectedManifest = true
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := registry.LoadBatch(ctx, store, manifestJSON, streams)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if reports["pmqc"].State != "complete" {
		t.Fatalf("report = %+v", reports["pmqc"])
	}
	ids := map[string]string{}
	assertions, err := store.ListAssertions(ctx, reports["pmqc"].RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, assertion := range assertions {
		ids[assertion.DisplayName] = assertion.ID
	}

	decision, err := registry.ReviewCandidate(ctx, store, store, ids["PMQC:E-201"], ids["PMQC:E-202"], "reviewer:e2e")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	recon, err := registry.ReconcileRun(ctx, store, canon, registry.SourceReview, "review:"+ids["PMQC:E-201"])
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if recon.Reconciled != 1 || recon.Skipped != 0 {
		t.Fatalf("report = %+v", recon)
	}

	var quality string
	var distance float64
	err = pool.QueryRow(ctx, `
		SELECT s.current_quality,
			ST_Distance(s.current_point, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography)
		FROM directory_stations AS s
		WHERE s.id = (SELECT station_id FROM registry_assertions WHERE id = $3::uuid)`,
		lonA, latA, decision.AssertionID,
	).Scan(&quality, &distance)
	if err != nil {
		t.Fatalf("projection lookup: %v", err)
	}
	if quality != "reviewed" {
		t.Fatalf("current quality = %q, want reviewed", quality)
	}
	if distance > registry.ReviewCorroborationM {
		t.Fatalf("projected point %.1f m from candidate evidence", distance)
	}
	t.Logf("measured promotion projection gap: %.3f m (bound 150 m)", distance)
}
