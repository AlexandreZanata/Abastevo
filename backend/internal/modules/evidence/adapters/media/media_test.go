package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

func loadValid(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "valid-4x4.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[0] != 0xFF || raw[1] != 0xD8 {
		t.Fatal("fixture is not a JPEG")
	}
	return raw
}

// withPatchedDimensions rewrites the SOF width/height of a valid JPEG,
// simulating a decompression-bomb header without a huge payload.
func withPatchedDimensions(t *testing.T, raw []byte, w, h int) []byte {
	t.Helper()
	out := append([]byte{}, raw...)
	for i := 0; i+8 < len(out); i++ {
		if out[i] == 0xFF && out[i+1] == 0xC0 {
			binary.BigEndian.PutUint16(out[i+5:i+7], uint16(h))
			binary.BigEndian.PutUint16(out[i+7:i+9], uint16(w))
			return out
		}
	}
	t.Fatal("no SOF0 marker found")
	return nil
}

// withEXIF splices an APP1 Exif segment after SOI, like a phone camera.
func withEXIF(raw []byte) []byte {
	payload := append([]byte("Exif\x00\x00"), bytes.Repeat([]byte{0xAB}, 32)...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte((len(payload) + 2) & 0xFF)}
	seg = append(seg, payload...)
	out := append([]byte{raw[0], raw[1]}, seg...)
	return append(out, raw[2:]...)
}

func TestSnapshotBoundsAndHashes(t *testing.T) {
	raw := loadValid(t)
	snap, err := Snapshot(raw, domain.MaxUploadBytes)
	if err != nil {
		t.Fatalf("snapshot = %v", err)
	}
	sum := sha256.Sum256(raw)
	if snap.SourceSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("hash = %q", snap.SourceSHA256)
	}
	if len(snap.Bytes) != len(raw) {
		t.Error("snapshot must carry the exact downloaded bytes")
	}
	huge := make([]byte, domain.MaxUploadBytes+1)
	if _, err := Snapshot(huge, domain.MaxUploadBytes); err == nil {
		t.Error("oversize snapshot accepted")
	}
}

func TestInspectHappyPath(t *testing.T) {
	raw := loadValid(t)
	dims, err := Inspect(raw)
	if err != nil {
		t.Fatalf("inspect = %v", err)
	}
	if dims.Width != 4 || dims.Height != 4 {
		t.Errorf("dims = %+v", dims)
	}
}

func TestInspectRejectsNonJPEG(t *testing.T) {
	if _, err := Inspect([]byte("not an image")); err == nil {
		t.Error("non-JPEG accepted")
	}
	if _, err := Inspect(nil); err == nil {
		t.Error("empty input accepted")
	}
}

func TestInspectRejectsBombDimensions(t *testing.T) {
	// A 65535x65535 header must die on policy before any pixel allocation.
	bomb := withPatchedDimensions(t, loadValid(t), 65535, 65535)
	if _, err := Inspect(bomb); err == nil {
		t.Error("bomb dimensions accepted")
	}
	wide := withPatchedDimensions(t, loadValid(t), MaxDimension+1, 4)
	if _, err := Inspect(wide); err == nil {
		t.Error("over-wide dimensions accepted")
	}
}

func TestInspectRejectsPolyglotTrailingData(t *testing.T) {
	// Appended executables after EOI must not ride along silently.
	raw := loadValid(t)
	polyglot := append(append([]byte{}, raw...), []byte("PK\x03\x04malicious-payload")...)
	if _, err := Inspect(polyglot); err == nil {
		t.Error("trailing data accepted")
	}
	if _, err := Inspect(raw); err != nil {
		t.Errorf("clean file rejected: %v", err)
	}
}

func TestSanitizeStripsEXIF(t *testing.T) {
	raw := withEXIF(loadValid(t))
	if !strings.Contains(string(raw), "Exif") {
		t.Fatal("EXIF splice failed")
	}
	if _, err := Inspect(raw); err != nil {
		t.Fatalf("EXIF file rejected at inspect: %v", err)
	}
	clean, err := Sanitize(raw)
	if err != nil {
		t.Fatalf("sanitize = %v", err)
	}
	if strings.Contains(string(clean), "Exif") {
		t.Error("sanitized output still carries EXIF")
	}
	img, err := jpeg.Decode(bytes.NewReader(clean))
	if err != nil {
		t.Fatalf("sanitized output does not decode: %v", err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 4 {
		t.Errorf("sanitized dims = %v", img.Bounds())
	}
	again, err := Sanitize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(clean, again) {
		t.Error("sanitization is not deterministic")
	}
}

func TestDHashSeparatesImages(t *testing.T) {
	raw := loadValid(t)
	a, err := DifferenceHash(raw)
	if err != nil {
		t.Fatal(err)
	}
	again, err := DifferenceHash(raw)
	if err != nil || again != a {
		t.Errorf("dHash unstable: %v %v", again, err)
	}
	// A different picture hashes differently: checkerboard against the
	// fixture's smooth gradient (a second gradient could share dHash bits).
	other := image.NewGray(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if (x+y)%2 == 0 {
				other.Pix[y*8+x] = 255
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, other, nil); err != nil {
		t.Fatal(err)
	}
	b, err := DifferenceHash(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if b == a {
		t.Error("distinct images share a dHash")
	}
}

func TestFinalKeyIsContentAddressed(t *testing.T) {
	raw := loadValid(t)
	sum := sha256.Sum256(raw)
	want := "f/" + hex.EncodeToString(sum[:])
	if got := FinalKey(raw); got != want {
		t.Errorf("final key = %q, want %q", got, want)
	}
	if !strings.HasPrefix(FinalKey([]byte("x")), "f/") {
		t.Error("final key escapes the server namespace")
	}
}
