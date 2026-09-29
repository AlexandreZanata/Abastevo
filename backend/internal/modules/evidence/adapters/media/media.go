package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/color"
	"image/jpeg"
)

// Policy caps from the SECURITY_PRIVACY evidence protocol: at most 3 MiB
// on the wire (domain bound), 12 megapixels and 8192 px per side against
// decoder bombs. Caps are enforced on headers before any pixel allocation
// and again inside Sanitize, so an uninspected caller stays safe.
const (
	MaxPixels       = 12_000_000
	MaxDimension    = 8192
	SanitizeQuality = 85
	// FinalPrefix namespaces every sanitized object. Client credentials
	// only ever authorize the quarantine namespace (storage package), so
	// final bytes are unwritable and unoverwritable from the outside.
	FinalPrefix = "f/"
)

var (
	ErrTooLarge           = errors.New("media: snapshot exceeds bound")
	ErrNotJPEG            = errors.New("media: not a JPEG still image")
	ErrInvalidImage       = errors.New("media: malformed image structure")
	ErrTrailingData       = errors.New("media: trailing data after image end")
	ErrDimensionsExceeded = errors.New("media: dimensions exceed policy caps")
)

// Snap is one bounded download: the exact bytes plus the server hash that
// binds everything downstream to them.
type Snap struct {
	Bytes        []byte
	SourceSHA256 string
}

// Snapshot bounds one download and hashes its exact bytes. The caller
// downloads once and passes the result through every later check, so no
// second read can substitute a different object under the hash.
func Snapshot(raw []byte, maxBytes int64) (Snap, error) {
	if maxBytes < 1 || int64(len(raw)) > maxBytes {
		return Snap{}, ErrTooLarge
	}
	if len(raw) < 2 {
		return Snap{}, ErrNotJPEG
	}
	sum := sha256.Sum256(raw)
	return Snap{Bytes: append([]byte{}, raw...), SourceSHA256: hex.EncodeToString(sum[:])}, nil
}

// Dims carries validated pixel dimensions.
type Dims struct{ Width, Height int }

// Inspect enforces magic, strict structure (no trailing data) and
// dimension caps. It allocates nothing proportional to the image: bomb
// headers die here, before any decode.
func Inspect(raw []byte) (Dims, error) {
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
	if err := checkDims(cfg.Width, cfg.Height); err != nil {
		return Dims{}, err
	}
	return Dims{Width: cfg.Width, Height: cfg.Height}, nil
}

func checkDims(w, h int) error {
	if w < 1 || h < 1 || w > MaxDimension || h > MaxDimension {
		return ErrDimensionsExceeded
	}
	if int64(w)*int64(h) > MaxPixels {
		return ErrDimensionsExceeded
	}
	return nil
}

// assertCleanStructure walks JPEG segments from SOI: every pre-SOS marker
// is consumed by its declared length, and the entropy-coded scan ends at
// the first EOI with nothing after it. Anything else — truncation,
// unknown scan markers, trailing payloads — fails instead of riding
// along into the verified output.
func assertCleanStructure(raw []byte) error {
	i := 2 // past SOI
	for {
		if i >= len(raw) {
			return ErrInvalidImage
		}
		// Skip fill bytes: any run of 0xFF introduces one marker.
		if raw[i] != 0xFF {
			return ErrInvalidImage
		}
		for i < len(raw) && raw[i] == 0xFF {
			i++
		}
		if i >= len(raw) {
			return ErrInvalidImage
		}
		marker := raw[i]
		i++
		switch {
		case marker == 0xD9:
			// EOI before any scan data: empty image.
			return ErrInvalidImage
		case marker == 0xDA:
			return assertCleanScan(raw, i)
		case marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7):
			// Standalone markers carry no length.
			continue
		default:
			if i+1 >= len(raw) {
				return ErrInvalidImage
			}
			length := int(raw[i])<<8 | int(raw[i+1])
			if length < 2 || i+length > len(raw) {
				return ErrInvalidImage
			}
			i += length
		}
	}
}

// assertCleanScan consumes the SOS header by its declared length, then
// requires the scan to end at the first EOI with no trailing bytes.
// Stuffed (FF 00) and restart (FF D0–D7) sequences are skipped; any other
// marker inside the scan is strict rejection.
func assertCleanScan(raw []byte, at int) error {
	if at+1 >= len(raw) {
		return ErrInvalidImage
	}
	length := int(raw[at])<<8 | int(raw[at+1])
	if length < 2 || at+length > len(raw) {
		return ErrInvalidImage
	}
	k := at + length
	for k < len(raw) {
		if raw[k] != 0xFF {
			k++
			continue
		}
		if k+1 >= len(raw) {
			return ErrInvalidImage
		}
		switch next := raw[k+1]; {
		case next == 0x00:
			k += 2
		case next >= 0xD0 && next <= 0xD7:
			k += 2
		case next == 0xD9:
			if k+2 != len(raw) {
				return ErrTrailingData
			}
			return nil
		default:
			return ErrInvalidImage
		}
	}
	return ErrInvalidImage
}

// Sanitize fully decodes one snapshot and re-encodes its pixels: decoder
// output only, so EXIF, embedded thumbnails and trailing payloads cannot
// survive. Caps are rechecked on headers first; the input must still be
// an inspected snapshot by contract.
func Sanitize(raw []byte) ([]byte, error) {
	if _, err := Inspect(raw); err != nil {
		return nil, err
	}
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrInvalidImage
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: SanitizeQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// DifferenceHash computes a 64-bit dHash over a 9x8 grayscale reduction:
// same picture hashes stably, distinct pictures diverge. It feeds the
// duplicate-media independence signals downstream (P06), never a truth
// verdict on its own.
func DifferenceHash(raw []byte) (uint64, error) {
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		return 0, ErrInvalidImage
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 1 || h < 1 {
		return 0, ErrInvalidImage
	}
	at := func(x, y int) uint8 {
		sx := bounds.Min.X + x*w/9
		sy := bounds.Min.Y + y*h/8
		return color.GrayModel.Convert(img.At(sx, sy)).(color.Gray).Y
	}
	var hash uint64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			hash <<= 1
			if at(x, y) > at(x+1, y) {
				hash |= 1
			}
		}
	}
	return hash, nil
}

// FinalKey derives the immutable server-only object key from the source
// hash: content-addressed, deterministic and unreachable from client
// credentials, which only authorize the quarantine namespace.
func FinalKey(raw []byte) string {
	sum := sha256.Sum256(raw)
	return FinalPrefix + hex.EncodeToString(sum[:])
}
