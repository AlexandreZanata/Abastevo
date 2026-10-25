// Package mapdata owns public map-overlay generations, exact source-specific
// assertions and bounded spatial reads. It never stores private account state
// or native map packages.
package mapdata

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/mapcontract"
)

var (
	ErrNoActiveGeneration = errors.New("mapdata: no active generation")
	ErrStaleGeneration    = errors.New("mapdata: generation changed")
	ErrRevokedGeneration  = errors.New("mapdata: generation revoked")
	ErrInvalidQuery       = errors.New("mapdata: invalid query")
	ErrUnknownAssertion   = errors.New("mapdata: unknown assertion")
)

const (
	MaxViewportAreaDeg2 = 25.0
	MaxLimit            = 100
)

type Generation struct {
	ID             string
	Key            string
	SourceRevision string
	Status         string
	ActivatedAt    time.Time
}

type Query struct {
	MinLat  float64
	MinLon  float64
	MaxLat  float64
	MaxLon  float64
	Source  string
	Product string
	Limit   int
	AfterID string
}

type Price struct {
	Source         string  `json:"source"`
	Product        string  `json:"product"`
	Unit           string  `json:"unit"`
	AmountMilliBRL int64   `json:"amount_milli_brl"`
	ObservedAt     string  `json:"observed_at"`
	PeriodStart    *string `json:"period_start"`
	PeriodEnd      *string `json:"period_end"`
}

type Coordinates struct {
	LatitudeDeg  string `json:"latitude_deg"`
	LongitudeDeg string `json:"longitude_deg"`
}

type OverlayAssertion struct {
	AssertionID  string          `json:"assertion_id"`
	Source       string          `json:"source"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	StationUUID  *string         `json:"station_uuid"`
	CNPJ         *string         `json:"cnpj"`
	PlaceID      *string         `json:"place_id"`
	EquipmentID  *string         `json:"equipment_id"`
	ConnectorID  *string         `json:"connector_id"`
	OSMType      *string         `json:"osm_type"`
	OSMID        *int64          `json:"osm_id"`
	Quality      string          `json:"location_quality"`
	DisplayPoint *string         `json:"display_point"`
	Coordinates  *Coordinates    `json:"coordinates"`
	Entrance     *Coordinates    `json:"entrance,omitempty"`
	Services     []string        `json:"services"`
	Prices       []Price         `json:"prices"`
	Charging     json.RawMessage `json:"charging,omitempty"`
	Clocks       json.RawMessage `json:"clocks"`
	Rights       json.RawMessage `json:"rights"`
	Capabilities json.RawMessage `json:"capabilities"`
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32], nil
}

// Publish validates one complete public overlay and activates it atomically.
// Replaying identical content is idempotent. Any failure rolls back to the
// previously active generation.
func (s *Store) Publish(ctx context.Context, envelopeBytes, assertionsBytes []byte) (Generation, error) {
	envelope, corpus, err := mapcontract.ParsePreparedOverlay(envelopeBytes, assertionsBytes)
	if err != nil {
		return Generation{}, err
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Generation{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('map_generation_publish'))"); err != nil {
		return Generation{}, err
	}
	var existing Generation
	err = tx.QueryRow(ctx, `SELECT id::text, generation_key, source_revision, status, activated_at
		FROM map_generations WHERE content_sha256 = $1 ORDER BY created_at LIMIT 1`, envelope.ContentSHA256).
		Scan(&existing.ID, &existing.Key, &existing.SourceRevision, &existing.Status, &existing.ActivatedAt)
	if err == nil {
		if existing.Status == "REVOKED" {
			return Generation{}, ErrRevokedGeneration
		}
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Generation{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE map_generations SET status = 'SUPERSEDED', superseded_at = now()
		WHERE status = 'ACTIVE'`); err != nil {
		return Generation{}, err
	}
	id, err := newUUID()
	if err != nil {
		return Generation{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO map_generations
		(id, generation_key, format, product_class, visibility, profile_id, profile_sha256, rights_sha256,
		 source_revision, assertions_sha256, content_sha256, input_count, accepted_count, duplicate_count,
		 quarantined_count, status, activated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'ACTIVE',now())`,
		id, corpus.Generation, envelope.Format, envelope.ProductClass, envelope.Visibility,
		envelope.ProfileID, envelope.ProfileSHA256, envelope.RightsSHA256, envelope.SourceRevision,
		envelope.Artifacts["assertions"].SHA256, envelope.ContentSHA256, envelope.Counts.Input,
		envelope.Counts.Accepted, envelope.Counts.Duplicates, envelope.Counts.Quarantined); err != nil {
		return Generation{}, err
	}
	for i := range corpus.Assertions {
		if err := insertAssertion(ctx, tx, id, &corpus.Assertions[i]); err != nil {
			return Generation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Generation{}, err
	}
	return Generation{ID: id, Key: corpus.Generation, SourceRevision: envelope.SourceRevision, Status: "ACTIVE", ActivatedAt: time.Now().UTC()}, nil
}

func insertAssertion(ctx context.Context, tx pgx.Tx, generationID string, a *mapcontract.Assertion) error {
	var lat, lon, entLat, entLon *float64
	if a.Location.LatitudeDeg != nil {
		v, err := strconv.ParseFloat(*a.Location.LatitudeDeg, 64)
		if err != nil {
			return err
		}
		lat = &v
	}
	if a.Location.LongitudeDeg != nil {
		v, err := strconv.ParseFloat(*a.Location.LongitudeDeg, 64)
		if err != nil {
			return err
		}
		lon = &v
	}
	if a.Location.EntranceLatitudeDeg != nil {
		v, err := strconv.ParseFloat(*a.Location.EntranceLatitudeDeg, 64)
		if err != nil {
			return err
		}
		entLat = &v
	}
	if a.Location.EntranceLongitudeDeg != nil {
		v, err := strconv.ParseFloat(*a.Location.EntranceLongitudeDeg, 64)
		if err != nil {
			return err
		}
		entLon = &v
	}
	var point any
	if lat != nil && lon != nil {
		point = fmt.Sprintf("SRID=4326;POINT(%.7f %.7f)", *lon, *lat)
	}
	var charging any
	if a.Charging != nil {
		raw, err := json.Marshal(a.Charging)
		if err != nil {
			return err
		}
		charging = string(raw)
	}
	clocks, _ := json.Marshal(a.Clocks)
	rights, _ := json.Marshal(a.Rights)
	capabilities, _ := json.Marshal(a.Capabilities)
	rawAssertion, _ := json.Marshal(a)
	sum := sha256.Sum256(rawAssertion)
	stationID := a.Identities.StationUUID
	if _, err := tx.Exec(ctx, `INSERT INTO map_assertions
		(generation_id, assertion_id, source, kind, name, station_id, cnpj, place_id, equipment_id, connector_id,
		 osm_type, osm_id, latitude_deg, longitude_deg, point, quality, display_point,
		 entrance_latitude_deg, entrance_longitude_deg, services, charging, clocks, rights, capabilities, assertion_sha256)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,
			CASE WHEN $15::text IS NULL THEN NULL ELSE ST_GeogFromText($15::text) END,
			$16,$17,$18,$19,$20,$21::jsonb,$22::jsonb,$23::jsonb,$24::jsonb,$25)`,
		generationID, a.ID, a.Source, a.Kind, a.Name, stationID, a.Identities.CNPJ,
		a.Identities.PlaceID, a.Identities.EquipmentID, a.Identities.ConnectorID,
		a.Identities.OSMType, a.Identities.OSMID, lat, lon, point,
		a.Location.Quality, a.Location.DisplayPoint, entLat, entLon, a.Services,
		charging, string(clocks), string(rights), string(capabilities), hex.EncodeToString(sum[:])); err != nil {
		return err
	}
	for i := range a.Prices {
		p := &a.Prices[i]
		observed, err := time.Parse("2006-01-02", p.ObservedAt)
		if err != nil {
			return err
		}
		var periodStart, periodEnd any
		if p.PeriodStart != nil {
			periodStart, err = time.Parse("2006-01-02", *p.PeriodStart)
			if err != nil {
				return err
			}
		}
		if p.PeriodEnd != nil {
			periodEnd, err = time.Parse("2006-01-02", *p.PeriodEnd)
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO map_assertion_prices
			(generation_id, assertion_id, source, station_id, product, unit, amount_milli_brl,
			 observed_at, period_start, period_end)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			generationID, a.ID, p.Source, p.StationUUID, p.Product, p.Unit, p.AmountMilliBRL,
			observed, periodStart, periodEnd); err != nil {
			return err
		}
	}
	return nil
}

// ActiveGeneration returns the only generation safe to serve.
func (s *Store) ActiveGeneration(ctx context.Context) (Generation, error) {
	var g Generation
	err := s.Pool.QueryRow(ctx, `SELECT id::text, generation_key, source_revision, status, activated_at
		FROM map_generations WHERE status = 'ACTIVE'`).Scan(&g.ID, &g.Key, &g.SourceRevision, &g.Status, &g.ActivatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Generation{}, ErrNoActiveGeneration
	}
	return g, err
}

// Query returns one bounded page of pin-eligible assertions. Prices are
// projected only for the requested source and optional product.
func (s *Store) Query(ctx context.Context, generationID string, q Query) ([]OverlayAssertion, string, error) {
	if q.Limit < 1 || q.Limit > MaxLimit || (q.Source != "ANP" && q.Source != "COMMUNITY") ||
		math.IsNaN(q.MinLat) || math.IsNaN(q.MinLon) || math.IsNaN(q.MaxLat) || math.IsNaN(q.MaxLon) ||
		math.IsInf(q.MinLat, 0) || math.IsInf(q.MinLon, 0) || math.IsInf(q.MaxLat, 0) || math.IsInf(q.MaxLon, 0) ||
		q.MaxLat <= q.MinLat || q.MaxLon <= q.MinLon ||
		q.MinLat < -90 || q.MaxLat > 90 || q.MinLon < -180 || q.MaxLon > 180 {
		return nil, "", ErrInvalidQuery
	}
	var active bool
	if err := s.Pool.QueryRow(ctx, `SELECT status = 'ACTIVE' FROM map_generations WHERE id = $1`, generationID).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrStaleGeneration
		}
		return nil, "", err
	}
	if !active {
		return nil, "", ErrStaleGeneration
	}
	after := strings.TrimSpace(q.AfterID)
	rows, err := s.Pool.Query(ctx, `SELECT a.assertion_id::text, a.source, a.kind, a.name,
		a.station_id::text, a.cnpj, a.place_id::text, a.equipment_id::text, a.connector_id::text,
		a.osm_type, a.osm_id, a.latitude_deg::text, a.longitude_deg::text, a.quality, a.display_point,
		a.entrance_latitude_deg::text, a.entrance_longitude_deg::text, a.services, a.charging,
		a.clocks, a.rights, a.capabilities,
		COALESCE((
			SELECT jsonb_agg(jsonb_build_object(
				'source', p.source, 'product', p.product, 'unit', p.unit,
				'amount_milli_brl', p.amount_milli_brl, 'observed_at', p.observed_at::text,
				'period_start', p.period_start::text, 'period_end', p.period_end::text)
				ORDER BY p.product, p.unit)
			FROM map_assertion_prices p
			WHERE p.generation_id = a.generation_id AND p.assertion_id = a.assertion_id
			  AND p.source = $2 AND ($3 = '' OR p.product = $3)
		), '[]'::jsonb)
		FROM map_assertions a
		WHERE a.generation_id = $1
		  AND a.point IS NOT NULL
		  AND a.capabilities->>'pin_eligible' = 'true'
		  AND a.point && ST_MakeEnvelope($4, $5, $6, $7, 4326)::geography
		  AND (a.source = $2 OR a.kind IN ('CHARGING_SITE', 'REFERENCE_POI') OR a.charging IS NOT NULL)
		  AND NOT EXISTS (SELECT 1 FROM map_assertion_revocations r WHERE r.assertion_id = a.assertion_id)
		  AND ($8 = '' OR a.assertion_id > NULLIF($8, '')::uuid)
		ORDER BY a.assertion_id
		LIMIT $9`, generationID, q.Source, q.Product, q.MinLon, q.MinLat, q.MaxLon, q.MaxLat, after, q.Limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]OverlayAssertion, 0, q.Limit+1)
	for rows.Next() {
		var item OverlayAssertion
		var station, cnpj, place, equipment, connector, osmType, display *string
		var osmID *int64
		var lat, lon, entLat, entLon *string
		var pricesJSON, charging, clocks, rights, capabilities []byte
		if err := rows.Scan(&item.AssertionID, &item.Source, &item.Kind, &item.Name,
			&station, &cnpj, &place, &equipment, &connector, &osmType, &osmID,
			&lat, &lon, &item.Quality, &display, &entLat, &entLon, &item.Services,
			&charging, &clocks, &rights, &capabilities, &pricesJSON); err != nil {
			return nil, "", err
		}
		item.StationUUID, item.CNPJ, item.PlaceID, item.EquipmentID, item.ConnectorID = station, cnpj, place, equipment, connector
		item.OSMType, item.OSMID, item.DisplayPoint = osmType, osmID, display
		if lat != nil && lon != nil {
			item.Coordinates = &Coordinates{LatitudeDeg: *lat, LongitudeDeg: *lon}
		}
		if entLat != nil && entLon != nil {
			item.Entrance = &Coordinates{LatitudeDeg: *entLat, LongitudeDeg: *entLon}
		}
		if len(charging) > 0 {
			item.Charging = charging
		}
		item.Clocks, item.Rights = clocks, rights
		if err := json.Unmarshal(pricesJSON, &item.Prices); err != nil {
			return nil, "", err
		}
		item.Capabilities = projectCapabilities(capabilities, len(item.Prices) > 0)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > q.Limit {
		items = items[:q.Limit]
		next = items[len(items)-1].AssertionID
	}
	return items, next, nil
}

// RevokeAssertion makes one source record permanently unservable without
// deleting its historical generations.
func (s *Store) RevokeAssertion(ctx context.Context, assertionID, reason, sourceReference string) error {
	if !validUUID(assertionID) || strings.TrimSpace(reason) == "" || len(reason) > 500 ||
		strings.TrimSpace(sourceReference) == "" || len(sourceReference) > 256 {
		return ErrInvalidQuery
	}
	id, err := newUUID()
	if err != nil {
		return err
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM map_assertions WHERE assertion_id = $1)`, assertionID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrUnknownAssertion
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO map_assertion_revocations (id, assertion_id, reason, source_reference)
		SELECT $1, $2, $3, $4 WHERE NOT EXISTS (
			SELECT 1 FROM map_assertion_revocations WHERE assertion_id = $2)`, id, assertionID, reason, sourceReference)
	return err
}

// RevokeGeneration is the kill switch for one active/staged generation.
func (s *Store) RevokeGeneration(ctx context.Context, generationID, reason, sourceReference string) error {
	if !validUUID(generationID) || strings.TrimSpace(reason) == "" || len(reason) > 500 ||
		strings.TrimSpace(sourceReference) == "" || len(sourceReference) > 256 {
		return ErrInvalidQuery
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE map_generations SET status = 'REVOKED', revoked_at = now()
		WHERE id = $1 AND status <> 'REVOKED'`, generationID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrStaleGeneration
	}
	id, err := newUUID()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO map_generation_revocations
		(id, generation_id, reason, source_reference) VALUES ($1,$2,$3,$4)`,
		id, generationID, reason, sourceReference); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// projectCapabilities keeps price_known aligned with the source-projected
// prices, not the unfiltered assertion. A community price hidden in ANP mode
// must not be reported as a known ANP price.
func projectCapabilities(raw []byte, hasPrices bool) []byte {
	var capabilities map[string]any
	if err := json.Unmarshal(raw, &capabilities); err != nil {
		return raw
	}
	capabilities["price_known"] = hasPrices
	projected, err := json.Marshal(capabilities)
	if err != nil {
		return raw
	}
	return projected
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
	}
	return true
}
