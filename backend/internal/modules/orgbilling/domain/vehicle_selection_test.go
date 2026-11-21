package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestPlanSelectionKeepsOnlyExplicitVehiclesAtTheNewLimit(t *testing.T) {
	active := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N"}
	keep := []string{"K", "N", "A", "B", "E", "G", "I", "D", "J", "C"}
	archived, err := SelectPlanArchives(active, keep, 10)
	if err != nil || !reflect.DeepEqual(archived, []string{"F", "H", "L", "M"}) {
		t.Fatalf("selection did not preserve the exact ten chosen vehicles: %v, %v", archived, err)
	}
	for _, tc := range []struct {
		active, keep []string
		limit        int64
	}{
		{active, keep[:9], 10},
		{active, append(append([]string{}, keep[:9]...), keep[0]), 10},
		{active, append(append([]string{}, keep[:9]...), "foreign"), 10},
		{active[:10], keep, 10},
		{active, nil, -1},
	} {
		if got, err := SelectPlanArchives(tc.active, tc.keep, tc.limit); !errors.Is(err, ErrVehicleSelection) || got != nil {
			t.Fatalf("invalid selection produced archives: %v / %v", got, err)
		}
	}
	all, err := SelectPlanArchives(active, nil, 0)
	if err != nil || !reflect.DeepEqual(all, active) {
		t.Fatalf("explicit zero limit did not produce an exact retained set: %v / %v", all, err)
	}
}
