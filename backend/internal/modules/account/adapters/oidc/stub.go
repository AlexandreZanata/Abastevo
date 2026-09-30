package oidc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// StubIssuer mints and signs test tokens for one issuer (tests and local
// runs only; production reads provider JWKS over HTTPS). Rotation is
// modeled by minting a second issuer whose kid is absent from the served
// set, exactly like a provider key rollover.
type StubIssuer struct {
	Issuer string
	Kid    string
	Alg    string
	rsaKey *rsa.PrivateKey
	ecKey  *ecdsa.PrivateKey
}

// NewStubIssuer mints a fresh RS256 or ES256 test key for issuer.
func NewStubIssuer(issuer, alg string) (StubIssuer, error) {
	s := StubIssuer{Issuer: issuer, Alg: alg}
	kidRaw, err := randBytes(8)
	if err != nil {
		return StubIssuer{}, err
	}
	s.Kid = base64.RawURLEncoding.EncodeToString(kidRaw)
	switch alg {
	case "RS256":
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return StubIssuer{}, err
		}
		s.rsaKey = key
	case "ES256":
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return StubIssuer{}, err
		}
		s.ecKey = key
	default:
		return StubIssuer{}, fmt.Errorf("oidc: unsupported stub alg %q", alg)
	}
	return s, nil
}

// PublicJWK publishes the stub verification key in JWKS shape.
func (s StubIssuer) PublicJWK() JSONWebKey {
	switch s.Alg {
	case "RS256":
		return JSONWebKey{
			Kid: s.Kid, Kty: "RSA", Alg: "RS256", Use: "sig",
			N: bigBytes(s.rsaKey.N), E: bigBytes(big.NewInt(int64(s.rsaKey.E))),
		}
	default:
		return JSONWebKey{
			Kid: s.Kid, Kty: "EC", Alg: "ES256", Use: "sig", Crv: "P-256",
			X: bigBytes(s.ecKey.X), Y: bigBytes(s.ecKey.Y),
		}
	}
}

// ServeJSON renders {"keys":[...]} for HTTP JWKS tests.
func (s StubIssuer) ServeJSON() []byte {
	raw, _ := json.Marshal(map[string]any{"keys": []JSONWebKey{s.PublicJWK()}})
	return raw
}

// Token mints a token valid for ttl from now.
func (s StubIssuer) Token(sub, email, aud, nonce string, now time.Time, ttl time.Duration) (string, error) {
	return s.TokenWith(sub, email, aud, nonce, now, now.Add(ttl), s.Issuer)
}

// TokenWith mints a token with explicit expiry and issuer for attack cases.
func (s StubIssuer) TokenWith(sub, email, aud, nonce string, now, exp time.Time, iss string) (string, error) {
	_ = now
	header, _ := json.Marshal(map[string]string{"alg": s.Alg, "kid": s.Kid, "typ": "JWT"})
	payload, _ := json.Marshal(map[string]any{
		"iss": iss, "sub": sub, "aud": aud,
		"exp": exp.Unix(), "iat": now.Unix(), "nonce": nonce, "email": email,
	})
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	var sig []byte
	digest := sha256.Sum256([]byte(signing))
	switch s.Alg {
	case "RS256":
		out, err := rsa.SignPKCS1v15(rand.Reader, s.rsaKey, crypto.SHA256, digest[:])
		if err != nil {
			return "", err
		}
		sig = out
	case "ES256":
		r, sv, err := ecdsa.Sign(rand.Reader, s.ecKey, digest[:])
		if err != nil {
			return "", err
		}
		sig = append(pad32(r.Bytes()), pad32(sv.Bytes())...)
	default:
		return "", errors.New("oidc: unsupported stub alg")
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func bigBytes(n *big.Int) string {
	return base64.RawURLEncoding.EncodeToString(n.Bytes())
}

func pad32(b []byte) []byte {
	if len(b) >= 32 {
		return b[len(b)-32:]
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}
