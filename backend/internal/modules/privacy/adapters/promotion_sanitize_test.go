package adapters

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// RVA06: container validation is not content anonymization. Until a
// price-region minimizer is qualified, valid content is UNCERTAIN.

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

func TestUnqualifiedContentNeverPasses(t *testing.T) {
	// No pixel layout is currently qualified, including price-only synthetic
	// input. Re-encoding cannot determine whether identifying content remains.
	for _, dimensions := range [][2]int{{8, 8}, {160, 80}, {1, 1}} {
		minimized, verdict, err := SanitizeImage(context.Background(), testJPEG(t, dimensions[0], dimensions[1]))
		if err != nil || verdict != domain.VerdictUncertain || len(minimized) != 0 {
			t.Fatalf("unqualified content escaped: %d bytes, %q, %v", len(minimized), verdict, err)
		}
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

func TestTrainingSourceIsUnavailable(t *testing.T) {
	body, err := TrainingSourceUnavailable(context.Background(), "synthetic-image")
	if !errors.Is(err, ErrTrainingCollectionUnavailable) || len(body) != 0 {
		t.Fatal("collection source enabled", len(body), err)
	}
}
func TestTrainingSanitizerBoundsAndCancellation(t *testing.T) {
	valid := testJPEG(t, 8, 8)
	bomb := append([]byte{}, valid...)
	sof := bytes.Index(bomb, []byte{0xff, 0xc0})
	if sof < 0 {
		t.Fatal("synthetic JPEG has no frame header")
	}
	bomb[sof+5], bomb[sof+6] = 0x7f, 0xff
	for name, raw := range map[string][]byte{"over-wire-bound": make([]byte, (3<<20)+1), "pixel-bomb": bomb} {
		body, verdict, err := SanitizeImage(context.Background(), raw)
		if err == nil || verdict != domain.VerdictFail || len(body) != 0 {
			t.Fatal(name, verdict, len(body), err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if body, verdict, err := SanitizeImage(ctx, valid); !errors.Is(err, context.Canceled) || verdict != domain.VerdictFail || len(body) != 0 {
		t.Fatal(verdict, len(body), err)
	}
	// An EXIF-free frame still has unknown identifying pixel content. A
	// synthetic EXIF marker cannot change the refusal to release bytes.
	segment := []byte{0xff, 0xe1, 0, 12, 'E', 'x', 'i', 'f', 0, 0, 'x', 'x', 'x', 'x'}
	withEXIF := append(append(append([]byte{}, valid[:2]...), segment...), valid[2:]...)
	if body, verdict, err := SanitizeImage(context.Background(), withEXIF); err != nil || verdict != domain.VerdictUncertain || len(body) != 0 {
		t.Fatal("EXIF image escaped", verdict, len(body), err)
	}
}
