package adapters

import (
	"context"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fuelapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// FuelingAuthority is the signed fueling.submit authority (EMP-FUEL-002,
// B-BR-EMP-266, EDR-177). It shares the meter authority's real sources:
// memberships, recorded role periods, capability, current devices and the
// FLEET-006 assigned-asset port. Errors use the fleet source vocabulary.
type FuelingAuthority MeterAuthority

var _ fuelapp.Authority = (*FuelingAuthority)(nil)

// fuelingHistoryRule is the EDR-177 rule of fuel.fueling.record: the cell is
// S for every holder, so a positive ORG rule covers the member's own new
// fueling; SELF covers assigned periods only for an own-responsible fueling.
func fuelingHistoryRule(selfResponsible bool) historyRule {
	return historyRule{permission: "fuel.fueling.record", orgCovers: func(c permission.Coverage) bool { return c != permission.Denied }, selfCovers: selfResponsible}
}

func (f *FuelingAuthority) ready() bool {
	return f != nil && !meterMissing(f.Memberships) && !meterMissing(f.Roles) && !meterMissing(f.Capabilities) && f.Devices != nil
}

// CheckFuelingTx proves current authority on the borrowed intake port: the
// current enrolled device (operational for RecordAction), the real target and
// the current role candidates over the server-resolved record facts. An empty
// FuelingID is the root selector and considers ORG grants only.
func (f *FuelingAuthority) CheckFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, target fuelapp.Target, action fuelapp.Action) error {
	if !f.ready() || meterMissing(db) {
		return fleet.ErrUnavailable
	}
	member := source.MembershipID
	if source.Validate() != nil || source.Scope != scope || legalIdentity(scope, member) != nil {
		return fleet.ErrUnauthenticated
	}
	policy := fuelapp.Policy(action)
	if policy.Permission == "" {
		return fleet.ErrUnavailable
	}
	root := target.FuelingID == ""
	for _, id := range []string{target.FuelingID, target.AssetID, target.UnitID, target.CostCentreID} {
		if id != "" && legalIdentity(scope, id) != nil {
			return fleet.ErrUnavailable
		}
	}
	if !root && target.AssetID == "" || root && (target.AssetID != "" || target.UnitID != "" || target.CostCentreID != "") {
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
	write := action == fuelapp.RecordAction
	if _, err := g.currentMeterDevice(ctx, db, scope, source, member, write); err != nil {
		return err
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
	// The record is the member's own fueling with its stored structural
	// snapshot (BR-AUTH-005); the root is the member's own admission only.
	facts := permission.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String(), AuthoredBy: member}
	if !root {
		facts.ID, facts.AssetID, facts.UnitID, facts.CostCentreID = target.FuelingID, target.AssetID, target.UnitID, target.CostCentreID
	}
	_, err = permission.CoveringRules(scope, member, assignments, policy.Permission, facts, now)
	return err
}

// ClassifyFuelingTx decides a new fueling from retained role and assignment
// history with the unchanged EDR-118 continuity (B-BR-EMP-266).
func (f *FuelingAuthority) ClassifyFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, asset string, selfResponsible bool, declared time.Time) (fuelapp.Disposition, error) {
	return f.classifyFueling(ctx, db, scope, source, asset, selfResponsible, declared, true)
}

// ClassifyFuelingForPlanHoldTx checks the recorded source without requiring
// today's operational capability. The state intake owns the prior-plan proof.
func (f *FuelingAuthority) ClassifyFuelingForPlanHoldTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, asset string,
	selfResponsible bool, declared time.Time,
) (fuelapp.Disposition, error) {
	return f.classifyFueling(ctx, db, scope, source, asset, selfResponsible, declared, false)
}

func (f *FuelingAuthority) classifyFueling(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, asset string,
	selfResponsible bool, declared time.Time, operational bool,
) (fuelapp.Disposition, error) {
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
	if _, err := g.currentMeterDevice(ctx, db, scope, source, member, operational); err != nil {
		return "", err
	}
	if err := meterTarget(ctx, db, scope, asset); err != nil {
		return "", err
	}
	recorded, err := reader.RecordedPeriodsDBTX(ctx, db, scope, member)
	if err != nil {
		return "", fleet.ErrUnavailable
	}
	rule := fuelingHistoryRule(selfResponsible)
	var assigned []meterAssignedPeriod
	if selfResponsible && f.Assignee != nil && !meterMissing(f.Assignee.Assignments) && selfRecorded(scope, recorded, rule.permission) {
		spans, err := f.Assignee.Assignments.AssignmentPeriodsDBTX(ctx, db, scope, member, asset)
		if err != nil {
			return "", fleet.ErrUnavailable
		}
		assigned = make([]meterAssignedPeriod, 0, len(spans))
		for _, span := range spans {
			assigned = append(assigned, meterAssignedPeriod{from: span.From, to: span.To, disposalEnded: span.DisposalEnded})
		}
	}
	now, err := g.currentMeterDevice(ctx, db, scope, source, member, operational)
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
		return fuelapp.Apply, nil
	case fleet.MeterHoldPermissionChanged:
		return fuelapp.HoldPermissionChanged, nil
	case fleet.MeterHoldDisposalEnded:
		return fuelapp.HoldDisposalEnded, nil
	}
	return "", fleet.ErrUnavailable
}
