// Package jobs runs verified EV ingestion with leases, cleanup and
// budgets. Imports and schedules default off; every run retains the last
// accepted public state on failure. No auto-close, auto-bind or live
// source activation occurs here.
package jobs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	// ChargingImportKind ingests one prepared batch (charging or vehicle).
	ChargingImportKind = "charging-import"
	// ChargingDiscoveryKind checks allowlisted sources for new revisions.
	ChargingDiscoveryKind = "charging-discovery"
	// MaxAttempts bounds job retries; dead budgets refuse without partial state.
	MaxAttempts = 3
	// JobTimeout bounds one import run; preparer, bridge and store share it.
	JobTimeout = 10 * time.Minute
)

func importError(code string) error {
	return errors.New("charging import: " + code)
}

// ImportPayload selects one snapshot input. Paths are workspace-local;
// URLs are never accepted here (no SSRF surface).
type ImportPayload struct {
	Version int    `json:"version"`
	Profile string `json:"profile"`
	// SnapshotDir holds admitted source files (OCM POIs or PBE text).
	SnapshotDir string `json:"snapshot_dir"`
	// Format is charging-prepared-v1 or vehicle-energy-reference-v1.
	Format string `json:"format"`
}

// ImportResult reports one verified ingestion with its batch identity
// and source coverage for review. Merge/split flags never auto-link.
type ImportResult struct {
	BatchID       string
	Entities      int
	NewPlaces     []string
	MissingPlaces []string
	ChangedPlaces []string
}

// Runner executes one import: preparer binary, bridge load and atomic
// store, with workspace isolation and cleanup-before-completion. The
// preparer and output paths stay private; raw snapshots are never
// published or retained past the run.
type Runner struct {
	// PrepareCharging and PrepareVehicle are trusted binary paths.
	PrepareCharging string
	PrepareVehicle  string
	// Store persists a verified batch; injected for tests.
	Store func(ctx context.Context, format, profile, dir string) (ImportResult, error)
}

// ReviewFlags compares incoming place external IDs with stored ones and
// reports merges, splits and changes for human review. It never auto-links,
// auto-closes or auto-binds: same ID with different content flags changed,
// missing IDs flag withdrawn, new IDs flag added. All flags require review.
type ReviewFlags struct {
	Added   []string
	Missing []string
	Changed []string
}

// ReviewMergeSplit compares stored place IDs (with content hashes) against
// incoming ones. Identical IDs with different hashes are changed (possible
// merge/split or correction); absent IDs are missing; new IDs are added.
func ReviewMergeSplit(stored map[string]string, incoming map[string]string) ReviewFlags {
	var flags ReviewFlags
	for id, incomingHash := range incoming {
		if storedHash, ok := stored[id]; !ok {
			flags.Added = append(flags.Added, id)
		} else if storedHash != incomingHash {
			flags.Changed = append(flags.Changed, id)
		}
	}
	for id := range stored {
		if _, ok := incoming[id]; !ok {
			flags.Missing = append(flags.Missing, id)
		}
	}
	return flags
}

// Run executes the import with a dead budget. Cleanup removes the
// workspace even on failure; a failed run retains prior public state
// by writing nothing durable outside the workspace.
func (r Runner) Run(ctx context.Context, payload ImportPayload) (result ImportResult, err error) {
	ctx, cancel := context.WithTimeout(ctx, JobTimeout)
	defer cancel()
	if payload.Version != 1 || payload.SnapshotDir == "" || payload.SnapshotDir != filepath.Clean(payload.SnapshotDir) {
		return result, importError("invalid payload")
	}
	if payload.Format != "charging-prepared-v1" && payload.Format != "vehicle-energy-reference-v1" {
		return result, importError("unknown format")
	}
	binary := r.PrepareCharging
	if payload.Format == "vehicle-energy-reference-v1" {
		binary = r.PrepareVehicle
	}
	if binary == "" {
		return result, importError("preparer unavailable")
	}
	workspace, err := os.MkdirTemp("", "charging-import-")
	if err != nil {
		return result, importError("workspace unavailable")
	}
	closed := false
	defer func() {
		if !closed {
			if removeErr := os.RemoveAll(workspace); removeErr != nil && err == nil {
				err = importError("workspace cleanup unavailable")
			}
		}
	}()
	out := filepath.Join(workspace, "prepared")
	prepareCtx, prepareCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer prepareCancel()
	cmd := exec.CommandContext(prepareCtx, binary, payload.SnapshotDir, out)
	cmd.Env = []string{}
	if out, err := cmd.CombinedOutput(); err != nil {
		return result, importError("preparer refused: " + string(out))
	}
	if r.Store == nil {
		return result, importError("store unavailable")
	}
	result, err = r.Store(ctx, payload.Format, payload.Profile, out)
	if err != nil {
		return result, err
	}
	closed = true
	if err := os.RemoveAll(workspace); err != nil {
		return result, importError("workspace cleanup unavailable")
	}
	if _, err := os.Lstat(workspace); !os.IsNotExist(err) {
		return result, importError("workspace retained")
	}
	return result, nil
}
