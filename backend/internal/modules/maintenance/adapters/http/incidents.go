package http

// EMP-CHECK-003 company routes (EDR-241): the handling step, one report, the
// organization queue and the reporter's own list.

import (
	"context"
	"errors"
	"fmt"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const IncidentPrefix = "/v1/orgs/{org_id}/incidents/reports"
const IncidentQueuePath = IncidentPrefix + "/list"
const IncidentMinePath = IncidentPrefix + "/mine"
const IncidentReadPath = IncidentPrefix + "/{report_id}/read"
const IncidentDecidePath = IncidentPrefix + "/{report_id}/decide"

// IncidentRecords accepts only this module's fixed incident policies and
// targets: the queue on the organization, a report on itself and the own
// list on the requesting membership.
type IncidentRecords struct{}

func (IncidentRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	root := facts.ID == scope.OrgID().String()
	own := facts.ID == subject.MembershipID() && facts.AssignedTo == subject.MembershipID()
	switch policy {
	case app.IncidentPolicy(app.IncidentQueue):
		if !own {
			return nil
		}
	case app.IncidentPolicy(app.IncidentDecide):
		if !root && !own {
			return nil
		}
	case app.IncidentPolicy(app.IncidentOwn):
		if own {
			return nil
		}
	}
	return access.ErrUnavailable
}

// reportResource resolves the path report as the protected record.
type reportResource struct{}

func (reportResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id := chi.URLParamFromCtx(ctx, "report_id")
	if !incident.CanonicalID(id) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}

// ownReports names the requesting membership itself: the own list is the
// member's own record (as the FLEET-006 own assignment list).
type ownReports struct{}

func (ownReports) Resolve(_ context.Context, scope kernel.OrgScope, subject access.Subject) (permissions.ResourceFacts, error) {
	member := subject.MembershipID()
	if scope.Validate() != nil || !incident.CanonicalID(member) {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: member, AssignedTo: member}, nil
}

type incidentListRequest struct {
	companyRequest
	orgapi.Page
}
type incidentReadRequest struct{ companyRequest }
type incidentDecideRequest struct {
	commandFields
	Action      string  `json:"action"`
	Note        *string `json:"note"`
	DuplicateOf *string `json:"duplicate_of"`
}

func (v *incidentListRequest) UnmarshalJSON(raw []byte) error {
	type plain incidentListRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = incidentListRequest(in)
	return nil
}
func (v *incidentReadRequest) UnmarshalJSON(raw []byte) error {
	type plain incidentReadRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = incidentReadRequest(in)
	return nil
}
func (v *incidentDecideRequest) UnmarshalJSON(raw []byte) error {
	type plain incidentDecideRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = incidentDecideRequest(in)
	return nil
}

func incidentMapped(err error) error {
	switch {
	case errors.Is(err, app.ErrSelfApproval):
		return orgapi.SelfApproval
	case errors.Is(err, incident.ErrInvalid):
		return orgapi.ValidationFailed
	}
	return mapped(err)
}

func incidentActor(ctx context.Context) (app.IncidentActor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.IncidentActor{}, err
	}
	return app.IncidentActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, nil
}

// IncidentRule returns the exact orgapi rule of an action.
func IncidentRule(action app.IncidentAction) orgapi.Rule {
	policy := app.IncidentPolicy(action)
	return orgapi.Rule{Permission: string(policy.Permission), Write: policy.Effect.Write(), Fresh: policy.Fresh, RequireRevision: action == app.IncidentDecide, CompanyOnly: true, Success: 200}
}

func projectReportSummary(v app.ReportSummary) (map[string]any, error) {
	occurred, err := stamp(v.OccurredAt)
	if err != nil {
		return nil, err
	}
	received, err := stamp(v.ReceivedAt)
	if err != nil {
		return nil, err
	}
	updated, err := stamp(v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"report_id": v.ReportID, "report_type": v.Type, "asset_id": v.AssetID, "reporter_membership_id": v.ReporterMembershipID, "blocks_use": v.BlocksUse,
		"state": v.State, "revision": v.Revision, "occurred_at": occurred, "received_at": received, "updated_at": updated}
	if v.Kind != "" {
		out["incident_kind"] = v.Kind
	}
	if v.TripID != "" {
		out["trip_id"] = v.TripID
	}
	return out, nil
}

func withText(out map[string]any, description string, location *string) map[string]any {
	out["description"] = description
	if location != nil {
		out["location"] = *location
	}
	return out
}

func projectReport(v app.ReportView) (map[string]any, error) {
	out, err := projectReportSummary(v.Summary)
	if err != nil {
		return nil, err
	}
	history := make([]map[string]any, 0, len(v.History))
	for _, h := range v.History {
		at, err := stamp(h.RecordedAt)
		if err != nil {
			return nil, err
		}
		item := map[string]any{"revision": h.Revision, "action": h.Action, "from_state": h.From, "to_state": h.To, "actor_membership_id": h.ActorMembershipID, "recorded_at": at}
		if h.Note != nil {
			item["note"] = *h.Note
		}
		if h.DuplicateOf != "" {
			item["duplicate_of"] = h.DuplicateOf
		}
		history = append(history, item)
	}
	attachments := make([]map[string]any, 0, len(v.Attachments))
	for _, a := range v.Attachments {
		at, err := stamp(a.LinkedAt)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, map[string]any{"document_id": a.DocumentID, "storage_state": a.Storage, "linked_at": at})
	}
	out = withText(out, v.Description, v.Location)
	out["history"], out["attachments"], out["data_mode"] = history, attachments, "SYNTHETIC"
	return out, nil
}

func pathReport(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "report_id")
	if !incident.CanonicalID(raw) {
		return "", orgapi.NotFound
	}
	return raw, nil
}

// RegisterIncidents mounts the four EMP-CHECK-003 routes: the queue on the
// organization target (queue authorization, no structure), one report and
// its handling on the report (structure through the stored snapshot) and the
// own list on the membership (own authorization).
func RegisterIncidents(router chi.Router, service app.IncidentService, cookies company.Handler, gateway orgapi.Gateway, queue, reports, own access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	for _, a := range []access.Authorization{queue, reports, own} {
		if missing(a.Source) || missing(a.Capabilities) || missing(a.Records) || a.Now == nil {
			return app.ErrUnavailable
		}
	}
	if router == nil || missing(service.Repository) || missing(reports.Structure) {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	list := func(path, binding string, action app.IncidentAction, authorization access.Authorization, resource access.ResourceResolver, page func(context.Context, orgapi.Principal, access.Decision, app.IncidentActor, app.ReportFilter) ([]map[string]any, string, error)) error {
		return guarded.Register(owned, path, gateway, IncidentRule(action), authorization, app.IncidentPolicy(action), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input incidentListRequest) (any, error) {
			actor, err := incidentActor(ctx)
			if err != nil {
				return nil, err
			}
			normalized, err := input.Page.Normalize()
			if err != nil {
				return nil, orgapi.Malformed
			}
			filter := app.ReportFilter{Limit: normalized.Limit}
			if filter.AfterID, err = gateway.OpenCursor(p, path, fmt.Sprintf("%s:%d", binding, normalized.Limit), normalized.Cursor); err != nil || filter.Validate() != nil {
				return nil, orgapi.Malformed
			}
			items, nextID, err := page(ctx, p, decision, actor, filter)
			if err != nil {
				return nil, err
			}
			next := ""
			if nextID != "" {
				if next, err = gateway.SealCursor(p, path, fmt.Sprintf("%s:%d", binding, normalized.Limit), nextID); err != nil {
					return nil, orgapi.Unavailable
				}
			}
			return map[string]any{"items": items, "next_cursor": next, "data_mode": "SYNTHETIC"}, nil
		})
	}
	if err := list(IncidentQueuePath, "incidents.reports.v1", app.IncidentQueue, queue, templateRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, actor app.IncidentActor, filter app.ReportFilter) ([]map[string]any, string, error) {
		result, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, "", incidentMapped(err)
		}
		items := make([]map[string]any, 0, len(result.Items))
		for _, v := range result.Items {
			item, err := projectReportSummary(v)
			if err != nil {
				return nil, "", err
			}
			items = append(items, item)
		}
		return items, result.NextID, nil
	}); err != nil {
		return err
	}
	if err := list(IncidentMinePath, "incidents.reports.mine.v1", app.IncidentOwn, own, ownReports{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, actor app.IncidentActor, filter app.ReportFilter) ([]map[string]any, string, error) {
		result, err := service.Mine(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, "", incidentMapped(err)
		}
		items := make([]map[string]any, 0, len(result.Items))
		for _, v := range result.Items {
			item, err := projectReportSummary(v.Summary)
			if err != nil {
				return nil, "", err
			}
			items = append(items, withText(item, v.Description, v.Location))
		}
		return items, result.NextID, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, IncidentReadPath, gateway, IncidentRule(app.IncidentRead), reports, app.IncidentPolicy(app.IncidentRead), reportResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ incidentReadRequest) (any, error) {
		actor, err := incidentActor(ctx)
		if err != nil {
			return nil, err
		}
		report, err := pathReport(ctx)
		if err != nil {
			return nil, err
		}
		view, err := service.Read(ctx, p.Scope, actor, decision, report)
		if err != nil {
			return nil, incidentMapped(err)
		}
		return projectReport(view)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, IncidentDecidePath, gateway, IncidentRule(app.IncidentDecide), reports, app.IncidentPolicy(app.IncidentDecide), reportResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input incidentDecideRequest) (any, error) {
		actor, err := incidentActor(ctx)
		if err != nil {
			return nil, err
		}
		report, err := pathReport(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		in := app.DecisionInput{OperationID: input.OperationID, ReportID: report, ExpectedRevision: *input.Revision, Action: incident.Action(input.Action), Note: input.Note}
		if input.DuplicateOf != nil {
			if *input.DuplicateOf == "" {
				return nil, incidentMapped(incident.ErrInvalid)
			}
			in.DuplicateOf = *input.DuplicateOf
		}
		command, err := app.NewDecisionCommand(in)
		if err != nil {
			return nil, incidentMapped(err)
		}
		result, replayed, err := service.Decide(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, incidentMapped(err)
		}
		at, err := stamp(result.RecordedAt)
		if err != nil {
			return nil, err
		}
		return map[string]any{"report_id": result.ReportID, "action": result.Step.Action, "state": result.Step.To, "revision": result.Step.Revision, "recorded_at": at, "replayed": replayed}, nil
	})
}
