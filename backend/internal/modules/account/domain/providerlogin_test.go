package domain

import (
	"strings"
	"testing"
)

func TestNativeLoginOffersAppleOnly(t *testing.T) {
	if !NativeLoginProvider("apple") {
		t.Fatal("apple must offer native sign-in")
	}
	for _, p := range []string{"google", "", "APPLE", "facebook"} {
		if NativeLoginProvider(p) {
			t.Fatalf("%q must not offer native sign-in", p)
		}
	}
}

func TestValidNativeNonceBounds(t *testing.T) {
	cases := map[string]bool{
		strings.Repeat("a", 15):          false,
		strings.Repeat("a", 16):          true,
		strings.Repeat("Z", 128):         true,
		strings.Repeat("Z", 129):         false,
		"abc-._~!0123456789":             true,
		"has space in the middle 123":    false,
		"tab\tinside-the-nonce-1":        false,
		"newline\ninside-the-nonce":      false,
		"non-ascii-é-inside-the-nonce":   false,
		"":                               false,
		"\x7f" + strings.Repeat("a", 16): false,
	}
	for raw, want := range cases {
		if got := ValidNativeNonce(raw); got != want {
			t.Errorf("ValidNativeNonce(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestNativeNonceClaimIsLowercaseHexSHA256(t *testing.T) {
	// Known-answer: SHA-256("abc").
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := NativeNonceClaim("abc"); got != want {
		t.Fatalf("NativeNonceClaim = %s", got)
	}
	if NativeNonceClaim("raw-nonce-0000001") == "raw-nonce-0000001" {
		t.Fatal("claim must never equal the raw nonce")
	}
}

func TestValidProviderTokenShape(t *testing.T) {
	if !ValidProviderToken("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln_-") {
		t.Fatal("compact JWS refused")
	}
	if !ValidProviderToken(strings.Repeat("a", MaxProviderTokenBytes)) {
		t.Fatal("boundary length refused")
	}
	for _, raw := range []string{
		"",
		strings.Repeat("a", MaxProviderTokenBytes+1),
		"eyJ hbGc.eyJ.sig",
		"eyJ+/=.eyJ.sig",
		"eyJ.eyJ.sig\n",
	} {
		if ValidProviderToken(raw) {
			t.Fatalf("ValidProviderToken accepted %q", raw)
		}
	}
}

func TestProviderLoginVerdictsAreStable(t *testing.T) {
	if got := VerdictCode(ErrProviderLoginUnavailable); got != "provider-unavailable" {
		t.Fatal(got)
	}
	if got := VerdictCode(ErrProviderProofMalformed); got != "provider-proof-malformed" {
		t.Fatal(got)
	}
}
