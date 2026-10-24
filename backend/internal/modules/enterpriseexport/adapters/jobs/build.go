package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const (
	BuildKind     = "enterprise-export-build"
	ReconcileKind = "enterprise-export-reconcile"

	defaultPageSize     = 1000
	defaultMaxStations  = 100_000
	defaultMaxRevisions = 500
	defaultMaxPrices    = 500_000
	defaultWindowWeeks  = 5
	maxWindowWeeks      = 12
	staleCandidateAge   = 24 * time.Hour
)

type BuildPayload struct {
	Version    int    `json:"version"`
	Trigger    string `json:"trigger,omitempty"`
	RevisionID string `json:"revision_id,omitempty"`
}

// Build is the bounded, single-active export builder. It owns a private
// workspace root, a file lock, a durable state file and a candidate directory;
// it never writes canonical database data.
type Build struct {
	Pool             *pgxpool.Pool
	Root             string
	WindowWeeks      int
	PageSize         int
	MaxStations      int
	MaxRevisions     int
	MaxPrices        int
	AnalyticsOptions analytics.Options
	Now              func() time.Time
	OfficialRecords  func(tx pgx.Tx) OfficialProvider
}

func (Build) Kind() string { return BuildKind }
func (Build) Version() int { return 1 }
func (b Build) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

func (b Build) pageSize() int {
	if b.PageSize <= 0 {
		return defaultPageSize
	}
	if b.PageSize > defaultPageSize {
		return defaultPageSize
	}
	return b.PageSize
}

func (b Build) windowWeeks() int {
	return normalizeWindowWeeks(b.WindowWeeks)
}

func normalizeWindowWeeks(weeks int) int {
	if weeks <= 0 {
		return defaultWindowWeeks
	}
	if weeks > maxWindowWeeks {
		return maxWindowWeeks
	}
	return weeks
}

func (b Build) maxStations() int {
	if b.MaxStations <= 0 {
		return defaultMaxStations
	}
	return b.MaxStations
}

func (b Build) maxRevisions() int {
	if b.MaxRevisions <= 0 {
		return defaultMaxRevisions
	}
	return b.MaxRevisions
}

func (b Build) maxPrices() int {
	if b.MaxPrices <= 0 {
		return defaultMaxPrices
	}
	return b.MaxPrices
}

// Handle resolves the newest official window, coalesces unchanged work through
// the state file, builds one candidate and writes analytics/state. A busy lock
// is a retryable error; a failed/cancelled build leaves the previous state.
func (b Build) Handle(ctx context.Context, job platformjobs.Job) error {
	if len(job.Payload) > 0 {
		var payload BuildPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("enterprise export: bad payload: %w", err)
		}
		if payload.Version != 1 {
			return fmt.Errorf("enterprise export: unsupported payload version")
		}
	}
	if b.Pool == nil || b.Root == "" {
		return fmt.Errorf("enterprise export: pool and root are required")
	}
	release, err := Lock(b.Root)
	if err != nil {
		return err
	}
	defer release()
	now := b.now()
	state, hasState := LoadState(b.Root)
	if err := CleanStale(b.Root, state.CandidateDir, staleCandidateAge, now); err != nil {
		return err
	}

	tx, err := b.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	dirReader := directoryread.NewExportReader(tx)
	officialReader := officialread.NewExportReader(tx)
	latest, err := officialReader.LatestRevision(ctx)
	if errors.Is(err, officialapp.ErrNoPublishedRevision) {
		return nil
	}
	if err != nil {
		return err
	}
	fromSurvey, toSurvey, err := windowFor(latest, b.windowWeeks())
	if err != nil {
		return err
	}
	pins, err := collectPins(ctx, officialReader, fromSurvey, toSurvey, b.pageSize(), b.maxRevisions())
	if err != nil {
		return err
	}
	fingerprint, err := dirReader.Fingerprint(ctx)
	if err != nil {
		return err
	}
	var currentOfficial *OfficialPin
	if b.OfficialRecords != nil {
		currentOfficial, err = b.OfficialRecords(tx).Pin(ctx)
		if err != nil {
			return err
		}
	}
	if hasState && stateMatches(state, fingerprint, pins, fromSurvey, toSurvey, currentOfficial) && state.Valid() {
		return nil
	}

	var snapshotID string
	if err := tx.QueryRow(ctx, "SELECT pg_current_snapshot()::text").Scan(&snapshotID); err != nil {
		return err
	}
	candidate, err := CandidateDir(b.Root, now)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(candidate)
		}
	}()
	builder := enterpriseexport.Builder{
		Stations: dirReader, Prices: officialReader,
		PageSize: b.pageSize(), MaxStations: b.maxStations(),
		MaxRevisions: b.maxRevisions(), MaxPrices: b.maxPrices(),
		Now: b.now,
	}
	manifest, err := builder.Build(ctx, enterpriseexport.Options{
		FromSurvey: fromSurvey, ToSurvey: toSurvey,
		OutputDir: candidate, DatabaseSnapshot: snapshotID,
	})
	if err != nil {
		return err
	}
	snapshot, err := enterpriseexport.ReadSnapshot(candidate)
	if err != nil {
		return err
	}
	projection, err := analytics.Compute(analytics.Input{
		ContentSHA256: manifest.ContentSHA256,
		Stations:      snapshot.Stations,
		Revisions:     snapshot.Revisions,
		Prices:        snapshot.Prices,
	}, b.AnalyticsOptions)
	if err != nil {
		return err
	}
	analyticsJSON, err := json.MarshalIndent(projection, "", "  ")
	if err != nil {
		return err
	}
	analyticsJSON = append(analyticsJSON, '\n')
	if err := os.WriteFile(filepath.Join(candidate, "analytics-v1.json"), analyticsJSON, 0o600); err != nil {
		return err
	}
	analyticsSum := sha256.Sum256(analyticsJSON)
	var officialPin *OfficialPin
	if currentOfficial != nil {
		officialPath := filepath.Join(candidate, "official-records.jsonl")
		officialFile, err := os.OpenFile(officialPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		build, buildErr := b.OfficialRecords(tx).Build(ctx, io.MultiWriter(officialFile, hasher))
		closeErr := officialFile.Close()
		if buildErr != nil {
			return buildErr
		}
		if closeErr != nil {
			return closeErr
		}
		if build.Pin.PayloadSHA256 == "" || "sha256:"+hex.EncodeToString(hasher.Sum(nil)) != "sha256:"+build.Pin.PayloadSHA256 {
			return fmt.Errorf("enterprise export: official payload hash mismatch")
		}
		if err := os.WriteFile(filepath.Join(candidate, "official-records-manifest.json"), build.Manifest, 0o600); err != nil {
			return err
		}
		officialPin = &build.Pin
	}
	next := State{
		SchemaVersion:        StateSchemaVersion,
		BuiltAt:              now.UTC().Format(time.RFC3339),
		CandidateDir:         candidate,
		ContentSHA256:        manifest.ContentSHA256,
		AnalyticsSHA256:      "sha256:" + hex.EncodeToString(analyticsSum[:]),
		AnalyticsBytes:       int64(len(analyticsJSON)),
		DirectoryFingerprint: fingerprint.Hash,
		DirectoryStations:    fingerprint.Stations,
		FromSurvey:           fromSurvey,
		ToSurvey:             toSurvey,
		OfficialRevisions:    pinsFromManifest(manifest),
		Official:             officialPin,
	}
	if err := WriteState(b.Root, next); err != nil {
		return err
	}
	success = true
	return nil
}

func windowFor(latest officialapp.ExportRevision, weeks int) (string, string, error) {
	start, err := time.Parse("2006-01-02", latest.SurveyStart)
	if err != nil {
		return "", "", fmt.Errorf("enterprise export: latest survey start")
	}
	if _, err := time.Parse("2006-01-02", latest.SurveyEnd); err != nil {
		return "", "", fmt.Errorf("enterprise export: latest survey end")
	}
	return start.AddDate(0, 0, -7*(weeks-1)).Format("2006-01-02"), latest.SurveyEnd, nil
}

func collectPins(ctx context.Context, reader officialapp.ExportReader, fromSurvey, toSurvey string, pageSize, max int) ([]RevisionState, error) {
	pins := make([]RevisionState, 0)
	after := ""
	for {
		page, next, err := reader.ExportRevisions(ctx, fromSurvey, toSurvey, after, pageSize)
		if err != nil {
			return nil, err
		}
		for _, revision := range page {
			pins = append(pins, RevisionState{RevisionID: revision.ID, SourceChecksum: revision.SourceChecksum})
			if len(pins) > max {
				return nil, fmt.Errorf("enterprise export: revision budget exceeded")
			}
		}
		if next == "" {
			break
		}
		if next == after {
			return nil, fmt.Errorf("enterprise export: revision cursor did not advance")
		}
		after = next
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].RevisionID < pins[j].RevisionID })
	return pins, nil
}

func pinsFromManifest(manifest enterpriseexport.Manifest) []RevisionState {
	pins := make([]RevisionState, 0, len(manifest.Sources.Official))
	for _, revision := range manifest.Sources.Official {
		pins = append(pins, RevisionState{RevisionID: revision.RevisionID, SourceChecksum: revision.SourceChecksum})
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].RevisionID < pins[j].RevisionID })
	return pins
}

func stateMatches(state State, fingerprint directoryapp.ExportFingerprint, pins []RevisionState, fromSurvey, toSurvey string, official *OfficialPin) bool {
	if state.FromSurvey != fromSurvey || state.ToSurvey != toSurvey {
		return false
	}
	if !sameOfficial(state.Official, official) {
		return false
	}
	if state.DirectoryFingerprint != fingerprint.Hash || state.DirectoryStations != fingerprint.Stations {
		return false
	}
	if len(state.OfficialRevisions) != len(pins) {
		return false
	}
	for i := range pins {
		if state.OfficialRevisions[i] != pins[i] {
			return false
		}
	}
	return true
}
