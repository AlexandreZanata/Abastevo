package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"os"
	"path/filepath"
	"testing"
	"time"

	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

func TestStateRoundTripAndValidity(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "candidates", "export-test")
	if err := os.MkdirAll(candidate, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"content_sha256":      "sha256:test",
		"public_field_policy": enterpriseexport.PublicFieldPolicy,
		"files": []map[string]any{
			{"name": "stations.jsonl"},
			{"name": "revisions.jsonl"},
			{"name": "prices.jsonl"},
		},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(candidate, "manifest.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stations.jsonl", "revisions.jsonl", "prices.jsonl", "analytics-v1.json"} {
		if err := os.WriteFile(filepath.Join(candidate, name), []byte(""), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	state := State{
		SchemaVersion:     StateSchemaVersion,
		BuiltAt:           "2026-10-24T12:00:00Z",
		CandidateDir:      candidate,
		ContentSHA256:     "sha256:test",
		AnalyticsSHA256:   "sha256:analytics",
		OfficialRevisions: []RevisionState{{RevisionID: "r1", SourceChecksum: "sha256:r1"}},
	}
	if !state.Valid() {
		t.Fatal("complete state is invalid")
	}
	if err := WriteState(root, state); err != nil {
		t.Fatal(err)
	}
	loaded, ok := LoadState(root)
	if !ok || loaded.CandidateDir != candidate || loaded.ContentSHA256 != state.ContentSHA256 {
		t.Fatalf("loaded = %+v, %v", loaded, ok)
	}
	if err := os.Remove(filepath.Join(candidate, "prices.jsonl")); err != nil {
		t.Fatal(err)
	}
	if loaded.Valid() {
		t.Fatal("state with a missing payload is valid")
	}
}

func TestStateBuiltUnderOlderPublicFieldPolicyIsInvalid(t *testing.T) {
	candidate := filepath.Join(t.TempDir(), "export-old-policy")
	if err := os.MkdirAll(candidate, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"content_sha256": "sha256:old", "public_field_policy": "esp-01",
		"files": []map[string]any{{"name": "stations.jsonl"}},
	})
	if err := os.WriteFile(filepath.Join(candidate, "manifest.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stations.jsonl", "analytics-v1.json"} {
		if err := os.WriteFile(filepath.Join(candidate, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	state := State{SchemaVersion: StateSchemaVersion, CandidateDir: candidate, ContentSHA256: "sha256:old", AnalyticsSHA256: "sha256:a"}
	if state.Valid() {
		t.Fatal("a snapshot built under an older public field policy is reused")
	}
}

func TestLockIsExclusive(t *testing.T) {
	root := t.TempDir()
	release, err := Lock(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(root); !errors.Is(err, ErrBuildBusy) {
		t.Fatalf("second lock = %v, want ErrBuildBusy", err)
	}
	release()
	release2, err := Lock(root)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	release2()
}

func TestCleanStaleOnlyRemovesOldTempCandidates(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "candidates")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	makeDir := func(name string, age time.Duration) string {
		path := filepath.Join(dir, name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		mod := time.Now().Add(-age)
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
		return path
	}
	oldTemp := makeDir(".export-old.tmp-1", 48*time.Hour)
	keep := makeDir("export-keep", 48*time.Hour)
	newTemp := makeDir(".export-new.tmp-2", time.Hour)
	if err := CleanStale(root, keep, 24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldTemp); !os.IsNotExist(err) {
		t.Error("old temp candidate survived")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("kept candidate removed")
	}
	if _, err := os.Stat(newTemp); err != nil {
		t.Error("recent temp candidate removed")
	}
}

func TestCandidateDirIsUnique(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	a, err := CandidateDir(root, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CandidateDir(root, now)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("candidate paths collide")
	}
}

func TestWindowFor(t *testing.T) {
	latest := officialapp.ExportRevision{SurveyStart: "2026-10-18", SurveyEnd: "2026-10-24"}
	from, to, err := windowFor(latest, 5)
	if err != nil {
		t.Fatal(err)
	}
	if from != "2026-09-20" || to != "2026-10-24" {
		t.Fatalf("window = %s..%s", from, to)
	}
	from, to, err = windowFor(latest, 1)
	if err != nil || from != "2026-10-18" || to != "2026-10-24" {
		t.Fatalf("single-week window = %s..%s, %v", from, to, err)
	}
	if _, _, err := windowFor(officialapp.ExportRevision{SurveyStart: "bad", SurveyEnd: "2026-10-24"}, 5); err == nil {
		t.Error("invalid start accepted")
	}
}

func TestExportStateKeyIncludesOfficialPin(t *testing.T) {
	fingerprint := directoryapp.ExportFingerprint{Stations: 2, Hash: "md5:a:b"}
	pins := []RevisionState{{RevisionID: "r1", SourceChecksum: "sha256:r1"}}
	base := exportStateKey(fingerprint, pins, nil)
	official := &OfficialPin{RevisionID: "f1", LeaseID: "l1", Edition: "2025", Rows: 1}
	withOfficial := exportStateKey(fingerprint, pins, official)
	if base == withOfficial {
		t.Error("official pin did not change the export state key")
	}
	official.LeaseID = "l2"
	if withOfficial == exportStateKey(fingerprint, pins, official) {
		t.Error("official lease did not change the export state key")
	}
}

func TestStateMatches(t *testing.T) {
	state := State{
		DirectoryFingerprint: "md5:a:b",
		DirectoryStations:    2,
		FromSurvey:           "2026-01-01",
		ToSurvey:             "2026-12-31",
		OfficialRevisions: []RevisionState{
			{RevisionID: "r1", SourceChecksum: "sha256:r1"},
			{RevisionID: "r2", SourceChecksum: "sha256:r2"},
		},
	}
	fingerprint := directoryapp.ExportFingerprint{Stations: 2, Hash: "md5:a:b"}
	pins := []RevisionState{
		{RevisionID: "r1", SourceChecksum: "sha256:r1"},
		{RevisionID: "r2", SourceChecksum: "sha256:r2"},
	}
	if !stateMatches(state, fingerprint, pins, "2026-01-01", "2026-12-31", nil) {
		t.Error("matching state rejected")
	}
	pins[1].SourceChecksum = "sha256:changed"
	if stateMatches(state, fingerprint, pins, "2026-01-01", "2026-12-31", nil) {
		t.Error("changed revision accepted")
	}
	if stateMatches(state, directoryapp.ExportFingerprint{Stations: 3, Hash: "md5:a:b"}, pins, "2026-01-01", "2026-12-31", nil) {
		t.Error("changed station count accepted")
	}
	if stateMatches(state, fingerprint, pins, "2026-02-01", "2026-12-31", nil) {
		t.Error("changed window accepted")
	}
	pins[1].SourceChecksum = "sha256:r2"
	official := &OfficialPin{RevisionID: "r1", LeaseID: "l1", Edition: "2025", SourceChecksum: "sum", Rows: 3, Accepted: 3}
	withOfficial := state
	withOfficial.Official = official
	if !stateMatches(withOfficial, fingerprint, pins, "2026-01-01", "2026-12-31", official) {
		t.Error("matching official pin rejected")
	}
	changed := *official
	changed.LeaseID = "l2"
	if stateMatches(withOfficial, fingerprint, pins, "2026-01-01", "2026-12-31", &changed) {
		t.Error("changed official pin accepted")
	}
}

func TestCollectPinsPaginatesAndBounds(t *testing.T) {
	reader := &fakeExportReader{revisions: []officialapp.ExportRevision{
		{ID: "00000000-0000-4000-8000-000000000003", SourceChecksum: "sha256:3"},
		{ID: "00000000-0000-4000-8000-000000000001", SourceChecksum: "sha256:1"},
		{ID: "00000000-0000-4000-8000-000000000002", SourceChecksum: "sha256:2"},
	}}
	pins, err := collectPins(context.Background(), reader, "2026-01-01", "2026-12-31", 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 3 || pins[0].RevisionID != "00000000-0000-4000-8000-000000000001" ||
		pins[2].RevisionID != "00000000-0000-4000-8000-000000000003" {
		t.Fatalf("pins = %+v", pins)
	}
	if _, err := collectPins(context.Background(), reader, "2026-01-01", "2026-12-31", 2, 2); err == nil {
		t.Error("revision budget not enforced")
	}
}

type fakeExportReader struct {
	revisions []officialapp.ExportRevision
}

func (f *fakeExportReader) ExportRevisions(_ context.Context, _, _, afterID string, limit int) ([]officialapp.ExportRevision, string, error) {
	start := 0
	if afterID != "" {
		for i, revision := range f.revisions {
			if revision.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	if end > len(f.revisions) {
		end = len(f.revisions)
	}
	next := ""
	if end < len(f.revisions) && end > start {
		next = f.revisions[end-1].ID
	}
	return f.revisions[start:end], next, nil
}

func (f *fakeExportReader) ExportPrices(context.Context, string, string, int) ([]officialapp.ExportPrice, string, error) {
	return nil, "", nil
}

func (f *fakeExportReader) LatestRevision(context.Context) (officialapp.ExportRevision, error) {
	return f.revisions[0], nil
}
