package application

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/authority"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/verify"
)

// ReviewPorts isolates restricted review: live claim/account/operator
// state, supplied signature and authority evidence, and the atomic
// decision/grant writer. Reviewers arrive as opaque operator ids (no
// public admin route, no moderator role from representation).
type ReviewPorts struct {
	Claims      ClaimStore
	Decisions   DecisionStore
	OperatorOf  func(ctx context.Context, stationID string) (cnpj string, found bool, err error)
	AccountLive func(ctx context.Context, accountID string) (bool, error)
	NewID       func() (string, error)
}

// DecisionStore writes audited decisions and narrowly scoped grants
// atomically: one transaction carries decision + grant + claim-state
// transition, so concurrent approvals converge instead of duplicating
// authority.
type DecisionStore interface {
	DecideAtomically(ctx context.Context, decision DecisionInput, grant *GrantInput, newState string) error
	ActiveGrant(ctx context.Context, accountID, stationID string) (GrantRow, bool, error)
}

// DecisionInput is one audited review outcome.
type DecisionInput struct {
	ID            string
	ClaimID       string
	Reviewer      string
	Decision      string
	Reason        string
	PolicyVersion string
	ProofVersion  int
	OperatorCNPJ  string
	Scopes        []string
}

// GrantInput is one narrowly scoped capability.
type GrantInput struct {
	ID           string
	AccountID    string
	StationID    string
	OperatorCNPJ string
	Role         string
	Scopes       []string
	ClaimID      string
	DecisionID   string
}

// GrantRow is the stored capability.
type GrantRow struct {
	ID           string
	AccountID    string
	StationID    string
	OperatorCNPJ string
	Role         string
	Scopes       []string
	Status       string
	Version      int
}

// ReviewEvidence carries the independently established signature and
// authority results the reviewer evaluated. The service rechecks live
// state at commit; evidence never substitutes for it.
type ReviewEvidence struct {
	Signature verify.Result
	Authority authority.AuthorityResult
}

var (
	ErrReviewSelf     = errors.New("stationprofile: reviewer cannot decide their own claim")
	ErrReviewOperator = errors.New("stationprofile: reviewer identity is required")
	ErrReviewStale    = errors.New("stationprofile: operator or evidence changed since review")
	ErrReviewDenied   = errors.New("stationprofile: evidence does not support approval")
	ErrVerifyClosed   = errors.New("stationprofile: claim already decided")
	ErrVerifyReason   = errors.New("stationprofile: decision reason is required")
	ErrAccountGone    = errors.New("stationprofile: author account is not active")
)

// ReviewClaim records one restricted review atomically. Approval needs
// a valid signature, sufficient authority, a live account and the same
// operator the claim bound; denial needs a reason. Self-review,
// stale operator evidence, dead accounts and concurrent
// approve/cancel races all fail safely with nothing granted.
func ReviewClaim(ctx context.Context, ports ReviewPorts, claimID, reviewer string, approve bool, reason string, evidence ReviewEvidence) error {
	if reviewer == "" {
		return ErrReviewOperator
	}
	claim, err := ports.Claims.Claim(ctx, claimID)
	if err != nil {
		return err
	}
	if claim.AccountID == reviewer {
		return ErrReviewSelf
	}
	if !openForReview(claim.State) {
		return ErrClaimClosed
	}
	live, err := ports.AccountLive(ctx, claim.AccountID)
	if err != nil {
		return err
	}
	if !live {
		return ErrAccountGone
	}
	operatorCNPJ, found, err := ports.OperatorOf(ctx, claim.StationID)
	if err != nil {
		return err
	}
	if !found || operatorCNPJ == "" {
		return ErrReviewStale
	}
	if approve {
		if evidence.Signature.Outcome != verify.OutcomeValid {
			return ErrReviewDenied
		}
		if evidence.Authority.Outcome != authority.OutcomeSufficient {
			return ErrReviewDenied
		}
		if operatorCNPJ != claim.OperatorCNPJ {
			return ErrReviewStale
		}
		if reason == "" {
			reason = "reviewed approval"
		}
		decisionID, err := ports.NewID()
		if err != nil {
			return err
		}
		grantID, err := ports.NewID()
		if err != nil {
			return err
		}
		return ports.Decisions.DecideAtomically(ctx,
			DecisionInput{
				ID: decisionID, ClaimID: claim.ID, Reviewer: reviewer,
				Decision: "approved", Reason: reason, PolicyVersion: "profile-v1",
				OperatorCNPJ: operatorCNPJ, Scopes: claim.Scopes,
			},
			&GrantInput{
				ID: grantID, AccountID: claim.AccountID, StationID: claim.StationID,
				OperatorCNPJ: operatorCNPJ, Role: claim.Role, Scopes: claim.Scopes,
				ClaimID: claim.ID, DecisionID: decisionID,
			},
			"approved")
	}
	if reason == "" {
		return ErrVerifyReason
	}
	decisionID, err := ports.NewID()
	if err != nil {
		return err
	}
	return ports.Decisions.DecideAtomically(ctx,
		DecisionInput{
			ID: decisionID, ClaimID: claim.ID, Reviewer: reviewer,
			Decision: "denied", Reason: reason, PolicyVersion: "profile-v1",
			OperatorCNPJ: operatorCNPJ, Scopes: claim.Scopes,
		},
		nil, "denied")
}

func openForReview(state string) bool {
	return state == "draft" || state == "awaiting_proof" || state == "checking" ||
		state == "needs_information" || state == "in_review"
}
