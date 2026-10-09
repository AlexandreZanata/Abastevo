package read

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

// Reader implements application.StationReader.
type Reader struct {
	pool *pgxpool.Pool
}

// NewReader wires the owned generated queries to a pool.
func NewReader(pool *pgxpool.Pool) *Reader {
	return &Reader{pool: pool}
}

// escapeLike neutralizes wildcard input so q stays a substring filter.
func escapeLike(q string) string {
	r := strings.ReplaceAll(q, `\`, `\\`)
	r = strings.ReplaceAll(r, "%", `\%`)
	return strings.ReplaceAll(r, "_", `\_`)
}

// parseWKT decodes POINT(lon lat) to lat/lon. Empty input means no reviewed
// position and yields nil coordinates without error.
func parseWKT(wkt string) (*application.LatLon, error) {
	wkt = strings.TrimSpace(wkt)
	if wkt == "" {
		return nil, nil
	}
	inner, ok := strings.CutPrefix(wkt, "POINT(")
	if !ok || !strings.HasSuffix(inner, ")") {
		return nil, fmt.Errorf("read: bad WKT %q", wkt)
	}
	parts := strings.Fields(inner[:len(inner)-1])
	if len(parts) != 2 {
		return nil, fmt.Errorf("read: bad WKT %q", wkt)
	}
	lon, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil, fmt.Errorf("read: bad WKT %q", wkt)
	}
	lat, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return nil, fmt.Errorf("read: bad WKT %q", wkt)
	}
	return &application.LatLon{Lat: lat, Lon: lon}, nil
}

func mustUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(strings.ReplaceAll(text, "-", ""))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("read: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func wktString(v any) string {
	s, _ := v.(string)
	return s
}

// station maps one row plus its active CNPJ to the public DTO. Coordinates
// surface only when a reviewed point exists; otherwise quality reports
// unknown and coordinates stay null — honest, never fabricated.
func (r *Reader) station(ctx context.Context, q *directory.Queries, id pgtype.UUID, display string, muni, state pgtype.Text, wkt any, quality pgtype.Text, rev pgtype.UUID, address []byte) (application.Station, error) {
	coords, err := parseWKT(wktString(wkt))
	if err != nil {
		return application.Station{}, err
	}
	qualityName := quality.String
	if coords == nil {
		qualityName = domain.QualityUnknown
	}
	var cnpj *string
	if value, err := q.StationCNPJ(ctx, id); err == nil {
		cnpj = &value
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return application.Station{}, err
	}
	var addr map[string]any
	if len(address) > 0 && string(address) != "{}" {
		var m map[string]any
		if jerr := json.Unmarshal(address, &m); jerr == nil && len(m) > 0 {
			addr = m
		}
	}
	st := application.Station{
		ID:               uuidString(id),
		DisplayName:      display,
		CNPJNormalized:   cnpj,
		MunicipalityCode: textPtr(muni),
		State:            textPtr(state),
		LocationQuality:  qualityName,
		Coordinates:      coords,
		Address:          addr,
	}
	if rev.Valid {
		s := uuidString(rev)
		st.CurrentRevisionID = &s
	}
	return st, nil
}

// Search walks stations in ID order with keyset pagination.
func (r *Reader) Search(ctx context.Context, f application.SearchFilter) ([]application.Station, string, error) {
	q := directory.New(r.pool)
	rows, err := q.SearchStations(ctx, directory.SearchStationsParams{
		State:        f.State,
		Municipality: f.Municipality,
		Q:            escapeLike(f.Q),
		AfterID:      f.AfterID,
		LimitPlusOne: int32(f.Limit + 1),
	})
	if err != nil {
		return nil, "", err
	}
	var out []application.Station
	for _, row := range rows {
		st, err := r.station(ctx, q, row.ID, row.DisplayName, row.MunicipalityCode, row.State,
			row.CurrentPointWkt, row.CurrentQuality, row.CurrentRevisionID, row.Address)
		if err != nil {
			return nil, "", err
		}
		out = append(out, st)
	}
	return page(out, f.Limit, func(s application.Station) string { return s.ID })
}

// Nearby walks reviewed positions by distance with keyset pagination.
func (r *Reader) Nearby(ctx context.Context, f application.NearbyFilter) ([]application.NearbyStation, string, error) {
	q := directory.New(r.pool)
	rows, err := q.NearbyStations(ctx, directory.NearbyStationsParams{
		Lon:          f.Lon,
		Lat:          f.Lat,
		RadiusM:      int32(f.RadiusM),
		HasCursor:    f.HasCursor,
		AfterDist:    f.AfterDist,
		AfterID:      f.AfterID,
		LimitPlusOne: int32(f.Limit + 1),
	})
	if err != nil {
		return nil, "", err
	}
	var out []application.NearbyStation
	for _, row := range rows {
		st, err := r.station(ctx, q, row.ID, row.DisplayName, row.MunicipalityCode, row.State,
			row.CurrentPointWkt, row.CurrentQuality, row.CurrentRevisionID, row.Address)
		if err != nil {
			return nil, "", err
		}
		dist, ok := row.DistanceM.(float64)
		if !ok {
			return nil, "", fmt.Errorf("read: bad distance %v", row.DistanceM)
		}
		out = append(out, application.NearbyStation{Station: st, DistanceM: dist})
	}
	return page(out, f.Limit, func(s application.NearbyStation) string {
		return strconv.FormatFloat(s.DistanceM, 'g', -1, 64) + ":" + s.ID
	})
}

// Detail loads one station with its active CNPJ.
func (r *Reader) Detail(ctx context.Context, id string) (application.Station, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.Station{}, err
	}
	q := directory.New(r.pool)
	row, err := q.GetStation(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.Station{}, application.ErrUnknownStation
		}
		return application.Station{}, err
	}
	return r.station(ctx, q, row.ID, row.DisplayName, row.MunicipalityCode, row.State,
		row.CurrentPointWkt, row.CurrentQuality, row.CurrentRevisionID, row.Address)
}

// page trims limit+1 rows to a page plus the next sort key.
func page[T any](items []T, limit int, key func(T) string) ([]T, string, error) {
	if len(items) <= limit {
		if items == nil {
			items = []T{}
		}
		return items, "", nil
	}
	return items[:limit], key(items[limit-1]), nil
}

// ByCNPJ resolves only an existing active identifier; it never runs intake.
func (r *Reader) ByCNPJ(ctx context.Context, raw string) (application.Station, error) {
	cnpj, err := kernel.ParseCNPJ(raw)
	if err != nil {
		return application.Station{}, application.ErrInvalidFilter
	}
	q := directory.New(r.pool)
	row, err := q.ResolveActiveIdentifier(ctx, directory.ResolveActiveIdentifierParams{Kind: "CNPJ", Value: cnpj.Normalized()})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.Station{}, application.ErrUnknownStation
	}
	if err != nil {
		return application.Station{}, err
	}
	station, err := r.Detail(ctx, uuidString(row.ID))
	if err != nil {
		return application.Station{}, err
	}
	if station.CNPJNormalized == nil || *station.CNPJNormalized != cnpj.Normalized() {
		return application.Station{}, application.ErrUnknownStation
	}
	return station, nil
}
