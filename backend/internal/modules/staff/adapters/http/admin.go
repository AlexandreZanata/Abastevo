package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type AdminHandler struct {
	Service *application.Service
	Admin   *adminhttp.Handler
}

func (h AdminHandler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/admin/roles", h.catalog)
	r.Post("/v1/admin/accounts/{id}/roles", h.assign)
	r.Post("/v1/admin/role-assignments/{id}/revoke", h.revoke)
}

func (h AdminHandler) catalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog_version": domain.CatalogVersion,
		"roles":           h.Service.Catalog(),
	})
}

type assignDTO struct {
	Role   string     `json:"role"`
	EndAt  *time.Time `json:"end_at"`
	Reason string     `json:"reason"`
}

func (h AdminHandler) assign(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	accountID := chi.URLParam(r, "id")
	var dto assignDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, "admin.staff.assign", "staff_assignment", accountID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	assignment, err := h.Service.Assign(r.Context(), m, application.AssignInput{
		AccountID: accountID, Role: dto.Role, EndAt: dto.EndAt,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	body, err := json.Marshal(assignment)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "staff.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusCreated, Body: body}, nil, false)
}

func (h AdminHandler) revoke(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}
	assignmentID := chi.URLParam(r, "id")
	var dto struct {
		Reason string `json:"reason"`
	}
	m, ok := h.Admin.PrepareMutation(w, r, session, "admin.staff.revoke", "staff_assignment", assignmentID, &dto)
	if !ok {
		return
	}
	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	assignment, err := h.Service.Revoke(r.Context(), m, assignmentID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	body, err := json.Marshal(assignment)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "staff.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusOK, Body: body}, nil, false)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string) {
	httpapi.WriteError(w, r, status, code, "staff request refused", nil)
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "staff.invalid")
	case errors.Is(err, domain.ErrIneligible):
		writeError(w, r, http.StatusConflict, "staff.account-ineligible")
	case errors.Is(err, domain.ErrConflict):
		writeError(w, r, http.StatusConflict, "staff.exists")
	case errors.Is(err, domain.ErrAlreadyRevoked):
		writeError(w, r, http.StatusConflict, "staff.already-revoked")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "staff.not-found")
	case errors.Is(err, admindomain.ErrConflict):
		// Idempotency-Key reused with a different body (ADM-12).
		writeError(w, r, http.StatusConflict, "admin.conflict")
	case errors.Is(err, admindomain.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
	case errors.Is(err, admindomain.ErrUnauthorized), errors.Is(err, admindomain.ErrDenied):
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
	default:
		writeError(w, r, http.StatusServiceUnavailable, "staff.unavailable")
	}
}
