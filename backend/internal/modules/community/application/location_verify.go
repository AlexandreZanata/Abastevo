package application

import (
	"errors"
	"fmt"
	"time"
)

// Server-side location verification (P16-T03, B-BR-L03). Device risk
// claims arrive as untrusted inputs: the server recomputes the frozen
// verdict from the claimed metadata on the server clock and refuses
// anything that does not match. A forged isMock=false cannot promote
// itself — without OS source info the recompute is UNKNOWN, and a
// VERIFIED claim over inconsistent/stale metadata refuses instead of
// downgrading. Exact coordinates, when present for proximity, are
// transient: reduced immediately, never persisted, never logged.

// RiskForgedClaim is the stable refusal when a claimed verdict does
// not match the server recompute. Fixed vocabulary only; no
// coordinates, keys or URLs ever travel in it.
const RiskForgedClaim = "forged-location-claim"

// MaxPlausibleSpeedMS bounds contributor movement between observations
// (100 m/s = 360 km/h, conservative for ground travel). Faster
// station-to-station jumps flag teleport suspicion for review; they
// never auto-reject a structurally valid observation.
const MaxPlausibleSpeedMS = 100.0

// ErrFixRefused marks a permanently refused device fix. Use errors.As
// for *FixRejection to read the stable reason.
var ErrFixRefused = errors.New("community: device fix refused")

// FixRejection is the permanent refusal with a stable reason code.
// Any other error from VerifyDeviceFix is transient or programmer
// error (bad receipt time, unknown verdict), never a silent pass.
type FixRejection struct {
	Reason string
}

func (e *FixRejection) Error() string {
	return fmt.Sprintf("%s: %s", ErrFixRefused, e.Reason)
}

// Unwrap matches ErrFixRefused so callers map by errors.Is.
func (e *FixRejection) Unwrap() error { return ErrFixRefused }

// DeviceFix is one untrusted device claim plus its optional transient
// exact fix. Latitude/Longitude exist only to derive proximity in the
// same call; they must not be stored or logged by any caller. There
// is deliberately no client-claimed age field: freshness derives from
// the server receipt clock alone, and a claim without a capture
// instant can never verify (missing time refuses VERIFIED).
type DeviceFix struct {
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

// VerifiedFix is the server verdict: the recomputed risk plus whether
// the claim survived. AllowsClaim is true only when the claimed
// verdict matched a VERIFIED recompute on server time.
type VerifiedFix struct {
	Risk        FixRisk
	AllowsClaim bool
}

// VerifyDeviceFix recomputes the frozen verdict from claimed metadata
// on the server receipt clock. Freshness and skew derive from
// receipt-minus-captured (server math); client-claimed age is
// informational and never trusted for the gate. A claimed verdict
// that differs from the recompute refuses with RiskForgedClaim —
// including garbage verdict strings, which can never match.
func VerifyDeviceFix(fix DeviceFix, receipt time.Time) (VerifiedFix, error) {
	if receipt.IsZero() {
		return VerifiedFix{}, errors.New("community: receipt time required")
	}
	in := FixInput{
		PermissionGranted: fix.PermissionGranted,
		HasFix:            fix.HasFix,
		SourceInfoPresent: fix.SourceInfoPresent,
		Simulated:         fix.Simulated,
		AccuracyMeters:    fix.AccuracyMeters,
		FixAgeSeconds:     serverAge(fix, receipt),
		ClockSkewSeconds:  fix.ClockSkewSeconds,
		Manual:            fix.Manual,
	}
	risk := ClassifyFix(in)
	if risk.Verdict != fix.ClaimedVerdict {
		return VerifiedFix{}, &FixRejection{Reason: RiskForgedClaim}
	}
	return VerifiedFix{Risk: risk, AllowsClaim: risk.AllowsClaim}, nil
}

// serverAge derives fix age from the server receipt clock alone. A
// missing or zero capture instant yields no age, which classifies
// no-fix and therefore refuses any VERIFIED claim: without a server
// timestamp there is no freshness to bound.
func serverAge(fix DeviceFix, receipt time.Time) *int64 {
	if fix.CapturedAt == nil || fix.CapturedAt.IsZero() {
		return nil
	}
	age := int64(receipt.Sub(*fix.CapturedAt) / time.Second)
	return &age
}

// TeleportRisk reports whether moving distanceM in elapsedSeconds is
// implausible for one contributor between observations. Zero or
// negative elapsed time with any movement is an anomaly (infinite
// speed); staying put never teleports. Pure policy for review
// routing, never an auto-reject.
func TeleportRisk(distanceM float64, elapsedSeconds int64) bool {
	if distanceM < 0 {
		distanceM = 0
	}
	if elapsedSeconds <= 0 {
		return distanceM > 0
	}
	return distanceM/float64(elapsedSeconds) > MaxPlausibleSpeedMS
}
