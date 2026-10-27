// Command station-locate applies a hash-bound national batch of best-tier
// station pins (B-BR-MAP-02) produced by prepare_national_coverage. It never
// creates stations, never guesses a coordinate, never downgrades a projection
// and refuses tiers the manifest's owner decisions do not enable.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	directoryadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const (
	manifestFormat  = "abastevo.station-location-tiers-v1"
	rowSchema       = "station-location-tier-v1"
	maxRows         = 200000
	maxFileBytes    = 64 << 20
	maxLineBytes    = 64 << 10
	maxProviderLen  = 64
	maxReferenceLen = 2000
)

type locationRow struct {
	SchemaVersion   string `json:"schema_version"`
	CNPJ            string `json:"cnpj"`
	Tier            string `json:"tier"`
	Quality         string `json:"quality"`
	LatitudeDeg     string `json:"latitude_deg"`
	LongitudeDeg    string `json:"longitude_deg"`
	Provider        string `json:"provider"`
	SourceReference string `json:"source_reference"`
}

type manifest struct {
	Format         string `json:"format"`
	SchemaVersion  string `json:"schema_version"`
	Generation     string `json:"generation"`
	OwnerDecisions struct {
		D1 bool `json:"D1_anp_reported"`
		D2 bool `json:"D2_address_point"`
	} `json:"owner_decisions"`
	Files map[string]struct {
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "station-locate:", err)
		os.Exit(1)
	}
}

func run() error {
	locationsPath := flag.String("locations", "", "locations.ndjson from prepare_national_coverage")
	manifestPath := flag.String("manifest", "", "locations-manifest.json binding the locations hash")
	reportPath := flag.String("report", "", "optional JSON report path")
	dsn := flag.String("dsn", os.Getenv("ANPFUEL_DATABASE_URL"), "postgres DSN (or ANPFUEL_DATABASE_URL)")
	dryRun := flag.Bool("dry-run", false, "resolve and compare inside a rolled-back transaction")
	timeout := flag.Duration("timeout", 5*time.Minute, "overall deadline")
	flag.Parse()
	if *locationsPath == "" || *manifestPath == "" || *dsn == "" {
		return fmt.Errorf("usage: station-locate --locations FILE --manifest FILE [--dsn URL] [--dry-run] [--report FILE]")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return fmt.Errorf("timeout must be positive and at most 10 minutes")
	}
	raw, m, err := readVerified(*locationsPath, *manifestPath)
	if err != nil {
		return err
	}
	rows, err := parseRows(raw, m)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	config, err := pgxpool.ParseConfig(*dsn)
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	config.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	defer pool.Close()
	started := time.Now()
	result, err := directoryadapters.ApplyTierLocations(ctx, pool, rows, *dryRun)
	if err != nil {
		return err
	}
	report := map[string]any{
		"generation": m.Generation,
		"result":     result,
		"skipped":    result.Skipped,
		"elapsed_ms": time.Since(started).Milliseconds(),
	}
	out, _ := json.Marshal(report)
	if *reportPath != "" {
		if err := os.WriteFile(*reportPath, append(out, '\n'), 0o600); err != nil {
			return fmt.Errorf("report: %w", err)
		}
	}
	fmt.Printf("generation=%s rows=%d outcomes=%v applied=%v dry_run=%t elapsed_ms=%d\n",
		m.Generation, result.Rows, result.Outcomes, result.Applied, result.DryRun, time.Since(started).Milliseconds())
	return nil
}

func readVerified(locationsPath, manifestPath string) ([]byte, manifest, error) {
	var m manifest
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, m, fmt.Errorf("manifest: %w", err)
	}
	if len(rawManifest) > 1<<20 {
		return nil, m, fmt.Errorf("manifest exceeds 1 MiB")
	}
	if err := json.Unmarshal(rawManifest, &m); err != nil {
		return nil, m, fmt.Errorf("manifest: %w", err)
	}
	if m.Format != manifestFormat || m.SchemaVersion != rowSchema || strings.TrimSpace(m.Generation) == "" {
		return nil, m, fmt.Errorf("manifest identity is incomplete")
	}
	descriptor, ok := m.Files["locations.ndjson"]
	if !ok {
		return nil, m, fmt.Errorf("manifest has no locations artifact")
	}
	info, err := os.Stat(locationsPath)
	if err != nil {
		return nil, m, fmt.Errorf("locations: %w", err)
	}
	if info.Size() > maxFileBytes {
		return nil, m, fmt.Errorf("locations exceed %d bytes", maxFileBytes)
	}
	if info.Size() != descriptor.Bytes {
		return nil, m, fmt.Errorf("locations bytes do not match manifest")
	}
	raw, err := os.ReadFile(locationsPath)
	if err != nil {
		return nil, m, fmt.Errorf("locations: %w", err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != descriptor.SHA256 {
		return nil, m, fmt.Errorf("locations hash does not match manifest")
	}
	return raw, m, nil
}

func parseRows(raw []byte, m manifest) ([]directoryadapters.TierLocation, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	var rows []directoryadapters.TierLocation
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if len(rows) >= maxRows {
			return nil, fmt.Errorf("row cap %d exceeded", maxRows)
		}
		var row locationRow
		if err := json.Unmarshal([]byte(text), &row); err != nil {
			return nil, fmt.Errorf("line %d: invalid json", line)
		}
		loc, err := validateRow(row, m)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		rows = append(rows, loc)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("locations file is empty")
	}
	return rows, nil
}

var errRow = errors.New("invalid location row")

func validateRow(row locationRow, m manifest) (directoryadapters.TierLocation, error) {
	if row.SchemaVersion != rowSchema {
		return directoryadapters.TierLocation{}, fmt.Errorf("%w: schema", errRow)
	}
	cnpj, err := kernel.ParseCNPJ(row.CNPJ)
	if err != nil {
		return directoryadapters.TierLocation{}, fmt.Errorf("%w: cnpj", errRow)
	}
	quality, err := domain.ParseQuality(row.Quality)
	if err != nil || !domain.IsPin(quality) {
		return directoryadapters.TierLocation{}, fmt.Errorf("%w: quality %q is not a pin tier", errRow, row.Quality)
	}
	if quality == domain.QualityANPReported && !m.OwnerDecisions.D1 {
		return directoryadapters.TierLocation{}, fmt.Errorf("%w: anp-reported needs owner decision D1", errRow)
	}
	if quality == domain.QualityAddressPoint && !m.OwnerDecisions.D2 {
		return directoryadapters.TierLocation{}, fmt.Errorf("%w: address-point needs owner decision D2", errRow)
	}
	lat, latErr := strconv.ParseFloat(strings.TrimSpace(row.LatitudeDeg), 64)
	lon, lonErr := strconv.ParseFloat(strings.TrimSpace(row.LongitudeDeg), 64)
	if latErr != nil || lonErr != nil || math.IsNaN(lat) || math.IsNaN(lon) ||
		lat < -34 || lat > 6 || lon < -74 || lon > -28 {
		return directoryadapters.TierLocation{}, fmt.Errorf("%w: point outside Brazil", errRow)
	}
	provider := strings.TrimSpace(row.Provider)
	reference := strings.TrimSpace(row.SourceReference)
	if provider == "" || len(provider) > maxProviderLen || reference == "" || len(reference) > maxReferenceLen {
		return directoryadapters.TierLocation{}, fmt.Errorf("%w: provider/reference", errRow)
	}
	return directoryadapters.TierLocation{
		CNPJ:            cnpj.Normalized(),
		Quality:         quality,
		Latitude:        lat,
		Longitude:       lon,
		Provider:        provider,
		SourceReference: reference,
	}, nil
}
