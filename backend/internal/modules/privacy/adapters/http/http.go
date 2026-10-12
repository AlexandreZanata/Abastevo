package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	legal "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/legalterms"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Handler serves training-consent writes and the owner read. The
// function fields keep transport decoupled from composition: unit
// tests inject fakes, cmd/api injects the real application commands
// over the guarded store.
type Handler struct {
	Authenticate  func(r *http.Request) (privacyapp.Caller, error)
	GrantConsent  func(ctx context.Context, caller privacyapp.Caller, dto privacyapp.GrantDTO) (privacyapp.ConsentResult, error)
	RevokeConsent func(ctx context.Context, caller privacyapp.Caller, dto privacyapp.RevokeDTO) (privacyapp.ConsentResult, error)
	ReadConsent   func(ctx context.Context, caller privacyapp.Caller) (privacyapp.ConsentView, error)
}

// RegisterRoutes mounts the consent endpoints under /v1. All three
// are owner-scoped: writes require proof, and the read authenticates
// before touching the store (absent consent is the caller's own
// default OFF, never another owner's state).
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/privacy/training-terms", func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{"version": legal.Version, "digest": legal.Digest, "locale": "pt-BR", "text": legal.Text, "collection_available": false})
		httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
	})
	r.Post("/v1/privacy/training-consent/grant", h.grant)
	r.Post("/v1/privacy/training-consent/revoke", h.revoke)
	r.Get("/v1/privacy/training-consent", h.status)
}

func (h Handler) caller(w http.ResponseWriter, r *http.Request) (privacyapp.Caller, bool) {
	caller, err := h.Authenticate(r)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "privacy.auth-required", "authentication required", nil)
		return privacyapp.Caller{}, false
	}
	return caller, true
}

func writeConsentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, privacyapp.ErrUnauthorized):
		httpapi.WriteError(w, r, http.StatusUnauthorized, "privacy.auth-required", "authentication required", nil)
	case errors.Is(err, privacydomain.ErrTermsOutdated):
		httpapi.WriteError(w, r, http.StatusBadRequest, "privacy.terms-outdated", "terms superseded; review the current terms and grant again",
			[]httpapi.Detail{{Field: "terms_version", Code: "outdated"}})
	case errors.Is(err, privacydomain.ErrStaleConsent):
		httpapi.WriteError(w, r, http.StatusConflict, "privacy.stale-revision", "decision superseded; bind the current revision", nil)
	case errors.Is(err, privacydomain.ErrConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "privacy.conflict", "same revision, different decision", nil)
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "privacy.invalid", "invalid consent request",
			[]httpapi.Detail{{Field: "body", Code: "invalid"}})
	}
}

// consentGrantDTO is the strict wire parse for explicit ON decisions:
// the reviewed terms pair plus the per-owner monotonic revision.
// Unknown fields rejected; revision in canonical JSON number form
// (strings, floats and exponents refuse even when numerically equal).
type consentGrantDTO struct {
	TermsVersion string `json:"terms_version"`
	TermsDigest  string `json:"terms_digest"`
	// Revision stays declared so the strict decoder accepts the known
	// field; its type and value are enforced by strictRevision, not here.
	Revision any `json:"revision"`
}

// consentRevokeDTO is the strict wire parse for explicit OFF decisions.
type consentRevokeDTO struct {
	// Declared for the same reason as above; strictRevision enforces it.
	Revision any `json:"revision"`
}

func parseConsentRevision(raw any) (uint64, error) {
	num, ok := raw.(json.Number)
	if !ok {
		return 0, errors.New("revision must be a canonical positive integer")
	}
	text := strings.TrimSpace(num.String())
	if text == "" {
		return 0, errors.New("revision required")
	}
	n, err := strconv.ParseUint(text, 10, 64)
	if err != nil || n < 1 || strconv.FormatUint(n, 10) != text {
		return 0, errors.New("revision must be a canonical positive integer")
	}
	return n, nil
}

// strictRevision inspects the revision's JSON type before shape
// decoding: DecodeJSON's final pass does not use UseNumber, so a
// string-encoded number would otherwise unmarshal into json.Number
// and slip through. Numbers arrive as json.Number here; anything
// else (strings, bools, objects) refuses.
func strictRevision(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	v, ok := m["revision"]
	if !ok {
		return nil, errors.New("revision required")
	}
	return v, nil
}

func parseGrantBody(raw []byte) (privacyapp.GrantDTO, error) {
	if !utf8.Valid(raw) {
		return privacyapp.GrantDTO{}, errors.New("body must be valid UTF-8")
	}
	var in consentGrantDTO
	if err := httpapi.DecodeJSON(raw, &in); err != nil {
		return privacyapp.GrantDTO{}, err
	}
	if strings.TrimSpace(in.TermsVersion) == "" || strings.TrimSpace(in.TermsDigest) == "" {
		return privacyapp.GrantDTO{}, errors.New("terms_version and terms_digest required")
	}
	typed, err := strictRevision(raw)
	if err != nil {
		return privacyapp.GrantDTO{}, err
	}
	revision, err := parseConsentRevision(typed)
	if err != nil {
		return privacyapp.GrantDTO{}, err
	}
	return privacyapp.GrantDTO{
		TermsVersion: in.TermsVersion, TermsDigest: in.TermsDigest, Revision: revision,
	}, nil
}

func parseRevokeBody(raw []byte) (privacyapp.RevokeDTO, error) {
	if !utf8.Valid(raw) {
		return privacyapp.RevokeDTO{}, errors.New("body must be valid UTF-8")
	}
	var in consentRevokeDTO
	if err := httpapi.DecodeJSON(raw, &in); err != nil {
		return privacyapp.RevokeDTO{}, err
	}
	typed, err := strictRevision(raw)
	if err != nil {
		return privacyapp.RevokeDTO{}, err
	}
	revision, err := parseConsentRevision(typed)
	if err != nil {
		return privacyapp.RevokeDTO{}, err
	}
	return privacyapp.RevokeDTO{Revision: revision}, nil
}

func readConsentBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	raw, err := httpapi.ReadBody(r, 64<<10)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "privacy.bad-body", "unreadable body",
			[]httpapi.Detail{{Field: "body", Code: "unreadable"}})
		return nil, false
	}
	return raw, true
}

// consentReceiptBody renders the decision receipt: identifiers of the
// decision only, never contributor, token, key or device material.
func consentReceiptBody(res privacyapp.ConsentResult) map[string]any {
	return map[string]any{
		"state":         res.State,
		"revision":      res.Revision,
		"terms_version": res.TermsVersion,
		"decided_at":    res.DecidedAt.Format(time.RFC3339),
		"replayed":      res.Replayed,
	}
}

func (h Handler) grant(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, ok := readConsentBody(w, r)
	if !ok {
		return
	}
	dto, err := parseGrantBody(raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "privacy.bad-body", "malformed consent grant",
			[]httpapi.Detail{{Field: "body", Code: "malformed"}})
		return
	}
	res, err := h.GrantConsent(r.Context(), caller, dto)
	if err != nil {
		writeConsentError(w, r, err)
		return
	}
	body, _ := json.Marshal(consentReceiptBody(res))
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func (h Handler) revoke(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, ok := readConsentBody(w, r)
	if !ok {
		return
	}
	dto, err := parseRevokeBody(raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "privacy.bad-body", "malformed consent revocation",
			[]httpapi.Detail{{Field: "body", Code: "malformed"}})
		return
	}
	res, err := h.RevokeConsent(r.Context(), caller, dto)
	if err != nil {
		writeConsentError(w, r, err)
		return
	}
	out := consentReceiptBody(res)
	out["delete_owned_training_assets"] = res.DeleteOwnedTrainingAssets
	body, _ := json.Marshal(out)
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	view, err := h.ReadConsent(r.Context(), caller)
	if err != nil {
		writeConsentError(w, r, err)
		return
	}
	out := map[string]any{"granted": view.Granted}
	if view.State != "" {
		out["state"] = view.State
	}
	if view.TermsVersion != "" {
		out["terms_version"] = view.TermsVersion
	}
	if view.TermsDigest != "" {
		out["terms_digest"] = view.TermsDigest
	}
	if view.Revision != 0 {
		out["revision"] = view.Revision
	}
	if !view.DecidedAt.IsZero() {
		out["decided_at"] = view.DecidedAt.Format(time.RFC3339)
	}
	if view.NeedsReGrant {
		out["needs_regrant"] = true
	}
	body, _ := json.Marshal(out)
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}
