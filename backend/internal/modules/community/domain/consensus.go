package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// Consensus algorithm version frozen with these weights and gates.
// Changes require fixtures and a new version, never silent
// reinterpretation: unknown versions fail loudly for replay safety.
const ConsensusV1 = "consensus-v1"

// ConsensusWindow bounds input eligibility and projection expiry: 48 h.
// Recency factors by vote age at evaluation time T.
const (
	ConsensusWindow   = 48 * time.Hour
	recencyFreshHours = 6
	recencyAgingHours = 48
	windowHours       = 48
)

// Trust factors by contributor tier. Blocked contributors never weigh:
// the loader excludes them, and Compute skips them defensively.
const (
	trustNew         = 1
	trustEstablished = 2
)

// Confidence gates and conflict ratios (spec tunables).
const (
	runnerConflictRatio  = 60
	runnerHighRatio      = 25
	mediumSupporters     = 2
	highSupporters       = 3
	highEstablished      = 2
	highPhotos           = 2
	highPhotoWindowHours = 6
)

// Verdicts and confidence/freshness classes.
const (
	VerdictPrice    = "PRICE"
	VerdictUnknown  = "UNKNOWN"
	VerdictDisputed = "DISPUTED"

	ConfLow    = "LOW"
	ConfMedium = "MEDIUM"
	ConfHigh   = "HIGH"

	// Freshness classes mirror the signals vocabulary by value
	// ("FRESH"/"AGING"); the strings are duplicated (not imported)
	// because domain owns its output contract.
	FreshFresh   = "FRESH"
	FreshAging   = "AGING"
	FreshStale   = "STALE"
	FreshUnknown = "UNKNOWN"
)

// Stable explanation codes. Public output carries counts and these
// codes only: never contributor IDs, thresholds or media keys.
const (
	ReasonNoEligibleVotes    = "no-eligible-votes"
	ReasonConfirmationsAlone = "confirmations-without-anchor"
	ReasonSelfConfirmation   = "self-confirmation-excluded"
	ReasonBlockedExcluded    = "blocked-voter-excluded"
	ReasonStaleVote          = "stale-vote-excluded"
	ReasonRunnerConflict     = "runner-up-conflict"
	ReasonOpenModeration     = "open-moderation-case"
	ReasonPhotoGate          = "photo-gate"
)

var (
	ErrBadConsensusInput = errors.New("community: invalid consensus input")
	ErrBadPriceKey       = errors.New("community: invalid price key")
	ErrUnknownVersion    = errors.New("community: unknown consensus version")
)

// PriceKey identifies one independent price computation: station,
// product, unit and full condition travel together, so APP qualifiers
// never mix with generic prices.
type PriceKey struct {
	StationID     string `json:"station_id"`
	Product       string `json:"fuel_product"`
	Unit          string `json:"unit"`
	ConditionKind string `json:"condition_kind"`
	Qualifier     string `json:"qualifier_id"`
}

// Vote is one eligible input fact: a direct observation anchor or a
// confirmation support. The loader resolves linked contributor keys,
// removes revoked and quarantined votes and feeds one price key;
// Compute additionally reduces to one latest vote per contributor,
// skips blocked voters and stale anchors defensively, and refuses any
// vote outside the evaluated key instead of mixing qualifiers.
// Amounts, identities and media travel here; payment, entitlement and
// ANP fields do not exist by construction.
type Vote struct {
	VoterRef         string    `json:"voter"`
	ObservationID    string    `json:"observation_id"`
	ConfirmationID   string    `json:"confirmation_id"`
	EventID          string    `json:"event_id"`
	AuthorRef        string    `json:"author"`
	AmountMilli      int64     `json:"amount_milli"`
	ReceivedAt       time.Time `json:"received_at"`
	AnchorReceivedAt time.Time `json:"anchor_received_at"`
	TrustTier        string    `json:"trust"`
	PhotoValidated   bool      `json:"photo_validated"`
	PhotoProximityOK bool      `json:"photo_proximity_ok"`
	MediaKey         string    `json:"media_key"`
	StationID        string    `json:"station_id"`
	Product          string    `json:"fuel_product"`
	Unit             string    `json:"unit"`
	ConditionKind    string    `json:"condition_kind"`
	Qualifier        string    `json:"qualifier_id"`
}

// Options carries the evaluation envelope: the policy version plus the
// unresolved-substantiated-moderation flag from the case queue.
type Options struct {
	Version        string
	ModerationOpen bool
}

// Result is one deterministic projection: verdict plus explanatory
// counts, freshness and reason codes. Weights, thresholds and
// contributor identities never leave this package's callers. Winner
// observation and confirmation IDs feed the restricted audit inputs;
// they stay out of public responses.
type Result struct {
	Verdict               string    `json:"verdict"`
	AmountMilli           int64     `json:"amount_milli"`
	Confidence            string    `json:"confidence"`
	Freshness             string    `json:"freshness"`
	ExpiresAt             time.Time `json:"expires_at"`
	ComputedAt            time.Time `json:"computed_at"`
	Supporters            int       `json:"supporters"`
	Confirmations         int       `json:"confirmations"`
	Groups                int       `json:"groups"`
	Reasons               []string  `json:"reasons"`
	WinnerObservationIDs  []string  `json:"winner_observation_ids"`
	WinnerConfirmationIDs []string  `json:"winner_confirmation_ids"`
	AlgorithmVersion      string    `json:"algorithm_version"`
}

// Compute evaluates one price key at injected UTC time T through
// consensus-v1: eligibility, per-contributor reduction, exact-amount
// grouping, deterministic ordering, conflict detection and confidence
// gates. Same facts, clock and version always produce the same result,
// independent of input order.
func Compute(key PriceKey, votes []Vote, now time.Time, opts Options) (Result, error) {
	if opts.Version != ConsensusV1 {
		return Result{}, ErrUnknownVersion
	}
	if strings.TrimSpace(key.StationID) == "" || strings.TrimSpace(key.Product) == "" ||
		strings.TrimSpace(key.Unit) == "" {
		return Result{}, ErrBadPriceKey
	}
	if now.IsZero() {
		return Result{}, ErrBadConsensusInput
	}
	eligible, reasons, err := eligibleVotes(key, votes, now)
	if err != nil {
		return Result{}, err
	}
	if len(eligible) == 0 {
		return Result{
			Verdict: VerdictUnknown, Freshness: FreshUnknown,
			ComputedAt: now, Reasons: reasons, AlgorithmVersion: ConsensusV1,
		}, nil
	}
	reduced := reducePerContributor(eligible)
	groups := groupByAmount(reduced)
	if len(groups) == 0 {
		return Result{
			Verdict: VerdictUnknown, Freshness: FreshUnknown,
			ComputedAt: now, Reasons: append(reasons, ReasonNoEligibleVotes),
			AlgorithmVersion: ConsensusV1,
		}, nil
	}
	anchors := directAnchors(reduced)
	if len(anchors) == 0 {
		return Result{
			Verdict: VerdictUnknown, Freshness: FreshUnknown,
			ComputedAt: now, Reasons: append(reasons, ReasonConfirmationsAlone),
			AlgorithmVersion: ConsensusV1,
		}, nil
	}
	freshest := newestAnchor(anchors)
	expires := freshest.Add(windowHours * time.Hour)
	freshness := freshnessAt(now, freshest)

	if opts.ModerationOpen {
		return Result{
			Verdict: VerdictDisputed, Freshness: freshness, ExpiresAt: expires,
			ComputedAt: now, Groups: len(groups),
			Reasons:          append(reasons, ReasonOpenModeration),
			AlgorithmVersion: ConsensusV1,
		}, nil
	}

	leader, runner := groups[0], nilGroup()
	hasRunner := len(groups) > 1
	if hasRunner {
		runner = groups[1]
	}
	if hasRunner && runner.supporters >= 2 && leader.supporters >= 2 &&
		runner.weight*100 >= leader.weight*runnerConflictRatio {
		return Result{
			Verdict: VerdictDisputed, Freshness: freshness, ExpiresAt: expires,
			ComputedAt: now, Groups: len(groups),
			Reasons:          append(reasons, ReasonRunnerConflict),
			AlgorithmVersion: ConsensusV1,
		}, nil
	}

	confidence, confReasons := confidenceFor(leader, runner, hasRunner, now)
	out := Result{
		Verdict: VerdictPrice, AmountMilli: leader.amount,
		Confidence: confidence, Freshness: freshness, ExpiresAt: expires,
		ComputedAt: now, Supporters: leader.supporters,
		Confirmations: leader.confirmations, Groups: len(groups),
		Reasons:               append(reasons, confReasons...),
		WinnerObservationIDs:  leader.sortedAnchorIDs(),
		WinnerConfirmationIDs: leader.sortedConfirmationIDs(),
		AlgorithmVersion:      ConsensusV1,
	}
	return out, nil
}

// eligibleVote is one validated vote with its computed weight.
type eligibleVote struct {
	vote   Vote
	weight int64
	direct bool
}

// eligibleVotes validates, filters and weighs raw inputs: key match,
// well-formed fields, blocked-voter and stale-anchor exclusion, and
// self-confirmation refusal. Malformed inputs and foreign keys fail
// loudly; exclusions return explanatory reason codes.
func eligibleVotes(key PriceKey, votes []Vote, now time.Time) ([]eligibleVote, []string, error) {
	var out []eligibleVote
	var reasons []string
	addReason := func(code string) {
		for _, r := range reasons {
			if r == code {
				return
			}
		}
		reasons = append(reasons, code)
	}
	for _, v := range votes {
		if strings.TrimSpace(v.VoterRef) == "" || strings.TrimSpace(v.ObservationID) == "" ||
			strings.TrimSpace(v.EventID) == "" || v.AmountMilli < 1 ||
			v.ReceivedAt.IsZero() || v.AnchorReceivedAt.IsZero() {
			return nil, nil, ErrBadConsensusInput
		}
		if v.StationID != key.StationID || v.Product != key.Product || v.Unit != key.Unit ||
			v.ConditionKind != key.ConditionKind || v.Qualifier != key.Qualifier {
			return nil, nil, ErrBadPriceKey
		}
		if v.TrustTier == "BLOCKED" {
			addReason(ReasonBlockedExcluded)
			continue
		}
		if v.TrustTier != "NEW" && v.TrustTier != "ESTABLISHED" {
			return nil, nil, ErrBadConsensusInput
		}
		if now.Sub(v.ReceivedAt) > windowHours*time.Hour ||
			now.Sub(v.AnchorReceivedAt) > windowHours*time.Hour {
			addReason(ReasonStaleVote)
			continue
		}
		if v.ConfirmationID != "" && v.VoterRef == v.AuthorRef {
			addReason(ReasonSelfConfirmation)
			continue
		}
		out = append(out, eligibleVote{
			vote: v, weight: recencyFactor(now.Sub(v.ReceivedAt)) * trustFactor(v.TrustTier),
			direct: v.ConfirmationID == "",
		})
	}
	return out, reasons, nil
}

func recencyFactor(age time.Duration) int64 {
	if age < 0 {
		age = 0
	}
	switch {
	case age <= recencyFreshHours*time.Hour:
		return 4
	case age <= 24*time.Hour:
		return 2
	default:
		return 1
	}
}

func trustFactor(tier string) int64 {
	if tier == "ESTABLISHED" {
		return trustEstablished
	}
	return trustNew
}

// reducePerContributor keeps one latest eligible vote per contributor
// ordered by receipt then event ID, so retries and superseded amounts
// never amplify: a contributor switching amount moves only its own vote.
func reducePerContributor(votes []eligibleVote) []eligibleVote {
	byVoter := map[string]eligibleVote{}
	for _, v := range votes {
		prev, ok := byVoter[v.vote.VoterRef]
		if !ok || v.vote.ReceivedAt.After(prev.vote.ReceivedAt) ||
			(v.vote.ReceivedAt.Equal(prev.vote.ReceivedAt) && v.vote.EventID > prev.vote.EventID) {
			byVoter[v.vote.VoterRef] = v
		}
	}
	out := make([]eligibleVote, 0, len(byVoter))
	for _, v := range byVoter {
		out = append(out, v)
	}
	// Deterministic order before grouping: voter, then event.
	sort.Slice(out, func(i, j int) bool {
		if out[i].vote.VoterRef != out[j].vote.VoterRef {
			return out[i].vote.VoterRef < out[j].vote.VoterRef
		}
		return out[i].vote.EventID < out[j].vote.EventID
	})
	return out
}

// amountGroup aggregates one exact milli-BRL amount: total weight,
// independent supporters, confirmations, newest anchor receipt,
// smallest anchor observation UUID, established count, the distinct
// validated photo anchors with proximity, and the winner fact IDs for
// restricted audit inputs.
type amountGroup struct {
	amount          int64
	weight          int64
	supporters      int
	confirmations   int
	newestAnchor    time.Time
	minObservation  string
	established     int
	photos          []photoAnchor
	anchors         []string
	confirmationIDs []string
}

// sortedAnchorIDs returns the winner anchor IDs in stable sorted order.
func (g amountGroup) sortedAnchorIDs() []string {
	out := append([]string{}, g.anchors...)
	sort.Strings(out)
	return out
}

// sortedConfirmationIDs returns the winner confirmation IDs in stable order.
func (g amountGroup) sortedConfirmationIDs() []string {
	out := append([]string{}, g.confirmationIDs...)
	sort.Strings(out)
	return out
}

type photoAnchor struct {
	mediaKey   string
	receivedAt time.Time
}

// groupByAmount clusters reduced votes by exact amount and sorts groups
// by total weight, independent contributors, newest anchor receipt,
// amount ascending, then smallest observation UUID. No fuzzy clusters
// in v1; the ordering never bypasses the conflict rule.
func groupByAmount(votes []eligibleVote) []amountGroup {
	byAmount := map[int64]*amountGroup{}
	var order []int64
	for _, v := range votes {
		g, ok := byAmount[v.vote.AmountMilli]
		if !ok {
			g = &amountGroup{amount: v.vote.AmountMilli, minObservation: v.vote.ObservationID}
			byAmount[v.vote.AmountMilli] = g
			order = append(order, v.vote.AmountMilli)
		}
		g.weight += v.weight
		g.supporters++
		if v.vote.ConfirmationID != "" {
			g.confirmations++
			g.confirmationIDs = append(g.confirmationIDs, v.vote.ConfirmationID)
		} else {
			g.anchors = append(g.anchors, v.vote.ObservationID)
		}
		if v.vote.AnchorReceivedAt.After(g.newestAnchor) {
			g.newestAnchor = v.vote.AnchorReceivedAt
		}
		if v.vote.ObservationID < g.minObservation {
			g.minObservation = v.vote.ObservationID
		}
		if v.vote.TrustTier == "ESTABLISHED" {
			g.established++
		}
		if v.direct && v.vote.PhotoValidated && v.vote.PhotoProximityOK {
			g.photos = append(g.photos, photoAnchor{mediaKey: v.vote.MediaKey, receivedAt: v.vote.AnchorReceivedAt})
		}
	}
	_ = order
	groups := make([]amountGroup, 0, len(byAmount))
	for _, g := range byAmount {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].weight != groups[j].weight {
			return groups[i].weight > groups[j].weight
		}
		if groups[i].supporters != groups[j].supporters {
			return groups[i].supporters > groups[j].supporters
		}
		if !groups[i].newestAnchor.Equal(groups[j].newestAnchor) {
			return groups[i].newestAnchor.After(groups[j].newestAnchor)
		}
		if groups[i].amount != groups[j].amount {
			return groups[i].amount < groups[j].amount
		}
		return groups[i].minObservation < groups[j].minObservation
	})
	return groups
}

func nilGroup() amountGroup { return amountGroup{} }

// directAnchors returns the anchor receipts of eligible direct votes
// for expiry and freshness.
func directAnchors(votes []eligibleVote) []time.Time {
	var out []time.Time
	for _, v := range votes {
		if v.direct {
			out = append(out, v.vote.AnchorReceivedAt)
		}
	}
	return out
}

func newestAnchor(anchors []time.Time) time.Time {
	best := anchors[0]
	for _, a := range anchors[1:] {
		if a.After(best) {
			best = a
		}
	}
	return best
}

func freshnessAt(now, freshest time.Time) string {
	age := now.Sub(freshest)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= recencyFreshHours*time.Hour:
		return FreshFresh
	case age <= windowHours*time.Hour:
		return FreshAging
	default:
		return FreshStale
	}
}

// confidenceFor applies the MEDIUM and HIGH gates to the leading group:
// unique validated photos with acceptable proximity, established depth
// and a weak runner-up. Anything below MEDIUM stays an honest LOW.
func confidenceFor(leader, runner amountGroup, hasRunner bool, now time.Time) (string, []string) {
	if leader.supporters < mediumSupporters || len(distinctPhotos(leader.photos, now, 0)) < 1 {
		return ConfLow, []string{ReasonPhotoGate}
	}
	if leader.supporters < highSupporters || leader.established < highEstablished ||
		len(distinctPhotos(leader.photos, now, highPhotoWindowHours*time.Hour)) < highPhotos {
		return ConfMedium, nil
	}
	if hasRunner && !(runner.weight*100 < leader.weight*runnerHighRatio) {
		return ConfMedium, nil
	}
	return ConfHigh, nil
}

// distinctPhotos counts validated photo anchors with acceptable
// proximity by distinct media key, optionally bounded to a freshness
// window. Exact-media duplicates never establish independence.
func distinctPhotos(photos []photoAnchor, now time.Time, within time.Duration) []photoAnchor {
	seen := map[string]bool{}
	var out []photoAnchor
	for _, p := range photos {
		if p.mediaKey == "" || seen[p.mediaKey] {
			continue
		}
		if within > 0 && now.Sub(p.receivedAt) > within {
			continue
		}
		seen[p.mediaKey] = true
		out = append(out, p)
	}
	return out
}
