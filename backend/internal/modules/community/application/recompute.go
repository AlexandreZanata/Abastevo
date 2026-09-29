package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// RecomputePorts declares every input for price recomputation: the
// eligible-anchor scan for one exact key, confirmations by anchor,
// contributor tiers defaulting to NEW, photo signals by evidence, and
// the moderation flag (always false until the P07 case queue lands).
// Unavailable inputs fail the run; they never count as verified.
type RecomputePorts struct {
	Clock         func() time.Time
	Store         RecomputeStore
	Anchors       func(ctx context.Context, key domain.PriceKey, cutoff time.Time) ([]domain.Observation, error)
	Confirmations func(ctx context.Context, anchorIDs []string) ([]ConfirmationVote, error)
	Tiers         func(ctx context.Context, refs []string) (map[string]string, error)
	// Photo resolves one anchor's photographic standing: server-verified
	// bytes plus acceptable proximity from the anchor's own signals.
	// Missing signals fail closed to no-proximity, never to verified.
	Photo      func(ctx context.Context, anchor domain.Observation) (PhotoInfo, error)
	Moderation func(ctx context.Context, key domain.PriceKey) (bool, error)
}

// ConfirmationVote is one recorded support vote for recomputation input.
type ConfirmationVote struct {
	ID             string
	ObservationID  string
	ContributorRef string
	ReceivedAt     time.Time
}

// PhotoInfo reports one anchor's photographic standing: validated
// server-side bytes with acceptable proximity, plus the media key for
// exact-duplicate independence. Absent evidence reports all-false.
type PhotoInfo struct {
	Found       bool
	Validated   bool
	ProximityOK bool
	MediaKey    string
}

// PolicyConfigVersion tracks the tunable thresholds alongside the
// algorithm: threshold changes ship a new consensus version with
// fixtures, so both columns move together by design.
const PolicyConfigVersion = domain.ConsensusV1

// PriceKeyString renders one price key for advisory locks, audit input
// keys and job dedupe. The pipe separator never appears in UUIDs or
// controlled vocabularies.
func PriceKeyString(key domain.PriceKey) string {
	return key.StationID + "|" + key.Product + "|" + key.Unit + "|" +
		key.ConditionKind + "|" + key.Qualifier
}

// RecomputeStore persists one projection version atomically under the
// price-key lock.
type RecomputeStore interface {
	Observation(ctx context.Context, id string) (domain.Observation, error)
	SaveProjection(ctx context.Context, key domain.PriceKey, result domain.Result, rep string, anchorAt, cutoff time.Time, next *time.Time, policyVersion string) error
}

// Recompute rebuilds one price projection from a trigger observation:
// the key derives from the trigger, eligible votes assemble through
// ports, Compute decides deterministically, and the versioned outcome
// persists with its audit inputs. A confirmation affirming a stale or
// otherwise ineligible anchor drops with that anchor instead of
// reviving it.
func Recompute(ctx context.Context, p RecomputePorts, observationID string) (domain.Result, error) {
	trigger, err := p.Store.Observation(ctx, observationID)
	if err != nil {
		return domain.Result{}, err
	}
	return RecomputeKey(ctx, p, priceKeyOf(trigger))
}

// RecomputeKey rebuilds the projection for an explicit price key: the
// boundary sweeper path, sharing the trigger core below.
func RecomputeKey(ctx context.Context, p RecomputePorts, key domain.PriceKey) (domain.Result, error) {
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	cutoff := now.Add(-domain.ConsensusWindow)
	anchors, err := p.Anchors(ctx, key, cutoff)
	if err != nil {
		return domain.Result{}, err
	}
	anchorIDs := make([]string, 0, len(anchors))
	anchorByID := make(map[string]domain.Observation, len(anchors))
	for _, a := range anchors {
		anchorIDs = append(anchorIDs, a.ID)
		anchorByID[a.ID] = a
	}
	confs, err := p.Confirmations(ctx, anchorIDs)
	if err != nil {
		return domain.Result{}, err
	}
	refs := map[string]bool{}
	for _, a := range anchors {
		refs[a.ContributorRef] = true
	}
	for _, c := range confs {
		refs[c.ContributorRef] = true
	}
	refList := make([]string, 0, len(refs))
	for r := range refs {
		refList = append(refList, r)
	}
	tiers, err := p.Tiers(ctx, refList)
	if err != nil {
		return domain.Result{}, err
	}
	moderation, err := p.Moderation(ctx, key)
	if err != nil {
		return domain.Result{}, err
	}
	votes := make([]domain.Vote, 0, len(anchors)+len(confs))
	for _, a := range anchors {
		info, err := p.Photo(ctx, a)
		if err != nil {
			return domain.Result{}, err
		}
		photo, prox, media := info.Validated, info.ProximityOK, info.MediaKey
		votes = append(votes, domain.Vote{
			VoterRef: a.ContributorRef, ObservationID: a.ID,
			EventID: "obs:" + a.ID, AuthorRef: a.ContributorRef,
			AmountMilli: a.AmountMilli,
			ReceivedAt:  a.ReceivedAt, AnchorReceivedAt: a.ReceivedAt,
			TrustTier:      tierOf(tiers, a.ContributorRef),
			PhotoValidated: photo, PhotoProximityOK: prox, MediaKey: media,
			StationID: a.StationID, Product: a.Product, Unit: a.Unit,
			ConditionKind: a.ConditionKind, Qualifier: a.QualifierKey,
		})
	}
	for _, c := range confs {
		anchor, ok := anchorByID[c.ObservationID]
		if !ok {
			continue
		}
		votes = append(votes, domain.Vote{
			VoterRef: c.ContributorRef, ObservationID: c.ObservationID,
			ConfirmationID: c.ID, EventID: "conf:" + c.ID, AuthorRef: anchor.ContributorRef,
			AmountMilli: anchor.AmountMilli,
			ReceivedAt:  c.ReceivedAt, AnchorReceivedAt: anchor.ReceivedAt,
			TrustTier: tierOf(tiers, c.ContributorRef),
			StationID: anchor.StationID, Product: anchor.Product, Unit: anchor.Unit,
			ConditionKind: anchor.ConditionKind, Qualifier: anchor.QualifierKey,
		})
	}
	result, err := domain.Compute(key, votes, now, domain.Options{Version: domain.ConsensusV1, ModerationOpen: moderation})
	if err != nil {
		return domain.Result{}, err
	}
	rep := representative(votes, result)
	next := NextRecomputeAt(votes, result, now)
	if err := p.Store.SaveProjection(ctx, key, result, rep, freshestAnchor(votes), cutoff, next, PolicyConfigVersion); err != nil {
		return domain.Result{}, err
	}
	return result, nil
}

// freshestAnchor returns the newest direct anchor receipt across the
// considered votes, or zero time when none exist. Stale anchors are
// older than any eligible one by construction of the 48 h window, so
// the maximum always reflects eligible input for priced projections.
func freshestAnchor(votes []domain.Vote) time.Time {
	var best time.Time
	for _, v := range votes {
		if v.ConfirmationID != "" {
			continue
		}
		if best.IsZero() || v.AnchorReceivedAt.After(best) {
			best = v.AnchorReceivedAt
		}
	}
	return best
}

func priceKeyOf(o domain.Observation) domain.PriceKey {
	return domain.PriceKey{
		StationID: o.StationID, Product: o.Product, Unit: o.Unit,
		ConditionKind: o.ConditionKind, Qualifier: o.QualifierKey,
	}
}

func tierOf(tiers map[string]string, ref string) string {
	if t, ok := tiers[ref]; ok && t != "" {
		return t
	}
	return "NEW"
}

// representative picks the newest winner anchor for confirmation and
// dispute targeting: it always matches the displayed amount.
func representative(votes []domain.Vote, result domain.Result) string {
	if result.Verdict != domain.VerdictPrice {
		return ""
	}
	winners := map[string]bool{}
	for _, id := range result.WinnerObservationIDs {
		winners[id] = true
	}
	best := ""
	var bestAt time.Time
	for _, v := range votes {
		if v.ConfirmationID != "" || !winners[v.ObservationID] {
			continue
		}
		if best == "" || v.AnchorReceivedAt.After(bestAt) {
			best, bestAt = v.ObservationID, v.AnchorReceivedAt
		}
	}
	return best
}

// NextRecomputeAt returns the earliest future weight or expiry boundary
// after now across the considered votes: receipt plus 6, 24 and 48
// hours, plus the projection expiry. Stale projections, or sets with no
// future boundary, schedule nothing: fresh writes retrigger instead, so
// dead keys never busy-loop the scheduler.
func NextRecomputeAt(votes []domain.Vote, result domain.Result, now time.Time) *time.Time {
	if result.Freshness == domain.FreshStale || result.Freshness == domain.FreshUnknown {
		return nil
	}
	var best *time.Time
	consider := func(t time.Time) {
		if t.After(now) && (best == nil || t.Before(*best)) {
			cp := t
			best = &cp
		}
	}
	for _, v := range votes {
		consider(v.ReceivedAt.Add(6 * time.Hour))
		consider(v.ReceivedAt.Add(24 * time.Hour))
		consider(v.ReceivedAt.Add(48 * time.Hour))
	}
	if !result.ExpiresAt.IsZero() {
		consider(result.ExpiresAt)
	}
	return best
}
