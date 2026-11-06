package domain

import (
	"testing"
)

func TestParseFeedUF(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty", raw: "", want: ""},
		{name: "uppercase", raw: "SP", want: "SP"},
		{name: "normalizes lowercase", raw: "sp", want: "SP"},
		{name: "trims spaces", raw: "  mg  ", want: "MG"},
		{name: "invalid code", raw: "XX", wantErr: true},
		{name: "too short", raw: "S", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFeedUF(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestParseFeedMunicipalityIBGE(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "empty", raw: "", want: 0},
		{name: "valid São Paulo", raw: "3550308", want: 3550308},
		{name: "too short", raw: "355030", wantErr: true},
		{name: "too long", raw: "35503080", wantErr: true},
		{name: "non numeric", raw: "35503a8", wantErr: true},
		{name: "below range", raw: "0999999", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFeedMunicipalityIBGE(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestValidateFeedRegion(t *testing.T) {
	sp := 3550308
	prefixMismatch := 3304557
	empty := ""

	if err := ValidateFeedRegion(empty, nil); err != nil {
		t.Fatalf("empty region must be accepted: %v", err)
	}
	if err := ValidateFeedRegion("SP", &sp); err != nil {
		t.Fatalf("matching UF/IBGE must be accepted: %v", err)
	}
	if err := ValidateFeedRegion("SP", &prefixMismatch); err == nil {
		t.Fatal("expected prefix mismatch to be refused")
	}
	if err := ValidateFeedRegion("XX", nil); err == nil {
		t.Fatal("expected unknown UF to be refused")
	}
}

func TestParseFeedFuelProduct(t *testing.T) {
	for _, fuel := range []string{
		"GASOLINE_REGULAR", "GASOLINE_PREMIUM", "ETHANOL",
		"DIESEL_S10", "DIESEL_S500", "GLP", "GNV",
	} {
		got, err := ParseFeedFuelProduct(fuel)
		if err != nil {
			t.Fatalf("fuel %s: unexpected error: %v", fuel, err)
		}
		if got != fuel {
			t.Fatalf("fuel %s: expected preserved, got %q", fuel, got)
		}
	}
	if got, err := ParseFeedFuelProduct("  "); err != nil || got != "" {
		t.Fatalf("blank fuel must be absent, got %q, %v", got, err)
	}
	if _, err := ParseFeedFuelProduct("DIESEL"); err == nil {
		t.Fatal("expected unknown fuel to be refused")
	}
}

func TestParseFeedStationID(t *testing.T) {
	valid := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	got, err := ParseFeedStationID(valid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != valid {
		t.Fatalf("expected normalized id, got %q", got)
	}
	if got, err := ParseFeedStationID("   "); err != nil || got != "" {
		t.Fatalf("blank station must be absent, got %q, %v", got, err)
	}
	for _, bad := range []string{"not-a-uuid", "d6c74c23-63db-4c24-a2e5-408cb23bad2"} {
		if _, err := ParseFeedStationID(bad); err == nil {
			t.Fatalf("expected %q to be refused", bad)
		}
	}
}
