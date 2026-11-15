package adapters

import (
	"context"
	"errors"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// MissionAuthority supplies current company proof through the mission port
// (EMP-TRIP-008, EDR-231). Creation and the organization list target the
// organization itself (no structure, EDR-161); read, assign and cancel target
// the mission with the MissionStructure kind (stored unit and asset); the own
// list targets the membership itself. Commands also require an ACTIVE root.
type MissionAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Structure    *ResourceStructure
}

var _ tripapp.MissionAuthority = (*MissionAuthority)(nil)

func (g *MissionAuthority) CheckMissionTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor tripapp.MissionActor, member, target string, action tripapp.MissionAction) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || g.Structure != nil && g.Structure.Kind != MissionStructure || legalIdentity(scope, target) != nil {
		return tripapp.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return tripapp.ErrMissionUnauthenticated
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}
	switch action {
	case tripapp.MissionRead, tripapp.MissionManage:
		if target != scope.OrgID().String() {
			guard.Structure = g.Structure
		}
	case tripapp.MissionOwnRead:
		if target != member {
			return tripapp.ErrUnavailable
		}
		guard.OwnTarget = true
	default:
		return tripapp.ErrUnavailable
	}
	policy := tripapp.MissionPolicy(action)
	if policy.Validate() != nil || database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return tripapp.ErrUnavailable
	}
	if err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, policy); err != nil {
		return missionAuthorityError(err)
	}
	if action != tripapp.MissionManage {
		return nil
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
		return tripapp.ErrUnavailable
	}
	if state != "ACTIVE" {
		return access.ErrSetupIncomplete
	}
	return nil
}

// EligibleAssigneeTx answers whether the membership currently holds
// trip.trip.start covering the mission as its own record (B-BR-EMP-355): the
// same coverage CheckMissionResponseTx requires of its response.
func (g *MissionAuthority) EligibleAssigneeTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, assignee string, facts tripapp.MissionFacts) (bool, error) {
	if g == nil || meterMissing(tx) || meterMissing(g.Roles) || legalIdentity(scope, assignee) != nil || legalIdentity(scope, facts.MissionID) != nil || legalIdentity(scope, facts.UnitID) != nil ||
		facts.AssetID != "" && legalIdentity(scope, facts.AssetID) != nil || database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return false, tripapp.ErrUnavailable
	}
	assignments, err := g.Roles.CandidatesTx(ctx, tx, scope, assignee)
	if err != nil {
		return false, tripapp.ErrUnavailable
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return false, tripapp.ErrUnavailable
	}
	resource := permission.ResourceFacts{OrgID: scope.OrgID(), ID: facts.MissionID, AssignedTo: assignee, UnitID: facts.UnitID, AssetID: facts.AssetID}
	if _, err := permission.CoveringRules(scope, assignee, assignments, tripapp.MissionRespondPermission, resource, now); err != nil {
		if errors.Is(err, permission.ErrPermissionDenied) {
			return false, nil
		}
		return false, tripapp.ErrUnavailable
	}
	return true, nil
}

func missionAuthorityError(err error) error {
	switch {
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnauthenticated), errors.Is(err, members.ErrEmployeeUnauthenticated):
		return tripapp.ErrMissionUnauthenticated
	case errors.Is(err, org.ErrUnavailable), errors.Is(err, members.ErrRegisterUnavailable):
		return tripapp.ErrUnavailable
	case errors.Is(err, org.ErrNotFound), errors.Is(err, members.ErrEmployeeNotFound):
		return tripapp.ErrNotFound
	}
	return err
}

var _ tripapp.MissionResponseAuthority = (*TripAuthority)(nil)

// CheckMissionResponseTx proves current trip.trip.start on the enrolled
// device over a mission (EDR-229): the current assignee membership is the
// record's assignee (MissionRespond) or the stored responder its author
// (MissionReceipt), with the stored unit and asset as structure. An empty
// MissionID is the root selector (ORG grants only, the member's own
// admission); a MissionID without a unit names a mission not stored here,
// which admits only the current device.
func (f *TripAuthority) CheckMissionResponseTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, target tripapp.MissionTarget, action tripapp.MissionAction) error {
	g := (*MeterAuthority)(f)
	if !(*TripAuthority)(g).ready() || meterMissing(db) {
		return fleet.ErrUnavailable
	}
	member := source.MembershipID
	if source.Validate() != nil || source.Scope != scope || legalIdentity(scope, member) != nil {
		return fleet.ErrUnauthenticated
	}
	if action != tripapp.MissionRespond && action != tripapp.MissionReceipt {
		return fleet.ErrUnavailable
	}
	policy := tripapp.MissionPolicy(action)
	for _, id := range []string{target.MissionID, target.AssigneeMembershipID, target.ResponderMembershipID, target.UnitID, target.AssetID} {
		if id != "" && legalIdentity(scope, id) != nil {
			return fleet.ErrUnavailable
		}
	}
	root := target.MissionID == ""
	if root && (target.AssigneeMembershipID != "" || target.ResponderMembershipID != "" || target.UnitID != "" || target.AssetID != "") ||
		!root && target.UnitID == "" && (target.AssigneeMembershipID != "" || target.ResponderMembershipID != "" || target.AssetID != "") {
		return fleet.ErrUnavailable
	}
	roles, ok := f.Roles.(meterRoleQueries)
	if !ok || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return fleet.ErrUnavailable
	}
	write := action == tripapp.MissionRespond
	if _, err := g.currentMeterDevice(ctx, db, scope, source, member, write); err != nil {
		return err
	}
	if !root && target.UnitID == "" {
		return nil
	}
	assignments, err := roles.CandidatesDBTX(ctx, db, scope, member)
	if err != nil {
		return fleet.ErrUnavailable
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
		facts = permission.ResourceFacts{OrgID: scope.OrgID(), ID: target.MissionID, UnitID: target.UnitID, AssetID: target.AssetID}
		if write {
			facts.AssignedTo = target.AssigneeMembershipID
		} else {
			facts.AuthoredBy = target.ResponderMembershipID
		}
	}
	_, err = permission.CoveringRules(scope, member, assignments, policy.Permission, facts, now)
	return err
}
