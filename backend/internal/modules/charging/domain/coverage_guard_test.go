package charging

import (
	"math"
	"reflect"
	"testing"
)

func TestCoverageDropsStrictThresholdAndMissingStates(t *testing.T) {
	previous := map[string]int64{"SP": 100, "MG": 100, "AC": 1, "PR": 20, "UNASSIGNED": 11}
	next := map[string]int64{"SP": 85, "MG": 84, "PR": 17, "RJ": 500, "UNASSIGNED": 900}
	got, err := CoverageDrops(previous, next)
	want := []CoverageDrop{{UF: "AC", Previous: 1, Next: 0}, {UF: "MG", Previous: 100, Next: 84}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("drops = %+v, err = %v, want %+v", got, err, want)
	}
	if got, err := CoverageDrops(nil, next); err != nil || len(got) != 0 {
		t.Fatalf("first generation = %+v, %v", got, err)
	}
}

func TestCoverageDropsValidationAndLargeCounts(t *testing.T) {
	for _, bad := range []map[string]int64{{"ZZ": 3}, {"SP": -1}, {"sp": 3}} {
		if _, err := CoverageDrops(nil, bad); err == nil {
			t.Errorf("invalid candidate accepted: %v", bad)
		}
		if _, err := CoverageDrops(bad, nil); err == nil {
			t.Errorf("invalid previous accepted: %v", bad)
		}
	}
	previous := map[string]int64{"DF": math.MaxInt64}
	// Largest integer drop that does not exceed 15 percent.
	minimum := int64(7839866231326559436)
	if got, err := CoverageDrops(previous, map[string]int64{"DF": minimum}); err != nil || len(got) != 0 {
		t.Fatalf("exact large boundary = %+v, %v", got, err)
	}
	if got, err := CoverageDrops(previous, map[string]int64{"DF": minimum - 1}); err != nil || len(got) != 1 {
		t.Fatalf("large drop = %+v, %v", got, err)
	}
}
