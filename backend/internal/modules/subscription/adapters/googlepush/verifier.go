// Package googlepush verifies authenticated Pub/Sub push identity using trusted
// provider keys supplied by the later payment module. No key/issuer is read from JWT URLs.
package googlepush

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"strings"
	"time"
)

type Verifier struct {
	Audience, ServiceAccount string
	Keys                     func(context.Context, string) (*rsa.PublicKey, error)
	Now                      func() time.Time
}

func (v Verifier) Verify(ctx context.Context, token string) error {
	if v.Keys == nil || v.Audience == "" || v.ServiceAccount == "" {
		return domain.ErrUnavailable
	}
	if len(token) == 0 || len(token) > 8192 {
		return domain.ErrUnauthorized
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return domain.ErrUnauthorized
	}
	head, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return domain.ErrUnauthorized
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(head, &h) != nil || h.Alg != "RS256" || h.Kid == "" || len(h.Kid) > 128 {
		return domain.ErrUnauthorized
	}
	body, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return domain.ErrUnauthorized
	}
	var c struct {
		Issuer   string `json:"iss"`
		Audience string `json:"aud"`
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Issued   int64  `json:"iat"`
		Expires  int64  `json:"exp"`
	}
	if json.Unmarshal(body, &c) != nil {
		return domain.ErrUnauthorized
	}
	now := time.Now()
	if v.Now != nil {
		now = v.Now()
	}
	if (c.Issuer != "https://accounts.google.com" && c.Issuer != "accounts.google.com") || c.Audience != v.Audience || c.Email != v.ServiceAccount || !c.Verified || c.Issued <= 0 || c.Expires <= now.Unix() || c.Issued > now.Add(time.Minute).Unix() || c.Expires <= c.Issued || c.Expires-c.Issued > 3660 {
		return domain.ErrUnauthorized
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[2])
	if e != nil {
		return domain.ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, domain.ProviderTimeout)
	defer cancel()
	key, e := v.Keys(ctx, h.Kid)
	if e != nil || key == nil || key.N == nil || key.N.BitLen() < 2048 {
		return domain.ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return domain.ErrUnauthorized
	}
	return nil
}
