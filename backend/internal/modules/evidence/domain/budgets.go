package domain

import (
	"errors"
	"time"
)

// Forward media budgets frozen by P15-T01 (B-BR-M01…M03) from
// docs/security/LOCAL_MEDIA_LOCATION_POLICY.md. These bound the
// P15-T02…T05 native/backend pipelines; the deployed v1 evidence
// protocol (AllowedMIME/MaxUploadBytes/SessionTTL above) stays
// untouched until those slices pass their tests. Values change only
// with a new measured analysis, never to make a failing test pass.
const (
	// ForwardWireMIME is the single frozen wire format: locally
	// re-encoded JPEG, metadata stripped. Supported inputs convert
	// to this; anything else fails safely in T02/T03.
	ForwardWireMIME = "image/jpeg"
	// ForwardTargetBytes is the legibility target: at most 150 KiB
	// after bounded re-encoding (at most ForwardMaxAttempts tries).
	ForwardTargetBytes = 150 << 10
	// ForwardCapBytes is the hard wire cap: at most 256 KiB. Larger
	// results reject instead of silently compressing away evidence.
	ForwardCapBytes = 256 << 10
	// ForwardMaxEdgePixels bounds the longest edge: 1600 pixels.
	ForwardMaxEdgePixels = 1600
	// ForwardMaxMegapixels bounds total pixels: 2 MP, measured as
	// width*height <= 2_000_000 (integer math, no float drift).
	ForwardMaxMegapixels = 2_000_000
	// ForwardMaxAttempts caps bounded encoding retries at three.
	ForwardMaxAttempts = 3
	// ForwardWorkingMemoryHypothesis caps the per-image working
	// hypothesis at 32 MiB, measured on P12 low-resource devices in
	// P15-T02. P15-T03 derives worker concurrency from measured peak
	// RSS against this hypothesis.
	ForwardWorkingMemoryHypothesis = 32 << 20
	// ForwardDeadline is the single expiry for every app-owned copy:
	// server first receipt + 24 hours. Retries, processing, copying,
	// disputes and downloads cannot extend it (M01).
	ForwardDeadline = 24 * time.Hour
)

var (
	// ErrForwardEmpty marks zero-byte wire payloads.
	ErrForwardEmpty = errors.New("evidence: forward wire payload is empty")
	// ErrForwardOverCap marks wire payloads above the 256 KiB cap.
	ErrForwardOverCap = errors.New("evidence: forward wire payload exceeds 256 KiB cap")
	// ErrForwardEdgeOver marks frames with longest edge above 1600.
	ErrForwardEdgeOver = errors.New("evidence: forward frame edge exceeds 1600 pixels")
	// ErrForwardPixelsOver marks frames above 2 megapixels.
	ErrForwardPixelsOver = errors.New("evidence: forward frame exceeds 2 megapixels")
)

// ForwardVerdict codes the stable machine reason for budget refusals;
// display strings stay out of the domain.
const (
	ForwardOK           = "ok"
	ForwardBytesEmpty   = "bytes-empty"
	ForwardBytesOverCap = "bytes-over-cap"
	ForwardEdgeOver     = "edge-over-limit"
	ForwardPixelsOver   = "pixels-over-limit"
)

// ValidateForwardWire refuses wire payloads outside the frozen
// budgets without coercion. Dimensions arrive from server-measured
// headers (never client claims); T03 enforces that boundary.
func ValidateForwardWire(sizeBytes int64, edgePixels int, totalPixels int64) error {
	if sizeBytes <= 0 {
		return ErrForwardEmpty
	}
	if sizeBytes > ForwardCapBytes {
		return ErrForwardOverCap
	}
	if edgePixels > ForwardMaxEdgePixels {
		return ErrForwardEdgeOver
	}
	if totalPixels > ForwardMaxMegapixels {
		return ErrForwardPixelsOver
	}
	return nil
}

// ForwardVerdictCode maps a validation error to its stable fixture code.
func ForwardVerdictCode(err error) string {
	switch {
	case err == nil:
		return ForwardOK
	case errors.Is(err, ErrForwardEmpty):
		return ForwardBytesEmpty
	case errors.Is(err, ErrForwardOverCap):
		return ForwardBytesOverCap
	case errors.Is(err, ErrForwardEdgeOver):
		return ForwardEdgeOver
	case errors.Is(err, ErrForwardPixelsOver):
		return ForwardPixelsOver
	default:
		return "invalid-value"
	}
}

// ForwardDeadlineAfter derives the single copy deadline from server
// first receipt. There is no extension parameter by construction:
// retries, processing, copying, disputes and downloads all reuse the
// first-receipt deadline (M01).
func ForwardDeadlineAfter(firstReceipt time.Time) time.Time {
	return firstReceipt.Add(ForwardDeadline)
}
