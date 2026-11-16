package http

import (
	"context"
	"errors"
	"fmt"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// EMP-TRIP-008 mission routes (EDR-231) and the EMP-TRIP-009 plan (EDR-249).
const (
	MissionPrefix     = "/v1/orgs/{org_id}/missions"
	MissionCreatePath = MissionPrefix + "/create"
	MissionListPath   = MissionPrefix + "/list"
	MissionMinePath   = MissionPrefix + "/mine/list"
	MissionReadPath   = MissionPrefix + "/{mission_id}/read"
	MissionPlanPath   = MissionPrefix + "/{mission_id}/plan"
	MissionAssignPath = MissionPrefix + "/{mission_id}/assign"
	MissionCancelPath = MissionPrefix + "/{mission_id}/cancel"
	// MissionVehiclePath replaces the mission's vehicle (EMP-TRIP-013, EDR-251).
	MissionVehiclePath = MissionPrefix + "/{mission_id}/vehicle/change"
	// MissionAssigneesPath lists the employees eligible for the mission
	// (EMP-TRIP-022, EDR-304).
	MissionAssigneesPath = MissionPrefix + "/{mission_id}/assignees/list"
	// MissionReservePath requests the reservation of the mission's vehicle
	// for its planned window (EMP-TRIP-023, EDR-266).
	MissionReservePath = MissionPrefix + "/{mission_id}/reservations/request"
)

// MissionRecords accepts only the mission policies on their own targets: the
// organization or a mission for trip.mission.manage, the requesting
// membership for the own list.
type MissionRecords struct{}

func (MissionRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	switch policy {
	case app.MissionPolicy(app.MissionRead), app.MissionPolicy(app.MissionManage):
		return nil
	case app.MissionPolicy(app.MissionOwnRead):
		if facts.ID == subject.MembershipID() && facts.AssignedTo == subject.MembershipID() {
			return nil
		}
	}
	return access.ErrUnavailable
}

// missionRoot resolves the organization itself: creation and the list are
// organization-level actions that only ORG grants cover (EDR-161).
type missionRoot struct{}

func (missionRoot) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

// missionResource resolves the path mission; its structure comes from the
// server-side MissionStructure source.
type missionResource struct{}

func (missionResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, err := missionPathID(ctx)
	if err != nil {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}

// ownMissions names the requesting membership itself (the FLEET-006 own-list
// pattern): the member's own missions are its own record.
type ownMissions struct{}

func (ownMissions) Resolve(_ context.Context, scope kernel.OrgScope, subject access.Subject) (permissions.ResourceFacts, error) {
	member := subject.MembershipID()
	id, err := kernel.ParseOrgID(member)
	if scope.Validate() != nil || err != nil || id.String() != member {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: member, AssignedTo: member}, nil
}

func missionPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "mission_id")
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}

type missionCommandFields struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
}

func (v missionCommandFields) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type missionCreateRequest struct {
	missionCommandFields
	UnitID   string  `json:"unit_id"`
	AssetID  *string `json:"asset_id"`
	Title    string  `json:"title"`
	StartsAt string  `json:"starts_at"`
	EndsAt   string  `json:"ends_at"`
}
type missionAssignRequest struct {
	missionCommandFields
	AssigneeEmployeeID string `json:"assignee_employee_id"`
}

// missionDestination and missionLoad are closed nested objects: the strict
// decoder refuses any other field, such as a passenger identity.
type missionDestination struct {
	Kind   string  `json:"kind"`
	Text   *string `json:"text"`
	UnitID *string `json:"unit_id"`
}
type missionLoad struct {
	Kind        string  `json:"kind"`
	Description *string `json:"description"`
	Count       *int    `json:"count"`
}
type missionPlanRequest struct {
	missionCommandFields
	Destination  *missionDestination `json:"destination"`
	Reason       string              `json:"reason"`
	CostCentreID *string             `json:"cost_centre_id"`
	Load         *missionLoad        `json:"load"`
}

// fields maps the wire plan onto the command; a kind/field mismatch is a
// validation failure (B-BR-EMP-382).
func (v missionPlanRequest) fields() (app.MissionPlanFields, error) {
	if v.Destination == nil || v.Load == nil {
		return app.MissionPlanFields{}, orgapi.Malformed
	}
	out := app.MissionPlanFields{Reason: v.Reason, Load: v.Load.Kind, CargoDescription: v.Load.Description, Passengers: v.Load.Count}
	switch d := v.Destination; {
	case d.Kind == "TEXT" && d.Text != nil && d.UnitID == nil:
		out.DestinationText = d.Text
	case d.Kind == "UNIT" && d.UnitID != nil && d.Text == nil && *d.UnitID != "":
		out.DestinationUnitID = *d.UnitID
	default:
		return app.MissionPlanFields{}, orgapi.ValidationFailed
	}
	if v.CostCentreID != nil {
		if *v.CostCentreID == "" {
			return app.MissionPlanFields{}, orgapi.ValidationFailed
		}
		out.CostCentreID = *v.CostCentreID
	}
	return out, nil
}

type missionVehicleRequest struct {
	missionCommandFields
	AssetID string `json:"asset_id"`
	Reason  string `json:"reason"`
}
type missionCancelRequest struct {
	missionCommandFields
	Reason string `json:"reason"`
}
type missionReserveRequest struct {
	missionCommandFields
	Purpose string `json:"purpose"`
}

func (v *missionReserveRequest) UnmarshalJSON(raw []byte) error {
	type plain missionReserveRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = missionReserveRequest(in)
	return nil
}

type missionListRequest struct {
	companyRequest
	orgapi.Page
}
type missionReadRequest struct{ companyRequest }

func (v *missionCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain missionCreateRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = missionCreateRequest(in)
	return nil
}
func (v *missionAssignRequest) UnmarshalJSON(raw []byte) error {
	type plain missionAssignRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = missionAssignRequest(in)
	return nil
}
func (v *missionPlanRequest) UnmarshalJSON(raw []byte) error {
	type plain missionPlanRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = missionPlanRequest(in)
	return nil
}
func (v *missionVehicleRequest) UnmarshalJSON(raw []byte) error {
	type plain missionVehicleRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = missionVehicleRequest(in)
	return nil
}
func (v *missionCancelRequest) UnmarshalJSON(raw []byte) error {
	type plain missionCancelRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = missionCancelRequest(in)
	return nil
}
func (v *missionListRequest) UnmarshalJSON(raw []byte) error {
	type plain missionListRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = missionListRequest(in)
	return nil
}
func (v *missionReadRequest) UnmarshalJSON(raw []byte) error {
	type plain missionReadRequest
	var in plain
	if err := decodeStrict(raw, &in); err != nil {
		return err
	}
	*v = missionReadRequest(in)
	return nil
}

// missionMapped maps the domain, store and authority refusals (EDR-231).
func missionMapped(err error) error {
	var denied *permissions.PermissionDenial
	if errors.As(err, &denied) {
		return orgapi.MissingPermission(string(denied.Permission()))
	}
	// EMP-TRIP-023: a reservation request reports only the conflicting window.
	var window fleetapp.ReservationConflict
	if errors.As(err, &window) {
		return orgapi.ReservationWindowConflict(window.From, window.To)
	}
	if errors.Is(err, fleetapp.ErrReservationConflict) {
		return orgapi.ReservationConflict
	}
	if errors.Is(err, fleet.ErrInvalid) {
		return orgapi.ValidationFailed
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{trip.ErrConflict, orgapi.Conflict}, {trip.ErrInvalidTransition, orgapi.Conflict}, {trip.ErrUnsupportedCommand, orgapi.Conflict},
		{app.ErrMissionAssetBusy, orgapi.Conflict}, {app.ErrMissionTripInProgress, orgapi.Conflict},
		{trip.ErrInvalid, orgapi.ValidationFailed}, {trip.ErrScope, orgapi.NotFound}, {app.ErrMissionIneligible, orgapi.ValidationFailed},
		{app.ErrMissionDisabled, orgapi.NotFound}, {app.ErrNotFound, orgapi.NotFound}, {app.ErrMissionUnauthenticated, orgapi.Unauthenticated},
		{app.ErrOperationReused, orgapi.OperationReused},
		{access.ErrNoMembership, orgapi.NotFound}, {access.ErrNotFound, orgapi.NotFound}, {access.ErrUnauthenticated, orgapi.Unauthenticated},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired}, {access.ErrMembershipSuspended, orgapi.MembershipSuspended}, {access.ErrOrgSuspended, orgapi.OrgSuspended},
		{access.ErrOrgArchived, orgapi.OrgArchived}, {access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete}, {access.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

func missionActor(ctx context.Context) (app.MissionActor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.MissionActor{}, err
	}
	return app.MissionActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, nil
}

// MissionRule returns the exact orgapi rule of an action; success is 201 only
// for creation.
func MissionRule(action app.MissionAction, revision bool, success int) orgapi.Rule {
	policy := app.MissionPolicy(action)
	return orgapi.Rule{Permission: string(policy.Permission), Write: policy.Effect.Write(), Fresh: policy.Fresh, RequireRevision: revision, CompanyOnly: true, Success: success}
}

func missionAck(v app.MissionAck, replayed bool) map[string]any {
	out := map[string]any{"mission_id": v.MissionID, "revision": v.Revision, "state": v.State, "replayed": replayed, "data_mode": "SYNTHETIC"}
	if v.ReservationID != "" {
		out["reservation_id"] = v.ReservationID
	}
	if v.AssigneeEmployeeID != "" {
		out["assignee_employee_id"] = v.AssigneeEmployeeID
	}
	if v.Warning != nil {
		out["asset_warning"] = map[string]any{"code": v.Warning.Code, "availability": v.Warning.Availability}
	}
	return out
}

// projectMission omits absent optional fields; it never emits null.
func projectMission(v app.MissionView) map[string]any {
	out := map[string]any{"mission_id": v.MissionID, "unit_id": v.UnitID, "title": v.Title, "starts_at": orgtime.FormatInstant(v.StartsAt), "ends_at": orgtime.FormatInstant(v.EndsAt),
		"state": v.State, "revision": v.Revision, "created_at": orgtime.FormatInstant(v.CreatedAt), "updated_at": orgtime.FormatInstant(v.UpdatedAt)}
	if v.AssetID != "" {
		out["asset_id"] = v.AssetID
	}
	if v.AssigneeEmployeeID != "" {
		out["assignee_employee_id"] = v.AssigneeEmployeeID
	}
	if v.Answer != "" {
		answer := map[string]any{"response": v.Answer}
		if v.AnswerReason != nil {
			answer["reason"] = *v.AnswerReason
		}
		out["answer"] = answer
	}
	if v.Plan != nil {
		out["plan"] = projectPlan(*v.Plan)
	}
	return out
}

// projectPlan is the current planning revision; the planner stays in the
// stored transition, planning record and audit event (EDR-249).
func projectPlan(p app.MissionPlanView) map[string]any {
	f := p.Fields
	destination := map[string]any{"kind": "TEXT"}
	if f.DestinationText != nil {
		destination["text"] = *f.DestinationText
	} else {
		destination["kind"], destination["unit_id"] = "UNIT", f.DestinationUnitID
	}
	load := map[string]any{"kind": f.Load}
	if f.CargoDescription != nil {
		load["description"] = *f.CargoDescription
	}
	if f.Passengers != nil {
		load["count"] = *f.Passengers
	}
	out := map[string]any{"revision": p.Revision, "destination": destination, "reason": f.Reason, "load": load, "planned_at": orgtime.FormatInstant(p.PlannedAt)}
	if f.CostCentreID != "" {
		out["cost_centre_id"] = f.CostCentreID
	}
	return out
}

// RegisterMissions mounts the eight trip.mission.manage routes and the own
// list. The organization and mission routes share the MissionStructure
// authorization; the own list carries no structure or assignee source.
func RegisterMissions(router chi.Router, service app.MissionService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || service.NewID == nil || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) ||
		authorization.Now == nil || missing(authorization.Structure) {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	command := func(ctx context.Context, p orgapi.Principal, decision access.Decision, in app.MissionCommandInput) (any, error) {
		actor, err := missionActor(ctx)
		if err != nil {
			return nil, err
		}
		c, err := app.NewMissionCommand(in)
		if err != nil {
			return nil, missionMapped(err)
		}
		ack, replayed, err := service.Command(ctx, p.Scope, actor, decision, c)
		if err != nil {
			return nil, missionMapped(err)
		}
		return missionAck(ack, replayed), nil
	}
	if err := guarded.Register(owned, MissionCreatePath, gateway, MissionRule(app.MissionManage, false, 201), authorization, app.MissionPolicy(app.MissionManage), missionRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input missionCreateRequest) (any, error) {
		if input.Revision != nil {
			return nil, orgapi.Malformed
		}
		in := app.MissionCommandInput{Kind: app.MissionCreate, OperationID: input.OperationID, UnitID: input.UnitID, Title: input.Title}
		if input.AssetID != nil {
			if *input.AssetID == "" {
				return nil, orgapi.ValidationFailed
			}
			in.AssetID = *input.AssetID
		}
		starts, err := orgtime.ParseInstant(input.StartsAt)
		if err != nil {
			return nil, orgapi.ValidationFailed
		}
		ends, err := orgtime.ParseInstant(input.EndsAt)
		if err != nil {
			return nil, orgapi.ValidationFailed
		}
		in.StartsAt, in.EndsAt = starts, ends
		return command(ctx, p, decision, in)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, MissionAssignPath, gateway, MissionRule(app.MissionManage, true, 200), authorization, app.MissionPolicy(app.MissionManage), missionResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input missionAssignRequest) (any, error) {
		mission, err := missionPathID(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		return command(ctx, p, decision, app.MissionCommandInput{Kind: app.MissionAssignCommand, OperationID: input.OperationID, MissionID: mission, ExpectedRevision: *input.Revision,
			AssigneeEmployeeID: input.AssigneeEmployeeID})
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, MissionPlanPath, gateway, MissionRule(app.MissionManage, true, 200), authorization, app.MissionPolicy(app.MissionManage), missionResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input missionPlanRequest) (any, error) {
		mission, err := missionPathID(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		plan, err := input.fields()
		if err != nil {
			return nil, err
		}
		return command(ctx, p, decision, app.MissionCommandInput{Kind: app.MissionPlanCommand, OperationID: input.OperationID, MissionID: mission, ExpectedRevision: *input.Revision, Plan: &plan})
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, MissionVehiclePath, gateway, MissionRule(app.MissionManage, true, 200), authorization, app.MissionPolicy(app.MissionManage), missionResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input missionVehicleRequest) (any, error) {
		mission, err := missionPathID(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		return command(ctx, p, decision, app.MissionCommandInput{Kind: app.MissionVehicleCommand, OperationID: input.OperationID, MissionID: mission, ExpectedRevision: *input.Revision,
			AssetID: input.AssetID, Reason: input.Reason})
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, MissionReservePath, gateway, MissionRule(app.MissionManage, true, 201), authorization, app.MissionPolicy(app.MissionManage), missionResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input missionReserveRequest) (any, error) {
		mission, err := missionPathID(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		return command(ctx, p, decision, app.MissionCommandInput{Kind: app.MissionReserveCommand, OperationID: input.OperationID, MissionID: mission, ExpectedRevision: *input.Revision, Reason: input.Purpose})
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, MissionCancelPath, gateway, MissionRule(app.MissionManage, true, 200), authorization, app.MissionPolicy(app.MissionManage), missionResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input missionCancelRequest) (any, error) {
		mission, err := missionPathID(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		return command(ctx, p, decision, app.MissionCommandInput{Kind: app.MissionCancelCommand, OperationID: input.OperationID, MissionID: mission, ExpectedRevision: *input.Revision, Reason: input.Reason})
	}); err != nil {
		return err
	}
	list := func(path string, own bool) func(context.Context, orgapi.Principal, access.Decision, missionListRequest) (any, error) {
		return func(ctx context.Context, p orgapi.Principal, decision access.Decision, input missionListRequest) (any, error) {
			actor, err := missionActor(ctx)
			if err != nil {
				return nil, err
			}
			page, err := input.Page.Normalize()
			if err != nil {
				return nil, orgapi.Malformed
			}
			binding := fmt.Sprintf("trip.missions.v1:%t:%d", own, page.Limit)
			filter := app.MissionFilter{Limit: page.Limit}
			if filter.AfterID, err = gateway.OpenCursor(p, path, binding, page.Cursor); err != nil || filter.Validate() != nil {
				return nil, orgapi.Malformed
			}
			var result app.MissionPage
			if own {
				result, err = service.Mine(ctx, p.Scope, actor, decision, filter)
			} else {
				result, err = service.List(ctx, p.Scope, actor, decision, filter)
			}
			if err != nil {
				return nil, missionMapped(err)
			}
			items := make([]map[string]any, 0, len(result.Missions))
			for _, m := range result.Missions {
				items = append(items, projectMission(m))
			}
			next := ""
			if result.NextID != "" {
				if next, err = gateway.SealCursor(p, path, binding, result.NextID); err != nil {
					return nil, orgapi.Unavailable
				}
			}
			return map[string]any{"items": items, "next_cursor": next, "data_mode": "SYNTHETIC"}, nil
		}
	}
	if err := guarded.Register(owned, MissionListPath, gateway, MissionRule(app.MissionRead, false, 200), authorization, app.MissionPolicy(app.MissionRead), missionRoot{}, list(MissionListPath, false)); err != nil {
		return err
	}
	if err := guarded.Register(owned, MissionAssigneesPath, gateway, MissionRule(app.MissionRead, false, 200), authorization, app.MissionPolicy(app.MissionRead), missionResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input missionListRequest) (any, error) {
		actor, err := missionActor(ctx)
		if err != nil {
			return nil, err
		}
		mission, err := missionPathID(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		// The cursor is bound to the route, this mission and the limit.
		binding := fmt.Sprintf("trip.mission.assignees.v1:%s:%d", mission, page.Limit)
		filter := app.MissionAssigneeFilter{Limit: page.Limit}
		if filter.AfterID, err = gateway.OpenCursor(p, MissionAssigneesPath, binding, page.Cursor); err != nil || filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		result, err := service.Assignees(ctx, p.Scope, actor, decision, mission, filter)
		if err != nil {
			return nil, missionMapped(err)
		}
		items := make([]map[string]any, 0, len(result.Assignees))
		for _, v := range result.Assignees {
			items = append(items, map[string]any{"employee_id": v.EmployeeID, "display_name": v.DisplayName, "registration_number": v.RegistrationNumber})
		}
		next := ""
		if result.NextID != "" {
			if next, err = gateway.SealCursor(p, MissionAssigneesPath, binding, result.NextID); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, MissionReadPath, gateway, MissionRule(app.MissionRead, false, 200), authorization, app.MissionPolicy(app.MissionRead), missionResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ missionReadRequest) (any, error) {
		actor, err := missionActor(ctx)
		if err != nil {
			return nil, err
		}
		mission, err := missionPathID(ctx)
		if err != nil {
			return nil, err
		}
		v, err := service.Read(ctx, p.Scope, actor, decision, mission)
		if err != nil {
			return nil, missionMapped(err)
		}
		out := projectMission(v)
		out["data_mode"] = "SYNTHETIC"
		return out, nil
	}); err != nil {
		return err
	}
	// The own list is the membership itself: no structure or assignee lookup.
	own := authorization
	own.Structure, own.Assignee = nil, nil
	return guarded.Register(owned, MissionMinePath, gateway, MissionRule(app.MissionOwnRead, false, 200), own, app.MissionPolicy(app.MissionOwnRead), ownMissions{}, list(MissionMinePath, true))
}
