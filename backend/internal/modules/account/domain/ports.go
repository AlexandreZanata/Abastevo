package domain

import (
	"context"
	"errors"
)

// Stable account verdict codes shared by fixtures, handlers and audit
// (P13-T01). Unknown addresses and wrong codes share "code-unknown" so
// responses never reveal registration.
const (
	VerdictOK = "ok"
)

var (
	ErrCodeExpired           = errors.New("account: code expired")
	ErrCodeConsumed          = errors.New("account: code already consumed")
	ErrCodeAttemptsExhausted = errors.New("account: code attempts exhausted")
	ErrCodeResendCooldown    = errors.New("account: resend cooldown active")
	ErrCodeUnknown           = errors.New("account: unknown code or address")
	ErrOIDCUnknownIssuer     = errors.New("account: unknown OIDC issuer")
	ErrOIDCWrongAudience     = errors.New("account: OIDC audience mismatch")
	ErrOIDCExpired           = errors.New("account: OIDC token expired")
	ErrOIDCNonceReused       = errors.New("account: OIDC nonce already consumed")
	ErrOIDCNonceMismatch     = errors.New("account: OIDC nonce mismatch")
	ErrOIDCUnavailable       = errors.New("account: provider unavailable")
	ErrLinkCrossAccount      = errors.New("account: proof belongs to another account")
	ErrLinkEmailOnly         = errors.New("account: email match is not linking proof")
	ErrLastLoginMethod       = errors.New("account: last login method cannot be removed")
	ErrProviderNotLinked     = errors.New("account: provider not linked")
	ErrAccountNotFound       = errors.New("account: unknown account")
	ErrAddressLinked         = errors.New("account: address already linked")
	ErrSessionReuse          = errors.New("account: refresh token reused")
	ErrSessionRevoked        = errors.New("account: session family revoked")
	ErrSessionExpired        = errors.New("account: session expired")
)

// Clock is the only time source portable logic may use; tests inject a fake.
type Clock interface {
	NowUnix() int64
}

// CodeHasher stores email codes as salted hashes, never plaintext. Compare
// must run in constant time over the hash bytes.
type CodeHasher interface {
	// NewSalt returns 16 random bytes encoded for storage.
	NewSalt() (string, error)
	// Hash derives the stored verifier for salt+code.
	Hash(salt, code string) string
	// Equal compares a stored verifier against a candidate code.
	Equal(storedHash, salt, code string) bool
}

// MailSender delivers access codes. Failures are redacted upstream; the
// sender never logs addresses or codes.
type MailSender interface {
	SendCode(ctx context.Context, address, code string) error
}

// ProviderSubject is the verified identity a provider token binds: issuer
// plus subject. Email is carried for display/relay only and is never a
// merge key on its own.
type ProviderSubject struct {
	Issuer  string
	Subject string
	Email   string
}

// ProviderVerifier checks issuer, audience, signature/JWKS, expiry, nonce
// and replay for one raw token. Unlisted issuers, audience mismatch,
// expiry past skew, consumed nonces and provider outage all fail closed.
type ProviderVerifier interface {
	Verify(ctx context.Context, provider, rawToken, audience, nonce string) (ProviderSubject, error)
}

// VerdictCode maps a domain error to the stable fixture/audit code so
// nothing fails without an explainable reason.
func VerdictCode(err error) string {
	switch {
	case err == nil:
		return VerdictOK
	case errors.Is(err, ErrCodeExpired):
		return "code-expired"
	case errors.Is(err, ErrCodeConsumed):
		return "code-consumed"
	case errors.Is(err, ErrCodeAttemptsExhausted):
		return "code-attempts-exhausted"
	case errors.Is(err, ErrCodeResendCooldown):
		return "code-resend-cooldown"
	case errors.Is(err, ErrCodeUnknown):
		return "code-unknown"
	case errors.Is(err, ErrOIDCUnknownIssuer):
		return "oidc-unknown-issuer"
	case errors.Is(err, ErrOIDCWrongAudience):
		return "oidc-wrong-audience"
	case errors.Is(err, ErrOIDCExpired):
		return "oidc-expired"
	case errors.Is(err, ErrOIDCNonceReused):
		return "oidc-nonce-reused"
	case errors.Is(err, ErrOIDCNonceMismatch):
		return "oidc-nonce-mismatch"
	case errors.Is(err, ErrOIDCUnavailable):
		return "oidc-unavailable"
	case errors.Is(err, ErrLinkCrossAccount):
		return "link-cross-account-refused"
	case errors.Is(err, ErrLinkEmailOnly):
		return "link-email-only-refused"
	case errors.Is(err, ErrLastLoginMethod):
		return "link-last-method-refused"
	case errors.Is(err, ErrProviderNotLinked):
		return "link-provider-not-linked"
	case errors.Is(err, ErrAccountNotFound):
		return "account-unknown"
	case errors.Is(err, ErrAddressLinked):
		return "address-linked"
	case errors.Is(err, ErrSessionReuse):
		return "session-reuse-revoked"
	case errors.Is(err, ErrSessionRevoked):
		return "session-revoked"
	case errors.Is(err, ErrSessionExpired):
		return "session-expired"
	default:
		return "invalid-value"
	}
}
