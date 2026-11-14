package http

import (
	"encoding/json"
	"mime"
	"net/http"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const OwnedSetupsPath = "/v1/org-access/organizations/setups/list"

type ownedSetupsEndpoint struct {
	source  app.CreatedSetupReader
	cookies company.Handler
}

func (ownedSetupsEndpoint) OwnedSetupsRule() orgapi.OwnedSetupsRule {
	return orgapi.OwnedSetupsRule{Cookie: true, CSRF: true, Binding: true, Success: 200}
}
func (e ownedSetupsEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	var input struct {
		Limit      *int   `json:"limit"`
		AfterOrgID string `json:"after_org_id"`
	}
	if httpapi.DecodeJSON(raw, &input) != nil {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	limit := 50
	if input.Limit != nil {
		limit = *input.Limit
	}
	id, parseErr := kernel.ParseOrgID(input.AfterOrgID)
	if limit < 1 || limit > 100 || input.AfterOrgID != "" && (parseErr != nil || id.String() != input.AfterOrgID) {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	e.cookies.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proof, ok := company.SessionFromContext(r.Context())
		if !ok {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
		page, err := e.source.ListCreatedSetups(r.Context(), app.CompanyActor{AccountID: proof.AccountID, SessionID: proof.ID}, limit, input.AfterOrgID)
		if err != nil {
			orgapi.WriteError(w, r, mapped(err))
			return
		}
		items := make([]map[string]any, 0, len(page.Setups))
		for _, item := range page.Setups {
			items = append(items, map[string]any{"id": item.ID, "name": item.Name, "membership_id": item.MembershipID, "state": item.State, "root_revision": item.RootRevision})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"account_id": page.AccountID, "setups": items, "next_after_org_id": page.NextAfterOrgID})
	})).ServeHTTP(w, r)
}
func RegisterCreatedSetups(router chi.Router, enabled bool, source app.CreatedSetupReader, cookies company.Handler) error {
	if !enabled {
		return nil
	}
	if source == nil || cookies.Service == nil || !cookies.Service.Enabled {
		return app.ErrUnavailable
	}
	router.Method(http.MethodPost, OwnedSetupsPath, ownedSetupsEndpoint{source, cookies})
	return nil
}
