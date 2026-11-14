package http

import (
	"context"
	"encoding/json"
	"errors"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
	"mime"
	"net/http"
)

const OffboardingReadPath = MembershipAccessPrefix + "/offboarding/read"
const OffboardingCompletePath = MembershipAccessPrefix + "/offboarding/complete"
const SelfLeaveReadPath = MembershipAccessPrefix + "/leave/read"
const SelfLeavePath = MembershipAccessPrefix + "/leave"

type offboardingRequest struct {
	companyRequest
	SchemaVersion     int      `json:"schema_version"`
	OperationID       string   `json:"operation_id"`
	Revision          *int64   `json:"revision"`
	EmployeeRevision  int64    `json:"employee_revision"`
	Reason            string   `json:"reason"`
	HandoverDigest    string   `json:"handover_digest"`
	Flags             []string `json:"flags"`
	TerminateEmployee bool     `json:"terminate_employee"`
}

func (v offboardingRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v *offboardingRequest) UnmarshalJSON(raw []byte) error {
	type plain offboardingRequest
	return decodeRedemption(raw, (*plain)(v), "schema_version", "operation_id", "revision", "employee_revision", "reason", "handover_digest", "flags", "terminate_employee")
}
func (v offboardingRequest) command(id string, self bool) app.OffboardingCommand {
	return app.OffboardingCommand{OperationID: v.OperationID, MembershipID: id, ExpectedRevision: *v.Revision, EmployeeRevision: v.EmployeeRevision, Reason: v.Reason, HandoverDigest: v.HandoverDigest, Flags: v.Flags, TerminateEmployee: v.TerminateEmployee, Self: self}
}
func offboardingMapped(e error) error {
	if errors.Is(e, d.ErrOpenResponsibilities) {
		return orgapi.OpenResponsibilities
	}
	if errors.Is(e, app.ErrManagedRestricted) {
		return orgapi.ManagedAccountRestricted
	}
	return membershipAccessMapped(e)
}
func offboardingView(v app.OffboardingView) map[string]any {
	out := membershipAccessView(app.MembershipAccessResult{State: v.State}, false)
	delete(out, "observed_at")
	out["employee_state"] = v.EmployeeState
	out["employee_revision"] = v.EmployeeRevision
	out["handover_digest"] = v.Digest
	out["items"] = v.Items
	out["handover_state"] = "NONE"
	if len(v.Items) > 0 {
		out["handover_state"] = "REVIEW_REQUIRED"
		if v.State.State == d.MembershipEnded {
			out["handover_state"] = "PENDING_MANAGER"
		}
	}
	out["scope_version"] = d.OffboardingScopeVersion
	out["pending_sources"] = []string{"MANAGED_ACCESS", "DEVICE_ENROLMENTS", "DELEGATIONS", "OPERATIONAL_WORK", "LATE_INTAKE"}
	return out
}
func offboardingAck(v app.OffboardingResult) map[string]any {
	out := offboardingView(v.View)
	out["operation_id"] = v.OperationID
	out["reason"] = v.Reason
	out["occurred_at"] = v.OccurredAt
	out["replayed"] = v.Replayed
	out["notice_state"] = "PENDING_NOTIFICATION_CORE"
	out["handover_state"] = "PENDING_MANAGER"
	if len(v.View.Items) == 0 {
		out["handover_state"] = "NONE"
	}
	return out
}

type selfLeaveEndpoint struct {
	repository app.OffboardingRepository
	cookies    company.Handler
	action     string
	admission  orgapi.WriteAdmission
}

func (e selfLeaveEndpoint) SelfMembershipRule() orgapi.SelfMembershipRule {
	return orgapi.SelfMembershipRule{Action: e.action, Cookie: true, CSRF: true, Fresh: true, Binding: true, Success: 200}
}
func (e selfLeaveEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Del("ETag")
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" || r.URL.ForceQuery {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	raw, err := httpapi.ReadBody(r, company.MaxBodyBytes)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	var input offboardingRequest
	if e.action == "read" {
		var v membershipAccessReadRequest
		err = httpapi.DecodeJSON(raw, &v)
	} else {
		err = httpapi.DecodeJSON(raw, &input)
		if err == nil && (input.SchemaVersion != 1 || input.Revision == nil) {
			err = orgapi.Malformed
		}
	}
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	e.cookies.RequireFreshSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := company.SessionFromContext(r.Context())
		if !ok {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
		a := app.EmployeeActor{AccountID: session.AccountID, SessionID: session.ID}
		rawOrg := chi.URLParam(r, "org_id")
		id, err := kernel.ParseOrgID(rawOrg)
		if err != nil || id.String() != rawOrg {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		scope, err := kernel.NewOrgScope(id)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		target, err := membershipAccessID(r.Context())
		if err != nil {
			orgapi.WriteError(w, r, err)
			return
		}
		view, err := e.repository.ReadOffboarding(r.Context(), scope, a, "", target, true)
		if err != nil {
			orgapi.WriteError(w, r, offboardingMapped(err))
			return
		}
		var result any = offboardingView(view)
		if e.action == "leave" {
			c := input.command(target, true)
			if err := c.Validate(); err != nil {
				orgapi.WriteError(w, r, offboardingMapped(err))
				return
			}
			rule := membershipAccessRule(app.MembershipSuspend)
			if err := e.admission.Check(r.Context(), orgapi.Principal{Actor: orgapi.Actor{AccountID: a.AccountID, Channel: "web"}, Scope: scope, MembershipID: target}, rule); err != nil {
				orgapi.WriteError(w, r, err)
				return
			}
			ack, err := e.repository.CompleteOffboarding(r.Context(), scope, a, "", c)
			if err != nil {
				orgapi.WriteError(w, r, offboardingMapped(err))
				return
			}
			result = offboardingAck(ack)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})).ServeHTTP(w, r)
}
func RegisterOffboarding(router chi.Router, service app.OffboardingService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots org.Repository) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || roots == nil || gateway.WriteAdmission == nil || gateway.WriteAdmission.Validate(membershipAccessRule(app.MembershipSuspend)) != nil {
		return app.ErrRegisterUnavailable
	}
	owned, g, e := orgweb.CompanyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	facts := orgweb.RootFacts{Roots: roots}
	if e := guarded.Register(owned, OffboardingReadPath, g, membershipAccessRule(app.MembershipAccessRead), authorization, app.OffboardingPolicy(false), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ membershipAccessReadRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := membershipAccessID(ctx)
		if e != nil {
			return nil, e
		}
		v, e := service.Read(ctx, p.Scope, a, decision, id)
		if e != nil {
			return nil, offboardingMapped(e)
		}
		return offboardingView(v), nil
	}); e != nil {
		return e
	}
	if e := guarded.Register(owned, OffboardingCompletePath, g, membershipAccessRule(app.MembershipSuspend), authorization, app.OffboardingPolicy(true), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input offboardingRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := membershipAccessID(ctx)
		if e != nil {
			return nil, e
		}
		v, e := service.Complete(ctx, p.Scope, a, decision, input.command(id, false))
		if e != nil {
			return nil, offboardingMapped(e)
		}
		return offboardingAck(v), nil
	}); e != nil {
		return e
	}
	router.Method(http.MethodPost, SelfLeaveReadPath, selfLeaveEndpoint{service.Repository, cookies, "read", gateway.WriteAdmission})
	router.Method(http.MethodPost, SelfLeavePath, selfLeaveEndpoint{service.Repository, cookies, "leave", gateway.WriteAdmission})
	return nil
}
