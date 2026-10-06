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

// ManageHandler serves scoped business management. Every privilege
// enforces server-side (grant scope, account liveness, operator
// currency, field policy, revision match) before any app button.
// All responses carry no-store.
type ManageHandler struct {
	EditPorts  func(r *http.Request) application.ManagePorts
	ReplyPorts func(r *http.Request) application.ManagePorts
	Sessions   func(ctx context.Context, familyID, accessToken string) (string, error)
}

// RegisterRoutes mounts the additive management paths.
func (h ManageHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/stations/{station_id}/profile", h.edit)
	r.Post("/v1/stations/{station_id}/profile/replies", h.reply)
}

type manageSessionDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

type editDTO struct {
	manageSessionDTO
	ExpectedRevision int               `json:"expected_revision"`
	Fields           map[string]string `json:"fields"`
}

type replyDTO struct {
	manageSessionDTO
	Product string `json:"product"`
	Text    string `json:"text"`
}

func readManage(r *http.Request, dst any) error {
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

func (h ManageHandler) sessionAccount(w http.ResponseWriter, r *http.Request, dto manageSessionDTO) (string, bool) {
	accountID, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "profile.session-invalid", "valid account session required", nil)
		return "", false
	}
	return accountID, true
}

func (h ManageHandler) edit(w http.ResponseWriter, r *http.Request) {
	var dto editDTO
	if err := readManage(r, &dto); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "profile.bad-request", "invalid profile input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.manageSessionDTO)
	if !ok {
		return
	}
	updated, err := application.EditBusinessFields(r.Context(), h.EditPorts(r), accountID, chi.URLParam(r, "station_id"), dto.ExpectedRevision, dto.Fields)
	if err != nil {
		failManage(w, r, err)
		return
	}
	raw, err := json.Marshal(map[string]any{
		"revision": updated.Revision, "business": updated.Business,
	})
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "profile.unavailable", "profile service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", raw)
}

func (h ManageHandler) reply(w http.ResponseWriter, r *http.Request) {
	var dto replyDTO
	if err := readManage(r, &dto); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "profile.bad-request", "invalid reply input", nil)
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.manageSessionDTO)
	if !ok {
		return
	}
	commentID, err := application.PostOfficialReply(r.Context(), h.ReplyPorts(r), accountID, chi.URLParam(r, "station_id"), dto.Product, dto.Text)
	if err != nil {
		failManage(w, r, err)
		return
	}
	raw, err := json.Marshal(map[string]any{"comment_id": commentID})
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "profile.unavailable", "profile service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, http.StatusCreated, "no-store", raw)
}

func failManage(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrManageGrant):
		httpapi.WriteError(w, r, http.StatusForbidden, "profile.forbidden", "no approved scope for this action", nil)
	case errors.Is(err, application.ErrManageStale):
		httpapi.WriteError(w, r, http.StatusConflict, "profile.stale", "operator changed, reload and retry", nil)
	case errors.Is(err, application.ErrManageFields):
		httpapi.WriteError(w, r, http.StatusBadRequest, "profile.invalid-fields", "business fields outside the frozen policy", nil)
	case errors.Is(err, application.ErrManageVersion):
		httpapi.WriteError(w, r, http.StatusConflict, "profile.stale", "profile changed, reload and retry", nil)
	case errors.Is(err, application.ErrAccountGone):
		httpapi.WriteError(w, r, http.StatusForbidden, "profile.forbidden", "account is not active", nil)
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "profile.invalid", "invalid profile input", nil)
	}
}
