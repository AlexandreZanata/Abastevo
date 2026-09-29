package application

import (
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// Signals policy version frozen with these bands and thresholds.
// Changes require fixtures and a new version, never silent
// reinterpretation. Thresholds are conservative hypotheses from
// COMMUNITY_PRICING_SPEC, not fraud-proof claims.
const SignalsV1 = "signals-v1"

// Derived bands: proximity, recency, capture, photo and regional
// classes. UNKNOWN-style bands are honest abstentions, never verdicts;
// no band certifies truth on its own.
const (
	ProximityNear    = "NEAR"
	ProximityFar     = "FAR"
	ProximityUnknown = "UNKNOWN"

	RecencyFresh = "FRESH"
	RecencyAging = "AGING"
	RecencyStale = "STALE"

	CaptureFresh      = "FRESH"
	CaptureHistorical = "HISTORICAL"

	PhotoNone      = "NONE"
	PhotoPresent   = "PRESENT"
	PhotoDuplicate = "DUPLICATE"

	RegionalConsistent = "CONSISTENT"
	RegionalDeviant    = "DEVIANT"
	RegionalUnknown    = "UNKNOWN"
)

// Stable risk codes routing facts to review. They flag suspicion for
// moderators and consensus gates; none erases or certifies a price.
const (
	RiskMissingClaim        = "missing-claim"
	RiskUnknownStationPoint = "unknown-station-point"
	RiskPoorAccuracy        = "poor-gps-accuracy"
	RiskFarFromStation      = "far-from-station"
	RiskHistoricalCapture   = "historical-capture"
	RiskNoPhoto             = "no-photo"
	RiskDuplicateImage      = "duplicate-image"
	RiskRegionalDeviation   = "regional-deviation"
)

// Tunable experiment thresholds (spec defaults): 300 m station
// tolerance plus claimant accuracy, 100 m acceptable accuracy, 6 h /
// 48 h recency windows matching consensus weights, 25% regional band.
const (
	ProximityToleranceM    = 300.0
	MaxAcceptableAccuracyM = 100.0
	RecencyFreshHours      = 6
	RecencyAgingHours      = 48
	RegionalDeviationRatio = 0.25
)

// StationSite classifies the station side for proximity without
// importing the directory module: only precise reviewed points count,
// city centroids never do (B-BR-014). The composition root maps
// directory quality onto these values, failing closed to unknown.
type StationSite string

const (
	SiteUnknown  StationSite = "UNKNOWN"
	SiteCentroid StationSite = "CENTROID"
	SitePrecise  StationSite = "PRECISE"
)

// PositionClaim carries a claimant fix reduced to accuracy and
// station distance in metres. Exact coordinates never reach this
// policy: callers reduce them first, and only bands persist.
type PositionClaim struct {
	AccuracyM float64
	DistanceM float64
}

// BandInput gathers every derivation input explicitly. Claim is nil
// until claimant-position intake exists; missing claims stay UNKNOWN
// and route to review instead of passing silently. RegionalMeanMilli
// nil means no benchmark (ANP lag never vetoes alone).
type BandInput struct {
	ReceivedAt        time.Time
	ClaimedCapturedAt time.Time
	StationSite       StationSite
	Claim             *PositionClaim
	HasPhoto          bool
	DuplicateCount    int
	AmountMilli       int64
	RegionalMeanMilli *int64
}

// Bands is one derived signal set: classes plus review routing. It
// proves nothing about truth; consensus (P06-T04) weighs it.
type Bands struct {
	Proximity      string
	Recency        string
	Capture        string
	Photo          string
	DuplicateCount int
	Regional       string
	RiskCodes      []string
	NeedsReview    bool
	PolicyVersion  string
}

// DeriveBands reduces explicit inputs to bands and risk codes. Pure and
// deterministic: same inputs always yield the same output, independent
// of call order or storage state.
func DeriveBands(in BandInput, now time.Time) Bands {
	out := Bands{PolicyVersion: SignalsV1}
	var risks []string
	add := func(code string) {
		for _, r := range risks {
			if r == code {
				return
			}
		}
		risks = append(risks, code)
	}

	out.Proximity = deriveProximity(in, add)
	out.Recency = deriveRecency(in.ReceivedAt, now)
	if domain.CaptureFreshness(in.ClaimedCapturedAt, in.ReceivedAt) == domain.Historical {
		out.Capture = CaptureHistorical
		add(RiskHistoricalCapture)
	} else {
		out.Capture = CaptureFresh
	}
	out.Photo, out.DuplicateCount = derivePhoto(in, add)
	out.Regional = deriveRegional(in, add)

	out.RiskCodes = risks
	out.NeedsReview = len(risks) > 0
	return out
}

func deriveProximity(in BandInput, add func(string)) string {
	if in.StationSite != SitePrecise {
		add(RiskUnknownStationPoint)
		return ProximityUnknown
	}
	claim := in.Claim
	if claim == nil || claim.AccuracyM < 0 || claim.DistanceM < 0 {
		add(RiskMissingClaim)
		return ProximityUnknown
	}
	if claim.AccuracyM > MaxAcceptableAccuracyM {
		add(RiskPoorAccuracy)
	}
	if claim.DistanceM > ProximityToleranceM+claim.AccuracyM {
		add(RiskFarFromStation)
		return ProximityFar
	}
	return ProximityNear
}

func deriveRecency(received, now time.Time) string {
	age := now.Sub(received)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= RecencyFreshHours*time.Hour:
		return RecencyFresh
	case age <= RecencyAgingHours*time.Hour:
		return RecencyAging
	default:
		return RecencyStale
	}
}

func derivePhoto(in BandInput, add func(string)) (string, int) {
	if !in.HasPhoto {
		add(RiskNoPhoto)
		return PhotoNone, 0
	}
	dups := in.DuplicateCount
	if dups < 0 {
		dups = 0
	}
	if dups > 0 {
		add(RiskDuplicateImage)
		return PhotoDuplicate, dups
	}
	return PhotoPresent, 0
}

func deriveRegional(in BandInput, add func(string)) string {
	if in.RegionalMeanMilli == nil || *in.RegionalMeanMilli <= 0 || in.AmountMilli <= 0 {
		return RegionalUnknown
	}
	mean := float64(*in.RegionalMeanMilli)
	dev := float64(in.AmountMilli)/mean - 1
	if dev < 0 {
		dev = -dev
	}
	if dev > RegionalDeviationRatio {
		add(RiskRegionalDeviation)
		return RegionalDeviant
	}
	return RegionalConsistent
}
