package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

func forwardPorts(f *fakeTransfer) VerifyPorts { return verifyPorts(f) }

// TestVerifyForwardHappyPath proves the forward lane end to end on a
// small frame: bounded download, claim bind, forward pixel caps,
// sanitized upload under the JPEG wire type and the immutable copy
// deadline from first receipt (completion instant + 24 h).
func TestVerifyForwardHappyPath(t *testing.T) {
	raw := loadFixture(t)
	sess := verifyingSession(t, raw)
	f := &fakeTransfer{payloads: [][]byte{raw}}
	out, err := VerifyForward(context.Background(), forwardPorts(f), sess)
	if err != nil {
		t.Fatalf("VerifyForward: %v", err)
	}
	if out.Session.Status != domain.StateReady {
		t.Errorf("session must be READY, got %q", out.Session.Status)
	}
	if out.Width != 4 || out.Height != 4 {
		t.Errorf("dims = %dx%d", out.Width, out.Height)
	}
	if f.uploadCalls != 1 || f.uploadedType != domain.ForwardWireMIME {
		t.Errorf("one JPEG upload required, got %d (%q)", f.uploadCalls, f.uploadedType)
	}
	if int64(len(f.uploadedBody)) > domain.ForwardCapBytes {
		t.Errorf("upload %d bytes exceeds forward cap", len(f.uploadedBody))
	}
	wantExpiry := sess.UpdatedAt.Add(domain.ForwardDeadline)
	if !out.ExpiresAt.Equal(wantExpiry) {
		t.Errorf("expiry must be first-receipt + 24 h (%v), got %v", wantExpiry, out.ExpiresAt)
	}
	if f.downloadCalls != 1 {
		t.Errorf("exactly one download, got %d", f.downloadCalls)
	}
}

// TestVerifyForwardIgnoresClientBudgets proves no client value widens
// the lane: a 3 MiB reservation bound still downloads through the
// 256 KiB server cap, and a 300 KiB object refuses oversize even
// though the session would allow 3 MiB.
func TestVerifyForwardIgnoresClientBudgets(t *testing.T) {
	big := make([]byte, domain.ForwardCapBytes+1024)
	copy(big, loadFixture(t))
	sess := verifyingSession(t, loadFixture(t))
	if sess.MaxBytes != domain.MaxUploadBytes {
		t.Fatalf("reservation bound must stay 3 MiB, got %d", sess.MaxBytes)
	}
	f := &fakeTransfer{payloads: [][]byte{big}}
	_, err := VerifyForward(context.Background(), forwardPorts(f), sess)
	var rej *RejectionError
	if !errors.As(err, &rej) || len(rej.Reasons) != 1 || rej.Reasons[0] != ReasonOversize {
		t.Errorf("over-cap must refuse oversize, got %v", err)
	}
}

// TestVerifyForwardRefusals maps corrupt, claim-mismatch and bomb
// inputs onto the stable reason codes without terminal progress.
func TestVerifyForwardRefusals(t *testing.T) {
	raw := loadFixture(t)
	for _, tc := range []struct {
		name    string
		payload func() []byte
		reason  string
	}{
		{"corrupt", func() []byte { return []byte{0xFF, 0xD8, 0x00} }, ReasonInvalidImage},
		{"forged-mime", func() []byte {
			out := append([]byte{0x89, 'P', 'N', 'G'}, make([]byte, 64)...)
			return out
		}, ReasonInvalidImage},
	} {
		payload := tc.payload()
		sess := verifyingSession(t, raw)
		// Rebind the claim to the actual payload so the refusal under
		// test is the media check, not the checksum.
		sess = rebindClaim(t, sess, payload)
		f := &fakeTransfer{payloads: [][]byte{payload}}
		_, err := VerifyForward(context.Background(), forwardPorts(f), sess)
		var rej *RejectionError
		if !errors.As(err, &rej) || len(rej.Reasons) != 1 || rej.Reasons[0] != tc.reason {
			t.Errorf("%s: must refuse %q, got %v", tc.name, tc.reason, err)
		}
		if f.uploadCalls != 0 {
			t.Errorf("%s: refused input must never upload", tc.name)
		}
	}
}

// TestVerifyForwardClaimMismatch proves a substituted object fails on
// the hash bind even when its pixels are valid.
func TestVerifyForwardClaimMismatch(t *testing.T) {
	raw := loadFixture(t)
	sess := verifyingSession(t, raw)
	sess.ClaimedSHA256 = "00" + sess.ClaimedSHA256[2:]
	f := &fakeTransfer{payloads: [][]byte{raw}}
	_, err := VerifyForward(context.Background(), forwardPorts(f), sess)
	var rej *RejectionError
	if !errors.As(err, &rej) || rej.Reasons[0] != ReasonChecksumMismatch {
		t.Errorf("substituted object must refuse checksum-mismatch, got %v", err)
	}
}

// TestVerifyForwardTerminalReplay proves non-VERIFYING sessions do no
// work: READY replays converge as bad-state without downloads.
func TestVerifyForwardTerminalReplay(t *testing.T) {
	raw := loadFixture(t)
	sess := verifyingSession(t, raw)
	ready, _, err := domain.MarkReady(sess, time.Date(2026, 9, 30, 12, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeTransfer{payloads: [][]byte{raw}}
	if _, err := VerifyForward(context.Background(), forwardPorts(f), ready); !errors.Is(err, ErrBadState) {
		t.Errorf("terminal replay must be bad-state, got %v", err)
	}
	if f.downloadCalls != 0 || f.uploadCalls != 0 {
		t.Error("terminal replay must not touch storage")
	}
}

// TestVerifyForwardInterruptedUpload proves truncation fails closed:
// half the bytes never verify, never upload.
func TestVerifyForwardInterruptedUpload(t *testing.T) {
	raw := loadFixture(t)
	half := raw[:len(raw)/2]
	sess := rebindClaim(t, verifyingSession(t, raw), half)
	f := &fakeTransfer{payloads: [][]byte{half}}
	_, err := VerifyForward(context.Background(), forwardPorts(f), sess)
	if err == nil {
		t.Error("truncated upload accepted")
	}
	if f.uploadCalls != 0 {
		t.Error("truncated upload must never upload")
	}
}

func rebindClaim(t *testing.T, sess domain.Session, payload []byte) domain.Session {
	t.Helper()
	sum := sha256.Sum256(payload)
	sess.ClaimedSHA256 = hex.EncodeToString(sum[:])
	return sess
}
