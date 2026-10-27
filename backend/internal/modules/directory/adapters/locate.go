package adapters

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

// TierLocation is one validated best-tier pin for a station identified by its
// normalized CNPJ (B-BR-MAP-02). Coordinates are WGS84/SIRGAS2000 degrees;
// the sub-metre datum difference is below the source precision.
type TierLocation struct {
	CNPJ            string
	Quality         string
	Latitude        float64
	Longitude       float64
	Provider        string
	SourceReference string
}

// Location outcomes reported per row.
const (
	LocateApplied    = "applied"
	LocateUnchanged  = "unchanged"
	LocateHigherTier = "kept-higher-tier"
	LocateUnknown    = "unknown-station"
	LocateInactive   = "inactive-station"
	// Same tier within this many metres is a no-op (idempotent reruns).
	locateSamePointM    = "0.5"
	locateStatementTime = "300s"
)

// LocateResult counts outcomes; Applied is broken down by quality.
type LocateResult struct {
	Rows     int            `json:"rows"`
	Outcomes map[string]int `json:"outcomes"`
	Applied  map[string]int `json:"applied_by_quality"`
	DryRun   bool           `json:"dry_run"`
	Skipped  []LocateSkip   `json:"-"`
}

// LocateSkip names a row that resolved to no active station.
type LocateSkip struct {
	CNPJ    string `json:"cnpj"`
	Outcome string `json:"outcome"`
}

var errLocateInput = errors.New("directory: invalid tier location batch")

// pinRankSQL renders the domain ladder as a SQL CASE so set-based tier
// comparisons can never drift from domain.PinRanks.
func pinRankSQL(column string) string {
	qualities := make([]string, 0, len(domain.PinRanks))
	for quality := range domain.PinRanks {
		qualities = append(qualities, quality)
	}
	sort.Strings(qualities)
	var b strings.Builder
	b.WriteString("CASE " + column)
	for _, quality := range qualities {
		fmt.Fprintf(&b, " WHEN '%s' THEN %d", quality, domain.PinRanks[quality])
	}
	b.WriteString(" ELSE 0 END")
	return b.String()
}

// ApplyTierLocations records and projects a batch of best-tier pins in one
// transaction: rows are staged with COPY, resolved through active CNPJ
// identifiers and the affected stations are locked before comparison. A
// lower tier never replaces a higher projection, an equal tier at the same
// point is a no-op, and every applied row appends a location revision that
// supersedes the previous projection. dryRun computes the same outcomes and
// rolls back.
func ApplyTierLocations(ctx context.Context, pool *pgxpool.Pool, rows []TierLocation, dryRun bool) (LocateResult, error) {
	result := LocateResult{Rows: len(rows), Outcomes: map[string]int{}, Applied: map[string]int{}, DryRun: dryRun}
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if !domain.IsPin(row.Quality) || row.CNPJ == "" {
			return result, errLocateInput
		}
		if _, dup := seen[row.CNPJ]; dup {
			return result, fmt.Errorf("%w: duplicate CNPJ %s", errLocateInput, row.CNPJ)
		}
		seen[row.CNPJ] = struct{}{}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '"+locateStatementTime+"'"); err != nil {
		return result, err
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE locate_stage (
		cnpj text PRIMARY KEY, quality text NOT NULL, lon float8 NOT NULL, lat float8 NOT NULL,
		provider text NOT NULL, source_reference text NOT NULL) ON COMMIT DROP`); err != nil {
		return result, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"locate_stage"},
		[]string{"cnpj", "quality", "lon", "lat", "provider", "source_reference"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{r.CNPJ, r.Quality, r.Longitude, r.Latitude, r.Provider, r.SourceReference}, nil
		})); err != nil {
		return result, fmt.Errorf("stage: %w", err)
	}
	// Lock in a stable order so concurrent batches cannot deadlock.
	if _, err := tx.Exec(ctx, `SELECT s.id FROM directory_stations s
		JOIN directory_identifiers i ON i.station_id = s.id AND i.kind = 'CNPJ' AND i.valid_to IS NULL
		JOIN locate_stage st ON st.cnpj = i.normalized_value
		ORDER BY s.id FOR UPDATE OF s`); err != nil {
		return result, fmt.Errorf("lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE locate_resolved ON COMMIT DROP AS
		SELECT st.cnpj, st.quality, st.provider, st.source_reference,
			ST_SetSRID(ST_MakePoint(st.lon, st.lat), 4326)::geography AS point,
			s.id AS station_id, s.current_revision_id,
			CASE
				WHEN s.id IS NULL THEN '`+LocateUnknown+`'
				WHEN s.status <> 'active' THEN '`+LocateInactive+`'
				WHEN `+pinRankSQL("s.current_quality")+` > `+pinRankSQL("st.quality")+` THEN '`+LocateHigherTier+`'
				WHEN s.current_quality = st.quality AND s.current_point IS NOT NULL
					AND ST_DWithin(s.current_point, ST_SetSRID(ST_MakePoint(st.lon, st.lat), 4326)::geography, `+locateSamePointM+`)
					THEN '`+LocateUnchanged+`'
				ELSE '`+LocateApplied+`'
			END AS outcome
		FROM locate_stage st
		LEFT JOIN directory_identifiers i ON i.kind = 'CNPJ' AND i.normalized_value = st.cnpj AND i.valid_to IS NULL
		LEFT JOIN directory_stations s ON s.id = i.station_id`); err != nil {
		return result, fmt.Errorf("resolve: %w", err)
	}
	if _, err := tx.Exec(ctx, `WITH ins AS (
			INSERT INTO directory_location_revisions
				(id, station_id, point, quality, provider, source_reference, supersedes_id)
			SELECT gen_random_uuid(), station_id, point, quality, provider, source_reference, current_revision_id
			FROM locate_resolved WHERE outcome = '`+LocateApplied+`'
			RETURNING id, station_id, point, quality)
		UPDATE directory_stations s
		SET current_point = ins.point, current_quality = ins.quality, current_revision_id = ins.id
		FROM ins WHERE s.id = ins.station_id`); err != nil {
		return result, fmt.Errorf("apply: %w", err)
	}
	counts, err := tx.Query(ctx, `SELECT outcome, quality, count(*) FROM locate_resolved GROUP BY outcome, quality`)
	if err != nil {
		return result, err
	}
	for counts.Next() {
		var outcome, quality string
		var n int
		if err := counts.Scan(&outcome, &quality, &n); err != nil {
			counts.Close()
			return result, err
		}
		result.Outcomes[outcome] += n
		if outcome == LocateApplied {
			result.Applied[quality] += n
		}
	}
	counts.Close()
	if err := counts.Err(); err != nil {
		return result, err
	}
	skipped, err := tx.Query(ctx, `SELECT cnpj, outcome FROM locate_resolved
		WHERE outcome IN ('`+LocateUnknown+`', '`+LocateInactive+`') ORDER BY cnpj`)
	if err != nil {
		return result, err
	}
	for skipped.Next() {
		var skip LocateSkip
		if err := skipped.Scan(&skip.CNPJ, &skip.Outcome); err != nil {
			skipped.Close()
			return result, err
		}
		result.Skipped = append(result.Skipped, skip)
	}
	skipped.Close()
	if err := skipped.Err(); err != nil {
		return result, err
	}
	if dryRun {
		return result, tx.Rollback(ctx)
	}
	return result, tx.Commit(ctx)
}
