// Command station-prices loads one Rust-prepared exact ANP survey into the
// Official module. It resolves only existing canonical CNPJs, never creates a
// station, and publishes through the same importer/review-gate used by the
// regular ANP worker.
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
	"os"
	"strings"
	"time"

	dbdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	officialadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	odomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	factSchema     = "anp-station-price-v1"
	priceParser    = "anp-station-prices-v1"
	maxFacts       = 100000
	sourceRevision = "anp-lpc-rust-v1"
)

type priceFact struct {
	SchemaVersion string `json:"schema_version"`
	CNPJ          string `json:"cnpj"`
	Product       string `json:"product"`
	Unit          string `json:"unit"`
	AmountMilli   int64  `json:"amount_milli_brl"`
	ObservedAt    string `json:"observed_at"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	SourceRow     int    `json:"source_row"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "station-prices:", err)
		os.Exit(1)
	}
}

func run() error {
	factsPath := flag.String("facts", "", "prices.ndjson produced by prepare_anp_prices")
	manifestPath := flag.String("manifest", "", "prepared price manifest")
	sourceURL := flag.String("source-url", "", "official source URL recorded with the revision")
	onlyCNPJs := flag.String("only-cnpjs", "", "optional review report/CNPJ list bounding the pilot scope")
	dsn := flag.String("dsn", os.Getenv("ANPFUEL_DATABASE_URL"), "postgres DSN (or ANPFUEL_DATABASE_URL)")
	dryRun := flag.Bool("dry-run", false, "validate and resolve stations without writing")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall deadline")
	flag.Parse()
	if *factsPath == "" || *manifestPath == "" || *sourceURL == "" || *dsn == "" {
		return fmt.Errorf("usage: station-prices --facts FILE --manifest FILE --source-url URL [--dsn URL] [--dry-run]")
	}
	if *timeout <= 0 || *timeout > 30*time.Minute {
		return fmt.Errorf("timeout must be positive and at most 30 minutes")
	}
	manifest, err := readManifest(*manifestPath)
	if err != nil {
		return err
	}
	che, err := os.Stat(*factsPath)
	if err != nil {
		return fmt.Errorf("facts: %w", err)
	}
	if che.Size() != manifest.Files["prices.ndjson"].Bytes {
		return fmt.Errorf("facts size does not match manifest")
	}
	rawHash, err := fileSHA256(*factsPath)
	if err != nil {
		return err
	}
	if rawHash != manifest.Files["prices.ndjson"].SHA256 {
		return fmt.Errorf("facts hash does not match manifest")
	}
	start, err := time.Parse("2006-01-02", manifest.PeriodStart)
	if err != nil {
		return fmt.Errorf("manifest period start: %w", err)
	}
	end, err := time.Parse("2006-01-02", manifest.PeriodEnd)
	if err != nil || end.Before(start) {
		return fmt.Errorf("manifest period end: invalid")
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
	scope, err := loadScope(*onlyCNPJs)
	if err != nil {
		return err
	}
	importer := officialadapters.NewImporter(pool)
	if *dryRun {
		return dryRunFacts(ctx, pool, *factsPath, scope)
	}
	begun, err := retry(ctx, func() (officialadapters.BeginResult, error) {
		return importer.BeginRun(ctx, odomain.ImportKey{
			SourceURL:      *sourceURL,
			SourceChecksum: manifest.SourceSHA256,
			ParserVersion:  priceParser,
		}, start, end)
	})
	if err != nil {
		return err
	}
	if begun.NoOp {
		fmt.Println("import=noop identical source/parser already completed")
		return nil
	}
	var tally odomain.QuarantineTally
	var staged int64
	batch := make([]odomain.PriceRow, 0, 500)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := retry(ctx, func() (int64, error) {
			return importer.StageBatch(ctx, begun.RevisionID, batch, &tally)
		})
		staged += n
		batch = batch[:0]
		return err
	}
	err = scanFacts(*factsPath, func(fact priceFact) error {
		if scope != nil && !scope[fact.CNPJ] {
			return nil
		}
		row, reason := validateFact(fact, start, end)
		if reason != "" {
			tally.Add(reason, fact.CNPJ)
			return nil
		}
		stationID, err := resolveStation(ctx, pool, fact.CNPJ)
		if errors.Is(err, pgx.ErrNoRows) {
			tally.Add("unresolved-station", fact.CNPJ)
			return nil
		}
		if err != nil {
			return err
		}
		row.StationID = stationID
		batch = append(batch, row)
		if len(batch) >= 500 {
			return flush()
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	fin, err := retry(ctx, func() (officialadapters.FinishResult, error) {
		return importer.FinishRun(ctx, begun.RunID, begun.RevisionID, &tally)
	})
	if err != nil {
		return err
	}
	fmt.Printf("revision=%s staged=%d quarantined=%d published=%t reason=%s\n",
		begun.RevisionID, staged, tally.Total(), fin.Published, fin.Verdict.Reason)
	if !fin.Published {
		return fmt.Errorf("revision did not publish: %s", fin.Verdict.Reason)
	}
	return nil
}

type manifestFile struct {
	Format        string `json:"format"`
	ParserVersion string `json:"parser_version"`
	SourceSHA256  string `json:"source_sha256"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	Files         map[string]struct {
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func readManifest(path string) (manifestFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifestFile{}, err
	}
	if len(raw) > 1<<20 {
		return manifestFile{}, fmt.Errorf("manifest exceeds 1 MiB")
	}
	var manifest manifestFile
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifestFile{}, err
	}
	if manifest.Format != "abastevo.anp-station-prices-v1" ||
		manifest.ParserVersion != priceParser ||
		len(manifest.SourceSHA256) != 64 {
		return manifestFile{}, fmt.Errorf("unsupported price manifest")
	}
	if _, ok := manifest.Files["prices.ndjson"]; !ok {
		return manifestFile{}, fmt.Errorf("manifest has no prices artifact")
	}
	return manifest, nil
}

func scanFacts(path string, visit func(priceFact) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 256<<10)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if count >= maxFacts {
			return fmt.Errorf("fact row cap exceeded")
		}
		var fact priceFact
		if err := json.Unmarshal([]byte(line), &fact); err != nil {
			return fmt.Errorf("invalid fact json: %w", err)
		}
		if fact.SchemaVersion != factSchema {
			return fmt.Errorf("unsupported fact schema")
		}
		count++
		if err := visit(fact); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func validateFact(fact priceFact, start, end time.Time) (odomain.PriceRow, string) {
	product, supported := officialProduct(fact.Product)
	if !supported {
		return odomain.PriceRow{}, "official-product-unsupported"
	}
	unit, ok := productUnit(product)
	if !ok {
		return odomain.PriceRow{}, "unmapped-product"
	}
	if fact.Unit != unit {
		return odomain.PriceRow{}, "product-unit-mismatch"
	}
	if fact.AmountMilli < 1 || fact.AmountMilli > 1_000_000 {
		return odomain.PriceRow{}, "price-out-of-range"
	}
	observed, err := time.Parse("2006-01-02", fact.ObservedAt)
	if err != nil || observed.Before(start) || observed.After(end) {
		return odomain.PriceRow{}, "outside-period"
	}
	if fact.PeriodStart != start.Format("2006-01-02") || fact.PeriodEnd != end.Format("2006-01-02") {
		return odomain.PriceRow{}, "period-mismatch"
	}
	return odomain.PriceRow{
		Product:     product,
		Unit:        fact.Unit,
		AmountMilli: fact.AmountMilli,
		RawText:     fmt.Sprintf("%d", fact.AmountMilli),
		CollectedOn: observed,
		SourceRow:   fact.SourceRow,
	}, ""
}

// officialProduct translates the frozen map/ANP product vocabulary to the
// Official module's persisted vocabulary. Premium-grade gasoline has no
// official_station_prices constraint value yet, so it stays out of this
// pilot revision instead of being mislabeled as additive gasoline.
func officialProduct(product string) (string, bool) {
	switch product {
	case "ETHANOL_HYDRATED":
		return "ETHANOL", true
	case "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "DIESEL_S500", "DIESEL_S10", "CNG", "LPG_P13":
		return product, true
	case "GASOLINE_PREMIUM_GRADE":
		return "", false
	default:
		return "", false
	}
}

func productUnit(product string) (string, bool) {
	switch product {
	case "CNG":
		return "M3", true
	case "LPG_P13":
		return "KG_13", true
	case "ETHANOL", "ETHANOL_HYDRATED", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED",
		"GASOLINE_PREMIUM_GRADE", "DIESEL_S500", "DIESEL_S10":
		return "L", true
	default:
		return "", false
	}
}

func resolveStation(ctx context.Context, pool *pgxpool.Pool, cnpj string) (string, error) {
	row, err := dbdirectory.New(pool).ResolveActiveIdentifier(ctx, dbdirectory.ResolveActiveIdentifierParams{
		Kind: "CNPJ", Value: cnpj,
	})
	if err != nil {
		return "", err
	}
	if row.Status != "active" {
		return "", pgx.ErrNoRows
	}
	return uuidText(row.ID), nil
}

func uuidText(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := fmt.Sprintf("%x", id.Bytes)
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func loadScope(path string) (map[string]bool, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("scope: %w", err)
	}
	defer file.Close()
	scope := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 256<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") {
			var row struct {
				CNPJ string `json:"cnpj"`
			}
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				return nil, fmt.Errorf("scope json: %w", err)
			}
			line = row.CNPJ
		}
		if len(line) != 14 {
			return nil, fmt.Errorf("scope CNPJ must have 14 characters")
		}
		scope[line] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(scope) == 0 {
		return nil, fmt.Errorf("scope is empty")
	}
	return scope, nil
}

func dryRunFacts(ctx context.Context, pool *pgxpool.Pool, path string, scope map[string]bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var total, resolved, unresolved int
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 256<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var fact priceFact
		if err := json.Unmarshal([]byte(line), &fact); err != nil {
			return err
		}
		if scope != nil && !scope[fact.CNPJ] {
			continue
		}
		total++
		if _, err := resolveStation(ctx, pool, fact.CNPJ); err != nil {
			unresolved++
		} else {
			resolved++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	fmt.Printf("facts=%d resolved=%d unresolved=%d\n", total, resolved, unresolved)
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func retry[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	var zero T
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		var value T
		value, err = operation()
		if err == nil || ctx.Err() != nil || attempt == 3 {
			return value, err
		}
		timer := time.NewTimer(time.Duration(attempt) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
	return zero, err
}
