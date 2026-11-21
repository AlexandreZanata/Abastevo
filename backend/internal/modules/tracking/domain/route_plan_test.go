package domain

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestValidateRoutePlanInput(t *testing.T) {
	owner := "7b0e2a54-0000-4000-8000-000000000801"
	compID := "7b0e2a54-0000-4000-8000-000000000802"
	pts := []RouteWaypoint{
		{Label: "Origem", Source: WaypointCoordinates, Lat: -235505200, Lon: -466333080},
		{Label: "Destino", Source: WaypointCoordinates, Lat: -235605200, Lon: -466433080},
	}
	dist := int64(42_300)
	manual := PlannedDistanceManual
	plan, err := ValidateRoutePlanInput(RouteOwnerTrip, owner, 1, pts, RouteOptions{AvoidToll: true}, &dist, &manual, "")
	if err != nil || len(plan.Waypoints) != 2 || *plan.PlannedDistanceMetres != 42_300 || *plan.PlannedDistanceSource != PlannedDistanceManual {
		t.Fatalf("valid route plan failed: %+v %v", plan, err)
	}

	// Nil distance stays nil ("não informada", never zero).
	planNoDist, err := ValidateRoutePlanInput(RouteOwnerMission, owner, 1, pts, RouteOptions{}, nil, nil, "")
	if err != nil || planNoDist.PlannedDistanceMetres != nil || planNoDist.PlannedDistanceSource != nil {
		t.Fatalf("route plan without distance failed: %+v %v", planNoDist, err)
	}

	// Duplicate coordinates refused (UC-ROUTE-01).
	dup := []RouteWaypoint{pts[0], {Label: "Cópia", Source: WaypointCoordinates, Lat: pts[0].Lat, Lon: pts[0].Lon}}
	if _, err := ValidateRoutePlanInput(RouteOwnerMission, owner, 1, dup, RouteOptions{}, nil, nil, ""); !errors.Is(err, ErrRouteValidation) {
		t.Fatalf("duplicate coordinates accepted: %v", err)
	}

	// >102 points refused with ErrRoutePointLimit (BR-ROUTE-002).
	tooMany := make([]RouteWaypoint, 103)
	for i := range tooMany {
		tooMany[i] = RouteWaypoint{Label: fmt.Sprintf("P%d", i), Source: WaypointCoordinates, Lat: -235000000 + int32(i)*100, Lon: -466000000}
	}
	if _, err := ValidateRoutePlanInput(RouteOwnerMission, owner, 1, tooMany, RouteOptions{}, nil, nil, ""); !errors.Is(err, ErrRoutePointLimit) {
		t.Fatalf("103 waypoints error=%v want ErrRoutePointLimit", err)
	}

	// ENGINE planned distance requires computation ID.
	eng := PlannedDistanceEngine
	if _, err := ValidateRoutePlanInput(RouteOwnerMission, owner, 1, pts, RouteOptions{}, &dist, &eng, ""); !errors.Is(err, ErrRouteValidation) {
		t.Fatal("ENGINE distance accepted without computation ID")
	}
	if _, err := ValidateRoutePlanInput(RouteOwnerMission, owner, 1, pts, RouteOptions{}, &dist, &eng, compID); err != nil {
		t.Fatalf("valid ENGINE distance rejected: %v", err)
	}
}

func TestValidateRouteComputationCarOnly(t *testing.T) {
	owner := "7b0e2a54-0000-4000-8000-000000000801"
	base := RouteComputation{
		OwnerKind:       RouteOwnerTrip,
		OwnerID:         owner,
		PlanRevision:    1,
		EngineID:        "comaps-local",
		MapDataVersion:  "20261110",
		Profile:         RouteProfileCar,
		Kind:            ComputationDeparture,
		DistanceMetres:  181_000,
		DurationSeconds: 7200,
		ComputedAt:      time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
	}
	got, err := ValidateRouteComputation(base)
	if err != nil || got.Attribution != OpenStreetMapAttribution {
		t.Fatalf("valid computation failed: %+v %v", got, err)
	}
	truck := base
	truck.Profile = "TRUCK"
	if _, err := ValidateRouteComputation(truck); !errors.Is(err, ErrRouteProfileUnsupported) {
		t.Fatalf("unsupported profile error=%v want ErrRouteProfileUnsupported", err)
	}
}
