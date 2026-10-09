package application

import (
	"testing"
)

const testUUID = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

func TestValidateHistory(t *testing.T) {
	f, err := ValidateHistory(testUUID, "GASOLINE_REGULAR", "", 20, "")
	if err != nil {
		t.Fatalf("valid history rejected: %v", err)
	}
	if f.HasCursor || f.Fuel != "GASOLINE_REGULAR" {
		t.Errorf("filter = %+v", f)
	}
	f, err = ValidateHistory(testUUID, "", testUUID, 20, "2026-09-24:"+testUUID)
	if err != nil || !f.HasCursor || f.RevisionID == "" {
		t.Errorf("cursor history = %+v, %v", f, err)
	}
	bad := []struct {
		name               string
		station, fuel, rev string
		limit              int
		key                string
	}{
		{"station garbage", "xyz", "", "", 20, ""},
		{"fuel unknown", testUUID, "JET_A1", "", 20, ""},
		{"fuel legacy", testUUID, "GASOLINE_PREMIUM", "", 20, ""},
		{"revision garbage", testUUID, "", "xyz", 20, ""},
		{"limit zero", testUUID, "", "", 0, ""},
		{"cursor garbage", testUUID, "", "", 20, "nope"},
		{"cursor bad date", testUUID, "", "", 20, "24-09-2026:" + testUUID},
		{"cursor bad uuid", testUUID, "", "", 20, "2026-09-24:not-a-uuid"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ValidateHistory(c.station, c.fuel, c.rev, c.limit, c.key); err == nil {
				t.Error("invalid history accepted")
			}
		})
	}
}

func TestValidateStationIDAndFuel(t *testing.T) {
	if _, err := ValidateStationID(testUUID); err != nil {
		t.Errorf("uuid rejected: %v", err)
	}
	if _, err := ValidateStationID("nope"); err == nil {
		t.Error("garbage id accepted")
	}
	if f, err := ValidateFuel(""); err != nil || f != "" {
		t.Errorf("empty fuel = %q, %v", f, err)
	}
	for _, fuel := range []string{"ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "DIESEL_S500", "DIESEL_S10", "CNG", "LPG_P13", "GASOLINE_PREMIUM_GRADE"} {
		if _, err := ValidateFuel(fuel); err != nil {
			t.Errorf("wire fuel %s rejected: %v", fuel, err)
		}
	}
}
