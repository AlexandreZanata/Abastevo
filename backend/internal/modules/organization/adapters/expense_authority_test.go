package adapters

import (
	"errors"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// B-BR-EMP-448/EDR-289: exp.expense.record is an S cell for every holder
// except APPROVER and AUDITOR. An ORG grant covers the member's own expense
// on any asset; SELF covers only assigned periods; structural grants stay
// unsupported; a role without the cell is FORBIDDEN.
func TestExpenseHistoryRuleCoversOwnExpensesOnAssignedAssets(t *testing.T) {
	base := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	at := func(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }
	scope := meterHistoryScope(t)
	member, asset := meterHistoryID(3), meterHistoryID(10)
	period := func(n int, role permission.Role, kind permission.ScopeType, from time.Time) meterPermissionPeriod {
		return meterHistoryPeriod(t, scope, n, role, kind, from, from, time.Time{}, time.Time{})
	}
	cases := []struct {
		name     string
		periods  []meterPermissionPeriod
		assigned []meterAssignedPeriod
		want     fleet.MeterDisposition
		denied   bool
		unknown  bool
	}{
		{name: "finance-org-covers", periods: []meterPermissionPeriod{period(1, permission.Finance, permission.OrgScope, at(-60))}, want: fleet.MeterApply},
		{name: "owner-org-covers", periods: []meterPermissionPeriod{period(1, permission.Owner, permission.OrgScope, at(-60))}, want: fleet.MeterApply},
		{name: "fleet-manager-org-new-grant-held", periods: []meterPermissionPeriod{period(1, permission.FleetManager, permission.OrgScope, at(-10))}, want: fleet.MeterHoldPermissionChanged},
		{name: "driver-self-assigned", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, assigned: []meterAssignedPeriod{{from: at(-60)}}, want: fleet.MeterApply},
		{name: "driver-self-assigned-after-declaration-held", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, assigned: []meterAssignedPeriod{{from: at(-10)}}, want: fleet.MeterHoldPermissionChanged},
		{name: "driver-self-never-assigned-forbidden", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, assigned: []meterAssignedPeriod{}, denied: true},
		{name: "driver-self-without-assignment-source-unavailable", periods: []meterPermissionPeriod{period(1, permission.Driver, permission.SelfScope, at(-120))}, unknown: true},
		{name: "auditor-org-forbidden", periods: []meterPermissionPeriod{period(1, permission.Auditor, permission.OrgScope, at(-60))}, denied: true},
		{name: "approver-org-forbidden", periods: []meterPermissionPeriod{period(1, permission.Approver, permission.OrgScope, at(-60))}, denied: true},
		{name: "supervisor-unit-unsupported", periods: []meterPermissionPeriod{period(1, permission.Supervisor, permission.UnitScope, at(-60))}, unknown: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := historyDisposition(expenseHistoryRule, scope, member, asset, at(-30), at(0), tc.periods, tc.assigned)
			switch {
			case tc.denied:
				var denial *permission.PermissionDenial
				if !errors.As(err, &denial) || denial.Permission() != "exp.expense.record" {
					t.Fatal("expected FORBIDDEN on exp.expense.record", got, err)
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
