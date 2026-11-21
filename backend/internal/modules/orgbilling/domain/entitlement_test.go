package domain_test

import (
	"errors"
	"math"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

func TestPlanEntitlementRequiresExactCurrentOrganizationAndPlan(t *testing.T) {
	s := scope(t, orgID)
	key := d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	known, err := capability.NewCatalog([]string{"organizations", "reports"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := d.NewPlan(key, "Synthetic", []string{"organizations"}, map[d.Meter]int64{d.ActiveVehicles: 10}, known)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	period := d.Period{Start: start, End: start.Add(time.Hour)}
	for _, tc := range []struct {
		name  string
		state d.SubscriptionState
		prior d.SubscriptionState
		at    time.Time
		allow bool
	}{
		{"trial in term", d.Trialing, "", start, true},
		{"paid in term", d.Active, "", start, true},
		{"paid at exclusive boundary", d.Active, "", period.End, false},
		{"evaluation predates confirmed term", d.Active, "", start.Add(-time.Second), false},
		{"pending active cancellation", d.Cancelling, d.Active, start, true},
		{"pending overdue cancellation", d.Cancelling, d.PastDue, start, false},
		{"past due", d.PastDue, "", start, false},
		{"suspended", d.Suspended, d.Active, start, false},
		{"terminal", d.Cancelled, "", start, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := d.RestoreSubscription(d.SubscriptionSnapshot{Scope: s, SubscriptionID: subsID, Plan: key,
				State: tc.state, PriorState: tc.prior, Period: period, Revision: 2, LastEventAt: start})
			if err != nil {
				t.Fatal(err)
			}
			got := d.CapabilityAllowed(s, sub, plan, "organizations", tc.at)
			if got != tc.allow {
				t.Fatalf("allowed=%v want=%v", got, tc.allow)
			}
			if d.CapabilityAllowed(s, sub, plan, "reports", tc.at) ||
				d.CapabilityAllowed(scope(t, "7b0e2a54-0000-4000-8000-000000000099"), sub, plan, "organizations", tc.at) {
				t.Fatal("entitlement widened its organization or capability")
			}
			wrongPlan, err := d.NewPlan(d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 2}, "Synthetic II",
				[]string{"organizations"}, nil, known)
			if err != nil {
				t.Fatal(err)
			}
			if d.CapabilityAllowed(s, sub, wrongPlan, "organizations", tc.at) {
				t.Fatal("a different plan revision unlocked the subscription")
			}
		})
	}
}

func TestResourceLimitDistinguishesUndeclaredZeroAndExactCeiling(t *testing.T) {
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := d.NewPlan(d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}, "Synthetic", nil,
		map[d.Meter]int64{d.ActiveVehicles: 10, d.StorageBytes: math.MaxInt64, d.ActiveStations: 0}, known)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		meter      d.Meter
		used, more int64
		want       error
	}{
		{d.ActiveVehicles, 9, 1, nil},
		{d.ActiveVehicles, 10, 1, d.ErrLimitReached},
		{d.ActiveStations, 0, 1, d.ErrLimitReached},
		{d.APICalls, 0, 1, d.ErrLimitUndefined},
		{d.StorageBytes, math.MaxInt64, 1, d.ErrLimitReached},
		{d.ActiveVehicles, -1, 1, d.ErrInvalidUsage},
		{d.ActiveVehicles, 1, 0, d.ErrInvalidUsage},
	} {
		if err := plan.CheckIncrease(tc.meter, tc.used, tc.more); !errors.Is(err, tc.want) {
			t.Fatalf("meter %s (%d + %d): got %v want %v", tc.meter, tc.used, tc.more, err, tc.want)
		}
	}
}
