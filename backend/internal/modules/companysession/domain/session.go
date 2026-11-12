// Package domain defines the separate account-owned company cookie policy.
package domain

import (
	"encoding/hex"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"strings"
	"time"
)

const (
	TokenPrefix      = "abv_company_"
	CSRFPrefix       = "abv_company_csrf_"
	IdleTimeout      = 30 * time.Minute
	AbsoluteLifetime = 8 * time.Hour
	FreshWindow      = 15 * time.Minute
	MaxLiveSessions  = 10 // Existing account-session admission precedent.
)

var (
	ErrInvalid      = errors.New("companysession: invalid input")
	ErrUnauthorized = errors.New("companysession: session required")
	ErrCSRF         = errors.New("companysession: csrf denied")
	ErrFresh        = errors.New("companysession: fresh authentication required")
	ErrUnavailable  = errors.New("companysession: unavailable")
	ErrDisabled     = errors.New("companysession: disabled")
	ErrRateLimited  = errors.New("companysession: session limit reached")
)

func ValidID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}
func ValidHash(raw string) bool {
	if len(raw) != 64 || strings.ToLower(raw) != raw {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
}
func ValidSecret(raw, prefix string) bool {
	return strings.HasPrefix(raw, prefix) && ValidHash(strings.TrimPrefix(raw, prefix))
}
func Live(created, lastSeen, expires, now time.Time) bool {
	return !created.IsZero() && !lastSeen.Before(created) && !now.Before(lastSeen) &&
		expires.After(created) && !expires.After(created.Add(AbsoluteLifetime)) && now.Before(expires) && now.Sub(lastSeen) < IdleTimeout
}
func Fresh(authenticated, now time.Time) bool {
	return !authenticated.IsZero() && !now.Before(authenticated) && now.Before(authenticated.Add(FreshWindow))
}
