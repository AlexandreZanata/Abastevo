// Package http exposes the guarded Phase F7 endpoints: the company routes
// for station agreements, fueling authorizations and redemption reads, and
// the station-session routes for the station party's agreements, decisions
// and once-only redemption confirmation.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/domain"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	StationAgreementsProposePath    = "/v1/orgs/{org_id}/station-agreements/propose"
	StationAgreementsReadPath       = "/v1/orgs/{org_id}/station-agreements/{agreement_id}/read"
	StationAgreementsListPath       = "/v1/orgs/{org_id}/station-agreements/list"
	StationAgreementsSuspendPath    = "/v1/orgs/{org_id}/station-agreements/{agreement_id}/suspend"
	StationAgreementsEndPath        = "/v1/orgs/{org_id}/station-agreements/{agreement_id}/end"
	FuelingAuthorizationsIssuePath  = "/v1/orgs/{org_id}/fueling-authorizations/issue"
	FuelingAuthorizationsReadPath   = "/v1/orgs/{org_id}/fueling-authorizations/{authorization_id}/read"
	FuelingAuthorizationsListPath   = "/v1/orgs/{org_id}/fueling-authorizations/list"
	FuelingAuthorizationsCancelPath = "/v1/orgs/{org_id}/fueling-authorizations/{authorization_id}/cancel"
	FuelingRedemptionsListPath      = "/v1/orgs/{org_id}/fueling-redemptions/list"
)

// AgreementRecords validates the fixed F7 policies and targets at the auth
// boundary; the catalog requirements (fleet side, live assignment) are
// enforced by the authorization service.
type AgreementRecords struct{}

func (AgreementRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	switch policy {
	case app.AgreementPolicy(false), app.AgreementPolicy(true),
		app.AuthorizationPolicy(false), app.AuthorizationPolicy(true):
		return nil
	}
	return access.ErrUnavailable
}

type orgRootResource struct{}

func (orgRootResource) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String(), AgreementSide: permissions.FleetParty}, nil
}

type pathIDResource struct{ key string }

func (r pathIDResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	raw := chi.URLParamFromCtx(ctx, r.key)
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: raw, AgreementSide: permissions.FleetParty}, nil
}

func missing(v any) bool {
	if v == nil {
		return true
	}
	type isEmpty interface{ IsZero() bool }
	if z, ok := v.(isEmpty); ok {
		return z.IsZero()
	}
	return false
}

func strictObject[T any](raw []byte, target *T) error {
	if httpapi.DecodeJSON(raw, target) != nil {
		return orgapi.Malformed
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&fields) != nil || fields == nil {
		return orgapi.Malformed
	}
	return nil
}

func readRule(p access.Policy) orgapi.Rule {
	return orgapi.Rule{Permission: string(p.Permission), Write: false, Fresh: p.Fresh, CompanyOnly: true, Success: 200}
}

func writeRule(p access.Policy, requireRevision bool) orgapi.Rule {
	return orgapi.Rule{Permission: string(p.Permission), Write: true, Fresh: p.Fresh, RequireRevision: requireRevision, CompanyOnly: true, Success: 200}
}

func mappedError(err error) error {
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	switch {
	case errors.Is(err, app.ErrUnauthenticated), errors.Is(err, access.ErrUnauthenticated):
		return orgapi.Unauthenticated
	case errors.Is(err, app.ErrNotFound), errors.Is(err, permissions.ErrResourceNotFound):
		return orgapi.NotFound
	case errors.Is(err, app.ErrOperationReused):
		return orgapi.OperationReused
	case errors.Is(err, app.ErrConflict):
		return orgapi.Conflict
	case errors.Is(err, app.ErrInvalid):
		return orgapi.ValidationFailed
	case errors.Is(err, app.ErrStationMismatch):
		return orgapi.ValidationFailed
	case errors.Is(err, access.ErrFreshAuthRequired):
		return orgapi.FreshAuthRequired
	case errors.Is(err, access.ErrEntitlementRequired):
		return orgapi.EntitlementRequired
	case errors.Is(err, permissions.ErrPermissionDenied):
		return orgapi.Forbidden
	}
	return orgapi.Unavailable
}

func companyActor(ctx context.Context) (app.Actor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.Actor{}, err
	}
	return app.NewActor(actor.AccountID, actor.SessionID)
}

type emptyRequest struct{}

func (emptyRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v *emptyRequest) UnmarshalJSON(raw []byte) error {
	type plain emptyRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = emptyRequest(in)
	return nil
}

type proposeRequest struct {
	SchemaVersion int             `json:"schema_version"`
	OperationID   string          `json:"operation_id"`
	StationID     string          `json:"station_id"`
	OrgLabel      string          `json:"org_label"`
	ValidFrom     string          `json:"valid_from"`
	ValidUntil    string          `json:"valid_until"`
	Conditions    string          `json:"conditions"`
	Terms         []app.TermInput `json:"terms"`
}

func (proposeRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v proposeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *proposeRequest) UnmarshalJSON(raw []byte) error {
	type plain proposeRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = proposeRequest(in)
	return nil
}

func parseInstant(raw string) (time.Time, error) { return time.Parse(time.RFC3339Nano, raw) }

type transitionRequest struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      int64  `json:"revision"`
	Reason        string `json:"reason"`
}

func (transitionRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v transitionRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: &v.Revision}
}
func (v *transitionRequest) UnmarshalJSON(raw []byte) error {
	type plain transitionRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = transitionRequest(in)
	return nil
}

type issueRequest struct {
	SchemaVersion    int    `json:"schema_version"`
	OperationID      string `json:"operation_id"`
	AgreementID      string `json:"agreement_id"`
	EmployeeID       string `json:"employee_id"`
	AssetID          string `json:"asset_id"`
	FuelProduct      string `json:"fuel_product"`
	Unit             string `json:"unit"`
	MaxQuantityMilli int64  `json:"max_quantity_milli"`
	MaxCentavos      int64  `json:"max_centavos"`
	ValidUntil       string `json:"valid_until"`
}

func (issueRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v issueRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *issueRequest) UnmarshalJSON(raw []byte) error {
	type plain issueRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = issueRequest(in)
	return nil
}

type cancelRequest struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Reason        string `json:"reason"`
}

func (cancelRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v cancelRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *cancelRequest) UnmarshalJSON(raw []byte) error {
	type plain cancelRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = cancelRequest(in)
	return nil
}

// RegisterCompany mounts the guarded company endpoints for Phase F7.
func RegisterCompany(router chi.Router, service app.Service, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	agreement := pathIDResource{"agreement_id"}
	authorizationTarget := pathIDResource{"authorization_id"}

	if err := guarded.Register(owned, StationAgreementsProposePath, gateway, writeRule(app.AgreementPolicy(true), false), authorization, app.AgreementPolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in proposeRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		from, err := parseInstant(in.ValidFrom)
		if err != nil {
			return nil, orgapi.ValidationFailed
		}
		until, err := parseInstant(in.ValidUntil)
		if err != nil {
			return nil, orgapi.ValidationFailed
		}
		view, replayed, err := service.Propose(ctx, p.Scope, actor, p.MembershipID, app.ProposeInput{
			OperationID: in.OperationID, StationID: in.StationID, OrgLabel: in.OrgLabel,
			ValidFrom: from, ValidUntil: until, Conditions: in.Conditions, Terms: in.Terms,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"agreement": view, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, StationAgreementsReadPath, gateway, readRule(app.AgreementPolicy(false)), authorization, app.AgreementPolicy(false), agreement, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, err := service.ReadAgreement(ctx, p.Scope, actor, p.MembershipID, chi.URLParamFromCtx(ctx, "agreement_id"))
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"agreement": view, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, StationAgreementsListPath, gateway, readRule(app.AgreementPolicy(false)), authorization, app.AgreementPolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		views, err := service.ListAgreements(ctx, p.Scope, actor, p.MembershipID)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"agreements": views, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	suspend := func(ctx context.Context, p orgapi.Principal, decision access.Decision, in transitionRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, replayed, err := service.Apply(ctx, p.Scope, actor, p.MembershipID, app.AgreementTransitionInput{
			OperationID: in.OperationID, AgreementID: chi.URLParamFromCtx(ctx, "agreement_id"), Revision: in.Revision, Reason: in.Reason,
		}, "SUSPEND")
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"agreement": view, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}
	if err := guarded.Register(owned, StationAgreementsSuspendPath, gateway, writeRule(app.AgreementPolicy(true), true), authorization, app.AgreementPolicy(true), agreement, suspend); err != nil {
		return err
	}
	end := func(ctx context.Context, p orgapi.Principal, decision access.Decision, in transitionRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, replayed, err := service.Apply(ctx, p.Scope, actor, p.MembershipID, app.AgreementTransitionInput{
			OperationID: in.OperationID, AgreementID: chi.URLParamFromCtx(ctx, "agreement_id"), Revision: in.Revision, Reason: in.Reason,
		}, "END")
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"agreement": view, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}
	if err := guarded.Register(owned, StationAgreementsEndPath, gateway, writeRule(app.AgreementPolicy(true), true), authorization, app.AgreementPolicy(true), agreement, end); err != nil {
		return err
	}
	if err := guarded.Register(owned, FuelingAuthorizationsIssuePath, gateway, writeRule(app.AuthorizationPolicy(true), false), authorization, app.AuthorizationPolicy(true), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in issueRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		until, err := parseInstant(in.ValidUntil)
		if err != nil {
			return nil, orgapi.ValidationFailed
		}
		ack, replayed, err := service.Issue(ctx, p.Scope, actor, p.MembershipID, app.IssueInput{
			OperationID: in.OperationID, AgreementID: in.AgreementID, EmployeeID: in.EmployeeID, AssetID: in.AssetID,
			Product: in.FuelProduct, Unit: in.Unit, MaxQuantityMilli: in.MaxQuantityMilli, MaxCentavos: in.MaxCentavos, ValidUntil: until,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		if replayed {
			// The one-time code never repeats: a replay answers the stored
			// authorization view only.
			return map[string]any{"authorization": ack.Authorization, "replayed": true, "data_mode": "SYNTHETIC"}, nil
		}
		return map[string]any{"authorization": ack.Authorization, "code": ack.Code, "replayed": false, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, FuelingAuthorizationsReadPath, gateway, readRule(app.AuthorizationPolicy(false)), authorization, app.AuthorizationPolicy(false), authorizationTarget, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, err := service.ReadAuthorization(ctx, p.Scope, actor, p.MembershipID, chi.URLParamFromCtx(ctx, "authorization_id"))
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"authorization": view, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, FuelingAuthorizationsListPath, gateway, readRule(app.AuthorizationPolicy(false)), authorization, app.AuthorizationPolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		views, err := service.ListAuthorizations(ctx, p.Scope, actor, p.MembershipID)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"authorizations": views, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, FuelingAuthorizationsCancelPath, gateway, writeRule(app.AuthorizationPolicy(true), false), authorization, app.AuthorizationPolicy(true), authorizationTarget, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in cancelRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		view, replayed, err := service.Cancel(ctx, p.Scope, actor, p.MembershipID, app.CancelInput{
			OperationID: in.OperationID, AuthorizationID: chi.URLParamFromCtx(ctx, "authorization_id"), Reason: in.Reason,
		})
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"authorization": view, "replayed": replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, FuelingRedemptionsListPath, gateway, readRule(app.AgreementPolicy(false)), authorization, app.AgreementPolicy(false), orgRootResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ emptyRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, mappedError(err)
		}
		views, err := service.ListRedemptions(ctx, p.Scope, actor, p.MembershipID)
		if err != nil {
			return nil, mappedError(err)
		}
		return map[string]any{"redemptions": views, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}
	return nil
}

// --- Station-session endpoints ---

// StationHandler serves the station party's Phase F7 surface through the
// same session proof the station profile uses: the live grant scope is
// rechecked server-side on every request and responses carry no-store.
type StationHandler struct {
	Service   app.Service
	Sessions  func(ctx context.Context, familyID, accessToken string) (string, error)
	Authority app.StationAuthority
}

// RegisterStationRoutes mounts the additive station paths.
func (h StationHandler) RegisterStationRoutes(r chi.Router) {
	r.Post("/v1/stations/{station_id}/agreements/list", h.list)
	r.Post("/v1/stations/{station_id}/agreements/{agreement_id}/accept", h.accept)
	r.Post("/v1/stations/{station_id}/agreements/{agreement_id}/decline", h.decline)
	r.Post("/v1/stations/{station_id}/redemptions/confirm", h.confirm)
}

type stationSessionDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

type stationDecisionDTO struct {
	stationSessionDTO
	Reason string `json:"reason"`
}

type stationConfirmDTO struct {
	stationSessionDTO
	OperationID   string `json:"operation_id"`
	Code          string `json:"code"`
	FuelProduct   string `json:"fuel_product"`
	Unit          string `json:"unit"`
	QuantityMilli int64  `json:"quantity_milli"`
}

// writeNoStore answers one bounded station response with no-store.
func writeNoStore(w http.ResponseWriter, r *http.Request, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "agreement.unavailable", "agreement temporarily unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", raw)
}

func readStationBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	raw, err := httpapi.ReadBody(r, 8<<10)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "agreement.bad-request", "invalid agreement input", nil)
		return false
	}
	if httpapi.DecodeJSON(raw, dst) != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "agreement.bad-request", "invalid agreement input", nil)
		return false
	}
	return true
}

func (h StationHandler) stationActor(w http.ResponseWriter, r *http.Request, dto stationSessionDTO) (app.StationActor, string, bool) {
	accountID, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil || accountID == "" {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "agreement.session-invalid", "valid account session required", nil)
		return app.StationActor{}, "", false
	}
	actor, err := app.NewStationActor(accountID)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "agreement.session-invalid", "valid account session required", nil)
		return app.StationActor{}, "", false
	}
	stationID := chi.URLParam(r, "station_id")
	if _, err := kernel.ParseOrgID(stationID); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "agreement.bad-request", "invalid station", nil)
		return app.StationActor{}, "", false
	}
	return actor, stationID, true
}

func (h StationHandler) list(w http.ResponseWriter, r *http.Request) {
	var dto stationSessionDTO
	if !readStationBody(w, r, &dto) {
		return
	}
	actor, stationID, ok := h.stationActor(w, r, dto)
	if !ok {
		return
	}
	views, err := h.Service.StationAgreements(r.Context(), stationID, actor, h.Authority)
	if err != nil {
		writeStationError(w, r, err)
		return
	}
	writeNoStore(w, r, map[string]any{"agreements": views})
}

func (h StationHandler) accept(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, true)
}

func (h StationHandler) decline(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, false)
}

func (h StationHandler) decide(w http.ResponseWriter, r *http.Request, accept bool) {
	var dto stationDecisionDTO
	if !readStationBody(w, r, &dto) {
		return
	}
	actor, stationID, ok := h.stationActor(w, r, dto.stationSessionDTO)
	if !ok {
		return
	}
	agreementID := chi.URLParam(r, "agreement_id")
	if _, err := kernel.ParseOrgID(agreementID); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "agreement.bad-request", "invalid agreement", nil)
		return
	}
	// A repeated decision is answered by the recorded state machine: the
	// CAS transition refuses the second command and every attempt is
	// audited, so no client operation identifier is accepted here.
	var view app.StationAgreementView
	var err error
	if accept {
		view, _, err = h.Service.StationAccept(r.Context(), stationID, actor, h.Authority, app.StationDecisionInput{AgreementID: agreementID, Reason: dto.Reason})
	} else {
		view, _, err = h.Service.StationDecline(r.Context(), stationID, actor, h.Authority, app.StationDecisionInput{AgreementID: agreementID, Reason: dto.Reason})
	}
	if err != nil {
		writeStationError(w, r, err)
		return
	}
	writeNoStore(w, r, map[string]any{"agreement": view})
}

func (h StationHandler) confirm(w http.ResponseWriter, r *http.Request) {
	var dto stationConfirmDTO
	if !readStationBody(w, r, &dto) {
		return
	}
	actor, stationID, ok := h.stationActor(w, r, dto.stationSessionDTO)
	if !ok {
		return
	}
	operationID := dto.OperationID
	if _, err := kernel.ParseOrgID(operationID); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "agreement.bad-request", "invalid redemption input", nil)
		return
	}
	view, replayed, err := h.Service.StationConfirmRedemption(r.Context(), stationID, actor, h.Authority, app.ConfirmInput{
		OperationID: operationID, Code: dto.Code, Product: dto.FuelProduct, Unit: dto.Unit, QuantityMilli: dto.QuantityMilli,
	})
	if err != nil {
		writeStationError(w, r, err)
		return
	}
	writeNoStore(w, r, map[string]any{
		"redemption": map[string]any{
			"id": view.ID, "agreement_id": view.AgreementID, "authorization_id": view.AuthorizationID,
			"station_id": view.StationID, "fuel_product": view.Product, "unit": view.Unit,
			"quantity_milli": view.QuantityMilli, "price_milli_brl": view.PriceMilliBRL,
			"total_centavos": view.TotalCentavos, "confirmed_at": view.ConfirmedAt,
		},
		"replayed": replayed,
	})
}

// operationOf is removed: decisions replay through the state machine.

func writeStationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrUnauthenticated), errors.Is(err, access.ErrUnauthenticated):
		httpapi.WriteError(w, r, http.StatusUnauthorized, "agreement.session-invalid", "valid account session required", nil)
	case errors.Is(err, app.ErrNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "agreement.not-found", "agreement not found", nil)
	case errors.Is(err, app.ErrOperationReused):
		httpapi.WriteError(w, r, http.StatusConflict, "agreement.operation-reused", "operation identifier already used", nil)
	case errors.Is(err, app.ErrConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "agreement.conflict", "agreement state conflict", nil)
	case errors.Is(err, app.ErrStationMismatch):
		httpapi.WriteError(w, r, http.StatusConflict, "agreement.station-mismatch", "voucher does not belong to this station", nil)
	case errors.Is(err, d.ErrAuthorizationLocked):
		httpapi.WriteError(w, r, http.StatusConflict, "agreement.authorization-locked", "voucher locked after bounded attempts", nil)
	case errors.Is(err, d.ErrAuthorizationTransitionRefused):
		httpapi.WriteError(w, r, http.StatusConflict, "agreement.authorization-state", "voucher no longer issuable", nil)
	case errors.Is(err, d.ErrAuthorizationExpired):
		httpapi.WriteError(w, r, http.StatusConflict, "agreement.authorization-expired", "voucher expired", nil)
	case errors.Is(err, d.ErrAuthorizationProductMismatch):
		httpapi.WriteError(w, r, http.StatusUnprocessableEntity, "agreement.authorization-mismatch", "product or unit does not match", nil)
	case errors.Is(err, d.ErrAuthorizationQuantityExceeded):
		httpapi.WriteError(w, r, http.StatusUnprocessableEntity, "agreement.quantity-exceeded", "quantity exceeds the authorization limit", nil)
	case errors.Is(err, d.ErrAuthorizationTotalExceeded):
		httpapi.WriteError(w, r, http.StatusUnprocessableEntity, "agreement.total-exceeded", "total exceeds the authorization limit", nil)
	case errors.Is(err, app.ErrInvalid):
		httpapi.WriteError(w, r, http.StatusBadRequest, "agreement.bad-request", "invalid agreement input", nil)
	default:
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "agreement.unavailable", "agreement temporarily unavailable", nil)
	}
}
