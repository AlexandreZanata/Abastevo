// Command station-review applies an explicit, evidence-bound location review to
// canonical Directory stations. It never creates stations, never guesses a
// coordinate and never marks a row reviewed without a complete review record.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	dbdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	directoryadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	reviewSchema   = "station-location-review-v1"
	reviewMethod   = "ANP_REGISTRY_API+OSM_ADDRESS_NAME"
	maxReviewRows  = 10000
	maxEvidenceLen = 2000
)

type reviewRow struct {
	SchemaVersion  string  `json:"schema_version"`
	CNPJ           string  `json:"cnpj"`
	StationName    string  `json:"station_name"`
	Municipality   string  `json:"municipality"`
	State          string  `json:"state"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	Method         string  `json:"method"`
	MatchDistanceM float64 `json:"match_distance_m"`
	Reviewer       string  `json:"reviewer"`
	Evidence       string  `json:"evidence"`
}

type reportRow struct {
	CNPJ      string `json:"cnpj"`
	StationID string `json:"station_id,omitempty"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "station-review:", err)
		os.Exit(1)
	}
}

func run() error {
	reviewsPath := flag.String("reviews", "", "review JSONL produced by prepare_location_reviews")
	manifestPath := flag.String("manifest", "", "review manifest to verify before applying")
	reportPath := flag.String("report", "", "optional applied/skipped report JSONL")
	dsn := flag.String("dsn", os.Getenv("ANPFUEL_DATABASE_URL"), "postgres DSN (or ANPFUEL_DATABASE_URL)")
	dryRun := flag.Bool("dry-run", false, "validate and resolve stations without writing")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall deadline")
	flag.Parse()
	if *reviewsPath == "" || *dsn == "" {
		return fmt.Errorf("usage: station-review --reviews FILE [--report FILE] [--dsn URL] [--dry-run]")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return fmt.Errorf("timeout must be positive and at most 10 minutes")
	}
	if *manifestPath != "" {
		if err := verifyManifest(*manifestPath, *reviewsPath); err != nil {
			return err
		}
	} else if !*dryRun {
		return fmt.Errorf("--manifest is required unless --dry-run")
	}
	rows, err := readReviews(*reviewsPath)
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
	repo := directoryadapters.NewRepository(pool)
	canon := directoryadapters.RegistryCanonicalizer{Repo: repo}
	report, err := os.CreateTemp(orReport(*reportPath), ".station-review-report-*")
	if err != nil {
		return fmt.Errorf("report: %w", err)
	}
	defer func() {
		_ = report.Close()
		_ = os.Remove(report.Name())
	}()
	var applied, skipped, failed int
	for _, row := range rows {
		result := applyReview(ctx, pool, repo, canon, row, *dryRun)
		switch result.Status {
		case "applied":
			applied++
		case "already-reviewed", "dry-run":
			skipped++
		default:
			failed++
		}
		if _, err := report.WriteString(mustJSON(result) + "\n"); err != nil {
			return fmt.Errorf("report write: %w", err)
		}
	}
	if err := report.Sync(); err != nil {
		return fmt.Errorf("report sync: %w", err)
	}
	if *reportPath != "" {
		if err := copyReport(report, *reportPath); err != nil {
			return err
		}
	}
	fmt.Printf("reviews=%d applied=%d skipped=%d failed=%d dry_run=%t\n", len(rows), applied, skipped, failed, *dryRun)
	if failed > 0 {
		return fmt.Errorf("%d review rows failed", failed)
	}
	return nil
}

func orReport(path string) string {
	if path == "" {
		return os.TempDir()
	}
	return filepath.Dir(path)
}

func copyReport(source *os.File, path string) error {
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".station-review-final-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
	}()
	if _, err := io.Copy(temp, source); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

func applyReview(ctx context.Context, pool *pgxpool.Pool, repo *directoryadapters.Repository, canon directoryadapters.RegistryCanonicalizer, row reviewRow, dryRun bool) reportRow {
	cnpj, err := kernel.ParseCNPJ(row.CNPJ)
	if err != nil {
		return reportRow{CNPJ: row.CNPJ, Status: "failed", Detail: "invalid-cnpj"}
	}
	normalized := cnpj.Normalized()
	resolved, err := dbdirectory.New(pool).ResolveActiveIdentifier(ctx, dbdirectory.ResolveActiveIdentifierParams{
		Kind: "CNPJ", Value: normalized,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return reportRow{CNPJ: normalized, Status: "failed", Detail: "unknown-station"}
	}
	if err != nil {
		return reportRow{CNPJ: normalized, Status: "failed", Detail: "station lookup"}
	}
	stationID := uuidText(resolved.ID)
	if resolved.Status != "active" {
		return reportRow{CNPJ: normalized, StationID: stationID, Status: "failed", Detail: "inactive-station"}
	}
	station, err := repo.Station(ctx, stationID)
	if err != nil {
		return reportRow{CNPJ: normalized, StationID: stationID, Status: "failed", Detail: "station read"}
	}
	if station.CurrentQuality == "reviewed" {
		return reportRow{CNPJ: normalized, StationID: stationID, Status: "already-reviewed"}
	}
	if dryRun {
		return reportRow{CNPJ: normalized, StationID: stationID, Status: "dry-run"}
	}
	reference := fmt.Sprintf("%s|reviewer=%s|%s", row.Method, row.Reviewer, row.Evidence)
	if len(reference) > maxEvidenceLen {
		return reportRow{CNPJ: normalized, StationID: stationID, Status: "failed", Detail: "evidence-too-long"}
	}
	if err := canon.RecordReviewedPoint(ctx, stationID, row.Latitude, row.Longitude, reference); err != nil {
		return reportRow{CNPJ: normalized, StationID: stationID, Status: "failed", Detail: "record-reviewed-point"}
	}
	return reportRow{CNPJ: normalized, StationID: stationID, Status: "applied"}
}

func uuidText(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	raw := id.Bytes
	hexed := fmt.Sprintf("%x", raw)
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func mustJSON(value reportRow) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

type reviewManifest struct {
	Format         string  `json:"format"`
	ParserVersion  string  `json:"parser_version"`
	Reviewer       string  `json:"reviewer"`
	MaxDistanceM   float64 `json:"max_distance_m"`
	SourceSHA256   string  `json:"source_sha256"`
	OSMSHA256      string  `json:"osm_sha256"`
	RegistrySHA256 string  `json:"registry_sha256"`
	Counts         struct {
		Input      int64 `json:"input"`
		Reviewed   int64 `json:"reviewed"`
		Rejected   int64 `json:"rejected"`
		Duplicates int64 `json:"duplicates"`
	} `json:"counts"`
	Files map[string]struct {
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func verifyManifest(manifestPath, reviewsPath string) error {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if len(raw) > 1<<20 {
		return fmt.Errorf("manifest exceeds 1 MiB")
	}
	var manifest reviewManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if manifest.Format != "abastevo.station-location-review-v1" ||
		manifest.ParserVersion != "station-location-review-v1" ||
		strings.TrimSpace(manifest.Reviewer) == "" ||
		manifest.MaxDistanceM <= 0 || manifest.MaxDistanceM > 200 ||
		manifest.SourceSHA256 == "" || manifest.OSMSHA256 == "" || manifest.RegistrySHA256 == "" {
		return fmt.Errorf("manifest identity is incomplete")
	}
	descriptor, ok := manifest.Files["reviews.ndjson"]
	if !ok {
		return fmt.Errorf("manifest has no reviews artifact")
	}
	info, err := os.Stat(reviewsPath)
	if err != nil {
		return fmt.Errorf("reviews: %w", err)
	}
	if info.Size() != descriptor.Bytes {
		return fmt.Errorf("review bytes do not match manifest")
	}
	rawReviews, err := os.ReadFile(reviewsPath)
	if err != nil {
		return fmt.Errorf("reviews: %w", err)
	}
	if sha256Hex(rawReviews) != descriptor.SHA256 {
		return fmt.Errorf("review hash does not match manifest")
	}
	if int64(len(rawReviews)) > 4<<20 {
		return fmt.Errorf("review file exceeds 4 MiB")
	}
	return nil
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func readReviews(path string) ([]reviewRow, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > 4<<20 {
		return nil, fmt.Errorf("review file exceeds 4 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 256<<10)
	var rows []reviewRow
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if len(rows) >= maxReviewRows {
			return nil, fmt.Errorf("review row cap exceeded")
		}
		var row reviewRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("invalid review json: %w", err)
		}
		if err := validateReview(row); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("review file is empty")
	}
	return rows, nil
}

func validateReview(row reviewRow) error {
	if row.SchemaVersion != reviewSchema {
		return fmt.Errorf("unsupported review schema")
	}
	if row.Method != reviewMethod {
		return fmt.Errorf("unsupported review method")
	}
	if strings.TrimSpace(row.Reviewer) == "" || strings.TrimSpace(row.Evidence) == "" {
		return fmt.Errorf("reviewer and evidence are required")
	}
	if math.IsNaN(row.Latitude) || math.IsInf(row.Latitude, 0) ||
		row.Latitude < -90 || row.Latitude > 90 ||
		math.IsNaN(row.Longitude) || math.IsInf(row.Longitude, 0) ||
		row.Longitude < -180 || row.Longitude > 180 {
		return fmt.Errorf("review coordinate out of range")
	}
	if row.MatchDistanceM < 0 || row.MatchDistanceM > 200 {
		return fmt.Errorf("review match distance out of bounds")
	}
	return nil
}
