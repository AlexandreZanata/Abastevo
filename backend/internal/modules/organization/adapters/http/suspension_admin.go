package http

import (
	"errors"
	"net/http"
	"strings"

	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	admin "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const SuspensionAdminPrefix = "/v1/admin/organizations/{org_id}"
const SuspensionAdminPath = SuspensionAdminPrefix + "/suspend"
const ReactivationAdminPath = SuspensionAdminPrefix + "/reactivate"

type SuspensionAdmin struct {
	Service app.SuspensionService
	Admin   *adminhttp.Handler
}
type suspensionAdminRequest struct {
	privateRequest
	SchemaVersion int      `json:"schema_version"`
	Revision      int64    `json:"revision"`
	Category      d.Reason `json:"reason_category"`
	Reason        string   `json:"reason"`
}

func (v *suspensionAdminRequest) UnmarshalJSON(raw []byte) error {
	type plain suspensionAdminRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = suspensionAdminRequest(input)
	return nil
}
func (h SuspensionAdmin) RegisterRoutes(r chi.Router) {
	for _, entry := range []struct {
		path   string
		action app.SuspensionAction
	}{{SuspensionAdminPath, app.SuspendOrganization}, {ReactivationAdminPath, app.ReactivateOrganization}} {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !h.Service.Enabled || h.Admin == nil {
				http.NotFound(w, r)
				return
			}
			session, ok := adminhttp.SessionFromContext(r.Context())
			if !ok {
				suspensionAdminError(w, r, admin.ErrUnauthorized)
				return
			}
			if len(r.Header.Values("Origin")) != 1 || len(r.Header.Values("X-Admin-CSRF")) != 1 || len(r.Header.Values("Idempotency-Key")) > 1 {
				suspensionAdminError(w, r, admin.ErrCSRF)
				return
			}
			cookies := 0
			for _, cookie := range r.Cookies() {
				if cookie.Name == "__Host-abv_admin" {
					cookies++
				}
			}
			if cookies != 1 || len(r.Header.Values("Authorization")) != 0 {
				suspensionAdminError(w, r, admin.ErrUnauthorized)
				return
			}
			if len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" || r.URL.ForceQuery {
				suspensionAdminError(w, r, admin.ErrInvalid)
				return
			}
			rawID := chi.URLParam(r, "org_id")
			id, e := kernel.ParseOrgID(rawID)
			if e != nil || id.String() != rawID {
				suspensionAdminError(w, r, d.ErrInvalid)
				return
			}
			scope, e := kernel.NewOrgScope(id)
			if e != nil {
				suspensionAdminError(w, r, d.ErrInvalid)
				return
			}
			var input suspensionAdminRequest
			mutation, ok := h.Admin.PrepareMutation(w, r, session, entry.action.AdminAction(), "organization", rawID, &input)
			if !ok {
				return
			}
			mutation.Reason = strings.TrimSpace(input.Reason)
			if input.SchemaVersion != 1 || !admin.ValidReason(mutation.Reason) {
				suspensionAdminError(w, r, d.ErrInvalid)
				return
			}
			result, e := h.Service.Change(r.Context(), scope, mutation, app.SuspensionCommand{Action: entry.action, ExpectedRevision: input.Revision, Category: input.Category})
			if e != nil {
				suspensionAdminError(w, r, e)
				return
			}
			w.Header().Del("ETag")
			h.Admin.FinishMutation(w, r, result, nil, false)
		})
		r.Method(http.MethodPost, entry.path, suspensionAdminEndpoint{Handler: handler, action: entry.action.AdminAction()})
	}
}
func suspensionAdminError(w http.ResponseWriter, r *http.Request, e error) {
	status, code := 503, "organization.unavailable"
	switch {
	case errors.Is(e, admin.ErrUnauthorized), errors.Is(e, admin.ErrDenied):
		status, code = 401, "admin.denied"
	case errors.Is(e, admin.ErrCSRF):
		status, code = 403, "admin.csrf"
	case errors.Is(e, admin.ErrInvalid):
		status, code = 400, "admin.invalid"
	case errors.Is(e, admin.ErrConflict):
		status, code = 409, "admin.conflict"
	case errors.Is(e, d.ErrConflict), errors.Is(e, d.ErrTransition):
		status, code = 409, "organization.conflict"
	case errors.Is(e, d.ErrInvalid):
		status, code = 422, "organization.invalid"
	case errors.Is(e, app.ErrNotFound), errors.Is(e, d.ErrClosed), errors.Is(e, app.ErrSurfaceDisabled):
		status, code = 404, "organization.not-found"
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("ETag")
	httpapi.WriteError(w, r, status, code, "Organization lifecycle request refused", nil)
}

type suspensionAdminEndpoint struct {
	http.Handler
	action string
}

func (e suspensionAdminEndpoint) AdminOrganizationRule() orgapi.AdminOrganizationRule {
	return orgapi.AdminOrganizationRule{Action: e.action, Cookie: true, CSRF: true, Fresh: true, RequireRevision: true, RequireReason: true, Success: 200}
}
