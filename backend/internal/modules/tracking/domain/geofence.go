package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Server-side geofences (EMP-GPS-013, BR-GPS-037..039, VAL-30). Evaluated
// strictly on received precise fixes; coarse or inaccurate fixes never certify
// crossings.
type GeofenceKind string

const (
	GeofenceCircle  GeofenceKind = "CIRCLE"
	GeofencePolygon GeofenceKind = "POLYGON"
)

type GeofenceEventKind string

const (
	GeofenceEnter GeofenceEventKind = "ENTER"
	GeofenceExit  GeofenceEventKind = "EXIT"
	GeofenceDwell GeofenceEventKind = "DWELL"
)

const (
	MinCircleRadiusM           int32 = 50
	MaxCircleRadiusM           int32 = 50_000
	MinPolygonVertices               = 3
	MaxPolygonVertices               = 100
	MinDwellSeconds            int32 = 60
	MaxDwellSeconds            int32 = 86_400
	MaxGeofenceRecipients            = 50
	LateGeofenceAlertThreshold       = 30 * time.Minute
)

var ErrGeofenceInvalidGeometry = errors.New("tracking: invalid geofence geometry")

// GeofenceSpec defines a circle or polygon geofence scoped to an organization
// and optionally to a unit.
type GeofenceSpec struct {
	redacted
	ID           string
	Name         string
	UnitID       string
	Kind         GeofenceKind
	CenterLat    int32
	CenterLon    int32
	RadiusMetres int32
	Vertices     []Point
	OnEnter      bool
	OnExit       bool
	DwellSeconds int32
	Recipients   []string
	Enabled      bool
}

func validGeofenceName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 || len([]rune(trimmed)) > 160 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == '\u202e' || r == '\u202d' || r == '\u202a' || r == '\u202b' || r == '\u202c' {
			return false
		}
	}
	return true
}

// Validate checks geometry invariants, rule bounds and recipient identities.
func (g GeofenceSpec) Validate() error {
	if !validGeofenceName(g.Name) {
		return ErrInvalid
	}
	if g.UnitID != "" {
		id, err := kernel.ParseOrgID(g.UnitID)
		if err != nil || id.String() != g.UnitID {
			return ErrInvalid
		}
	}
	if !g.OnEnter && !g.OnExit && g.DwellSeconds == 0 {
		return ErrInvalid
	}
	if g.DwellSeconds != 0 && (g.DwellSeconds < MinDwellSeconds || g.DwellSeconds > MaxDwellSeconds) {
		return ErrInvalid
	}
	if len(g.Recipients) > MaxGeofenceRecipients {
		return ErrInvalid
	}
	seenRecipients := make(map[string]bool, len(g.Recipients))
	for _, r := range g.Recipients {
		id, err := kernel.ParseOrgID(r)
		if err != nil || id.String() != r || seenRecipients[r] {
			return ErrInvalid
		}
		seenRecipients[r] = true
	}

	switch g.Kind {
	case GeofenceCircle:
		if !validPoint(Point{Lat: g.CenterLat, Lon: g.CenterLon}) ||
			g.RadiusMetres < MinCircleRadiusM || g.RadiusMetres > MaxCircleRadiusM ||
			len(g.Vertices) != 0 {
			return ErrGeofenceInvalidGeometry
		}
	case GeofencePolygon:
		if len(g.Vertices) < MinPolygonVertices || len(g.Vertices) > MaxPolygonVertices {
			return ErrGeofenceInvalidGeometry
		}
		for _, v := range g.Vertices {
			if !validPoint(v) {
				return ErrGeofenceInvalidGeometry
			}
		}
		if !validPolygon(g.Vertices) {
			return ErrGeofenceInvalidGeometry
		}
	default:
		return ErrGeofenceInvalidGeometry
	}
	return nil
}

// validPolygon checks non-zero area and absence of self-intersections or
// duplicate consecutive vertices on a local equirectangular projection.
func validPolygon(verts []Point) bool {
	n := len(verts)
	const k = math.Pi / 180 / 1e7
	cos := math.Cos(float64(verts[0].Lat) * k)
	xy := make([][2]float64, n)
	for i, p := range verts {
		xy[i] = [2]float64{
			float64(p.Lon-verts[0].Lon) * k * cos * EarthRadiusMetres,
			float64(p.Lat-verts[0].Lat) * k * EarthRadiusMetres,
		}
	}
	area := 0.0
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		if xy[i][0] == xy[j][0] && xy[i][1] == xy[j][1] {
			return false
		}
		area += xy[i][0]*xy[j][1] - xy[j][0]*xy[i][1]
	}
	if math.Abs(area) < 1.0 {
		return false
	}
	for i := 0; i < n; i++ {
		a1, a2 := xy[i], xy[(i+1)%n]
		for j := i + 1; j < n; j++ {
			b1, b2 := xy[j], xy[(j+1)%n]
			// Adjacent edges share a vertex; check only non-adjacent pairs.
			if (i+1)%n == j || (j+1)%n == i {
				continue
			}
			if segmentsIntersect(a1, a2, b1, b2) {
				return false
			}
		}
	}
	return true
}

func cross(o, a, b [2]float64) float64 {
	return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
}

func onSegment(a, b, p [2]float64) bool {
	return math.Min(a[0], b[0])-1e-9 <= p[0] && p[0] <= math.Max(a[0], b[0])+1e-9 &&
		math.Min(a[1], b[1])-1e-9 <= p[1] && p[1] <= math.Max(a[1], b[1])+1e-9
}

func segmentsIntersect(a1, a2, b1, b2 [2]float64) bool {
	d1 := cross(b1, b2, a1)
	d2 := cross(b1, b2, a2)
	d3 := cross(a1, a2, b1)
	d4 := cross(a1, a2, b2)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	if d1 == 0 && onSegment(b1, b2, a1) {
		return true
	}
	if d2 == 0 && onSegment(b1, b2, a2) {
		return true
	}
	if d3 == 0 && onSegment(a1, a2, b1) {
		return true
	}
	if d4 == 0 && onSegment(a1, a2, b2) {
		return true
	}
	return false
}

// Contains tests whether p lies inside or on the boundary of the geofence.
func (g GeofenceSpec) Contains(p Point) bool {
	if !validPoint(p) {
		return false
	}
	switch g.Kind {
	case GeofenceCircle:
		return Metres(Point{Lat: g.CenterLat, Lon: g.CenterLon}, p) <= float64(g.RadiusMetres)
	case GeofencePolygon:
		if len(g.Vertices) < MinPolygonVertices {
			return false
		}
		const k = math.Pi / 180 / 1e7
		cos := math.Cos(float64(g.Vertices[0].Lat) * k)
		px := float64(p.Lon-g.Vertices[0].Lon) * k * cos * EarthRadiusMetres
		py := float64(p.Lat-g.Vertices[0].Lat) * k * EarthRadiusMetres
		inside := false
		n := len(g.Vertices)
		j := n - 1
		for i := 0; i < n; i++ {
			xi := float64(g.Vertices[i].Lon-g.Vertices[0].Lon) * k * cos * EarthRadiusMetres
			yi := float64(g.Vertices[i].Lat-g.Vertices[0].Lat) * k * EarthRadiusMetres
			xj := float64(g.Vertices[j].Lon-g.Vertices[0].Lon) * k * cos * EarthRadiusMetres
			yj := float64(g.Vertices[j].Lat-g.Vertices[0].Lat) * k * EarthRadiusMetres
			if ((yi > py) != (yj > py)) && (px < (xj-xi)*(py-yi)/(yj-yi)+xi) {
				inside = !inside
			}
			j = i
		}
		return inside
	default:
		return false
	}
}

// GeofenceState tracks the current boundary state across ordered fixes.
type GeofenceState struct {
	Known        bool
	Inside       bool
	DwellEmitted bool
	EnteredAt    time.Time
}

// GeofenceEvent is one derived crossing or dwell observation (coordinates omitted).
type GeofenceEvent struct {
	GeofenceID     string
	TripID         string
	SessionID      string
	Kind           GeofenceEventKind
	FixAt          time.Time
	DetectedAt     time.Time
	LateSuppressed bool
}

// EvaluateGeofence evaluates precise accepted fixes in chronological order
// against a single geofence specification (BR-GPS-007, BR-GPS-037, VAL-30).
func EvaluateGeofence(g GeofenceSpec, tripID, sessionID string, state GeofenceState, fixes []TrackFix, detectedAt time.Time) ([]GeofenceEvent, GeofenceState) {
	if !g.Enabled {
		return nil, state
	}
	var events []GeofenceEvent
	det := detectedAt.UTC()
	for _, f := range fixes {
		if f.Coarse || f.Accuracy < 0 || f.Accuracy > MaxPreciseAccuracyMetres {
			continue
		}
		p := Point{Lat: f.Lat, Lon: f.Lon}
		if !validPoint(p) {
			continue
		}
		inside := g.Contains(p)
		at := f.At.UTC()
		late := det.Sub(at) > LateGeofenceAlertThreshold
		if !state.Known {
			state.Known = true
			state.Inside = inside
			if inside {
				state.EnteredAt = at
			}
			continue
		}
		if !state.Inside && inside {
			state.Inside = true
			state.EnteredAt = at
			state.DwellEmitted = false
			if g.OnEnter {
				events = append(events, GeofenceEvent{
					GeofenceID:     g.ID,
					TripID:         tripID,
					SessionID:      sessionID,
					Kind:           GeofenceEnter,
					FixAt:          at,
					DetectedAt:     det,
					LateSuppressed: late,
				})
			}
		} else if state.Inside && !inside {
			state.Inside = false
			state.DwellEmitted = false
			if g.OnExit {
				events = append(events, GeofenceEvent{
					GeofenceID:     g.ID,
					TripID:         tripID,
					SessionID:      sessionID,
					Kind:           GeofenceExit,
					FixAt:          at,
					DetectedAt:     det,
					LateSuppressed: late,
				})
			}
		} else if state.Inside && inside && g.DwellSeconds >= MinDwellSeconds && !state.DwellEmitted {
			if !at.Before(state.EnteredAt.Add(time.Duration(g.DwellSeconds) * time.Second)) {
				state.DwellEmitted = true
				events = append(events, GeofenceEvent{
					GeofenceID:     g.ID,
					TripID:         tripID,
					SessionID:      sessionID,
					Kind:           GeofenceDwell,
					FixAt:          at,
					DetectedAt:     det,
					LateSuppressed: late,
				})
			}
		}
	}
	return events, state
}

// Keep unused imports guard happy across tests.
var _ = testing.AllocsPerRun
