package registry

// RST-05 loader unit tests: manifest/stream validation, idempotent replay
// and failure behavior against the in-memory fake. Real-PostGIS coverage
// lives in loadbatch_integration_test.go. Every vector is synthetic with
// owned [RST05-TEST] markers; checksums are content hashes of the
// vectors themselves.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func vectorChecksum(seed string) string {
	sum := sha256.Sum256([]byte("rst05:" + seed))
	return hex.EncodeToString(sum[:])
}

func stringPtr(value string) *string { return &value }

// Standard vectors: registry input of 4 rows (3 accepted incl. one
// superseded history link, 1 in-batch duplicate, 1 quarantined) and a
// PMQC input of 3 rows (2 accepted incl. one pointless candidate,
// 1 quarantined).
func prepAssertionRow(sourceKey, checksum, simp string, supersedes *string) BatchAssertion {
	return BatchAssertion{
		SchemaVersion:          SchemaAssertion,
		Source:                 "registry-13col",
		SourceKey:              sourceKey,
		Checksum:               checksum,
		SimpRef:                simp,
		AuthorizationRef:       "PRC-2024-0001",
		BusinessNameRaw:        "[RST05-TEST] POSTO VETOR LTDA",
		BusinessNameNormalized: "[RST05-TEST] POSTO VETOR LTDA",
		AddressRaw:             "AV VETOR 100",
		AddressNormalized:      map[string]string{"street": "AV VETOR", "number": "100"},
		UF:                     "SP",
		MunicipalityNameRaw:    "SÃO PAULO",
		MunicipalityIBGE:       "3550308",
		BrandRaw:               "BRANCA",
		PublishedAt:            "2024-03-01",
		EffectiveAt:            "2024-03-10",
		AuthState:              "unknown",
		Eligibility:            "pending",
		AuthEvidence:           "operating-membership 2024-03-01 snapshot; not an authorization grant",
		LocationQuality:        "unknown",
		Supersedes:             supersedes,
	}
}

func prepCandidateRow(sample, sourceKey string, latitude, longitude *float64) BatchCandidate {
	return BatchCandidate{
		SchemaVersion: SchemaCandidate,
		Source:        "pmqc",
		SourceKey:     sourceKey,
		SampleRef:     sample,
		ObservedAt:    "2024-04-01",
		Latitude:      latitude,
		Longitude:     longitude,
		OriginalCRS:   "EPSG:4674",
		EvidenceRef:   "pmqc-sample.csv#" + sample,
		ReviewState:   "pending",
	}
}

func prepQuarantineRow(source, locator, reason string, sourceKey *string) BatchQuarantine {
	return BatchQuarantine{
		SchemaVersion: SchemaQuarantine,
		Source:        source,
		RowLocator:    locator,
		Reason:        reason,
		Detail:        "synthetic triage",
		SourceKey:     sourceKey,
	}
}

func marshalLines(t *testing.T, rows ...any) []byte {
	t.Helper()
	var out []byte
	for _, row := range rows {
		raw, err := json.Marshal(row)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out = append(out, raw...)
		out = append(out, '\n')
	}
	return out
}

func prepStreams(t *testing.T) BatchStreams {
	t.Helper()
	newChecksum := vectorChecksum("rst05-new")
	oldChecksum := vectorChecksum("rst05-old")
	assertions := marshalLines(t,
		prepAssertionRow("48151623000191", vectorChecksum("rst05-a1"), "SIMP-0001", nil),
		prepAssertionRow("48151623000191", vectorChecksum("rst05-a1"), "SIMP-0001", nil),
		prepAssertionRow("48151623000191", oldChecksum, "SIMP-0000", stringPtr(newChecksum)),
		prepAssertionRow("48151623000191", newChecksum, "SIMP-0002", nil),
	)
	latitude, longitude := -23.561, -46.656
	candidates := marshalLines(t,
		prepCandidateRow("V-001", "00428184000195", &latitude, &longitude),
		prepCandidateRow("V-002", "00428184000195", nil, nil),
	)
	quarantine := marshalLines(t,
		prepQuarantineRow("registry-13col", "registry-13col-sample.csv:8", "invalid_cnpj", nil),
		prepQuarantineRow("pmqc", "pmqc-sample.csv:6", "invalid_point", stringPtr("12ABC34501DE35")),
	)
	return BatchStreams{Assertions: assertions, Candidates: candidates, Quarantine: quarantine}
}

func streamOutput(path string, raw []byte) BatchOutput {
	sum := sha256.Sum256(raw)
	return BatchOutput{
		Path:   path,
		SHA256: hex.EncodeToString(sum[:]),
		Bytes:  uint64(len(raw)),
		Rows:   strings.Count(string(raw), "\n"),
	}
}

func prepManifest(t *testing.T, streams BatchStreams) []byte {
	t.Helper()
	manifest := BatchManifest{
		FormatVersion: FormatBatch,
		RunID:         "11111111-2222-4333-8444-555555555555",
		ParserVersion: "station-prep-v0.2.0",
		PolicyVersion: "station-policy-v1",
		Inputs: []BatchInput{
			{Key: "registry-13col", Reference: "registry-13col-sample.csv", Edition: "synthetic-2024-03", EditionSeq: 3, SHA256: vectorChecksum("raw-registry"), Bytes: 3903, Rows: 5},
			{Key: "pmqc", Reference: "pmqc-sample.csv", Edition: "synthetic-2024-04", EditionSeq: 4, SHA256: vectorChecksum("raw-pmqc"), Bytes: 573, Rows: 3},
		},
		Counts: map[string]BatchCounts{
			"registry-13col": {Input: 5, Accepted: 3, Duplicates: 1, Quarantined: 1},
			"pmqc":           {Input: 3, Accepted: 2, Duplicates: 0, Quarantined: 1},
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
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return raw
}

// linkRecorder wraps a Store and records succession links.
type linkRecorder struct {
	Store
	links map[string]string
}

func (s *linkRecorder) SetAssertionSuperseded(_ context.Context, assertionID, supersededBy string) error {
	s.links[assertionID] = supersededBy
	return nil
}

func stagedChecksums(store *fakeStore, runID string) map[string]bool {
	out := map[string]bool{}
	for _, assertion := range store.assertions {
		if assertion.RunID() == runID {
			out[assertion.Checksum] = true
		}
	}
	return out
}

func TestLoadBatchStagesValidBatch(t *testing.T) {
	streams := prepStreams(t)
	manifestJSON := prepManifest(t, streams)
	inner := newFakeStore()
	store := &linkRecorder{Store: inner, links: map[string]string{}}

	reports, err := LoadBatch(context.Background(), store, manifestJSON, streams)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	registryReport, ok := reports["registry-13col"]
	if !ok || registryReport.State != "complete" {
		t.Fatalf("registry report = %+v", registryReport)
	}
	if registryReport.Accepted != 3 || registryReport.Duplicates != 1 || registryReport.Rejected != 1 {
		t.Fatalf("registry counts = %+v", registryReport)
	}
	pmqcReport, ok := reports["pmqc"]
	if !ok || pmqcReport.State != "complete" {
		t.Fatalf("pmqc report = %+v", pmqcReport)
	}
	if pmqcReport.Accepted != 2 || pmqcReport.Duplicates != 0 || pmqcReport.Rejected != 1 {
		t.Fatalf("pmqc counts = %+v", pmqcReport)
	}
	if got := len(stagedChecksums(inner, registryReport.RunID)); got != 3 {
		t.Fatalf("staged registry assertions = %d, want 3", got)
	}
	if got := len(stagedChecksums(inner, pmqcReport.RunID)); got != 2 {
		t.Fatalf("staged pmqc assertions = %d, want 2", got)
	}
	// Succession history links the older checksum to the newer identity.
	if len(store.links) != 1 {
		t.Fatalf("supersede links = %v", store.links)
	}
	for older, newer := range store.links {
		if older == newer {
			t.Fatal("supersede link is a self loop")
		}
	}
}

func TestLoadBatchRejectsBeforeAnyRun(t *testing.T) {
	cases := map[string]func(t *testing.T, manifest []byte, streams BatchStreams) ([]byte, BatchStreams){
		"bad format": func(t *testing.T, manifest []byte, streams BatchStreams) ([]byte, BatchStreams) {
			t.Helper()
			var decoded BatchManifest
			if err := json.Unmarshal(manifest, &decoded); err != nil {
				t.Fatal(err)
			}
			decoded.FormatVersion = "station-batch-v9"
			raw, _ := json.Marshal(decoded)
			return raw, streams
		},
		"tampered stream": func(t *testing.T, manifest []byte, streams BatchStreams) ([]byte, BatchStreams) {
			t.Helper()
			streams.Assertions[10] ^= 0xFF
			return manifest, streams
		},
		"count mismatch": func(t *testing.T, manifest []byte, streams BatchStreams) ([]byte, BatchStreams) {
			t.Helper()
			var decoded BatchManifest
			if err := json.Unmarshal(manifest, &decoded); err != nil {
				t.Fatal(err)
			}
			counts := decoded.Counts["registry-13col"]
			counts.Accepted++
			decoded.Counts["registry-13col"] = counts
			raw, _ := json.Marshal(decoded)
			return raw, streams
		},
		"bad CNPJ": func(t *testing.T, manifest []byte, streams BatchStreams) ([]byte, BatchStreams) {
			t.Helper()
			row := prepAssertionRow("123", vectorChecksum("rst05-bad"), "SIMP-9", nil)
			streams.Assertions = marshalLines(t, row)
			outputs := []BatchOutput{
				streamOutput(PrepAssertionsFile, streams.Assertions),
				streamOutput(PrepCandidatesFile, streams.Candidates),
				streamOutput(PrepQuarantineFile, streams.Quarantine),
			}
			var decoded BatchManifest
			if err := json.Unmarshal(manifest, &decoded); err != nil {
				t.Fatal(err)
			}
			decoded.Outputs = outputs
			raw, _ := json.Marshal(decoded)
			return raw, streams
		},
		"unknown reason": func(t *testing.T, manifest []byte, streams BatchStreams) ([]byte, BatchStreams) {
			t.Helper()
			row := prepQuarantineRow("pmqc", "f:1", "guessed_reason", nil)
			streams.Quarantine = marshalLines(t, row,
				prepQuarantineRow("registry-13col", "f:2", "invalid_cnpj", nil))
			outputs := []BatchOutput{
				streamOutput(PrepAssertionsFile, streams.Assertions),
				streamOutput(PrepCandidatesFile, streams.Candidates),
				streamOutput(PrepQuarantineFile, streams.Quarantine),
			}
			var decoded BatchManifest
			if err := json.Unmarshal(manifest, &decoded); err != nil {
				t.Fatal(err)
			}
			decoded.Outputs = outputs
			raw, _ := json.Marshal(decoded)
			return raw, streams
		},
		"reviewed smuggling": func(t *testing.T, manifest []byte, streams BatchStreams) ([]byte, BatchStreams) {
			t.Helper()
			row := prepAssertionRow("48151623000191", vectorChecksum("rst05-sneak"), "SIMP-9", nil)
			row.LocationQuality = "reviewed"
			streams.Assertions = marshalLines(t, row)
			outputs := []BatchOutput{
				streamOutput(PrepAssertionsFile, streams.Assertions),
				streamOutput(PrepCandidatesFile, streams.Candidates),
				streamOutput(PrepQuarantineFile, streams.Quarantine),
			}
			var decoded BatchManifest
			if err := json.Unmarshal(manifest, &decoded); err != nil {
				t.Fatal(err)
			}
			decoded.Outputs = outputs
			raw, _ := json.Marshal(decoded)
			return raw, streams
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			streams := prepStreams(t)
			manifestJSON := prepManifest(t, streams)
			manifestJSON, streams = mutate(t, manifestJSON, streams)
			store := newFakeStore()
			if _, err := LoadBatch(context.Background(), store, manifestJSON, streams); err == nil {
				t.Fatalf("%s loaded, want rejection", name)
			}
			if len(store.runs) != 0 {
				t.Fatalf("%s created %d runs, want zero", name, len(store.runs))
			}
		})
	}
}

func TestLoadBatchReplayConverges(t *testing.T) {
	streams := prepStreams(t)
	manifestJSON := prepManifest(t, streams)
	store := newFakeStore()

	first, err := LoadBatch(context.Background(), store, manifestJSON, streams)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := LoadBatch(context.Background(), store, manifestJSON, streams)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	for key, before := range first {
		after, ok := second[key]
		if !ok || after.RunID != before.RunID || after.Accepted != before.Accepted {
			t.Fatalf("replay diverged for %q: %+v vs %+v", key, before, after)
		}
	}
	if got := len(stagedChecksums(store, first["registry-13col"].RunID)); got != 3 {
		t.Fatalf("registry assertions after replay = %d, want 3", got)
	}
}

func TestLoadBatchStageErrorFailsRun(t *testing.T) {
	streams := prepStreams(t)
	manifestJSON := prepManifest(t, streams)
	store := newFakeStore()
	store.failStage = true

	if _, err := LoadBatch(context.Background(), store, manifestJSON, streams); err == nil {
		t.Fatal("load with failing store succeeded, want error")
	}
	for snapshot, report := range store.runs {
		if report.State == "complete" {
			t.Fatalf("run %q completed despite store failure", snapshot)
		}
	}
}

func TestLoadBatchLiveCountMismatchFailsRun(t *testing.T) {
	streams := prepStreams(t)
	manifestJSON := prepManifest(t, streams)
	// A store that silently drops new rows: validation passes against the
	// manifest, but live staging diverges, so the run must fail instead
	// of completing with a partial-publication claim.
	inner := newFakeStore()
	store := &dropStore{Store: inner}
	if _, err := LoadBatch(context.Background(), store, manifestJSON, streams); err == nil {
		t.Fatal("silently lossy batch loaded, want failure")
	}
	for snapshot, report := range inner.runs {
		if report.State != "failed" {
			t.Fatalf("run %q is %q, want failed", snapshot, report.State)
		}
	}
}

// dropStore stages nothing but reports success, simulating a silently
// lossy backend behind the loader.
type dropStore struct {
	Store
}

func (s *dropStore) StageAssertion(_ context.Context, _ Assertion) (bool, error) {
	return false, nil
}
