package domain

import "errors"

// ClaimOpen reports whether the claim still accepts proof/actions.
// Terminal states (approved/denied/cancelled/expired) never reopen:
// appeals and renewed proof open linked new claims.
func ClaimOpen(state string) bool {
	switch state {
	case "draft", "awaiting_proof", "checking", "needs_information", "in_review":
		return true
	default:
		return false
	}
}

// ActiveDeclarationState reports whether the declaration version still
// binds proof. Superseded/consumed/expired versions never rebind.
func ActiveDeclarationState(state string) bool {
	return state == "active"
}

// ScopesForRole freezes the capability matrix (P30-T01): administrator
// holds the full bounded set, manager a strict subset. No OWNER role
// exists; unknown roles fail instead of defaulting.
func ScopesForRole(role string) ([]string, error) {
	admin := []string{"profile.edit", "reply.official", "invite.propose", "revoke.request"}
	switch role {
	case "administrator":
		return admin, nil
	case "manager":
		return []string{"profile.edit", "reply.official"}, nil
	default:
		return nil, errors.New("stationprofile: unknown role")
	}
}

// Claim/declaration limits frozen by P30-T01.
const (
	DeclarationTTLMinutes = 30
	MaxProofAttempts      = 5
	MaxOpenClaims         = 3
)
