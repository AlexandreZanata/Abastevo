package portalgen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	ReleaseStateSchema = "enterprise-portal-release-v1"
	ReleasePointer     = "current"
	ReleaseDir         = "releases"
	ReleaseStateFile   = "state.json"
	// RetainedDir holds hardlinked copies of previous releases' assets so
	// cached old pages keep loading correct dependencies across switches.
	// Pages are never retained: only the current release serves HTML.
	RetainedDir = "retained"
)

const (
	defaultRetainKeep     = 2
	defaultRetainMaxBytes = 100 << 20
)

const (
	// withdrawalPause marks a temporary operational withdrawal. The same
	// source content may be served again once the authoritative source is
	// published; nothing about the official identity is revoked.
	withdrawalPause = "operator-pause"
	// withdrawalRevoked marks invalid source content. Its official
	// revision/lease identity can never be served again by any hash.
	withdrawalRevoked = "revoked-source"
)

var (
	ErrReleaseBusy        = errors.New("portal release: another promotion holds the lock")
	ErrReleaseFence       = errors.New("portal release: current release changed")
	ErrReleaseState       = errors.New("portal release: state unavailable")
	ErrReleaseUnavailable = errors.New("portal release: release unavailable")
	ErrReleaseWithdrawn   = errors.New("portal release: release withdrawn")
	ErrOfficialRevoked    = errors.New("portal release: official revision revoked")
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
	WithdrawalReason       string `json:"withdrawal_reason,omitempty"`
}

type ReleaseState struct {
	SchemaVersion string    `json:"schema_version"`
	Current       string    `json:"current"`
	Previous      string    `json:"previous,omitempty"`
	Releases      []Release `json:"releases"`
	UpdatedAt     string    `json:"updated_at"`
	// RevokedOfficial records official revision/lease identities withdrawn
	// as invalid source content. A revoked identity can never be served
	// again by any artifact or hash; only an operator-pause withdrawal
	// (Reason "") leaves the content eligible for a future release.
	RevokedOfficial []OfficialRevocation `json:"revoked_official,omitempty"`
}

// OfficialRevocation is the retained identity of source content withdrawn as
// invalid. It survives restarts in state.json; there is no administrative
// path that clears it.
type OfficialRevocation struct {
	RevisionID  string `json:"revision_id"`
	LeaseID     string `json:"lease_id"`
	Edition     string `json:"edition,omitempty"`
	Reason      string `json:"reason"`
	WithdrawnAt string `json:"withdrawn_at"`
}

type PromoteOptions struct {
	ArtifactDir     string
	RootDir         string
	SiteURL         string
	ExpectedCurrent string
	Fence           bool
	Now             func() time.Time
	OfficialOnly    bool
	RetainKeep      int
	RetainMaxBytes  int64
}

type RollbackOptions struct {
	RootDir        string
	Target         string
	SiteURL        string
	Now            func() time.Time
	OfficialOnly   bool
	RetainKeep     int
	RetainMaxBytes int64
}

type WithdrawOptions struct {
	RootDir      string
	Fallback     string
	SiteURL      string
	Now          func() time.Time
	OfficialOnly bool
	// Reason distinguishes an operational pause ("", "operator-pause":
	// content stays eligible) from an invalid-content withdrawal
	// ("revoked-source": the official revision/lease is recorded as
	// revoked and can never be served again). Unknown reasons refuse.
	Reason string
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

	verified, err := Verify(opts.ArtifactDir, VerifyOptions{SiteURL: opts.SiteURL, OfficialOnly: opts.OfficialOnly})
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
	// A revoked release can never be served again. An operational pause
	// stays eligible: the same source content may be promoted once the
	// authoritative source is published again, clearing the pause mark.
	if existing, ok := findRelease(state, content); ok && existing.Withdrawn && !recoverablePause(state, existing) {
		return ReleaseState{}, ErrReleaseWithdrawn
	}
	if revokedOfficial(state, releaseMeta) {
		return ReleaseState{}, ErrOfficialRevoked
	}
	if current == content {
		release, ok := findRelease(state, content)
		wasWithdrawn := release.Withdrawn
		if release.Withdrawn && !recoverablePause(state, release) {
			return ReleaseState{}, ErrReleaseWithdrawn
		}
		if err := verifyRelease(opts.RootDir, content, opts.SiteURL, opts.OfficialOnly); err != nil {
			return ReleaseState{}, ErrReleaseUnavailable
		}
		release.ContentSHA256 = content
		release.Withdrawn = false
		release.WithdrawalReason = ""
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
		if _, err := rebuildRetained(opts.RootDir, state, []string{content}, opts.RetainKeep, opts.RetainMaxBytes); err != nil {
			return ReleaseState{}, fmt.Errorf("portal release: asset preparation: %w", err)
		}
		if state.Current != content || !ok || wasWithdrawn {
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
		if err := verifyRelease(opts.RootDir, content, opts.SiteURL, opts.OfficialOnly); err != nil {
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
		if err := verifyReleaseDir(tmp, opts.SiteURL, content, opts.OfficialOnly); err != nil {
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

	if _, err := rebuildRetained(opts.RootDir, state, []string{content}, opts.RetainKeep, opts.RetainMaxBytes); err != nil {
		return ReleaseState{}, fmt.Errorf("portal release: asset preparation: %w", err)
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
	if revokedOfficial(state, release) {
		return ReleaseState{}, ErrOfficialRevoked
	}
	if opts.Target == current {
		if err := verifyRelease(opts.RootDir, current, opts.SiteURL, opts.OfficialOnly); err != nil {
			return ReleaseState{}, ErrReleaseUnavailable
		}
		if _, err := rebuildRetained(opts.RootDir, state, []string{current}, opts.RetainKeep, opts.RetainMaxBytes); err != nil {
			return ReleaseState{}, fmt.Errorf("portal release: asset preparation: %w", err)
		}
		return state, nil
	}
	if err := verifyRelease(opts.RootDir, opts.Target, opts.SiteURL, opts.OfficialOnly); err != nil {
		return ReleaseState{}, ErrReleaseUnavailable
	}
	if _, err := rebuildRetained(opts.RootDir, state, []string{opts.Target}, opts.RetainKeep, opts.RetainMaxBytes); err != nil {
		return ReleaseState{}, fmt.Errorf("portal release: asset preparation: %w", err)
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
	switch opts.Reason {
	case "", "operator-pause", "revoked-source":
	default:
		return ReleaseState{}, fmt.Errorf("portal release: unknown withdrawal reason %q", opts.Reason)
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
	withdrawnAt := now().UTC().Format(time.RFC3339)
	reason := withdrawalPause
	if opts.Reason == withdrawalRevoked {
		reason = withdrawalRevoked
	}
	for i := range state.Releases {
		if state.Releases[i].ContentSHA256 == current {
			state.Releases[i].Withdrawn = true
			state.Releases[i].WithdrawalReason = reason
			// Invalid source content revokes its official identity: no
			// artifact or hash carrying this revision/lease may be served
			// again. An operational pause records no revocation.
			if reason == withdrawalRevoked {
				if state.Releases[i].OfficialRevisionID == "" || state.Releases[i].OfficialLeaseID == "" {
					return ReleaseState{}, fmt.Errorf("portal release: revoked-source requires an official pin")
				}
				state = recordRevocation(state, state.Releases[i], reason, withdrawnAt)
			}
		}
	}
	fallback := opts.Fallback
	if fallback != "" {
		if fallback == current {
			return ReleaseState{}, ErrReleaseWithdrawn
		}
		release, ok := findRelease(state, fallback)
		if !ok || release.Withdrawn {
			return ReleaseState{}, ErrReleaseWithdrawn
		}
		if revokedOfficial(state, release) {
			return ReleaseState{}, ErrOfficialRevoked
		}
		if err := verifyRelease(opts.RootDir, fallback, opts.SiteURL, opts.OfficialOnly); err != nil {
			return ReleaseState{}, ErrReleaseUnavailable
		}
	}
	// Prepare the post-withdrawal asset store before changing the pointer:
	// withdrawn content never survives, shared assets of the fallback stay,
	// and a failed rebuild leaves the previous pointer untouched.
	var required []string
	if fallback != "" {
		required = append(required, fallback)
	}
	if _, err := rebuildRetained(opts.RootDir, state, required, defaultRetainKeep, defaultRetainMaxBytes); err != nil {
		return ReleaseState{}, err
	}
	if fallback != "" {
		if err := switchCurrent(opts.RootDir, releaseName(fallback)); err != nil {
			return ReleaseState{}, err
		}
		state.Previous = current
		state.Current = fallback
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
	if err := verifyRetainedCurrent(root, current); err != nil {
		return Release{}, ReleaseState{}, err
	}
	return release, state, nil
}

func CurrentRelease(root, siteURL string, officialOnly bool) (Release, ReleaseState, error) {
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
	if err := verifyRelease(root, current, siteURL, officialOnly); err != nil {
		return Release{}, ReleaseState{}, ErrReleaseUnavailable
	}
	if err := verifyRetainedCurrent(root, current); err != nil {
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

// revokedOfficial reports whether the candidate's official revision/lease
// was withdrawn as invalid source content. Candidates without an official
// pin are never revoked.
// recoverablePause reports whether a withdrawal is an operational pause for
// content whose official identity is still valid. Legacy withdrawals without
// a persisted reason fail closed, and a revocation always wins.
func recoverablePause(state ReleaseState, release Release) bool {
	return release.Withdrawn && release.WithdrawalReason == withdrawalPause && !revokedOfficial(state, release)
}

func revokedOfficial(state ReleaseState, meta Release) bool {
	if meta.OfficialRevisionID == "" || meta.OfficialLeaseID == "" {
		return false
	}
	for _, revoked := range state.RevokedOfficial {
		if revoked.RevisionID == meta.OfficialRevisionID && revoked.LeaseID == meta.OfficialLeaseID {
			return true
		}
	}
	return false
}

// recordRevocation appends the release's official identity to the revoked
// list, preserving history across restarts. Duplicates converge.
func recordRevocation(state ReleaseState, release Release, reason, at string) ReleaseState {
	revocation := OfficialRevocation{
		RevisionID: release.OfficialRevisionID, LeaseID: release.OfficialLeaseID,
		Edition: release.OfficialEdition, Reason: reason, WithdrawnAt: at,
	}
	for _, existing := range state.RevokedOfficial {
		if existing.RevisionID == revocation.RevisionID && existing.LeaseID == revocation.LeaseID {
			return state
		}
	}
	state.RevokedOfficial = append(state.RevokedOfficial, revocation)
	return state
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

func verifyRelease(root, content, siteURL string, officialOnly bool) error {
	return verifyReleaseDir(releasePath(root, content), siteURL, content, officialOnly)
}

func verifyReleaseDir(path, siteURL, expected string, officialOnly bool) error {
	result, err := Verify(path, VerifyOptions{SiteURL: siteURL, OfficialOnly: officialOnly})
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

// RetainedSummary reports one asset-retention rebuild.
type RetainedSummary struct {
	Releases int
	Files    int
	Bytes    int64
}

// RetainAssets rebuilds the retained asset store around the current release
// plus the newest `keep` non-withdrawn releases (by promotion time). The
// current release is mandatory even when it is older than the keep window or
// a rollback target; a page that can be served as current never depends on
// promotion ordering. Mandatory assets must fit maxBytes before any pointer
// is exposed. Shared paths are hardlinked once and counted once. Withdrawn
// releases never contribute.
func RetainAssets(root string, keep int, maxBytes int64) (RetainedSummary, error) {
	var summary RetainedSummary
	if keep <= 0 {
		return summary, fmt.Errorf("portal release: retain keep must be positive")
	}
	if maxBytes <= 0 {
		return summary, fmt.Errorf("portal release: retain max bytes must be positive")
	}
	unlock, err := lockRelease(root)
	if err != nil {
		return summary, err
	}
	defer unlock()
	state, ok := LoadReleaseState(root)
	if !ok {
		return summary, ErrReleaseState
	}
	var required []string
	current, exists, err := readCurrent(root)
	if err != nil {
		return summary, err
	}
	if exists {
		release, ok := findRelease(state, current)
		if !ok || release.Withdrawn {
			return summary, ErrReleaseWithdrawn
		}
		required = append(required, current)
	}
	return rebuildRetained(root, state, required, keep, maxBytes)
}

// rebuildRetained materializes a complete retained store and switches it
// into place only after every required asset has been verified and linked.
// The caller holds the release lock.
func rebuildRetained(root string, state ReleaseState, required []string, keep int, maxBytes int64) (RetainedSummary, error) {
	var summary RetainedSummary
	if keep == 0 {
		keep = defaultRetainKeep
	}
	if maxBytes == 0 {
		maxBytes = defaultRetainMaxBytes
	}
	if keep <= 0 {
		return summary, fmt.Errorf("portal release: retain keep must be positive")
	}
	if maxBytes <= 0 {
		return summary, fmt.Errorf("portal release: retain max bytes must be positive")
	}
	type candidate struct {
		content   string
		promoted  string
		mandatory bool
	}
	seen := map[string]bool{}
	candidates := make([]candidate, 0, len(state.Releases)+len(required))
	for _, content := range required {
		if seen[content] {
			continue
		}
		seen[content] = true
		candidates = append(candidates, candidate{content: content, mandatory: true})
	}
	optional := make([]candidate, 0, len(state.Releases))
	for _, release := range state.Releases {
		if release.Withdrawn || seen[release.ContentSHA256] {
			continue
		}
		seen[release.ContentSHA256] = true
		optional = append(optional, candidate{content: release.ContentSHA256, promoted: release.PromotedAt})
	}
	sort.SliceStable(optional, func(i, j int) bool { return optional[i].promoted > optional[j].promoted })
	candidates = append(candidates, optional...)

	tmp, err := os.MkdirTemp(root, ".retained-*.tmp")
	if err != nil {
		return summary, err
	}
	keepTmp := true
	defer func() {
		if keepTmp {
			_ = os.RemoveAll(tmp)
		}
	}()
	assetsDir := filepath.Join(tmp, "assets")
	if err := os.MkdirAll(assetsDir, 0o700); err != nil {
		return summary, err
	}
	linked := map[string]FileDigest{}
	type chosenAsset struct {
		digest FileDigest
		source string
	}
	var chosen []chosenAsset
	var total int64
	included := 0
	for _, cand := range candidates {
		if !cand.mandatory && included >= keep {
			break
		}
		releaseDir := releasePath(root, cand.content)
		assets, err := verifiedReleaseAssets(releaseDir)
		if err != nil {
			if cand.mandatory {
				return summary, fmt.Errorf("portal release: required release assets unavailable: %w", err)
			}
			continue
		}
		var incremental int64
		conflict := ""
		for _, asset := range assets {
			if existing, ok := linked[asset.Path]; ok {
				if existing.SHA256 != asset.SHA256 || existing.Bytes != asset.Bytes {
					conflict = asset.Path
					break
				}
				continue
			}
			incremental += asset.Bytes
		}
		if conflict != "" {
			return summary, fmt.Errorf("portal release: retained asset path has conflicting content: %s", conflict)
		}
		if total+incremental > maxBytes {
			if cand.mandatory {
				return summary, fmt.Errorf("portal release: required release exceeds the retention budget")
			}
			continue
		}
		for _, asset := range assets {
			if _, ok := linked[asset.Path]; ok {
				continue
			}
			linked[asset.Path] = asset
			chosen = append(chosen, chosenAsset{
				digest: asset,
				source: filepath.Join(releaseDir, filepath.FromSlash(asset.Path)),
			})
		}
		total += incremental
		included++
	}
	for _, asset := range chosen {
		dst := filepath.Join(tmp, filepath.FromSlash(asset.digest.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return summary, err
		}
		// Remove-then-link keeps reruns idempotent across retries.
		_ = os.Remove(dst)
		if err := os.Link(asset.source, dst); err != nil {
			return summary, fmt.Errorf("portal release: retained asset link %s: %w", asset.digest.Path, err)
		}
		if err := os.Chmod(dst, 0o600); err != nil {
			return summary, err
		}
	}
	retained := filepath.Join(root, RetainedDir)
	_ = os.RemoveAll(retained + ".prev")
	if _, err := os.Stat(retained); err == nil {
		if err := os.Rename(retained, retained+".prev"); err != nil {
			return summary, err
		}
	}
	if err := os.Rename(tmp, retained); err != nil {
		if _, serr := os.Stat(retained + ".prev"); serr == nil {
			_ = os.Rename(retained+".prev", retained)
		}
		return summary, err
	}
	_ = os.RemoveAll(retained + ".prev")
	keepTmp = false
	summary.Releases = included
	summary.Files = len(chosen)
	summary.Bytes = total
	return summary, nil
}

// verifiedReleaseAssets returns the manifest's safe asset paths after
// verifying their bytes. Only regular files under assets/ are eligible for
// hardlinking; traversal, absolute or symlinked paths fail closed.
func verifiedReleaseAssets(releaseDir string) ([]FileDigest, error) {
	manifest, err := readPortalManifest(releaseDir)
	if err != nil {
		return nil, err
	}
	assets := make([]FileDigest, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		if !strings.HasPrefix(file.Path, "assets/") {
			continue
		}
		clean, ok := safeRetainedAssetPath(file.Path)
		if !ok {
			return nil, fmt.Errorf("portal release: unsafe retained asset path %q", file.Path)
		}
		source := filepath.Join(releaseDir, filepath.FromSlash(clean))
		info, err := os.Lstat(source)
		if err != nil {
			return nil, fmt.Errorf("portal release: retained asset unreadable %s: %w", file.Path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("portal release: retained asset is not a regular file %s", file.Path)
		}
		body, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("portal release: retained asset unreadable %s: %w", file.Path, err)
		}
		sum := sha256.Sum256(body)
		if "sha256:"+hex.EncodeToString(sum[:]) != file.SHA256 || int64(len(body)) != file.Bytes {
			return nil, fmt.Errorf("portal release: retained asset failed verification %s", file.Path)
		}
		file.Path = clean
		assets = append(assets, file)
	}
	return assets, nil
}

// safeRetainedAssetPath accepts only clean, relative, slash-separated asset
// paths. The store and the edge both address the returned value.
// verifyRetainedCurrent proves every asset referenced by the current page
// set is present in the retained store before the edge can serve it. Bytes
// are verified during promotion; this is the cheap restart/tamper check.
func verifyRetainedCurrent(root, content string) error {
	assets, err := verifiedReleaseAssets(releasePath(root, content))
	if err != nil {
		return fmt.Errorf("portal release: current assets unavailable: %w", err)
	}
	for _, asset := range assets {
		info, err := os.Lstat(filepath.Join(root, RetainedDir, filepath.FromSlash(asset.Path)))
		if err != nil {
			return fmt.Errorf("portal release: current asset not retained %s: %w", asset.Path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("portal release: current asset not retained %s", asset.Path)
		}
	}
	return nil
}

func safeRetainedAssetPath(value string) (string, bool) {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return "", false
	}
	clean := path.Clean(value)
	if clean != value || clean == "assets" || !strings.HasPrefix(clean, "assets/") {
		return "", false
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	return clean, true
}
