package adapters

import (
	"context"
	"time"

	expenseapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// ExpenseAuthority is the signed expense.submit authority (EMP-EXP-002,
// B-BR-EMP-448, EDR-289). It shares the meter authority's real sources:
// memberships, recorded role periods, capability, current devices and the
// FLEET-006 assigned-asset port. Errors use the fleet source vocabulary.
type ExpenseAuthority MeterAuthority

var _ expenseapp.Authority = (*ExpenseAuthority)(nil)

// expenseHistoryRule is the EDR-177 rule applied to exp.expense.record: the
// cell is S for every holder, so a positive ORG rule covers the member's own
// expense on any asset; SELF covers assigned periods (the expense is always
// the author's own record).
var expenseHistoryRule = historyRule{permission: "exp.expense.record", orgCovers: func(c permission.Coverage) bool { return c != permission.Denied }, selfCovers: true}

func (f *ExpenseAuthority) ready() bool {
	return f != nil && !meterMissing(f.Memberships) && !meterMissing(f.Roles) && !meterMissing(f.Capabilities) && f.Devices != nil
}

// CheckExpenseTx proves current authority on the borrowed intake port: the
// current enrolled device (operational for RecordAction), the real asset and
// the current role candidates over the server-resolved record facts. An
// empty ExpenseID is the root selector and considers ORG grants only.
func (f *ExpenseAuthority) CheckExpenseTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, target expenseapp.Target, action expenseapp.Action) error {
	if !f.ready() || meterMissing(db) {
		return fleet.ErrUnavailable
	}
	member := source.MembershipID
	if source.Validate() != nil || source.Scope != scope || legalIdentity(scope, member) != nil {
		return fleet.ErrUnauthenticated
	}
	policy := expenseapp.Policy(action)
	if policy.Permission == "" {
		return fleet.ErrUnavailable
	}
	root := target.ExpenseID == ""
	for _, id := range []string{target.ExpenseID, target.AssetID, target.UnitID, target.CostCentreID} {
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
	write := action == expenseapp.RecordAction
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
	// The record is the member's own expense with its stored structural
	// snapshot (BR-AUTH-005); the root is the member's own admission only.
	facts := permission.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String(), AuthoredBy: member}
	if !root {
		facts.ID, facts.AssetID, facts.UnitID, facts.CostCentreID = target.ExpenseID, target.AssetID, target.UnitID, target.CostCentreID
	}
	_, err = permission.CoveringRules(scope, member, assignments, policy.Permission, facts, now)
	return err
}

// ClassifyExpenseTx decides a new direct expense from retained role and
// assignment history with the unchanged EDR-118 continuity (B-BR-EMP-448).
func (f *ExpenseAuthority) ClassifyExpenseTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, asset string, declared time.Time) (expenseapp.Disposition, error) {
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
	var assigned []meterAssignedPeriod
	if f.Assignee != nil && !meterMissing(f.Assignee.Assignments) && selfRecorded(scope, recorded, expenseHistoryRule.permission) {
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
	disposition, err := historyDisposition(expenseHistoryRule, scope, member, asset, declared, now, periods, assigned)
	if err != nil {
		return "", err
	}
	switch disposition {
	case fleet.MeterApply:
		return expenseapp.Apply, nil
	case fleet.MeterHoldPermissionChanged:
		return expenseapp.HoldPermissionChanged, nil
	case fleet.MeterHoldDisposalEnded:
		return expenseapp.HoldDisposalEnded, nil
	}
	return "", fleet.ErrUnavailable
}
