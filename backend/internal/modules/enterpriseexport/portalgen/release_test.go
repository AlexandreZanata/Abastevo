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
