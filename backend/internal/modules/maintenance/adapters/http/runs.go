package http

// EMP-CHECK-002 company routes (EDR-285): one run of an asset under
// fleet.asset.read and the release of a blocking run under fresh
// check.incident.manage; EMP-CHECK-004 (EDR-328): the organization's run
// queue under the incident queue's policy.

import (
	"context"
	"fmt"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const RunReadPath = ChecklistPrefix + "/assets/{asset_id}/runs/{run_id}/read"
const RunReleasePath = ChecklistPrefix + "/runs/{run_id}/release"
const RunQueuePath = ChecklistPrefix + "/runs/list"

// runResource resolves the path run as the protected record of a release.
type runResource struct{}

func (runResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id := chi.URLParamFromCtx(ctx, "run_id")
	if !checklist.CanonicalID(id) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}

type runReadRequest struct{ companyRequest }
type runQueueRequest struct {
	companyRequest
	orgapi.Page
}
type runReleaseRequest struct {
	commandFields
	Reason *string `json:"reason"`
}

func (v *runReadRequest) UnmarshalJSON(raw []byte) error {
	type plain runReadRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = runReadRequest(in)
	return nil
}
func (v *runQueueRequest) UnmarshalJSON(raw []byte) error {
	type plain runQueueRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = runQueueRequest(in)
	return nil
}
func (v *runReleaseRequest) UnmarshalJSON(raw []byte) error {
	type plain runReleaseRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = runReleaseRequest(in)
	return nil
}

// RunRule returns the exact orgapi rule of a run action.
func RunRule(action app.RunCompanyAction) orgapi.Rule {
	policy := app.RunCompanyPolicy(action)
	return orgapi.Rule{Permission: string(policy.Permission), Write: policy.Effect.Write(), Fresh: policy.Fresh, RequireRevision: action == app.RunRelease, CompanyOnly: true, Success: 200}
}

// runQueueBinding binds a sealed queue cursor to its page size.
func runQueueBinding(limit int) string { return fmt.Sprintf("checklists.runs.queue.v1:%d", limit) }

func ints(values []int) []int {
	if values == nil {
		return []int{}
	}
	return append([]int(nil), values...)
}

// projectRun shows both outcomes, the flags and the positions of the issues;
// never a note, a photo or the author's identity (B-BR-EMP-441).
func projectRun(v app.RunView) (map[string]any, error) {
	started, err := stamp(v.StartedAt)
	if err != nil {
		return nil, err
	}
	completed, err := stamp(v.CompletedAt)
	if err != nil {
		return nil, err
	}
	received, err := stamp(v.ReceivedAt)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"run_id": v.RunID, "asset_id": v.AssetID, "template_id": v.TemplateID, "context": v.Context, "template_version": v.Version,
		"policy_revision": v.PolicyRevision, "server_policy_revision": v.ServerPolicyRevision, "revision": v.Revision, "started_at": started, "completed_at": completed,
		"received_at": received, "device_outcome": v.DeviceOutcome, "recheck_outcome": v.RecheckOutcome, "outcome": v.Outcome, "outcome_differs": v.Differs,
		"review_required": v.Differs, "template_superseded": v.Superseded, "acknowledged": v.Acknowledged, "issues": ints(v.Issues), "critical_issues": ints(v.Critical),
		"data_mode": "SYNTHETIC"}
	if v.ReleasedAt != nil {
		at, err := stamp(*v.ReleasedAt)
		if err != nil {
			return nil, err
		}
		out["released_at"] = at
	}
	return out, nil
}

func pathID(ctx context.Context, name string) (string, error) {
	raw := chi.URLParamFromCtx(ctx, name)
	if !checklist.CanonicalID(raw) {
		return "", orgapi.NotFound
	}
	return raw, nil
}

// RegisterRuns mounts the two EMP-CHECK-002 routes: the read of a run on its
// asset (the applicable-read authorization: asset structure and SELF
// assignee) and the release on the stored run (structure through the run's
// stored snapshot); and the EMP-CHECK-004 queue on the organization (the
// incident queue's authorization, no structure).
func RegisterRuns(router chi.Router, service app.RunService, cookies company.Handler, gateway orgapi.Gateway, assets, runs, queue access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	for _, a := range []access.Authorization{assets, runs} {
		if missing(a.Source) || missing(a.Capabilities) || missing(a.Records) || a.Now == nil || missing(a.Structure) {
			return app.ErrUnavailable
		}
	}
	if missing(queue.Source) || missing(queue.Capabilities) || missing(queue.Records) || queue.Now == nil {
		return app.ErrUnavailable
	}
	if router == nil || missing(service.Repository) {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	if err := guarded.Register(owned, RunQueuePath, gateway, RunRule(app.RunQueue), queue, app.RunCompanyPolicy(app.RunQueue), templateRoot{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input runQueueRequest) (any, error) {
		actor, err := incidentActor(ctx)
		if err != nil {
			return nil, err
		}
		normalized, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		raw, err := gateway.OpenCursor(p, RunQueuePath, runQueueBinding(normalized.Limit), normalized.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		after, err := app.ParseRunQueueKey(raw)
		filter := app.RunQueueFilter{After: after, Limit: normalized.Limit}
		if err != nil || filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		page, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, incidentMapped(err)
		}
		items := make([]map[string]any, 0, len(page.Items))
		for _, v := range page.Items {
			item, err := projectRun(v)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		next := ""
		if page.Next != nil {
			if next, err = gateway.SealCursor(p, RunQueuePath, runQueueBinding(normalized.Limit), page.Next.Cursor()); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, RunReadPath, gateway, RunRule(app.RunRead), assets, app.RunCompanyPolicy(app.RunRead), assetResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ runReadRequest) (any, error) {
		actor, err := incidentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset, err := pathID(ctx, "asset_id")
		if err != nil {
			return nil, err
		}
		run, err := pathID(ctx, "run_id")
		if err != nil {
			return nil, err
		}
		view, err := service.Read(ctx, p.Scope, actor, decision, asset, run)
		if err != nil {
			return nil, incidentMapped(err)
		}
		return projectRun(view)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, RunReleasePath, gateway, RunRule(app.RunRelease), runs, app.RunCompanyPolicy(app.RunRelease), runResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input runReleaseRequest) (any, error) {
		actor, err := incidentActor(ctx)
		if err != nil {
			return nil, err
		}
		run, err := pathID(ctx, "run_id")
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		reason := ""
		if input.Reason != nil {
			reason = *input.Reason
		}
		command, err := app.NewReleaseCommand(app.ReleaseInput{OperationID: input.OperationID, RunID: run, ExpectedRevision: *input.Revision, Reason: reason})
		if err != nil {
			return nil, incidentMapped(err)
		}
		view, replayed, err := service.Release(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, incidentMapped(err)
		}
		out, err := projectRun(view)
		if err != nil {
			return nil, err
		}
		out["replayed"] = replayed
		return out, nil
	})
}
