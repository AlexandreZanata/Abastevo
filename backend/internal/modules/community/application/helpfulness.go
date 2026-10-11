package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// ErrStaleSequence marks a vote bound to a revision the target already
// left: older-revision votes never transfer to an edited contribution.
var ErrStaleSequence = errors.New("community: helpfulness target moved on")

// ErrReviewUnavailable marks a tally read that cannot verify the review
// state: serving un-annotated counts would hide an open review, so the
// read fails closed for honest retry instead.
var ErrReviewUnavailable = errors.New("community: review state unavailable")

// HelpfulnessTarget is the resolved vote target: an existing,
// validated, visible contribution plus the immutable revision the
// caller must bind to. Unknown, hidden, deleted and ineligible targets
// refuse writes before any mutation.
type HelpfulnessTarget struct {
	ObservationID  string
	AuthorRef      string
	State          string
	Hidden         bool
	Deleted        bool
	LatestSequence int64
}

// Eligible reports whether the target accepts helpfulness writes.
func (t HelpfulnessTarget) Eligible() bool {
	return t.State == domain.StateValidated && !t.Hidden && !t.Deleted
}

// HelpfulnessStore is the owned persistence port for helpfulness votes:
// target reads plus atomic insert-or-converge writes with exact tallies.
// Atomicity (one active vote per account+contribution+revision under
// concurrency) is implemented by the store in CS02; this contract pins
// the semantics the store must hold.
type HelpfulnessStore interface {
	Target(ctx context.Context, observationID string) (HelpfulnessTarget, error)
	CastVote(ctx context.Context, rec domain.StoredVote) (stored domain.StoredVote, created bool, err error)
	RemoveVote(ctx context.Context, contributorRef, observationID string, sequence int64) (removed bool, err error)
	Tally(ctx context.Context, observationID string, sequence int64) (domain.Tally, bool, error)
	MyVote(ctx context.Context, contributorRef, observationID string, sequence int64) (domain.Choice, bool, error)
}

// ReviewTargetContribution names the moderation review target for
// community contributions. It must stay spelled exactly like the
// moderation queue's CONTRIBUTION target; the community module never
// imports the moderation domain, so the spelling is pinned by the
// annotation tests instead of the compiler.
const ReviewTargetContribution = "CONTRIBUTION"

// ReviewLookup reports whether a contribution target carries an open
// moderation review. Implementations read the moderation queue; the
// community module owns only this narrow interface, never the
// moderation tables.
type ReviewLookup interface {
	OpenReview(ctx context.Context, targetType, targetID string) (bool, error)
}

// HelpfulnessPorts wires helpfulness commands through identity and the
// owned store. Attribution always derives from the verified caller.
// CheckQuota caps write intake per fingerprint before any store work;
// denials surface for 429 mapping and never mutate.
type HelpfulnessPorts struct {
	Clock        func() time.Time
	NewID        func() (string, error)
	CheckAccount func(ctx context.Context, contributorID string) error
	CheckQuota   func(ctx context.Context, subject, operation string) (time.Duration, error)
	Store        HelpfulnessStore
	Reviews      ReviewLookup
}

// CastDTO carries one helpfulness vote: target plus immutable revision
// plus client operation identity for safe retries.
type CastDTO struct {
	ObservationID      string
	DecisionSequence   int64
	Choice             domain.Choice
	ClientSubmissionID string
}

// CastResult is the safe acknowledgment: the vote identity, exact tally
// and the caller's own choice overlay. Tallies reveal no voter list.
type CastResult struct {
	VoteID   string
	Created  bool
	Replayed bool
	Vote     domain.StoredVote
	Tally    domain.Tally
	Share    domain.Share
	MyChoice domain.Choice
}

// TallyResult is the public count snapshot plus the private owner
// overlay. Public caches must carry only the counts; MyChoice travels
// on the caller-bound read path. UnderReview annotates counts whose
// target carries an open contribution review; the frozen wording lives
// at the wire layer.
type TallyResult struct {
	Tally       domain.Tally
	Share       domain.Share
	MyChoice    *domain.Choice
	UnderReview bool
}

// Cast records one helpfulness vote on a validated, visible
// contribution. Repeats converge; choice changes rewrite in place;
// lost-response retries replay on the client submission identity;
// removals stay idempotent. Concurrent replacements cannot inflate
// totals: the store holds one live row per attribution+target+revision.
// Vote identity binds caller.Token (the stable attribution reference
// observations carry), exactly like community confirmations; the
// account gate keeps using the contributor ID.
func Cast(ctx context.Context, p HelpfulnessPorts, caller Caller, dto CastDTO) (CastResult, error) {
	if strings.TrimSpace(caller.ContributorID) == "" || strings.TrimSpace(caller.Token) == "" {
		return CastResult{}, ErrUnauthorized
	}
	if !domain.ValidChoice(dto.Choice) {
		return CastResult{}, domain.ErrVoteChoiceInvalid
	}
	if strings.TrimSpace(dto.ClientSubmissionID) == "" {
		return CastResult{}, domain.ErrInvalidVote
	}
	if err := checkHelpfulnessAccount(ctx, p, caller.ContributorID); err != nil {
		return CastResult{}, err
	}
	if p.CheckQuota == nil {
		return CastResult{}, ErrQuotaDenied
	}
	if _, err := p.CheckQuota(ctx, caller.Fingerprint, "write"); err != nil {
		return CastResult{}, err
	}
	target, err := p.Store.Target(ctx, strings.TrimSpace(dto.ObservationID))
	if err != nil {
		return CastResult{}, err
	}
	if !target.Eligible() {
		return CastResult{}, ErrIneligibleTarget
	}
	if dto.DecisionSequence != target.LatestSequence {
		return CastResult{}, ErrStaleSequence
	}
	id, err := p.NewID()
	if err != nil {
		return CastResult{}, err
	}
	now := p.Clock()
	stored, created, err := castNewVote(p, caller, target, dto, id, now)
	if err != nil {
		return CastResult{}, err
	}
	return finishCast(ctx, p, stored, created)
}

// castNewVote freezes the vote through the domain constructor (which
// owns the self-vote ban) before any store mutation.
func castNewVote(p HelpfulnessPorts, caller Caller, target HelpfulnessTarget, dto CastDTO, id string, now time.Time) (domain.StoredVote, bool, error) {
	rec, _, err := domain.NewVote(domain.VoteParams{
		ID:                 id,
		ContributorRef:     caller.Token,
		AuthorRef:          target.AuthorRef,
		ObservationID:      target.ObservationID,
		DecisionSequence:   dto.DecisionSequence,
		Choice:             dto.Choice,
		ClientSubmissionID: dto.ClientSubmissionID,
		ReceivedAt:         now,
	})
	if err != nil {
		return domain.StoredVote{}, false, err
	}
	return p.Store.CastVote(context.Background(), rec)
}

// finishCast reloads the exact tally and owner overlay after the write.
// A replayed submission returns the converged vote, never a duplicate.
func finishCast(ctx context.Context, p HelpfulnessPorts, stored domain.StoredVote, created bool) (CastResult, error) {
	tally, _, err := p.Store.Tally(ctx, stored.ObservationID, stored.DecisionSequence)
	if err != nil {
		return CastResult{}, err
	}
	share, err := tally.Share()
	if err != nil {
		return CastResult{}, err
	}
	return CastResult{
		VoteID: stored.ID, Created: created, Replayed: !created,
		Vote: stored, Tally: tally, Share: share, MyChoice: stored.Choice,
	}, nil
}

// Remove tombstones the caller's live vote on the bound revision.
// Absent votes converge as a no-op success, so retries never fail;
// history stays for audit and erasure.
func Remove(ctx context.Context, p HelpfulnessPorts, caller Caller, observationID string, sequence int64) error {
	if strings.TrimSpace(caller.ContributorID) == "" || strings.TrimSpace(caller.Token) == "" {
		return ErrUnauthorized
	}
	if err := checkHelpfulnessAccount(ctx, p, caller.ContributorID); err != nil {
		return err
	}
	target, err := p.Store.Target(ctx, strings.TrimSpace(observationID))
	if err != nil {
		return err
	}
	if !target.Eligible() {
		return ErrIneligibleTarget
	}
	if sequence != target.LatestSequence {
		return ErrStaleSequence
	}
	_, err = p.Store.RemoveVote(ctx, caller.Token, target.ObservationID, sequence)
	return err
}

// TallyFor reads the exact per-revision snapshot plus the caller's own
// choice overlay. Zero votes surface through the null share, never 0%.
func TallyFor(ctx context.Context, p HelpfulnessPorts, caller Caller, observationID string, sequence int64) (TallyResult, error) {
	target, err := p.Store.Target(ctx, strings.TrimSpace(observationID))
	if err != nil {
		return TallyResult{}, err
	}
	if !target.Eligible() {
		return TallyResult{}, ErrIneligibleTarget
	}
	if sequence != target.LatestSequence {
		return TallyResult{}, ErrStaleSequence
	}
	tally, found, err := p.Store.Tally(ctx, target.ObservationID, sequence)
	if err != nil {
		return TallyResult{}, err
	}
	if !found {
		tally = domain.Tally{ObservationID: target.ObservationID, DecisionSequence: sequence}
	}
	share, err := tally.Share()
	if err != nil {
		return TallyResult{}, err
	}
	res := TallyResult{Tally: tally, Share: share}
	if strings.TrimSpace(caller.Token) != "" {
		if choice, ok, err := p.Store.MyVote(ctx, caller.Token, target.ObservationID, sequence); err != nil {
			return TallyResult{}, err
		} else if ok {
			res.MyChoice = &choice
		}
	}
	if p.Reviews == nil {
		return res, nil
	}
	review, err := p.Reviews.OpenReview(ctx, ReviewTargetContribution, target.ObservationID)
	if err != nil {
		return TallyResult{}, ErrReviewUnavailable
	}
	res.UnderReview = review
	return res, nil
}

func checkHelpfulnessAccount(ctx context.Context, p HelpfulnessPorts, contributorID string) error {
	if p.CheckAccount == nil {
		return ErrAccountBlocked
	}
	if err := p.CheckAccount(ctx, contributorID); err != nil {
		return ErrAccountBlocked
	}
	return nil
}
