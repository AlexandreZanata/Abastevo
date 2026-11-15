package adapters

import (
	"context"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// TripAuthority is the signed trip.start and trip.finish authority
// (EMP-TRIP-002, B-BR-EMP-316, EDR-206). It shares the meter authority's real
// sources: memberships, recorded role periods, capability, current devices
// and the FLEET-006 assigned-asset port. Errors use the fleet vocabulary.
type TripAuthority MeterAuthority

var _ tripapp.Authority = (*TripAuthority)(nil)

// tripHistoryRule is the EDR-177 rule applied to trip.trip.start: the cell is
// S for every holder, so a positive ORG rule covers the member's own trip;
// SELF covers assigned periods only when the member's employee is the driver.
func tripHistoryRule(selfDriver bool) historyRule {
	return historyRule{permission: "trip.trip.start", orgCovers: func(c permission.Coverage) bool { return c != permission.Denied }, selfCovers: selfDriver}
}

func (f *TripAuthority) ready() bool {
	return f != nil && !meterMissing(f.Memberships) && !meterMissing(f.Roles) && !meterMissing(f.Capabilities) && f.Devices != nil
}

// CheckTripTx proves current authority on the borrowed intake port: the
// current enrolled device (operational for RecordAction), the real target and
// the current role candidates over the server-resolved record facts (the
// stored author when given). An empty TripID is the root selector and
// considers ORG grants only; a TripID without an asset names a target that is
// not stored here, which admits only the current device (its answer is a
// pending dependency that reveals nothing).
func (f *TripAuthority) CheckTripTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, target tripapp.Target, action tripapp.Action) error {
	if !f.ready() || meterMissing(db) {
		return fleet.ErrUnavailable
	}
	member := source.MembershipID
	if source.Validate() != nil || source.Scope != scope || legalIdentity(scope, member) != nil {
		return fleet.ErrUnauthenticated
	}
	policy := tripapp.Policy(action)
	if policy.Permission == "" {
		return fleet.ErrUnavailable
	}
	root := target.TripID == ""
	for _, id := range []string{target.TripID, target.AssetID, target.UnitID, target.CostCentreID, target.AuthorMembershipID} {
		if id != "" && legalIdentity(scope, id) != nil {
			return fleet.ErrUnavailable
		}
	}
	if root && (target.AssetID != "" || target.UnitID != "" || target.CostCentreID != "" || target.AuthorMembershipID != "") ||
		!root && target.AssetID == "" && (target.UnitID != "" || target.CostCentreID != "" || target.AuthorMembershipID != "") {
		return fleet.ErrUnavailable
	}
	roles, ok := f.Roles.(meterRoleQueries)
	if !ok {
		return fleet.ErrUnavailable
	}
	if err := database.ValidateOrgDBTX(ctx, db, scope); err != nil {
		return fleet.ErrUnavailable
	}
	g := (*MeterAuthority)(f)
	write := action == tripapp.RecordAction
	if _, err := g.currentMeterDevice(ctx, db, scope, source, member, write); err != nil {
		return err
	}
	if !root && target.AssetID == "" {
		return nil
	}
	if !root {
		if err := meterTarget(ctx, db, scope, target.AssetID); err != nil {
			return err
		}
	}
	assignments, err := roles.CandidatesDBTX(ctx, db, scope, member)
	if err != nil {
		return err
	}
	if root {
		filtered := make([]permission.Assignment, 0, len(assignments))
		for _, a := range assignments {
			v, err := a.Snapshot(scope)
			if err != nil {
				return fleet.ErrUnavailable
			}
			if v.Scope == permission.OrgScope {
				filtered = append(filtered, a)
			}
		}
		assignments = filtered
	}
	now, err := g.currentMeterDevice(ctx, db, scope, source, member, write)
	if err != nil {
		return err
	}
	// The record is the stored trip (or the member's own new trip) with its
	// structural snapshot (BR-AUTH-005); the root is the member's admission.
	facts := permission.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String(), AuthoredBy: member}
	if !root {
		facts.ID, facts.AssetID, facts.UnitID, facts.CostCentreID = target.TripID, target.AssetID, target.UnitID, target.CostCentreID
		if target.AuthorMembershipID != "" {
			facts.AuthoredBy = target.AuthorMembershipID
		}
	}
	_, err = permission.CoveringRules(scope, member, assignments, policy.Permission, facts, now)
	return err
}

// ClassifyTripTx decides a new trip event from retained role and assignment
// history with the unchanged EDR-118 continuity (B-BR-EMP-316).
func (f *TripAuthority) ClassifyTripTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, asset string, selfDriver bool, declared time.Time) (tripapp.Disposition, error) {
	if !f.ready() || meterMissing(db) {
		return "", fleet.ErrUnavailable
	}
	member := source.MembershipID
	if source.Validate() != nil || source.Scope != scope || legalIdentity(scope, member) != nil {
		return "", fleet.ErrUnauthenticated
	}
	if legalIdentity(scope, asset) != nil || declared.IsZero() {
		return "", fleet.ErrUnavailable
	}
	reader, ok := f.Roles.(access.RecordedRolePeriods)
	if !ok || meterMissing(reader) {
		return "", fleet.ErrUnavailable
	}
	if err := database.ValidateOrgDBTX(ctx, db, scope); err != nil {
		return "", fleet.ErrUnavailable
	}
	g := (*MeterAuthority)(f)
	if _, err := g.currentMeterDevice(ctx, db, scope, source, member, true); err != nil {
		return "", err
	}
	if err := meterTarget(ctx, db, scope, asset); err != nil {
		return "", err
	}
	recorded, err := reader.RecordedPeriodsDBTX(ctx, db, scope, member)
	if err != nil {
		return "", fleet.ErrUnavailable
	}
	rule := tripHistoryRule(selfDriver)
	var assigned []meterAssignedPeriod
	if selfDriver && f.Assignee != nil && !meterMissing(f.Assignee.Assignments) && selfRecorded(scope, recorded, rule.permission) {
		spans, err := f.Assignee.Assignments.AssignmentPeriodsDBTX(ctx, db, scope, member, asset)
		if err != nil {
			return "", fleet.ErrUnavailable
		}
		assigned = make([]meterAssignedPeriod, 0, len(spans))
		for _, span := range spans {
			assigned = append(assigned, meterAssignedPeriod{from: span.From, to: span.To, disposalEnded: span.DisposalEnded})
		}
	}
	now, err := g.currentMeterDevice(ctx, db, scope, source, member, true)
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
	disposition, err := historyDisposition(rule, scope, member, asset, declared, now, periods, assigned)
	if err != nil {
		return "", err
	}
	switch disposition {
	case fleet.MeterApply:
		return tripapp.Apply, nil
	case fleet.MeterHoldPermissionChanged:
		return tripapp.HoldPermissionChanged, nil
	case fleet.MeterHoldDisposalEnded:
		return tripapp.HoldDisposalEnded, nil
	}
	return "", fleet.ErrUnavailable
}
