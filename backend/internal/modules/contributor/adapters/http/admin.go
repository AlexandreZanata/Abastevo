package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type AdminHandler struct {
	Service *application.Service
	Admin   *adminhttp.Handler
}

func (h AdminHandler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/admin/github/contributors", h.githubContributors)
	r.Get("/v1/admin/contributor-claims", h.listClaims)
	r.Post("/v1/admin/contributor-claims/{id}/approve", h.approveClaim)
	r.Post("/v1/admin/contributor-claims/{id}/reject", h.rejectClaim)
	r.Post("/v1/admin/accounts/{id}/github-link", h.linkGitHub)
	r.Post("/v1/admin/contributor-profiles/{account_id}/hide", func(w http.ResponseWriter, r *http.Request) { h.setHidden(w, r, true) })
	r.Post("/v1/admin/contributor-profiles/{account_id}/unhide", func(w http.ResponseWriter, r *http.Request) { h.setHidden(w, r, false) })
}

func (h AdminHandler) githubContributors(w http.ResponseWriter, r *http.Request) {
	refresh := false
	if raw := strings.TrimSpace(r.URL.Query().Get("refresh")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "contributor.invalid")
			return
		}
		refresh = value
	}
	view, err := h.Service.AdminGitHubContributors(r.Context(), refresh)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(view.Logins))
	for _, item := range view.Logins {
		entry := map[string]any{"login": item.Login}
		if item.LinkedAccountID != "" {
			entry["linked_account_id"] = item.LinkedAccountID
		}
		if item.ClaimID != "" {
			entry["claim_id"] = item.ClaimID
			entry["claim_state"] = item.ClaimState
			entry["claim_proof_status"] = item.ClaimProofStatus
		}
		items = append(items, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repo": view.Repo, "fetched_at": view.FetchedAt.UTC(), "stale": view.Stale, "contributors": items,
	})
}

func (h AdminHandler) listClaims(w http.ResponseWriter, r *http.Request) {
	stateRaw := strings.TrimSpace(r.URL.Query().Get("state"))
	if stateRaw != "" && !domain.ValidClaimState(stateRaw) {
		writeError(w, r, http.StatusBadRequest, "contributor.invalid")
		return
	}
	limit, ok := parseLimit(r.URL.Query().Get("limit"), 20, 100)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "contributor.invalid")
		return
	}
	filter := domain.ClaimFilter{State: domain.ClaimState(stateRaw), Limit: limit + 1}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" {
		if len(h.Admin.CursorSecret) < 32 {
			writeError(w, r, http.StatusServiceUnavailable, "contributor.unavailable")
			return
		}
		position, err := httpapi.Open(h.Admin.CursorSecret, cursor, httpapi.FilterHash("contributor-claims-v1", stateRaw), time.Now())
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "contributor.invalid")
			return
		}
		stamp, id, ok := strings.Cut(position, ",")
		if !ok || !domain.ValidUUID(id) {
			writeError(w, r, http.StatusBadRequest, "contributor.invalid")
			return
		}
		created, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "contributor.invalid")
			return
		}
		filter.BeforeCreated, filter.BeforeID = created, id
	}
	claims, err := h.Service.AdminListClaims(r.Context(), filter)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	next := ""
	if len(claims) > limit {
		claims = claims[:limit]
		last := claims[len(claims)-1]
		next = httpapi.Seal(h.Admin.CursorSecret, httpapi.FilterHash("contributor-claims-v1", stateRaw), last.CreatedAt.UTC().Format(time.RFC3339Nano)+","+last.ID, time.Now())
	}
	items := make([]map[string]any, 0, len(claims))
	for _, claim := range claims {
		entry := viewClaim(claim, false)
		entry["account_id"] = claim.AccountID
		items = append(items, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

type approveDTO struct {
	Reason   string `json:"reason"`
	Override bool   `json:"override"`
}

func (h AdminHandler) approveClaim(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	claimID := chi.URLParam(r, "id")
	var dto approveDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, "contributor.claim.approve", "contributor_claim", claimID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	result, err := h.Service.ApproveClaim(r.Context(), m, claimID, dto.Override)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	body, err := json.Marshal(result)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "contributor.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusOK, Body: body}, nil, false)
}

func (h AdminHandler) rejectClaim(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	claimID := chi.URLParam(r, "id")
	var dto approveDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, "contributor.claim.reject", "contributor_claim", claimID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	claim, err := h.Service.RejectClaim(r.Context(), m, claimID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	body, err := json.Marshal(claim)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "contributor.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusOK, Body: body}, nil, false)
}

type linkDTO struct {
	Login    string `json:"login"`
	Reason   string `json:"reason"`
	Override bool   `json:"override"`
}

func (h AdminHandler) linkGitHub(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	accountID := chi.URLParam(r, "id")
	var dto linkDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, "contributor.github.link", "account", accountID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	login, err := h.Service.LinkGitHub(r.Context(), m, accountID, dto.Login, dto.Override)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	body, err := json.Marshal(map[string]string{"account_id": accountID, "github_login": login})
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "contributor.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusCreated, Body: body}, nil, false)
}

func (h AdminHandler) setHidden(w http.ResponseWriter, r *http.Request, hidden bool) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	accountID := chi.URLParam(r, "account_id")
	var dto approveDTO
	action := "contributor.profile.unhide"
	if hidden {
		action = "contributor.profile.hide"
	}
	m, ok := h.Admin.PrepareMutation(w, r, session, action, "contributor_profile", accountID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	profile, err := h.Service.HideProfile(r.Context(), m, accountID, hidden)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	body, err := json.Marshal(profile)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "contributor.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusOK, Body: body}, nil, false)
}
