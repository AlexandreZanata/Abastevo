package application

import (
	"context"
	"errors"
	"testing"
)

// P33-T02 — End-to-end authority lifecycle on the updated candidate.
//
// open → reissue (versions, supersedes) → approve (grant) → terminal:
// cancel/reissue past approval fail closed, rival owners stay isolated,
// and competing cases never leak across accounts. Device/provider rows
// stay OWED at conjunto closure; this locks the source lifecycle.

func TestAuthorityLifecycleTerminalClosed(t *testing.T) {
	ctx := context.Background()
	store := &fakeClaimStore{}
	decisions := &fakeDecisions{}
	cports := claimPorts(store)
	rports := reviewTestPorts(store, decisions)

	opened, created, err := OpenClaim(ctx, cports, "acc-1", "station-1", "administrator", []string{"profile.edit"}, "lifecycle-1")
	if err != nil || !created {
		t.Fatalf("open: %+v %v", opened, err)
	}
	reissued, err := ReissueDeclaration(ctx, cports, "acc-1", opened.ID)
	if err != nil {
		t.Fatalf("reissue: %v", err)
	}
	if reissued.Version != 2 {
		t.Fatalf("version = %d, want 2 (supersede chain)", reissued.Version)
	}
	if err := ReviewClaim(ctx, rports, opened.ID, "op-1", true, "", goodEvidence()); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := CancelClaim(ctx, store, "acc-1", opened.ID); !errors.Is(err, ErrClaimClosed) {
		t.Fatalf("cancel past approval = %v, want ErrClaimClosed", err)
	}
	if _, err := ReissueDeclaration(ctx, cports, "acc-1", opened.ID); !errors.Is(err, ErrClaimClosed) {
		t.Fatalf("reissue past approval = %v, want ErrClaimClosed", err)
	}
	if _, err := ClaimStatus(ctx, store, "acc-rival", opened.ID); !errors.Is(err, ErrClaimNotFound) {
		t.Fatalf("rival status = %v, want not-found (no ownership oracle)", err)
	}
	if err := CancelClaim(ctx, store, "acc-rival", opened.ID); !errors.Is(err, ErrClaimNotFound) {
		t.Fatalf("rival cancel = %v, want not-found", err)
	}
	mine, err := ListMine(ctx, store, "acc-rival")
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 0 {
		t.Fatalf("rival sees %d claims, want 0", len(mine))
	}
}
