package metrics

import (
	"testing"
	"time"
)

func TestMEX09RequireBothFullAnchorsAndHomogeneousAnchor(t *testing.T) {
	full := Fill{QuantityMilli: 1, TotalCentavos: 1, Full: true, Fuel: "ETHANOL", Unit: "L"}
	partial := full
	partial.Full = false
	wrongFuel := full
	wrongFuel.Fuel = "GASOLINE_REGULAR"
	wrongUnit := full
	wrongUnit.Unit = "M3"
	for _, tc := range []struct {
		name   string
		anchor Fill
		fills  []Fill
		want   string
	}{
		{"missing-start", partial, []Fill{full, full}, "full-anchors-required"},
		{"trailing-partial", full, []Fill{full, partial}, "full-anchors-required"},
		{"changed-anchor-fuel", wrongFuel, []Fill{full}, "unsupported-mixed-method"},
		{"invalid-unit", wrongUnit, []Fill{wrongUnit}, "incompatible-unit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := (Interval{StartingFillExcluded: tc.anchor, Fills: tc.fills, DistanceMetres: 1000}).Compute()
			if got.Unknown != tc.want {
				t.Fatalf("got %+v want %s", got, tc.want)
			}
		})
	}
}

func TestMEX09LargeValidCostDoesNotWrapWhileScaling(t *testing.T) {
	fills := make([]Fill, 10000)
	for i := range fills {
		fills[i] = Fill{QuantityMilli: 1, TotalCentavos: MaxTotalCentavos, Full: true, Fuel: "ETHANOL", Unit: "L"}
	}
	got, err := (Interval{StartingFillExcluded: fills[0], Fills: fills, DistanceMetres: 1}).Compute()
	if err != nil || got.CentavosPerKm != "10000000000000000000" {
		t.Fatalf("valid integer scaling wrapped: %+v %v", got, err)
	}
}

func TestMEX09MatchedWindowRejectsInvalidBaseMoney(t *testing.T) {
	got := MatchedWindowCost(-1, 1, nil, Window{StartOdometer: 1, EndOdometer: 2, StartTime: time.UnixMilli(1000), EndTime: time.UnixMilli(2000)})
	if got.Unknown != "input-out-of-bounds" {
		t.Fatalf("negative actual cost accepted %+v", got)
	}
}
