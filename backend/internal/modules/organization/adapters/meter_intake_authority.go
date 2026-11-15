package adapters

import (
	"context"
	"slices"
	"sort"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

var _ fleet.MeterIntakeAuthority = (*MeterAuthority)(nil)

type meterPermissionPeriod struct {
	assignment permission.Assignment
	createdAt  time.Time
}

type meterCoverageInterval struct{ from, to time.Time }

// meterAssignedPeriod is one retained period [from, to) in which the asset was
// assigned to the member (zero to: open); disposalEnded marks an end caused
// by the asset's disposal (EDR-173/174).
type meterAssignedPeriod struct {
	from, to      time.Time
	disposalEnded bool
}

// meterHistoryDisposition classifies one reading from complete retained
// evidence. A nil assigned means no assigned-asset source: SELF grants stay
// unsupported (B-BR-EMP-262). ORG grants with an O cell cover the asset over
// their recorded period (B-BR-EMP-160); SELF grants cover it where their
// recorded period meets a retained assignment period (B-BR-EMP-259).
func meterHistoryDisposition(scope kernel.OrgScope, member, asset string, declared, received time.Time, periods []meterPermissionPeriod, assigned []meterAssignedPeriod) (fleet.MeterDisposition, error) {
	if scope.Validate() != nil || periods == nil || received.IsZero() || received.UTC().Year() < 1 || received.UTC().Year() > 9999 || declared.UTC().Year() < 1 || declared.UTC().Year() > 9999 {
		return "", fleet.ErrUnavailable
	}
	for _, id := range []string{member, asset} {
		parsed, err := kernel.ParseOrgID(id)
		if err != nil || parsed.String() != id {
			return "", fleet.ErrUnavailable
		}
	}
	for _, period := range assigned {
		if period.from.IsZero() || !period.to.IsZero() && !period.from.Before(period.to) || period.disposalEnded && period.to.IsZero() {
			return "", fleet.ErrUnavailable
		}
	}
	// intervals is the actual coverage; opened is the same coverage with the
	// assignment ends caused by the asset's disposal removed. It can only turn
	// a hold into a disposal-only hold label, never apply (EDR-174).
	intervals := make([]meterCoverageInterval, 0, len(periods))
	opened := make([]meterCoverageInterval, 0, len(periods))
	unsupported := false
	for _, period := range periods {
		value, err := period.assignment.Snapshot(scope)
		if err != nil || value.MembershipID != member || period.createdAt.IsZero() || period.createdAt.After(received) {
			return "", fleet.ErrUnavailable
		}
		rule, err := permission.SystemCatalog().Lookup(value.Role, "meter.reading.record")
		if err != nil {
			return "", fleet.ErrUnavailable
		}
		if rule.Coverage() == permission.Denied {
			continue
		}
		from := value.ValidFrom
		if period.createdAt.After(from) {
			from = period.createdAt
		}
		to := value.ValidTo
		if !value.EndedAt.IsZero() && (to.IsZero() || value.EndedAt.Before(to)) {
			to = value.EndedAt
		}
		if !to.IsZero() && !from.Before(to) {
			continue
		}
		known := true
		for _, requirement := range rule.Requirements() {
			switch requirement {
			case permission.ActiveMembership, permission.LiveScopedAssignment, permission.OrganizationAndPlan, permission.RecordRules:
			default:
				known = false
			}
		}
		switch {
		case known && value.Scope == permission.OrgScope && rule.Coverage() == permission.AssignedScope:
			interval := meterCoverageInterval{from: from.UTC(), to: to.UTC()}
			intervals, opened = append(intervals, interval), append(opened, interval)
		case known && value.Scope == permission.SelfScope && assigned != nil:
			for _, span := range assigned {
				if interval, ok := meterIntersect(from, to, span.from, span.to); ok {
					intervals = append(intervals, interval)
					if !span.disposalEnded {
						opened = append(opened, interval)
					}
				}
				if span.disposalEnded {
					if interval, ok := meterIntersect(from, to, span.from, time.Time{}); ok {
						opened = append(opened, interval)
					}
				}
			}
		default:
			if !from.After(received) {
				unsupported = true
			}
		}
	}
	disposition, err := meterCoverageDisposition(declared, received, intervals, unsupported)
	if err != nil || disposition != fleet.MeterHoldPermissionChanged {
		return disposition, err
	}
	if label, err := meterCoverageDisposition(declared, received, opened, unsupported); err == nil && label == fleet.MeterApply {
		return fleet.MeterHoldDisposalEnded, nil
	}
	return disposition, nil
}

// meterSelfRecorded reports whether any recorded period is a SELF grant whose
// role has a positive meter.reading.record rule. Invalid evidence is left to
// meterHistoryDisposition, which refuses it as unavailable.
func meterSelfRecorded(scope kernel.OrgScope, recorded []access.RecordedRolePeriod) bool {
	for _, period := range recorded {
		value, err := period.Assignment.Snapshot(scope)
		if err != nil {
			return true
		}
		if value.Scope != permission.SelfScope {
			continue
		}
		if rule, err := permission.SystemCatalog().Lookup(value.Role, "meter.reading.record"); err != nil || rule.Coverage() != permission.Denied {
			return true
		}
	}
	return false
}

// meterIntersect returns [from, to) of two half-open periods (zero to: open).
func meterIntersect(aFrom, aTo, bFrom, bTo time.Time) (meterCoverageInterval, bool) {
	from, to := aFrom.UTC(), aTo.UTC()
	if bFrom.After(from) {
		from = bFrom.UTC()
	}
	if to.IsZero() || !bTo.IsZero() && bTo.Before(to) {
		to = bTo.UTC()
	}
	return meterCoverageInterval{from: from, to: to}, to.IsZero() || from.Before(to)
}

// meterCoverageDisposition applies the EDR-118 rule to coverage intervals:
// never effective by receipt is FORBIDDEN (or unavailable when an unsupported
// grant was effective), current coverage continuous across the declared
// instant and the receipt applies, anything else is held.
func meterCoverageDisposition(declared, received time.Time, intervals []meterCoverageInterval, unsupported bool) (fleet.MeterDisposition, error) {
	current, past := false, false
	for _, interval := range intervals {
		if !interval.from.After(received) {
			past = true
			if interval.to.IsZero() || received.Before(interval.to) {
				current = true
			}
		}
	}
	if !past {
		if unsupported {
			return "", fleet.ErrUnavailable
		}
		return "", permission.DenyPermission("meter.reading.record")
	}
	if !current {
		return fleet.MeterHoldPermissionChanged, nil
	}
	start, end := declared.UTC(), received.UTC()
	if end.Before(start) {
		start, end = end, start
	}
	sorted := slices.Clone(intervals)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].from.Before(sorted[j].from) })
	covered := false
	var until time.Time
	for _, interval := range sorted {
		if !covered {
			if interval.from.After(start) || !interval.to.IsZero() && !start.Before(interval.to) {
				continue
			}
			covered = true
			until = interval.to
		} else {
			if until.IsZero() {
				return fleet.MeterApply, nil
			}
			if interval.from.After(until) {
				break
			}
			if interval.to.IsZero() || interval.to.After(until) {
				until = interval.to
			}
		}
		if until.IsZero() || end.Before(until) {
			return fleet.MeterApply, nil
		}
	}
	return fleet.MeterHoldPermissionChanged, nil
}

func (g *MeterAuthority) ClassifyMeterTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source fleet.MeterSource, member string, command fleet.MeterReadingCommand) (fleet.MeterDisposition, error) {
	if g == nil || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || g.Devices == nil || meterMissing(db) {
		return "", fleet.ErrUnavailable
	}
	if source.Validate(scope) != nil || source.Device == nil || source.Device.MembershipID != member || legalIdentity(scope, member) != nil {
		return "", fleet.ErrUnauthenticated
	}
	in, err := command.Snapshot()
	if err != nil || legalIdentity(scope, in.AssetID) != nil {
		return "", fleet.ErrUnavailable
	}
	reader, ok := g.Roles.(access.RecordedRolePeriods)
	if !ok || meterMissing(reader) {
		return "", fleet.ErrUnavailable
	}
	if err := database.ValidateOrgDBTX(ctx, db, scope); err != nil {
		return "", fleet.ErrUnavailable
	}
	if _, err := g.currentMeterDevice(ctx, db, scope, *source.Device, member, true); err != nil {
		return "", err
	}
	if err := meterTarget(ctx, db, scope, in.AssetID); err != nil {
		return "", err
	}
	recorded, err := reader.RecordedPeriodsDBTX(ctx, db, scope, member)
	if err != nil {
		return "", fleet.ErrUnavailable
	}
	// EDR-173: the retained assignment periods of this asset, read through
	// the composed FLEET-006 port on the borrowed intake port, only when a
	// recorded SELF grant could cover it (ORG-only members keep their previous
	// queries). Without that source SELF history stays unavailable (nil),
	// never "never assigned".
	var assigned []meterAssignedPeriod
	if g.Assignee != nil && !meterMissing(g.Assignee.Assignments) && meterSelfRecorded(scope, recorded) {
		spans, err := g.Assignee.Assignments.AssignmentPeriodsDBTX(ctx, db, scope, member, in.AssetID)
		if err != nil {
			return "", fleet.ErrUnavailable
		}
		assigned = make([]meterAssignedPeriod, 0, len(spans))
		for _, span := range spans {
			assigned = append(assigned, meterAssignedPeriod{from: span.From, to: span.To, disposalEnded: span.DisposalEnded})
		}
	}
	// Current device/root proof and server time are sampled after every source
	// read. Historical periods only choose APPLY versus a real held record.
	now, err := g.currentMeterDevice(ctx, db, scope, *source.Device, member, true)
	if err != nil {
		return "", err
	}
	var periods []meterPermissionPeriod
	if recorded != nil {
		periods = make([]meterPermissionPeriod, 0, len(recorded))
	}
	for _, period := range recorded {
		periods = append(periods, meterPermissionPeriod{assignment: period.Assignment, createdAt: period.CreatedAt})
	}
	return meterHistoryDisposition(scope, member, in.AssetID, in.Declared.OccurredAt, now, periods, assigned)
}
