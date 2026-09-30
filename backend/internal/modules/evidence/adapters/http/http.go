package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Handler serves upload reservation and owner status. Authentication
// already happened upstream: Authenticate resolves the caller from the
// verified proof, so this package never sees keys or signatures.
type Handler struct {
	Authenticate func(r *http.Request) (application.Caller, error)
	Reserve      func(ctx context.Context, caller application.Caller, in application.Intent, body []byte) (application.Result, error)
	Complete     func(ctx context.Context, caller application.Caller, id string) (application.StatusView, error)
	Status       func(ctx context.Context, caller application.Caller, id string) (application.StatusView, error)
}

// RegisterRoutes mounts evidence endpoints under /v1.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/uploads", h.reserve)
	r.Get("/v1/uploads/{upload_id}", h.status)
	r.Post("/v1/uploads/{upload_id}/complete", h.complete)
}

func (h Handler) caller(w http.ResponseWriter, r *http.Request) (application.Caller, bool) {
	caller, err := h.Authenticate(r)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "evidence.auth-required", "authentication required", nil)
		return application.Caller{}, false
	}
	return caller, true
}

func writeAPIError(w http.ResponseWriter, r *http.Request, err error) {
	var denied *application.QuotaDeniedError
	if errors.As(err, &denied) {
		w.Header().Set("Retry-After", strconv.FormatInt(int64(denied.RetryAfter/time.Second), 10))
		httpapi.WriteError(w, r, http.StatusTooManyRequests, "evidence.quota-exceeded", "quota exhausted, retry later", nil)
		return
	}
	switch {
	case errors.Is(err, application.ErrUnauthorized):
		httpapi.WriteError(w, r, http.StatusUnauthorized, "evidence.auth-required", "authentication required", nil)
	case errors.Is(err, application.ErrSessionNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "evidence.not-found", "upload session not found", nil)
	case errors.Is(err, application.ErrConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "evidence.conflict", "same key, different intent", nil)
	case errors.Is(err, application.ErrStorageUnavailable):
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "evidence.storage-unavailable", "upload storage not configured", nil)
	case errors.Is(err, domain.ErrUnsupportedMedia),
		errors.Is(err, domain.ErrSizeOutOfBounds),
		errors.Is(err, domain.ErrBadHashClaim),
		errors.Is(err, domain.ErrInvalidSession):
		httpapi.WriteError(w, r, http.StatusBadRequest, "evidence.invalid", "invalid upload intent",
			[]httpapi.Detail{{Field: "body", Code: "invalid"}})
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "evidence.invalid", "invalid upload intent",
			[]httpapi.Detail{{Field: "body", Code: "invalid"}})
	}
}

// reserveDTO is the strict wire parse: unknown fields rejected, sizes kept
// in canonical integer form (no float, no exponent), like observations.
type reserveDTO struct {
	ClientSessionID string      `json:"client_submission_id"`
	ContentType     string      `json:"content_type"`
	SizeBytes       json.Number `json:"size_bytes"`
	SHA256          string      `json:"sha256"`
}

func parseReserveBody(raw []byte) (application.Intent, error) {
	var in reserveDTO
	if err := httpapi.DecodeJSON(raw, &in); err != nil {
		return application.Intent{}, err
	}
	amountStr := in.SizeBytes.String()
	if amountStr == "" {
		return application.Intent{}, errors.New("size required")
	}
	for _, r := range amountStr {
		if r < '0' || r > '9' {
			return application.Intent{}, errors.New("size must be a canonical integer")
		}
	}
	size, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		return application.Intent{}, err
	}
	if in.ClientSessionID == "" || in.ContentType == "" || in.SHA256 == "" {
		return application.Intent{}, errors.New("client_submission_id, content_type and sha256 are required")
	}
	return application.Intent{
		ClientSessionID: in.ClientSessionID,
		MIME:            in.ContentType,
		DeclaredBytes:   size,
		ClaimedSHA256:   in.SHA256,
	}, nil
}

func (h Handler) reserve(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, err := httpapi.ReadBody(r, 64<<10)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "evidence.bad-body", "unreadable body",
			[]httpapi.Detail{{Field: "body", Code: "unreadable"}})
		return
	}
	intent, err := parseReserveBody(raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "evidence.bad-body", "malformed upload intent",
			[]httpapi.Detail{{Field: "body", Code: "malformed"}})
		return
	}
	res, err := h.Reserve(r.Context(), caller, intent, raw)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"upload_id": res.SessionID, "method": "PUT", "url": res.URL,
		"required_headers": res.RequiredHeaders,
		"expires_at":       res.URLExpiresAt.Format(time.RFC3339),
		"max_bytes":        res.MaxBytes,
	})
	httpapi.WriteJSON(w, r, http.StatusCreated, "no-store", body)
}

func wireStatus(view application.StatusView) map[string]any {
	var evidence any
	if view.EvidenceID != "" {
		evidence = view.EvidenceID
	}
	return map[string]any{
		"upload_id": view.SessionID, "state": view.State,
		"evidence_id": evidence,
		"expires_at":  view.ExpiresAt.Format(time.RFC3339),
		"updated_at":  view.UpdatedAt.Format(time.RFC3339),
	}
}

func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	view, err := h.Status(r.Context(), caller, chi.URLParam(r, "upload_id"))
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	// writeAPIError maps unknown sessions to 404; a direct not-found here
	// keeps that shape even if the mapping ever changes.
	if view.SessionID == "" {
		httpapi.WriteError(w, r, http.StatusNotFound, "evidence.not-found", "upload session not found", nil)
		return
	}
	body, _ := json.Marshal(wireStatus(view))
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func (h Handler) complete(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	view, err := h.Complete(r.Context(), caller, chi.URLParam(r, "upload_id"))
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	if view.SessionID == "" {
		httpapi.WriteError(w, r, http.StatusNotFound, "evidence.not-found", "upload session not found", nil)
		return
	}
	body, _ := json.Marshal(wireStatus(view))
	// Non-terminal intents are accepted for processing; terminal states
	// report themselves with no new work.
	status := http.StatusOK
	if view.State == domain.StateVerifying {
		status = http.StatusAccepted
	}
	httpapi.WriteJSON(w, r, status, "no-store", body)
}
