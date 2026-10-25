package jobs

// Maintained regressions for automatic withdrawal classification: only an
// explicit source-side "withdrawn" marker revokes the official identity. A
// temporary outage pauses without revoking, in both full and official-only
// pipelines, and a later published revision recovers normally.

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
)

func TestESPFollowupAutomaticWithdrawalRevokesSourceIdentity(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	handler.OfficialOnly = true
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
	}
	source := writeRefreshExport(t, exportRoot, "review-a", "sha256:"+strings.Repeat("a", 64), "6000", refreshOfficialA)
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: false, RevisionID: refreshOfficialA, Status: "withdrawn"}, nil
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	// Change presentation only: the source revision and lease remain revoked.
	candidate := filepath.Join(t.TempDir(), "rebuilt")
	if _, err := portalgen.Generate(portalgen.Options{SnapshotDir: source.CandidateDir, OutputDir: candidate, OfficialOnly: true, Indexable: true, SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
	_, err := portalgen.Promote(portalgen.PromoteOptions{ArtifactDir: candidate, RootDir: portalRoot, SiteURL: "https://portal.example", OfficialOnly: true})
	if !errors.Is(err, portalgen.ErrOfficialRevoked) {
		t.Fatalf("automatic withdrawal failed to revoke source identity; rebuilt candidate promotion err=%v", err)
	}
	receipt, _ := loadRefreshState(portalRoot)
	if receipt.LastOutcome != "withdrawn" || receipt.WithdrawnReason != "revoked-source" || receipt.WithdrawnRevisionID != refreshOfficialA {
		t.Fatalf("withdrawal receipt = %+v", receipt)
	}
	// The revocation survives a restart: state is reloaded from disk here.
	persisted, ok := portalgen.LoadReleaseState(portalRoot)
	if !ok || len(persisted.RevokedOfficial) != 1 {
		t.Fatalf("revocation missing from persisted history = %+v", persisted)
	}
}

func TestESPFollowupOutagePausesWithoutRevoking(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	handler.OfficialOnly = true
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
	}
	source := writeRefreshExport(t, exportRoot, "review-a", "sha256:"+strings.Repeat("a", 64), "6000", refreshOfficialA)
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	// Temporary outage without a source-side withdrawn marker: pause only.
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: false, RevisionID: refreshOfficialA, Status: "unavailable"}, nil
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	receipt, _ := loadRefreshState(portalRoot)
	if receipt.LastOutcome != "withdrawn" || receipt.WithdrawnReason != "" {
		t.Fatalf("outage became a revocation: %+v", receipt)
	}
	persisted, ok := portalgen.LoadReleaseState(portalRoot)
	if !ok {
		t.Fatal("release state missing")
	}
	if len(persisted.RevokedOfficial) != 0 {
		t.Fatalf("outage recorded a revocation: %+v", persisted.RevokedOfficial)
	}
	// The same content stays eligible: republication after recovery works.
	candidate := filepath.Join(t.TempDir(), "rebuilt")
	if _, err := portalgen.Generate(portalgen.Options{SnapshotDir: source.CandidateDir, OutputDir: candidate, OfficialOnly: true}); err != nil {
		t.Fatal(err)
	}
	// Same content hash as the withdrawn release: still refused as
	// withdrawn, but the identity is NOT revoked.
	if _, err := portalgen.Promote(portalgen.PromoteOptions{ArtifactDir: candidate, RootDir: portalRoot, SiteURL: "https://portal.example", OfficialOnly: true}); !errors.Is(err, portalgen.ErrReleaseWithdrawn) {
		t.Fatalf("paused content must stay withdrawn, not revoked: %v", err)
	}
}

func TestESPFollowupFullPipelineRevokesOnWithdrawn(t *testing.T) {
	exportRoot, portalRoot, handler, _ := refreshFixture(t)
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
	}
	writeRefreshExport(t, exportRoot, "review-a", "sha256:"+strings.Repeat("a", 64), "6000", refreshOfficialA)
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	handler.Status = func(context.Context) (SourceStatus, error) {
		return SourceStatus{HasPublished: false, RevisionID: refreshOfficialA, Status: "withdrawn"}, nil
	}
	if err := runRefresh(t, handler); err != nil {
		t.Fatal(err)
	}
	receipt, _ := loadRefreshState(portalRoot)
	if receipt.WithdrawnReason != "revoked-source" {
		t.Fatalf("full-pipeline withdrawal receipt = %+v", receipt)
	}
	persisted, ok := portalgen.LoadReleaseState(portalRoot)
	if !ok || len(persisted.RevokedOfficial) != 1 {
		t.Fatalf("full-pipeline revocation missing = %+v", persisted)
	}
}
