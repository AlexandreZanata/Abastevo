package http

import (
	"encoding/json"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
	"mime"
	"net/http"
	"time"
)

const AccountContextPrefix = "/v1/org-access/contexts/"

type accountContextEndpoint struct {
	source  app.AccountContextReader
	cookies company.Handler
	action  string
}

func (e accountContextEndpoint) AccountContextRule() orgapi.AccountContextRule {
	return orgapi.AccountContextRule{Action: e.action, Cookie: true, CSRF: true, Binding: true, Success: 200}
}
func contextMetadata(v app.AccountContext) map[string]any {
	return map[string]any{"id": v.ID, "name": v.Name, "membership_id": v.MembershipID}
}
func (e accountContextEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Del("ETag")
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" || r.URL.ForceQuery {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(15 * time.Second))
	raw, err := httpapi.ReadBody(r, company.MaxBodyBytes)
	var fields map[string]any
	if err != nil || json.Unmarshal(raw, &fields) != nil || fields == nil || !canonical(fields) {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	var list struct {
		privateRequest
		Limit      *int   `json:"limit"`
		AfterOrgID string `json:"after_org_id"`
	}
	var read struct {
		privateRequest
		OrganizationID string `json:"organization_id"`
		MembershipID   string `json:"membership_id"`
	}
	if e.action == "list" {
		err = httpapi.DecodeJSON(raw, &list)
	} else {
		err = httpapi.DecodeJSON(raw, &read)
	}
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	e.cookies.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := company.SessionFromContext(r.Context())
		if !ok {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
		actor := app.CompanyActor{AccountID: session.AccountID, SessionID: session.ID}
		var response map[string]any
		if e.action == "list" {
			limit := 50
			if list.Limit != nil {
				limit = *list.Limit
			}
			if limit < 1 || limit > 100 {
				orgapi.WriteError(w, r, orgapi.Malformed)
				return
			}
			if list.AfterOrgID != "" {
				id, err := kernel.ParseOrgID(list.AfterOrgID)
				if err != nil || id.String() != list.AfterOrgID {
					orgapi.WriteError(w, r, orgapi.Malformed)
					return
				}
			}
			page, err := e.source.ListAccountContexts(r.Context(), actor, limit, list.AfterOrgID)
			if err != nil {
				orgapi.WriteError(w, r, mapped(err))
				return
			}
			organizations := make([]map[string]any, 0, len(page.Organizations))
			for _, v := range page.Organizations {
				organizations = append(organizations, contextMetadata(v))
			}
			response = map[string]any{"account_id": session.AccountID, "organizations": organizations, "next_after_org_id": page.NextAfterOrgID}
		} else {
			org, err := kernel.ParseOrgID(read.OrganizationID)
			if err != nil || org.String() != read.OrganizationID {
				orgapi.WriteError(w, r, orgapi.NotFound)
				return
			}
			member, err := kernel.ParseOrgID(read.MembershipID)
			if err != nil || member.String() != read.MembershipID {
				orgapi.WriteError(w, r, orgapi.NotFound)
				return
			}
			scope, err := kernel.NewOrgScope(org)
			if err != nil {
				orgapi.WriteError(w, r, orgapi.NotFound)
				return
			}
			v, err := e.source.ReadAccountContext(r.Context(), actor, scope, read.MembershipID)
			if err != nil {
				orgapi.WriteError(w, r, mapped(err))
				return
			}
			response = map[string]any{"account_id": session.AccountID, "organization": contextMetadata(v.Organization), "permissions": v.Permissions, "invitation_grants": v.InvitationGrants, "invitation_validity_days": v.InvitationValidityDays}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})).ServeHTTP(w, r)
}
func RegisterAccountContexts(router chi.Router, enabled bool, source app.AccountContextReader, cookies company.Handler) error {
	if !enabled {
		return nil
	}
	if source == nil {
		return app.ErrUnavailable
	}
	for _, action := range []string{"list", "read"} {
		router.Method(http.MethodPost, AccountContextPrefix+action, accountContextEndpoint{source, cookies, action})
	}
	return nil
}
