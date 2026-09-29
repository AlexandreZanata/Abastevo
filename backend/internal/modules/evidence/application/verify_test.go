package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

type fakeTransfer struct {
	mu            sync.Mutex
	payloads      [][]byte
	downloadCalls int
	uploadCalls   int
	uploadedKey   string
	uploadedType  string
	uploadedBody  []byte
	downloadErr   error
	uploadErr     error
}

func (f *fakeTransfer) download(_ context.Context, key string, maxBytes int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloadCalls++
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	if key == "" || maxBytes < 1 {
		return nil, errors.New("bad download args")
	}
	p := f.payloads[0]
	if len(f.payloads) > 1 {
		// Alternate payloads across calls to simulate a swapped source:
		// the orchestration must bind exactly one download.
		f.payloads = append(f.payloads[1:], p)
	}
	return p, nil
}

func (f *fakeTransfer) upload(_ context.Context, key, contentType string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploadCalls++
	if f.uploadErr != nil {
		return f.uploadErr
	}
	f.uploadedKey, f.uploadedType, f.uploadedBody = key, contentType, append([]byte{}, body...)
	return nil
}

func loadFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "adapters", "media", "testdata", "valid-4x4.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func verifyingSession(t *testing.T, raw []byte) domain.Session {
	t.Helper()
	sum := sha256.Sum256(raw)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	s, _, err := domain.NewSession(domain.Params{
		ID: "e0000000-0000-4000-8000-000000000001", ContributorRef: "tok-c1",
		ClientSessionID: "upl-1", MIME: "image/jpeg", DeclaredBytes: int64(len(raw)),
		ClaimedSHA256: hex.EncodeToString(sum[:]), QuarantineKey: "q/e0000000000000000000000000000001",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := domain.RequestVerification(s, now)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func verifyPorts(f *fakeTransfer) VerifyPorts {
	return VerifyPorts{
		Clock:    func() time.Time { return time.Date(2026, 9, 30, 12, 5, 0, 0, time.UTC) },
		Download: f.download,
		Upload:   f.upload,
	}
}

func asRejection(t *testing.T, err error) []string {
	t.Helper()
	var rej *RejectionError
	if !errors.As(err, &rej) {
		t.Fatalf("error = %v, want permanent rejection", err)
	}
	return rej.Reasons
}

func TestVerifyHappyPathBindsOneDownload(t *testing.T) {
	raw := loadFixture(t)
	f := &fakeTransfer{payloads: [][]byte{raw}}
	out, err := Verify(context.Background(), verifyPorts(f), verifyingSession(t, raw))
	if err != nil {
		t.Fatalf("verify = %v", err)
	}
	if out.Session.Status != domain.StateReady {
		t.Errorf("status = %q, want READY", out.Session.Status)
	}
	sum := sha256.Sum256(raw)
	if want := "f/" + hex.EncodeToString(sum[:]); out.FinalKey != want {
		t.Errorf("final key = %q, want %q", out.FinalKey, want)
	}
	if f.downloadCalls != 1 {
		t.Errorf("downloads = %d, want exactly one (TOCTOU)", f.downloadCalls)
	}
	if f.uploadCalls != 1 || f.uploadedKey != out.FinalKey || f.uploadedType != "image/jpeg" {
		t.Errorf("upload = %d %q %q", f.uploadCalls, f.uploadedKey, f.uploadedType)
	}
	if _, err := jpeg.Decode(bytes.NewReader(f.uploadedBody)); err != nil {
		t.Errorf("uploaded bytes do not decode: %v", err)
	}
	if strings.Contains(string(f.uploadedBody), "Exif") {
		t.Error("uploaded bytes carry EXIF")
	}
}

func TestVerifyRefusesWrongState(t *testing.T) {
	raw := loadFixture(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	s, _, err := domain.NewSession(domain.Params{
		ID: "e0000000-0000-4000-8000-000000000001", ContributorRef: "tok-c1",
		ClientSessionID: "upl-1", MIME: "image/jpeg", DeclaredBytes: int64(len(raw)),
		ClaimedSHA256: strings.Repeat("a", 64), QuarantineKey: "q/k", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeTransfer{payloads: [][]byte{raw}}
	if _, err := Verify(context.Background(), verifyPorts(f), s); !errors.Is(err, ErrBadState) {
		t.Errorf("issued verify = %v, want bad state", err)
	}
	if f.downloadCalls != 0 {
		t.Error("wrong-state session touched storage")
	}
}

func TestVerifyRejectsOversize(t *testing.T) {
	raw := loadFixture(t)
	sess := verifyingSession(t, raw)
	big := make([]byte, sess.MaxBytes+1)
	copy(big, raw)
	f := &fakeTransfer{payloads: [][]byte{big}}
	// Align the claim so only the size gate fires.
	sum := sha256.Sum256(big)
	sess.ClaimedSHA256 = hex.EncodeToString(sum[:])
	reasons := asRejection(t, mustVerify(t, f, sess))
	if reasons[0] != ReasonOversize {
		t.Errorf("reasons = %v", reasons)
	}
	if f.uploadCalls != 0 {
		t.Error("oversize payload reached the final key")
	}
}

func mustVerify(t *testing.T, f *fakeTransfer, s domain.Session) error {
	t.Helper()
	_, err := Verify(context.Background(), verifyPorts(f), s)
	if err == nil {
		t.Fatal("verification succeeded, want rejection")
	}
	return err
}

func TestVerifyRejectsInvalidAndMismatch(t *testing.T) {
	raw := loadFixture(t)
	f := &fakeTransfer{payloads: [][]byte{[]byte("not an image")}}
	sess := verifyingSession(t, raw)
	sum := sha256.Sum256([]byte("not an image"))
	sess.ClaimedSHA256 = hex.EncodeToString(sum[:])
	if reasons := asRejection(t, mustVerify(t, f, sess)); reasons[0] != ReasonInvalidImage {
		t.Errorf("reasons = %v", reasons)
	}

	f = &fakeTransfer{payloads: [][]byte{raw}}
	sess = verifyingSession(t, raw)
	sess.ClaimedSHA256 = strings.Repeat("b", 64)
	if reasons := asRejection(t, mustVerify(t, f, sess)); reasons[0] != ReasonChecksumMismatch {
		t.Errorf("reasons = %v", reasons)
	}

	polyglot := append(append([]byte{}, raw...), []byte("PK\x03\x04evil")...)
	f = &fakeTransfer{payloads: [][]byte{polyglot}}
	sess = verifyingSession(t, raw)
	sum = sha256.Sum256(polyglot)
	sess.ClaimedSHA256 = hex.EncodeToString(sum[:])
	if reasons := asRejection(t, mustVerify(t, f, sess)); reasons[0] != ReasonTrailingData {
		t.Errorf("reasons = %v", reasons)
	}
}

func TestVerifySurfacesTransientFailures(t *testing.T) {
	raw := loadFixture(t)
	f := &fakeTransfer{payloads: [][]byte{raw}, downloadErr: errors.New("storage down")}
	if _, err := Verify(context.Background(), verifyPorts(f), verifyingSession(t, raw)); err == nil {
		t.Error("download failure accepted")
	} else {
		var rej *RejectionError
		if errors.As(err, &rej) {
			t.Errorf("transient mapped as permanent: %v", err)
		}
	}
	f = &fakeTransfer{payloads: [][]byte{raw}, uploadErr: errors.New("r2 down")}
	if _, err := Verify(context.Background(), verifyPorts(f), verifyingSession(t, raw)); err == nil {
		t.Error("upload failure accepted")
	}
}

func TestVerifyBindsFirstPayloadOnSwap(t *testing.T) {
	// The source alternates bytes across reads (reused presigned URL
	// race): one download means the outcome binds exactly what was
	// hashed, never a later copy.
	first := loadFixture(t)
	second := append(append([]byte{}, first...), 0x00)
	second[len(second)-1] = 0xFF
	f := &fakeTransfer{payloads: [][]byte{first, second}}
	out, err := Verify(context.Background(), verifyPorts(f), verifyingSession(t, first))
	if err != nil {
		t.Fatalf("verify = %v", err)
	}
	sum := sha256.Sum256(first)
	if out.SourceSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("bound hash = %q, want first payload", out.SourceSHA256)
	}
	if f.downloadCalls != 1 {
		t.Errorf("downloads = %d, want exactly one", f.downloadCalls)
	}
}
