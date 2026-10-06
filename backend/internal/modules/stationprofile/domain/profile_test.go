package domain

import (
	"testing"
)

// P30-T02 public profile and operator-link rules (stdlib-only domain).
// One profile per station UUID; operator revisions carry validity with
// exactly one open revision; public DTOs never carry private proof,
// personal identity, prices or grants.

func TestPolicyVersionFrozen(t *testing.T) {
	if PolicyVersion != "profile-v1" {
		t.Fatalf("policy version = %q", PolicyVersion)
	}
}

func TestOperatorSourceFrozen(t *testing.T) {
	for _, source := range []string{"registry", "dou", "review"} {
		if !ValidOperatorSource(source) {
			t.Fatalf("source %q must be valid", source)
		}
	}
	if ValidOperatorSource("suggestion") {
		t.Fatal("suggestions are not operator evidence")
	}
}

func TestClaimStatesTerminal(t *testing.T) {
	for _, state := range []string{"approved", "denied", "cancelled", "expired"} {
		if !TerminalClaimState(state) {
			t.Fatalf("state %q must be terminal", state)
		}
	}
	for _, state := range []string{"draft", "awaiting_proof", "in_review"} {
		if TerminalClaimState(state) {
			t.Fatalf("state %q must not be terminal", state)
		}
	}
}

func TestPublicProjectionKeysBounded(t *testing.T) {
	allowed := map[string]bool{}
	for _, key := range PublicBusinessKeys {
		if allowed[key] {
			t.Fatalf("duplicate key %q", key)
		}
		allowed[key] = true
	}
	for _, forbidden := range []string{"cpf", "private_key", "password", "gps", "price"} {
		if allowed[forbidden] {
			t.Fatalf("private key %q must never be public", forbidden)
		}
	}
}
