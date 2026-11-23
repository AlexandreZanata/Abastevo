package domain

// Provider grant revocation (P13-T05, App Store guideline 5.1.1(v)):
// apps offering Sign in with Apple revoke the user's Apple tokens when the
// account is deleted. A native sign-in may carry a single-use
// authorization code; when revocation credentials are configured the
// server redeems it for a refresh token, which persists only sealed
// (AES-256-GCM, key outside the database, bound to GrantAAD of the grant
// id). Deletion queues the sealed grants atomically with the account
// status change; a worker revokes them at the provider.
//
// Capture is best-effort: a failed or unconfigured exchange never blocks
// sign-in, and an account without a captured grant has nothing to
// revoke. Revocation retries with a capped exponential backoff and the
// sealed token is dropped after GrantRevocationWindowSeconds even when
// the provider never accepted it, so a deleted account's ciphertext has
// a bounded lifetime.
const (
	// MaxAuthorizationCodeBytes bounds the native authorization code
	// (Apple codes are well under 100 bytes, single-use, valid five
	// minutes).
	MaxAuthorizationCodeBytes = 512
	// GrantRevocationLeaseSeconds is one worker's exclusive claim.
	GrantRevocationLeaseSeconds = 120
	// GrantRevocationRetryBaseSeconds doubles per failed attempt up to
	// GrantRevocationRetryMaxSeconds.
	GrantRevocationRetryBaseSeconds = 60
	GrantRevocationRetryMaxSeconds  = 3600
	// GrantRevocationWindowSeconds bounds retention of a queued sealed
	// token: 30 days leaves operators time to repair credentials.
	GrantRevocationWindowSeconds = 30 * 24 * 3600
	// GrantRevocationBatch bounds one reconcile run.
	GrantRevocationBatch = 20
)

// ProviderGrant is one sealed provider refresh token of a live account.
type ProviderGrant struct {
	ID         string
	AccountID  string
	Provider   string
	Subject    string
	Sealed     []byte
	CapturedAt int64
}

// GrantRevocation is one queued revocation. It carries no account,
// subject or email: only the sealed token and its retry state.
type GrantRevocation struct {
	ID          string
	Provider    string
	Sealed      []byte
	RequestedAt int64
	Attempts    int
	LeaseNonce  string
}

// GrantAAD binds a sealed token to its grant id, so ciphertext moved
// between rows never opens.
func GrantAAD(grantID string) string {
	return "account-provider-grant:v1:" + grantID
}

// ValidAuthorizationCode reports whether code is 1..MaxAuthorizationCodeBytes
// visible ASCII characters.
func ValidAuthorizationCode(code string) bool {
	if len(code) == 0 || len(code) > MaxAuthorizationCodeBytes {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < 0x21 || code[i] > 0x7e {
			return false
		}
	}
	return true
}

// GrantRevocationBackoff returns the delay before the next attempt after
// attempts failures: base doubling, capped.
func GrantRevocationBackoff(attempts int) int64 {
	delay := int64(GrantRevocationRetryBaseSeconds)
	for i := 1; i < attempts && delay < GrantRevocationRetryMaxSeconds; i++ {
		delay *= 2
	}
	if delay > GrantRevocationRetryMaxSeconds {
		delay = GrantRevocationRetryMaxSeconds
	}
	return delay
}

// GrantRevocationExpired reports whether a revocation requested at
// requestedUnix is past its retention window at nowUnix. The boundary
// itself is still live.
func GrantRevocationExpired(requestedUnix, nowUnix int64) bool {
	return nowUnix-requestedUnix > GrantRevocationWindowSeconds
}
