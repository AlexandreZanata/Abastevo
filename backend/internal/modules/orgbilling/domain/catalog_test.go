package domain_test

import (
	"errors"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

func TestPlanRequiresExactVersionAndRegisteredCapabilities(t *testing.T) {
	known, err := capability.NewCatalog([]string{"fleet", "reports"})
	if err != nil {
		t.Fatal(err)
	}
	key := d.PlanKey{Product: "fleet", Plan: "synthetic-plan", Revision: 1}
	caps := []string{"fleet"}
	limits := map[d.Meter]int64{d.ActiveVehicles: 2, d.StorageBytes: 0}
	plan, err := d.NewPlan(key, "Synthetic plan", caps, limits, known)
	if err != nil {
		t.Fatal(err)
	}

	caps[0] = "reports"
	limits[d.ActiveVehicles] = 25
	if plan.Key() != key || plan.Name() != "Synthetic plan" || !plan.Includes("fleet") || plan.Includes("reports") {
		t.Fatal("a caller changed the saved plan or received an undeclared capability")
	}
	if got, err := plan.Limit(d.ActiveVehicles); err != nil || got != 2 {
		t.Fatalf("vehicle ceiling = %d, %v", got, err)
	}
	if got, err := plan.Limit(d.StorageBytes); err != nil || got != 0 {
		t.Fatalf("explicit zero storage ceiling = %d, %v", got, err)
	}
	for _, missing := range []d.Meter{d.APICalls, "unknown-meter"} {
		if _, err := plan.Limit(missing); !errors.Is(err, d.ErrLimitUndefined) {
			t.Fatalf("limit %q was silently assigned: %v", missing, err)
		}
	}
}

func TestPlanRejectsInvalidFactsAndPreservesDistinctVersions(t *testing.T) {
	known, err := capability.NewCatalog([]string{"fleet"})
	if err != nil {
		t.Fatal(err)
	}
	key := d.PlanKey{Product: "fleet", Plan: "synthetic-plan", Revision: 2}
	for _, invalid := range []struct {
		name   string
		key    d.PlanKey
		label  string
		caps   []string
		limits map[d.Meter]int64
	}{
		{name: "unknown capability", key: key, label: "Synthetic", caps: []string{"unregistered"}},
		{name: "duplicate capability", key: key, label: "Synthetic", caps: []string{"fleet", "fleet"}},
		{name: "missing revision", key: d.PlanKey{Product: "fleet", Plan: "synthetic-plan"}, label: "Synthetic"},
		{name: "wrong product code", key: d.PlanKey{Product: "Fleet", Plan: "synthetic-plan", Revision: 2}, label: "Synthetic"},
		{name: "unknown meter", key: key, label: "Synthetic", limits: map[d.Meter]int64{"unknown": 1}},
		{name: "negative ceiling", key: key, label: "Synthetic", limits: map[d.Meter]int64{d.ActiveVehicles: -1}},
		{name: "unusable label", key: key, label: "some\nplan"},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			if _, err := d.NewPlan(invalid.key, invalid.label, invalid.caps, invalid.limits, known); !errors.Is(err, d.ErrInvalidPlan) {
				t.Fatalf("invalid plan accepted: %v", err)
			}
		})
	}
	previous := key
	previous.Revision--
	one, err := d.NewPlan(previous, "Synthetic", nil, map[d.Meter]int64{d.ActiveVehicles: 2}, known)
	if err != nil {
		t.Fatal(err)
	}
	two, err := d.NewPlan(key, "Synthetic", nil, map[d.Meter]int64{d.ActiveVehicles: 3}, known)
	if err != nil {
		t.Fatal(err)
	}
	priorLimit, _ := one.Limit(d.ActiveVehicles)
	currentLimit, _ := two.Limit(d.ActiveVehicles)
	if one.Key() == two.Key() || priorLimit != 2 || currentLimit != 3 {
		t.Fatal("the newer revision overwrote the older version")
	}
}
