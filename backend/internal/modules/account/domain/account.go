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

// Active reports whether the account may authenticate. Suspended and
// deleted accounts fail closed on every auth path (P13-T04A); deletion
// additionally drops address and provider bindings so a later signup
// with the same address mints a new account without reputation
// carryover.
func (a Account) Active() bool {
	return a.Status == StatusActive
}

// ContributorBinding links one anonymous device contributor to a FREE
// account (P13-T04B, B-BR-A04). The contributor keeps its observations
// and reputation; the account only scopes which key may write under
// it. KeyFingerprint is the proven device key at bind time.
type ContributorBinding struct {
	AccountID      string
	ContributorID  string
	KeyFingerprint string
	BoundAt        int64
}

// BindingAudit is one immutable bind/unbind record for the recovery
// audit trail.
type BindingAudit struct {
	ID            string
	AccountID     string
	ContributorID string
	Action        string
	OccurredAt    int64
}

// Binding actions recorded in the audit ledger.
const (
	BindingActionBind   = "bind"
	BindingActionUnbind = "unbind"
)

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
// persist. Proven reuse of a superseded refresh token (SEC07-A: matched
// against the bounded hashed rotation history) revokes the whole family;
// a random invalid credential refuses without revoking.
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

// RotatedVerifier is one superseded refresh verifier kept for proven-reuse
// detection (SEC07-A). Salted hash only, never plaintext.
type RotatedVerifier struct {
	Salt string
	Hash string
}

// MaxRefreshHistory bounds the per-family rotation history (SEC07-A):
// the newest N superseded verifiers prove reuse; older ones age out and
// refuse as invalid without revoking.
const MaxRefreshHistory = 8

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

// ValidEmail reports whether address is a cheaply-checkable email shape
// (SEC01-C, B-BR-A02): non-empty, <=254 bytes, one @ with local and domain
// parts, no spaces. Invalid shapes refuse before any store IO so floods
// of malformed input cost no backend reads or rows. Deeper deliverability
// is never claimed here; the HTTP layer shares this exact predicate.
func ValidEmail(address string) bool {
	address = strings.TrimSpace(address)
	if len(address) == 0 || len(address) > 254 {
		return false
	}
	at := strings.IndexByte(address, '@')
	return at > 0 && at < len(address)-1 && !strings.Contains(address, " ")
}

// AddressHash derives the PII-minimized lookup key for an address.
func AddressHash(address string) string {
	sum := sha256.Sum256([]byte(NormalizeAddress(address)))
	return hex.EncodeToString(sum[:])
}
