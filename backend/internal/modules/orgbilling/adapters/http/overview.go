package http

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	document "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const PlanOverviewPath = "/v1/orgs/{org_id}/subscriptions/plan/read"

type planRecords struct{}

func (planRecords) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

func (planRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, target permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || scope != subject.Scope() || policy != billing.ReadPlanPolicy() ||
		target.OrgID != scope.OrgID() || target.ID != scope.OrgID().String() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	return nil
}

type readRequest struct{}

func (readRequest) Credentials() orgapi.Session   { return orgapi.Session{} }
func (readRequest) CommandFields() orgapi.Command { return orgapi.Command{} }
func (request *readRequest) UnmarshalJSON(raw []byte) error {
	type plain readRequest
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return orgapi.Malformed
	}
	return httpapi.DecodeJSON(raw, (*plain)(request))
}

type overviewResponse struct {
	ObservedAt time.Time  `json:"observed_at"`
	Plans      []planView `json:"plans"`
}

type pendingView struct {
	Name        string    `json:"name"`
	Revision    string    `json:"revision"`
	EffectiveAt time.Time `json:"effective_at"`
}

type termReceiptView struct {
	AmountCentavos string         `json:"amount_centavos"`
	Currency       string         `json:"currency"`
	SettledAt      time.Time      `json:"settled_at"`
	ProviderPaidOn string         `json:"payment_date,omitempty"`
	FiscalStatus   document.State `json:"fiscal_status"`
}

type planView struct {
	Product       string                   `json:"product"`
	Name          string                   `json:"name"`
	Revision      string                   `json:"revision"`
	State         domain.SubscriptionState `json:"state"`
	TermStart     time.Time                `json:"term_start"`
	TermEnd       time.Time                `json:"term_end"`
	TermCurrent   bool                     `json:"term_current"`
	ConfirmedPaid bool                     `json:"confirmed_paid"`
	Receipt       *termReceiptView         `json:"receipt,omitempty"`
	Capabilities  []string                 `json:"capabilities"`
	Limits        map[domain.Meter]string  `json:"limits"`
	Pending       *pendingView             `json:"pending,omitempty"`
}

func response(overview billing.Overview) overviewResponse {
	out := overviewResponse{ObservedAt: overview.ObservedAt, Plans: make([]planView, 0, len(overview.Plans))}
	for _, row := range overview.Plans {
		p := planView{Product: row.Catalog.Key().Product, Name: row.Catalog.Name(), Revision: strconv.FormatInt(row.Catalog.Key().Revision, 10),
			State: row.State, TermStart: row.Term.Start, TermEnd: row.Term.End, TermCurrent: row.TermCurrent,
			ConfirmedPaid: row.Receipt != nil, Capabilities: row.Catalog.Capabilities(), Limits: make(map[domain.Meter]string)}
		for _, meter := range []domain.Meter{domain.ActiveVehicles, domain.ActiveStations, domain.StorageBytes, domain.APICalls} {
			if limit, err := row.Catalog.Limit(meter); err == nil {
				p.Limits[meter] = strconv.FormatInt(limit, 10)
			}
		}
		if row.Receipt != nil {
			p.Receipt = &termReceiptView{AmountCentavos: strconv.FormatInt(row.Receipt.Amount.Centavos, 10),
				Currency: string(row.Receipt.Amount.Currency), SettledAt: row.Receipt.SettledAt,
				ProviderPaidOn: row.Receipt.PaidOn, FiscalStatus: document.NotConsulted}
		}
		if row.Pending != nil {
			p.Pending = &pendingView{Name: row.Pending.Catalog.Name(), Revision: strconv.FormatInt(row.Pending.Catalog.Key().Revision, 10),
				EffectiveAt: row.Pending.EffectiveAt}
		}
		out.Plans = append(out.Plans, p)
	}
	return out
}

func RegisterPlanOverview(router chi.Router, service billing.OverviewReader, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if service == nil {
		return billing.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	policy := billing.ReadPlanPolicy()
	authorization.Records = planRecords{}
	return guarded.Register(owned, PlanOverviewPath, gateway, orgapi.Rule{Permission: string(policy.Permission), CompanyOnly: true, Success: 200},
		authorization, policy, planRecords{}, func(ctx context.Context, p orgapi.Principal, _ access.Decision, _ readRequest) (any, error) {
			actor, err := organization.CurrentCompanyActor(ctx)
			if err != nil {
				return nil, orgapi.Unauthenticated
			}
			view, err := service.Read(ctx, p.Scope, actor, p.MembershipID)
			switch {
			case err == nil:
				return response(view), nil
			case errors.Is(err, permissions.ErrPermissionDenied):
				return nil, orgapi.MissingPermission(string(policy.Permission))
			case errors.Is(err, access.ErrNoMembership), errors.Is(err, orgapp.ErrNotFound):
				return nil, orgapi.NotFound
			case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, orgapp.ErrUnauthenticated):
				return nil, orgapi.Unauthenticated
			default:
				return nil, orgapi.Unavailable
			}
		})
}
