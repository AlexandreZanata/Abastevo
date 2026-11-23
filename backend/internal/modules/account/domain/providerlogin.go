package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

// Native provider sign-in (B-BR-A01/A03, P13-T05): a verified provider
// identity token opens a FREE account session without a prior email
// or key account. Only Apple is offered natively; its identity token
// carries the client application's own audience (the app bundle ID),
// never the frozen link audience, so a token minted for linking can
// never sign in and the reverse.
//
// Nonce binding: Apple copies the request nonce verbatim into the
// token's nonce claim. The native client generates a random raw nonce,
// sets the request nonce to NativeNonceClaim(raw) and sends the raw
// value to the server, which hashes it before comparing. A captured
// identity token alone therefore never satisfies the server: the raw
// preimage is required, and the claim is consumed once in the shared
// nonce ledger.
const (
	// NativeNonceMinLength/MaxLength bound the raw nonce: at least 16
	// visible ASCII characters (comfortably above 64 bits from a
	// random alphanumeric source) and at most 128.
	NativeNonceMinLength = 16
	NativeNonceMaxLength = 128
	// MaxProviderTokenBytes caps the compact identity token before
	// any parsing or key fetch. Apple identity tokens are about 1 KiB.
	MaxProviderTokenBytes = 4096
)

// NativeLoginProvider reports whether provider offers native sign-in.
// Google stays link-only until its native flow is specified.
func NativeLoginProvider(provider string) bool {
	return provider == "apple"
}

// ValidNativeNonce reports whether raw is a well-formed native nonce:
// NativeNonceMinLength..NativeNonceMaxLength visible ASCII characters
// (0x21-0x7E), no whitespace or control bytes.
func ValidNativeNonce(raw string) bool {
	if len(raw) < NativeNonceMinLength || len(raw) > NativeNonceMaxLength {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x21 || raw[i] > 0x7e {
			return false
		}
	}
	return true
}

// NativeNonceClaim derives the nonce claim the native client bound into
// the identity token request: lowercase hex SHA-256 of the raw nonce.
func NativeNonceClaim(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ValidProviderToken reports whether raw has the cheap shape of a
// compact JWS within MaxProviderTokenBytes: base64url segments joined
// by dots. Signature, issuer, audience, expiry and nonce stay with the
// verifier; this only refuses oversized or foreign input before it.
func ValidProviderToken(raw string) bool {
	if len(raw) == 0 || len(raw) > MaxProviderTokenBytes {
		return false
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}
