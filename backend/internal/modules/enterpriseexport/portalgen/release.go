package portalgen

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	ReleaseStateSchema = "enterprise-portal-release-v1"
	ReleasePointer     = "current"
	ReleaseDir         = "releases"
	ReleaseStateFile   = "state.json"
)

var (
	ErrReleaseBusy        = errors.New("portal release: another promotion holds the lock")
	ErrReleaseFence       = errors.New("portal release: current release changed")
	ErrReleaseState       = errors.New("portal release: state unavailable")
	ErrReleaseUnavailable = errors.New("portal release: release unavailable")
	ErrReleaseWithdrawn   = errors.New("portal release: release withdrawn")
	ErrNoCurrentRelease   = errors.New("portal release: no current release")
)

type Release struct {
	ContentSHA256          string `json:"content_sha256"`
	PromotedAt             string `json:"promoted_at"`
	Pages                  int    `json:"pages"`
	Files                  int    `json:"files"`
	Bytes                  int64  `json:"bytes"`
	OfficialRevisionID     string `json:"official_revision_id,omitempty"`
	OfficialLeaseID        string `json:"official_lease_id,omitempty"`
	OfficialEdition        string `json:"official_edition,omitempty"`
	OfficialSourceChecksum string `json:"official_source_checksum,omitempty"`
	Withdrawn              bool   `json:"withdrawn,omitempty"`
}

type ReleaseState struct {
	SchemaVersion string    `json:"schema_version"`
	Current       string    `json:"current"`
	Previous      string    `json:"previous,omitempty"`
	Releases      []Release `json:"releases"`
	UpdatedAt     string    `json:"updated_at"`
}

type PromoteOptions struct {
	ArtifactDir     string
	RootDir         string
	SiteURL         string
	ExpectedCurrent string
	Fence           bool
	Now             func() time.Time
}

type RollbackOptions struct {
	RootDir string
	Target  string
	SiteURL string
	Now     func() time.Time
}

type WithdrawOptions struct {
	RootDir  string
	Fallback string
	SiteURL  string
	Now      func() time.Time
}

// copyFileHook is the bounded copy boundary. Tests replace it to inject
// ENOSPC, corruption or a pause between complete copy and pointer switch.
var copyFileHook = copyFile

func Promote(opts PromoteOptions) (ReleaseState, error) {
	if opts.ArtifactDir == "" || opts.RootDir == "" || opts.SiteURL == "" {
		return ReleaseState{}, fmt.Errorf("portal release: artifact, root and site URL are required")
	}
	if err := ensurePrivateRoot(opts.RootDir); err != nil {
		return ReleaseState{}, err
	}
	artifactAbs, _ := filepath.Abs(opts.ArtifactDir)
	rootAbs, _ := filepath.Abs(opts.RootDir)
	if artifactAbs == rootAbs || inside(opts.ArtifactDir, opts.RootDir) || inside(filepath.Join(opts.RootDir, ReleaseDir), opts.ArtifactDir) {
		return ReleaseState{}, fmt.Errorf("portal release: artifact and root overlap")
	}
	unlock, err := lockRelease(opts.RootDir)
	if err != nil {
		return ReleaseState{}, err
	}
	defer unlock()
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	verified, err := Verify(opts.ArtifactDir, VerifyOptions{SiteURL: opts.SiteURL})
	if err != nil {
		return ReleaseState{}, fmt.Errorf("portal release: candidate verification: %w", err)
	}
	portalManifest, err := readPortalManifest(opts.ArtifactDir)
	if err != nil {
		return ReleaseState{}, err
	}
	content := verified.ContentSHA256
	releaseMeta := releaseMetadata(portalManifest)
	current, currentExists, err := readCurrent(opts.RootDir)
	if err != nil {
		return ReleaseState{}, err
	}
	state, stateExists := LoadReleaseState(opts.RootDir)
	if currentExists && !stateExists {
		return ReleaseState{}, ErrReleaseState
	}
	if !stateExists {
		state = ReleaseState{SchemaVersion: ReleaseStateSchema, Releases: []Release{}}
	}
	if opts.Fence && current != opts.ExpectedCurrent {
		return ReleaseState{}, ErrReleaseFence
	}
	if current == content {
		release, ok := findRelease(state, content)
		if release.Withdrawn {
			return ReleaseState{}, ErrReleaseState
		}
		if err := verifyRelease(opts.RootDir, content, opts.SiteURL); err != nil {
			return ReleaseState{}, ErrReleaseUnavailable
		}
		release.ContentSHA256 = content
		release.Pages = verified.Pages
		release.Files = verified.Files
		release.Bytes = verified.Bytes
		release.OfficialRevisionID = releaseMeta.OfficialRevisionID
		release.OfficialLeaseID = releaseMeta.OfficialLeaseID
		release.OfficialEdition = releaseMeta.OfficialEdition
		release.OfficialSourceChecksum = releaseMeta.OfficialSourceChecksum
		if release.PromotedAt == "" {
			release.PromotedAt = now().UTC().Format(time.RFC3339)
		}
		if state.Current != content || !ok {
			previous := state.Current
			if previous == content {
				previous = state.Previous
			}
			state = updateRelease(state, release, content, previous)
			if err := writeReleaseState(opts.RootDir, state); err != nil {
				return ReleaseState{}, err
			}
		}
		return state, nil
	}

	targetName := releaseName(content)
	releasePath := filepath.Join(opts.RootDir, ReleaseDir, targetName)
	if _, err := os.Stat(releasePath); err == nil {
		if err := verifyRelease(opts.RootDir, content, opts.SiteURL); err != nil {
			if current == content {
				return ReleaseState{}, ErrReleaseUnavailable
			}
			if err := os.RemoveAll(releasePath); err != nil {
				return ReleaseState{}, err
			}
		}
	} else if !os.IsNotExist(err) {
		return ReleaseState{}, err
	}
	if _, err := os.Stat(releasePath); os.IsNotExist(err) {
		tmp, err := os.MkdirTemp(filepath.Join(opts.RootDir, ReleaseDir), ".tmp-"+targetName+"-")
		if err != nil {
			return ReleaseState{}, err
		}
		keep := false
		defer func() {
			if !keep {
				_ = os.RemoveAll(tmp)
			}
		}()
		if err := copyArtifact(opts.ArtifactDir, tmp); err != nil {
			return ReleaseState{}, err
		}
		if err := verifyReleaseDir(tmp, opts.SiteURL, content); err != nil {
			return ReleaseState{}, fmt.Errorf("portal release: copied artifact verification: %w", err)
		}
		if err := os.Rename(tmp, releasePath); err != nil {
			return ReleaseState{}, err
		}
		keep = true
		if err := syncDir(filepath.Join(opts.RootDir, ReleaseDir)); err != nil {
			return ReleaseState{}, err
		}
	} else if err != nil {
		return ReleaseState{}, err
	}

	if err := switchCurrent(opts.RootDir, targetName); err != nil {
		return ReleaseState{}, err
	}
	state = updateRelease(state, Release{
		ContentSHA256: content, PromotedAt: now().UTC().Format(time.RFC3339),
		Pages: verified.Pages, Files: verified.Files, Bytes: verified.Bytes,
		OfficialRevisionID:     releaseMeta.OfficialRevisionID,
		OfficialLeaseID:        releaseMeta.OfficialLeaseID,
		OfficialEdition:        releaseMeta.OfficialEdition,
		OfficialSourceChecksum: releaseMeta.OfficialSourceChecksum,
	}, content, current)
	if err := writeReleaseState(opts.RootDir, state); err != nil {
		return ReleaseState{}, err
	}
	return state, nil
}

func Rollback(opts RollbackOptions) (ReleaseState, error) {
	if opts.RootDir == "" || opts.Target == "" || opts.SiteURL == "" {
		return ReleaseState{}, fmt.Errorf("portal release: root, target and site URL are required")
	}
	unlock, err := lockRelease(opts.RootDir)
	if err != nil {
		return ReleaseState{}, err
	}
	defer unlock()
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	state, ok := LoadReleaseState(opts.RootDir)
	if !ok {
		return ReleaseState{}, ErrReleaseState
	}
	current, exists, err := readCurrent(opts.RootDir)
	if err != nil {
		return ReleaseState{}, err
	}
	if !exists {
		return ReleaseState{}, ErrNoCurrentRelease
	}
	release, ok := findRelease(state, opts.Target)
	if !ok || release.Withdrawn {
		return ReleaseState{}, ErrReleaseWithdrawn
	}
	if opts.Target == current {
		if err := verifyRelease(opts.RootDir, current, opts.SiteURL); err != nil {
			return ReleaseState{}, ErrReleaseUnavailable
		}
		return state, nil
	}
	if err := verifyRelease(opts.RootDir, opts.Target, opts.SiteURL); err != nil {
		return ReleaseState{}, ErrReleaseUnavailable
	}
	if err := switchCurrent(opts.RootDir, releaseName(opts.Target)); err != nil {
		return ReleaseState{}, err
	}
	state.Previous = current
	state.Current = opts.Target
	state.UpdatedAt = now().UTC().Format(time.RFC3339)
	if err := writeReleaseState(opts.RootDir, state); err != nil {
		return ReleaseState{}, err
	}
	return state, nil
}

func Withdraw(opts WithdrawOptions) (ReleaseState, error) {
	if opts.RootDir == "" || opts.SiteURL == "" {
		return ReleaseState{}, fmt.Errorf("portal release: root and site URL are required")
	}
	unlock, err := lockRelease(opts.RootDir)
	if err != nil {
		return ReleaseState{}, err
	}
	defer unlock()
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	state, ok := LoadReleaseState(opts.RootDir)
	if !ok {
		return ReleaseState{}, ErrReleaseState
	}
	current, exists, err := readCurrent(opts.RootDir)
	if err != nil {
		return ReleaseState{}, err
	}
	if !exists {
		return ReleaseState{}, ErrNoCurrentRelease
	}
	for i := range state.Releases {
		if state.Releases[i].ContentSHA256 == current {
			state.Releases[i].Withdrawn = true
		}
	}
	if opts.Fallback != "" {
		if opts.Fallback == current {
			return ReleaseState{}, ErrReleaseWithdrawn
		}
		release, ok := findRelease(state, opts.Fallback)
		if !ok || release.Withdrawn {
			return ReleaseState{}, ErrReleaseWithdrawn
		}
		if err := verifyRelease(opts.RootDir, opts.Fallback, opts.SiteURL); err != nil {
			return ReleaseState{}, ErrReleaseUnavailable
		}
		if err := switchCurrent(opts.RootDir, releaseName(opts.Fallback)); err != nil {
			return ReleaseState{}, err
		}
		state.Previous = current
		state.Current = opts.Fallback
	} else {
		if err := os.Remove(filepath.Join(opts.RootDir, ReleasePointer)); err != nil && !os.IsNotExist(err) {
			return ReleaseState{}, err
		}
		state.Previous = current
		state.Current = ""
	}
	state.UpdatedAt = now().UTC().Format(time.RFC3339)
	if err := writeReleaseState(opts.RootDir, state); err != nil {
		return ReleaseState{}, err
	}
	return state, nil
}

// CurrentReleaseHeader returns the selected release metadata without rehashing
// the complete tree. Promotion verifies the candidate and copied release before
// switching; use CurrentRelease (or the portalcheck verify command) for an
// explicit full integrity check.
func CurrentReleaseHeader(root string) (Release, ReleaseState, error) {
	state, ok := LoadReleaseState(root)
	if !ok {
		return Release{}, ReleaseState{}, ErrReleaseState
	}
	current, exists, err := readCurrent(root)
	if err != nil {
		return Release{}, ReleaseState{}, err
	}
	if !exists {
		return Release{}, ReleaseState{}, ErrNoCurrentRelease
	}
	if state.Current != current {
		return Release{}, ReleaseState{}, ErrReleaseState
	}
	release, ok := findRelease(state, current)
	if !ok || release.Withdrawn {
		return Release{}, ReleaseState{}, ErrReleaseWithdrawn
	}
	return release, state, nil
}

func CurrentRelease(root, siteURL string) (Release, ReleaseState, error) {
	state, ok := LoadReleaseState(root)
	if !ok {
		return Release{}, ReleaseState{}, ErrReleaseState
	}
	current, exists, err := readCurrent(root)
	if err != nil {
		return Release{}, ReleaseState{}, err
	}
	if !exists {
		return Release{}, ReleaseState{}, ErrNoCurrentRelease
	}
	if state.Current != current {
		return Release{}, ReleaseState{}, ErrReleaseState
	}
	release, ok := findRelease(state, current)
	if !ok || release.Withdrawn {
		return Release{}, ReleaseState{}, ErrReleaseWithdrawn
	}
	if err := verifyRelease(root, current, siteURL); err != nil {
		return Release{}, ReleaseState{}, ErrReleaseUnavailable
	}
	return release, state, nil
}

func LoadReleaseState(root string) (ReleaseState, bool) {
	raw, err := os.ReadFile(filepath.Join(root, ReleaseStateFile))
	if err != nil {
		return ReleaseState{}, false
	}
	var state ReleaseState
	if err := json.Unmarshal(raw, &state); err != nil || state.SchemaVersion != ReleaseStateSchema {
		return ReleaseState{}, false
	}
	return state, true
}

func writeReleaseState(root string, state ReleaseState) error {
	if state.SchemaVersion != ReleaseStateSchema {
		return fmt.Errorf("portal release: bad state schema")
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
	if err := os.Rename(name, filepath.Join(root, ReleaseStateFile)); err != nil {
		return err
	}
	ok = true
	return syncDir(root)
}

func updateRelease(state ReleaseState, next Release, content, previous string) ReleaseState {
	state.SchemaVersion = ReleaseStateSchema
	state.Current = content
	state.Previous = previous
	state.UpdatedAt = next.PromotedAt
	for i := range state.Releases {
		if state.Releases[i].ContentSHA256 == content {
			state.Releases[i] = next
			return state
		}
	}
	state.Releases = append(state.Releases, next)
	return state
}

func findRelease(state ReleaseState, content string) (Release, bool) {
	for _, release := range state.Releases {
		if release.ContentSHA256 == content {
			return release, true
		}
	}
	return Release{}, false
}

func ensurePrivateRoot(root string) error {
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("portal release: root must not be a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ReleaseDir), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return err
	}
	return os.Chmod(filepath.Join(root, ReleaseDir), 0o700)
}

func lockRelease(root string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(root, ".promote.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrReleaseBusy
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func readCurrent(root string) (string, bool, error) {
	path := filepath.Join(root, ReleasePointer)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", false, fmt.Errorf("portal release: current pointer is not a symlink")
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", false, err
	}
	clean := filepath.Clean(target)
	parts := strings.Split(clean, string(os.PathSeparator))
	if len(parts) != 2 || parts[0] != ReleaseDir || !validReleaseName(parts[1]) {
		return "", false, fmt.Errorf("portal release: current pointer is outside releases")
	}
	return "sha256:" + parts[1], true, nil
}

func switchCurrent(root, name string) error {
	if !validReleaseName(name) {
		return fmt.Errorf("portal release: invalid release name")
	}
	if _, err := os.Stat(filepath.Join(root, ReleaseDir, name)); err != nil {
		return err
	}
	random, err := randomSuffix()
	if err != nil {
		return err
	}
	tmp := filepath.Join(root, ".current-"+random)
	if err := os.Symlink(filepath.Join(ReleaseDir, name), tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(root, ReleasePointer)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return syncDir(root)
}

func readPortalManifest(dir string) (Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "portal-manifest.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("portal release: portal manifest unavailable")
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("portal release: portal manifest invalid")
	}
	return manifest, nil
}

func releaseMetadata(manifest Manifest) Release {
	if manifest.OfficialRecords == nil {
		return Release{}
	}
	return Release{
		OfficialRevisionID:     manifest.OfficialRecords.RevisionID,
		OfficialLeaseID:        manifest.OfficialRecords.LeaseID,
		OfficialEdition:        manifest.OfficialRecords.Edition,
		OfficialSourceChecksum: manifest.OfficialRecords.SourceChecksum,
	}
}

func verifyRelease(root, content, siteURL string) error {
	return verifyReleaseDir(releasePath(root, content), siteURL, content)
}

func verifyReleaseDir(path, siteURL, expected string) error {
	result, err := Verify(path, VerifyOptions{SiteURL: siteURL})
	if err != nil {
		return err
	}
	if result.ContentSHA256 != expected {
		return fmt.Errorf("portal release: content hash mismatch")
	}
	return nil
}

func releasePath(root, content string) string {
	name := releaseName(content)
	return filepath.Join(root, ReleaseDir, name)
}

func releaseName(content string) string {
	return strings.TrimPrefix(content, "sha256:")
}

func validReleaseName(name string) bool {
	if len(name) != 64 || name != strings.ToLower(name) {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil
}

func copyArtifact(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("portal release: symlink in candidate")
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			if rel == "." {
				return nil
			}
			return os.MkdirAll(target, 0o700)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return copyFileHook(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, 0o600); err != nil {
		return err
	}
	ok = true
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func randomSuffix() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func inside(parent, child string) bool {
	parent, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	child, err = filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
