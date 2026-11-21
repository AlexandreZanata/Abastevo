package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	ReadReviewPath  = "/v1/orgs/{org_id}/fuelings/{fueling_id}/review/read"
	WriteReviewPath = "/v1/orgs/{org_id}/fuelings/{fueling_id}/review/decide"
)

type reviewRequest struct{}

func (reviewRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (r *reviewRequest) UnmarshalJSON(raw []byte) error {
	type plain reviewRequest
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return orgapi.Malformed
	}
	return httpapi.DecodeJSON(raw, (*plain)(r))
}

type decisionRequest struct {
	orgapi.Command
	Decision        review.Kind       `json:"decision"`
	Reason          string            `json:"reason,omitempty"`
	WarningNotes    map[string]string `json:"warning_notes,omitempty"`
	FuelEnteredTank *bool             `json:"fuel_entered_tank,omitempty"`
}

type reviewRecords struct{ policy access.Policy }

func (r reviewRecords) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	id := chi.URLParamFromCtx(ctx, "fueling_id")
	if !audit.ValidID(id) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}

func (r reviewRecords) Check(ctx context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || scope != subject.Scope() || policy != r.policy ||
		facts.OrgID != scope.OrgID() || facts.ID != chi.URLParamFromCtx(ctx, "fueling_id") || len(rules) == 0 {
		return access.ErrUnavailable
	}
	return nil
}

type response struct {
	app.ReviewStatus
	Replayed bool `json:"replayed"`
}

func RegisterReview(router chi.Router, service app.ReviewService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if service == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	if err := registerReview(owned, ReadReviewPath, gateway, authorization, false,
		func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, id string, _ reviewRequest) (any, error) {
			view, err := service.ReadReview(ctx, p.Scope, actor, p.MembershipID, id)
			return view, mapReviewError(err)
		}); err != nil {
		return err
	}
	return registerReview(owned, WriteReviewPath, gateway, authorization, true,
		func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, id string, request decisionRequest) (any, error) {
			status, replayed, err := service.WriteReview(ctx, p.Scope, actor, p.MembershipID, id, request.OperationID,
				app.ReviewInput{Kind: request.Decision, ExpectedSequence: *request.Revision, Reason: request.Reason,
					WarningNotes: request.WarningNotes, FuelEnteredTank: request.FuelEnteredTank})
			return response{ReviewStatus: status, Replayed: replayed}, mapReviewError(err)
		})
}

func registerReview[T orgapi.Request](router chi.Router, path string, gateway orgapi.Gateway, authorization access.Authorization,
	write bool, serve func(context.Context, orgapi.Principal, orgapp.CompanyActor, string, T) (any, error),
) error {
	policy := app.ReviewPolicy(write)
	records := reviewRecords{policy: policy}
	authorization.Records = records
	return guarded.Register(router, path, gateway, orgapi.Rule{
		Permission: string(policy.Permission), Write: write, Fresh: policy.Fresh, RequireRevision: write, CompanyOnly: true, Success: http.StatusOK,
	}, authorization, policy, records, func(ctx context.Context, p orgapi.Principal, _ access.Decision, request T) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, orgapi.Unauthenticated
		}
		return serve(ctx, p, actor, chi.URLParamFromCtx(ctx, "fueling_id"), request)
	})
}

func mapReviewError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, review.ErrSelfApproval):
		return orgapi.SelfApproval
	case errors.Is(err, review.ErrForbidden), errors.Is(err, permissions.ErrPermissionDenied):
		return orgapi.MissingPermission("fuel.fueling.review")
	case errors.Is(err, app.ErrNotFound), errors.Is(err, orgapp.ErrNotFound), errors.Is(err, access.ErrNoMembership):
		return orgapi.NotFound
	case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, orgapp.ErrUnauthenticated):
		return orgapi.Unauthenticated
	case errors.Is(err, access.ErrFreshAuthRequired):
		return orgapi.FreshAuthRequired
	case errors.Is(err, access.ErrOrgSuspended):
		return orgapi.OrgSuspended
	case errors.Is(err, access.ErrOrgArchived):
		return orgapi.OrgArchived
	case errors.Is(err, access.ErrSetupIncomplete):
		return orgapi.OrgSetupIncomplete
	case errors.Is(err, access.ErrEntitlementRequired), errors.Is(err, capability.ErrEntitlementRequired):
		return orgapi.EntitlementRequired
	case errors.Is(err, review.ErrInvalid), errors.Is(err, review.ErrReasonRequired), errors.Is(err, app.ErrWarningNotesRequired):
		return orgapi.ValidationFailed
	case errors.Is(err, review.ErrConflict), errors.Is(err, review.ErrInvalidTransition), errors.Is(err, app.ErrDuplicateUnresolved):
		return orgapi.Conflict
	case errors.Is(err, audit.ErrReused):
		return orgapi.OperationReused
	default:
		return orgapi.Unavailable
	}
}
