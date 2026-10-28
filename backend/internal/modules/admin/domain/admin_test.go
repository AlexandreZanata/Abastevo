package domain

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// TestKeyFormat_ADM01_ADM02 covers the generated key shape and SHA-256.
func TestKeyFormat_ADM01_ADM02(t *testing.T) {
	raw := make([]byte, KeyRandom)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	key := KeyPrefix + base64.RawURLEncoding.EncodeToString(raw)
	if !ValidKey(key) {
		t.Fatal("generated key rejected")
	}
	if !ValidHash(HashKey(key)) {
		t.Fatal("generated hash rejected")
	}
	if Fingerprint(HashKey(key)) != HashKey(key)[:12] {
		t.Fatal("fingerprint mismatch")
	}
	for _, bad := range []string{
		"", "abv_admin_", "abv_admin_short",
		"abv_admin_" + strings.Repeat("a", 42), "abv_admin_" + strings.Repeat("a", 44),
	} {
		if ValidKey(bad) {
			t.Fatalf("invalid key accepted: %q", bad)
		}
	}
	for _, bad := range []string{"", strings.Repeat("A", 64), strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		if ValidHash(bad) {
			t.Fatalf("invalid hash accepted: %q", bad)
		}
	}
}

// TestKeyMatch_ADM03 checks constant-time matching and rotation fingerprints.
func TestKeyMatch_ADM03_ADM07(t *testing.T) {
	first := "abv_admin_" + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("1", KeyRandom)))
	second := "abv_admin_" + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("2", KeyRandom)))
	keys := []ConfiguredKey{
		{Hash: HashKey(first), Fingerprint: Fingerprint(HashKey(first))},
		{Hash: HashKey(second), Fingerprint: Fingerprint(HashKey(second))},
	}
	if key, ok := MatchKey(HashKey(second), keys); !ok || key.Fingerprint != Fingerprint(HashKey(second)) {
		t.Fatal("configured key not matched")
	}
	if _, ok := MatchKey(HashKey("abv_admin_unknown"), keys); ok {
		t.Fatal("unknown key matched")
	}
	if !FingerprintAllowed(Fingerprint(HashKey(first)), keys) {
		t.Fatal("live fingerprint refused")
	}
	if FingerprintAllowed(Fingerprint(HashKey("abv_admin_removed")), keys[:1]) {
		t.Fatal("removed key fingerprint accepted")
	}
}

// TestReason_ADM12 covers the 10–500 Unicode character bounds after trim.
func TestReason_ADM12(t *testing.T) {
	if ValidReason("   short   ") {
		t.Fatal("short reason accepted")
	}
	if !ValidReason("  valid administrative reason  ") {
		t.Fatal("valid reason refused")
	}
	if ValidReason(strings.Repeat("é", 500)) != true {
		t.Fatal("500-rune reason refused")
	}
	if ValidReason(strings.Repeat("é", 501)) {
		t.Fatal("501-rune reason accepted")
	}
	if !ValidUUID("11111111-1111-4111-8111-111111111111") {
		t.Fatal("valid UUID refused")
	}
	if ValidUUID("11111111-1111-4111-8111-11111111111Z") {
		t.Fatal("invalid UUID accepted")
	}
}

// TestSessionTiming_ADM05 covers the idle and absolute boundaries.
func TestSessionTiming_ADM05(t *testing.T) {
	created := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	expires := created.Add(AbsoluteLifetime)
	if !SessionLive(created, created, expires, created.Add(IdleTimeout-time.Second)) {
		t.Fatal("live session refused")
	}
	if SessionLive(created, created, expires, created.Add(IdleTimeout)) {
		t.Fatal("idle boundary accepted")
	}
	if SessionLive(created, created.Add(IdleTimeout-time.Second), expires, expires) {
		t.Fatal("absolute boundary accepted")
	}
}
