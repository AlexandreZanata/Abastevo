package http

import (
	"context"
	"encoding/json"
	"errors"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"

	pg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
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

const PersonalInvitationPrefix = "/v1/org-access/invitations/"
const AdmissionReadPath = InvitationPrefix + "/{invitation_id}/acceptance/read"
const AdmissionConfirmPath = InvitationPrefix + "/{invitation_id}/confirm"
const AdmissionRejectPath = InvitationPrefix + "/{invitation_id}/reject"

type UnknownInvitationAdmission func(context.Context, app.EmployeeActor, string) error
type personalInvitationEndpoint struct {
	service app.RedemptionRepository
	cookies company.Handler
	action  string
	admit   UnknownInvitationAdmission
}

func (e personalInvitationEndpoint) PersonalInvitationRule() orgapi.PersonalInvitationRule {
	return orgapi.PersonalInvitationRule{Action: e.action, Cookie: true, CSRF: true, Fresh: true, Success: 200}
}

type previewRequest struct {
	companyRequest
	Code string `json:"code"`
}
type acceptRequest struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Code          string `json:"code"`
	Consent       bool   `json:"consent"`
	ConsentDigest string `json:"consent_digest"`
}
type declineRequest struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Code          string `json:"code"`
}

func (v *previewRequest) UnmarshalJSON(raw []byte) error {
	type plain previewRequest
	return decodeRedemption(raw, (*plain)(v), "code")
}
func (v *acceptRequest) UnmarshalJSON(raw []byte) error {
	type plain acceptRequest
	return decodeRedemption(raw, (*plain)(v), "schema_version", "operation_id", "code", "consent", "consent_digest")
}
func (v *declineRequest) UnmarshalJSON(raw []byte) error {
	type plain declineRequest
	return decodeRedemption(raw, (*plain)(v), "schema_version", "operation_id", "code")
}
func decodeRedemption(raw []byte, v any, required ...string) error {
	if e := decodeInvitation(raw, v); e != nil {
		return e
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return orgapi.Malformed
	}
	for _, key := range required {
		if fields[key] == nil {
			return orgapi.Malformed
		}
	}
	return nil
}
func redemptionMapped(e error) error {
	switch {
	case errors.Is(e, pg.ErrUnknownInvitationLocator), errors.Is(e, app.ErrTokenInvalid):
		return orgapi.TokenInvalid
	case errors.Is(e, app.ErrAlreadyMember):
		return orgapi.AlreadyMember
	case errors.Is(e, app.ErrManagedRestricted):
		return orgapi.ManagedAccountRestricted
	case errors.Is(e, app.ErrConsentChanged):
		return orgapi.Conflict
	}
	return invitationMapped(e)
}
func (e personalInvitationEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	var c app.RedemptionCommand
	switch e.action {
	case "preview":
		var v previewRequest
		if httpapi.DecodeJSON(raw, &v) != nil {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}
		c = app.RedemptionCommand{Action: "PREVIEW", Code: v.Code}
	case "accept":
		var v acceptRequest
		if httpapi.DecodeJSON(raw, &v) != nil || v.SchemaVersion != 1 {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}
		c = app.RedemptionCommand{Action: "ACCEPT", OperationID: v.OperationID, Code: v.Code, Consent: v.Consent, ConsentDigest: v.ConsentDigest}
	case "decline":
		var v declineRequest
		if httpapi.DecodeJSON(raw, &v) != nil || v.SchemaVersion != 1 {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}
		c = app.RedemptionCommand{Action: "DECLINE", OperationID: v.OperationID, Code: v.Code}
	default:
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	e.cookies.RequireFreshSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		row, ok := company.SessionFromContext(r.Context())
		if !ok {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
		a := app.EmployeeActor{AccountID: row.AccountID, SessionID: row.ID}
		if err := c.Validate(); err != nil {
			orgapi.WriteError(w, r, redemptionMapped(err))
			return
		}
		result, err := e.service.Redeem(r.Context(), a, c)
		if errors.Is(err, pg.ErrUnknownInvitationLocator) {
			if limited := e.admit(r.Context(), a, r.RemoteAddr); limited != nil {
				orgapi.WriteError(w, r, limited)
				return
			}
		}
		if err != nil {
			orgapi.WriteError(w, r, redemptionMapped(err))
			return
		}
		var value map[string]any
		if c.Action == "PREVIEW" {
			value = map[string]any{"organization": map[string]any{"name": result.OrganizationName, "legal_entity_verified": result.Verified}, "grants": result.Grants, "confirmation_required": result.ConfirmationRequired, "consent_digest": result.ConsentDigest, "notice": map[string]any{"version": app.InvitationNoticeVersion, "text": app.InvitationNotice}, "data_mode": "SYNTHETIC"}
		} else {
			value = map[string]any{"organization_id": result.OrgID, "invitation_id": result.InvitationID, "membership_id": result.MembershipID, "state": result.State, "membership_state": result.MembershipState, "replayed": result.Replayed, "data_mode": "SYNTHETIC"}
			if result.State == "ACCEPTED" {
				value["accepted_at"] = result.AcceptedAt
				value["confirmation_deadline"] = result.ConfirmationDeadline
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	})).ServeHTTP(w, r)
}
func admissionResponse(v app.AdmissionView, replay bool) map[string]any {
	return map[string]any{"invitation_id": v.InvitationID, "membership_id": v.MembershipID, "account_alias": v.Alias, "account_erased": v.AccountErased, "state": v.State, "revision": v.Revision, "accepted_at": v.AcceptedAt, "confirmation_deadline": v.Deadline, "replayed": replay, "data_mode": "SYNTHETIC"}
}
func RegisterRedemption(router chi.Router, enabled bool, service app.RedemptionRepository, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots org.Repository, admit UnknownInvitationAdmission) error {
	if !enabled {
		return nil
	}
	if service == nil || admit == nil {
		return app.ErrRegisterUnavailable
	}
	owned, g, err := orgweb.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	facts := orgweb.RootFacts{Roots: roots}
	for _, route := range []struct {
		path, action string
		policy       app.InvitationAction
	}{{AdmissionConfirmPath, "CONFIRM", app.InvitationIssue}, {AdmissionRejectPath, "REJECT", app.InvitationRevoke}} {
		rule := invitationRule(route.policy)
		rule.RequireRevision = true
		if e := guarded.Register(owned, route.path, g, rule, authorization, app.InvitationPolicy(route.policy), facts, func(ctx context.Context, p orgapi.Principal, _ access.Decision, input invitationRevisionRequest) (any, error) {
			a, e := actor(ctx)
			if e != nil {
				return nil, e
			}
			id, e := invitationPathID(ctx)
			if e != nil {
				return nil, e
			}
			v, replay, e := service.ManageAdmission(ctx, p.Scope, a, p.MembershipID, id, route.action, *input.Revision, input.OperationID)
			if e != nil {
				return nil, redemptionMapped(e)
			}
			return admissionResponse(v, replay), nil
		}); e != nil {
			return e
		}
	}
	if e := guarded.Register(owned, AdmissionReadPath, g, invitationRule(app.InvitationRead), authorization, app.InvitationPolicy(app.InvitationRead), facts, func(ctx context.Context, p orgapi.Principal, _ access.Decision, _ invitationReadRequest) (any, error) {
		a, e := actor(ctx)
		if e != nil {
			return nil, e
		}
		id, e := invitationPathID(ctx)
		if e != nil {
			return nil, e
		}
		v, replay, e := service.ManageAdmission(ctx, p.Scope, a, p.MembershipID, id, "READ", 0, "")
		if e != nil {
			return nil, redemptionMapped(e)
		}
		return admissionResponse(v, replay), nil
	}); e != nil {
		return e
	}
	for _, action := range []string{"preview", "accept", "decline"} {
		router.Method(http.MethodPost, PersonalInvitationPrefix+action, personalInvitationEndpoint{service, cookies, action, admit})
	}
	return nil
}
