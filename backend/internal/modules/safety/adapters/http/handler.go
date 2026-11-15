// Package http exposes community safety (B-BR-SAFE-01…09) as signed,
// non-cacheable JSON commands and private owner reads.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

const maxBody = 16 << 10

// Handler serves the safety routes. Sessions and SignedOwner authenticate
// the account and its bound signing key; Guard re-checks both inside
// write transactions (B-BR-SAFE-03).
type Handler struct {
	Service     application.Service
	Sessions    func(ctx context.Context, familyID, accessToken string) (accountID string, err error)
	SignedOwner func(r *http.Request, accountID string) (contributorID, keyID string, err error)
	Guard       func(ctx context.Context, tx pgx.Tx, accountID, familyID, accessToken, contributorID, keyID string) error
}

// RegisterRoutes mounts the five safety routes.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/community/user-reports", h.reportUser)
	r.Post("/v1/community/blocks", h.block)
	r.Post("/v1/community/blocks/list", h.listBlocks)
	r.Post("/v1/community/blocks/hidden", h.hidden)
	r.Post("/v1/community/blocks/{block_id}/remove", h.unblock)
}

type sessionEnvelope struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

type contentRequest struct {
	sessionEnvelope
	ContentKind string `json:"content_kind"`
	ContentID   string `json:"content_id"`
}

type reportRequest struct {
	sessionEnvelope
	ContentKind string `json:"content_kind"`
	ContentID   string `json:"content_id"`
	Reason      string `json:"reason"`
	Detail      string `json:"detail,omitempty"`
}

type refDTO struct {
	ContentKind string `json:"content_kind"`
	ContentID   string `json:"content_id"`
}

type hiddenRequest struct {
	sessionEnvelope
	Items []refDTO `json:"items"`
}

type blockDTO struct {
	BlockID    string `json:"block_id"`
	Label      string `json:"label"`
	SourceKind string `json:"source_kind"`
	CreatedAt  string `json:"created_at"`
}

type principal struct {
	accountID, familyID, accessToken, contributorID, keyID string
}

func (h Handler) authenticate(r *http.Request, env sessionEnvelope) (principal, bool) {
	if h.Sessions == nil || h.SignedOwner == nil {
		return principal{}, false
	}
	if _, err := domain.ParseID(env.FamilyID); err != nil || env.AccessToken == "" || len(env.AccessToken) > 512 {
		return principal{}, false
	}
	accountID, err := h.Sessions(r.Context(), env.FamilyID, env.AccessToken)
	if err != nil {
		return principal{}, false
	}
	if _, err := domain.ParseID(accountID); err != nil {
		return principal{}, false
	}
	contributorID, keyID, err := h.SignedOwner(r, accountID)
	if err != nil {
		return principal{}, false
	}
	return principal{accountID: accountID, familyID: env.FamilyID, accessToken: env.AccessToken, contributorID: contributorID, keyID: keyID}, true
}

func (h Handler) guard(p principal) application.Guard {
	if h.Guard == nil {
		return nil
	}
	return func(ctx context.Context, tx pgx.Tx) error {
		return h.Guard(ctx, tx, p.accountID, p.familyID, p.accessToken, p.contributorID, p.keyID)
	}
}

// readBody accepts only bounded application/json with strict decoding.
func readBody(r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return domain.ErrInvalid
	}
	raw, err := httpapi.ReadBody(r, maxBody)
	if err != nil {
		return domain.ErrInvalid
	}
	if err := httpapi.DecodeJSON(raw, dst); err != nil {
		return domain.ErrInvalid
	}
	return nil
}

// begin decodes the body and authenticates; it writes the refusal itself.
func (h Handler) begin(w http.ResponseWriter, r *http.Request, dst any, env func() sessionEnvelope) (principal, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if err := readBody(r, dst); err != nil {
		writeError(w, r, err)
		return principal{}, false
	}
	p, ok := h.authenticate(r, env())
	if !ok {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "safety.auth-required", "valid signed account session required", nil)
		return principal{}, false
	}
	return p, true
}

func (h Handler) reportUser(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	p, ok := h.begin(w, r, &req, func() sessionEnvelope { return req.sessionEnvelope })
	if !ok {
		return
	}
	receipt, err := h.Service.ReportUser(r.Context(), p.accountID, req.ContentKind, req.ContentID, req.Reason, req.Detail, h.guard(p))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, map[string]any{"receipt": map[string]any{
		"report_id":  receipt.ID,
		"reason":     string(receipt.Reason),
		"created_at": receipt.CreatedAt.UTC().Format(time.RFC3339),
		"replayed":   receipt.Replayed,
	}})
}

func (h Handler) block(w http.ResponseWriter, r *http.Request) {
	var req contentRequest
	p, ok := h.begin(w, r, &req, func() sessionEnvelope { return req.sessionEnvelope })
	if !ok {
		return
	}
	res, err := h.Service.Block(r.Context(), p.accountID, req.ContentKind, req.ContentID, h.guard(p))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, map[string]any{"block": toBlockDTO(res.Block), "replayed": res.Replayed})
}

func (h Handler) listBlocks(w http.ResponseWriter, r *http.Request) {
	var req sessionEnvelope
	p, ok := h.begin(w, r, &req, func() sessionEnvelope { return req })
	if !ok {
		return
	}
	blocks, err := h.Service.ListBlocks(r.Context(), p.accountID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items := make([]blockDTO, 0, len(blocks))
	for _, b := range blocks {
		items = append(items, toBlockDTO(b))
	}
	writeJSON(w, r, map[string]any{"items": items})
}

func (h Handler) unblock(w http.ResponseWriter, r *http.Request) {
	var req sessionEnvelope
	p, ok := h.begin(w, r, &req, func() sessionEnvelope { return req })
	if !ok {
		return
	}
	if err := h.Service.Unblock(r.Context(), p.accountID, chi.URLParam(r, "block_id"), h.guard(p)); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, map[string]any{"removed": true})
}

func (h Handler) hidden(w http.ResponseWriter, r *http.Request) {
	var req hiddenRequest
	p, ok := h.begin(w, r, &req, func() sessionEnvelope { return req.sessionEnvelope })
	if !ok {
		return
	}
	raw := make([]domain.RawRef, 0, len(req.Items))
	for _, it := range req.Items {
		raw = append(raw, domain.RawRef{Kind: it.ContentKind, ID: it.ContentID})
	}
	refs, err := h.Service.Hidden(r.Context(), p.accountID, raw)
	if err != nil {
		writeError(w, r, err)
		return
	}
	hidden := make([]refDTO, 0, len(refs))
	for _, ref := range refs {
		hidden = append(hidden, refDTO{ContentKind: string(ref.Kind), ContentID: ref.ID})
	}
	writeJSON(w, r, map[string]any{"hidden": hidden})
}

func toBlockDTO(b domain.Block) blockDTO {
	return blockDTO{BlockID: b.ID, Label: b.Label, SourceKind: string(b.SourceKind), CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339)}
}

func writeJSON(w http.ResponseWriter, r *http.Request, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "safety.unavailable", "safety service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", raw)
}

// writeError maps domain refusals to stable codes. Unexpected errors,
// including a session refused inside the transaction guard, never leak
// their text.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Cache-Control", "no-store")
	var quota *domain.QuotaError
	switch {
	case errors.As(err, &quota):
		w.Header().Set("Retry-After", strconv.Itoa(quota.RetryAfterSeconds))
		httpapi.WriteError(w, r, http.StatusTooManyRequests, "safety.rate-limited", "safety quota exceeded", nil)
	case errors.Is(err, domain.ErrInvalid):
		httpapi.WriteError(w, r, http.StatusBadRequest, "safety.bad-request", "invalid safety request", nil)
	case errors.Is(err, domain.ErrSelfTarget):
		httpapi.WriteError(w, r, http.StatusBadRequest, "safety.self-target", "cannot report or block your own content", nil)
	case errors.Is(err, domain.ErrNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "safety.not-found", "resource not found", nil)
	case errors.Is(err, domain.ErrBlockLimit):
		httpapi.WriteError(w, r, http.StatusConflict, "safety.block-limit", "active block limit reached", nil)
	case errors.Is(err, ErrGuardRefused):
		httpapi.WriteError(w, r, http.StatusUnauthorized, "safety.auth-required", "valid signed account session required", nil)
	default:
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "safety.unavailable", "safety service unavailable", nil)
	}
}

// ErrGuardRefused is returned by a composed Guard when the session, key
// binding or account status no longer holds inside the transaction.
var ErrGuardRefused = errors.New("safety: guard refused")
