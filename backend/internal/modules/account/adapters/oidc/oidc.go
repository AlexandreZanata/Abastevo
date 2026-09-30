// Package oidc verifies Google and Apple identity tokens for FREE account
// linking (P13-T03A, B-BR-A03). Only established OIDC flows: the adapter
// checks issuer allowlist, exact audience, key id against the issuer JWKS,
// RS256/ES256 signatures from stdlib crypto, expiry inside the frozen skew
// and single-use nonces. Client identity flags are untrusted; equal email
// strings never link without a verified token (enforced in T03B).
// Provider outage, key rotation and revoked grants fail closed, never into
// a weaker proof.
package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// JSONWebKey is one JWKS entry (RSA or EC subset, nothing else accepted).
type JSONWebKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
}

// KeySource fetches an issuer's trusted key set. HTTPJWKS serves production;
// MemKeys serves tests and local runs; rotation is modeled by swapping the
// served set, exactly like a provider key rollover.
type KeySource interface {
	Fetch(ctx context.Context, issuer string) ([]JSONWebKey, error)
}

// NonceStore records consumed login nonces. One nonce authorizes one link.
type NonceStore interface {
	// Consume reserves nonce; false means already consumed.
	Consume(nonce string) bool
}

// Verifier implements domain.ProviderVerifier.
type Verifier struct {
	Clock    func() time.Time
	Issuers  map[string]string
	Audience string
	Keys     KeySource
	Nonces   NonceStore
	Skew     time.Duration
	CacheTTL time.Duration
}

// Verify checks provider, token, audience and nonce binding in order.
func (v Verifier) Verify(ctx context.Context, provider, rawToken, audience, nonce string) (domain.ProviderSubject, error) {
	issuer, ok := v.Issuers[provider]
	if !ok || issuer == "" {
		return domain.ProviderSubject{}, domain.ErrOIDCUnknownIssuer
	}
	if nonce == "" {
		return domain.ProviderSubject{}, domain.ErrOIDCNonceMismatch
	}
	claims, kid, alg, sig, signed, err := split(rawToken)
	if err != nil {
		return domain.ProviderSubject{}, domain.ErrOIDCWrongAudience
	}
	keys, err := v.Keys.Fetch(ctx, issuer)
	if err != nil {
		return domain.ProviderSubject{}, domain.ErrOIDCUnavailable
	}
	key, err := selectKey(keys, kid)
	if err != nil {
		return domain.ProviderSubject{}, domain.ErrOIDCUnknownIssuer
	}
	if err := verifySignature(key, alg, signed, sig); err != nil {
		return domain.ProviderSubject{}, domain.ErrOIDCWrongAudience
	}
	now := v.Clock()
	if claims.Issuer != issuer {
		return domain.ProviderSubject{}, domain.ErrOIDCUnknownIssuer
	}
	if !claims.audienceHas(audience) {
		return domain.ProviderSubject{}, domain.ErrOIDCWrongAudience
	}
	if now.After(time.Time(claims.Expiry).Add(v.Skew)) {
		return domain.ProviderSubject{}, domain.ErrOIDCExpired
	}
	if claims.Nonce != nonce {
		return domain.ProviderSubject{}, domain.ErrOIDCNonceMismatch
	}
	if claims.Subject == "" {
		return domain.ProviderSubject{}, domain.ErrOIDCWrongAudience
	}
	if !v.Nonces.Consume(nonce) {
		return domain.ProviderSubject{}, domain.ErrOIDCNonceReused
	}
	return domain.ProviderSubject{Issuer: issuer, Subject: claims.Subject, Email: claims.Email}, nil
}

type claims struct {
	Issuer  string   `json:"iss"`
	Subject string   `json:"sub"`
	Aud     any      `json:"aud"`
	Expiry  unixTime `json:"exp"`
	Nonce   string   `json:"nonce"`
	Email   string   `json:"email"`
}

type unixTime time.Time

func (u *unixTime) UnmarshalJSON(raw []byte) error {
	var secs int64
	if err := json.Unmarshal(raw, &secs); err != nil {
		return err
	}
	*u = unixTime(time.Unix(secs, 0).UTC())
	return nil
}

func (c claims) audienceHas(want string) bool {
	switch aud := c.Aud.(type) {
	case string:
		return aud == want
	case []any:
		for _, a := range aud {
			if s, ok := a.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func split(raw string) (claims, string, string, []byte, []byte, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return claims{}, "", "", nil, nil, errors.New("oidc: malformed token")
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims{}, "", "", nil, nil, err
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil || header.Kid == "" {
		return claims{}, "", "", nil, nil, errors.New("oidc: malformed header")
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims{}, "", "", nil, nil, err
	}
	var c claims
	if err := json.Unmarshal(payloadRaw, &c); err != nil {
		return claims{}, "", "", nil, nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims{}, "", "", nil, nil, err
	}
	return c, header.Kid, header.Alg, sig, []byte(parts[0] + "." + parts[1]), nil
}

func selectKey(keys []JSONWebKey, kid string) (JSONWebKey, error) {
	for _, k := range keys {
		if k.Kid == kid {
			return k, nil
		}
	}
	return JSONWebKey{}, fmt.Errorf("oidc: unknown kid %q", kid)
}

func verifySignature(key JSONWebKey, alg string, signed, sig []byte) error {
	digest := sha256.Sum256(signed)
	switch {
	case key.Kty == "RSA" && alg == "RS256":
		n, e, err := rsaParts(key)
		if err != nil {
			return err
		}
		return rsa.VerifyPKCS1v15(&rsa.PublicKey{N: n, E: e}, crypto.SHA256, digest[:], sig)
	case key.Kty == "EC" && key.Crv == "P-256" && alg == "ES256":
		x, y, err := ecParts(key)
		if err != nil {
			return err
		}
		if len(sig) != 64 {
			return errors.New("oidc: malformed ES256 signature")
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(&ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, digest[:], r, s) {
			return errors.New("oidc: ES256 verification failed")
		}
		return nil
	default:
		return fmt.Errorf("oidc: unsupported key %q/%q", key.Kty, alg)
	}
}

func b64big(s string) (*big.Int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(raw), nil
}

func rsaParts(key JSONWebKey) (*big.Int, int, error) {
	n, err := b64big(key.N)
	if err != nil {
		return nil, 0, err
	}
	e, err := b64big(key.E)
	if err != nil {
		return nil, 0, err
	}
	if !e.IsInt64() || e.Int64() <= 0 || e.Int64() > 1<<31 {
		return nil, 0, errors.New("oidc: bad RSA exponent")
	}
	return n, int(e.Int64()), nil
}

func ecParts(key JSONWebKey) (*big.Int, *big.Int, error) {
	x, err := b64big(key.X)
	if err != nil {
		return nil, nil, err
	}
	y, err := b64big(key.Y)
	if err != nil {
		return nil, nil, err
	}
	return x, y, nil
}

// MemKeys serves fixed issuer key sets for tests and local runs.
type MemKeys struct {
	mu    sync.Mutex
	sets  map[string][]JSONWebKey
	calls int
}

// NewMemKeys returns a source over issuer URL to key set.
func NewMemKeys(sets map[string]StubIssuer) *MemKeys {
	out := map[string][]JSONWebKey{}
	for issuer, stub := range sets {
		out[issuer] = []JSONWebKey{stub.PublicJWK()}
	}
	return &MemKeys{sets: out}
}

// Fetch returns the served set or unavailability when the issuer is absent.
func (m *MemKeys) Fetch(_ context.Context, issuer string) ([]JSONWebKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	set, ok := m.sets[issuer]
	if !ok {
		return nil, domain.ErrOIDCUnavailable
	}
	return set, nil
}

// FailingKeys models provider outage: every fetch fails closed.
type FailingKeys struct{}

// Fetch always reports unavailability.
func (FailingKeys) Fetch(_ context.Context, _ string) ([]JSONWebKey, error) {
	return nil, domain.ErrOIDCUnavailable
}

// MemNonces is the mutex single-use set for tests and local runs.
type MemNonces struct {
	mu   sync.Mutex
	used map[string]bool
}

// NewMemNonces returns an empty nonce set.
func NewMemNonces() *MemNonces {
	return &MemNonces{used: map[string]bool{}}
}

// Consume reserves nonce, reporting false when already consumed.
func (m *MemNonces) Consume(nonce string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.used[nonce] {
		return false
	}
	m.used[nonce] = true
	return true
}
