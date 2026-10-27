// Package portalexport builds a static portal candidate from the published
// FQ-07 official lease through a read-only repeatable-read transaction. It
// never writes canonical data, never fetches sources and never publishes:
// promotion stays with portalrelease. It replaces one-off staging helpers
// with a maintained, tested command reusing ExportPin, BuildOfficialExport
// and the portalgen verifier.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	enterpriseexport "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	enterprisejobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config selects one official-only (default) or full portal build. OfficialOnly
// emits only the admitted sanctions slice: no price, station, search, insight
// or download routes, and linked records render without station links.
type Config struct {
	DSN          string
	SnapshotDir  string
	PortalOut    string
	OfficialOnly bool
	Indexable    bool
	SiteURL      string
}

// Result reports the frozen official pin and the verified candidate identity.
// No row data is returned: counts only.
type Result struct {
	Edition       string
	Rows          int
	Linked        int
	Unmatched     int
	Ambiguous     int
	Pages         int
	Files         int
	Bytes         int64
	ContentSHA256 string
}

func (c Config) validate() error {
	if c.DSN == "" {
		return fmt.Errorf("portalexport: database DSN is required")
	}
	if c.SnapshotDir == "" || c.PortalOut == "" {
		return fmt.Errorf("portalexport: snapshot and portal output directories are required")
	}
	if c.Indexable && c.SiteURL == "" {
		return fmt.Errorf("portalexport: indexable output requires a site URL")
	}
	return nil
}

// RunExport builds and verifies one portal candidate from the current
// published official lease. The database transaction is read-only and
// repeatable-read; a concurrent publication fails closed instead of mixing
// revisions.
func RunExport(ctx context.Context, cfg Config) (Result, error) {
	if err := cfg.validate(); err != nil {
		return Result{}, err
	}
	pool, err := database.Open(ctx, cfg.DSN, database.DefaultOptions())
	if err != nil {
		return Result{}, err
	}
	defer pool.Close()
	return runExportTx(ctx, pool.Underlying(), cfg)
}

func runExportTx(ctx context.Context, pool *pgxpool.Pool, cfg Config) (Result, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	provider := enterprisejobs.NewQualityOfficialProvider(qualityread.NewExportReader(tx))
	snapshot, pin, err := writeOfficialSnapshot(ctx, provider, cfg)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	generated, err := portalgen.Generate(portalgen.Options{
		SnapshotDir: snapshot, OutputDir: cfg.PortalOut,
		OfficialOnly: cfg.OfficialOnly, Indexable: cfg.Indexable, SiteURL: cfg.SiteURL,
	})
	if err != nil {
		return Result{}, err
	}
	if _, err := portalgen.Verify(cfg.PortalOut, portalgen.VerifyOptions{SiteURL: cfg.SiteURL, OfficialOnly: cfg.OfficialOnly}); err != nil {
		return Result{}, err
	}
	return Result{
		Edition: pin.Edition, Rows: pin.Rows,
		Linked: pin.Linked, Unmatched: pin.Unmatched, Ambiguous: pin.Ambiguous,
		Pages: generated.Pages, Files: generated.Files, Bytes: generated.Bytes,
		ContentSHA256: generated.ContentSHA256,
	}, nil
}

// writeOfficialSnapshot freezes the current published official lease into a
// price-empty export snapshot: official JSONL/manifest plus the minimal
// manifest, empty JSONL streams and analytics projection portalgen requires.
// The price streams stay empty on purpose: official-only generation never
// reads them, and no price fact enters the candidate.
func writeOfficialSnapshot(ctx context.Context, provider enterprisejobs.OfficialProvider, cfg Config) (string, enterprisejobs.OfficialPin, error) {
	snapshot := cfg.SnapshotDir
	if err := os.MkdirAll(snapshot, 0o700); err != nil {
		return "", enterprisejobs.OfficialPin{}, err
	}
	payloadPath := filepath.Join(snapshot, "official-records.jsonl")
	payloadFile, err := os.OpenFile(payloadPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", enterprisejobs.OfficialPin{}, err
	}
	hasher := sha256.New()
	build, buildErr := provider.Build(ctx, io.MultiWriter(payloadFile, hasher))
	closeErr := payloadFile.Close()
	if buildErr != nil {
		return "", enterprisejobs.OfficialPin{}, buildErr
	}
	if closeErr != nil {
		return "", enterprisejobs.OfficialPin{}, closeErr
	}
	if build.Pin.PayloadSHA256 == "" || build.Pin.PayloadSHA256 != hex.EncodeToString(hasher.Sum(nil)) {
		return "", enterprisejobs.OfficialPin{}, fmt.Errorf("portalexport: official payload hash mismatch")
	}
	if err := os.WriteFile(filepath.Join(snapshot, "official-records-manifest.json"), append(build.Manifest, '\n'), 0o600); err != nil {
		return "", enterprisejobs.OfficialPin{}, err
	}
	for _, name := range []string{"stations.jsonl", "revisions.jsonl", "prices.jsonl"} {
		if err := os.WriteFile(filepath.Join(snapshot, name), nil, 0o600); err != nil {
			return "", enterprisejobs.OfficialPin{}, err
		}
	}
	inputSum := sha256.Sum256([]byte("official-only|" + build.Pin.PayloadSHA256))
	inputHash := "sha256:" + hex.EncodeToString(inputSum[:])
	// The official-only candidate is published as of its lease, so an
	// unchanged lease rebuilds the identical candidate at any time; the wall
	// clock is only a fallback for a manifest without published_at.
	builtAt := time.Now().UTC().Format(time.RFC3339)
	var lease struct {
		PublishedAt string `json:"published_at"`
	}
	if json.Unmarshal(build.Manifest, &lease) == nil {
		if at, err := time.Parse(time.RFC3339, lease.PublishedAt); err == nil {
			builtAt = at.UTC().Format(time.RFC3339)
		}
	}
	exportManifest := enterpriseexport.Manifest{
		SchemaVersion: enterpriseexport.SchemaVersion, ContentVersion: enterpriseexport.ContentVersion,
		PublicFieldPolicy: enterpriseexport.PublicFieldPolicy, BuiltAt: builtAt,
		Scope:  enterpriseexport.Scope{PageSize: 500, MaxStations: 100000, MaxRevisions: 2000, MaxPrices: 5000000},
		Counts: enterpriseexport.Counts{}, ContentSHA256: inputHash,
	}
	if err := writeJSONFile(filepath.Join(snapshot, "manifest.json"), exportManifest); err != nil {
		return "", enterprisejobs.OfficialPin{}, err
	}
	projection, err := analytics.Compute(analytics.Input{ContentSHA256: inputHash}, analytics.Options{})
	if err != nil {
		return "", enterprisejobs.OfficialPin{}, err
	}
	if err := writeJSONFile(filepath.Join(snapshot, "analytics-v1.json"), projection); err != nil {
		return "", enterprisejobs.OfficialPin{}, err
	}
	return snapshot, build.Pin, nil
}

func writeJSONFile(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}
