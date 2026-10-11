package domain

// Frozen OTP policy (P13-T01, B-BR-A02). Six digits over a 10-minute TTL with
// five attempts: guessing odds stay 5/10^6 per code while legitimate typos
// survive; resend cooldown plus the hourly issuance cap bound mail cost and
// SMS-style bombing. Values change only with a new threat analysis, never to
// make a failing test pass.
const (
	CodeDigits                = 6
	CodeTTLSeconds            = 600
	CodeMaxAttempts           = 5
	ResendCooldownSeconds     = 60
	MaxCodesPerAddressPerHour = 5
)

// Frozen session policy (P13-T01, B-BR-A04). Short-lived access with rotating
// refresh families and a 30-day absolute ceiling; refresh reuse revokes the
// whole family (P13-T02). Suspension, deletion, device loss and provider
// change invalidate the appropriate scope immediately (P13-T04).
const (
	SessionAccessTTLSeconds     = 900
	RefreshAbsoluteLifetimeDays = 30
)

// Frozen admission policy (SEC01-C, B-BR-A01/A02/A04/A05). Aggregate and
// per-account ceilings bound durable and in-memory growth while keeping
// free registration and social participation free. Values change only
// with a new threat analysis, never to make a failing test pass.
const (
	// MaxAggregateCodesPerHour caps issued email codes across all
	// addresses per rolling hour. Per-address quota (5/hour) stays;
	// this stops distinct-address floods bypassing it.
	MaxAggregateCodesPerHour = 20
	// MaxKeyAccountsPerHour caps anonymous key-account creations
	// across all usernames per rolling hour.
	MaxKeyAccountsPerHour = 100
	// MaxLiveSessionsPerAccount caps live (non-revoked,
	// non-absolutely-expired) session families per account.
	MaxLiveSessionsPerAccount = 10
	// CodePurgeGraceSeconds retains expired/consumed code verifiers
	// briefly for audit, then PurgeExpiredCodes removes them.
	// Inventory: purged <=24h after expiry (FREE_ACCOUNT_ACCESS).
	CodePurgeGraceSeconds = 24 * 3600
	// SessionPurgeGraceSeconds retains absolutely-expired families
	// briefly, then PurgeExpiredFamilies removes them (<=7d after
	// absolute expiry per inventory).
	SessionPurgeGraceSeconds = 7 * 24 * 3600
)

// Frozen OIDC verification bounds (P13-T01, B-BR-A03). Only the two listed
// issuers, exact audience match, mandatory nonce and a 120s skew allowance;
// JWKS responses cache for one hour. Provider outage or key rotation yields
// an explicit refusal, never a weaker fallback proof.
const (
	OIDCClockSkewSeconds = 120
	JWKSCacheTTLSeconds  = 3600
)

// Allowed OIDC issuers. Apple relay addresses are accepted as subjects;
// accounts never merge on equal email strings without linking proof.
const (
	IssuerGoogle = "https://accounts.google.com"
	IssuerApple  = "https://appleid.apple.com"
)

// CodeExpired reports whether a code issued at issuedUnix is past its TTL at
// nowUnix. The TTL boundary itself is still live; one second more is not.
func CodeExpired(issuedUnix, nowUnix int64) bool {
	return nowUnix-issuedUnix > CodeTTLSeconds
}

// AttemptAllowed reports whether another guess fits inside the attempt bound.
func AttemptAllowed(attemptsUsed int) bool {
	return attemptsUsed < CodeMaxAttempts
}

// ResendAllowed reports whether the cooldown since lastIssuedUnix has passed.
func ResendAllowed(lastIssuedUnix, nowUnix int64) bool {
	return nowUnix-lastIssuedUnix >= ResendCooldownSeconds
}

// RefreshExpired reports whether a refresh family issued at issuedUnix is
// past its absolute lifetime at nowUnix. The boundary itself stays live.
func RefreshExpired(issuedUnix, nowUnix int64) bool {
	return nowUnix-issuedUnix > int64(RefreshAbsoluteLifetimeDays)*24*3600
}
