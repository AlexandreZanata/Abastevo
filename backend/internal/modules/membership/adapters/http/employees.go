// Package http binds the private employee register to normal company proof.
package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const EmployeePrefix = "/v1/orgs/{org_id}/employees"
const EmployeeCreatePath = EmployeePrefix + "/create"
const EmployeeChangePath = EmployeePrefix + "/{employee_id}/change"
const EmployeeCompletePath = EmployeePrefix + "/{employee_id}/complete"
const EmployeeReadPath = EmployeePrefix + "/{employee_id}/read"
const EmployeeListPath = EmployeePrefix + "/list"
const EmployeeMatchesPath = EmployeePrefix + "/matches"

type privateDTO struct{}

func (privateDTO) String() string                 { return "EmployeeRequest(redacted)" }
func (p privateDTO) Format(f fmt.State, _ rune)   { _, _ = fmt.Fprint(f, p.String()) }
func (p privateDTO) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }
func (p privateDTO) LogValue() slog.Value         { return slog.StringValue(p.String()) }

type companyRequest struct{ privateDTO }

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type profileFields struct {
	privateDTO
	DisplayName        string `json:"display_name"`
	RegistrationNumber string `json:"registration_number"`
	WorkEmail          string `json:"work_email"`
	WorkPhone          string `json:"work_phone"`
	JobTitle           string `json:"job_title"`
	HomeUnitID         string `json:"home_unit_id"`
}

func (v profileFields) domain() d.EmployeeProfile {
	return d.EmployeeProfile{DisplayName: v.DisplayName, RegistrationNumber: v.RegistrationNumber, WorkEmail: v.WorkEmail, WorkPhone: v.WorkPhone, JobTitle: v.JobTitle, HomeUnitID: v.HomeUnitID}
}

type createRequest struct {
	privateDTO
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	profileFields
	AllowPossibleDuplicate bool `json:"allow_possible_duplicate"`
}

func (v createRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type changeRequest struct {
	createRequest
	Revision *int64 `json:"revision"`
}

func (v changeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type completeRequest struct{ changeRequest }
type readRequest struct{ companyRequest }
type listRequest struct {
	companyRequest
	orgapi.Page
	State d.EmployeeState `json:"state,omitempty"`
}
type matchRequest struct {
	companyRequest
	orgapi.Page
	DisplayName       string `json:"display_name"`
	WorkEmail         string `json:"work_email"`
	WorkPhone         string `json:"work_phone"`
	ExcludeEmployeeID string `json:"exclude_employee_id"`
}

var fieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func decode(raw []byte, target any) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return orgapi.Malformed
	}
	for k, v := range fields {
		if !fieldPattern.MatchString(k) || string(v) == "null" {
			return orgapi.Malformed
		}
	}
	if httpapi.DecodeJSON(raw, target) != nil {
		return orgapi.Malformed
	}
	return nil
}
func (v *createRequest) UnmarshalJSON(raw []byte) error {
	type plain createRequest
	var p plain
	if e := decode(raw, &p); e != nil {
		return e
	}
	*v = createRequest(p)
	return nil
}
func (v *changeRequest) UnmarshalJSON(raw []byte) error {
	// Embed a field-only alias: an embedded createRequest's UnmarshalJSON would
	// otherwise consume the whole object and conceal revision/unknown fields.
	type flat struct {
		companyRequest
		SchemaVersion int    `json:"schema_version"`
		OperationID   string `json:"operation_id"`
		profileFields
		AllowPossibleDuplicate bool   `json:"allow_possible_duplicate"`
		Revision               *int64 `json:"revision"`
	}
	var p flat
	if e := decode(raw, &p); e != nil {
		return e
	}
	*v = changeRequest{createRequest: createRequest{SchemaVersion: p.SchemaVersion, OperationID: p.OperationID, profileFields: p.profileFields, AllowPossibleDuplicate: p.AllowPossibleDuplicate}, Revision: p.Revision}
	return nil
}
func (v *completeRequest) UnmarshalJSON(raw []byte) error {
	var p changeRequest
	if e := json.Unmarshal(raw, &p); e != nil {
		return e
	}
	v.changeRequest = p
	return nil
}
func (v *readRequest) UnmarshalJSON(raw []byte) error {
	type plain readRequest
	var p plain
	if e := decode(raw, &p); e != nil {
		return e
	}
	*v = readRequest(p)
	return nil
}
func (v *listRequest) UnmarshalJSON(raw []byte) error {
	type plain listRequest
	var p plain
	if e := decode(raw, &p); e != nil {
		return e
	}
	*v = listRequest(p)
	return nil
}
func (v *matchRequest) UnmarshalJSON(raw []byte) error {
	type plain matchRequest
	var p plain
	if e := decode(raw, &p); e != nil {
		return e
	}
	*v = matchRequest(p)
	return nil
}

type EmployeeRecords struct{}

func (EmployeeRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 || policy != app.EmployeePolicy(app.EmployeeRead) && policy != app.EmployeePolicy(app.EmployeeReview) && policy != app.EmployeePolicy(app.EmployeeManage) {
		return access.ErrUnavailable
	}
	return nil
}
func pathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "employee_id")
	id, e := kernel.ParseOrgID(raw)
	if e != nil || id.String() != raw {
		return "", access.ErrNotFound
	}
	return raw, nil
}

type employeeResource struct{ lookup app.EmployeeLookup }

func (r employeeResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if r.lookup == nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, e := pathID(ctx)
	if e != nil {
		return permissions.ResourceFacts{}, e
	}
	v, e := r.lookup.FindEmployee(ctx, scope, id)
	if errors.Is(e, app.ErrEmployeeNotFound) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	if e != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	if _, e = v.Snapshot(scope); e != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}
func actor(ctx context.Context) (app.EmployeeActor, error) {
	a, e := orgweb.CurrentCompanyActor(ctx)
	if e != nil {
		return app.EmployeeActor{}, e
	}
	return app.EmployeeActor{AccountID: a.AccountID, SessionID: a.SessionID}, nil
}
func mapped(e error) error {
	var denial *permissions.PermissionDenial
	if errors.As(e, &denial) {
		return orgapi.MissingPermission(string(denial.Permission()))
	}
	if errors.Is(e, app.ErrRegistrationDuplicate) {
		return orgapi.EmployeeRegistrationDuplicate()
	}
	for _, v := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrRegisterDisabled, orgapi.NotFound}, {app.ErrEmployeeNotFound, orgapi.NotFound}, {app.ErrEmployeeUnauthenticated, orgapi.Unauthenticated}, {app.ErrEmployeeOperationReused, orgapi.OperationReused}, {app.ErrDuplicateReview, orgapi.DuplicateReviewRequired},
		{d.ErrInvalid, orgapi.ValidationFailed}, {d.ErrScope, orgapi.NotFound}, {d.ErrConflict, orgapi.Conflict}, {d.ErrRevision, orgapi.Conflict}, {d.ErrTransition, orgapi.Conflict}, {d.ErrMerged, orgapi.Conflict},
		{access.ErrNoMembership, orgapi.NotFound}, {access.ErrOrgSuspended, orgapi.OrgSuspended}, {access.ErrOrgArchived, orgapi.OrgArchived}, {access.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(e, v.source) {
			return v.code
		}
	}
	return orgapi.Unavailable
}
func employeeAck(v app.EmployeeResult, replay bool) map[string]any {
	return map[string]any{"employee_id": v.EmployeeID, "state": v.State, "identity_revision": v.IdentityRevision, "profile_revision": v.ProfileRevision, "data_mode": "SYNTHETIC", "replayed": replay}
}
func employeeView(scope kernel.OrgScope, r d.EmployeeRecord) (map[string]any, error) {
	v, e := r.Snapshot(scope)
	if e != nil {
		return nil, orgapi.Unavailable
	}
	identity, e := v.Employee.Snapshot(scope)
	if e != nil {
		return nil, orgapi.Unavailable
	}
	return map[string]any{"id": identity.ID, "state": identity.State, "identity_revision": identity.Revision, "profile_revision": v.ProfileRevision, "profile_pending": v.ProfileRevision == 0, "display_name": v.Profile.DisplayName, "registration_number": v.Profile.RegistrationNumber, "work_email": v.Profile.WorkEmail, "work_phone": v.Profile.WorkPhone, "job_title": v.Profile.JobTitle, "home_unit_id": v.Profile.HomeUnitID, "merged_into": identity.MergedInto, "data_mode": "SYNTHETIC"}, nil
}
func employeeRule(action app.EmployeeAction, revision bool, status int) orgapi.Rule {
	p := app.EmployeePolicy(action)
	return orgapi.Rule{Permission: string(p.Permission), Write: p.Effect.Write(), Fresh: p.Fresh, RequireRevision: revision, CompanyOnly: true, Success: status}
}
func RegisterEmployees(router chi.Router, service app.EmployeeService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots org.Repository, lookup app.EmployeeLookup) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || service.NewID == nil || roots == nil || lookup == nil {
		return app.ErrRegisterUnavailable
	}
	owned, gateway, e := orgweb.CompanyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	resource := employeeResource{lookup}
	if e := guarded.Register(owned, EmployeeCreatePath, gateway, employeeRule(app.EmployeeManage, false, 201), authorization, app.EmployeePolicy(app.EmployeeManage), orgweb.RootFacts{Roots: roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input createRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		c, e := app.NewEmployeeWrite(app.EmployeeWriteInput{Kind: app.CreateEmployee, OperationID: input.OperationID, Profile: input.profileFields.domain(), AllowPossibleDuplicate: input.AllowPossibleDuplicate})
		if e != nil {
			return nil, mapped(e)
		}
		r, replay, e := service.Write(ctx, p.Scope, a, decision, c)
		if e != nil {
			return nil, mapped(e)
		}
		return employeeAck(r, replay), nil
	}); e != nil {
		return e
	}
	for _, v := range []struct {
		path string
		kind app.EmployeeWriteKind
	}{{EmployeeChangePath, app.ChangeEmployee}, {EmployeeCompletePath, app.CompleteEmployee}} {
		if e := guarded.Register(owned, v.path, gateway, employeeRule(app.EmployeeManage, true, 200), authorization, app.EmployeePolicy(app.EmployeeManage), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input changeRequest) (any, error) {
			a, e := actor(ctx)
			if e != nil {
				return nil, e
			}
			id, e := pathID(ctx)
			if e != nil {
				return nil, orgapi.NotFound
			}
			c, e := app.NewEmployeeWrite(app.EmployeeWriteInput{Kind: v.kind, OperationID: input.OperationID, EmployeeID: id, ExpectedRevision: *input.Revision, Profile: input.profileFields.domain(), AllowPossibleDuplicate: input.AllowPossibleDuplicate})
			if e != nil {
				return nil, mapped(e)
			}
			r, replay, e := service.Write(ctx, p.Scope, a, decision, c)
			if e != nil {
				return nil, mapped(e)
			}
			return employeeAck(r, replay), nil
		}); e != nil {
			return e
		}
	}
	if e := guarded.Register(owned, EmployeeReadPath, gateway, employeeRule(app.EmployeeRead, false, 200), authorization, app.EmployeePolicy(app.EmployeeRead), resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ readRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := pathID(ctx)
		if e != nil {
			return nil, orgapi.NotFound
		}
		r, e := service.Read(ctx, p.Scope, a, decision, id)
		if e != nil {
			return nil, mapped(e)
		}
		return employeeView(p.Scope, r)
	}); e != nil {
		return e
	}
	if e := guarded.Register(owned, EmployeeListPath, gateway, employeeRule(app.EmployeeRead, false, 200), authorization, app.EmployeePolicy(app.EmployeeRead), orgweb.RootFacts{Roots: roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input listRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		page, e := input.Page.Normalize()
		if e != nil {
			return nil, orgapi.Malformed
		}
		f := app.EmployeeFilter{Limit: page.Limit, State: input.State}
		if f.Validate() != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("employees.v1:%s:%d", input.State, page.Limit)
		f.AfterID, e = gateway.OpenCursor(p, EmployeeListPath, filters, page.Cursor)
		if e != nil {
			return nil, orgapi.Malformed
		}
		result, e := service.List(ctx, p.Scope, a, decision, f)
		if e != nil {
			return nil, mapped(e)
		}
		return employeePage(p, gateway, EmployeeListPath, filters, result)
	}); e != nil {
		return e
	}
	return guarded.Register(owned, EmployeeMatchesPath, gateway, employeeRule(app.EmployeeReview, false, 200), authorization, app.EmployeePolicy(app.EmployeeReview), orgweb.RootFacts{Roots: roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input matchRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		page, e := input.Page.Normalize()
		if e != nil {
			return nil, orgapi.Malformed
		}
		profile, e := d.CanonicalEmployeeProfile(d.EmployeeProfile{DisplayName: input.DisplayName, WorkEmail: input.WorkEmail, WorkPhone: input.WorkPhone})
		if e != nil {
			return nil, mapped(e)
		}
		f := app.EmployeeMatch{Profile: profile, ExcludeID: input.ExcludeEmployeeID, Limit: page.Limit}
		if f.Validate() != nil {
			return nil, orgapi.Malformed
		}
		raw, _ := json.Marshal([]any{1, profile.DisplayName, profile.WorkEmail, profile.WorkPhone, f.ExcludeID, page.Limit})
		filters := fmt.Sprintf("employee_matches.v1:%x", sha256.Sum256(raw))
		f.AfterID, e = gateway.OpenCursor(p, EmployeeMatchesPath, filters, page.Cursor)
		if e != nil {
			return nil, orgapi.Malformed
		}
		result, e := service.Matches(ctx, p.Scope, a, decision, f)
		if e != nil {
			return nil, mapped(e)
		}
		return employeePage(p, gateway, EmployeeMatchesPath, filters, result)
	})
}
func employeePage(p orgapi.Principal, gateway orgapi.Gateway, path, filters string, page app.EmployeePage) (map[string]any, error) {
	items := make([]map[string]any, 0, len(page.Employees))
	for _, r := range page.Employees {
		v, e := employeeView(p.Scope, r)
		if e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	next := ""
	var e error
	if page.NextID != "" {
		next, e = gateway.SealCursor(p, path, filters, page.NextID)
		if e != nil {
			return nil, orgapi.Unavailable
		}
	}
	return map[string]any{"items": items, "next_cursor": next}, nil
}
