package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Route plans, device route computations and routing adapter boundaries
// (EMP-ROUTE-001..003, BR-ROUTE-001..010, BR-ROUTE-023..024).
type RouteOwnerKind string

const (
	RouteOwnerMission RouteOwnerKind = "MISSION"
	RouteOwnerTrip    RouteOwnerKind = "TRIP"
)

func ParseRouteOwnerKind(raw string) (RouteOwnerKind, error) {
	switch RouteOwnerKind(raw) {
	case RouteOwnerMission, RouteOwnerTrip:
		return RouteOwnerKind(raw), nil
	}
	return "", ErrRouteValidation
}

type WaypointSource string

const (
	WaypointOrgPlace       WaypointSource = "ORGANIZATION_PLACE"
	WaypointStationCatalog WaypointSource = "STATION_CATALOG"
	WaypointOfflineMap     WaypointSource = "OFFLINE_MAP_SEARCH"
	WaypointCoordinates    WaypointSource = "COORDINATES"
)

func ParseWaypointSource(raw string) (WaypointSource, error) {
	switch WaypointSource(raw) {
	case WaypointOrgPlace, WaypointStationCatalog, WaypointOfflineMap, WaypointCoordinates:
		return WaypointSource(raw), nil
	}
	return "", ErrRouteValidation
}

type PlannedDistanceSource string

const (
	PlannedDistanceManual PlannedDistanceSource = "MANUAL"
	PlannedDistanceEngine PlannedDistanceSource = "ENGINE"
)

func ParsePlannedDistanceSource(raw string) (PlannedDistanceSource, error) {
	switch PlannedDistanceSource(raw) {
	case PlannedDistanceManual, PlannedDistanceEngine:
		return PlannedDistanceSource(raw), nil
	}
	return "", ErrRouteValidation
}

type RouteComputationKind string

const (
	ComputationPlanning      RouteComputationKind = "PLANNING"
	ComputationDeparture     RouteComputationKind = "DEPARTURE"
	ComputationRecalculation RouteComputationKind = "RECALCULATION"
)

func ParseRouteComputationKind(raw string) (RouteComputationKind, error) {
	switch RouteComputationKind(raw) {
	case ComputationPlanning, ComputationDeparture, ComputationRecalculation:
		return RouteComputationKind(raw), nil
	}
	return "", ErrRouteValidation
}

type RouteProfile string

const (
	RouteProfileCar RouteProfile = "CAR"
)

const (
	MinRouteWaypoints        = 2
	MaxRouteWaypoints        = 102 // Origin + up to 100 intermediate stops + destination
	MaxRouteGeometryPoints   = 1000
	MaxRouteDistanceMetres   = int64(100_000_000)
	MaxRouteDurationSeconds  = int64(10_000_000)
	OpenStreetMapAttribution = "© OpenStreetMap contributors"
)

var (
	ErrRoutePointLimit          = errors.New("tracking: route point limit exceeded")
	ErrRouteValidation          = errors.New("tracking: route plan validation failed")
	ErrRouteProfileUnsupported  = errors.New("tracking: route profile unsupported")
	ErrServerRoutingUnavailable = errors.New("tracking: server routing adapter unavailable")
)

type RouteWaypoint struct {
	Sequence  int            `json:"sequence"`
	Label     string         `json:"label"`
	Source    WaypointSource `json:"source"`
	PlaceID   string         `json:"place_id,omitempty"`
	StationID string         `json:"station_id,omitempty"`
	Note      string         `json:"note,omitempty"`
	Lat       int32          `json:"lat_e7"`
	Lon       int32          `json:"lon_e7"`
}

type RouteOptions struct {
	AvoidToll     bool `json:"avoid_toll"`
	AvoidMotorway bool `json:"avoid_motorway"`
	AvoidFerry    bool `json:"avoid_ferry"`
	AvoidUnpaved  bool `json:"avoid_unpaved"`
}

type RoutePlan struct {
	redacted
	ID                    string
	OwnerID               string
	OwnerKind             RouteOwnerKind
	Revision              int64
	Waypoints             []RouteWaypoint
	Options               RouteOptions
	PlannedDistanceMetres *int64
	PlannedDistanceSource *PlannedDistanceSource
	ComputationID         string
	HeavyAssetNotice      bool
	CreatedAt             time.Time
}

type RouteComputation struct {
	redacted
	ID              string
	OwnerID         string
	OwnerKind       RouteOwnerKind
	PlanRevision    int64
	EngineID        string
	MapDataVersion  string
	Profile         RouteProfile
	Kind            RouteComputationKind
	Options         RouteOptions
	DistanceMetres  int64
	DurationSeconds int64
	Line            []Point
	Attribution     string
	ComputedAt      time.Time
}

func validRouteText(s string, maxRunes int, required bool) bool {
	trimmed := strings.TrimSpace(s)
	if required && trimmed == "" {
		return false
	}
	if len([]rune(trimmed)) > maxRunes {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '\u202e' || r == '\u202d' || r == '\u202a' || r == '\u202b' || r == '\u202c' {
			return false
		}
	}
	return true
}

func canonicalUUID(s string) bool {
	id, err := kernel.ParseOrgID(s)
	return err == nil && id.String() == s
}

// ValidateRoutePlanInput enforces BR-ROUTE-001..006 and §3 invariants.
func ValidateRoutePlanInput(ownerKind RouteOwnerKind, ownerID string, revision int64, waypoints []RouteWaypoint, options RouteOptions, plannedDistanceMetres *int64, plannedDistanceSource *PlannedDistanceSource, computationID string) (RoutePlan, error) {
	if _, err := ParseRouteOwnerKind(string(ownerKind)); err != nil || !canonicalUUID(ownerID) || revision < 1 {
		return RoutePlan{}, ErrRouteValidation
	}
	if len(waypoints) > MaxRouteWaypoints {
		return RoutePlan{}, ErrRoutePointLimit
	}
	if len(waypoints) < MinRouteWaypoints {
		return RoutePlan{}, ErrRouteValidation
	}
	type coord struct{ lat, lon int32 }
	seenCoords := make(map[coord]bool, len(waypoints))
	normalized := make([]RouteWaypoint, len(waypoints))
	for i, wp := range waypoints {
		if !validRouteText(wp.Label, 160, true) || !validRouteText(wp.Note, 280, false) {
			return RoutePlan{}, ErrRouteValidation
		}
		if _, err := ParseWaypointSource(string(wp.Source)); err != nil {
			return RoutePlan{}, ErrRouteValidation
		}
		if !validPoint(Point{Lat: wp.Lat, Lon: wp.Lon}) {
			return RoutePlan{}, ErrRouteValidation
		}
		c := coord{wp.Lat, wp.Lon}
		if seenCoords[c] {
			return RoutePlan{}, ErrRouteValidation
		}
		seenCoords[c] = true
		if wp.PlaceID != "" && !canonicalUUID(wp.PlaceID) {
			return RoutePlan{}, ErrRouteValidation
		}
		if wp.StationID != "" && !canonicalUUID(wp.StationID) {
			return RoutePlan{}, ErrRouteValidation
		}
		if wp.Source == WaypointOrgPlace && wp.PlaceID == "" {
			return RoutePlan{}, ErrRouteValidation
		}
		if wp.Source == WaypointStationCatalog && wp.StationID == "" {
			return RoutePlan{}, ErrRouteValidation
		}
		wp.Sequence = i + 1
		wp.Label = strings.TrimSpace(wp.Label)
		wp.Note = strings.TrimSpace(wp.Note)
		normalized[i] = wp
	}

	var distCopy *int64
	var srcCopy *PlannedDistanceSource
	if plannedDistanceMetres == nil && plannedDistanceSource != nil {
		return RoutePlan{}, ErrRouteValidation
	}
	if plannedDistanceMetres != nil {
		if *plannedDistanceMetres <= 0 || *plannedDistanceMetres > MaxRouteDistanceMetres || plannedDistanceSource == nil {
			return RoutePlan{}, ErrRouteValidation
		}
		parsedSrc, err := ParsePlannedDistanceSource(string(*plannedDistanceSource))
		if err != nil {
			return RoutePlan{}, ErrRouteValidation
		}
		if parsedSrc == PlannedDistanceEngine && !canonicalUUID(computationID) {
			return RoutePlan{}, ErrRouteValidation
		}
		d := *plannedDistanceMetres
		s := parsedSrc
		distCopy = &d
		srcCopy = &s
	} else if computationID != "" && !canonicalUUID(computationID) {
		return RoutePlan{}, ErrRouteValidation
	}

	return RoutePlan{
		OwnerID:               ownerID,
		OwnerKind:             ownerKind,
		Revision:              revision,
		Waypoints:             normalized,
		Options:               options,
		PlannedDistanceMetres: distCopy,
		PlannedDistanceSource: srcCopy,
		ComputationID:         computationID,
	}, nil
}

// ValidateRouteComputation enforces BR-ROUTE-005, BR-ROUTE-009 and BR-ROUTE-010.
func ValidateRouteComputation(comp RouteComputation) (RouteComputation, error) {
	if _, err := ParseRouteOwnerKind(string(comp.OwnerKind)); err != nil || !canonicalUUID(comp.OwnerID) || comp.PlanRevision < 1 {
		return RouteComputation{}, ErrRouteValidation
	}
	if _, err := ParseRouteComputationKind(string(comp.Kind)); err != nil {
		return RouteComputation{}, ErrRouteValidation
	}
	if comp.Profile == "" {
		comp.Profile = RouteProfileCar
	}
	if comp.Profile != RouteProfileCar {
		return RouteComputation{}, ErrRouteProfileUnsupported
	}
	if !validRouteText(comp.EngineID, 64, true) || !validRouteText(comp.MapDataVersion, 64, true) ||
		comp.DistanceMetres < 0 || comp.DistanceMetres > MaxRouteDistanceMetres ||
		comp.DurationSeconds < 0 || comp.DurationSeconds > MaxRouteDurationSeconds ||
		comp.ComputedAt.IsZero() || len(comp.Line) > MaxRouteGeometryPoints {
		return RouteComputation{}, ErrRouteValidation
	}
	for _, p := range comp.Line {
		if !validPoint(p) {
			return RouteComputation{}, ErrRouteValidation
		}
	}
	comp.EngineID = strings.TrimSpace(comp.EngineID)
	comp.MapDataVersion = strings.TrimSpace(comp.MapDataVersion)
	comp.ComputedAt = comp.ComputedAt.UTC()
	if comp.Attribution == "" {
		comp.Attribution = OpenStreetMapAttribution
	}
	return comp, nil
}
