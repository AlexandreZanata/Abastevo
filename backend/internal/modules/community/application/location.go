package application

// Location risk contract (P16-T01, B-BR-L01…L04). Native OS source
// signals (Android LocationCompat.isMock / iOS
// CLLocationSourceInformation.isSimulatedBySoftware) and fix metadata
// reduce to one frozen verdict before any proximity claim exists.
// This layer detects platform-marked simulation and misses signal
// honestly; it never promises universal spoof-proofing, and a forged
// client flag cannot manufacture source info that the OS did not
// provide. Policy version changes require fixtures, never silent
// reinterpretation.

// LocationV1 freezes the verdict vocabulary shared with the native
// clients through contracts/testdata/location/risk-v1.json.
const LocationV1 = "location-v1"

// Fix verdicts. VERIFIED is the only verdict that may authorize a
// location-dependent claim; every other verdict preserves browsing,
// manual lookup and offline use without granting proximity.
const (
	FixVerified  = "VERIFIED"
	FixDegraded  = "DEGRADED"
	FixManual    = "MANUAL"
	FixDenied    = "DENIED"
	FixSimulated = "SIMULATED"
	FixUnknown   = "UNKNOWN"
)

// Stable reason codes. UI wording stays outside; these codes travel in
// fixtures, audit and risk routing. No coordinate, key or URL ever
// belongs in a code.
const (
	FixReasonSimulatedSource  = "simulated-source"
	FixReasonPermissionDenied = "permission-denied"
	FixReasonNoFix            = "no-fix"
	FixReasonSourceMissing    = "source-info-missing"
	FixReasonStaleFix         = "stale-fix"
	FixReasonClockAnomaly     = "clock-anomaly"
	FixReasonCoarseAccuracy   = "coarse-accuracy"
	FixReasonManualEntry      = "manual-entry"
)

// Frozen tunables (conservative hypotheses, not fraud-proof claims):
// a fix must be at most 2 minutes old, within ±5 minutes of claimed
// time, and at most 100 m accuracy to carry a claim. The accuracy
// bound mirrors MaxAcceptableAccuracyM in the signals policy.
const (
	MaxFixAgeSeconds    = 120
	MaxClockSkewSeconds = 300
	MaxClaimAccuracyM   = 100.0
)

// Freshness and accuracy bands persist instead of coordinates
// (SECURITY_PRIVACY: bands only, never raw fixes).
const (
	FixFreshnessFresh   = "FRESH"
	FixFreshnessStale   = "STALE"
	FixFreshnessUnknown = "UNKNOWN"

	FixAccuracyAccurate = "ACCURATE"
	FixAccuracyCoarse   = "COARSE"
	FixAccuracyUnknown  = "UNKNOWN"
)

// FixInput is one device location signal reduced to non-identifying
// values. SourceInfoPresent is OS-provided truth (the mock/simulated
// flag existed); Simulated is meaningful only then. Nil pointers mark
// missing data, never zero defaults.
type FixInput struct {
	PermissionGranted bool
	HasFix            bool
	SourceInfoPresent bool
	Simulated         bool
	AccuracyMeters    *float64
	FixAgeSeconds     *int64
	ClockSkewSeconds  *int64
	Manual            bool
}

// FixRisk is the frozen classification: verdict plus bands and the
// single claim gate. AllowsClaim is true only for VERIFIED; an UNKNOWN
// can never become verified proximity (acceptance for P16-T01).
type FixRisk struct {
	Verdict       string
	Reason        string
	Freshness     string
	Accuracy      string
	AllowsClaim   bool
	PolicyVersion string
}

// ClassifyFix reduces one signal to its verdict. Precedence is
// deliberate: manual entry (no device fix at all) beats permission
// denial; permission denial beats signal quality; missing signal
// data beats a claimed simulated flag; OS source info gates the
// simulated flag itself. Pure and deterministic.
func ClassifyFix(in FixInput) FixRisk {
	out := FixRisk{PolicyVersion: LocationV1}
	out.Freshness = freshnessBand(in.FixAgeSeconds)
	out.Accuracy = accuracyBand(in.AccuracyMeters)

	switch {
	case in.Manual:
		out.Verdict, out.Reason = FixManual, FixReasonManualEntry
	case !in.PermissionGranted:
		out.Verdict, out.Reason = FixDenied, FixReasonPermissionDenied
	case !in.HasFix || in.AccuracyMeters == nil || in.FixAgeSeconds == nil:
		// A fix missing any usable component is not a fix: no signal
		// beats a synthesized one.
		out.Verdict, out.Reason = FixUnknown, FixReasonNoFix
	case !in.SourceInfoPresent:
		// The OS did not provide source information: a client-claimed
		// "not simulated" cannot upgrade this to VERIFIED.
		out.Verdict, out.Reason = FixUnknown, FixReasonSourceMissing
	case in.Simulated:
		out.Verdict, out.Reason = FixSimulated, FixReasonSimulatedSource
	case *in.FixAgeSeconds < 0:
		out.Verdict, out.Reason = FixUnknown, FixReasonClockAnomaly
	case in.ClockSkewSeconds != nil && absInt64(*in.ClockSkewSeconds) > MaxClockSkewSeconds:
		out.Verdict, out.Reason = FixUnknown, FixReasonClockAnomaly
	case *in.FixAgeSeconds > MaxFixAgeSeconds:
		out.Verdict, out.Reason = FixUnknown, FixReasonStaleFix
	case *in.AccuracyMeters > MaxClaimAccuracyM:
		out.Verdict, out.Reason = FixDegraded, FixReasonCoarseAccuracy
	default:
		out.Verdict, out.Reason = FixVerified, ""
	}
	out.AllowsClaim = out.Verdict == FixVerified
	return out
}

// ClaimForFix reduces a classified fix to a position claim: only a
// VERIFIED fix may carry proximity, so unknown, degraded, manual,
// denied and simulated inputs can never become verified proximity.
// Returns nil for every non-verified verdict.
func ClaimForFix(risk FixRisk, accuracyM, distanceM float64) *PositionClaim {
	if !risk.AllowsClaim {
		return nil
	}
	return &PositionClaim{AccuracyM: accuracyM, DistanceM: distanceM}
}

func freshnessBand(age *int64) string {
	if age == nil || *age < 0 {
		return FixFreshnessUnknown
	}
	if *age > MaxFixAgeSeconds {
		return FixFreshnessStale
	}
	return FixFreshnessFresh
}

func accuracyBand(accuracy *float64) string {
	if accuracy == nil {
		return FixAccuracyUnknown
	}
	if *accuracy > MaxClaimAccuracyM {
		return FixAccuracyCoarse
	}
	return FixAccuracyAccurate
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
