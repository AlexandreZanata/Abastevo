package intake

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Handler serves the private suggestion routes with an injected service
// and a session validator. The validator resolves a live account
// session to its account ID; the composition root maps session failures
// onto intake errors so this package never imports other modules.
// Anonymous contributor proof alone is insufficient: every route here
// requires a live session. All responses carry no-store; missing and
// foreign records share one 404 (no ownership oracle).
type Handler struct {
	Service  application.IntakeService
	Sessions func(ctx context.Context, familyID, accessToken string) (string, error)
}

// RegisterRoutes mounts the additive intake paths. Private reads use
// POST with the session in the JSON body (never the URL), mirroring
// the account binding routes.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/stations/suggestions", h.submit)
	r.Post("/v1/stations/suggestions/mine", h.listMine)
	r.Post("/v1/stations/suggestions/{id}/status", h.status)
	r.Post("/v1/stations/suggestions/{id}/cancel", h.cancel)
}

type sessionDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

type proposalDTO struct {
	DisplayName      string  `json:"display_name"`
	MunicipalityCode string  `json:"municipality_code"`
	State            string  `json:"state"`
	CNPJ             string  `json:"cnpj_normalized"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	HasCoords        bool    `json:"has_coordinates"`
	EvidenceRef      string  `json:"evidence_ref"`
}

type submitDTO struct {
	sessionDTO
	ClientSubmissionID string      `json:"client_submission_id"`
	Proposal           proposalDTO `json:"proposal"`
}

func read(r *http.Request, dst any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return errors.New("JSON required")
	}
	raw, err := httpapi.ReadBody(r, 8<<10)
	if err != nil {
		return err
	}
	return httpapi.DecodeJSON(raw, dst)
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func writeSuggestion(w http.ResponseWriter, r *http.Request, s application.Suggestion, created bool) {
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeDoc(w, r, status, map[string]any{
		"id":                   s.ID,
		"client_submission_id": s.ClientSubmissionID,
		"state":                s.State,
		"created_at":           s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func writeDoc(w http.ResponseWriter, r *http.Request, status int, doc map[string]any) {
	raw, err := json.Marshal(doc)
	if err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusInternalServerError, "intake.unavailable", "intake service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, status, "no-store", raw)
}

func (h Handler) sessionAccount(w http.ResponseWriter, r *http.Request, dto sessionDTO) (string, bool) {
	accountID, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		noStore(w)
		if errors.Is(err, application.ErrAuthorForbidden) {
			httpapi.WriteError(w, r, http.StatusForbidden, "intake.author-forbidden", "account cannot write suggestions", nil)
		} else {
			httpapi.WriteError(w, r, http.StatusUnauthorized, "intake.session-invalid", "valid account session required", nil)
		}
		return "", false
	}
	return accountID, true
}

func (h Handler) submit(w http.ResponseWriter, r *http.Request) {
	var dto submitDTO
	if err := read(r, &dto); err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusBadRequest, "intake.bad-request", "invalid suggestion input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.sessionDTO)
	if !ok {
		return
	}
	suggestion, created, err := h.Service.Submit(r.Context(), accountID, dto.ClientSubmissionID, application.ProposalInput{
		DisplayName: dto.Proposal.DisplayName, MunicipalityCode: dto.Proposal.MunicipalityCode,
		State: dto.Proposal.State, CNPJ: dto.Proposal.CNPJ,
		Latitude: dto.Proposal.Latitude, Longitude: dto.Proposal.Longitude,
		HasCoords: dto.Proposal.HasCoords, EvidenceRef: dto.Proposal.EvidenceRef,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	writeSuggestion(w, r, suggestion, created)
}

func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusBadRequest, "intake.bad-request", "invalid suggestion input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto)
	if !ok {
		return
	}
	suggestion, err := h.Service.Owned(r.Context(), accountID, chi.URLParam(r, "id"))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeSuggestion(w, r, suggestion, false)
}

func (h Handler) listMine(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusBadRequest, "intake.bad-request", "invalid suggestion input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto)
	if !ok {
		return
	}
	list, err := h.Service.ListOwned(r.Context(), accountID, 20, 0)
	if err != nil {
		fail(w, r, err)
		return
	}
	items := make([]any, 0, len(list))
	for _, s := range list {
		items = append(items, map[string]any{
			"id": s.ID, "client_submission_id": s.ClientSubmissionID,
			"state": s.State, "created_at": s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	writeDoc(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) cancel(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusBadRequest, "intake.bad-request", "invalid suggestion input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto)
	if !ok {
		return
	}
	if err := h.Service.Cancel(r.Context(), accountID, chi.URLParam(r, "id")); err != nil {
		fail(w, r, err)
		return
	}
	writeDoc(w, r, http.StatusOK, map[string]any{"status": "cancelled"})
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	noStore(w)
	switch {
	case errors.Is(err, application.ErrSuggestionQuota):
		httpapi.WriteError(w, r, http.StatusTooManyRequests, "intake.quota-exceeded", "daily suggestion quota exhausted", nil)
	case errors.Is(err, application.ErrSuggestionConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "intake.idempotency-conflict", "same key with different proposal", nil)
	case errors.Is(err, application.ErrSuggestionNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "intake.not-found", "suggestion not found", nil)
	case errors.Is(err, application.ErrSuggestionClosed):
		httpapi.WriteError(w, r, http.StatusConflict, "intake.closed", "suggestion is no longer pending", nil)
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "intake.invalid", "invalid suggestion input", nil)
	}
}
