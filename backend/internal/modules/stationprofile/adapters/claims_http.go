package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// ClaimHandler serves the private claim routes with an injected
// service closure set and a session validator. The validator resolves
// a live account session to its account ID; the composition root maps
// session failures so this package never imports other modules.
// Anonymous contributor proof alone is insufficient on every route.
// All responses carry no-store; missing and foreign records share one
// 404 (no ownership oracle, no competing-case leakage).
type ClaimHandler struct {
	Ports    application.ClaimPorts
	Sessions func(ctx context.Context, familyID, accessToken string) (string, error)
}

// RegisterRoutes mounts the additive claim paths.
func (h ClaimHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/stations/{station_id}/claims", h.open)
	r.Post("/v1/profile/claims/mine", h.mine)
	r.Post("/v1/profile/claims/{id}/status", h.status)
	r.Post("/v1/profile/claims/{id}/reissue", h.reissue)
	r.Post("/v1/profile/claims/{id}/cancel", h.cancel)
}

type sessionDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

type openDTO struct {
	sessionDTO
	Role      string   `json:"role"`
	Scopes    []string `json:"scopes"`
	ClientKey string   `json:"client_key"`
}

func read(r *http.Request, dst any) error {
	return readLimit(r, dst, 8<<10)
}

func readLimit(r *http.Request, dst any, max int64) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return errors.New("JSON required")
	}
	raw, err := httpapi.ReadBody(r, max)
	if err != nil {
		return err
	}
	return httpapi.DecodeJSON(raw, dst)
}

func writeClaim(w http.ResponseWriter, r *http.Request, claim application.Claim, status int) {
	raw, err := json.Marshal(map[string]any{
		"id":                claim.ID,
		"station_id":        claim.StationID,
		"role":              claim.Role,
		"scopes":            claim.Scopes,
		"state":             claim.State,
		"version":           claim.Version,
		"declaration_state": claim.DeclarationState,
		"declaration_id":    claim.DeclarationID,
		"declaration":       claim.Declaration,
		"expires_at":        claim.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "claim.unavailable", "claim service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, status, "no-store", raw)
}

func (h ClaimHandler) sessionAccount(w http.ResponseWriter, r *http.Request, dto sessionDTO) (string, bool) {
	accountID, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "claim.session-invalid", "valid account session required", nil)
		return "", false
	}
	return accountID, true
}

func (h ClaimHandler) open(w http.ResponseWriter, r *http.Request) {
	var dto openDTO
	if err := read(r, &dto); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "claim.bad-request", "invalid claim input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.sessionDTO)
	if !ok {
		return
	}
	claim, created, err := application.OpenClaim(r.Context(), h.Ports, accountID, chi.URLParam(r, "station_id"), dto.Role, dto.Scopes, dto.ClientKey)
	if err != nil {
		failClaim(w, r, err)
		return
	}
	if created {
		writeClaim(w, r, claim, http.StatusCreated)
		return
	}
	writeClaim(w, r, claim, http.StatusOK)
}

func (h ClaimHandler) mine(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "claim.bad-request", "invalid claim input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto)
	if !ok {
		return
	}
	items, err := application.ListMine(r.Context(), h.Ports.Store, accountID)
	if err != nil {
		failClaim(w, r, err)
		return
	}
	docs := make([]any, 0, len(items))
	for _, item := range items {
		docs = append(docs, map[string]any{"id": item.ID, "state": item.State})
	}
	raw, err := json.Marshal(map[string]any{"items": docs})
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "claim.unavailable", "claim service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", raw)
}

func (h ClaimHandler) status(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "claim.bad-request", "invalid claim input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto)
	if !ok {
		return
	}
	claim, err := application.ClaimStatus(r.Context(), h.Ports.Store, accountID, chi.URLParam(r, "id"))
	if err != nil {
		failClaim(w, r, err)
		return
	}
	writeClaim(w, r, claim, http.StatusOK)
}

func (h ClaimHandler) reissue(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "claim.bad-request", "invalid claim input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto)
	if !ok {
		return
	}
	claim, err := application.ReissueDeclaration(r.Context(), h.Ports, accountID, chi.URLParam(r, "id"))
	if err != nil {
		failClaim(w, r, err)
		return
	}
	writeClaim(w, r, claim, http.StatusOK)
}

func (h ClaimHandler) cancel(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "claim.bad-request", "invalid claim input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto)
	if !ok {
		return
	}
	if err := application.CancelClaim(r.Context(), h.Ports.Store, accountID, chi.URLParam(r, "id")); err != nil {
		failClaim(w, r, err)
		return
	}
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", []byte(`{"status": "cancelled"}`))
}

func failClaim(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrClaimQuota):
		httpapi.WriteError(w, r, http.StatusTooManyRequests, "claim.quota-exceeded", "too many open claims", nil)
	case errors.Is(err, application.ErrClaimConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "claim.idempotency-conflict", "same key with different request", nil)
	case errors.Is(err, application.ErrClaimBusy):
		httpapi.WriteError(w, r, http.StatusConflict, "claim.busy-retry", "claim is being published, retry the same request", nil)
	case errors.Is(err, application.ErrClaimNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "claim.not-found", "claim not found", nil)
	case errors.Is(err, application.ErrClaimClosed):
		httpapi.WriteError(w, r, http.StatusConflict, "claim.closed", "claim is no longer open", nil)
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "claim.invalid", "invalid claim input", nil)
	}
}
