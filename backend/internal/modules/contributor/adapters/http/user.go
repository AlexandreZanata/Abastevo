package http

import (
	"context"
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	"github.com/go-chi/chi/v5"
)

type UserHandler struct {
	Service  *application.Service
	Sessions func(context.Context, string, string) (string, error)
}

func (h UserHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/contributor/claims", h.createClaim)
	r.Post("/v1/contributor/claims/read", h.readClaims)
	r.Post("/v1/contributor/claims/{id}/proof", h.claimProof)
	r.Post("/v1/contributor/claims/{id}/withdraw", h.withdrawClaim)
	r.Post("/v1/contributor/profile/read", h.readProfile)
	r.Post("/v1/contributor/profile", h.upsertProfile)
}

type sessionDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func (h UserHandler) session(w http.ResponseWriter, r *http.Request, dto sessionDTO) (string, bool) {
	accountID, err := principal(r.Context(), h.Sessions, dto.FamilyID, dto.AccessToken)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "contributor.auth-required")
		return "", false
	}
	return accountID, true
}

func (h UserHandler) createClaim(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
		GitHubLogin string `json:"github_login"`
		OperationID string `json:"operation_id"`
	}
	if err := readJSON(r, &dto); err != nil {
		writeServiceError(w, r, domain.ErrInvalid)
		return
	}
	accountID, ok := h.session(w, r, sessionDTO{FamilyID: dto.FamilyID, AccessToken: dto.AccessToken})
	if !ok {
		return
	}
	claim, err := h.Service.CreateClaim(r.Context(), accountID, dto.GitHubLogin, dto.OperationID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"claim_id": claim.ID, "code": claim.Code})
}

func (h UserHandler) readClaims(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := readJSON(r, &dto); err != nil {
		writeServiceError(w, r, domain.ErrInvalid)
		return
	}
	accountID, ok := h.session(w, r, dto)
	if !ok {
		return
	}
	claims, err := h.Service.ListOwnClaims(r.Context(), accountID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(claims))
	for _, claim := range claims {
		items = append(items, viewClaim(claim, true))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h UserHandler) claimProof(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
		GistID      string `json:"gist_id"`
	}
	if err := readJSON(r, &dto); err != nil {
		writeServiceError(w, r, domain.ErrInvalid)
		return
	}
	accountID, ok := h.session(w, r, sessionDTO{FamilyID: dto.FamilyID, AccessToken: dto.AccessToken})
	if !ok {
		return
	}
	claim, err := h.Service.SetClaimProof(r.Context(), accountID, chi.URLParam(r, "id"), dto.GistID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewClaim(claim, false))
}

func (h UserHandler) withdrawClaim(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := readJSON(r, &dto); err != nil {
		writeServiceError(w, r, domain.ErrInvalid)
		return
	}
	accountID, ok := h.session(w, r, dto)
	if !ok {
		return
	}
	claim, err := h.Service.WithdrawClaim(r.Context(), accountID, chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, viewClaim(claim, false))
}

func (h UserHandler) readProfile(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := readJSON(r, &dto); err != nil {
		writeServiceError(w, r, domain.ErrInvalid)
		return
	}
	accountID, ok := h.session(w, r, dto)
	if !ok {
		return
	}
	result, err := h.Service.ReadProfile(r.Context(), accountID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response := map[string]any{"roles": result.Roles}
	if result.Found {
		response["profile"] = viewProfile(result.Profile, result.Links)
	} else {
		response["profile"] = nil
	}
	writeJSON(w, http.StatusOK, response)
}

func (h UserHandler) upsertProfile(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
		DisplayName string `json:"display_name"`
		Links       []struct {
			Platform string `json:"platform"`
			Handle   string `json:"handle"`
		} `json:"links"`
		Visibility        string `json:"visibility"`
		PublicConsent     bool   `json:"public_consent"`
		ShowRolesPublicly bool   `json:"show_roles_publicly"`
	}
	if err := readJSON(r, &dto); err != nil {
		writeServiceError(w, r, domain.ErrInvalid)
		return
	}
	accountID, ok := h.session(w, r, sessionDTO{FamilyID: dto.FamilyID, AccessToken: dto.AccessToken})
	if !ok {
		return
	}
	links := make([]domain.Link, 0, len(dto.Links))
	for _, link := range dto.Links {
		links = append(links, domain.Link{Platform: link.Platform, Handle: link.Handle})
	}
	result, err := h.Service.UpsertProfile(r.Context(), accountID, dto.DisplayName, links, dto.Visibility, dto.PublicConsent, dto.ShowRolesPublicly)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": viewProfile(result.Profile, result.Links), "roles": result.Roles})
}
