package application

import (
	"testing"
)

func TestValidateSearch(t *testing.T) {
	if _, err := ValidateSearch("SP", "3550308", "Posto", 20, ""); err != nil {
		t.Errorf("valid search rejected: %v", err)
	}
	if _, err := ValidateSearch("sp", "", "", 20, ""); err != nil {
		t.Errorf("lowercase state rejected: %v", err)
	}
	bad := []struct {
		name           string
		state, muni, q string
		limit          int
		after          string
	}{
		{"state length", "SPO", "", "", 20, ""},
		{"municipality long", "", "12345678901234567", "", 20, ""},
		{"q short", "", "", "x", 20, ""},
		{"limit zero", "", "", "", 0, ""},
		{"limit over", "", "", "", 101, ""},
		{"cursor garbage", "", "", "", 20, "nope"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ValidateSearch(c.state, c.muni, c.q, c.limit, c.after); err == nil {
				t.Error("invalid search accepted")
			}
		})
	}
}

func TestValidateNearby(t *testing.T) {
	f, err := ValidateNearby(-23.55, -46.63, 3000, 20, "")
	if err != nil {
		t.Fatalf("valid nearby rejected: %v", err)
	}
	if f.HasCursor {
		t.Error("first page has cursor")
	}
	if _, err := ValidateNearby(-23.55, -46.63, 3000, 20, "12.5:d6c74c23-63db-4c24-a2e5-408cb23bad26"); err != nil {
		t.Errorf("valid key rejected: %v", err)
	}
	bad := []struct {
		name          string
		lat, lon      float64
		radius, limit int
		key           string
	}{
		{"lat over", 91, 0, 3000, 20, ""},
		{"lat under", -91, 0, 3000, 20, ""},
		{"lon over", 0, 181, 3000, 20, ""},
		{"radius small", 0, 0, 99, 20, ""},
		{"radius big", 0, 0, 15001, 20, ""},
		{"bad key", 0, 0, 3000, 20, "nope"},
		{"bad key uuid", 0, 0, 3000, 20, "1.5:not-a-uuid"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ValidateNearby(c.lat, c.lon, c.radius, c.limit, c.key); err == nil {
				t.Error("invalid nearby accepted")
			}
		})
	}
}

func TestValidateStationID(t *testing.T) {
	if _, err := ValidateStationID("d6c74c23-63db-4c24-a2e5-408cb23bad26"); err != nil {
		t.Errorf("uuid rejected: %v", err)
	}
	for _, id := range []string{"", "xyz", "d6c74c23-63db-4c24-a2e5-408cb23bad2", "d6c74c23-63db-4c24-a2e5-408cb23bad2g"} {
		if _, err := ValidateStationID(id); err == nil {
			t.Errorf("id %q accepted", id)
		}
	}
}
