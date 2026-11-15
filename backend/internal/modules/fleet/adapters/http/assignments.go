package http

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const AssignmentCreatePath = AssetPrefix + "/{asset_id}/assignments/create"
const AssignmentEndPath = AssetPrefix + "/{asset_id}/assignments/{assignment_id}/end"
const AssignmentListPath = AssetPrefix + "/{asset_id}/assignments/list"
const OwnAssignmentListPath = "/v1/orgs/{org_id}/fleet/assignments/mine/list"

// assignmentCreateRequest admits exactly the BUC-EMP-124 intent: no state,
// recording instant, unit or caller authority is chosen by the request.
type assignmentCreateRequest struct {
	companyRequest
	SchemaVersion  int      `json:"schema_version"`
	OperationID    string   `json:"operation_id"`
	Kind           string   `json:"kind"`
	EmployeeIDs    []string `json:"employee_ids"`
	ValidFrom      *string  `json:"valid_from"`
	ValidTo        *string  `json:"valid_to"`
	Reason         *string  `json:"reason"`
	ReplacePrimary *bool    `json:"replace_primary"`
}

func (v assignmentCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *assignmentCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain assignmentCreateRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = assignmentCreateRequest(input)
	return nil
}

func wireInstant(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	at, err := orgtime.ParseInstant(*raw)
	if err != nil {
		return nil, d.ErrInvalid
	}
	return &at, nil
}

// command converts exact wire text: an unknown kind, a non-RFC 3339 instant
// or an empty reason is a validation refusal, never a guessed value.
func (v assignmentCreateRequest) command(asset string) (app.AssignmentCreate, error) {
	kind, err := d.ParseAssignmentKind(v.Kind)
	if err != nil {
		return app.AssignmentCreate{}, d.ErrInvalid
	}
	in := app.AssignmentCreateInput{OperationID: v.OperationID, AssetID: asset, Kind: kind, EmployeeIDs: v.EmployeeIDs}
	if in.Start, err = wireInstant(v.ValidFrom); err != nil {
		return app.AssignmentCreate{}, err
	}
	if in.End, err = wireInstant(v.ValidTo); err != nil {
		return app.AssignmentCreate{}, err
	}
	if v.Reason != nil {
		if *v.Reason == "" {
			return app.AssignmentCreate{}, d.ErrInvalid
		}
		in.Reason = *v.Reason
	}
	if v.ReplacePrimary != nil {
		in.ReplacePrimary = *v.ReplacePrimary
	}
	return app.NewAssignmentCreate(in)
}

type assignmentEndRequest struct {
	companyRequest
	SchemaVersion int     `json:"schema_version"`
	OperationID   string  `json:"operation_id"`
	Reason        *string `json:"reason"`
}

func (v assignmentEndRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *assignmentEndRequest) UnmarshalJSON(raw []byte) error {
	type plain assignmentEndRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = assignmentEndRequest(input)
	return nil
}

type assignmentListRequest struct {
	companyRequest
	orgapi.Page
}

func (v *assignmentListRequest) UnmarshalJSON(raw []byte) error {
	type plain assignmentListRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = assignmentListRequest(input)
	return nil
}

// assignmentMapped adds the B-BR-EMP-244 refusals to the asset mapping.
func assignmentMapped(err error) error {
	switch {
	case errors.Is(err, app.ErrAssignmentOverlap):
		return orgapi.AssignmentOverlap
	case errors.Is(err, d.ErrAssignmentEnded):
		return orgapi.Conflict
	}
	return assetMapped(err)
}

func canonical(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

func assignmentAck(result app.AssignmentResult, replayed bool) (map[string]any, error) {
	if !canonical(result.AssetID) || len(result.AssignmentIDs) == 0 || result.ReplacedID != "" && !canonical(result.ReplacedID) {
		return nil, orgapi.Unavailable
	}
	ids := make([]string, 0, len(result.AssignmentIDs))
	for _, id := range result.AssignmentIDs {
		if !canonical(id) {
			return nil, orgapi.Unavailable
		}
		ids = append(ids, id)
	}
	out := map[string]any{"asset_id": result.AssetID, "assignment_ids": ids, "replayed": replayed, "data_mode": "SYNTHETIC"}
	if result.ReplacedID != "" {
		out["replaced_assignment_id"] = result.ReplacedID
	}
	return out, nil
}

// assignmentItems projects stored rows exactly with the state derived at the
// page instant; valid_to, end_cause and ended_at are omitted while absent.
func assignmentItems(scope kernel.OrgScope, page app.AssignmentPage) ([]map[string]any, error) {
	if scope.Validate() != nil || page.At.IsZero() {
		return nil, orgapi.Unavailable
	}
	items := make([]map[string]any, 0, len(page.Assignments))
	for _, v := range page.Assignments {
		period, err := d.NewAssignmentPeriod(v.Period.From, v.Period.To)
		if err != nil || !canonical(v.ID) || !canonical(v.AssetID) || !canonical(v.EmployeeID) || v.RecordedAt.IsZero() || v.State != period.StateAt(page.At) || (v.EndCause == "") != (v.EndedAt == nil) {
			return nil, orgapi.Unavailable
		}
		if _, err := d.ParseAssignmentKind(string(v.Kind)); err != nil {
			return nil, orgapi.Unavailable
		}
		item := map[string]any{"assignment_id": v.ID, "asset_id": v.AssetID, "employee_id": v.EmployeeID, "kind": v.Kind, "state": v.State, "valid_from": orgtime.FormatInstant(period.From), "reason": v.Reason, "recorded_at": orgtime.FormatInstant(v.RecordedAt)}
		if period.To != nil {
			item["valid_to"] = orgtime.FormatInstant(*period.To)
		}
		if v.EndedAt != nil {
			item["end_cause"], item["ended_at"] = v.EndCause, orgtime.FormatInstant(*v.EndedAt)
		}
		items = append(items, item)
	}
	return items, nil
}

// ownAssignmentsResource names the requesting membership itself: the own
// assignment list is the member's own record (B-BR-EMP-246).
type ownAssignmentsResource struct{}

func (ownAssignmentsResource) Resolve(_ context.Context, scope kernel.OrgScope, subject access.Subject) (permissions.ResourceFacts, error) {
	member := subject.MembershipID()
	if scope.Validate() != nil || !canonical(member) {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: member, AssignedTo: member}, nil
}

func assignRule(status int) orgapi.Rule {
	return orgapi.Rule{Permission: string(app.AssignPolicy().Permission), Write: true, Fresh: true, CompanyOnly: true, Success: status}
}

func assignmentPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "assignment_id")
	if !canonical(raw) {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func listResponse(p orgapi.Principal, gateway orgapi.Gateway, path, binding string, page app.AssignmentPage, next string) (any, error) {
	items, err := assignmentItems(p.Scope, page)
	if err != nil {
		return nil, err
	}
	cursor := ""
	if next != "" {
		if cursor, err = gateway.SealCursor(p, path, binding, next); err != nil {
			return nil, orgapi.Unavailable
		}
	}
	return map[string]any{"items": items, "next_cursor": cursor, "evaluated_at": orgtime.FormatInstant(page.At)}, nil
}

// RegisterAssetAssignments exposes the EMP-FLEET-006 commands (fresh
// fleet.asset.assign covering the asset) and reads (current fleet.asset.read
// covering the asset, or the requesting member's own list).
func RegisterAssetAssignments(router chi.Router, service app.AssignmentService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || service.NewID == nil || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	resource := assetResource{}
	if err := guarded.Register(owned, AssignmentCreatePath, gateway, assignRule(201), authorization, app.AssignPolicy(), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assignmentCreateRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := input.command(id)
		if err != nil {
			return nil, assignmentMapped(err)
		}
		result, replayed, err := service.Create(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, assignmentMapped(err)
		}
		return assignmentAck(result, replayed)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, AssignmentEndPath, gateway, assignRule(200), authorization, app.AssignPolicy(), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assignmentEndRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		assignment, err := assignmentPathID(ctx)
		if err != nil {
			return nil, err
		}
		in := app.AssignmentEndInput{OperationID: input.OperationID, AssetID: asset, AssignmentID: assignment}
		if input.Reason != nil {
			if *input.Reason == "" {
				return nil, orgapi.ValidationFailed
			}
			in.Reason = *input.Reason
		}
		command, err := app.NewAssignmentEnd(in)
		if err != nil {
			return nil, assignmentMapped(err)
		}
		result, replayed, err := service.End(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, assignmentMapped(err)
		}
		return assignmentAck(result, replayed)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, AssignmentListPath, gateway, assetRule(false, false, 200), authorization, app.Policy(false), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assignmentListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("fleet.asset.assignments.v1:%s:%d", id, page.Limit)
		after, err := gateway.OpenCursor(p, AssignmentListPath, binding, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.AssignmentFilter{Limit: page.Limit}
		if after != "" {
			from, key, ok := strings.Cut(after, "|")
			at, err := orgtime.ParseInstant(from)
			if !ok || err != nil || orgtime.FormatInstant(at) != from || !canonical(key) {
				return nil, orgapi.Malformed
			}
			filter.AfterFrom, filter.AfterID = &at, key
		}
		v, err := service.List(ctx, p.Scope, actor, decision, id, filter)
		if err != nil {
			return nil, assignmentMapped(err)
		}
		next := ""
		if v.NextFrom != nil {
			next = orgtime.FormatInstant(*v.NextFrom) + "|" + v.NextID
		}
		return listResponse(p, gateway, AssignmentListPath, binding, v, next)
	}); err != nil {
		return err
	}
	// The own list is the membership itself: no structure or assignee lookup
	// applies to it (a structural grant never covers a membership).
	own := authorization
	own.Structure, own.Assignee = nil, nil
	return guarded.Register(owned, OwnAssignmentListPath, gateway, assetRule(false, false, 200), own, app.Policy(false), ownAssignmentsResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assignmentListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		binding := fmt.Sprintf("fleet.assignments.mine.v1:%d", page.Limit)
		after, err := gateway.OpenCursor(p, OwnAssignmentListPath, binding, page.Cursor)
		if err != nil || after != "" && !canonical(after) {
			return nil, orgapi.Malformed
		}
		v, err := service.Mine(ctx, p.Scope, actor, decision, app.OwnAssignmentFilter{Limit: page.Limit, AfterID: after})
		if err != nil {
			return nil, assignmentMapped(err)
		}
		return listResponse(p, gateway, OwnAssignmentListPath, binding, v, v.NextID)
	})
}
