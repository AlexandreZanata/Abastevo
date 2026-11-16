package adapters

import (
	"context"
	"errors"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	incidents "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	memberdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// ReportAuthority is the signed check.incident.report authority
// (EMP-CHECK-003, EDR-238). It shares the meter authority's real sources:
// memberships, recorded role periods, capability, current devices and the
// FLEET-006 assigned-asset port. Errors use the fleet vocabulary.
type ReportAuthority MeterAuthority

var _ incidents.ReportAuthority = (*ReportAuthority)(nil)

// reportHistoryRule is the EDR-177 rule applied to check.incident.report:
// the cell is S for every holder, so a positive ORG rule covers the member's
// own new report on any asset of the organization, and SELF covers only the
// assets assigned to the member at the declared instant.
var reportHistoryRule = historyRule{permission: incidents.ReportPermission, orgCovers: func(c permission.Coverage) bool { return c != permission.Denied }, selfCovers: true}

func (f *ReportAuthority) ready() bool {
	return f != nil && !meterMissing(f.Memberships) && !meterMissing(f.Roles) && !meterMissing(f.Capabilities) && f.Devices != nil
}

// CheckReportTx proves current authority on the borrowed intake port: the
// current enrolled device (operational for a write), the real asset and the
// current role candidates over the server-resolved record facts (the stored
// reporter when given). An empty ReportID is the root selector and considers
// ORG grants only.
func (f *ReportAuthority) CheckReportTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, target incidents.ReportTarget, action incidents.ReportAction) error {
	return f.checkRecordTx(ctx, db, scope, source, recordTarget{ID: target.ReportID, AssetID: target.AssetID, UnitID: target.UnitID, CostCentreID: target.CostCentreID,
		AuthorMembershipID: target.AuthorMembershipID}, incidents.ReportPolicy(action), action == incidents.ReportWrite)
}

// recordTarget names the server-resolved facts of a member's own signed
// record (a report or a checklist run); an empty ID is the root selector.
type recordTarget struct {
	ID, AssetID, UnitID, CostCentreID, AuthorMembershipID string
}

// checkRecordTx is the shared own-record intake check of the S-cell
// maintenance keys (check.incident.report, check.checklist.execute).
func (f *ReportAuthority) checkRecordTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, target recordTarget, policy access.Policy, write bool) error {
	if !f.ready() || meterMissing(db) {
		return fleet.ErrUnavailable
	}
	member := source.MembershipID
	if source.Validate() != nil || source.Scope != scope || legalIdentity(scope, member) != nil {
		return fleet.ErrUnauthenticated
	}
	if policy.Permission == "" {
		return fleet.ErrUnavailable
	}
	root := target.ID == ""
	for _, id := range []string{target.ID, target.AssetID, target.UnitID, target.CostCentreID, target.AuthorMembershipID} {
		if id != "" && legalIdentity(scope, id) != nil {
			return fleet.ErrUnavailable
		}
	}
	if root && (target.AssetID != "" || target.UnitID != "" || target.CostCentreID != "" || target.AuthorMembershipID != "") || !root && target.AssetID == "" {
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
	facts := permission.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String(), AuthoredBy: member}
	if !root {
		facts.ID, facts.AssetID, facts.UnitID, facts.CostCentreID = target.ID, target.AssetID, target.UnitID, target.CostCentreID
		if target.AuthorMembershipID != "" {
			facts.AuthoredBy = target.AuthorMembershipID
		}
	}
	_, err = permission.CoveringRules(scope, member, assignments, policy.Permission, facts, now)
	return err
}

// ClassifyReportTx decides a new report from retained role and assignment
// history with the unchanged EDR-118 continuity.
func (f *ReportAuthority) ClassifyReportTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, asset string, declared time.Time) (incidents.ReportDisposition, error) {
	return f.classifyRecordTx(ctx, db, scope, source, asset, declared, reportHistoryRule)
}

// classifyRecordTx is the shared EDR-118 classification of a new own record.
func (f *ReportAuthority) classifyRecordTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, asset string, declared time.Time, rule historyRule) (incidents.ReportDisposition, error) {
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
	if f.Assignee != nil && !meterMissing(f.Assignee.Assignments) && selfRecorded(scope, recorded, rule.permission) {
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
		return incidents.ReportApply, nil
	case fleet.MeterHoldPermissionChanged:
		return incidents.ReportPermissionMoved, nil
	case fleet.MeterHoldDisposalEnded:
		return incidents.ReportDisposalEnded, nil
	}
	return "", fleet.ErrUnavailable
}

// IncidentAuthority proves the company policies of EMP-CHECK-003
// (EDR-241): the organization queue (organization target, so only an ORG
// grant covers it), one report (structural grants through the report's
// stored asset, unit and cost centre), a handling step (fresh proof, ACTIVE
// root) and the member's own list (own target = the membership).
type IncidentAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Structure    *ResourceStructure
}

var _ incidents.IncidentAuthority = (*IncidentAuthority)(nil)

func (g *IncidentAuthority) CheckIncidentTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor incidents.IncidentActor, member, target string, action incidents.IncidentAction) (incidents.Handler, error) {
	if g == nil || g.Structure != nil && g.Structure.Kind != IncidentStructure || legalIdentity(scope, target) != nil {
		return incidents.Handler{}, incidents.ErrUnavailable
	}
	var scoped *companyAuthority
	switch action {
	case incidents.IncidentQueue:
		if target != scope.OrgID().String() {
			return incidents.Handler{}, incidents.ErrUnavailable
		}
	case incidents.IncidentRead, incidents.IncidentDecide:
		if target == scope.OrgID().String() {
			return incidents.Handler{}, incidents.ErrUnavailable
		}
		scoped = &companyAuthority{Structure: g.Structure}
	case incidents.IncidentOwn:
		if target != member {
			return incidents.Handler{}, incidents.ErrUnavailable
		}
		scoped = &companyAuthority{OwnTarget: true}
	default:
		return incidents.Handler{}, incidents.ErrUnavailable
	}
	return g.checkTx(ctx, tx, scope, actor, member, target, incidents.IncidentPolicy(action), action == incidents.IncidentDecide, scoped)
}

// checkTx is the shared company check of the maintenance policies: the
// account/proof → capability → root → membership/role order with the given
// structural or own lookups, and for a handling command an ACTIVE root and
// the actual handler (account, membership, active canonical employee).
func (g *IncidentAuthority) checkTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor incidents.IncidentActor, member, target string, policy access.Policy, command bool, scoped *companyAuthority) (incidents.Handler, error) {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || legalIdentity(scope, target) != nil {
		return incidents.Handler{}, incidents.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return incidents.Handler{}, incidents.ErrUnauthenticated
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}
	if scoped != nil {
		guard.Structure, guard.Assignee, guard.OwnTarget = scoped.Structure, scoped.Assignee, scoped.OwnTarget
	}
	if policy.Validate() != nil || database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return incidents.Handler{}, incidents.ErrUnavailable
	}
	proof := org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}
	if err := guard.check(ctx, tx, scope, proof, member, target, policy); err != nil {
		return incidents.Handler{}, incidentAuthorityError(err)
	}
	if !command {
		return incidents.Handler{AccountID: actor.AccountID, MembershipID: member}, nil
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
		return incidents.Handler{}, incidents.ErrUnavailable
	}
	if state != "ACTIVE" {
		return incidents.Handler{}, access.ErrSetupIncomplete
	}
	current, err := g.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
	if err != nil {
		return incidents.Handler{}, access.ErrNoMembership
	}
	membership, err := current.Snapshot(scope)
	if err != nil || membership.ID != member {
		return incidents.Handler{}, access.ErrNoMembership
	}
	employee, err := g.Memberships.GetEmployeeTx(ctx, tx, scope, membership.EmployeeID)
	if err != nil {
		return incidents.Handler{}, access.ErrNoMembership
	}
	person, err := employee.Snapshot(scope)
	if err != nil || person.State != memberdomain.EmployeeActive || person.MergedInto != "" {
		return incidents.Handler{}, access.ErrNoMembership
	}
	if err := guard.check(ctx, tx, scope, proof, member, target, policy); err != nil {
		return incidents.Handler{}, incidentAuthorityError(err)
	}
	return incidents.Handler{AccountID: actor.AccountID, MembershipID: membership.ID, EmployeeID: person.ID}, nil
}

// incidentAuthorityError keeps the shared access vocabulary the routes map.
func incidentAuthorityError(err error) error {
	switch {
	case errors.Is(err, org.ErrUnauthenticated), errors.Is(err, members.ErrEmployeeUnauthenticated):
		return incidents.ErrUnauthenticated
	case errors.Is(err, org.ErrUnavailable), errors.Is(err, members.ErrRegisterUnavailable):
		return incidents.ErrUnavailable
	case errors.Is(err, org.ErrNotFound), errors.Is(err, members.ErrEmployeeNotFound):
		return incidents.ErrNotFound
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	}
	return err
}
