package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

// SEC03-B transfer-boundary pins (AUD-02): the enforced byte bound is
// the frozen session maximum, never the client declaration; transport
// oversize, substitution and absence all fail without promotion.

// Declared intent never widens enforcement: a 1 KiB declaration with
// actual bytes past the frozen maximum still rejects oversize and
// uploads nothing to the final key.
func TestBoundaryDeclaredIntentNeverWidensEnforcement(t *testing.T) {
	raw := loadFixture(t)
	sess := verifyingSession(t, raw)
	big := make([]byte, sess.MaxBytes+1)
	copy(big, raw)
	sum := sha256.Sum256(big)
	sess.ClaimedSHA256 = hex.EncodeToString(sum[:])
	sess.DeclaredBytes = 1024
	f := &fakeTransfer{payloads: [][]byte{big}}
	if reasons := asRejection(t, mustVerify(t, f, sess)); reasons[0] != ReasonOversize {
		t.Fatalf("reasons = %v, want oversize", reasons)
	}
	if f.uploadCalls != 0 {
		t.Error("bypass payload reached the final key")
	}
}

// A substituted object behind a reused URL binds the claim at verify
// time: bytes that disagree with the reservation claim reject with
// checksum-mismatch and never promote.
func TestBoundarySubstitutedBytesMismatchClaim(t *testing.T) {
	raw := loadFixture(t)
	sess := verifyingSession(t, raw)
	sub := append([]byte{}, raw...)
	sub[len(sub)-1] ^= 0xFF
	f := &fakeTransfer{payloads: [][]byte{sub}}
	if reasons := asRejection(t, mustVerify(t, f, sess)); reasons[0] != ReasonChecksumMismatch {
		t.Fatalf("reasons = %v, want checksum-mismatch", reasons)
	}
	if f.uploadCalls != 0 {
		t.Error("substituted payload reached the final key")
	}
}

// An interrupted transfer (absent object) is transient, never a
// promotion and never a permanent rejection: the sweeper owns
// expiry, not the verifier.
func TestBoundaryAbsentObjectStaysTransient(t *testing.T) {
	raw := loadFixture(t)
	f := &fakeTransfer{downloadErr: errors.New("storage: object missing")}
	_, err := Verify(context.Background(), verifyPorts(f), verifyingSession(t, raw))
	if err == nil {
		t.Fatal("absent object accepted")
	}
	var rej *RejectionError
	if errors.As(err, &rej) {
		t.Fatalf("absence mapped as permanent: %v", err)
	}
}
