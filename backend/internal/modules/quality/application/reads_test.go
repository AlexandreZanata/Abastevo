package application

import (
	"strings"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func TestValidateStationID(t *testing.T) {
	id, err := ValidateStationID("AAAAAAAA-1111-4111-8111-000000000001")
	if err != nil || id != "aaaaaaaa-1111-4111-8111-000000000001" {
		t.Errorf("valid UUID -> %q %v", id, err)
	}
	for _, bad := range []string{"", "123", "not-a-uuid-at-all-00000000000000", "aaaaaaaa-1111-4111-8111-00000000001"} {
		if _, err := ValidateStationID(bad); err == nil {
			t.Errorf("station id %q accepted", bad)
		}
	}
}

func TestValidateFuel(t *testing.T) {
	for _, f := range domain.AllFuels {
		if _, err := ValidateFuel(string(f)); err != nil {
			t.Errorf("wire fuel %q rejected", f)
		}
	}
	if _, err := ValidateFuel(""); err != nil {
		t.Errorf("empty fuel filter rejected: %v", err)
	}
	for _, bad := range []string{"HYDROGEN", "GASOLINE_PREMIUM", "ethanol"} {
		if _, err := ValidateFuel(bad); err == nil {
			t.Errorf("fuel %q accepted", bad)
		}
	}
}

func TestStale(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	if stale, _ := Stale(false, time.Time{}, now); !stale {
		t.Error("missing lease reads fresh")
	}
	if stale, _ := Stale(true, now.Add(-time.Hour), now); stale {
		t.Error("fresh lease reads stale")
	}
	if stale, reason := Stale(true, now.AddDate(0, 0, -(StaleAfterDays+1)), now); !stale || reason == "" {
		t.Error("old lease reads fresh or reasonless")
	}
	if stale, _ := Stale(true, now.AddDate(0, 0, -StaleAfterDays).Add(time.Second), now); stale {
		t.Error("boundary lease reads stale")
	}
}

func TestCoverageNote(t *testing.T) {
	note := CoverageNote("2026-09", domain.Counts{Input: 10, Accepted: 7, Duplicates: 2, Quarantined: 1})
	for _, want := range []string{"2026-09", "10 input", "7 accepted", "2 duplicates", "1 quarantined", "never rewrite"} {
		if !strings.Contains(note, want) {
			t.Errorf("coverage note %q misses %q", note, want)
		}
	}
}
