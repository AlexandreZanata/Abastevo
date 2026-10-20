//go:build integration && qualitysource

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
)

// Explicit manual live-source smoke only, excluded from deterministic CI.
// The immutable expected source identity belongs to the acceptance candidate;
// a changed remote resource requires fresh reconciliation, never a relaxed test.
func TestOfficialFineWorkerNetworkBoundary(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH")
	if binary == "" {
		t.Fatal("trusted fine CLI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	refs, e := fetchFineListing(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var selected native.FineRef
	for _, ref := range refs {
		if ref.Period == "2025" {
			selected = ref
		}
	}
	if selected.URL == "" {
		t.Fatal("verified 2025 href no longer listed")
	}
	root := t.TempDir()
	file, e := nativeFineFetch(root)(ctx, selected.URL)
	if e != nil {
		t.Fatal(e)
	}
	closed := false
	defer func() {
		if !closed && file.Close != nil {
			if e := file.Close(); e != nil {
				t.Error(e)
			}
		}
	}()
	if file.Identity.SHA256 != "277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96" || file.Identity.Bytes != 450504 {
		t.Fatal("remote fine source changed; reconcile a new source candidate")
	}
	batch, e := (native.FineRunner{Binary: binary, WorkspaceRoot: root}).Prepare(ctx, native.FineRequest{Path: file.Path, URL: selected.URL, Edition: selected.Period, Identity: file.Identity})
	if e != nil || batch.Counts.Input != 3395 || batch.Counts.Accepted != 3375 || batch.Counts.Duplicates != 0 || batch.Counts.Quarantined != 20 || batch.ExtractionMonth != "2026-01" {
		t.Fatal("live source reconciliation failed", e)
	}
	if e = file.Close(); e != nil {
		t.Fatal(e)
	}
	closed = true
	children, e := os.ReadDir(root)
	if e != nil || len(children) != 0 {
		t.Fatal("private live source retained", e)
	}
	t.Logf("listed verified fine resources=%d; immutable 2025 source reconciled: 3395=3375+0+20", len(refs))
}

func TestOfficialHistoricalFineWorkerNetworkBoundary(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH")
	if binary == "" {
		t.Fatal("trusted historical CLI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	refs, e := fetchFineListing(ctx)
	if e != nil || len(refs) != 7 {
		t.Fatal("verified inventory changed", len(refs), e)
	}
	var selected native.FineRef
	for _, ref := range refs {
		if ref.Period == "2021" {
			selected = ref
		}
	}
	if selected.URL == "" {
		t.Fatal("misnamed CSV href no longer listed")
	}
	root := t.TempDir()
	file, e := nativeFineFetch(root)(ctx, selected.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if file.Close != nil {
			if e := file.Close(); e != nil {
				t.Error(e)
			}
		}
	}()
	if file.Identity.SHA256 != "85122d0d1ae999173bac2c772ffb4e14ec414aec3c2f8e30bf7203187936a92d" || file.Identity.Bytes != 720842 {
		t.Fatal("remote history changed; reconcile new candidate")
	}
	batch, e := (native.FineHistoryRunner{Binary: binary, WorkspaceRoot: root}).Prepare(ctx, native.FineRequest{Path: file.Path, URL: selected.URL, Edition: selected.Period, Identity: file.Identity})
	if e != nil || batch.Counts.Input != 4976 || batch.Counts.Accepted != 4929 || batch.Counts.Quarantined != 47 || batch.MetadataRecords != 3 || batch.Clock.Value != "2022-06" {
		t.Fatal("live misnamed CSV reconciliation", e)
	}
	if e = file.Close(); e != nil {
		t.Fatal(e)
	}
	file.Close = nil
	children, e := os.ReadDir(root)
	if e != nil || len(children) != 0 {
		t.Fatal("private network file retained", e)
	}
	t.Log("seven verified resource hrefs;2021 exact source reconciled:4976=4929+44CPF+3metadata")
}

func TestOfficialSevenFileWorkerPipeline(t *testing.T) {
	sourceDir := os.Getenv("ANPFUEL_TEST_FINE_HISTORY_SOURCE_DIR")
	if sourceDir == "" {
		t.Fatal("private pinned source directory required")
	}
	binary := os.Getenv("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH")
	historyBinary := os.Getenv("ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH")
	if binary == "" || historyBinary == "" {
		t.Fatal("both trusted CLIs required")
	}
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	root := t.TempDir()
	im := nativeFineImport(p, binary, historyBinary, root)
	var totalInput, totalAccepted, totalQuarantine, totalMetadata int
	for _, tc := range []struct {
		period, file, sha, clock              string
		bytes                                 int64
		input, accepted, quarantine, metadata int
	}{
		{"2016-2019", "multas-aplicadas-2016a2019.csv", "15ccbf54367881946104dc6baf65c246b6189638cb306ab06662570451989f25", "2020-02-06", 2301471, 16440, 16290, 150, 0},
		{"2020", "multas-aplicadas-2020.csv", "aef7fce874b29db234ab7779019c0bc9fcf13d1bdb2de7d59bbf09f51e32654f", "2021-02", 304268, 2026, 2001, 25, 3},
		{"2021", "multas-2021-junho22.xlsx", "85122d0d1ae999173bac2c772ffb4e14ec414aec3c2f8e30bf7203187936a92d", "2022-06", 720842, 4976, 4929, 47, 3},
		{"2022", "multas-aplicadas-2022.csv", "5ee3df1d9188f273fbf3fbeb7e95f2cea0bc2171fdda23bc4af3cce9478251b5", "2022-04", 436282, 2996, 2970, 26, 0},
		{"2023", "multas-aplicadas-2023.csv", "3865025eb04a82ee5d937d703f063d617843eb7a9f5cbf4c18f989caa33839bb", "2024-06", 445861, 3074, 3053, 21, 0},
		{"2024", "multas-aplicadas-2024.csv", "0723561b182ac0cc3d031a77050049844e3b29ec098472383b0229051cb67dcd", "2025-03", 416653, 3004, 2994, 10, 0},
		{"2025", "multas-aplicadas-2025.csv", "277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96", "2026-01", 450504, 3395, 3375, 20, 0},
	} {
		t.Run(tc.period, func(t *testing.T) {
			filename := tc.file
			if tc.period == "2025" {
				filename = "fines-2025.csv"
			}
			raw, e := os.ReadFile(filepath.Join(sourceDir, filename))
			if e != nil {
				t.Fatal("private candidate unavailable")
			}
			sum := sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != tc.sha || int64(len(raw)) != tc.bytes {
				t.Fatal("private candidate changed")
			}
			im.Fetch = func(_ context.Context, url string) (qualityjobs.FileArtifact, error) {
				if url != "https://www.gov.br"+native.FineResourcePrefix+tc.file {
					t.Fatal("unexpected candidate URL")
				}
				path := filepath.Join(root, "download")
				if e := os.WriteFile(path, raw, 0600); e != nil {
					return qualityjobs.FileArtifact{}, e
				}
				return qualityjobs.FileArtifact{Path: path, Identity: prepared.InputIdentity{SHA256: tc.sha, Bytes: tc.bytes}, Close: func() error { return os.Remove(path) }}, nil
			}
			payload, e := json.Marshal(qualityjobs.ImportPayload{Version: 1, Source: "sanctions", Stream: "sanction-v1", Edition: tc.period, URL: "https://www.gov.br" + native.FineResourcePrefix + tc.file, SHA256: tc.sha})
			if e != nil {
				t.Fatal(e)
			}
			for range 2 {
				if e := im.Handle(ctx, platformjobs.Job{Payload: payload}); e != nil {
					t.Fatal("whole worker attempt/replay", e)
				}
			}
			r := qualityread.NewReader(p)
			_, _, lease, e := r.Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 100)
			if e != nil || lease.Edition != tc.period || lease.Counts.Input != tc.input || lease.Counts.Accepted != tc.accepted || lease.Counts.Quarantined != tc.quarantine || (lease.SourceMetadataRecords == nil || *lease.SourceMetadataRecords != tc.metadata) || lease.SourceClockValue != tc.clock {
				t.Fatal("worker census/clock mismatch", e)
			}
			var count int
			afterNative, afterID := "", ""
			seen := map[string]bool{}
			for {
				rows, next, _, e := r.AtRevision(lease.RevisionID).Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", afterNative, afterID, 100)
				if e != nil {
					t.Fatal(e)
				}
				for _, row := range rows {
					if seen[row.ID] {
						t.Fatal("worker traversal repeated a record")
					}
					seen[row.ID] = true
				}
				count += len(rows)
				if next == "" {
					break
				}
				parts := strings.SplitN(next, "|", 2)
				if len(parts) != 2 {
					t.Fatal("invalid internal continuation")
				}
				afterNative, afterID = parts[0], parts[1]
			}
			if count != tc.accepted {
				t.Fatal("worker public traversal lost records", count)
			}
			children, e := os.ReadDir(root)
			if e != nil || len(children) != 0 {
				t.Fatal("worker retained private candidate", e)
			}
			totalInput += tc.input
			totalAccepted += tc.accepted
			totalQuarantine += tc.quarantine
			totalMetadata += tc.metadata
		})
	}
	var revisions, links, watermarks, recompute int
	if e := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_links),(SELECT count(*) FROM quality_watermarks),(SELECT count(*) FROM job_queue WHERE kind='quality-recompute')`).Scan(&revisions, &links, &watermarks, &recompute); e != nil || revisions != 7 || links != 0 || watermarks != 1 || recompute != 7 {
		t.Fatal("worker replay/association/ack mismatch", revisions, links, watermarks, recompute, e)
	}
	if totalInput != 35911 || totalAccepted != 35612 || totalQuarantine != 299 || totalMetadata != 6 {
		t.Fatal("seven-source denominator mismatch")
	}
	t.Log("whole seven-resource runtime pipeline/replay/full traversal:35911=35612accepted+293CPF+6metadata;zero station links")
}
