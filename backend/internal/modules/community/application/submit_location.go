package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// ErrLocationForged refuses a location claim whose verdict contradicts
// its own metadata on server time. The observation is not stored: a
// forged VERIFIED can never become verified proximity. Honest
// non-verified states pass through without granting claims.
var ErrLocationForged = errors.New("community: location claim contradicts its metadata")

// LocationEvidence is the optional device location claim attached to
// a submission. Every field is untrusted input: the server
// recomputes the verdict, bounds freshness on its own clock, and
// discards the exact fix after deriving bands. Coordinates never
// persist and never log.
type LocationEvidence struct {
	ClaimedVerdict    string
	PermissionGranted bool
	HasFix            bool
	SourceInfoPresent bool
	Simulated         bool
	AccuracyMeters    *float64
	ClockSkewSeconds  *int64
	Manual            bool
	CapturedAt        *time.Time
	Latitude          *float64
	Longitude         *float64
}

// attachLocation verifies optional location evidence and stamps the
// observation with bands only. Nil evidence leaves the fact
// untouched (pre-location behavior); configured ports are required
// otherwise, failing closed. Forged claims refuse the submission;
// honest states store their verdict with UNKNOWN proximity unless a
// verified fix, precise station and transient fix yield a band;
// station-hopping past the teleport bound flags review without
// rejecting the fact.
func attachLocation(
	ctx context.Context,
	p Ports,
	callerToken string,
	dto SubmitDTO,
	obs domain.Observation,
	now time.Time,
) (domain.Observation, error) {
	if dto.Location == nil {
		return obs, nil
	}
	if p.Locate == nil || p.LastSite == nil || p.StationDistance == nil {
		return domain.Observation{}, errors.New("community: location intake not configured")
	}
	loc := dto.Location
	verified, err := VerifyDeviceFix(DeviceFix{
		ClaimedVerdict:    loc.ClaimedVerdict,
		PermissionGranted: loc.PermissionGranted,
		HasFix:            loc.HasFix,
		SourceInfoPresent: loc.SourceInfoPresent,
		Simulated:         loc.Simulated,
		AccuracyMeters:    loc.AccuracyMeters,
		ClockSkewSeconds:  loc.ClockSkewSeconds,
		Manual:            loc.Manual,
		CapturedAt:        loc.CapturedAt,
		Latitude:          loc.Latitude,
		Longitude:         loc.Longitude,
	}, now)
	if err != nil {
		var rej *FixRejection
		if errors.As(err, &rej) {
			return domain.Observation{}, ErrLocationForged
		}
		return domain.Observation{}, err
	}
	obs.LocationVerdict = verified.Risk.Verdict
	obs.LocationReason = verified.Risk.Reason
	obs.LocationProximity = ProximityUnknown
	if verified.AllowsClaim {
		obs.LocationProximity = submitProximity(ctx, p, obs, loc, verified.Risk)
		if obs.LocationProximity == "" {
			obs.LocationProximity = ProximityUnknown
		}
	}
	// Teleport infrastructure failures degrade to unflagged rather
	// than refusing the price fact: the flag only routes review,
	// while proximity gating (the security-critical path) already
	// resolved above and can never fabricate NEAR from an error.
	if suspect := teleportSuspect(ctx, p, callerToken, obs, now); suspect {
		obs.LocationReason = RiskTeleportSuspect
	}
	return obs, nil
}

// submitProximity derives the submit-time band for a verified fix:
// precise station plus transient fix measured in PostGIS, reduced
// through the frozen matrix. Anything missing or imprecise stays
// UNKNOWN: unknown can never become verified proximity.
func submitProximity(
	ctx context.Context,
	p Ports,
	obs domain.Observation,
	loc *LocationEvidence,
	risk FixRisk,
) string {
	if loc.Latitude == nil || loc.Longitude == nil {
		return ProximityUnknown
	}
	distanceM, site, err := p.Locate(ctx, obs.StationID, *loc.Latitude, *loc.Longitude)
	if err != nil || site != SitePrecise {
		return ProximityUnknown
	}
	accuracy := 0.0
	if loc.AccuracyMeters != nil {
		accuracy = *loc.AccuracyMeters
	}
	claim := ClaimForFix(risk, accuracy, distanceM)
	if claim == nil {
		return ProximityUnknown
	}
	return deriveProximity(BandInput{StationSite: site, Claim: claim}, func(string) {})
}

// teleportSuspect compares the current station against the
// contributor's most recent observation site: jumps faster than the
// plausible bound flag review. First observations and missing
// baselines never teleport. Baseline infrastructure failures degrade
// to unflagged (the flag routes review only; proximity already
// resolved safely above).
func teleportSuspect(
	ctx context.Context,
	p Ports,
	callerToken string,
	obs domain.Observation,
	now time.Time,
) bool {
	lastStation, lastReceived, found, err := p.LastSite(ctx, callerToken)
	if err != nil {
		return false
	}
	if !found || lastStation == "" || lastStation == obs.StationID {
		return false
	}
	distanceM, err := p.StationDistance(ctx, lastStation, obs.StationID)
	if err != nil {
		return false
	}
	elapsed := int64(now.Sub(lastReceived) / time.Second)
	return TeleportRisk(distanceM, elapsed)
}
