package media

import (
	"bytes"
	"image/jpeg"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// Forward revalidation against the frozen P15-T01 budgets
// (B-BR-M02/M03): JPEG still-image magic, strict structure (no
// trailing data) and header pixel caps (1600-pixel edge, 2 MP)
// enforced before any pixel allocation, then bounded re-encode.
// The deployed v1 path (Inspect/Sanitize, 3 MiB/12 MP) stays
// untouched until the P15-T04 migration/rollout; this lane never
// trusts client-declared budgets or deadlines — every bound below
// is a server constant from the evidence domain.

// ForwardQualities is the bounded re-encode schedule: at most three
// attempts tightening toward the hard cap, mirroring the native
// qualityForAttempt (85/70/55). More attempts never help a noisy
// frame fit; over-cap results refuse instead of crushing evidence.
var ForwardQualities = []int{85, 70, 55}

// InspectForward enforces JPEG magic, strict structure and the
// forward pixel caps on headers alone: bomb headers die here with
// zero pixel allocation.
func InspectForward(raw []byte) (Dims, error) {
	if len(raw) < 2 || raw[0] != 0xFF || raw[1] != 0xD8 {
		return Dims{}, ErrNotJPEG
	}
	if err := assertCleanStructure(raw); err != nil {
		return Dims{}, err
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return Dims{}, ErrInvalidImage
	}
	if err := checkForwardDims(cfg.Width, cfg.Height); err != nil {
		return Dims{}, err
	}
	return Dims{Width: cfg.Width, Height: cfg.Height}, nil
}

func checkForwardDims(w, h int) error {
	if w < 1 || h < 1 || w > domain.ForwardMaxEdgePixels || h > domain.ForwardMaxEdgePixels {
		return ErrDimensionsExceeded
	}
	if int64(w)*int64(h) > domain.ForwardMaxMegapixels {
		return ErrDimensionsExceeded
	}
	return nil
}

// SanitizeForward fully decodes one forward-inspected snapshot and
// re-encodes its pixels within three bounded attempts: decoder output
// only, so EXIF, embedded thumbnails and trailing payloads cannot
// survive. Results above the 256 KiB hard cap refuse (ErrTooLarge)
// instead of silently compressing away evidence; the input must still
// be a forward-inspected snapshot by contract.
func SanitizeForward(raw []byte) ([]byte, error) {
	if _, err := InspectForward(raw); err != nil {
		return nil, err
	}
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrInvalidImage
	}
	for _, quality := range ForwardQualities {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, err
		}
		if int64(out.Len()) <= int64(domain.ForwardCapBytes) {
			return out.Bytes(), nil
		}
	}
	return nil, ErrTooLarge
}
