package portalgen

// Integration between the maintained generator and the publication preflight
// script: the gate must accept a valid official-only indexable candidate and
// refuse wrong origins, mode mismatches, noindex public candidates, broken
// sitemaps/links and scopes without external approval references.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func preflightScript(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(dir, "infra", "scripts", "load", "publication_preflight.py")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("publication_preflight.py not found above the package")
		}
		dir = parent
	}
}

func preflightScope(t *testing.T, manifest Manifest, origin string) map[string]any {
	t.Helper()
	pin := manifest.OfficialRecords
	if pin == nil {
		t.Fatal("candidate has no official pin")
	}
	return map[string]any{
		"schema_version": "abastevo-portal-publication-scope-v1", "origin": origin,
		"host": "vps-public-01", "root": "/srv/abastevo/portal", "cdn": "none",
		"tls": "acme", "approval_ref": "APR-2026-001", "approved_by": "project-owner",
		"window_start": "2026-11-01T12:00:00Z", "window_end": "2026-11-01T14:00:00Z",
		"candidate_content_sha256":   manifest.ContentSHA256,
		"rollback_content_sha256":    "sha256:" + strings.Repeat("0", 64),
		"source_slices":              []string{"official-sanctions"},
		"source_admission_ref":       "docs/product/ENTERPRISE_SOURCE_ADMISSION.md",
		"rights_evidence":            []string{"CC BY metadata; event-level admitted"},
		"host_ownership_attestation": "ownership-record-001",
		"source_revision_id":         pin.RevisionID, "source_lease_id": pin.LeaseID,
		"source_checksum": pin.SourceChecksum, "source_edition": pin.Edition,
		"source_rows": pin.Rows,
	}
}

func runPreflight(t *testing.T, script, artifact string, scope map[string]any) (int, map[string]any) {
	t.Helper()
	scopeRaw, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	scopePath := filepath.Join(t.TempDir(), "scope.json")
	if err := os.WriteFile(scopePath, scopeRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "preflight.json")
	cmd := exec.Command("python3", script, "--artifact", artifact, "--scope", scopePath, "--out", outPath)
	output, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("preflight execution: %v %s", err, output)
		}
		code = exit.ExitCode()
	}
	raw, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatalf("preflight report missing (exit %d): %v %s", code, readErr, output)
	}
	var report map[string]any
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	return code, report
}

func preflightManifest(t *testing.T, artifact string) Manifest {
	t.Helper()
	manifest, err := readPortalManifest(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestPreflightAcceptsOfficialOnlyIndexable(t *testing.T) {
	script := preflightScript(t)
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-public")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true, Indexable: true, SiteURL: "https://portal.abastevo.com.br"}); err != nil {
		t.Fatal(err)
	}
	manifest := preflightManifest(t, out)
	code, report := runPreflight(t, script, out, preflightScope(t, manifest, "https://portal.abastevo.com.br"))
	if code != 3 || report["status"] != "READY_FOR_EXECUTION_APPROVAL" {
		t.Fatalf("preflight = exit %d status %v report %v", code, report["status"], report)
	}
	if report["publication_executed"] != false {
		t.Fatal("preflight must never publish")
	}
}

func TestPreflightRefusesWrongOrigin(t *testing.T) {
	script := preflightScript(t)
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-public")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true, Indexable: true, SiteURL: "https://portal.abastevo.com.br"}); err != nil {
		t.Fatal(err)
	}
	manifest := preflightManifest(t, out)
	code, report := runPreflight(t, script, out, preflightScope(t, manifest, "https://portal2.abastevo.com.br"))
	if code != 1 || report["status"] != "FAILED_LOCAL_GATE" {
		t.Fatalf("wrong origin accepted: exit %d status %v", code, report["status"])
	}
}

func TestPreflightRefusesModeMismatch(t *testing.T) {
	script := preflightScript(t)
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-public")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true, Indexable: true, SiteURL: "https://portal.abastevo.com.br"}); err != nil {
		t.Fatal(err)
	}
	manifest := preflightManifest(t, out)
	scope := preflightScope(t, manifest, "https://portal.abastevo.com.br")
	scope["publication_mode"] = "full"
	code, report := runPreflight(t, script, out, scope)
	if code != 1 {
		t.Fatalf("mode mismatch accepted: exit %d status %v", code, report["status"])
	}
}

func TestPreflightRefusesNoindexPublicCandidate(t *testing.T) {
	script := preflightScript(t)
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-noindex")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true}); err != nil {
		t.Fatal(err)
	}
	manifest := preflightManifest(t, out)
	code, report := runPreflight(t, script, out, preflightScope(t, manifest, "https://portal.abastevo.com.br"))
	if code != 1 || report["status"] != "FAILED_LOCAL_GATE" {
		t.Fatalf("noindex candidate accepted for publication: exit %d status %v", code, report["status"])
	}
}

func TestPreflightRefusesBrokenArtifactLink(t *testing.T) {
	script := preflightScript(t)
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-broken")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true, Indexable: true, SiteURL: "https://portal.abastevo.com.br"}); err != nil {
		t.Fatal(err)
	}
	// A sitemap entry outside the approved origin must fail the gate even
	// when the page tree is intact.
	sitemapPath := filepath.Join(out, "sitemap.xml")
	raw, err := os.ReadFile(sitemapPath)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := strings.Replace(string(raw), "</urlset>", "<url><loc>https://evil.example/stolen/</loc></url></urlset>", 1)
	if err := os.WriteFile(sitemapPath, []byte(corrupt), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := preflightManifest(t, out)
	code, _ := runPreflight(t, script, out, preflightScope(t, manifest, "https://portal.abastevo.com.br"))
	if code != 1 {
		t.Fatalf("broken sitemap accepted: exit %d", code)
	}
}

func TestPreflightRefusesScopeWithoutApproval(t *testing.T) {
	script := preflightScript(t)
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-public")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true, Indexable: true, SiteURL: "https://portal.abastevo.com.br"}); err != nil {
		t.Fatal(err)
	}
	manifest := preflightManifest(t, out)
	scope := preflightScope(t, manifest, "https://portal.abastevo.com.br")
	delete(scope, "approval_ref")
	delete(scope, "approved_by")
	delete(scope, "host_ownership_attestation")
	code, report := runPreflight(t, script, out, scope)
	if code != 1 || report["status"] != "FAILED_SCOPE" {
		t.Fatalf("scope without approval accepted: exit %d status %v", code, report["status"])
	}
}
