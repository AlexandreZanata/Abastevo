package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const (
	RefreshKind         = "enterprise-portal-refresh"
	RefreshStateSchema  = "enterprise-portal-refresh-state-v1"
	refreshLockFile     = ".portal-refresh.lock"
	refreshStateFile    = "portal-refresh-state.json"
	refreshCandidates   = "candidates"
	refreshCandidateAge = 24 * time.Hour
)

var ErrRefreshBusy = errors.New("enterprise portal refresh: another refresh holds the lock")

type RefreshPayload struct {
	Version             int    `json:"version"`
	Trigger             string `json:"trigger,omitempty"`
	ObservedAt          string `json:"observed_at,omitempty"`
	ExportContentSHA256 string `json:"export_content_sha256,omitempty"`
}

type SourceStatus struct {
	RevisionID       string
	LeaseID          string
	Edition          string
	SourcePeriod     string
	SourceChecksum   string
	SourceClockValue string
	PublishedAt      string
	Status           string
	HasPublished     bool
}

type RefreshState struct {
	SchemaVersion            string `json:"schema_version"`
	LastAttemptAt            string `json:"last_attempt_at"`
	LastOutcome              string `json:"last_outcome"`
	LastError                string `json:"last_error,omitempty"`
	Trigger                  string `json:"trigger,omitempty"`
	ObservedAt               string `json:"observed_at,omitempty"`
	ExportBuiltAt            string `json:"export_built_at,omitempty"`
	PortalBuiltAt            string `json:"portal_built_at,omitempty"`
	ReleasedAt               string `json:"released_at,omitempty"`
	WithdrawnAt              string `json:"withdrawn_at,omitempty"`
	WithdrawnRevisionID      string `json:"withdrawn_revision_id,omitempty"`
	WithdrawnReason          string `json:"withdrawn_reason,omitempty"`
	ExportContentSHA256      string `json:"export_content_sha256,omitempty"`
	PortalContentSHA256      string `json:"portal_content_sha256,omitempty"`
	SourceRevisionID         string `json:"source_revision_id,omitempty"`
	SourceLeaseID            string `json:"source_lease_id,omitempty"`
	SourceEdition            string `json:"source_edition,omitempty"`
	SourceChecksum           string `json:"source_checksum,omitempty"`
	SourceClockValue         string `json:"source_clock_value,omitempty"`
	GenerationSeconds        int64  `json:"generation_seconds,omitempty"`
	ExportToReleaseSeconds   int64  `json:"export_to_release_seconds,omitempty"`
	ObservedToReleaseSeconds int64  `json:"observed_to_release_seconds,omitempty"`
	// FailedBudget is the budget key of a deterministic budget failure for
	// ExportContentSHA256; identical inputs are not regenerated.
	FailedBudget string `json:"failed_budget,omitempty"`
}

// Refresh turns the latest durable export candidate into a verified portal
// release. It coalesces unchanged export/source pins, preserves the previous
// release on any failure, and withdraws a release whose official revision is no
// longer published.
type Refresh struct {
	ExportRoot       string
	PortalRoot       string
	SiteURL          string
	SearchScriptPath string
	// LandingURL is the optional landing origin for the shared portal
	// header/footer; empty keeps the portal-only chrome.
	LandingURL string
	Indexable  bool
	// OfficialOnly restricts the pipeline to the admitted official-records
	// slice: generation, verification, promotion, rollback and withdrawal
	// all run in official-only mode, and withdrawal never falls back to a
	// full release. An official-only refresh can never emit the full portal
	// or widen the source scope.
	OfficialOnly bool
	MaxPages     int
	MaxBytes     int64
	Now          func() time.Time
	Status       func(context.Context) (SourceStatus, error)
	Generate     func(portalgen.Options) (portalgen.Result, error)
	Promote      func(portalgen.PromoteOptions) (portalgen.ReleaseState, error)
	Current      func(root, siteURL string, officialOnly bool) (portalgen.Release, portalgen.ReleaseState, error)
	Withdraw     func(portalgen.WithdrawOptions) (portalgen.ReleaseState, error)
}

func (Refresh) Kind() string { return RefreshKind }
func (Refresh) Version() int { return 1 }

func (r Refresh) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r Refresh) Handle(ctx context.Context, job platformjobs.Job) error {
	if r.ExportRoot == "" || r.PortalRoot == "" || r.SiteURL == "" {
		return fmt.Errorf("enterprise portal refresh: export root, portal root and site URL are required")
	}
	if r.ExportRoot == r.PortalRoot {
		return fmt.Errorf("enterprise portal refresh: export and portal roots must differ")
	}
	var payload RefreshPayload
	if len(job.Payload) > 0 {
		if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.Version != 1 {
			return fmt.Errorf("enterprise portal refresh: bad payload")
		}
		if payload.ObservedAt != "" {
			if _, err := time.Parse(time.RFC3339, payload.ObservedAt); err != nil {
				return fmt.Errorf("enterprise portal refresh: bad observed_at")
			}
		}
	}
	if err := ensureRefreshRoot(r.PortalRoot); err != nil {
		return err
	}
	unlock, err := lockRefresh(r.PortalRoot)
	if err != nil {
		return err
	}
	defer unlock()
	now := r.now()
	refresh, _ := loadRefreshState(r.PortalRoot)
	state, ok := LoadState(r.ExportRoot)
	if !ok || !state.Valid() {
		return r.recordNoop(refresh, now, payload, State{}, portalgen.Release{}, "awaiting-export")
	}
	current, releases, err := r.current()
	if errors.Is(err, portalgen.ErrNoCurrentRelease) {
		current = portalgen.Release{}
		if loaded, ok := portalgen.LoadReleaseState(r.PortalRoot); ok {
			releases = loaded
		}
	} else if err != nil {
		return r.fail(refresh, now, payload, state, err)
	}
	status := SourceStatus{HasPublished: true}
	if r.Status != nil {
		status, err = r.Status(ctx)
		if err != nil {
			return r.fail(refresh, now, payload, state, err)
		}
	}
	if !status.HasPublished {
		if current.ContentSHA256 != "" && current.OfficialRevisionID != "" {
			return r.withdraw(refresh, now, payload, state, current, releases, status)
		}
		if state.Official != nil {
			return r.recordNoop(refresh, now, payload, state, current, "source-unavailable")
		}
		// A price-only export remains publishable while the official source is
		// absent; no official claim is added.
	} else if r.Status != nil {
		if state.Official == nil || state.Official.RevisionID != status.RevisionID {
			return r.recordNoop(refresh, now, payload, state, current, "awaiting-export")
		}
	}
	if r.OfficialOnly && state.Official == nil {
		return r.fail(refresh, now, payload, state, fmt.Errorf("enterprise portal refresh: official-only refresh requires a published official revision"))
	}
	if (refresh.LastOutcome == "released" || refresh.LastOutcome == "noop") &&
		state.ContentSHA256 == refresh.ExportContentSHA256 &&
		sameRefreshPin(refresh, state.Official) &&
		current.ContentSHA256 != "" && current.ContentSHA256 == refresh.PortalContentSHA256 {
		return r.recordNoop(refresh, now, payload, state, current, "noop")
	}
	if refresh.LastOutcome == "failed" && refresh.FailedBudget != "" &&
		refresh.FailedBudget == r.budgetKey() &&
		state.ContentSHA256 == refresh.ExportContentSHA256 && sameRefreshPin(refresh, state.Official) {
		// Same snapshot, same budgets: regenerating would fail identically.
		return r.fail(refresh, now, payload, state, &portalgen.BudgetError{Msg: refresh.LastError})
	}
	candidate, err := refreshCandidateDir(r.PortalRoot, now)
	if err != nil {
		return r.fail(refresh, now, payload, state, err)
	}
	if err := cleanRefreshCandidates(r.PortalRoot, now.Add(-refreshCandidateAge)); err != nil {
		return r.fail(refresh, now, payload, state, err)
	}
	started := r.now()
	generated, err := r.generate(portalgen.Options{
		SnapshotDir: state.CandidateDir, OutputDir: candidate,
		Indexable: r.Indexable, SiteURL: r.SiteURL, SearchScriptPath: r.SearchScriptPath,
		LandingURL: r.LandingURL, MaxPages: r.MaxPages, MaxBytes: r.MaxBytes, OfficialOnly: r.OfficialOnly,
	})
	if err != nil {
		_ = os.RemoveAll(candidate)
		return r.fail(refresh, now, payload, state, err)
	}
	portalBuilt := r.now()
	current, _, err = r.current()
	if errors.Is(err, portalgen.ErrNoCurrentRelease) {
		current = portalgen.Release{}
	} else if err != nil {
		_ = os.RemoveAll(candidate)
		return r.fail(refresh, now, payload, state, err)
	}
	if current.ContentSHA256 == generated.ContentSHA256 {
		_ = os.RemoveAll(candidate)
		trigger, observed := refreshObservation(payload, state, now)
		refresh.SchemaVersion = RefreshStateSchema
		refresh.LastAttemptAt = now.UTC().Format(time.RFC3339)
		refresh.LastOutcome = "noop"
		refresh.LastError = ""
		refresh.Trigger = trigger
		refresh.ObservedAt = observed
		refresh.ExportBuiltAt = state.BuiltAt
		refresh.PortalBuiltAt = portalBuilt.UTC().Format(time.RFC3339)
		refresh.ExportContentSHA256 = state.ContentSHA256
		refresh.PortalContentSHA256 = current.ContentSHA256
		refresh.ReleasedAt = current.PromotedAt
		refresh.GenerationSeconds = secondsBetweenTimes(started, portalBuilt)
		fillRefreshSource(&refresh, state.Official, status)
		return writeRefreshState(r.PortalRoot, refresh)
	}
	promoted, err := r.promote(portalgen.PromoteOptions{
		ArtifactDir: candidate, RootDir: r.PortalRoot, SiteURL: r.SiteURL,
		Fence: true, ExpectedCurrent: current.ContentSHA256, OfficialOnly: r.OfficialOnly,
	})
	if err != nil {
		_ = os.RemoveAll(candidate)
		return r.fail(refresh, now, payload, state, err)
	}
	_ = os.RemoveAll(candidate)
	if promoted.Current != generated.ContentSHA256 {
		return r.fail(refresh, now, payload, state, fmt.Errorf("enterprise portal refresh: promoted release mismatch"))
	}
	released := r.now()
	trigger, observed := refreshObservation(payload, state, now)
	refresh.SchemaVersion = RefreshStateSchema
	refresh.LastAttemptAt = now.UTC().Format(time.RFC3339)
	refresh.LastOutcome = "released"
	refresh.LastError = ""
	refresh.Trigger = trigger
	refresh.ObservedAt = observed
	refresh.ExportBuiltAt = state.BuiltAt
	refresh.PortalBuiltAt = portalBuilt.UTC().Format(time.RFC3339)
	refresh.ReleasedAt = released.UTC().Format(time.RFC3339)
	refresh.WithdrawnAt = ""
	refresh.WithdrawnRevisionID = ""
	refresh.WithdrawnReason = ""
	refresh.ExportContentSHA256 = state.ContentSHA256
	refresh.PortalContentSHA256 = generated.ContentSHA256
	refresh.GenerationSeconds = secondsBetweenTimes(started, portalBuilt)
	refresh.ExportToReleaseSeconds = secondsBetween(state.BuiltAt, released)
	refresh.ObservedToReleaseSeconds = secondsBetween(observed, released)
	fillRefreshSource(&refresh, state.Official, status)
	return writeRefreshState(r.PortalRoot, refresh)
}

func (r Refresh) current() (portalgen.Release, portalgen.ReleaseState, error) {
	var current portalgen.Release
	var releases portalgen.ReleaseState
	var err error
	if r.Current != nil {
		current, releases, err = r.Current(r.PortalRoot, r.SiteURL, r.OfficialOnly)
	} else if r.OfficialOnly {
		current, releases, err = portalgen.CurrentRelease(r.PortalRoot, r.SiteURL, true)
	} else {
		current, releases, err = portalgen.CurrentReleaseHeader(r.PortalRoot)
	}
	if errors.Is(err, portalgen.ErrReleaseState) {
		if _, ok := portalgen.LoadReleaseState(r.PortalRoot); !ok {
			return portalgen.Release{}, portalgen.ReleaseState{}, nil
		}
	}
	return current, releases, err
}

func (r Refresh) generate(opts portalgen.Options) (portalgen.Result, error) {
	if r.Generate != nil {
		return r.Generate(opts)
	}
	return portalgen.Generate(opts)
}

func (r Refresh) promote(opts portalgen.PromoteOptions) (portalgen.ReleaseState, error) {
	if r.Promote != nil {
		return r.Promote(opts)
	}
	return portalgen.Promote(opts)
}

func (r Refresh) withdrawRelease(opts portalgen.WithdrawOptions) (portalgen.ReleaseState, error) {
	if r.Withdraw != nil {
		return r.Withdraw(opts)
	}
	return portalgen.Withdraw(opts)
}

func (r Refresh) withdraw(refresh RefreshState, now time.Time, payload RefreshPayload, state State, current portalgen.Release, releases portalgen.ReleaseState, status SourceStatus) error {
	// The withdrawal reason is classified from the authoritative source
	// state, never inferred from the outage itself: only an explicit
	// source-side "withdrawn" marker revokes the official identity. A
	// temporary outage or operational pause records no revocation, so the
	// same content stays eligible for a future release.
	reason := ""
	if status.Status == "withdrawn" {
		reason = "revoked-source"
	}
	// An official-only pipeline must never fall back to a full release:
	// withdrawal removes the pointer (404) instead of serving wider scope.
	fallback := ""
	if !r.OfficialOnly {
		fallback = safeRefreshFallback(releases, current)
	}
	if _, err := r.withdrawRelease(portalgen.WithdrawOptions{
		RootDir: r.PortalRoot, Fallback: fallback, SiteURL: r.SiteURL,
		OfficialOnly: r.OfficialOnly, Reason: reason,
	}); err != nil {
		return r.fail(refresh, now, payload, state, err)
	}
	trigger, observed := refreshObservation(payload, state, now)
	refresh.SchemaVersion = RefreshStateSchema
	refresh.LastAttemptAt = now.UTC().Format(time.RFC3339)
	refresh.LastOutcome = "withdrawn"
	refresh.LastError = ""
	refresh.Trigger = trigger
	refresh.ObservedAt = observed
	refresh.ExportBuiltAt = state.BuiltAt
	refresh.WithdrawnAt = now.UTC().Format(time.RFC3339)
	refresh.WithdrawnRevisionID = current.OfficialRevisionID
	if reason != "" {
		refresh.WithdrawnReason = reason
	}
	refresh.PortalContentSHA256 = current.ContentSHA256
	refresh.ReleasedAt = current.PromotedAt
	refresh.ExportContentSHA256 = state.ContentSHA256
	fillRefreshSource(&refresh, state.Official, SourceStatus{RevisionID: current.OfficialRevisionID, LeaseID: current.OfficialLeaseID, Edition: current.OfficialEdition, SourceChecksum: current.OfficialSourceChecksum})
	return writeRefreshState(r.PortalRoot, refresh)
}

func (r Refresh) budgetKey() string {
	return portalgen.BudgetKey(r.MaxPages, r.MaxBytes)
}

func (r Refresh) fail(refresh RefreshState, now time.Time, payload RefreshPayload, state State, cause error) error {
	trigger, observed := refreshObservation(payload, state, now)
	refresh.SchemaVersion = RefreshStateSchema
	refresh.LastAttemptAt = now.UTC().Format(time.RFC3339)
	refresh.LastOutcome = "failed"
	refresh.LastError = truncateRefreshError(cause.Error())
	refresh.FailedBudget = ""
	var budget *portalgen.BudgetError
	if errors.As(cause, &budget) {
		refresh.FailedBudget = r.budgetKey()
	}
	refresh.Trigger = trigger
	refresh.ObservedAt = observed
	if state.SchemaVersion != "" {
		refresh.ExportBuiltAt = state.BuiltAt
		refresh.ExportContentSHA256 = state.ContentSHA256
		fillRefreshSource(&refresh, state.Official, SourceStatus{})
	}
	if err := writeRefreshState(r.PortalRoot, refresh); err != nil {
		return fmt.Errorf("%w (receipt: %v)", cause, err)
	}
	return cause
}

func (r Refresh) recordNoop(refresh RefreshState, now time.Time, payload RefreshPayload, state State, current portalgen.Release, outcome string) error {
	trigger, observed := refreshObservation(payload, state, now)
	refresh.SchemaVersion = RefreshStateSchema
	refresh.LastAttemptAt = now.UTC().Format(time.RFC3339)
	refresh.LastOutcome = outcome
	refresh.LastError = ""
	refresh.Trigger = trigger
	refresh.ObservedAt = observed
	if state.SchemaVersion != "" {
		refresh.ExportBuiltAt = state.BuiltAt
		refresh.ExportContentSHA256 = state.ContentSHA256
		fillRefreshSource(&refresh, state.Official, SourceStatus{})
	}
	if current.ContentSHA256 != "" {
		refresh.PortalContentSHA256 = current.ContentSHA256
		refresh.ReleasedAt = current.PromotedAt
	}
	return writeRefreshState(r.PortalRoot, refresh)
}

func refreshObservation(payload RefreshPayload, state State, now time.Time) (string, string) {
	trigger := payload.Trigger
	if trigger == "" {
		trigger = "reconcile"
	}
	observed := payload.ObservedAt
	if observed == "" {
		observed = state.ObservedAt
	}
	if observed == "" {
		observed = state.BuiltAt
	}
	if _, err := time.Parse(time.RFC3339, observed); err != nil {
		observed = now.UTC().Format(time.RFC3339)
	}
	return trigger, observed
}

func fillRefreshSource(refresh *RefreshState, official *OfficialPin, status SourceStatus) {
	if official != nil {
		refresh.SourceRevisionID = official.RevisionID
		refresh.SourceLeaseID = official.LeaseID
		refresh.SourceEdition = official.Edition
		refresh.SourceChecksum = official.SourceChecksum
		refresh.SourceClockValue = official.SourceClockValue
		return
	}
	refresh.SourceRevisionID = status.RevisionID
	refresh.SourceLeaseID = status.LeaseID
	refresh.SourceEdition = status.Edition
	refresh.SourceChecksum = status.SourceChecksum
	refresh.SourceClockValue = status.SourceClockValue
}

func sameRefreshPin(refresh RefreshState, official *OfficialPin) bool {
	if official == nil {
		return refresh.SourceRevisionID == "" && refresh.SourceLeaseID == "" && refresh.SourceChecksum == ""
	}
	return refresh.SourceRevisionID == official.RevisionID &&
		refresh.SourceLeaseID == official.LeaseID &&
		refresh.SourceEdition == official.Edition &&
		refresh.SourceChecksum == official.SourceChecksum
}

func safeRefreshFallback(state portalgen.ReleaseState, current portalgen.Release) string {
	for i := len(state.Releases) - 1; i >= 0; i-- {
		release := state.Releases[i]
		if release.ContentSHA256 == current.ContentSHA256 || release.Withdrawn || release.OfficialRevisionID != "" {
			continue
		}
		return release.ContentSHA256
	}
	return ""
}

func loadRefreshState(root string) (RefreshState, bool) {
	raw, err := os.ReadFile(filepath.Join(root, refreshStateFile))
	if err != nil {
		return RefreshState{}, false
	}
	var state RefreshState
	if err := json.Unmarshal(raw, &state); err != nil || state.SchemaVersion != RefreshStateSchema {
		return RefreshState{}, false
	}
	return state, true
}

func writeRefreshState(root string, state RefreshState) error {
	if state.SchemaVersion != RefreshStateSchema {
		return fmt.Errorf("enterprise portal refresh: bad state schema")
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(root, ".refresh-state-*.tmp")
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
	if err := os.Rename(name, filepath.Join(root, refreshStateFile)); err != nil {
		return err
	}
	ok = true
	return syncRefreshDir(root)
}

func ensureRefreshRoot(root string) error {
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("enterprise portal refresh: root must not be a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, refreshCandidates), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return err
	}
	return os.Chmod(filepath.Join(root, refreshCandidates), 0o700)
}

func lockRefresh(root string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(root, refreshLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrRefreshBusy
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func refreshCandidateDir(root string, now time.Time) (string, error) {
	dir := filepath.Join(root, refreshCandidates)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return filepath.Join(dir, "portal-"+now.UTC().Format("20060102T150405Z")+"-"+hex.EncodeToString(random[:])), nil
}

func cleanRefreshCandidates(root string, cutoff time.Time) error {
	entries, err := os.ReadDir(filepath.Join(root, refreshCandidates))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "portal-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, refreshCandidates, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func secondsBetween(start string, end time.Time) int64 {
	at, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return 0
	}
	return secondsBetweenTimes(at, end)
}

func secondsBetweenTimes(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return int64(end.Sub(start).Round(time.Second) / time.Second)
}

func truncateRefreshError(value string) string {
	if len(value) > 512 {
		return value[:512]
	}
	return value
}

func syncRefreshDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
