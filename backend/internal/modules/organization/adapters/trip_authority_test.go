package adapters

import (
	"errors"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// B-BR-EMP-316/EDR-206: trip.trip.start is an S cell for every holder. An
// ORG grant covers the member's own trip; SELF covers only assigned periods
// when the member is the driver; structural grants stay unsupported.
func TestTripHistoryRuleCoversOwnTripsOnAssignedAssets(t *testing.T) {
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	at := func(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }
	scope := meterHistoryScope(t)
	member, asset := meterHistoryID(3), meterHistoryID(10)
	period := func(n int, role permission.Role, kind permission.ScopeType, from time.Time) meterPermissionPeriod {
		return meterHistoryPeriod(t, scope, n, role, kind, from, from, time.Time{}, time.Time{})
	}
	assigned := []meterAssignedPeriod{{from: at(-60)}}
	cases := []struct {
		name     string
		periods  []meterPermissionPeriod
		assigned []meterAssignedPeriod
		self     bool
		want     fleet.MeterDisposition
		denied   bool
		unknown  bool
	}{
		{name: "owner-org-s-cell-covers", periods: []meterPermissionPeriod{period(1, permission.Owner, permission.OrgScope, at(-60))}, want: fleet.MeterApply},
		{name: "owner-org-names-another-driver", periods: []meterPermissionPeriod{period(1, permission.Owner, permission.OrgScope, at(-60))}, self: false, want: fleet.MeterApply},
		{name: "fleet-manager-org-new-grant-held", periods: []meterPermissionPeriod{period(1, permission.FleetManager, permission.OrgScope, at(-10))}, want: fleet.MeterHoldPermissionChanged},
		{name: "driver-self-assigned-own-trip", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, assigned: assigned, self: true, want: fleet.MeterApply},
		{name: "driver-self-assigned-after-declaration-held", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, assigned: []meterAssignedPeriod{{from: at(-10)}}, self: true, want: fleet.MeterHoldPermissionChanged},
		{name: "driver-self-never-assigned-forbidden", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, assigned: []meterAssignedPeriod{}, self: true, denied: true},
		{name: "driver-self-other-driver-forbidden", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, assigned: assigned, self: false, denied: true},
		{name: "driver-self-without-assignment-source-unavailable", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, assigned: nil, self: true, unknown: true},
		{name: "finance-org-denied-forbidden", periods: []meterPermissionPeriod{period(1, permission.Finance, permission.OrgScope, at(-60))}, denied: true},
		{name: "supervisor-unit-unsupported", periods: []meterPermissionPeriod{period(1, permission.Supervisor, permission.UnitScope, at(-60))}, unknown: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := historyDisposition(tripHistoryRule(tc.self), scope, member, asset, at(-30), at(0), tc.periods, tc.assigned)
			switch {
			case tc.denied:
				var denial *permission.PermissionDenial
				if !errors.As(err, &denial) || denial.Permission() != "trip.trip.start" {
					t.Fatal("expected FORBIDDEN on trip.trip.start", got, err)
				}
			case tc.unknown:
				if !errors.Is(err, fleet.ErrUnavailable) {
					t.Fatal("expected unavailable", got, err)
				}
			default:
				if err != nil || got != tc.want {
					t.Fatal("unexpected disposition", got, err)
				}
			}
		})
	}
}
