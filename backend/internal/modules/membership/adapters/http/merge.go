package http

import (
	"context"
	"errors"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const EmployeeMergePrefix = EmployeePrefix + "/{employee_id}/merge"
const EmployeeMergePreviewPath = EmployeeMergePrefix + "/preview"
const EmployeeMergeConfirmPath = EmployeeMergePrefix + "/confirm"
const EmployeeResolvePath = EmployeePrefix + "/{employee_id}/resolution/read"

type mergePreviewRequest struct {
	companyRequest
	SurvivorID string `json:"survivor_id"`
}

func (v *mergePreviewRequest) UnmarshalJSON(raw []byte) error {
	type plain mergePreviewRequest
	return decodeRedemption(raw, (*plain)(v), "survivor_id")
}

type mergeRequest struct {
	companyRequest
	SchemaVersion    int    `json:"schema_version"`
	OperationID      string `json:"operation_id"`
	Revision         *int64 `json:"revision"`
	SurvivorID       string `json:"survivor_id"`
	SurvivorRevision int64  `json:"survivor_revision"`
	ReviewDigest     string `json:"review_digest"`
	Reviewed         bool   `json:"reviewed"`
	Reason           string `json:"reason"`
}

func (v mergeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *mergeRequest) UnmarshalJSON(raw []byte) error {
	type plain mergeRequest
	return decodeRedemption(raw, (*plain)(v), "schema_version", "operation_id", "revision", "survivor_id", "survivor_revision", "review_digest", "reviewed", "reason")
}

type EmployeeMergeRecords struct{}

func (EmployeeMergeRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 || (policy != app.EmployeeMergePolicy(false) && policy != app.EmployeeMergePolicy(true)) {
		return access.ErrUnavailable
	}
	return nil
}
func employeeMergeRule(write bool) orgapi.Rule {
	p := app.EmployeeMergePolicy(write)
	return orgapi.Rule{Permission: string(p.Permission), Write: write, Fresh: true, RequireRevision: write, CompanyOnly: true, Success: 200}
}
func mergeMapped(e error) error {
	if errors.Is(e, d.ErrMergeConflict) {
		return orgapi.MergeConflict
	}
	return membershipAccessMapped(e)
}
func mergeView(scope kernel.OrgScope, v app.EmployeeMergeView) (map[string]any, error) {
	a, e := employeeView(scope, v.Source)
	if e != nil {
		return nil, e
	}
	b, e := employeeView(scope, v.Survivor)
	if e != nil {
		return nil, e
	}
	return map[string]any{"source": a, "survivor": b, "source_live_periods": v.SourceLive, "survivor_live_periods": v.SurvivorLive, "teams": v.Teams, "review_digest": v.Digest, "data_mode": "SYNTHETIC"}, nil
}
func mergeResponse(v app.EmployeeMergeResult) map[string]any {
	return map[string]any{"source_id": v.SourceID, "survivor_id": v.SurvivorID, "source_state": v.SourceState, "survivor_state": v.SurvivorState, "source_revision": v.SourceRevision, "survivor_revision": v.SurvivorRevision, "operation_id": v.OperationID, "reason": v.Reason, "occurred_at": v.OccurredAt, "replayed": v.Replayed, "team_effects": v.Teams, "data_mode": "SYNTHETIC"}
}
func RegisterEmployeeMerge(router chi.Router, service app.EmployeeMergeService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots org.Repository) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || roots == nil {
		return app.ErrRegisterUnavailable
	}
	owned, g, e := orgweb.CompanyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	facts := orgweb.RootFacts{Roots: roots}
	if e := guarded.Register(owned, EmployeeMergePreviewPath, g, employeeMergeRule(false), authorization, app.EmployeeMergePolicy(false), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input mergePreviewRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := pathID(ctx)
		if e != nil {
			return nil, e
		}
		v, e := service.Preview(ctx, p.Scope, a, decision, id, input.SurvivorID)
		if e != nil {
			return nil, mergeMapped(e)
		}
		return mergeView(p.Scope, v)
	}); e != nil {
		return e
	}
	if e := guarded.Register(owned, EmployeeMergeConfirmPath, g, employeeMergeRule(true), authorization, app.EmployeeMergePolicy(true), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input mergeRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := pathID(ctx)
		if e != nil {
			return nil, e
		}
		out, e := service.Merge(ctx, p.Scope, a, decision, app.EmployeeMergeCommand{OperationID: input.OperationID, SourceID: id, SurvivorID: input.SurvivorID, SourceRevision: *input.Revision, SurvivorRevision: input.SurvivorRevision, ReviewDigest: input.ReviewDigest, Reviewed: input.Reviewed, Reason: input.Reason})
		if e != nil {
			return nil, mergeMapped(e)
		}
		return mergeResponse(out), nil
	}); e != nil {
		return e
	}
	return guarded.Register(owned, EmployeeResolvePath, g, employeeMergeRule(false), authorization, app.EmployeeMergePolicy(false), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ readRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := pathID(ctx)
		if e != nil {
			return nil, e
		}
		v, e := service.Resolve(ctx, p.Scope, a, decision, id)
		if e != nil {
			return nil, mergeMapped(e)
		}
		source, e := employeeView(p.Scope, v.Source)
		if e != nil {
			return nil, e
		}
		canonical, e := employeeView(p.Scope, v.Canonical)
		if e != nil {
			return nil, e
		}
		return map[string]any{"source": source, "canonical": canonical, "chain": v.Chain, "data_mode": "SYNTHETIC"}, nil
	})
}
