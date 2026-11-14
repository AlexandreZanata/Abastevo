package adapters

import (
	"context"
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

func meterHistoryDisposition(scope kernel.OrgScope, member, asset string, declared, received time.Time, periods []meterPermissionPeriod) (fleet.MeterDisposition, error) {
	if scope.Validate() != nil || periods == nil || received.IsZero() || received.UTC().Year() < 1 || received.UTC().Year() > 9999 || declared.UTC().Year() < 1 || declared.UTC().Year() > 9999 {
		return "", fleet.ErrUnavailable
	}
	for _, id := range []string{member, asset} {
		parsed, err := kernel.ParseOrgID(id)
		if err != nil || parsed.String() != id {
			return "", fleet.ErrUnavailable
		}
	}
	intervals := make([]meterCoverageInterval, 0, len(periods))
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
		if value.Scope != permission.OrgScope || rule.Coverage() != permission.AssignedScope {
			if !from.After(received) {
				unsupported = true
			}
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
		if !known {
			if !from.After(received) {
				unsupported = true
			}
			continue
		}
		intervals = append(intervals, meterCoverageInterval{from: from.UTC(), to: to.UTC()})
	}
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
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].from.Before(intervals[j].from) })
	covered := false
	var until time.Time
	for _, interval := range intervals {
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
	return meterHistoryDisposition(scope, member, in.AssetID, in.Declared.OccurredAt, now, periods)
}
