package mapdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StaticOverlayFormat         = "abastevo.map-overlay-v1"
	StaticOverlayManifestFormat = "abastevo.map-overlay-manifest-v1"
)

type StaticOverlayOptions struct {
	OutputRoot string
}

type StaticPriceWeek struct {
	SurveyStart string `json:"survey_week_start"`
	SurveyEnd   string `json:"survey_week_end"`
}

type StaticOverlayCounts struct {
	Assertions     int `json:"assertions"`
	WithPrices     int `json:"with_prices"`
	UFs            int `json:"ufs"`
	SkippedNoState int `json:"skipped_no_state"`
}

type StaticOverlayFile struct {
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256"`
	Assertions int    `json:"assertions"`
	WithPrices int    `json:"with_prices"`
}

type StaticOverlayPrice struct {
	Source          string `json:"source"`
	FuelProduct     string `json:"fuel_product"`
	Unit            string `json:"unit"`
	Condition       string `json:"condition"`
	AmountMilliBRL  int64  `json:"amount_milli_brl"`
	Currency        string `json:"currency"`
	CollectedOn     string `json:"collected_on"`
	SurveyWeekStart string `json:"survey_week_start"`
	SurveyWeekEnd   string `json:"survey_week_end"`
	RevisionID      string `json:"revision_id"`
}

type StaticOverlayAssertion struct {
	ID           string               `json:"id"`
	Source       string               `json:"source"`
	Kind         string               `json:"kind"`
	Name         string               `json:"name"`
	CNPJ         string               `json:"cnpj,omitempty"`
	OSMType      string               `json:"osm_type,omitempty"`
	OSMID        *int64               `json:"osm_id,omitempty"`
	LatitudeDeg  string               `json:"latitude_deg,omitempty"`
	LongitudeDeg string               `json:"longitude_deg,omitempty"`
	Quality      string               `json:"quality"`
	DisplayPoint string               `json:"display_point,omitempty"`
	Services     []string             `json:"services"`
	Rights       json.RawMessage      `json:"rights,omitempty"`
	Capabilities json.RawMessage      `json:"capabilities,omitempty"`
	Prices       []StaticOverlayPrice `json:"prices"`
}

type staticOverlayUF struct {
	Format     string                   `json:"format"`
	Generation string                   `json:"generation"`
	UF         string                   `json:"uf"`
	PriceWeek  *StaticPriceWeek         `json:"price_week"`
	Assertions []StaticOverlayAssertion `json:"assertions"`
}

type staticOverlayManifest struct {
	Format         string                       `json:"format"`
	Release        string                       `json:"release"`
	Generation     string                       `json:"generation"`
	GenerationID   string                       `json:"generation_id"`
	SourceRevision string                       `json:"source_revision"`
	ActivatedAt    string                       `json:"activated_at"`
	PriceWeek      *StaticPriceWeek             `json:"price_week"`
	Counts         StaticOverlayCounts          `json:"counts"`
	Files          map[string]StaticOverlayFile `json:"files"`
	ContentSHA256  string                       `json:"content_sha256"`
}

type StaticOverlayResult struct {
	Release        string
	Generation     string
	GenerationID   string
	SourceRevision string
	ActivatedAt    time.Time
	PriceWeek      *StaticPriceWeek
	Counts         StaticOverlayCounts
	Files          map[string]StaticOverlayFile
	ContentSHA256  string
	Noop           bool
}

type staticOverlayRow struct {
	UF        string
	Assertion StaticOverlayAssertion
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sanitizeRelease(value string) string {
	var builder strings.Builder
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '-', character == '_', character == '.':
			builder.WriteRune(character)
		default:
			builder.WriteByte('-')
		}
	}
	cleaned := strings.Trim(builder.String(), "-.")
	if cleaned == "" {
		return "release"
	}
	if len(cleaned) > 80 {
		return cleaned[:80]
	}
	return cleaned
}

func assembleStaticOverlay(
	generation, generationID, sourceRevision string,
	activatedAt time.Time,
	rows []staticOverlayRow,
	prices map[string][]StaticOverlayPrice,
	priceWeek *StaticPriceWeek,
) (map[string][]byte, staticOverlayManifest, StaticOverlayResult, error) {
	sort.Slice(rows, func(left, right int) bool {
		return rows[left].Assertion.ID < rows[right].Assertion.ID
	})
	byUF := map[string][]StaticOverlayAssertion{}
	counts := StaticOverlayCounts{}
	for _, row := range rows {
		uf := strings.TrimSpace(row.UF)
		if uf == "" {
			uf = "??"
			counts.SkippedNoState++
		}
		assertion := row.Assertion
		if assertion.CNPJ != "" {
			if values, ok := prices[assertion.CNPJ]; ok {
				assertion.Prices = append([]StaticOverlayPrice(nil), values...)
				sort.Slice(assertion.Prices, func(left, right int) bool {
					if assertion.Prices[left].FuelProduct != assertion.Prices[right].FuelProduct {
						return assertion.Prices[left].FuelProduct < assertion.Prices[right].FuelProduct
					}
					return assertion.Prices[left].Unit < assertion.Prices[right].Unit
				})
			}
		}
		if assertion.Prices == nil {
			assertion.Prices = []StaticOverlayPrice{}
		}
		byUF[uf] = append(byUF[uf], assertion)
		counts.Assertions++
		if len(assertion.Prices) > 0 {
			counts.WithPrices++
		}
	}
	ufNames := make([]string, 0, len(byUF))
	for uf := range byUF {
		ufNames = append(ufNames, uf)
	}
	sort.Strings(ufNames)
	files := make(map[string][]byte, len(ufNames))
	fileDigests := make(map[string]StaticOverlayFile, len(ufNames))
	for _, uf := range ufNames {
		payload := staticOverlayUF{
			Format:     StaticOverlayFormat,
			Generation: generation,
			UF:         uf,
			PriceWeek:  priceWeek,
			Assertions: byUF[uf],
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, staticOverlayManifest{}, StaticOverlayResult{}, err
		}
		raw = append(raw, '\n')
		name := uf + ".json"
		files[name] = raw
		withPrices := 0
		for _, assertion := range byUF[uf] {
			if len(assertion.Prices) > 0 {
				withPrices++
			}
		}
		fileDigests[name] = StaticOverlayFile{
			Bytes:      int64(len(raw)),
			SHA256:     sha256Hex(raw),
			Assertions: len(byUF[uf]),
			WithPrices: withPrices,
		}
	}
	counts.UFs = len(ufNames)
	manifest := staticOverlayManifest{
		Format:         StaticOverlayManifestFormat,
		Generation:     generation,
		GenerationID:   generationID,
		SourceRevision: sourceRevision,
		ActivatedAt:    activatedAt.UTC().Format(time.RFC3339),
		PriceWeek:      priceWeek,
		Counts:         counts,
		Files:          fileDigests,
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return nil, staticOverlayManifest{}, StaticOverlayResult{}, err
	}
	contentSHA256 := sha256Hex(canonical)
	release := sanitizeRelease(generation) + "-" + contentSHA256[:8]
	manifest.Release = release
	manifest.ContentSHA256 = contentSHA256
	result := StaticOverlayResult{
		Release:        release,
		Generation:     generation,
		GenerationID:   generationID,
		SourceRevision: sourceRevision,
		ActivatedAt:    activatedAt,
		PriceWeek:      priceWeek,
		Counts:         counts,
		Files:          fileDigests,
		ContentSHA256:  contentSHA256,
	}
	return files, manifest, result, nil
}

func writeStaticOverlay(outputRoot string, files map[string][]byte, manifest staticOverlayManifest, result *StaticOverlayResult) error {
	if outputRoot == "" {
		return errors.New("static overlay output root required")
	}
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		return err
	}
	releaseDir := filepath.Join(outputRoot, manifest.Release)
	if _, err := os.Stat(releaseDir); err == nil {
		existing, err := os.ReadFile(filepath.Join(releaseDir, "manifest.json"))
		if err != nil {
			return err
		}
		var parsed staticOverlayManifest
		if err := json.Unmarshal(existing, &parsed); err != nil {
			return err
		}
		if parsed.ContentSHA256 != manifest.ContentSHA256 {
			return fmt.Errorf("static overlay release %s exists with different content", manifest.Release)
		}
		if err := updateCurrentSymlink(outputRoot, manifest.Release); err != nil {
			return err
		}
		result.Noop = true
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	tempDir, err := os.MkdirTemp(outputRoot, ".tmp-"+manifest.Release+"-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempDir)
		}
	}()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := files[name]
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return err
		}
		written, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if sha256Hex(written) != manifest.Files[name].SHA256 {
			return fmt.Errorf("static overlay file %s hash mismatch after write", name)
		}
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(filepath.Join(tempDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tempDir, releaseDir); err != nil {
		return err
	}
	cleanup = false
	if err := updateCurrentSymlink(outputRoot, manifest.Release); err != nil {
		return err
	}
	return nil
}

func updateCurrentSymlink(outputRoot, release string) error {
	current := filepath.Join(outputRoot, "current")
	if target, err := os.Readlink(current); err == nil && target == release {
		return nil
	}
	temp := filepath.Join(outputRoot, ".current-"+strconv.Itoa(os.Getpid()))
	_ = os.Remove(temp)
	if err := os.Symlink(release, temp); err != nil {
		return err
	}
	if err := os.Rename(temp, current); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

// BuildStaticOverlay reads the active map generation and the latest published
// official week, then writes an immutable, release-addressed per-UF JSON
// overlay. It never writes to the database.
func BuildStaticOverlay(ctx context.Context, pool *pgxpool.Pool, opts StaticOverlayOptions) (StaticOverlayResult, error) {
	if opts.OutputRoot == "" {
		return StaticOverlayResult{}, errors.New("static overlay output root required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return StaticOverlayResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var generationID, generationKey, sourceRevision string
	var activatedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT id::text, generation_key, source_revision, activated_at
		FROM map_generations WHERE status = 'ACTIVE'`).Scan(&generationID, &generationKey, &sourceRevision, &activatedAt); err != nil {
		return StaticOverlayResult{}, fmt.Errorf("active map generation: %w", err)
	}

	rows, err := tx.Query(ctx, `SELECT a.assertion_id::text, a.source, a.kind, a.name,
			COALESCE(a.cnpj, ''), COALESCE(a.osm_type, ''), a.osm_id,
			COALESCE(a.latitude_deg::text, ''), COALESCE(a.longitude_deg::text, ''),
			a.quality, COALESCE(a.display_point, ''), a.services, a.rights, a.capabilities,
			COALESCE(ds.state, '')
		FROM map_assertions a
		LEFT JOIN directory_identifiers di
			ON di.kind = 'CNPJ' AND di.normalized_value = a.cnpj AND di.valid_to IS NULL
		LEFT JOIN directory_stations ds ON ds.id = di.station_id
		WHERE a.generation_id = $1
		ORDER BY a.assertion_id`, generationID)
	if err != nil {
		return StaticOverlayResult{}, err
	}
	defer rows.Close()
	overlayRows := make([]staticOverlayRow, 0)
	for rows.Next() {
		var row staticOverlayRow
		var assertion StaticOverlayAssertion
		var rights, capabilities []byte
		if err := rows.Scan(
			&assertion.ID, &assertion.Source, &assertion.Kind, &assertion.Name,
			&assertion.CNPJ, &assertion.OSMType, &assertion.OSMID,
			&assertion.LatitudeDeg, &assertion.LongitudeDeg, &assertion.Quality,
			&assertion.DisplayPoint, &assertion.Services, &rights, &capabilities, &row.UF,
		); err != nil {
			return StaticOverlayResult{}, err
		}
		if len(rights) > 0 {
			assertion.Rights = json.RawMessage(rights)
		}
		if len(capabilities) > 0 {
			assertion.Capabilities = json.RawMessage(capabilities)
		}
		row.Assertion = assertion
		overlayRows = append(overlayRows, row)
	}
	if err := rows.Err(); err != nil {
		return StaticOverlayResult{}, err
	}

	var priceWeek *StaticPriceWeek
	var revisionID, surveyStart, surveyEnd string
	err = tx.QueryRow(ctx, `SELECT revision_id::text, survey_start::text, survey_end::text
		FROM official_current_revisions
		ORDER BY survey_end DESC, survey_start DESC
		LIMIT 1`).Scan(&revisionID, &surveyStart, &surveyEnd)
	if err == nil {
		priceWeek = &StaticPriceWeek{SurveyStart: surveyStart, SurveyEnd: surveyEnd}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return StaticOverlayResult{}, err
	}
	prices := map[string][]StaticOverlayPrice{}
	if priceWeek != nil {
		priceRows, err := tx.Query(ctx, `WITH latest AS (
				SELECT revision_id, survey_start, survey_end
				FROM official_current_revisions
				ORDER BY survey_end DESC, survey_start DESC
				LIMIT 1
			)
			SELECT di.normalized_value, p.fuel_product, p.unit,
				p.amount_milli_brl, p.collected_on::text,
				latest.survey_start::text, latest.survey_end::text, latest.revision_id::text
			FROM latest
			JOIN official_station_prices p ON p.revision_id = latest.revision_id
			JOIN directory_identifiers di
				ON di.station_id = p.station_id AND di.kind = 'CNPJ' AND di.valid_to IS NULL
			ORDER BY di.normalized_value, p.fuel_product, p.unit`)
		if err != nil {
			return StaticOverlayResult{}, err
		}
		defer priceRows.Close()
		for priceRows.Next() {
			var cnpj string
			var price StaticOverlayPrice
			if err := priceRows.Scan(&cnpj, &price.FuelProduct, &price.Unit, &price.AmountMilliBRL,
				&price.CollectedOn, &price.SurveyWeekStart, &price.SurveyWeekEnd, &price.RevisionID); err != nil {
				return StaticOverlayResult{}, err
			}
			price.Source = "ANP"
			price.Condition = "STANDARD"
			price.Currency = "BRL"
			prices[cnpj] = append(prices[cnpj], price)
		}
		if err := priceRows.Err(); err != nil {
			return StaticOverlayResult{}, err
		}
	}

	files, manifest, result, err := assembleStaticOverlay(
		generationKey, generationID, sourceRevision, activatedAt, overlayRows, prices, priceWeek,
	)
	if err != nil {
		return StaticOverlayResult{}, err
	}
	if err := writeStaticOverlay(opts.OutputRoot, files, manifest, &result); err != nil {
		return StaticOverlayResult{}, err
	}
	return result, nil
}
