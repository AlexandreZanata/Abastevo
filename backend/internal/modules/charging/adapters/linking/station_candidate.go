package linking

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

// OSMObject identifies the fuel-and-charging object in admitted source facts.
type OSMObject struct {
	Type string
	ID   int64
}

// StationSourceFacts carries source-backed values from the prepared charger
// record. Combined is set only for an admitted OSM COMBINED_SITE assertion.
type StationSourceFacts struct {
	TradeName string
	Brand     string
	Combined  *OSMObject
}

// StationCandidate identifies one canonical fuel station and the evidence
// path by which it was selected.
type StationCandidate struct {
	StationID string
	Rule      string
}

const (
	ruleCombinedOSM  = "combined-osm"
	rulePinTradeName = "pin-trade-name"
	rulePinBrand     = "pin-brand"
)

// Register only identities anchored to a completed ANP run. A curated label,
// stale CNPJ, or location evidence alone does not establish an ANP station.
const hasANPRegistryIdentity = `EXISTS (
	SELECT 1 FROM directory_identifiers i
	JOIN registry_assertions a ON a.station_id=s.id AND a.source_key=i.normalized_value
	WHERE i.station_id=s.id AND i.kind='CNPJ' AND i.valid_to IS NULL AND a.superseded_by IS NULL
	AND EXISTS (
		SELECT 1 FROM registry_source_runs r
		WHERE r.state='complete' AND r.finished_at IS NOT NULL AND r.source=a.source
		AND (r.id=a.run_id OR EXISTS (
			SELECT 1 FROM registry_run_assertions m WHERE m.run_id=r.id AND m.assertion_id=a.id
		))
		AND (a.source IN ('registry-csv','registry-api') OR (
			a.source='station-prep' AND a.latitude IS NULL AND a.longitude IS NULL
			AND jsonb_typeof(a.address)='object' AND a.address ? 'municipio_ibge' AND a.address ? 'uf'
			AND r.prepared_manifest_sha256<>'' AND r.snapshot_identity LIKE 'station-prep:%:registry-13col'
		))
	)
)`

type candidateQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// FindStationCandidate reads the eligible relationship without publishing it.
// The eventual importer uses the same resolver inside the link transaction.
func (s Store) FindStationCandidate(ctx context.Context, placeID string, facts StationSourceFacts) (StationCandidate, bool, error) {
	if s.Pool == nil {
		return StationCandidate{}, false, errors.New("linking: store unavailable")
	}
	return findStationCandidate(ctx, s.Pool, placeID, facts)
}

func findStationCandidate(ctx context.Context, db candidateQuerier, placeID string, facts StationSourceFacts) (StationCandidate, bool, error) {
	place, err := uuid(placeID)
	if err != nil {
		return StationCandidate{}, false, err
	}
	if object := facts.Combined; object != nil {
		if object.ID <= 0 || (object.Type != "node" && object.Type != "way" && object.Type != "relation") {
			return StationCandidate{}, false, errors.New("linking: invalid combined OSM object")
		}
		rows, err := db.Query(ctx, `SELECT DISTINCT s.id::text, ''::text, ''::text
			FROM map_assertions a
			JOIN map_generations g ON g.id=a.generation_id AND g.status='ACTIVE'
			JOIN directory_stations s ON s.id=a.station_id
			WHERE a.source='OSM' AND a.kind IN ('FUEL_STATION','COMBINED_SITE')
			AND a.osm_type=$1 AND a.osm_id=$2
			AND EXISTS (SELECT 1 FROM charging_places WHERE id=$3)
			AND NOT EXISTS (SELECT 1 FROM map_assertion_revocations r WHERE r.assertion_id=a.assertion_id)
			AND NOT EXISTS (SELECT 1 FROM map_generation_revocations r WHERE r.generation_id=g.id)
			AND `+hasANPRegistryIdentity+` LIMIT 2`, object.Type, object.ID, place)
		if err != nil {
			return StationCandidate{}, false, err
		}
		match, count, err := readStationCandidates(rows, ruleCombinedOSM, "", "")
		if err != nil || count != 0 {
			return match, count == 1, err
		}
	}

	name, brand := foldStationName(facts.TradeName), foldStationName(facts.Brand)
	if name == "" && brand == "" {
		return StationCandidate{}, false, nil
	}
	rows, err := db.Query(ctx, `SELECT s.id::text, COALESCE(s.trade_name,''), COALESCE(b.label,'')
		FROM charging_places p
		JOIN directory_stations s ON ST_DWithin(
			s.current_point, ST_SetSRID(ST_MakePoint(p.longitude::float8,p.latitude::float8),4326)::geography,75)
		LEFT JOIN LATERAL (
			SELECT c.proposed_value->>'value' AS label
			FROM station_corrections c
			WHERE $2 <> '' AND c.station_id=s.id AND c.field='brand' AND c.status='accepted'
			AND c.decided_at IS NOT NULL AND jsonb_typeof(c.proposed_value->'value')='string'
			AND EXISTS (SELECT 1 FROM station_correction_decisions d
				WHERE d.correction_id=c.id AND d.decision='accepted' AND d.decider_kind='staff')
			ORDER BY c.decided_at DESC,c.id DESC LIMIT 1
		) b ON TRUE
		WHERE p.id=$1 AND p.longitude::float8 BETWEEN -180 AND 180 AND p.latitude::float8 BETWEEN -90 AND 90
		AND s.current_quality IN ('reviewed','osm-mapped','anp-reported')
		AND (s.trade_name IS NOT NULL OR b.label IS NOT NULL)
		AND `+hasANPRegistryIdentity, place, brand)
	if err != nil {
		return StationCandidate{}, false, err
	}
	match, count, err := readStationCandidates(rows, rulePinTradeName, name, brand)
	return match, count == 1, err
}

func readStationCandidates(rows pgx.Rows, rule, name, brand string) (StationCandidate, int, error) {
	defer rows.Close()
	var candidate StationCandidate
	for rows.Next() {
		var id, tradeName, reviewedBrand string
		if err := rows.Scan(&id, &tradeName, &reviewedBrand); err != nil {
			return StationCandidate{}, 0, err
		}
		matchedRule := rule
		if rule == rulePinTradeName {
			label := foldStationName(tradeName)
			switch {
			case label != "" && (label == name || label == brand):
			case brand != "" && foldStationName(reviewedBrand) == brand:
				matchedRule = rulePinBrand
			default:
				continue
			}
		}
		if candidate.StationID != "" {
			return StationCandidate{}, 2, nil
		}
		candidate = StationCandidate{StationID: id, Rule: matchedRule}
	}
	if err := rows.Err(); err != nil {
		return StationCandidate{}, 0, err
	}
	if candidate.StationID == "" {
		return StationCandidate{}, 0, nil
	}
	return candidate, 1, nil
}

func foldStationName(value string) string {
	var folded strings.Builder
	for _, r := range norm.NFD.String(value) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			folded.WriteRune(unicode.ToLower(r))
		case !unicode.Is(unicode.Mn, r):
			folded.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(folded.String()), " ")
}
