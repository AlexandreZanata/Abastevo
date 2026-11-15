package adapters

import (
	"errors"
	"fmt"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
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
	if kind.Structural() {
		target = meterHistoryID(70)
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
			actual, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), declared, base, periods, nil)
			if actual != want || !errors.Is(err, wantError) {
				t.Fatal("history classification inferred authority or lost retained evidence", actual, err)
			}
		})
	}
}

func TestMeterHistoryUnknownSourceIsNotNeverAuthorized(t *testing.T) {
	scope := meterHistoryScope(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	if _, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), now.Add(-time.Hour), now, nil, nil); !errors.Is(err, fleet.ErrUnavailable) {
		t.Fatal("unknown history became never-authorized proof")
	}
	if _, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), now.Add(-time.Hour), now, []meterPermissionPeriod{}, nil); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatal("complete empty history was not refused")
	}
	period := meterHistoryPeriod(t, scope, 101, permission.Admin, permission.OrgScope, now.Add(-time.Hour), now.Add(-time.Hour), time.Time{}, time.Time{})
	period.createdAt = time.Time{}
	if _, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), now.Add(-time.Minute), now, []meterPermissionPeriod{period}, nil); !errors.Is(err, fleet.ErrUnavailable) {
		t.Fatal("missing recorded creation became historical authority")
	}
}

// B-BR-EMP-259..262 (EDR-173/174): SELF coverage is the recorded SELF grant
// period intersected with the retained assignment periods of the asset; it is
// united with ORG coverage under the EDR-118 continuity rule.
func TestMeterHistoryResolvesSelfFromRetainedAssignmentPeriods(t *testing.T) {
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	at := func(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }
	open := time.Time{}
	type span = meterAssignedPeriod
	deny, unavailable := permission.ErrPermissionDenied, fleet.ErrUnavailable
	for _, c := range []struct {
		name     string
		periods  func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod
		assigned []meterAssignedPeriod
		declared time.Time
		want     fleet.MeterDisposition
		err      error
	}{
		{name: "assigned_continuous", assigned: []span{{from: at(-50)}}, declared: at(-30), want: fleet.MeterApply},
		{name: "never_assigned", assigned: []span{}, declared: at(-30), err: deny},
		{name: "no_assignment_source", assigned: nil, declared: at(-30), err: unavailable},
		{name: "ended_before_receipt", assigned: []span{{from: at(-50), to: at(-10)}}, declared: at(-30), want: fleet.MeterHoldPermissionChanged},
		{name: "ended_before_declaration", assigned: []span{{from: at(-50), to: at(-40)}}, declared: at(-30), want: fleet.MeterHoldPermissionChanged},
		{name: "started_after_declaration", assigned: []span{{from: at(-20)}}, declared: at(-30), want: fleet.MeterHoldPermissionChanged},
		{name: "scheduled_after_receipt", assigned: []span{{from: at(10)}}, declared: at(-30), err: deny},
		{name: "start_boundary_exact", assigned: []span{{from: at(-30)}}, declared: at(-30), want: fleet.MeterApply},
		{name: "start_boundary_minus_1ns", assigned: []span{{from: at(-30)}}, declared: at(-30).Add(-time.Nanosecond), want: fleet.MeterHoldPermissionChanged},
		{name: "temporary_end_minus_1ns", assigned: []span{{from: at(-50), to: at(30)}}, declared: at(30).Add(-time.Nanosecond), want: fleet.MeterApply},
		{name: "temporary_end_exact", assigned: []span{{from: at(-50), to: at(30)}}, declared: at(30), want: fleet.MeterHoldPermissionChanged},
		{name: "receipt_at_end_exact", assigned: []span{{from: at(-50), to: at(0)}}, declared: at(-30), want: fleet.MeterHoldPermissionChanged},
		{name: "gap_between_assignments", assigned: []span{{from: at(-50), to: at(-35)}, {from: at(-25)}}, declared: at(-40), want: fleet.MeterHoldPermissionChanged},
		{name: "adjacent_assignments", assigned: []span{{from: at(-50), to: at(-20)}, {from: at(-20)}}, declared: at(-40), want: fleet.MeterApply},
		{name: "self_grant_ended", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Driver, permission.SelfScope, at(-60), at(-60), open, at(-10))}
		}, assigned: []span{{from: at(-50)}}, declared: at(-30), want: fleet.MeterHoldPermissionChanged},
		{name: "self_grant_never_overlaps_assignment", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Driver, permission.SelfScope, at(-20), at(-20), open, open)}
		}, assigned: []span{{from: at(-50), to: at(-30)}}, declared: at(-25), err: deny},
		{name: "self_grant_recorded_after_assignment_start", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Driver, permission.SelfScope, at(-20), at(-60), open, open)}
		}, assigned: []span{{from: at(-50)}}, declared: at(-30), want: fleet.MeterHoldPermissionChanged},
		{name: "org_then_self_union", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Admin, permission.OrgScope, at(-60), at(-60), open, at(-20)), meterHistoryPeriod(t, scope, 102, permission.Driver, permission.SelfScope, at(-60), at(-60), open, open)}
		}, assigned: []span{{from: at(-25)}}, declared: at(-40), want: fleet.MeterApply},
		{name: "self_never_assigned_with_unit_grant", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Driver, permission.SelfScope, at(-60), at(-60), open, open), meterHistoryPeriod(t, scope, 102, permission.Supervisor, permission.UnitScope, at(-60), at(-60), open, open)}
		}, assigned: []span{}, declared: at(-30), err: unavailable},
		{name: "driver_org_scope_stays_unsupported", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Driver, permission.OrgScope, at(-60), at(-60), open, open)}
		}, assigned: []span{{from: at(-50)}}, declared: at(-30), err: unavailable},
		{name: "self_grant_by_denied_role", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Finance, permission.SelfScope, at(-60), at(-60), open, open)}
		}, assigned: []span{{from: at(-50)}}, declared: at(-30), err: deny},
		{name: "disposal_ended_declared_after", assigned: []span{{from: at(-50), to: at(-10), disposalEnded: true}}, declared: at(-5), want: fleet.MeterHoldDisposalEnded},
		{name: "disposal_ended_declared_before", assigned: []span{{from: at(-50), to: at(-10), disposalEnded: true}}, declared: at(-30), want: fleet.MeterHoldDisposalEnded},
		{name: "command_ended_declared_after", assigned: []span{{from: at(-50), to: at(-10)}}, declared: at(-5), want: fleet.MeterHoldPermissionChanged},
		{name: "disposal_and_grant_ended", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Driver, permission.SelfScope, at(-60), at(-60), open, at(-5))}
		}, assigned: []span{{from: at(-50), to: at(-10), disposalEnded: true}}, declared: at(-30), want: fleet.MeterHoldPermissionChanged},
		{name: "disposal_end_never_creates_coverage", periods: func(t *testing.T, scope kernel.OrgScope) []meterPermissionPeriod {
			return []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Driver, permission.SelfScope, at(-5), at(-5), open, open)}
		}, assigned: []span{{from: at(-50), to: at(-10), disposalEnded: true}}, declared: at(-3), err: deny},
		{name: "invalid_open_start", assigned: []span{{}}, declared: at(-30), err: unavailable},
		{name: "invalid_empty_period", assigned: []span{{from: at(-20), to: at(-20)}}, declared: at(-30), err: unavailable},
	} {
		t.Run(c.name, func(t *testing.T) {
			scope := meterHistoryScope(t)
			periods := []meterPermissionPeriod{meterHistoryPeriod(t, scope, 101, permission.Driver, permission.SelfScope, at(-60), at(-60), open, open)}
			if c.periods != nil {
				periods = c.periods(t, scope)
			}
			actual, err := meterHistoryDisposition(scope, meterHistoryID(3), meterHistoryID(10), c.declared, base, periods, c.assigned)
			if actual != c.want || !errors.Is(err, c.err) || (c.err == nil) != (err == nil) {
				t.Fatalf("SELF history classification: got %q %v, want %q %v", actual, err, c.want, c.err)
			}
		})
	}
}

// ORG-only members keep their previous queries: only a recorded SELF grant
// with a positive rule needs the assignment periods (EDR-173).
func TestMeterSelfRecordedOnlyForPositiveSelfGrants(t *testing.T) {
	scope := meterHistoryScope(t)
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	record := func(role permission.Role, kind permission.ScopeType) access.RecordedRolePeriod {
		p := meterHistoryPeriod(t, scope, 101, role, kind, at, at, time.Time{}, time.Time{})
		return access.RecordedRolePeriod{Assignment: p.assignment, CreatedAt: at}
	}
	for _, c := range []struct {
		name     string
		recorded []access.RecordedRolePeriod
		want     bool
	}{
		{"none", nil, false},
		{"org_only", []access.RecordedRolePeriod{record(permission.Admin, permission.OrgScope), record(permission.Driver, permission.OrgScope)}, false},
		{"self_denied_role", []access.RecordedRolePeriod{record(permission.Finance, permission.SelfScope)}, false},
		{"structural_only", []access.RecordedRolePeriod{record(permission.Supervisor, permission.UnitScope)}, false},
		{"self_driver", []access.RecordedRolePeriod{record(permission.Admin, permission.OrgScope), record(permission.Driver, permission.SelfScope)}, true},
		{"self_admin", []access.RecordedRolePeriod{record(permission.Admin, permission.SelfScope)}, true},
		{"invalid_evidence", []access.RecordedRolePeriod{{}}, true},
	} {
		if got := meterSelfRecorded(scope, c.recorded); got != c.want {
			t.Fatal("SELF period detection", c.name, got)
		}
	}
}
