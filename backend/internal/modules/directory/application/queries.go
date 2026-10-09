package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Bounds from API_PLAN.
const (
	MinRadiusM = 100
	MaxRadiusM = 15000
	MinQuery   = 2
	MaxQuery   = 100
)

var (
	ErrInvalidFilter  = errors.New("directory: invalid search filter")
	ErrOutOfBounds    = errors.New("directory: coordinates or radius out of bounds")
	ErrUnknownStation = errors.New("directory: unknown station")
	ErrBadUUID        = errors.New("directory: malformed station id")
)

// Station is the public directory DTO. Coordinates is nil unless the
// projection is reviewed; a nil here is honesty, never a fabrication.
type Station struct {
	ID                string         `json:"station_id"`
	DisplayName       string         `json:"display_name"`
	CNPJNormalized    *string        `json:"cnpj_normalized"`
	Address           map[string]any `json:"address"`
	MunicipalityCode  *string        `json:"municipality_code"`
	State             *string        `json:"state"`
	LocationQuality   string         `json:"location_quality"`
	Coordinates       *LatLon        `json:"coordinates"`
	CurrentRevisionID *string        `json:"current_revision_id"`
	// Official carries the latest complete official anchor for this
	// establishment, or nil when none exists. Community-only stations
	// stay distinguishable: a nil here never borrows official trust,
	// and official rows never absorb community reports.
	Official *OfficialAnchor `json:"official"`
}

// OfficialAnchor is the source-separated official provenance of a
// station: which official source last evidenced it, and when. The
// timestamp is the freshness signal; readers must not infer liveness
// beyond it.
type OfficialAnchor struct {
	Source           string     `json:"source"`
	DisplayName      string     `json:"display_name"`
	MunicipalityCode *string    `json:"municipality_code"`
	State            *string    `json:"state"`
	FinishedAt       *time.Time `json:"finished_at"`
}

// LatLon is a public reviewed position.
type LatLon struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// NearbyStation adds the public distance in metres.
type NearbyStation struct {
	Station
	DistanceM float64 `json:"distance_m"`
}

// StationReader is the owned read port adapters implement.
type StationReader interface {
	Search(ctx context.Context, f SearchFilter) ([]Station, string, error)
	Nearby(ctx context.Context, f NearbyFilter) ([]NearbyStation, string, error)
	Detail(ctx context.Context, id string) (Station, error)
	ByCNPJ(ctx context.Context, cnpj string) (Station, error)
}

// SearchFilter carries validated search input. AfterID is the opaque sort
// position from a previous cursor ("" on first page).
type SearchFilter struct {
	State        string
	Municipality string
	Q            string
	Limit        int
	AfterID      string
}

// ValidateSearch enforces allowlisted filters: fixed shapes only, no sort
// expressions, q length bounds.
func ValidateSearch(state, municipality, q string, limit int, afterID string) (SearchFilter, error) {
	if len(state) != 0 && len(state) != 2 {
		return SearchFilter{}, fmt.Errorf("%w: state", ErrInvalidFilter)
	}
	if len(municipality) > 16 {
		return SearchFilter{}, fmt.Errorf("%w: municipality", ErrInvalidFilter)
	}
	if q != "" && (len([]rune(q)) < MinQuery || len([]rune(q)) > MaxQuery) {
		return SearchFilter{}, fmt.Errorf("%w: q", ErrInvalidFilter)
	}
	if limit < 1 || limit > 100 {
		return SearchFilter{}, fmt.Errorf("%w: limit", ErrInvalidFilter)
	}
	if afterID != "" {
		if !isUUID(afterID) {
			return SearchFilter{}, fmt.Errorf("%w: cursor", ErrInvalidFilter)
		}
	}
	return SearchFilter{
		State:        strings.ToUpper(state),
		Municipality: municipality,
		Q:            q,
		Limit:        limit,
		AfterID:      afterID,
	}, nil
}

// NearbyFilter carries a validated request position. The raw coordinate
// never leaves this struct toward logs or caches; only distances and
// public station positions flow out.
type NearbyFilter struct {
	Lat       float64
	Lon       float64
	RadiusM   int
	Limit     int
	AfterDist float64
	AfterID   string
	HasCursor bool
}

// ValidateNearby enforces lat/lon/radius bounds from API_PLAN.
func ValidateNearby(lat, lon float64, radiusM, limit int, lastKey string) (NearbyFilter, error) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return NearbyFilter{}, fmt.Errorf("%w: lat/lon", ErrOutOfBounds)
	}
	if radiusM < MinRadiusM || radiusM > MaxRadiusM {
		return NearbyFilter{}, fmt.Errorf("%w: radius_m", ErrOutOfBounds)
	}
	if limit < 1 || limit > 100 {
		return NearbyFilter{}, fmt.Errorf("%w: limit", ErrInvalidFilter)
	}
	f := NearbyFilter{Lat: lat, Lon: lon, RadiusM: radiusM, Limit: limit}
	if lastKey != "" {
		dist, id, ok := strings.Cut(lastKey, ":")
		d, err := strconv.ParseFloat(dist, 64)
		if !ok || err != nil || d < 0 || !isUUID(id) {
			return NearbyFilter{}, fmt.Errorf("%w: cursor", ErrInvalidFilter)
		}
		f.AfterDist, f.AfterID, f.HasCursor = d, id, true
	}
	return f, nil
}

// ValidateStationID accepts canonical UUID text only.
func ValidateStationID(id string) (string, error) {
	if !isUUID(id) {
		return "", ErrBadUUID
	}
	return strings.ToLower(id), nil
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if r != '-' {
				return false
			}
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
