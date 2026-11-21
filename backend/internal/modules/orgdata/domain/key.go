package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalidKey  = errors.New("orgdata: invalid key")
	ErrKeyDenied   = errors.New("orgdata: key denied")
	ErrKeyConflict = errors.New("orgdata: key revision conflict")
)

type Scope string

const ReadRegional Scope = "regional.read"

type KeyState string

const (
	KeyActive   KeyState = "ACTIVE"
	KeyRotating KeyState = "ROTATING"
	KeyRevoked  KeyState = "REVOKED"
	KeyExpired  KeyState = "EXPIRED"
)

type Key struct {
	ID           string
	Label        string
	Scopes       []Scope
	State        KeyState
	Revision     int64
	IssuedAt     time.Time
	ExpiresAt    time.Time
	OverlapUntil time.Time
}

func (key Key) LiveAt(instant time.Time, required Scope) bool {
	if instant.IsZero() || instant.Before(key.IssuedAt) || !instant.Before(key.ExpiresAt) ||
		!slices.Contains(key.Scopes, required) {
		return false
	}
	return key.State == KeyActive || key.State == KeyRotating && instant.Before(key.OverlapUntil)
}

func (key Key) DisplayStateAt(instant time.Time) KeyState {
	if key.State != KeyRevoked && (!instant.Before(key.ExpiresAt) || key.State == KeyRotating && !instant.Before(key.OverlapUntil)) {
		return KeyExpired
	}
	return key.State
}

func KeyTerms(label string, scopes []Scope, expiresAt time.Time) error {
	if !publicText(label, 100) || len(scopes) != 1 || scopes[0] != ReadRegional || expiresAt.IsZero() ||
		expiresAt.Year() < 1 || expiresAt.Year() > 9999 {
		return ErrInvalidKey
	}
	return nil
}

func (key Key) CanRotate(scopes []Scope, overlapUntil, expiresAt, now time.Time) error {
	if key.State != KeyActive || now.Before(key.IssuedAt) || !now.Before(key.ExpiresAt) ||
		!now.Before(expiresAt) || overlapUntil.Before(now) ||
		overlapUntil.After(key.ExpiresAt) || overlapUntil.After(expiresAt) {
		return ErrKeyConflict
	}
	for _, scope := range scopes {
		if !slices.Contains(key.Scopes, scope) {
			return ErrKeyDenied
		}
	}
	return nil
}

const bearerPrefix = "abv_data_"

type Bearer struct {
	id     string
	secret string
}

func (Bearer) String() string       { return "[redacted Data key]" }
func (Bearer) GoString() string     { return "[redacted Data key]" }
func (Bearer) LogValue() slog.Value { return slog.StringValue("[redacted Data key]") }
func (key Bearer) ID() string       { return key.id }
func (key Bearer) Empty() bool      { return key.id == "" }

// Reveal is used only by the first successful issuance response.
func (key Bearer) Reveal() string {
	if key.Empty() {
		return ""
	}
	return bearerPrefix + key.id + "." + key.secret
}

func ParseBearer(raw string) (Bearer, error) {
	if len(raw) != len(bearerPrefix)+36+1+64 {
		return Bearer{}, ErrKeyDenied
	}
	body, ok := strings.CutPrefix(raw, bearerPrefix)
	if !ok {
		return Bearer{}, ErrKeyDenied
	}
	id, secret, ok := strings.Cut(body, ".")
	parsed, err := kernel.ParseOrgID(id)
	hash, hashErr := hex.DecodeString(secret)
	if !ok || err != nil || hashErr != nil || parsed.String() != id || len(hash) != sha256.Size ||
		secret != strings.ToLower(secret) {
		return Bearer{}, ErrKeyDenied
	}
	return Bearer{id: id, secret: secret}, nil
}

func NewBearer(id, secret string) (Bearer, error) {
	return ParseBearer(bearerPrefix + id + "." + secret)
}

type KeyVerifier struct{ pepper []byte }

func (KeyVerifier) String() string       { return "[redacted Data verifier]" }
func (KeyVerifier) GoString() string     { return "[redacted Data verifier]" }
func (KeyVerifier) LogValue() slog.Value { return slog.StringValue("[redacted Data verifier]") }

func NewKeyVerifier(base []byte) (KeyVerifier, error) {
	if len(base) < 32 {
		return KeyVerifier{}, ErrInvalidKey
	}
	mac := hmac.New(sha256.New, base)
	mac.Write([]byte("abastevo.organization-data-key.v1"))
	return KeyVerifier{pepper: mac.Sum(nil)}, nil
}

func (v KeyVerifier) Digest(scope kernel.OrgScope, key Bearer) (string, error) {
	if len(v.pepper) != sha256.Size || scope.Validate() != nil || key.Empty() {
		return "", ErrInvalidKey
	}
	mac := hmac.New(sha256.New, v.pepper)
	mac.Write([]byte(scope.OrgID().String() + ":" + key.id + ":" + key.secret))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (v KeyVerifier) Matches(scope kernel.OrgScope, key Bearer, stored string) bool {
	digest, err := v.Digest(scope, key)
	return err == nil && subtle.ConstantTimeCompare([]byte(digest), []byte(stored)) == 1
}
