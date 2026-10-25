package portalgen

// Maintained regressions for the ESP review: withdrawn releases can never be
// re-served, the publication preflight understands official-only artifacts,
// and the official-only verifier enforces an exact file allowlist.
// Every operation reloads state.json from disk, so re-invocation after a
// restart is covered by construction; persistence is asserted explicitly.

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

func espRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller unavailable")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "infra", "scripts", "load", "publication_preflight.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root with publication_preflight.py not found")
		}
		dir = parent
	}
}

func espOfficialArtifact(t *testing.T, marker string) string {
	t.Helper()
	snapshot := writeFixtureWith(t, func(stations *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		(*stations)[0].DisplayName = marker
	})
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "artifact")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	return out
}

func espPromote(t *testing.T, artifact, root string) ReleaseState {
	t.Helper()
	state, err := Promote(PromoteOptions{ArtifactDir: artifact, RootDir: root, SiteURL: "https://portal.example"})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestESPReviewWithdrawnArtifactCannotBePromoted(t *testing.T) {
	root := releaseRoot(t)
	first := releaseCandidate(t, "Review Alfa")
	second := releaseCandidate(t, "Review Beta")
	// Valid operational rollback across two candidates: A -> B -> A.
	state := espPromote(t, first, root)
	firstHash := state.Current
	state = espPromote(t, second, root)
	secondHash := state.Current
	if state, err := Rollback(RollbackOptions{RootDir: root, Target: firstHash, SiteURL: "https://portal.example"}); err != nil || state.Current != firstHash {
		t.Fatalf("operational rollback A = %+v %v", state, err)
	}
	// Withdraw A as an operational pause: history keeps the mark.
	if _, err := Withdraw(WithdrawOptions{RootDir: root, SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
	persisted, ok := LoadReleaseState(root)
	if !ok {
		t.Fatal("release state did not survive the withdrawal")
	}
	found := false
	for _, release := range persisted.Releases {
		if release.ContentSHA256 == firstHash && release.Withdrawn {
			found = true
		}
	}
	if !found {
		t.Fatal("withdrawal mark missing from persisted history")
	}
	// Resurrection refused with no current release.
	if _, err := Promote(PromoteOptions{ArtifactDir: first, RootDir: root, SiteURL: "https://portal.example"}); !errors.Is(err, ErrReleaseWithdrawn) {
		t.Fatalf("withdrawn artifact promoted with no current: %v", err)
	}
	// Resurrection refused with another current release.
	espPromote(t, second, root)
	if _, err := Promote(PromoteOptions{ArtifactDir: first, RootDir: root, SiteURL: "https://portal.example"}); !errors.Is(err, ErrReleaseWithdrawn) {
		t.Fatalf("withdrawn artifact promoted over another current: %v", err)
	}
	// Resurrection refused through rollback and fallback paths.
	if _, err := Rollback(RollbackOptions{RootDir: root, Target: firstHash, SiteURL: "https://portal.example"}); !errors.Is(err, ErrReleaseWithdrawn) {
		t.Fatalf("rollback resurrected a withdrawn release: %v", err)
	}
	if _, err := Withdraw(WithdrawOptions{RootDir: root, Fallback: firstHash, SiteURL: "https://portal.example"}); !errors.Is(err, ErrReleaseWithdrawn) {
		t.Fatalf("withdraw fallback resurrected a withdrawn release: %v", err)
	}
	if got := currentFixture(t, root); got.ContentSHA256 != secondHash {
		t.Fatalf("current changed during refusal = %+v", got)
	}
}

func TestESPReviewRevokedSourceCannotReturnByAnotherHash(t *testing.T) {
	root := releaseRoot(t)
	revoked := espOfficialArtifact(t, "Review Revogada")
	espPromote(t, revoked, root)
	// Same official pin, different content hash: a rebuild after the source
	// was declared invalid must still be refused.
	rebuilt := espOfficialArtifact(t, "Review Revogada Reconstruida")
	if _, err := Withdraw(WithdrawOptions{RootDir: root, SiteURL: "https://portal.example", Reason: "revoked-source"}); err != nil {
		t.Fatal(err)
	}
	persisted, ok := LoadReleaseState(root)
	if !ok || len(persisted.RevokedOfficial) != 1 || persisted.RevokedOfficial[0].RevisionID == "" {
		t.Fatalf("revocation identity missing from persisted history = %+v", persisted)
	}
	if _, err := Promote(PromoteOptions{ArtifactDir: revoked, RootDir: root, SiteURL: "https://portal.example"}); !errors.Is(err, ErrReleaseWithdrawn) {
		t.Fatalf("withdrawn artifact promoted: %v", err)
	}
	if _, err := Promote(PromoteOptions{ArtifactDir: rebuilt, RootDir: root, SiteURL: "https://portal.example"}); !errors.Is(err, ErrOfficialRevoked) {
		t.Fatalf("revoked official content returned by another hash: %v", err)
	}
	// A legacy state carrying the revoked pin on a non-withdrawn release
	// must still refuse rollback: the revocation guard is independent of
	// the withdrawal mark.
	legacyState, ok := LoadReleaseState(root)
	if !ok {
		t.Fatal("release state unavailable")
	}
	legacyState.Releases = append(legacyState.Releases, Release{ContentSHA256: "sha256:" + strings.Repeat("7", 64), OfficialRevisionID: persisted.RevokedOfficial[0].RevisionID, OfficialLeaseID: persisted.RevokedOfficial[0].LeaseID})
	rawLegacy, err := json.Marshal(legacyState)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ReleaseStateFile), append(rawLegacy, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	plain := releaseCandidate(t, "Review Limpa")
	espPromote(t, plain, root)
	// Content without the revoked pin stays eligible: revocation is scoped
	// to the official identity, not a blanket ban (proven by the promotion
	// above succeeding).
	if _, err := Rollback(RollbackOptions{RootDir: root, Target: "sha256:" + strings.Repeat("7", 64), SiteURL: "https://portal.example"}); !errors.Is(err, ErrOfficialRevoked) {
		t.Fatalf("rollback served revoked official content: %v", err)
	}
	if _, err := Withdraw(WithdrawOptions{RootDir: root, SiteURL: "https://portal.example", Reason: "bogus"}); err == nil {
		t.Fatal("unknown withdrawal reason accepted")
	}
}

func TestESPReviewOfficialOnlyPublicPreflight(t *testing.T) {
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-public")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true, Indexable: true, SiteURL: "https://portal.abastevo.com.br"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "portal-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	pin := manifest["official_records"].(map[string]any)
	scope := map[string]any{"candidate_content_sha256": manifest["content_sha256"], "origin": "https://portal.abastevo.com.br", "source_slices": []string{"official-sanctions"}}
	for key, value := range map[string]string{"source_revision_id": "revision_id", "source_lease_id": "lease_id", "source_checksum": "source_checksum", "source_edition": "edition", "source_rows": "rows"} {
		scope[key] = pin[value]
	}
	scopeRaw, _ := json.Marshal(scope)
	script := "import importlib.util,json,sys; s=importlib.util.spec_from_file_location('preflight',sys.argv[3]); m=importlib.util.module_from_spec(s);s.loader.exec_module(m); print(json.dumps(m.verify_artifact(m.Path(sys.argv[1]),json.loads(sys.argv[2]))[0]))"
	preflight := filepath.Join(espRepoRoot(t), "infra", "scripts", "load", "publication_preflight.py")
	result, err := exec.Command("python3", "-c", script, out, string(scopeRaw), preflight).CombinedOutput()
	if err != nil {
		t.Fatalf("preflight execution: %v %s", err, result)
	}
	if strings.TrimSpace(string(result)) != "[]" {
		t.Fatalf("valid official-only indexable artifact rejected: %s", result)
	}
}

func TestESPReviewOfficialOnlyRejectsUndeclaredCSV(t *testing.T) {
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true}); err != nil {
		t.Fatal(err)
	}
	manifest, err := readPortalManifest(out)
	if err != nil {
		t.Fatal(err)
	}
	extra := []byte("amount_milli_brl\n5990\n")
	path := "price-export.csv"
	if err := os.WriteFile(filepath.Join(out, filepath.FromSlash(path)), extra, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(extra)
	manifest.Files = append(manifest.Files, FileDigest{Path: path, SHA256: fmt.Sprintf("sha256:%x", sum), Bytes: int64(len(extra))})
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	manifest.ContentSHA256, err = manifestContentDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "portal-manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(out, VerifyOptions{OfficialOnly: true}); err == nil {
		t.Fatal("official-only verifier accepted an extra price CSV at artifact root")
	} else {
		t.Logf("extra CSV refused: %v", err)
	}
}
