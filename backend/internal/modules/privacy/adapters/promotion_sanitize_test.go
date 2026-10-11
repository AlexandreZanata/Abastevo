package adapters

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// CS10B: the sanitizer adapter maps the evidence media pipeline onto
// the promotion Sanitizer port. Success minimizes (PASS); every
// pipeline refusal maps to FAIL — fail-closed with no fallback.
// UNCERTAIN stays reserved for a future content classifier; nothing
// here invents one.

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSanitizePassesValidJPEG(t *testing.T) {
	minimized, verdict, err := SanitizeImage(context.Background(), testJPEG(t, 8, 8))
	if err != nil || verdict != domain.VerdictPass || len(minimized) == 0 {
		t.Fatalf("sanitize = %d bytes, %q, %v", len(minimized), verdict, err)
	}
}

func TestSanitizeFailsClosed(t *testing.T) {
	for name, raw := range map[string][]byte{
		"not an image":  []byte("fuel price 5999"),
		"empty":         {},
		"truncated":     testJPEG(t, 8, 8)[:64],
		"trailing data": append(append([]byte{}, testJPEG(t, 8, 8)...), 0x00),
	} {
		if _, verdict, err := SanitizeImage(context.Background(), raw); err == nil || verdict != domain.VerdictFail {
			t.Errorf("%s = %q, %v, want FAIL with error", name, verdict, err)
		}
	}
}
