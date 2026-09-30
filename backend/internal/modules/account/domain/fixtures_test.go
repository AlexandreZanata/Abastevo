package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type accountFixture struct {
	ID         string         `json:"id"`
	Version    int            `json:"version"`
	Provenance string         `json:"provenance"`
	Attack     string         `json:"attack"`
	Verdict    string         `json:"verdict"`
	Code       string         `json:"code"`
	Inputs     map[string]any `json:"inputs"`
	Note       string         `json:"note"`
}

func accountFixturesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts", "testdata", "account")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("fixtures dir missing: %s", dir)
	}
	return dir
}

// TestAccountAttackFixtures loads every account-*.json vector and enforces
// the frozen shape: stable id/version/provenance, a known attack family, an
// explicit accept/reject verdict with a stable code, and no real secrets.
func TestAccountAttackFixtures(t *testing.T) {
	dir := accountFixturesDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "account-") && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if len(files) < 10 {
		t.Fatalf("want at least 10 account fixtures, got %d", len(files))
	}
	validVerdicts := map[string]bool{"accept": true, "reject": true}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			var f accountFixture
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if f.ID == "" || f.Version != 1 || f.Provenance == "" {
				t.Errorf("fixture needs id/version=1/provenance: %+v", f)
			}
			if strings.TrimSuffix(name, ".json") != f.ID {
				t.Errorf("filename %q must match id %q", name, f.ID)
			}
			if f.Attack == "" || f.Note == "" {
				t.Errorf("fixture needs attack family and note: %+v", f)
			}
			if !validVerdicts[f.Verdict] {
				t.Errorf("verdict %q must be accept or reject", f.Verdict)
			}
			if f.Code == "" {
				t.Errorf("fixture needs a stable code")
			}
			if f.Verdict == "accept" && f.Code != "ok" {
				t.Errorf("accept verdict must carry code ok, got %q", f.Code)
			}
			body := strings.ToLower(string(raw))
			for _, secret := range []string{"ghp_", "private_key", "password"} {
				if strings.Contains(body, secret) {
					t.Errorf("fixture must not carry secrets (%q)", secret)
				}
			}
		})
	}
}
