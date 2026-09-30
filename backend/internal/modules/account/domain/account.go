package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Account statuses. Suspension/erasure enforcement lands in P13-T04; the
// field is stored from day one so no migration rewrites history.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusDeleted   = "deleted"
)

// Account is a FREE identity: opaque alias for display, never email,
// provider subject or location.
type Account struct {
	ID        string
	Alias     string
	Status    string
	CreatedAt int64
}

// EmailCode is one issued access code. Only Salt+Hash persist; the code
// itself never touches storage, logs or errors.
type EmailCode struct {
	ID          string
	AddressHash string
	Salt        string
	Hash        string
	IssuedAt    int64
	Attempts    int
	ConsumedAt  int64
}

// Live reports whether the code still accepts guesses at nowUnix.
func (c EmailCode) Live(nowUnix int64) bool {
	return c.ConsumedAt == 0 && !CodeExpired(c.IssuedAt, nowUnix) && AttemptAllowed(c.Attempts)
}

// SessionFamily is one rotating refresh chain. Only salted token hashes
// persist; reuse of a superseded refresh token revokes the whole family.
type SessionFamily struct {
	ID            string
	AccountID     string
	RefreshSalt   string
	RefreshHash   string
	AccessSalt    string
	AccessHash    string
	AccessExpires int64
	IssuedAt      int64
	RevokedAt     int64
}

// Live reports whether the family may still rotate at nowUnix.
func (f SessionFamily) Live(nowUnix int64) bool {
	return f.RevokedAt == 0 && !RefreshExpired(f.IssuedAt, nowUnix)
}

// ProviderLink binds one verified external subject to a FREE account
// (P13-T03B, B-BR-A03). Issuer plus subject is the only merge key; Email
// is display/relay only and never authorizes a merge on its own.
type ProviderLink struct {
	AccountID string
	Provider  string
	Issuer    string
	Subject   string
	Email     string
	LinkedAt  int64
}

// ValidProvider reports whether provider names a supported OIDC provider.
func ValidProvider(provider string) bool {
	return provider == "google" || provider == "apple"
}

// NormalizeAddress trims and lowercases for hashing and quota keys. Lookup
// keys are always hashes of this form, never raw addresses.
func NormalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// AddressHash derives the PII-minimized lookup key for an address.
func AddressHash(address string) string {
	sum := sha256.Sum256([]byte(NormalizeAddress(address)))
	return hex.EncodeToString(sum[:])
}
