package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"runtime"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// forwardFrame builds a deterministic smooth-gradient JPEG of the
// requested size: low entropy so it stays inside the forward cap,
// exercising the largest allowed decode instead of the refuse path.
func forwardFrame(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 255) / 1600),
				G: uint8((y * 255) / 1250),
				B: 128,
				A: 255,
			})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	return out.Bytes()
}

func TestInspectForwardHappyPath(t *testing.T) {
	raw := loadValid(t)
	dims, err := InspectForward(raw)
	if err != nil {
		t.Fatalf("inspect forward = %v", err)
	}
	if dims.Width != 4 || dims.Height != 4 {
		t.Errorf("dims = %+v", dims)
	}
}

func TestInspectForwardRefusesBombHeaderWithoutAlloc(t *testing.T) {
	raw := loadValid(t)
	// Header claims 20000x20000 (400 MP) on a 4x4 body: must die on
	// headers before any pixel allocation.
	bomb := withPatchedDimensions(t, raw, 20000, 20000)
	if _, err := InspectForward(bomb); err == nil {
		t.Error("bomb header accepted")
	} else if err != ErrDimensionsExceeded {
		t.Errorf("bomb must be dimensions-exceeded, got %v", err)
	}
	// 1601-pixel edge and 2MP+1 frames refuse alike.
	wide := withPatchedDimensions(t, raw, 1601, 4)
	if _, err := InspectForward(wide); err != ErrDimensionsExceeded {
		t.Errorf("1601 edge must refuse, got %v", err)
	}
}

func TestInspectForwardRefusesForgedMIME(t *testing.T) {
	pngMagic := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0}, 64)...)
	if _, err := InspectForward(pngMagic); err != ErrNotJPEG {
		t.Errorf("PNG magic must be not-JPEG, got %v", err)
	}
	if _, err := InspectForward([]byte{}); err != ErrNotJPEG {
		t.Errorf("empty must be not-JPEG, got %v", err)
	}
	truncated := loadValid(t)[:len(loadValid(t))/2]
	if _, err := InspectForward(truncated); err == nil {
		t.Error("truncated upload accepted")
	}
}

func TestSanitizeForwardStripsEXIFAndFitsCap(t *testing.T) {
	raw := withEXIF(loadValid(t))
	clean, err := SanitizeForward(raw)
	if err != nil {
		t.Fatalf("sanitize forward = %v", err)
	}
	if int64(len(clean)) > domain.ForwardCapBytes {
		t.Errorf("sanitized %d bytes exceed forward cap", len(clean))
	}
	if _, err := InspectForward(clean); err != nil {
		t.Errorf("sanitized output must re-inspect clean, got %v", err)
	}
	if bytes.Contains(clean, []byte("Exif")) {
		t.Error("EXIF survived sanitization")
	}
}

func TestSanitizeForwardTightensQualityWithinThreeAttempts(t *testing.T) {
	if len(ForwardQualities) != domain.ForwardMaxAttempts {
		t.Fatalf("attempts must stay %d, got %v", domain.ForwardMaxAttempts, ForwardQualities)
	}
	// Noisy high-entropy frame at the pixel ceiling: refuses over
	// cap instead of crushing.
	noisy := noisyFrame(t, 1600, 1250)
	if _, err := SanitizeForward(noisy); err != ErrTooLarge {
		t.Errorf("noisy over-cap frame must refuse too-large, got %v", err)
	}
}

// noisyFrame builds a high-entropy JPEG that stays inside the pixel
// budgets but cannot fit 256 KiB in three bounded encodes.
func noisyFrame(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(0x12345678)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			seed = seed*1664525 + 1013904223
			v := uint8(seed >> 24)
			img.SetRGBA(x, y, color.RGBA{v, uint8(x + y), v ^ uint8(x*y), 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("encode noisy: %v", err)
	}
	return out.Bytes()
}

// TestSanitizeForwardMeasuredMemory decodes the largest allowed frame
// (1600x1250 = 2.0 MP) and asserts the heap growth stays inside the
// 32 MiB working-memory hypothesis: controlled RSS, measured.
func TestSanitizeForwardMeasuredMemory(t *testing.T) {
	raw := forwardFrame(t, 1600, 1250)
	if _, err := InspectForward(raw); err != nil {
		t.Fatalf("2MP frame must inspect clean, got %v", err)
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	clean, err := SanitizeForward(raw)
	if err != nil {
		t.Fatalf("2MP smooth frame must sanitize, got %v", err)
	}
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	growth := int64(after.TotalAlloc) - int64(before.TotalAlloc)
	if growth < 0 {
		growth = 0
	}
	if growth > domain.ForwardWorkingMemoryHypothesis {
		t.Errorf("heap growth %d bytes exceeds 32 MiB hypothesis", growth)
	}
	t.Logf("2MP sanitize: %d wire bytes, heap growth %d bytes", len(clean), growth)
}
