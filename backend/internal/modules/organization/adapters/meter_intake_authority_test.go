package adapters

import (
	"errors"
	"fmt"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func meterHistoryID(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }
func meterHistoryScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(meterHistoryID(65))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
func meterHistoryPeriod(t *testing.T, scope kernel.OrgScope, n int, role permission.Role, kind permission.ScopeType, created, from, to, ended time.Time) meterPermissionPeriod {
	t.Helper()
	target := scope.OrgID().String()
	if kind == permission.SelfScope {
		target = meterHistoryID(3)
	}
	revision := int64(1)
	if !ended.IsZero() {
		revision = 2
	}
	assignment, err := permission.RehydrateAssignment(scope, permission.AssignmentSnapshot{OrgID: scope.OrgID(), ID: meterHistoryID(n), MembershipID: meterHistoryID(3), AssignedBy: meterHistoryID(9), Role: role, Scope: kind, TargetID: target, ValidFrom: from, ValidTo: to, EndedAt: ended, Reason: "Synthetic retained permission period", Revision: revision})
	if err != nil {
		t.Fatal("invalid value fixture", err)
	}
	return meterPermissionPeriod{assignment: assignment, createdAt: created}
}

func TestMeterHistoryUsesContinuousActualCoverageAndNeverDeviceAuthority(t *testing.T) {
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for _, name := range []string{"continuous", "new_grant", "removed", "declared_after_removal", "gap_regrant", "contiguous_replacement", "overlap_replacement", "future_only", "future_declaration", "future_after_known_end", "denied_only", "self_only", "self_plus_org", "cancelled_scheduled", "created_after_declared"} {
		t.Run(name, func(t *testing.T) {
			scope := meterHistoryScope(t)
			at := func(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }
			period := func(n int, role permission.Role, kind permission.ScopeType, from, to, ended time.Time) meterPermissionPeriod {
				return meterHistoryPeriod(t, scope, n, role, kind, from, from, to, ended)
			}
			declared := at(-30)
			want := fleet.MeterApply
			var wantError error
			periods := []meterPermissionPeriod{period(101, permission.Admin, permission.OrgScope, at(-60), time.Time{}, time.Time{})}
			switch name {
			case "new_grant":
				periods[0] = period(101, permission.Admin, permission.OrgScope, at(-10), time.Time{}, time.Time{})
				want = fleet.MeterHoldPermissionChanged
			case "removed":
				periods[0] = period(101, permission.Admin, permission.OrgScope, at(-60), time.Time{}, at(-10))
				want = fleet.MeterHoldPermissionChanged
			case "declared_after_removal":
				periods[0] = period(101, permission.Admin, permission.OrgScope, at(-60), time.Time{}, at(-40))
				want = fleet.MeterHoldPermissionChanged
			case "gap_regrant":
				periods = []meterPermissionPeriod{period(101, permission.Admin, permission.OrgScope, at(-60), time.Time{}, at(-20)), period(102, permission.Admin, permission.OrgScope, at(-10), time.Time{}, time.Time{})}
				want = fleet.MeterHoldPermissionChanged
			case "contiguous_replacement":
				periods = []meterPermissionPeriod{period(101, permission.Admin, permission.OrgScope, at(-60), at(-10), time.Time{}), period(102, permission.Supervisor, permission.OrgScope, at(-10), time.Time{}, time.Time{})}
			case "overlap_replacement":
				periods = []meterPermissionPeriod{period(101, permission.Admin, permission.OrgScope, at(-60), at(-10), time.Time{}), period(102, permission.FleetManager, permission.OrgScope, at(-20), time.Time{}, time.Time{})}
			case "future_only":
				periods = []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Admin, permission.OrgScope, at(-1), at(10), time.Time{}, time.Time{})}
				want = ""
				wantError = permission.ErrPermissionDenied
			case "future_declaration":
				declared = at(1000)
			case "future_after_known_end":
				declared = at(1000)
				periods[0] = period(101, permission.Admin, permission.OrgScope, at(-60), at(30), time.Time{})
				want = fleet.MeterHoldPermissionChanged
			case "denied_only":
				periods[0] = period(101, permission.Finance, permission.OrgScope, at(-60), time.Time{}, time.Time{})
				want = ""
				wantError = permission.ErrPermissionDenied
			case "self_only":
				periods[0] = period(101, permission.Driver, permission.SelfScope, at(-60), time.Time{}, time.Time{})
				want = ""
				wantError = fleet.ErrUnavailable
			case "self_plus_org":
				periods = append(periods, period(102, permission.Driver, permission.SelfScope, at(-60), time.Time{}, time.Time{}))
			case "cancelled_scheduled":
				periods = []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Admin, permission.OrgScope, at(-60), at(10), time.Time{}, at(-5))}
				want = ""
				wantError = permission.ErrPermissionDenied
			case "created_after_declared":
				periods[0] = meterHistoryPeriod(t, scope, 101, permission.Admin, permission.OrgScope, at(-10), at(-60), time.Time{}, time.Time{})
				want = fleet.MeterHoldPermissionChanged
			}
			actual, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), declared, base, periods)
			if actual != want || !errors.Is(err, wantError) {
				t.Fatal("history classification inferred authority or lost retained evidence", actual, err)
			}
		})
	}
}

func TestMeterHistoryUnknownSourceIsNotNeverAuthorized(t *testing.T) {
	scope := meterHistoryScope(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	if _, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), now.Add(-time.Hour), now, nil); !errors.Is(err, fleet.ErrUnavailable) {
		t.Fatal("unknown history became never-authorized proof")
	}
	if _, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), now.Add(-time.Hour), now, []meterPermissionPeriod{}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatal("complete empty history was not refused")
	}
	period := meterHistoryPeriod(t, scope, 101, permission.Admin, permission.OrgScope, now.Add(-time.Hour), now.Add(-time.Hour), time.Time{}, time.Time{})
	period.createdAt = time.Time{}
	if _, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), now.Add(-time.Minute), now, []meterPermissionPeriod{period}); !errors.Is(err, fleet.ErrUnavailable) {
		t.Fatal("missing recorded creation became historical authority")
	}
}
