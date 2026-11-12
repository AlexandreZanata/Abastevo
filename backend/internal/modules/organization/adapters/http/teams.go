package http

import (
	"context"
	"errors"
	"fmt"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const TeamPrefix = "/v1/orgs/{org_id}/teams"
const TeamCreatePath = TeamPrefix + "/create"
const TeamListPath = TeamPrefix + "/list"
const TeamReadPath = TeamPrefix + "/{team_id}/read"
const TeamChangePath = TeamPrefix + "/{team_id}/change"
const TeamArchivePath = TeamPrefix + "/{team_id}/archive"
const TeamRestorePath = TeamPrefix + "/{team_id}/restore"

type teamFields struct {
	privateRequest
	Name   string `json:"name"`
	UnitID string `json:"unit_id"`
}

func (v teamFields) domain() d.TeamProfile {
	return d.TeamProfile{Name: v.Name, UnitID: v.UnitID}
}

type teamCreateRequest struct {
	privateRequest
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	teamFields
}

func (v teamCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type teamChangeRequest struct {
	privateRequest
	unitRevisionCommand
	teamFields
}
type teamTransitionRequest struct{ unitRevisionCommand }
type teamReadRequest struct{ companyRequest }
type teamListRequest struct {
	companyRequest
	orgapi.Page
	State d.TeamState `json:"state,omitempty"`
}

func (v *teamCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain teamCreateRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = teamCreateRequest(input)
	return nil
}
func (v *teamChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain teamChangeRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = teamChangeRequest(input)
	return nil
}
func (v *teamTransitionRequest) UnmarshalJSON(raw []byte) error {
	type plain teamTransitionRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = teamTransitionRequest(input)
	return nil
}
func (v *teamReadRequest) UnmarshalJSON(raw []byte) error {
	type plain teamReadRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = teamReadRequest(input)
	return nil
}
func (v *teamListRequest) UnmarshalJSON(raw []byte) error {
	type plain teamListRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = teamListRequest(input)
	return nil
}

type teamResource struct{ lookup app.TeamLookup }

func (r teamResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if r.lookup == nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, err := legalPathID(ctx, "team_id")
	if err != nil {
		return permissions.ResourceFacts{}, err
	}
	c, err := r.lookup.FindTeam(ctx, scope, id)
	if errors.Is(err, app.ErrNotFound) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	if err != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	if _, err := c.Snapshot(scope); err != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}
func teamMapped(err error) error {
	if errors.Is(err, d.ErrConditions) {
		return orgapi.Conflict
	}
	if errors.Is(err, app.ErrTeamNameDuplicate) {
		return orgapi.UnitDuplicate("name")
	}
	return unitMapped(err)
}
func teamAck(v app.TeamResult, replay bool) map[string]any {
	result := map[string]any{"team_id": v.TeamID, "state": v.State, "revision": v.Revision, "data_mode": "SYNTHETIC", "replayed": replay}
	if v.LinkID != "" {
		result["link_id"] = v.LinkID
	}
	return result
}
func teamView(scope kernel.OrgScope, c d.Team) (map[string]any, error) {
	v, err := c.Snapshot(scope)
	if err != nil {
		return nil, orgapi.Unavailable
	}
	display := v.Name
	if v.State == d.TeamArchived {
		display += " (arquivada)"
	}
	return map[string]any{"id": v.ID, "name": v.Name, "display_name": display, "unit_id": v.UnitID, "state": v.State, "revision": v.Revision, "data_mode": "SYNTHETIC"}, nil
}
func RegisterTeams(router chi.Router, service app.TeamService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots app.Repository, lookup app.TeamLookup) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || service.NewID == nil || roots == nil || lookup == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := companyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	resource := teamResource{lookup}
	writePolicy, readPolicy := app.StructurePolicy(true), app.StructurePolicy(false)
	if err := guarded.Register(owned, TeamCreatePath, gateway, unitRule(true, false, 201), authorization, writePolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input teamCreateRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		command, err := app.NewTeamWrite(app.TeamWriteInput{Kind: app.CreateTeam, OperationID: input.OperationID, Profile: input.teamFields.domain()})
		if err != nil {
			return nil, teamMapped(err)
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, teamMapped(err)
		}
		return teamAck(result, replay), nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, TeamChangePath, gateway, unitRule(true, true, 200), authorization, writePolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input teamChangeRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "team_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := app.NewTeamWrite(app.TeamWriteInput{Kind: app.ChangeTeam, OperationID: input.OperationID, TeamID: id, ExpectedRevision: *input.Revision, Profile: input.teamFields.domain()})
		if err != nil {
			return nil, teamMapped(err)
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, teamMapped(err)
		}
		return teamAck(result, replay), nil
	}); err != nil {
		return err
	}
	for _, transition := range []struct {
		path string
		kind app.TeamWriteKind
	}{{TeamArchivePath, app.ArchiveTeam}, {TeamRestorePath, app.RestoreTeam}} {
		if err := guarded.Register(owned, transition.path, gateway, unitRule(true, true, 200), authorization, writePolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input teamTransitionRequest) (any, error) {
			actor, err := companyActor(ctx)
			if err != nil {
				return nil, err
			}
			id, err := legalPathID(ctx, "team_id")
			if err != nil {
				return nil, orgapi.NotFound
			}
			command, err := app.NewTeamWrite(app.TeamWriteInput{Kind: transition.kind, OperationID: input.OperationID, TeamID: id, ExpectedRevision: *input.Revision})
			if err != nil {
				return nil, teamMapped(err)
			}
			result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
			if err != nil {
				return nil, teamMapped(err)
			}
			return teamAck(result, replay), nil
		}); err != nil {
			return err
		}
	}
	if err := guarded.Register(owned, TeamReadPath, gateway, unitRule(false, false, 200), authorization, readPolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ teamReadRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "team_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		c, err := service.Read(ctx, p.Scope, actor, decision, id)
		if err != nil {
			return nil, teamMapped(err)
		}
		return teamView(p.Scope, c)
	}); err != nil {
		return err
	}
	if err := registerTeamMembers(owned, service, gateway, authorization, resource); err != nil {
		return err
	}
	return guarded.Register(owned, TeamListPath, gateway, unitRule(false, false, 200), authorization, readPolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input teamListRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.TeamFilter{Limit: page.Limit, State: input.State}
		if filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("teams.v1:%s:%d", input.State, page.Limit)
		filter.AfterID, err = gateway.OpenCursor(p, TeamListPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		values, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, teamMapped(err)
		}
		items := make([]map[string]any, 0, len(values.Teams))
		for _, c := range values.Teams {
			value, err := teamView(p.Scope, c)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		next := ""
		if values.NextID != "" {
			next, err = gateway.SealCursor(p, TeamListPath, filters, values.NextID)
			if err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}

const TeamMemberAddPath = TeamPrefix + "/{team_id}/members/{employee_id}/add"
const TeamMemberRemovePath = TeamPrefix + "/{team_id}/members/{employee_id}/remove"
const TeamMemberListPath = TeamPrefix + "/{team_id}/members/list"

type teamMemberRequest struct{ unitRevisionCommand }
type teamMemberListRequest struct {
	companyRequest
	orgapi.Page
	State string `json:"state,omitempty"`
}

func (v *teamMemberRequest) UnmarshalJSON(raw []byte) error {
	type plain teamMemberRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = teamMemberRequest(input)
	return nil
}
func (v *teamMemberListRequest) UnmarshalJSON(raw []byte) error {
	type plain teamMemberListRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = teamMemberListRequest(input)
	return nil
}
func registerTeamMembers(owned chi.Router, service app.TeamService, gateway orgapi.Gateway, authorization access.Authorization, resource teamResource) error {
	for _, v := range []struct {
		path string
		kind app.TeamWriteKind
	}{{TeamMemberAddPath, app.AddTeamEmployee}, {TeamMemberRemovePath, app.RemoveTeamEmployee}} {
		if err := guarded.Register(owned, v.path, gateway, unitRule(true, true, 200), authorization, app.StructurePolicy(true), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input teamMemberRequest) (any, error) {
			actor, err := companyActor(ctx)
			if err != nil {
				return nil, err
			}
			id, err := legalPathID(ctx, "team_id")
			if err != nil {
				return nil, orgapi.NotFound
			}
			employeeID, err := legalPathID(ctx, "employee_id")
			if err != nil {
				return nil, orgapi.NotFound
			}
			command, err := app.NewTeamWrite(app.TeamWriteInput{Kind: v.kind, OperationID: input.OperationID, TeamID: id, EmployeeID: employeeID, ExpectedRevision: *input.Revision})
			if err != nil {
				return nil, teamMapped(err)
			}
			result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
			if err != nil {
				return nil, teamMapped(err)
			}
			return teamAck(result, replay), nil
		}); err != nil {
			return err
		}
	}
	return guarded.Register(owned, TeamMemberListPath, gateway, unitRule(false, false, 200), authorization, app.StructurePolicy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input teamMemberListRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "team_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.TeamLinkFilter{Limit: page.Limit, State: input.State}
		if filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("team_links.v1:%s:%s:%d", id, input.State, page.Limit)
		filter.AfterID, err = gateway.OpenCursor(p, TeamMemberListPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		values, err := service.Links(ctx, p.Scope, actor, decision, id, filter)
		if err != nil {
			return nil, teamMapped(err)
		}
		items := make([]map[string]any, 0, len(values.Links))
		for _, link := range values.Links {
			v, err := link.Snapshot(p.Scope)
			if err != nil {
				return nil, orgapi.Unavailable
			}
			state, ended := "ACTIVE", ""
			if !v.EndedAt.IsZero() {
				state = "ENDED"
				ended = v.EndedAt.Format(time.RFC3339Nano)
			}
			items = append(items, map[string]any{"id": v.ID, "team_id": v.TeamID, "employee_id": v.EmployeeID, "state": state, "started_at": v.StartedAt.Format(time.RFC3339Nano), "ended_at": ended, "end_reason": v.EndReason})
		}
		next := ""
		if values.NextID != "" {
			next, err = gateway.SealCursor(p, TeamMemberListPath, filters, values.NextID)
			if err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}
