// Package http exposes the source-owned synthetic organization setup commands.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const CreationPath = "/v1/org-access/organizations/create"
const ActivationPath = "/v1/orgs/{org_id}/organization/activate"

type privateRequest struct{}

func (privateRequest) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, "OrganizationRequest(redacted)")
}
func (privateRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal("OrganizationRequest(redacted)")
}
func (privateRequest) LogValue() slog.Value { return slog.StringValue("OrganizationRequest(redacted)") }

type creationRequest struct {
	privateRequest
	SchemaVersion int      `json:"schema_version"`
	OperationID   string   `json:"operation_id"`
	Name          string   `json:"name"`
	TimeZone      string   `json:"time_zone"`
	Kinds         []d.Kind `json:"kinds"`
	UnitName      string   `json:"unit_name"`
	Terms         struct {
		Version  string `json:"version"`
		Digest   string `json:"digest"`
		Accepted bool   `json:"accepted"`
	} `json:"terms"`
}

func (creationRequest) String() string     { return "OrganizationCreation(redacted)" }
func (v creationRequest) GoString() string { return v.String() }

// Activation is a PANEL command: account proof is exchanged through login,
// never carried in this business DTO.
type activationRequest struct {
	privateRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
}

func (activationRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v activationRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

// CreationAdmission spends the creating account's write budget in its own
// committed admission, before the creation transaction, so refused and
// rolled-back attempts still count. A refusal is a registered orgapi error.
type CreationAdmission func(ctx context.Context, accountID string) error

type creationEndpoint struct {
	service app.Onboarding
	company company.Handler
	admit   CreationAdmission
}

func (creationEndpoint) OrganizationCreationRule() orgapi.OrganizationCreationRule {
	return orgapi.OrganizationCreationRule{Cookie: true, CSRF: true, Fresh: true, Success: 201}
}

var fieldName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func canonical(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if !fieldName.MatchString(key) || !canonical(item) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !canonical(item) {
				return false
			}
		}
	case nil:
		return false
	}
	return true
}
func mapped(err error) error {
	var setup *d.SetupError
	if errors.As(err, &setup) {
		keys := map[d.Missing]string{d.MissingName: "name", d.MissingTimeZone: "time_zone", d.MissingUnit: "active_unit", d.MissingOwner: "active_owner", d.MissingTerms: "terms"}
		fields := make([]string, 0, len(setup.Missing()))
		for _, missing := range setup.Missing() {
			fields = append(fields, keys[missing])
		}
		return orgapi.IncompleteSetup(fields)
	}
	var denied *permission.PermissionDenial
	if errors.As(err, &denied) {
		return orgapi.MissingPermission(string(denied.Permission()))
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrSurfaceDisabled, orgapi.NotFound}, {app.ErrUnauthenticated, orgapi.Unauthenticated},
		{kernel.ErrPendingOrgLimit, orgapi.RateLimited}, {kernel.ErrOwnerRequiresPersonal, orgapi.OwnerRequiresPersonal},
		{app.ErrOperationReused, orgapi.OperationReused}, {d.ErrConflict, orgapi.Conflict},
		{d.ErrInvalid, orgapi.ValidationFailed}, {d.ErrTerms, orgapi.ValidationFailed}, {d.ErrSetup, orgapi.OrgSetupIncomplete},
		{d.ErrTransition, orgapi.Conflict}, {app.ErrNotFound, orgapi.NotFound}, {capability.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}
func (e creationEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("ETag")
	if !e.service.Enabled {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" || r.URL.ForceQuery {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	raw, err := httpapi.ReadBody(r, company.MaxBodyBytes)
	var input creationRequest
	var fields map[string]any
	if err != nil || httpapi.DecodeJSON(raw, &input) != nil || json.Unmarshal(raw, &fields) != nil || !canonical(fields) || input.SchemaVersion != 1 {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	// Like the guarded gateway: transport syntax first, then the company
	// proof, and only then the body's meaning, so an unauthenticated or
	// cross-site caller never learns validation outcomes.
	e.company.RequireFreshSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		row, ok := company.SessionFromContext(r.Context())
		if !ok {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
		command, err := d.NewCreation(d.CreationInput{OperationID: input.OperationID, Profile: d.Profile{Name: input.Name, TimeZone: input.TimeZone, Kinds: input.Kinds}, UnitName: input.UnitName, Terms: d.TermsAcceptance{Version: input.Terms.Version, Digest: input.Terms.Digest, Accepted: input.Terms.Accepted}})
		if err != nil {
			orgapi.WriteError(w, r, mapped(err))
			return
		}
		if e.admit == nil {
			orgapi.WriteError(w, r, orgapi.Unavailable)
			return
		}
		if err := e.admit(r.Context(), row.AccountID); err != nil {
			orgapi.WriteError(w, r, err)
			return
		}
		result, replay, err := e.service.Create(r.Context(), app.CreationActor{AccountID: row.AccountID, SessionID: row.ID}, command)
		if err != nil {
			orgapi.WriteError(w, r, mapped(err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"organization_id": result.OrgID.String(), "unit_id": result.UnitID, "employee_id": result.EmployeeID, "membership_id": result.MembershipID, "assignment_id": result.AssignmentID, "state": result.State, "revision": result.Revision, "data_mode": result.DataMode, "replayed": replay})
	})).ServeHTTP(w, r)
}

// RootFacts resolves only the selected root through the ordinary scoped port.
type RootFacts struct{ Roots app.Repository }

func (p RootFacts) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permission.ResourceFacts, error) {
	if p.Roots == nil {
		return permission.ResourceFacts{}, access.ErrUnavailable
	}
	if _, err := p.Roots.Get(ctx, scope); err != nil {
		if errors.Is(err, app.ErrNotFound) {
			return permission.ResourceFacts{}, access.ErrNotFound
		}
		return permission.ResourceFacts{}, access.ErrUnavailable
	}
	return permission.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

// SetupRecords is confined to configuration on this organization root. Actual
// units, owners, terms and the current role/capability are rechecked in the write.
type SetupRecords struct{}

func (SetupRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permission.ResourceFacts, rules []permission.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || policy != ActivationPolicy() || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	return nil
}
func ActivationPolicy() access.Policy {
	return access.Policy{Permission: "org.settings.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}
func ActivationRule() orgapi.Rule {
	return orgapi.Rule{Permission: "org.settings.manage", Write: true, Fresh: true, RequireRevision: true, CompanyOnly: true, Success: 200}
}
func Register(router chi.Router, service app.Onboarding, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots app.Repository, admit CreationAdmission) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || service.NewID == nil || cookies.Service == nil || !cookies.Service.Enabled || roots == nil || admit == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := companyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	if err := guarded.Register(owned, ActivationPath, gateway, ActivationRule(), authorization, ActivationPolicy(), RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input activationRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		result, replay, err := service.Activate(ctx, p.Scope, actor, decision, input.OperationID, *input.Revision)
		if err != nil {
			return nil, mapped(err)
		}
		return map[string]any{"organization_id": result.OrgID.String(), "state": result.State, "revision": result.Revision, "data_mode": result.DataMode, "replayed": replay}, nil
	}); err != nil {
		return err
	}
	router.Method(http.MethodPost, CreationPath, creationEndpoint{service, cookies, admit})
	return nil
}
