package application

import (
	"testing"
	"time"
)

func signalNow() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }

func decentInput() BandInput {
	mean := int64(5999)
	return BandInput{
		ReceivedAt:        signalNow().Add(-time.Hour),
		ClaimedCapturedAt: signalNow().Add(-2 * time.Hour),
		StationSite:       SitePrecise,
		Claim:             &PositionClaim{AccuracyM: 25, DistanceM: 120},
		HasPhoto:          true,
		AmountMilli:       5999,
		RegionalMeanMilli: &mean,
	}
}

func hasReason(b Bands, code string) bool {
	for _, r := range b.RiskCodes {
		if r == code {
			return true
		}
	}
	return false
}

func TestDeriveBandsHappyPath(t *testing.T) {
	b := DeriveBands(decentInput(), signalNow())
	if b.Proximity != ProximityNear || b.Recency != RecencyFresh ||
		b.Capture != CaptureFresh || b.Photo != PhotoPresent ||
		b.Regional != RegionalConsistent {
		t.Errorf("bands = %+v", b)
	}
	if len(b.RiskCodes) != 0 || b.NeedsReview {
		t.Errorf("clean input flagged: %+v", b)
	}
	if b.PolicyVersion != SignalsV1 {
		t.Errorf("policy = %q", b.PolicyVersion)
	}
}

func TestUnknownStationPointNeverCertifies(t *testing.T) {
	// B-BR-014: without a precise station point — or with only a city
	// centroid — proximity is UNKNOWN even when the claimant is exact.
	for _, site := range []StationSite{SiteUnknown, SiteCentroid} {
		in := decentInput()
		in.StationSite = site
		b := DeriveBands(in, signalNow())
		if b.Proximity != ProximityUnknown {
			t.Errorf("site %q proximity = %q", site, b.Proximity)
		}
		if !hasReason(b, RiskUnknownStationPoint) || !b.NeedsReview {
			t.Errorf("site %q = %+v, want review", site, b)
		}
	}
}

func TestMissingClaimStaysUnknown(t *testing.T) {
	// Current production has no claimant-position intake: missing claims
	// stay UNKNOWN and route to review instead of passing silently.
	in := decentInput()
	in.Claim = nil
	b := DeriveBands(in, signalNow())
	if b.Proximity != ProximityUnknown || !hasReason(b, RiskMissingClaim) || !b.NeedsReview {
		t.Errorf("missing claim = %+v", b)
	}
}

func TestFarAndPoorAccuracyFlagged(t *testing.T) {
	in := decentInput()
	in.Claim = &PositionClaim{AccuracyM: 10, DistanceM: 500}
	b := DeriveBands(in, signalNow())
	if b.Proximity != ProximityFar || !hasReason(b, RiskFarFromStation) || !b.NeedsReview {
		t.Errorf("far = %+v", b)
	}
	in.Claim = &PositionClaim{AccuracyM: 250, DistanceM: 50}
	b = DeriveBands(in, signalNow())
	if b.Proximity != ProximityNear || !hasReason(b, RiskPoorAccuracy) || !b.NeedsReview {
		t.Errorf("poor accuracy = %+v", b)
	}
}

func TestOldCaptureAndStaleReceipt(t *testing.T) {
	in := decentInput()
	in.ClaimedCapturedAt = signalNow().Add(-30 * time.Hour)
	b := DeriveBands(in, signalNow())
	if b.Capture != CaptureHistorical || !hasReason(b, RiskHistoricalCapture) {
		t.Errorf("old capture = %+v", b)
	}
	in = decentInput()
	in.ReceivedAt = signalNow().Add(-50 * time.Hour)
	b = DeriveBands(in, signalNow())
	if b.Recency != RecencyStale {
		t.Errorf("old receipt = %+v", b)
	}
}

func TestPhotoAndRegionalSignals(t *testing.T) {
	in := decentInput()
	in.HasPhoto = false
	b := DeriveBands(in, signalNow())
	if b.Photo != PhotoNone || !hasReason(b, RiskNoPhoto) || !b.NeedsReview {
		t.Errorf("no photo = %+v", b)
	}
	in = decentInput()
	in.DuplicateCount = 2
	b = DeriveBands(in, signalNow())
	if b.Photo != PhotoDuplicate || !hasReason(b, RiskDuplicateImage) || b.DuplicateCount != 2 {
		t.Errorf("duplicate = %+v", b)
	}
	in = decentInput()
	deviant := int64(4000)
	in.RegionalMeanMilli = &deviant
	b = DeriveBands(in, signalNow())
	if b.Regional != RegionalDeviant || !hasReason(b, RiskRegionalDeviation) {
		t.Errorf("deviant = %+v", b)
	}
	in.RegionalMeanMilli = nil
	b = DeriveBands(in, signalNow())
	if b.Regional != RegionalUnknown || hasReason(b, RiskRegionalDeviation) {
		t.Errorf("missing benchmark = %+v", b)
	}
}

func TestSingleStrongSignalNeverCertifies(t *testing.T) {
	// A validated photo alone, with everything else unknown, still
	// routes to review: no single weak signal certifies truth.
	in := BandInput{
		ReceivedAt:  signalNow().Add(-time.Hour),
		StationSite: SiteUnknown,
		HasPhoto:    true,
		AmountMilli: 5999,
	}
	b := DeriveBands(in, signalNow())
	if !b.NeedsReview || len(b.RiskCodes) == 0 {
		t.Errorf("photo-only = %+v, want review", b)
	}
	if b.Proximity != ProximityUnknown || b.Regional != RegionalUnknown {
		t.Errorf("photo-only overclaims: %+v", b)
	}
}
