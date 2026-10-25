package portalgen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func retainedAssetPath(t *testing.T, root, asset string) string {
	t.Helper()
	return filepath.Join(root, RetainedDir, filepath.FromSlash(asset))
}

func releaseJSAssets(t *testing.T, artifact string) []string {
	t.Helper()
	manifest, err := readPortalManifest(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var assets []string
	for _, file := range manifest.Files {
		if strings.HasPrefix(file.Path, "assets/") && strings.HasSuffix(file.Path, ".js") {
			assets = append(assets, file.Path)
		}
	}
	if len(assets) == 0 {
		t.Fatal("fixture has no JavaScript asset")
	}
	return assets
}

func TestRetentionKeepsCurrentAssetsBeyondKeep(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Retention A", "globalThis.__retentionA=1;\n")
	second := releaseCandidateWithJS(t, "Retention B", "globalThis.__retentionB=2;\n")
	promoteFixture(t, first, root)
	promoteFixture(t, second, root)
	if _, err := Rollback(RollbackOptions{RootDir: root, Target: currentFixture(t, root).ContentSHA256, SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
	firstState, err := Promote(PromoteOptions{ArtifactDir: first, RootDir: root, SiteURL: "https://portal.example"})
	if err != nil {
		t.Fatal(err)
	}
	// Roll back to A, which is older than a keep=1 window.
	if _, err := Rollback(RollbackOptions{RootDir: root, Target: firstState.Current, SiteURL: "https://portal.example", RetainKeep: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := RetainAssets(root, 1, 100<<20); err != nil {
		t.Fatal(err)
	}
	for _, asset := range releaseJSAssets(t, first) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
			t.Fatalf("current rollback asset missing from retained store: %s: %v", asset, err)
		}
	}
	for _, asset := range releaseJSAssets(t, second) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); !os.IsNotExist(err) {
			t.Fatalf("keep=1 retained a non-current release asset: %s", asset)
		}
	}
	if _, _, err := CurrentRelease(root, "https://portal.example", false); err != nil {
		t.Fatalf("current release unavailable after keep pruning: %v", err)
	}
}

func TestFirstPublicationRetainsEveryCurrentAsset(t *testing.T) {
	root := releaseRoot(t)
	artifact := releaseCandidateWithJS(t, "Retention First", "globalThis.__retentionFirst=1;\n")
	manifest, err := readPortalManifest(artifact)
	if err != nil {
		t.Fatal(err)
	}
	promoteFixture(t, artifact, root)
	for _, file := range manifest.Files {
		if !strings.HasPrefix(file.Path, "assets/") {
			continue
		}
		info, err := os.Lstat(retainedAssetPath(t, root, file.Path))
		if err != nil {
			t.Fatalf("first publication asset missing: %s: %v", file.Path, err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("first publication asset is not regular: %s", file.Path)
		}
	}
}

func TestPromotionFailsBeforeSwitchWhenRequiredAssetsDoNotFit(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Budget A", "globalThis.__budgetA=1;\n")
	second := releaseCandidateWithJS(t, "Budget B", "globalThis.__budgetB=2;\n")
	firstState := promoteFixture(t, first, root)
	if _, err := Promote(PromoteOptions{
		ArtifactDir: second, RootDir: root, SiteURL: "https://portal.example",
		RetainMaxBytes: 1,
	}); err == nil {
		t.Fatal("promotion accepted a budget that cannot hold current assets")
	}
	if got := currentFixture(t, root); got.ContentSHA256 != firstState.Current {
		t.Fatalf("failed asset preparation changed current = %+v", got)
	}
	for _, asset := range releaseJSAssets(t, first) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
			t.Fatalf("failed preparation lost the previous current asset %s: %v", asset, err)
		}
	}
	state, ok := LoadReleaseState(root)
	if !ok || len(state.Releases) != 1 || state.Current != firstState.Current {
		t.Fatalf("failed preparation mutated state = %+v", state)
	}
	// A first publication must also fail before exposing a pointer.
	fresh := releaseRoot(t)
	if _, err := Promote(PromoteOptions{
		ArtifactDir: second, RootDir: fresh, SiteURL: "https://portal.example",
		RetainMaxBytes: 1,
	}); err == nil {
		t.Fatal("first publication accepted an impossible asset budget")
	}
	if _, exists, err := readCurrent(fresh); err != nil || exists {
		t.Fatalf("failed first publication exposed a pointer: exists=%v err=%v", exists, err)
	}
}

func TestSafeRetainedAssetPath(t *testing.T) {
	for _, valid := range []string{"assets/app.js", "assets/nested/app.css", "assets/a/b/c.json"} {
		if got, ok := safeRetainedAssetPath(valid); !ok || got != valid {
			t.Errorf("valid path %q rejected: %q %v", valid, got, ok)
		}
	}
	for _, invalid := range []string{"", "/assets/app.js", "assets", "assets/", "assets//app.js", "assets/./app.js", "assets/../app.js", "../assets/app.js", "pages/app.js", `assets\app.js`} {
		if got, ok := safeRetainedAssetPath(invalid); ok {
			t.Errorf("unsafe path %q accepted as %q", invalid, got)
		}
	}
}

func TestRetentionHandlesNestedAssetsAndWithdrawPurgesThem(t *testing.T) {
	root := t.TempDir()
	content := "sha256:" + strings.Repeat("a", 64)
	name := strings.TrimPrefix(content, "sha256:")
	releaseDir := filepath.Join(root, ReleaseDir, name)
	nested := filepath.Join(releaseDir, "assets", "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	var files []FileDigest
	for _, base := range []string{"keep.js", "drop.js"} {
		body := []byte("globalThis.__" + base + "=1;\n")
		if err := os.WriteFile(filepath.Join(nested, base), body, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		files = append(files, FileDigest{
			Path: "assets/nested/" + base, SHA256: "sha256:" + hex.EncodeToString(sum[:]), Bytes: int64(len(body)),
		})
	}
	raw, err := json.Marshal(Manifest{SchemaVersion: "portal-v1", Files: files})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, "portal-manifest.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	state := ReleaseState{
		SchemaVersion: ReleaseStateSchema,
		Current:       content,
		Releases:      []Release{{ContentSHA256: content, PromotedAt: time.Unix(100, 0).UTC().Format(time.RFC3339)}},
	}
	if err := writeReleaseState(root, state); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ReleaseDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(ReleaseDir, name), filepath.Join(root, ReleasePointer)); err != nil {
		t.Fatal(err)
	}
	if _, err := RetainAssets(root, 1, 100<<20); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if _, err := os.Stat(retainedAssetPath(t, root, file.Path)); err != nil {
			t.Fatalf("nested asset not retained %s: %v", file.Path, err)
		}
	}
	if _, err := Withdraw(WithdrawOptions{RootDir: root, SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, RetainedDir, "assets", "nested")); !os.IsNotExist(err) {
		t.Fatalf("withdraw left nested retained assets: %v", err)
	}
}

func TestRetentionRejectsConflictingSharedPaths(t *testing.T) {
	root := t.TempDir()
	firstContent := "sha256:" + strings.Repeat("b", 64)
	secondContent := "sha256:" + strings.Repeat("c", 64)
	var releases []Release
	for i, content := range []string{firstContent, secondContent} {
		name := strings.TrimPrefix(content, "sha256:")
		releaseDir := filepath.Join(root, ReleaseDir, name)
		if err := os.MkdirAll(filepath.Join(releaseDir, "assets"), 0o700); err != nil {
			t.Fatal(err)
		}
		body := []byte("globalThis.__conflict=" + string(rune('0'+i)) + ";\n")
		if err := os.WriteFile(filepath.Join(releaseDir, "assets", "app.js"), body, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		raw, _ := json.Marshal(Manifest{SchemaVersion: "portal-v1", Files: []FileDigest{{
			Path: "assets/app.js", SHA256: "sha256:" + hex.EncodeToString(sum[:]), Bytes: int64(len(body)),
		}}})
		if err := os.WriteFile(filepath.Join(releaseDir, "portal-manifest.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		releases = append(releases, Release{ContentSHA256: content, PromotedAt: time.Unix(int64(100+i), 0).UTC().Format(time.RFC3339)})
	}
	state := ReleaseState{SchemaVersion: ReleaseStateSchema, Current: secondContent, Releases: releases}
	if err := writeReleaseState(root, state); err != nil {
		t.Fatal(err)
	}
	if _, err := rebuildRetained(root, state, []string{secondContent}, 2, 100<<20, true); err == nil {
		t.Fatal("conflicting shared asset path was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, RetainedDir)); !os.IsNotExist(err) {
		t.Fatalf("failed rebuild exposed a retained store: %v", err)
	}
}

func TestRetainAssetsRequiresCurrentState(t *testing.T) {
	root := releaseRoot(t)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := RetainAssets(root, 1, 100<<20); !errors.Is(err, ErrReleaseState) {
		t.Fatalf("retain without state = %v", err)
	}
}
