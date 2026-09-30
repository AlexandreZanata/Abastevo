// Package http exposes the frozen identity contract through real request
// binding, bounded anonymous quotas and server-owned challenge metadata.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/profile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type Registrar interface {
	domain.Registrar
	Challenge(context.Context, string, string) (domain.Challenge, error)
	Rotate(context.Context, domain.RotationRequest) (domain.Rotation, error)
}
type Handler struct {
	Registrar   Registrar
	Authority   string
	QuotaSecret []byte
	CheckQuota  func(context.Context, string, string) (time.Duration, error)
	Now         func() time.Time
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/identity/challenges", h.challenge)
	r.Post("/v1/contributors", h.register)
	r.Post("/v1/contributors/me/keys/rotate", h.rotate)
}

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (j jwk) fingerprint() (string, error) {
	if j.Kty != "EC" || j.Crv != "P-256" {
		return "", domain.ErrProofRequired
	}
	return profile.Thumbprint(j.X, j.Y)
}

type proof struct {
	Lines     []string `json:"lines"`
	Signature string   `json:"signature"`
}
type rotationProof struct {
	ChallengeID string   `json:"challenge_id"`
	Lines       []string `json:"lines"`
	Signature   string   `json:"signature"`
}
type registration struct {
	PublicJWK   jwk    `json:"public_jwk"`
	ChallengeID string `json:"challenge_id"`
	Proof       proof  `json:"proof"`
}
type rotation struct {
	Old    rotationProof `json:"old"`
	NewJWK jwk           `json:"new_jwk"`
	New    rotationProof `json:"new"`
}

// Rotation intent is separately canonicalized so neither signature covers
// itself. Both keys sign the same intent digest, binding the proposed key
// and both challenges; an intercepted old proof cannot be retargeted.
type rotationIntent struct {
	NewJWK         jwk    `json:"new_jwk"`
	OldChallengeID string `json:"old_challenge_id"`
	NewChallengeID string `json:"new_challenge_id"`
}

func read(r *http.Request, dst any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return errors.New("JSON required")
	}
	raw, err := httpapi.ReadBody(r, 64<<10)
	if err != nil {
		return err
	}
	return httpapi.DecodeJSON(raw, dst)
}
func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := http.StatusServiceUnavailable, "identity.unavailable", "identity service unavailable"
	switch {
	case errors.Is(err, domain.ErrQuotaExceeded):
		status, code, msg = 429, "identity.quota-exceeded", "quota exhausted, retry later"
	case errors.Is(err, domain.ErrChallengeSpent), errors.Is(err, domain.ErrKeyTakeover):
		status, code, msg = 409, "identity.conflict", "proof already used or key unavailable"
	case errors.Is(err, domain.ErrProofRequired), errors.Is(err, domain.ErrBadNonce), errors.Is(err, domain.ErrChallengeExpired):
		status, code, msg = 401, "identity.proof-required", "valid proof required"
	case errors.Is(err, domain.ErrBadFingerprint), errors.Is(err, domain.ErrUnknownPurpose):
		status, code, msg = 400, "identity.invalid", "invalid identity request"
	}
	httpapi.WriteError(w, r, status, code, msg, nil)
}
func bad(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteError(w, r, 400, "identity.bad-body", "malformed identity request", nil)
}
func (h Handler) quota(w http.ResponseWriter, r *http.Request, fp, op string) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		bad(w, r)
		return false
	}
	day := h.now().UTC().Format("2006-01-02")
	subject, err := domain.HashIPSubject(day, h.QuotaSecret, peer)
	if err != nil {
		fail(w, r, err)
		return false
	}
	// Raw IP and untrusted proxy headers never enter the quota table.
	for _, s := range []string{subject, fp} {
		wait, err := h.CheckQuota(r.Context(), s, op)
		if err != nil {
			if wait > 0 {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(wait.Seconds())+1, 10))
			}
			fail(w, r, err)
			return false
		}
	}
	return true
}
func (h Handler) challenge(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		Fingerprint string `json:"fingerprint"`
		Purpose     string `json:"purpose"`
	}
	if err := read(r, &dto); err != nil {
		bad(w, r)
		return
	}
	fp, err := domain.ParseFingerprint(dto.Fingerprint)
	if err != nil {
		fail(w, r, err)
		return
	}
	purpose, err := domain.ParsePurpose(dto.Purpose)
	if err != nil {
		fail(w, r, err)
		return
	}
	if !h.quota(w, r, fp, domain.OperationChallenge) {
		return
	}
	ch, err := h.Registrar.IssueChallenge(r.Context(), fp, purpose)
	if err != nil {
		fail(w, r, err)
		return
	}
	body, _ := json.Marshal(map[string]any{"challenge_id": ch.ID, "nonce": ch.Nonce, "fingerprint": ch.Fingerprint, "purpose": ch.Purpose, "expires_at": ch.ExpiresAt.UTC().Format(time.RFC3339)})
	httpapi.WriteJSON(w, r, 201, "no-store", body)
}
func (h Handler) bound(r *http.Request, id string, p proof, intent []byte) (domain.RotationProof, error) {
	if len(p.Lines) != 8 && len(p.Lines) != 10 {
		return domain.RotationProof{}, domain.ErrProofRequired
	}
	metadata := p.Lines[len(p.Lines)-4:]
	vals := make([]string, 4)
	for i, name := range []string{"created", "expires", "keyid", "nonce"} {
		prefix := `"` + name + `": `
		if !strings.HasPrefix(metadata[i], prefix) {
			return domain.RotationProof{}, domain.ErrProofRequired
		}
		vals[i] = strings.TrimPrefix(metadata[i], prefix)
		if i >= 2 {
			v, err := strconv.Unquote(vals[i])
			if err != nil || strings.ContainsAny(v, "\r\n") {
				return domain.RotationProof{}, domain.ErrProofRequired
			}
			vals[i] = v
		} else {
			n, err := strconv.ParseInt(vals[i], 10, 64)
			if err != nil || strconv.FormatInt(n, 10) != vals[i] {
				return domain.RotationProof{}, domain.ErrProofRequired
			}
		}
	}
	expected := auth.BaseLines(r, h.Authority, vals[0], vals[1], vals[2], vals[3], intent, intent != nil)
	if !slices.Equal(expected, p.Lines) {
		return domain.RotationProof{}, domain.ErrProofRequired
	}
	ch, err := h.Registrar.Challenge(r.Context(), id, vals[3])
	if err != nil {
		return domain.RotationProof{}, err
	}
	if ch.Fingerprint != vals[2] {
		return domain.RotationProof{}, domain.ErrProofRequired
	}
	return domain.RotationProof{Challenge: ch, BaseLines: expected, Signature: p.Signature}, nil
}
func (h Handler) register(w http.ResponseWriter, r *http.Request) {
	var dto registration
	if err := read(r, &dto); err != nil {
		bad(w, r)
		return
	}
	fp, err := dto.PublicJWK.fingerprint()
	if err != nil {
		bad(w, r)
		return
	}
	if !h.quota(w, r, fp, domain.OperationRegister) {
		return
	}
	bound, err := h.bound(r, dto.ChallengeID, dto.Proof, nil)
	if err != nil {
		fail(w, r, err)
		return
	}
	out, err := h.Registrar.Register(r.Context(), domain.RegistrationRequest{JWKX: dto.PublicJWK.X, JWKY: dto.PublicJWK.Y, Challenge: bound.Challenge, BaseLines: bound.BaseLines, Signature: bound.Signature, VerifiedAt: h.now()})
	if err != nil {
		fail(w, r, err)
		return
	}
	body, _ := json.Marshal(map[string]any{"contributor_id": out.ContributorID, "key_id": out.KeyID, "existed": out.Existed})
	httpapi.WriteJSON(w, r, 201, "no-store", body)
}
func (h Handler) rotate(w http.ResponseWriter, r *http.Request) {
	var dto rotation
	if err := read(r, &dto); err != nil {
		bad(w, r)
		return
	}
	fp, err := dto.NewJWK.fingerprint()
	if err != nil {
		bad(w, r)
		return
	}
	if !h.quota(w, r, fp, domain.OperationRegister) {
		return
	}
	intent, _ := json.Marshal(rotationIntent{dto.NewJWK, dto.Old.ChallengeID, dto.New.ChallengeID})
	old, err := h.bound(r, dto.Old.ChallengeID, proof{dto.Old.Lines, dto.Old.Signature}, intent)
	if err != nil {
		fail(w, r, err)
		return
	}
	fresh, err := h.bound(r, dto.New.ChallengeID, proof{dto.New.Lines, dto.New.Signature}, intent)
	if err != nil {
		fail(w, r, err)
		return
	}
	out, err := h.Registrar.Rotate(r.Context(), domain.RotationRequest{Old: old, NewJWKX: dto.NewJWK.X, NewJWKY: dto.NewJWK.Y, New: fresh, VerifiedAt: h.now()})
	if err != nil {
		fail(w, r, err)
		return
	}
	body, _ := json.Marshal(map[string]any{"contributor_id": out.ContributorID, "old_key_id": out.OldKeyID, "new_key_id": out.NewKeyID, "existed": out.Existed})
	httpapi.WriteJSON(w, r, 200, "no-store", body)
}
