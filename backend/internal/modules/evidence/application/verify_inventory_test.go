package application

import (
	"context"
	"testing"
)

func TestVerifyRefusesUninventoriedOutput(t *testing.T) {
	raw := loadFixture(t)
	f := &fakeTransfer{payloads: [][]byte{raw}}
	p := verifyPorts(f)
	p.BeforeUpload = nil
	if _, err := Verify(context.Background(), p, verifyingSession(t, raw)); err == nil || f.uploadCalls != 0 {
		t.Fatalf("uninventoried final write: err=%v uploads=%d", err, f.uploadCalls)
	}
}

func TestVerifyInventoryRefusalAndReadOnlyReuse(t *testing.T) {
	raw := loadFixture(t)
	for _, forward := range []bool{false, true} {
		for _, reuse := range []bool{false, true} {
			f := &fakeTransfer{payloads: [][]byte{raw}}
			p := verifyPorts(f)
			calls := 0
			p.BeforeUpload = func(context.Context, string, string) (FinalWrite, error) {
				calls++
				if reuse {
					return FinalWrite{Reused: true}, nil
				}
				return FinalWrite{}, ErrBadState
			}
			verify := Verify
			if forward {
				verify = VerifyForward
			}
			out, err := verify(context.Background(), p, verifyingSession(t, raw))
			if calls != 1 || f.uploadCalls != 0 {
				t.Fatalf("guard calls=%d uploads=%d", calls, f.uploadCalls)
			}
			if reuse && (err != nil || !out.Write.Reused) {
				t.Fatal(out, err)
			}
			if !reuse && err == nil {
				t.Fatal("guard refusal ignored")
			}
		}
	}
}
