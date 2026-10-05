// Package authority implements P31-T02 corporate authority
// verification: applicant linkage and sufficient powers for the exact
// operating branch. Company existence, signer identity and authority
// stay separately supported inputs; this package decides only what
// the facts establish. Name-only, brand/root, custodian and
// shareholder shortcuts deny; joint-signature gaps and unknown facts
// route to review (insufficient), never auto-approval. Fixtures are
// synthetic; Receita/QSA/corporate-act retrieval stays an explicit
// OPEN access item owned by the sourcing task.
package authority

import (
	"strings"
	"time"
)

// Outcomes: sufficient (powers established), insufficient (review),
// denied (established lack).
type Outcome string

const (
	OutcomeSufficient   Outcome = "sufficient"
	OutcomeInsufficient Outcome = "insufficient"
	OutcomeDenied       Outcome = "denied"
)

// Person is the applicant as identified by verified documents (never
// a bare name: empty documents cannot resolve identity).
type Person struct {
	Document string
	Name     string
}

// BranchClaim is the claimed establishment and role.
type BranchClaim struct {
	CNPJ string
	Role string
}

// PersonAuthority is one independently obtained power record.
type PersonAuthority struct {
	Document     string
	Name         string
	Role         string
	Powers       []string
	Branches     []string
	MandateUntil time.Time
	JointWith    string
}

// CompanyFacts are the independently obtained company records.
type CompanyFacts struct {
	Branches map[string]bool
	People   []PersonAuthority
}

// AuthorityQuery binds applicant, claim and company facts at a policy
// time.
type AuthorityQuery struct {
	Applicant Person
	Claim     BranchClaim
	Company   CompanyFacts
	At        time.Time
}

// AuthorityResult carries the outcome with a safe reason (never
// private document dumps).
type AuthorityResult struct {
	Outcome Outcome
	Reason  string
}

// Verify establishes applicant linkage and sufficient powers. Denials
// are terminal facts (custodian, shareholder, homonym, mismatch,
// expiry); gaps route to review.
func Verify(query AuthorityQuery) AuthorityResult {
	applicant := query.Applicant
	if strings.TrimSpace(applicant.Document) == "" {
		return AuthorityResult{Outcome: OutcomeDenied, Reason: "identity unresolved: name-only match never resolves"}
	}
	if !query.Company.Branches[query.Claim.CNPJ] {
		return AuthorityResult{Outcome: OutcomeInsufficient, Reason: "operating branch not established"}
	}
	var holder *PersonAuthority
	for i := range query.Company.People {
		person := &query.Company.People[i]
		if person.Document == applicant.Document {
			holder = person
			break
		}
	}
	if holder == nil {
		return AuthorityResult{Outcome: OutcomeDenied, Reason: "applicant holds no recorded position"}
	}
	if holder.Role != "administrator" && holder.Role != "manager" {
		return AuthorityResult{Outcome: OutcomeDenied, Reason: "position carries no management powers: " + holder.Role}
	}
	if !hasPower(holder.Powers, "manage") {
		return AuthorityResult{Outcome: OutcomeDenied, Reason: "recorded powers exclude management"}
	}
	covered := false
	for _, branch := range holder.Branches {
		if branch == query.Claim.CNPJ {
			covered = true
		}
	}
	if !covered {
		return AuthorityResult{Outcome: OutcomeDenied, Reason: "powers cover another branch"}
	}
	if !holder.MandateUntil.IsZero() && query.At.After(holder.MandateUntil) {
		return AuthorityResult{Outcome: OutcomeDenied, Reason: "mandate expired"}
	}
	if holder.JointWith != "" {
		return AuthorityResult{Outcome: OutcomeInsufficient, Reason: "joint signature requires the co-signer"}
	}
	if query.Claim.Role == "administrator" && holder.Role != "administrator" {
		return AuthorityResult{Outcome: OutcomeDenied, Reason: "manager cannot claim administration"}
	}
	return AuthorityResult{Outcome: OutcomeSufficient, Reason: "current administrator with branch coverage"}
}

func hasPower(powers []string, want string) bool {
	for _, power := range powers {
		if power == want {
			return true
		}
	}
	return false
}
