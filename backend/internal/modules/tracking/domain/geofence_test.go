package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func pointOffsetNorthMetres(lat, lon int32, northM float64) Point {
	dLat := int32(math.Round((northM / EarthRadiusMetres) * (180.0 / math.Pi) * 1e7))
	return Point{Lat: lat + dLat, Lon: lon}
}

func TestGeofenceSpecValidation(t *testing.T) {
	validCircle := GeofenceSpec{
		Name:         "Pátio Central",
		Kind:         GeofenceCircle,
		CenterLat:    -235505200,
		CenterLon:    -466333080,
		RadiusMetres: 200,
		OnEnter:      true,
		OnExit:       true,
		Enabled:      true,
	}
	if err := validCircle.Validate(); err != nil {
		t.Fatalf("valid circle rejected: %v", err)
	}

	// Self-intersecting bowtie polygon must return ErrGeofenceInvalidGeometry.
	bowtie := GeofenceSpec{
		Name:    "Laço Inválido",
		Kind:    GeofencePolygon,
		OnEnter: true,
		Vertices: []Point{
			{Lat: -235500000, Lon: -466300000},
			{Lat: -235400000, Lon: -466200000},
			{Lat: -235500000, Lon: -466200000},
			{Lat: -235400000, Lon: -466300000},
		},
	}
	if err := bowtie.Validate(); !errors.Is(err, ErrGeofenceInvalidGeometry) {
		t.Fatalf("bowtie polygon error=%v want ErrGeofenceInvalidGeometry", err)
	}

	// Valid square polygon.
	square := GeofenceSpec{
		Name:    "Área Sul",
		Kind:    GeofencePolygon,
		OnEnter: true,
		Vertices: []Point{
			{Lat: -235500000, Lon: -466300000},
			{Lat: -235500000, Lon: -466290000},
			{Lat: -235490000, Lon: -466290000},
			{Lat: -235490000, Lon: -466300000},
		},
	}
	if err := square.Validate(); err != nil {
		t.Fatalf("valid polygon rejected: %v", err)
	}

	// Radius bounds.
	badRadius := validCircle
	badRadius.RadiusMetres = 49
	if err := badRadius.Validate(); !errors.Is(err, ErrGeofenceInvalidGeometry) {
		t.Fatalf("radius 49 accepted: %v", err)
	}
	badRadius.RadiusMetres = 50_001
	if err := badRadius.Validate(); !errors.Is(err, ErrGeofenceInvalidGeometry) {
		t.Fatalf("radius 50001 accepted: %v", err)
	}
}

func TestEvaluateGeofenceVAL30AndQualityAndLateSuppression(t *testing.T) {
	centerLat, centerLon := int32(-235505200), int32(-466333080)
	gf := GeofenceSpec{
		ID:           "gf-1",
		Name:         "Terminal VAL-30",
		Kind:         GeofenceCircle,
		CenterLat:    centerLat,
		CenterLon:    centerLon,
		RadiusMetres: 200,
		OnEnter:      true,
		OnExit:       true,
		Enabled:      true,
	}
	p350 := pointOffsetNorthMetres(centerLat, centerLon, 350)
	p150 := pointOffsetNorthMetres(centerLat, centerLon, 150)
	p120 := pointOffsetNorthMetres(centerLat, centerLon, 120)
	p260 := pointOffsetNorthMetres(centerLat, centerLon, 260)

	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	fixes := []TrackFix{
		{At: t0, Lat: p350.Lat, Lon: p350.Lon, Accuracy: 10},
		{At: t0.Add(1 * time.Minute), Lat: p150.Lat, Lon: p150.Lon, Accuracy: 10},
		{At: t0.Add(2 * time.Minute), Lat: p120.Lat, Lon: p120.Lon, Accuracy: 10},
		{At: t0.Add(3 * time.Minute), Lat: p260.Lat, Lon: p260.Lon, Accuracy: 10},
	}

	// VAL-30: one entry at fix 2, one exit at fix 4.
	events, state := EvaluateGeofence(gf, "trip-1", "sess-1", GeofenceState{}, fixes, t0.Add(4*time.Minute))
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d (%+v)", len(events), events)
	}
	if events[0].Kind != GeofenceEnter || !events[0].FixAt.Equal(fixes[1].At) || events[0].LateSuppressed {
		t.Fatalf("unexpected enter event: %+v", events[0])
	}
	if events[1].Kind != GeofenceExit || !events[1].FixAt.Equal(fixes[3].At) || events[1].LateSuppressed {
		t.Fatalf("unexpected exit event: %+v", events[1])
	}
	if !state.Known || state.Inside {
		t.Fatalf("unexpected final state: %+v", state)
	}

	// Coarse or inaccurate fixes never emit geofence crossings (BR-GPS-007).
	coarseFixes := []TrackFix{
		{At: t0, Lat: p350.Lat, Lon: p350.Lon, Accuracy: 5, Coarse: true},
		{At: t0.Add(time.Minute), Lat: p150.Lat, Lon: p150.Lon, Accuracy: 5, Coarse: true},
		{At: t0.Add(2 * time.Minute), Lat: p150.Lat, Lon: p150.Lon, Accuracy: 80, Coarse: false},
	}
	evCoarse, _ := EvaluateGeofence(gf, "trip-1", "sess-1", GeofenceState{}, coarseFixes, t0.Add(3*time.Minute))
	if len(evCoarse) != 0 {
		t.Fatalf("coarse/inaccurate fixes emitted %d events", len(evCoarse))
	}

	// Event detected more than 30 minutes after fix carries LateSuppressed=true (BR-GPS-037).
	evLate, _ := EvaluateGeofence(gf, "trip-1", "sess-1", GeofenceState{}, fixes[:2], t0.Add(35*time.Minute))
	if len(evLate) != 1 || !evLate[0].LateSuppressed {
		t.Fatalf("expected late-suppressed enter event, got %+v", evLate)
	}

	// Dwell trigger test.
	gfDwell := gf
	gfDwell.OnEnter, gfDwell.OnExit, gfDwell.DwellSeconds = false, false, 120
	dwellFixes := []TrackFix{
		{At: t0, Lat: p150.Lat, Lon: p150.Lon, Accuracy: 10},
		{At: t0.Add(60 * time.Second), Lat: p120.Lat, Lon: p120.Lon, Accuracy: 10},
		{At: t0.Add(120 * time.Second), Lat: p120.Lat, Lon: p120.Lon, Accuracy: 10},
		{At: t0.Add(180 * time.Second), Lat: p120.Lat, Lon: p120.Lon, Accuracy: 10},
	}
	evDwell, _ := EvaluateGeofence(gfDwell, "trip-1", "sess-1", GeofenceState{}, dwellFixes, t0.Add(4*time.Minute))
	if len(evDwell) != 1 || evDwell[0].Kind != GeofenceDwell || !evDwell[0].FixAt.Equal(dwellFixes[2].At) {
		t.Fatalf("expected single dwell event at +120s, got %+v", evDwell)
	}
}
