package adapters

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// ProofHandler serves private proof intake. Bytes arrive base64 in JSON
// (bounded bodies), validated before any storage; without provisioned
// object storage the endpoint refuses with 503 — missing storage
// blocks intake, never approval fallback. All responses no-store.
type ProofHandler struct {
	Ports    application.ProofPorts
	Sessions func(ctx context.Context, familyID, accessToken string) (string, error)
}

// RegisterRoutes mounts the additive proof path.
func (h ProofHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/profile/claims/{id}/proof", h.submit)
}

type proofDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
	Declaration string `json:"declaration_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Kind        string `json:"kind"`
	ContentB64  string `json:"content_base64"`
}

func (h ProofHandler) submit(w http.ResponseWriter, r *http.Request) {
	var dto proofDTO
	if err := read(r, &dto); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "proof.bad-request", "invalid proof input", nil)
		return
	}
	accountID, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "proof.session-invalid", "valid account session required", nil)
		return
	}
	body, err := base64.StdEncoding.DecodeString(dto.ContentB64)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "proof.bad-request", "content is not base64", nil)
		return
	}
	proof, created, err := application.SubmitProof(r.Context(), h.Ports, accountID, chi.URLParam(r, "id"), dto.Declaration, dto.Filename, dto.ContentType, dto.Kind, body)
	if err != nil {
		failProof(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	raw, merr := json.Marshal(map[string]any{
		"id": proof.ID, "status": proof.Status,
		"expires_at": proof.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
	if merr != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "proof.unavailable", "proof service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, status, "no-store", raw)
}

func failProof(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrClaimNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "proof.not-found", "claim or declaration not found", nil)
	case errors.Is(err, application.ErrClaimClosed) || errors.Is(err, application.ErrClaimProof):
		httpapi.WriteError(w, r, http.StatusConflict, "proof.closed", "claim or declaration is no longer open", nil)
	case errors.Is(err, application.ErrProofStorage):
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "proof.storage-unavailable", "proof storage not configured", nil)
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "proof.invalid", "invalid proof input", nil)
	}
}
