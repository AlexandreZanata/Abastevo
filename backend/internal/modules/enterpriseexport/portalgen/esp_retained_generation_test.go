package portalgen

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func retainedLinkTarget(t *testing.T, root string) string {
	t.Helper()
	target, err := os.Readlink(filepath.Join(root, RetainedDir))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Base(target)
}

func retainedGenerationEntries(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, retainedGenerationsDir))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && validRetainedGenerationName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	return names
}

func TestRetainedStoreSwitchFailureKeepsPreviousGeneration(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Atomic A", "globalThis.__atomicA=1;\n")
	second := releaseCandidateWithJS(t, "Atomic B", "globalThis.__atomicB=2;\n")
	firstState := promoteFixture(t, first, root)
	generation := retainedLinkTarget(t, root)
	original := publishRetainedGenerationHook
	defer func() { publishRetainedGenerationHook = original }()
	publishRetainedGenerationHook = func(string, string) error {
		return errors.New("synthetic retained switch failure")
	}
	if _, err := Promote(PromoteOptions{ArtifactDir: second, RootDir: root, SiteURL: "https://portal.example"}); err == nil {
		t.Fatal("promotion accepted a failed retained switch")
	}
	if got := currentFixture(t, root); got.ContentSHA256 != firstState.Current {
		t.Fatalf("failed retained switch changed current = %+v", got)
	}
	if got := retainedLinkTarget(t, root); got != generation {
		t.Fatalf("failed retained switch changed the store: got %s want %s", got, generation)
	}
	for _, asset := range releaseJSAssets(t, first) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
			t.Fatalf("previous generation asset lost after failed switch %s: %v", asset, err)
		}
	}
	if entries := retainedGenerationEntries(t, root); len(entries) != 1 || entries[0] != generation {
		t.Fatalf("failed switch left generations = %v", entries)
	}
}

func TestRetainedStoreRecoversAfterMissingLink(t *testing.T) {
	root := releaseRoot(t)
	artifact := releaseCandidateWithJS(t, "Recovery A", "globalThis.__recoveryA=1;\n")
	promoteFixture(t, artifact, root)
	generation := retainedLinkTarget(t, root)
	if err := os.Remove(filepath.Join(root, RetainedDir)); err != nil {
		t.Fatal(err)
	}
	if got, err := recoverRetained(root); err != nil || got != generation {
		t.Fatalf("recovery = %q, %v; want %s", got, err, generation)
	}
	for _, asset := range releaseJSAssets(t, artifact) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
			t.Fatalf("recovered store missing asset %s: %v", asset, err)
		}
	}
	// A complete newer generation is preferred on recovery; incomplete
	// pending trees are ignored.
	pending := filepath.Join(root, retainedGenerationsDir, retainedPendingPrefix+"orphan")
	if err := os.MkdirAll(filepath.Join(pending, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, RetainedDir)); err != nil {
		t.Fatal(err)
	}
	if got, err := recoverRetained(root); err != nil || got != generation {
		t.Fatalf("recovery picked an incomplete tree: %q %v", got, err)
	}
	newer, err := newRetainedGenerationName()
	if err != nil {
		t.Fatal(err)
	}
	newerPath := filepath.Join(root, retainedGenerationsDir, newer)
	if err := os.MkdirAll(filepath.Join(newerPath, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newerPath, "assets", "recovered.js"), []byte("globalThis.__newer=1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newerPath, retainedCompleteMarker), []byte("complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(newerPath, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, RetainedDir)); err != nil {
		t.Fatal(err)
	}
	if got, err := recoverRetained(root); err != nil || got != newer {
		t.Fatalf("recovery did not select the newest complete generation: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, RetainedDir, "assets", "recovered.js")); err != nil {
		t.Fatalf("recovered generation is not served: %v", err)
	}
}

func TestRetainedLegacyDirectoryMigrationIsRecoverable(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, RetainedDir, "assets")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "legacy.js"), []byte("globalThis.__legacy=1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	generation, err := recoverRetained(root)
	if err != nil || generation == "" {
		t.Fatalf("legacy migration = %q %v", generation, err)
	}
	if info, err := os.Lstat(filepath.Join(root, RetainedDir)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("legacy retained path was not replaced by a symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, RetainedDir, "assets", "legacy.js")); err != nil {
		t.Fatalf("migrated legacy asset unavailable: %v", err)
	}
	if err := os.Remove(filepath.Join(root, RetainedDir)); err != nil {
		t.Fatal(err)
	}
	if recovered, err := recoverRetained(root); err != nil || recovered != generation {
		t.Fatalf("recovery after missing legacy link = %q %v", recovered, err)
	}
}

func TestRetainedStoreCleansOldGenerationsAfterSwitch(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Cleanup A", "globalThis.__cleanupA=1;\n")
	second := releaseCandidateWithJS(t, "Cleanup B", "globalThis.__cleanupB=2;\n")
	third := releaseCandidateWithJS(t, "Cleanup C", "globalThis.__cleanupC=3;\n")
	promoteFixture(t, first, root)
	promoteFixture(t, second, root)
	secondGeneration := retainedLinkTarget(t, root)
	promoteFixture(t, third, root)
	entries := retainedGenerationEntries(t, root)
	// The current and immediately previous generations remain for in-flight
	// readers; older generations are removed.
	if len(entries) != 2 {
		t.Fatalf("generations after three promotions = %v", entries)
	}
	link := retainedLinkTarget(t, root)
	if link == secondGeneration || !containsString(entries, link) || !containsString(entries, secondGeneration) {
		t.Fatalf("generations = %v, served=%s previous=%s", entries, link, secondGeneration)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRetainedHTTPReadersSurviveTransitions(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Readers A", "globalThis.__readersA=1;\n")
	second := releaseCandidateWithJS(t, "Readers B", "globalThis.__readersB=2;\n")
	firstState := promoteFixture(t, first, root)
	secondState := promoteFixture(t, second, root)
	firstAssets := releaseJSAssets(t, first)
	secondAssets := releaseJSAssets(t, second)

	assets := http.FileServer(http.Dir(filepath.Join(root, RetainedDir, "assets")))
	pages := http.FileServer(http.Dir(filepath.Join(root, ReleasePointer)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			clone := r.Clone(r.Context())
			clone.URL.Path = strings.TrimPrefix(r.URL.Path, "/assets/")
			assets.ServeHTTP(w, clone)
			return
		}
		pages.ServeHTTP(w, r)
	}))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	paths := []string{"/consultar/", "/assets/" + strings.TrimPrefix(firstAssets[0], "assets/"), "/assets/" + strings.TrimPrefix(secondAssets[0], "assets/")}
	var requests atomic.Int64
	var mu sync.Mutex
	var failures []string
	record := func(message string) {
		mu.Lock()
		if len(failures) < 10 {
			failures = append(failures, message)
		}
		mu.Unlock()
	}
	done := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				for _, path := range paths {
					resp, err := client.Get(server.URL + path)
					if err != nil {
						record(fmt.Sprintf("GET %s: %v", path, err))
						continue
					}
					_ = resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						record(fmt.Sprintf("GET %s: status %d", path, resp.StatusCode))
					}
					requests.Add(1)
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		target := firstState.Current
		if i%2 == 1 {
			target = secondState.Current
		}
		if _, err := Rollback(RollbackOptions{RootDir: root, Target: target, SiteURL: "https://portal.example", RetainKeep: 2}); err != nil {
			close(done)
			readers.Wait()
			t.Fatalf("rollback %d: %v", i, err)
		}
		if _, err := RetainAssets(root, 2, 100<<20); err != nil {
			close(done)
			readers.Wait()
			t.Fatalf("retain %d: %v", i, err)
		}
	}
	close(done)
	readers.Wait()
	if requests.Load() == 0 {
		t.Fatal("readers did not run during transitions")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(failures) != 0 {
		t.Fatalf("readers observed broken transitions: %v", failures)
	}
}

func TestWithdrawKeepsCurrentAssetsUntilPointerMoves(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Withdraw A", "globalThis.__withdrawA=1;\n")
	second := releaseCandidateWithJS(t, "Withdraw B", "globalThis.__withdrawB=2;\n")
	firstState := promoteFixture(t, first, root)
	secondState := promoteFixture(t, second, root)
	original := publishRetainedGenerationHook
	defer func() { publishRetainedGenerationHook = original }()
	calls := 0
	publishRetainedGenerationHook = func(root, generation string) error {
		calls++
		if calls == 1 {
			for _, asset := range append(releaseJSAssets(t, first), releaseJSAssets(t, second)...) {
				if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
					t.Errorf("transition store missing asset %s while current is still served: %v", asset, err)
				}
			}
		}
		return original(root, generation)
	}
	state, err := Withdraw(WithdrawOptions{RootDir: root, Fallback: firstState.Current, SiteURL: "https://portal.example"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("withdraw retained rebuilds = %d, want transition + purge", calls)
	}
	if state.Current != firstState.Current || state.Previous != secondState.Current {
		t.Fatalf("withdraw state = %+v", state)
	}
	for _, asset := range releaseJSAssets(t, first) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
			t.Fatalf("fallback asset missing after withdraw: %s: %v", asset, err)
		}
	}
	for _, asset := range releaseJSAssets(t, second) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); !os.IsNotExist(err) {
			t.Fatalf("withdrawn asset still served after purge: %s", asset)
		}
	}
	if _, _, err := CurrentRelease(root, "https://portal.example", false); err != nil {
		t.Fatalf("fallback unavailable after withdraw: %v", err)
	}
}

func TestWithdrawPurgeFailureLeavesServingFallback(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Purge A", "globalThis.__purgeA=1;\n")
	second := releaseCandidateWithJS(t, "Purge B", "globalThis.__purgeB=2;\n")
	firstState := promoteFixture(t, first, root)
	promoteFixture(t, second, root)
	original := publishRetainedGenerationHook
	defer func() { publishRetainedGenerationHook = original }()
	calls := 0
	publishRetainedGenerationHook = func(root, generation string) error {
		calls++
		if calls == 2 {
			return errors.New("synthetic purge failure")
		}
		return original(root, generation)
	}
	if _, err := Withdraw(WithdrawOptions{RootDir: root, Fallback: firstState.Current, SiteURL: "https://portal.example"}); err == nil {
		t.Fatal("withdraw hid a failed retention purge")
	}
	state, ok := LoadReleaseState(root)
	if !ok || state.Current != firstState.Current {
		t.Fatalf("committed withdrawal state = %+v", state)
	}
	for _, asset := range releaseJSAssets(t, first) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
			t.Fatalf("serving fallback asset lost after purge failure: %s: %v", asset, err)
		}
	}
	if _, _, err := CurrentRelease(root, "https://portal.example", false); err != nil {
		t.Fatalf("fallback unavailable after purge failure: %v", err)
	}
	if _, err := RetainAssets(root, 2, 100<<20); err != nil {
		t.Fatalf("repair retain: %v", err)
	}
	for _, asset := range releaseJSAssets(t, second) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); !os.IsNotExist(err) {
			t.Fatalf("repair retain kept withdrawn asset %s", asset)
		}
	}
}

func TestRollbackKeepsCurrentAssetsUntilPointerMoves(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidateWithJS(t, "Switch A", "globalThis.__switchA=1;\n")
	second := releaseCandidateWithJS(t, "Switch B", "globalThis.__switchB=2;\n")
	firstState := promoteFixture(t, first, root)
	promoteFixture(t, second, root)
	original := publishRetainedGenerationHook
	defer func() { publishRetainedGenerationHook = original }()
	publishRetainedGenerationHook = func(root, generation string) error {
		for _, asset := range append(releaseJSAssets(t, first), releaseJSAssets(t, second)...) {
			if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
				t.Errorf("transition store missing %s while the previous current is still served: %v", asset, err)
			}
		}
		return original(root, generation)
	}
	if _, err := Rollback(RollbackOptions{RootDir: root, Target: firstState.Current, SiteURL: "https://portal.example", RetainKeep: 1}); err != nil {
		t.Fatal(err)
	}
	if got := currentFixture(t, root); got.ContentSHA256 != firstState.Current {
		t.Fatalf("rollback current = %+v", got)
	}
	for _, asset := range releaseJSAssets(t, first) {
		if _, err := os.Stat(retainedAssetPath(t, root, asset)); err != nil {
			t.Fatalf("rollback target asset missing: %s: %v", asset, err)
		}
	}
}
