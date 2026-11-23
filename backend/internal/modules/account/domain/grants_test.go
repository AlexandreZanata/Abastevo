package domain

import (
	"strings"
	"testing"
)

func TestValidAuthorizationCodeBounds(t *testing.T) {
	if !ValidAuthorizationCode("c0a1b2.0.mrtz.AbCdEf_-") {
		t.Fatal("apple-shaped code refused")
	}
	if !ValidAuthorizationCode(strings.Repeat("c", MaxAuthorizationCodeBytes)) {
		t.Fatal("boundary length refused")
	}
	for _, code := range []string{"", strings.Repeat("c", MaxAuthorizationCodeBytes+1), "has space", "tab\tcode", "é"} {
		if ValidAuthorizationCode(code) {
			t.Fatalf("accepted %q", code)
		}
	}
}

func TestGrantRevocationBackoffDoublesAndCaps(t *testing.T) {
	want := map[int]int64{0: 60, 1: 60, 2: 120, 3: 240, 6: 1920, 7: 3600, 50: 3600}
	for attempts, delay := range want {
		if got := GrantRevocationBackoff(attempts); got != delay {
			t.Errorf("backoff(%d) = %d, want %d", attempts, got, delay)
		}
	}
}

func TestGrantRevocationWindowBoundary(t *testing.T) {
	const at = 1_800_000_000
	if GrantRevocationExpired(at, at+GrantRevocationWindowSeconds) {
		t.Fatal("the window boundary itself must stay live")
	}
	if !GrantRevocationExpired(at, at+GrantRevocationWindowSeconds+1) {
		t.Fatal("one second past the window must expire")
	}
}

func TestGrantAADBindsGrantID(t *testing.T) {
	if GrantAAD("a") == GrantAAD("b") || !strings.HasSuffix(GrantAAD("grant-1"), "grant-1") {
		t.Fatal("aad must bind the grant id")
	}
}
