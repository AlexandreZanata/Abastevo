package domain

import "testing"

func TestFrozenOTPBounds(t *testing.T) {
	if CodeDigits != 6 {
		t.Errorf("CodeDigits = %d, want 6", CodeDigits)
	}
	if CodeTTLSeconds != 600 {
		t.Errorf("CodeTTLSeconds = %d, want 600", CodeTTLSeconds)
	}
	if CodeMaxAttempts != 5 {
		t.Errorf("CodeMaxAttempts = %d, want 5", CodeMaxAttempts)
	}
	if ResendCooldownSeconds != 60 {
		t.Errorf("ResendCooldownSeconds = %d, want 60", ResendCooldownSeconds)
	}
	if MaxCodesPerAddressPerHour != 5 {
		t.Errorf("MaxCodesPerAddressPerHour = %d, want 5", MaxCodesPerAddressPerHour)
	}
}

func TestCodeExpiredBoundary(t *testing.T) {
	if CodeExpired(1000, 1599) {
		t.Error("age 599 must be live")
	}
	if CodeExpired(1000, 1600) {
		t.Error("age 600 must be live (TTL is inclusive)")
	}
	if !CodeExpired(1000, 1601) {
		t.Error("age 601 must be expired")
	}
}

func TestAttemptAndResendGates(t *testing.T) {
	if !AttemptAllowed(4) {
		t.Error("4 used attempts must allow one more")
	}
	if AttemptAllowed(5) {
		t.Error("5 used attempts must lock the code")
	}
	if !ResendAllowed(0, 60) {
		t.Error("60s since issue must allow resend")
	}
	if ResendAllowed(0, 59) {
		t.Error("59s since issue must hold the cooldown")
	}
}

func TestSessionBounds(t *testing.T) {
	if SessionAccessTTLSeconds != 900 {
		t.Errorf("SessionAccessTTLSeconds = %d, want 900", SessionAccessTTLSeconds)
	}
	if RefreshAbsoluteLifetimeDays != 30 {
		t.Errorf("RefreshAbsoluteLifetimeDays = %d, want 30", RefreshAbsoluteLifetimeDays)
	}
	if RefreshExpired(0, 30*24*3600) {
		t.Error("exactly 30d must still be live")
	}
	if !RefreshExpired(0, 30*24*3600+1) {
		t.Error("30d+1s must be expired")
	}
}

func TestOIDCSkewBound(t *testing.T) {
	if OIDCClockSkewSeconds != 120 {
		t.Errorf("OIDCClockSkewSeconds = %d, want 120", OIDCClockSkewSeconds)
	}
	if JWKSCacheTTLSeconds != 3600 {
		t.Errorf("JWKSCacheTTLSeconds = %d, want 3600", JWKSCacheTTLSeconds)
	}
}
