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

func TestESPFollowupPauseRecoversSameSource(t *testing.T) {
	for _, officialOnly := range []bool{false, true} {
		name := "full"
		if officialOnly {
			name = "official-only"
		}
		t.Run(name, func(t *testing.T) {
			exportRoot, portalRoot, handler, _ := refreshFixture(t)
			handler.OfficialOnly = officialOnly
			handler.Status = func(context.Context) (SourceStatus, error) {
				return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
			}
			writeRefreshExport(t, exportRoot, "review-a", "sha256:"+strings.Repeat("a", 64), "6000", refreshOfficialA)
			if err := runRefresh(t, handler); err != nil {
				t.Fatal(err)
			}
			first, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", officialOnly)
			if err != nil || first.ContentSHA256 == "" {
				t.Fatalf("first release = %+v %v", first, err)
			}
			// Temporary outage without a source-side withdrawn marker: pause
			// only. The pointer goes away and no identity is revoked.
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
			if !ok || len(persisted.RevokedOfficial) != 0 {
				t.Fatalf("outage recorded a revocation: %+v", persisted.RevokedOfficial)
			}
			if _, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", officialOnly); !errors.Is(err, portalgen.ErrNoCurrentRelease) {
				t.Fatalf("paused pointer still serves content: %v", err)
			}
			// A failure while recovering must preserve the pause, not half
			// publish or invent a revision.
			failing := handler
			failing.Promote = func(portalgen.PromoteOptions) (portalgen.ReleaseState, error) {
				return portalgen.ReleaseState{}, errors.New("synthetic recovery failure")
			}
			failing.Status = func(context.Context) (SourceStatus, error) {
				return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
			}
			if err := runRefresh(t, failing); err == nil {
				t.Fatal("failed recovery was reported as success")
			}
			if receipt, _ := loadRefreshState(portalRoot); receipt.LastOutcome != "failed" {
				t.Fatalf("failed recovery receipt = %+v", receipt)
			}
			if _, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", officialOnly); !errors.Is(err, portalgen.ErrNoCurrentRelease) {
				t.Fatalf("failed recovery served content: %v", err)
			}
			// Restart: state is reloaded from disk by a fresh handler. The
			// authoritative source is published again with the same bytes.
			restarted := handler
			restarted.Status = func(context.Context) (SourceStatus, error) {
				return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
			}
			if err := runRefresh(t, restarted); err != nil {
				t.Fatalf("recovery of unchanged eligible source failed: %v", err)
			}
			recovered, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", officialOnly)
			if err != nil {
				t.Fatal(err)
			}
			if recovered.ContentSHA256 != first.ContentSHA256 {
				t.Fatalf("recovery changed content: got %s want %s", recovered.ContentSHA256, first.ContentSHA256)
			}
			releaseState, _ := portalgen.LoadReleaseState(portalRoot)
			for _, release := range releaseState.Releases {
				if release.ContentSHA256 == first.ContentSHA256 && (release.Withdrawn || release.WithdrawalReason != "") {
					t.Fatalf("recovered release still paused: %+v", release)
				}
			}
			if receipt, _ := loadRefreshState(portalRoot); receipt.LastOutcome != "released" || receipt.WithdrawnReason != "" {
				t.Fatalf("recovery receipt = %+v", receipt)
			}
			// A subsequent valid revision still publishes normally; the
			// recovery path did not freeze the source.
			handler.Status = func(context.Context) (SourceStatus, error) {
				return SourceStatus{HasPublished: true, RevisionID: refreshOfficialB, Status: "published"}, nil
			}
			writeRefreshExport(t, exportRoot, "review-b", "sha256:"+strings.Repeat("b", 64), "6500", refreshOfficialB)
			if err := runRefresh(t, handler); err != nil {
				t.Fatal(err)
			}
			next, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", officialOnly)
			if err != nil || next.ContentSHA256 == first.ContentSHA256 || next.OfficialRevisionID != refreshOfficialB {
				t.Fatalf("next valid revision = %+v %v", next, err)
			}
		})
	}
}

func TestESPFollowupRevokedSourceStaysRefusedAfterRepublish(t *testing.T) {
	for _, officialOnly := range []bool{false, true} {
		name := "full"
		if officialOnly {
			name = "official-only"
		}
		t.Run(name, func(t *testing.T) {
			exportRoot, portalRoot, handler, _ := refreshFixture(t)
			handler.OfficialOnly = officialOnly
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
			// The source cannot clear a revocation by claiming to be
			// published again, even with different presentation bytes.
			handler.Status = func(context.Context) (SourceStatus, error) {
				return SourceStatus{HasPublished: true, RevisionID: refreshOfficialA, Status: "published"}, nil
			}
			if err := runRefresh(t, handler); !errors.Is(err, portalgen.ErrReleaseWithdrawn) && !errors.Is(err, portalgen.ErrOfficialRevoked) {
				t.Fatalf("republished revoked source was accepted: %v", err)
			}
			writeRefreshExport(t, exportRoot, "review-a-rebuilt", "sha256:"+strings.Repeat("c", 64), "6100", refreshOfficialA)
			if err := runRefresh(t, handler); !errors.Is(err, portalgen.ErrOfficialRevoked) {
				t.Fatalf("revoked identity returned under another hash: %v", err)
			}
			if _, _, err := portalgen.CurrentRelease(portalRoot, "https://portal.example", officialOnly); !errors.Is(err, portalgen.ErrNoCurrentRelease) {
				t.Fatalf("revoked content still served: %v", err)
			}
		})
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
