package portalgen

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

func releaseCandidate(t *testing.T, marker string) string {
	t.Helper()
	snapshot := writeFixtureWith(t, func(stations *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		(*stations)[0].DisplayName = marker
	})
	out := filepath.Join(t.TempDir(), "artifact")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	return out
}

func releaseRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "root")
}

func promoteFixture(t *testing.T, artifact, root string) ReleaseState {
	t.Helper()
	state, err := Promote(PromoteOptions{ArtifactDir: artifact, RootDir: root, SiteURL: "https://portal.example"})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func currentFixture(t *testing.T, root string) Release {
	t.Helper()
	release, _, err := CurrentRelease(root, "https://portal.example", false)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func TestPromoteSwitchRollbackWithdraw(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidate(t, "Posto Alfa")
	second := releaseCandidate(t, "Posto Alfa Revisado")
	state := promoteFixture(t, first, root)
	firstHash := state.Current
	if got := currentFixture(t, root); got.ContentSHA256 != firstHash {
		t.Fatalf("current = %+v", got)
	}
	state = promoteFixture(t, second, root)
	secondHash := state.Current
	if secondHash == firstHash || state.Previous != firstHash {
		t.Fatalf("promotion state = %+v", state)
	}
	if got := currentFixture(t, root); got.ContentSHA256 != secondHash {
		t.Fatalf("current = %+v", got)
	}
	state, err := Rollback(RollbackOptions{RootDir: root, Target: firstHash, SiteURL: "https://portal.example"})
	if err != nil || state.Current != firstHash || state.Previous != secondHash {
		t.Fatalf("rollback = %+v %v", state, err)
	}
	state, err = Withdraw(WithdrawOptions{RootDir: root, Fallback: secondHash, SiteURL: "https://portal.example"})
	if err != nil || state.Current != secondHash || state.Previous != firstHash {
		t.Fatalf("withdraw fallback = %+v %v", state, err)
	}
	if _, err := Rollback(RollbackOptions{RootDir: root, Target: firstHash, SiteURL: "https://portal.example"}); !errors.Is(err, ErrReleaseWithdrawn) {
		t.Fatal("withdrawn release resurrected", err)
	}
	state, err = Withdraw(WithdrawOptions{RootDir: root, SiteURL: "https://portal.example"})
	if err != nil || state.Current != "" {
		t.Fatalf("withdraw current = %+v %v", state, err)
	}
	if _, _, err := CurrentRelease(root, "https://portal.example", false); !errors.Is(err, ErrNoCurrentRelease) {
		t.Fatal("current survived withdrawal", err)
	}
	for _, path := range []string{root, filepath.Join(root, ReleaseDir)} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("root permissions = %v %v", info, err)
		}
	}
	stateInfo, err := os.Stat(filepath.Join(root, ReleaseStateFile))
	if err != nil || stateInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state permissions = %v %v", stateInfo, err)
	}
}

func TestPromoteFenceIdempotenceAndBusy(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidate(t, "Posto Alfa")
	second := releaseCandidate(t, "Posto Alfa Revisado")
	state := promoteFixture(t, first, root)
	firstHash := state.Current
	again := promoteFixture(t, first, root)
	if len(again.Releases) != 1 || again.Current != firstHash {
		t.Fatalf("idempotent promote = %+v", again)
	}
	if _, err := Promote(PromoteOptions{ArtifactDir: second, RootDir: root, SiteURL: "https://portal.example", Fence: true, ExpectedCurrent: "sha256:wrong"}); !errors.Is(err, ErrReleaseFence) {
		t.Fatal("stale fence accepted", err)
	}
	if got := currentFixture(t, root); got.ContentSHA256 != firstHash {
		t.Fatal("fence changed current")
	}
	if _, err := Promote(PromoteOptions{ArtifactDir: second, RootDir: root, SiteURL: "https://portal.example", Fence: true, ExpectedCurrent: firstHash}); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockRelease(root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := Promote(PromoteOptions{ArtifactDir: first, RootDir: root, SiteURL: "https://portal.example"}); !errors.Is(err, ErrReleaseBusy) {
		t.Fatal("busy promotion accepted", err)
	}
}

func TestPromotePreservesCurrentDuringCopy(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidate(t, "Posto Alfa")
	second := releaseCandidate(t, "Posto Alfa Revisado")
	state := promoteFixture(t, first, root)
	firstHash := state.Current
	started := make(chan struct{})
	unblock := make(chan struct{})
	var once sync.Once
	copyFileHook = func(src, dst string) error {
		once.Do(func() {
			close(started)
			<-unblock
		})
		return copyFile(src, dst)
	}
	defer func() { copyFileHook = copyFile }()
	done := make(chan error, 1)
	go func() {
		_, err := Promote(PromoteOptions{ArtifactDir: second, RootDir: root, SiteURL: "https://portal.example"})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("copy did not start")
	}
	if got := currentFixture(t, root); got.ContentSHA256 != firstHash {
		t.Fatal("current switched before copy completed")
	}
	close(unblock)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := currentFixture(t, root); got.ContentSHA256 == firstHash {
		t.Fatal("current did not switch")
	}
}

func TestPromoteCopyFailuresPreserveCurrent(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidate(t, "Posto Alfa")
	second := releaseCandidate(t, "Posto Alfa Revisado")
	firstHash := promoteFixture(t, first, root).Current
	t.Run("disk full", func(t *testing.T) {
		copyFileHook = func(string, string) error { return syscall.ENOSPC }
		defer func() { copyFileHook = copyFile }()
		if _, err := Promote(PromoteOptions{ArtifactDir: second, RootDir: root, SiteURL: "https://portal.example"}); err == nil {
			t.Fatal("disk full accepted")
		}
		if got := currentFixture(t, root); got.ContentSHA256 != firstHash {
			t.Fatal("disk full changed current")
		}
		temps, _ := filepath.Glob(filepath.Join(root, ReleaseDir, ".tmp-*"))
		if len(temps) != 0 {
			t.Fatalf("temporary releases retained: %v", temps)
		}
	})
	t.Run("corruption", func(t *testing.T) {
		copyFileHook = func(src, dst string) error {
			if err := copyFile(src, dst); err != nil {
				return err
			}
			return os.WriteFile(dst, []byte("corrupt"), 0o600)
		}
		defer func() { copyFileHook = copyFile }()
		if _, err := Promote(PromoteOptions{ArtifactDir: second, RootDir: root, SiteURL: "https://portal.example"}); err == nil {
			t.Fatal("corrupt copy accepted")
		}
		if got := currentFixture(t, root); got.ContentSHA256 != firstHash {
			t.Fatal("corrupt copy changed current")
		}
	})
}

func TestPromotePermissionFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(parent, 0o700) }()
	artifact := releaseCandidate(t, "Posto Alfa")
	if _, err := Promote(PromoteOptions{ArtifactDir: artifact, RootDir: filepath.Join(parent, "root"), SiteURL: "https://portal.example"}); err == nil {
		t.Fatal("permission failure accepted")
	}
}

func TestCorruptCurrentRollsBack(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidate(t, "Posto Alfa")
	second := releaseCandidate(t, "Posto Alfa Revisado")
	firstHash := promoteFixture(t, first, root).Current
	secondState := promoteFixture(t, second, root)
	secondHash := secondState.Current
	releaseDir := releasePath(root, secondHash)
	var target string
	err := filepath.Walk(releaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, "index.html") {
			return err
		}
		target = path
		return filepath.SkipAll
	})
	if err != nil || target == "" {
		t.Fatalf("corruption target = %q %v", target, err)
	}
	if err := os.WriteFile(target, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CurrentRelease(root, "https://portal.example", false); !errors.Is(err, ErrReleaseUnavailable) {
		t.Fatal("corrupt current accepted", err)
	}
	state, err := Rollback(RollbackOptions{RootDir: root, Target: firstHash, SiteURL: "https://portal.example"})
	if err != nil || state.Current != firstHash {
		t.Fatalf("rollback from corrupt current = %+v %v", state, err)
	}
	if got := currentFixture(t, root); got.ContentSHA256 != firstHash {
		t.Fatal("rollback did not restore safe release")
	}
}

func TestPromoteStateMissingFailsClosed(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidate(t, "Posto Alfa")
	second := releaseCandidate(t, "Posto Alfa Revisado")
	promoteFixture(t, first, root)
	if err := os.Remove(filepath.Join(root, ReleaseStateFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := Promote(PromoteOptions{ArtifactDir: second, RootDir: root, SiteURL: "https://portal.example"}); !errors.Is(err, ErrReleaseState) {
		t.Fatal("missing state accepted", err)
	}
}

func TestPromoteReconcilesStaleState(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidate(t, "Posto Alfa")
	state := promoteFixture(t, first, root)
	current := state.Current
	state.Current = "sha256:stale"
	if err := writeReleaseState(root, state); err != nil {
		t.Fatal(err)
	}
	repaired, err := Promote(PromoteOptions{ArtifactDir: first, RootDir: root, SiteURL: "https://portal.example"})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Current != current || len(repaired.Releases) != 1 {
		t.Fatalf("reconciled state = %+v", repaired)
	}
}

func TestReleaseStoresOfficialPin(t *testing.T) {
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "artifact")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	root := releaseRoot(t)
	promoteFixture(t, out, root)
	current := currentFixture(t, root)
	if current.OfficialRevisionID != "00000000-0000-4000-8000-0000000000f1" || current.OfficialLeaseID != strings.Repeat("b", 64) || current.OfficialEdition != "2025" {
		t.Fatalf("official release metadata = %+v", current)
	}
}

func releaseCandidateWithJS(t *testing.T, marker, script string) string {
	t.Helper()
	snapshot := writeFixtureWith(t, func(stations *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		(*stations)[0].DisplayName = marker
	})
	js := filepath.Join(t.TempDir(), "search.js")
	if err := os.WriteFile(js, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "artifact")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, SearchScriptPath: js}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRetainAssetsAcrossReleases(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Posto Alfa", "globalThis.__searchA=1;\n")
	second := releaseCandidateWithJS(t, "Posto Alfa Revisado", "globalThis.__searchB=2;\n")
	firstState := promoteFixture(t, first, root)
	secondState := promoteFixture(t, second, root)
	hashes := map[string]string{
		first:  strings.TrimPrefix(firstState.Current, "sha256:"),
		second: strings.TrimPrefix(secondState.Current, "sha256:"),
	}
	summary, err := RetainAssets(root, 2, 100<<20)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Releases != 2 || summary.Files == 0 {
		t.Fatalf("retained summary = %+v", summary)
	}
	// Each release's unique script is served from the retained store with
	// byte-identical content, sharing inodes with the release copy (no
	// second copy of the bytes).
	for _, artifact := range []string{first, second} {
		manifest, err := readPortalManifest(artifact)
		if err != nil {
			t.Fatal(err)
		}
		releaseDir := filepath.Join(root, ReleaseDir, hashes[artifact])
		for _, file := range manifest.Files {
			if !strings.HasSuffix(file.Path, ".js") {
				continue
			}
			original, err := os.ReadFile(filepath.Join(releaseDir, filepath.FromSlash(file.Path)))
			if err != nil {
				t.Fatal(err)
			}
			// Find this exact script in the retained store.
			found := false
			entries, err := os.ReadDir(filepath.Join(root, RetainedDir, "assets"))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if !strings.HasSuffix(entry.Name(), ".js") {
					continue
				}
				candidate, err := os.ReadFile(filepath.Join(root, RetainedDir, "assets", entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if string(candidate) == string(original) {
					found = true
					a, _ := os.Stat(filepath.Join(releaseDir, filepath.FromSlash(file.Path)))
					b, _ := os.Stat(filepath.Join(root, RetainedDir, "assets", entry.Name()))
					if !os.SameFile(a, b) {
						t.Error("retained asset is a copy, not a hardlink")
					}
				}
			}
			if !found {
				t.Errorf("unique script of %s missing from retained store", file.Path)
			}
		}
	}
	// Pages are never retained: only the current release serves HTML.
	var htmlFiles []string
	retainedRoot, err := filepath.EvalSymlinks(filepath.Join(root, RetainedDir))
	if err != nil {
		t.Fatal(err)
	}
	_ = filepath.Walk(retainedRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
			htmlFiles = append(htmlFiles, path)
		}
		return nil
	})
	if len(htmlFiles) != 0 {
		t.Fatalf("retained pages: %v", htmlFiles)
	}
}

func TestRetainAssetsKeepLimitAndBudget(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Posto Alfa", "globalThis.__searchA=1;\n")
	second := releaseCandidateWithJS(t, "Posto Alfa Revisado", "globalThis.__searchB=2;\n")
	promoteFixture(t, first, root)
	promoteFixture(t, second, root)
	summary, err := RetainAssets(root, 1, 100<<20)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Releases != 1 {
		t.Fatalf("retained summary = %+v, want only the newest release", summary)
	}
	// A budget smaller than the newest release alone fails closed and
	// keeps the previous store untouched.
	if _, err := RetainAssets(root, 2, 1); err == nil {
		t.Fatal("impossible budget accepted")
	}
	entries, err := os.ReadDir(filepath.Join(root, RetainedDir, "assets"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("previous retained store lost: %v %v", entries, err)
	}
}

func TestWithdrawPurgesRetainedAssets(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Posto Alfa", "globalThis.__searchA=1;\n")
	second := releaseCandidateWithJS(t, "Posto Alfa Revisado", "globalThis.__searchB=2;\n")
	promoteFixture(t, first, root)
	promoteFixture(t, second, root)
	if _, err := RetainAssets(root, 2, 100<<20); err != nil {
		t.Fatal(err)
	}
	// Withdrawing the current release purges its exclusive assets; the
	// shared stylesheet referenced by the live release stays.
	if _, err := Withdraw(WithdrawOptions{RootDir: root, SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := readPortalManifest(second)
	if err != nil {
		t.Fatal(err)
	}
	withdrawnJS := ""
	sharedCSS := ""
	for _, file := range manifest.Files {
		base := filepath.Base(file.Path)
		switch {
		case strings.HasSuffix(base, ".js"):
			withdrawnJS = base
		case strings.HasSuffix(base, ".css"):
			sharedCSS = base
		}
	}
	if withdrawnJS == "" || sharedCSS == "" {
		t.Fatal("fixture lacks distinct JS/CSS assets")
	}
	if _, err := os.Stat(filepath.Join(root, RetainedDir, "assets", withdrawnJS)); !os.IsNotExist(err) {
		t.Errorf("withdrawn exclusive asset still retained: %s", withdrawnJS)
	}
	if _, err := os.Stat(filepath.Join(root, RetainedDir, "assets", sharedCSS)); err != nil {
		t.Errorf("shared asset purged with the withdrawal: %s", sharedCSS)
	}
}

func TestPromoteLinksSameFilesystemArtifactAndFallsBackToCopy(t *testing.T) {
	sameInode := func(t *testing.T, artifact, root, hash string) bool {
		t.Helper()
		a, err := os.Stat(filepath.Join(artifact, "portal-manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		r, err := os.Stat(filepath.Join(releasePath(root, hash), "portal-manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		return os.SameFile(a, r)
	}
	linkedRoot := releaseRoot(t)
	linkedArtifact := releaseCandidate(t, "Posto Alfa")
	linked := promoteFixture(t, linkedArtifact, linkedRoot)
	if !sameInode(t, linkedArtifact, linkedRoot, linked.Current) {
		t.Fatal("same-filesystem promotion copied bytes instead of linking")
	}
	if err := os.RemoveAll(linkedArtifact); err != nil {
		t.Fatal(err)
	}
	if got := currentFixture(t, linkedRoot); got.ContentSHA256 != linked.Current {
		t.Fatal("removing the promoted artifact broke the linked release")
	}

	linkFile = func(string, string) error { return syscall.EXDEV }
	defer func() { linkFile = os.Link }()
	copiedRoot := releaseRoot(t)
	copiedArtifact := releaseCandidate(t, "Posto Beta")
	copied := promoteFixture(t, copiedArtifact, copiedRoot)
	if sameInode(t, copiedArtifact, copiedRoot, copied.Current) {
		t.Fatal("cross-filesystem fallback did not copy")
	}
	if got := currentFixture(t, copiedRoot); got.ContentSHA256 != copied.Current {
		t.Fatal("copied release is not current")
	}
}
