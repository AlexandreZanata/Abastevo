package domain

import (
	"errors"
	"strings"
	"time"
)

// Trust policy version frozen with these tiers and thresholds. Changes
// require fixtures and a new version, never silent reinterpretation.
const PolicyV1 = "trust-v1"

// Contributor tiers. Every identity starts NEW; ESTABLISHED arrives only
// through independently reviewed outcomes; BLOCKED arrives only through
// an audited abuse decision with case references.
const (
	TierNew         = "NEW"
	TierEstablished = "ESTABLISHED"
	TierBlocked     = "BLOCKED"
)

// ESTABLISHED thresholds (trust-v1 conservative hypotheses): key age at
// least 14 days with at least 10 independently reviewed successful
// observations spread across at least 5 days, and no upheld abuse
// decision. Ordinary consensus success alone never promotes: circular
// reinforcement would let colluding identities train their own
// reputation.
const (
	MinKeyAgeDays        = 14
	MinReviewedSuccesses = 10
	MinActiveDays        = 5
)

var (
	ErrInvalidDecision = errors.New("trust: invalid decision")
	ErrUnknownTier     = errors.New("trust: unknown tier")
	ErrCaseRequired    = errors.New("trust: audited case references required")
)

// DecisionParams carries one trust verdict plus its audit trail. Case
// references name the independently reviewed outcomes (moderator review
// or validated pilot labels) behind ESTABLISHED, or the abuse case
// behind BLOCKED.
type DecisionParams struct {
	ID             string
	ContributorRef string
	Tier           string
	Reason         string
	CaseRefs       []string
	OccurredAt     time.Time
	PolicyVersion  string
}

// Decision is one immutable trust verdict. Corrections and
// rehabilitations arrive as new decisions; nothing here mutates.
type Decision struct {
	ID             string
	ContributorRef string
	Tier           string
	Reason         string
	CaseRefs       []string
	OccurredAt     time.Time
	PolicyVersion  string
}

// ContributorTrustChanged is the domain event recorded at each decision.
// Downstream recomputation of affected price keys consumes it.
type ContributorTrustChanged struct {
	ContributorRef string
	Tier           string
	OccurredAt     time.Time
}

// Name derives the emitted event name.
func (e ContributorTrustChanged) Name() string { return "ContributorTrustChanged" }

// NewDecision validates and freezes one verdict: known tier, reason,
// clock, and audited case references for every tier but plain NEW.
func NewDecision(p DecisionParams) (Decision, ContributorTrustChanged, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ContributorRef) == "" ||
		strings.TrimSpace(p.Reason) == "" {
		return Decision{}, ContributorTrustChanged{}, ErrInvalidDecision
	}
	switch p.Tier {
	case TierNew, TierEstablished, TierBlocked:
	default:
		return Decision{}, ContributorTrustChanged{}, ErrUnknownTier
	}
	if p.Tier != TierNew {
		if len(p.CaseRefs) == 0 {
			return Decision{}, ContributorTrustChanged{}, ErrCaseRequired
		}
		for _, c := range p.CaseRefs {
			if strings.TrimSpace(c) == "" {
				return Decision{}, ContributorTrustChanged{}, ErrCaseRequired
			}
		}
	}
	if p.OccurredAt.IsZero() {
		return Decision{}, ContributorTrustChanged{}, ErrInvalidDecision
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	d := Decision{
		ID: p.ID, ContributorRef: p.ContributorRef, Tier: p.Tier,
		Reason: p.Reason, CaseRefs: append([]string{}, p.CaseRefs...),
		OccurredAt: p.OccurredAt, PolicyVersion: p.PolicyVersion,
	}
	return d, ContributorTrustChanged{ContributorRef: d.ContributorRef, Tier: d.Tier, OccurredAt: d.OccurredAt}, nil
}

// ReviewHistory carries exactly the four reviewed inputs promotion may
// read: key age, independently reviewed successes and their active-day
// spread, plus any upheld abuse flag. Volume, payment, email and device
// fields do not exist here by construction (see the invariance test).
type ReviewHistory struct {
	KeyAgeDays        int
	ReviewedSuccesses int
	ActiveDays        int
	UpheldAbuse       bool
}

// Evaluate maps reviewed history onto a tier recommendation. BLOCKED
// never arises here: only an audited NewDecision blocks, and upheld
// abuse merely withholds promotion.
func Evaluate(h ReviewHistory) string {
	if h.UpheldAbuse {
		return TierNew
	}
	if h.KeyAgeDays >= MinKeyAgeDays && h.ReviewedSuccesses >= MinReviewedSuccesses && h.ActiveDays >= MinActiveDays {
		return TierEstablished
	}
	return TierNew
}

// CurrentTier derives the present tier from a decision history ordered
// oldest-first: the latest verdict wins, and no history means NEW.
// Ties in time resolve by recorded order, never by silent preference.
func CurrentTier(history []Decision) string {
	if len(history) == 0 {
		return TierNew
	}
	return history[len(history)-1].Tier
}
