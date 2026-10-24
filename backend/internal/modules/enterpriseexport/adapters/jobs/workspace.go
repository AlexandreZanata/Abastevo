package jobs

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

const StateSchemaVersion = "enterprise-export-state-v1"

var ErrBuildBusy = errors.New("enterprise export: another build holds the workspace lock")

type RevisionState struct {
	RevisionID     string `json:"revision_id"`
	SourceChecksum string `json:"source_checksum"`
}

type State struct {
	SchemaVersion        string          `json:"schema_version"`
	BuiltAt              string          `json:"built_at"`
	CandidateDir         string          `json:"candidate_dir"`
	ContentSHA256        string          `json:"content_sha256"`
	AnalyticsSHA256      string          `json:"analytics_sha256"`
	AnalyticsBytes       int64           `json:"analytics_bytes"`
	DirectoryFingerprint string          `json:"directory_fingerprint"`
	DirectoryStations    int64           `json:"directory_stations"`
	FromSurvey           string          `json:"from_survey"`
	ToSurvey             string          `json:"to_survey"`
	OfficialRevisions    []RevisionState `json:"official_revisions"`
	Official             *OfficialPin    `json:"official,omitempty"`
}

// Valid checks that the state points at a complete candidate payload.
func (s State) Valid() bool {
	if s.SchemaVersion != StateSchemaVersion || s.CandidateDir == "" ||
		s.ContentSHA256 == "" || s.AnalyticsSHA256 == "" {
		return false
	}
	manifestPath := filepath.Join(s.CandidateDir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return false
	}
	var manifest enterpriseexport.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.ContentSHA256 != s.ContentSHA256 {
		return false
	}
	for _, file := range manifest.Files {
		if _, err := os.Stat(filepath.Join(s.CandidateDir, file.Name)); err != nil {
			return false
		}
	}
	if _, err := os.Stat(filepath.Join(s.CandidateDir, "analytics-v1.json")); err != nil {
		return false
	}
	if s.Official != nil {
		if s.Official.PayloadSHA256 == "" {
			return false
		}
		payload, err := os.ReadFile(filepath.Join(s.CandidateDir, "official-records.jsonl"))
		if err != nil {
			return false
		}
		sum := sha256.Sum256(payload)
		if "sha256:"+hex.EncodeToString(sum[:]) != "sha256:"+s.Official.PayloadSHA256 {
			return false
		}
		if _, err := os.Stat(filepath.Join(s.CandidateDir, "official-records-manifest.json")); err != nil {
			return false
		}
	}
	return true
}

// LoadState treats a missing or corrupt state as absent; the build is safe to
// recreate it.
func LoadState(root string) (State, bool) {
	raw, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		return State{}, false
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil || state.SchemaVersion != StateSchemaVersion {
		return State{}, false
	}
	return state, true
}

// WriteState atomically replaces the state file.
func WriteState(root string, state State) error {
	if state.SchemaVersion != StateSchemaVersion {
		return fmt.Errorf("enterprise export: bad state schema")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(root, ".state-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if _, err := tmp.Write(raw); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(root, "state.json")); err != nil {
		return err
	}
	ok = true
	return nil
}

// Lock serializes the bounded builder on the private workspace. The OS releases
// the lock on process exit, so a crashed build cannot leave it held forever.
func Lock(root string) (func(), error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, ".build.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrBuildBusy
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

// CandidateDir reserves a fresh private candidate path. The export builder
// creates the directory itself via an atomic rename.
func CandidateDir(root string, now time.Time) (string, error) {
	dir := filepath.Join(root, "candidates")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	name := "export-" + now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(random[:])
	return filepath.Join(dir, name), nil
}

// CleanStale removes old temp/orphan candidates while preserving the candidate
// referenced by the current state.
func CleanStale(root, keep string, olderThan time.Duration, now time.Time) error {
	dir := filepath.Join(root, "candidates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := now.Add(-olderThan)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, ".export-") {
			continue
		}
		path := filepath.Join(dir, name)
		if path == keep {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
