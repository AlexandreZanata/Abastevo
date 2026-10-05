package domain

import (
	"testing"
)

// P30-T03 claim/declaration state rules (stdlib-only domain). Terminal
// claims never reopen (appeals link new claims); declarations have one
// active version; consumed/expired proofs never rebind.

func TestClaimTransitions(t *testing.T) {
	open := []string{"draft", "awaiting_proof", "checking", "needs_information", "in_review"}
	for _, from := range open {
		if !ClaimOpen(from) {
			t.Fatalf("state %q must be open", from)
		}
		if TerminalClaimState(from) {
			t.Fatalf("state %q must not be terminal", from)
		}
	}
	if ClaimOpen("approved") || ClaimOpen("cancelled") {
		t.Fatal("terminal states must not be open")
	}
}

func TestDeclarationStates(t *testing.T) {
	if !ActiveDeclarationState("active") {
		t.Fatal("active must be active")
	}
	for _, state := range []string{"superseded", "consumed", "expired"} {
		if ActiveDeclarationState(state) {
			t.Fatalf("state %q must not be active", state)
		}
	}
}

func TestRoleScopesBounded(t *testing.T) {
	admin, err := ScopesForRole("administrator")
	if err != nil || len(admin) == 0 {
		t.Fatalf("admin = %v, err = %v", admin, err)
	}
	manager, err := ScopesForRole("manager")
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	for _, scope := range manager {
		found := false
		for _, allowed := range admin {
			if scope == allowed {
				found = true
			}
		}
		if !found {
			t.Fatalf("manager scope %q exceeds administrator", scope)
		}
	}
	if _, err := ScopesForRole("owner"); err == nil {
		t.Fatal("owner role must not exist")
	}
	if _, err := ScopesForRole(""); err == nil {
		t.Fatal("blank role must fail")
	}
}
