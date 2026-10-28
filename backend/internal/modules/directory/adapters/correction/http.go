package correction

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

// Handler serves the private correction routes. The session validator
// resolves a live account; anonymous contributor proof alone is insufficient.
// Every response is no-store and foreign owners share one 404.
type Handler struct {
	Service  application.CorrectionService
	Sessions func(ctx context.Context, familyID, accessToken string) (string, error)
}

func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/stations/{id}/corrections", h.submit)
	r.Post("/v1/stations/corrections/mine", h.listMine)
	r.Post("/v1/stations/corrections/{id}/status", h.status)
	r.Post("/v1/stations/corrections/{id}/cancel", h.cancel)
}

type sessionDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

type submitDTO struct {
	sessionDTO
	ClientSubmissionID string          `json:"client_submission_id"`
	Field              string          `json:"field"`
	ProposedValue      json.RawMessage `json:"proposed_value"`
	Note               string          `json:"note"`
	EvidenceRef        string          `json:"evidence_ref"`
	ProximityBucket    string          `json:"proximity_bucket"`
}

func read(r *http.Request, dst any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return errors.New("JSON required")
	}
	raw, err := httpapi.ReadBody(r, 16<<10)
	if err != nil {
		return err
	}
	return httpapi.DecodeJSON(raw, dst)
}

func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

func writeDoc(w http.ResponseWriter, r *http.Request, status int, doc any) {
	raw, err := json.Marshal(doc)
	if err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusInternalServerError, "correction.unavailable", "correction service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, status, "no-store", raw)
}

func (h Handler) sessionAccount(w http.ResponseWriter, r *http.Request, dto sessionDTO) (string, bool) {
	accountID, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		noStore(w)
		if errors.Is(err, application.ErrCorrectionForbidden) {
			httpapi.WriteError(w, r, http.StatusForbidden, "correction.author-forbidden", "account cannot write corrections", nil)
		} else {
			httpapi.WriteError(w, r, http.StatusUnauthorized, "correction.session-invalid", "valid account session required", nil)
		}
		return "", false
	}
	return accountID, true
}

func correctionDoc(c application.Correction) map[string]any {
	doc := map[string]any{
		"id": c.ID, "station_id": c.StationID, "field": c.Field,
		"status": c.Status, "client_submission_id": c.ClientSubmissionID,
		"created_at": c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if c.DecidedAt != nil {
		doc["decided_at"] = c.DecidedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	if len(c.ReasonCodes) > 0 {
		doc["reason_codes"] = c.ReasonCodes
	}
	return doc
}

func (h Handler) submit(w http.ResponseWriter, r *http.Request) {
	var dto submitDTO
	if err := read(r, &dto); err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusBadRequest, "correction.bad-request", "invalid correction input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.sessionDTO)
	if !ok {
		return
	}
	correction, created, err := h.Service.Submit(r.Context(), accountID, dto.ClientSubmissionID, application.CorrectionInput{
		StationID: chi.URLParam(r, "id"), Field: dto.Field, ProposedValue: dto.ProposedValue,
		Note: dto.Note, EvidenceRef: dto.EvidenceRef, ProximityBucket: dto.ProximityBucket,
		ClientSubmissionID: dto.ClientSubmissionID,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeDoc(w, r, status, correctionDoc(correction))
}

func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusBadRequest, "correction.bad-request", "invalid correction input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto)
	if !ok {
		return
	}
	correction, err := h.Service.Owned(r.Context(), accountID, chi.URLParam(r, "id"))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeDoc(w, r, http.StatusOK, correctionDoc(correction))
}

func (h Handler) listMine(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusBadRequest, "correction.bad-request", "invalid correction input", nil)
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
	items := make([]map[string]any, 0, len(list))
	for _, c := range list {
		items = append(items, correctionDoc(c))
	}
	writeDoc(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) cancel(w http.ResponseWriter, r *http.Request) {
	var dto sessionDTO
	if err := read(r, &dto); err != nil {
		noStore(w)
		httpapi.WriteError(w, r, http.StatusBadRequest, "correction.bad-request", "invalid correction input", nil)
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
	writeDoc(w, r, http.StatusOK, map[string]any{"status": application.CorrectionCancelled})
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	noStore(w)
	switch {
	case errors.Is(err, application.ErrCorrectionQuota):
		httpapi.WriteError(w, r, http.StatusTooManyRequests, "correction.quota-exceeded", "daily correction quota exhausted", nil)
	case errors.Is(err, application.ErrCorrectionConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "correction.idempotency-conflict", "same key with different correction", nil)
	case errors.Is(err, application.ErrCorrectionOpen):
		httpapi.WriteError(w, r, http.StatusConflict, "correction.open", "an open correction already exists for this field", nil)
	case errors.Is(err, application.ErrCorrectionNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "correction.not-found", "correction not found", nil)
	case errors.Is(err, application.ErrCorrectionClosed):
		httpapi.WriteError(w, r, http.StatusConflict, "correction.closed", "correction is no longer pending", nil)
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "correction.invalid", "invalid correction input", nil)
	}
}
