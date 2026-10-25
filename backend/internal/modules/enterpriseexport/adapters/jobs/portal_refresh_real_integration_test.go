//go:build integration

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func readRealOfficialPin(t *testing.T, dir, revisionOverride string) *OfficialPin {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "official-records-2025.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		RevisionID           string `json:"revision_id"`
		LeaseID              string `json:"lease_id"`
		Edition              string `json:"edition"`
		SourceURL            string `json:"source_url"`
		SourceChecksum       string `json:"source_checksum"`
		SourcePeriod         string `json:"source_period"`
		SourceClockPrecision string `json:"source_clock_precision"`
		SourceClockValue     string `json:"source_clock_value"`
		Counts               struct {
			Input, Accepted, Quarantined int
		} `json:"counts"`
		AssociationCensus struct {
			Linked, Unmatched, Ambiguous, WrongBranch, SuccessionReview int
		} `json:"association_census"`
		Rows          int    `json:"rows"`
		PayloadSHA256 string `json:"payload_sha256"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if revisionOverride != "" {
		manifest.RevisionID = revisionOverride
	}
	return &OfficialPin{
		RevisionID: manifest.RevisionID, LeaseID: manifest.LeaseID, Edition: manifest.Edition,
		SourceURL: manifest.SourceURL, SourceChecksum: manifest.SourceChecksum,
		SourcePeriod: manifest.SourcePeriod, SourceClockPrecision: manifest.SourceClockPrecision,
		SourceClockValue: manifest.SourceClockValue,
		Input:            manifest.Counts.Input, Accepted: manifest.Counts.Accepted,
		Quarantined: manifest.Counts.Quarantined, Rows: manifest.Rows,
		Linked: manifest.AssociationCensus.Linked, Unmatched: manifest.AssociationCensus.Unmatched,
		Ambiguous: manifest.AssociationCensus.Ambiguous, WrongBranch: manifest.AssociationCensus.WrongBranch,
		SuccessionReview: manifest.AssociationCensus.SuccessionReview,
		PayloadSHA256:    manifest.PayloadSHA256,
	}
}

func copyRealOfficial(t *testing.T, sourceDir, candidate string) {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(sourceDir, "official-records-2025.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(sourceDir, "official-records-2025.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "official-records.jsonl"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "official-records-manifest.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshRealOfficialSnapshotAndWithdrawal(t *testing.T) {
	officialDir := os.Getenv("ANPFUEL_TEST_REAL_OFFICIAL_DIR")
	if officialDir == "" {
		t.Skip("ANPFUEL_TEST_REAL_OFFICIAL_DIR is not set")
	}
	exportRoot := t.TempDir()
	portalRoot := filepath.Join(t.TempDir(), "portal")
	first := writeRefreshExport(t, exportRoot, "real-a", "sha256:"+strings.Repeat("7", 64), "6000", "")
	copyRealOfficial(t, officialDir, first.CandidateDir)
	first.Official = readRealOfficialPin(t, officialDir, "")
	if err := WriteState(exportRoot, first); err != nil {
		t.Fatal(err)
	}
	handler := Refresh{
		ExportRoot: exportRoot, PortalRoot: portalRoot, SiteURL: "https://portal.example",
		Indexable: true, SearchScriptPath: os.Getenv("ANPFUEL_TEST_PORTAL_SEARCH_JS"),
	}
	revision := first.Official.RevisionID
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: revision, Status: "published"}, nil
	}
	ctx := context.Background()
	if err := handler.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	released, _, err := portalgen.CurrentReleaseHeader(portalRoot)
	if err != nil || released.OfficialRevisionID != revision {
		t.Fatalf("real refresh = %+v %v", released, err)
	}
	t.Logf("real refresh release: pages=%d files=%d bytes=%d content=%s official=%s",
		released.Pages, released.Files, released.Bytes, released.ContentSHA256, released.OfficialRevisionID)
	if err := handler.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	receipt, _ := loadRefreshState(portalRoot)
	if receipt.LastOutcome != "noop" {
		t.Fatalf("real no-op = %+v", receipt)
	}
	second := writeRefreshExport(t, exportRoot, "real-b", "sha256:"+strings.Repeat("8", 64), "6500", "")
	copyRealOfficial(t, officialDir, second.CandidateDir)
	second.Official = readRealOfficialPin(t, officialDir, "")
	if err := WriteState(exportRoot, second); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	corrected, releases, err := portalgen.CurrentReleaseHeader(portalRoot)
	if err != nil || corrected.ContentSHA256 == released.ContentSHA256 || len(releases.Releases) != 2 {
		t.Fatalf("real correction = %+v %+v %v", corrected, releases.Releases, err)
	}
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: false, RevisionID: revision, Status: "withdrawn"}, nil
	}
	if err := handler.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := portalgen.CurrentReleaseHeader(portalRoot); !errors.Is(err, portalgen.ErrNoCurrentRelease) {
		t.Fatalf("withdrawn real release still current: %v", err)
	}
	receipt, _ = loadRefreshState(portalRoot)
	if receipt.LastOutcome != "withdrawn" || receipt.WithdrawnRevisionID != revision {
		t.Fatalf("withdrawal receipt = %+v", receipt)
	}
}
