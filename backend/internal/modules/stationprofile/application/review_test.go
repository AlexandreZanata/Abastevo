package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/authority"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/verify"
)

type fakeDecisions struct {
	mu      sync.Mutex
	claims  *fakeClaimStore
	grants  map[string]GrantRow
	decided map[string]bool
}

func (f *fakeDecisions) DecideAtomically(_ context.Context, decision DecisionInput, grant *GrantInput, newState string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.decided == nil {
		f.decided = map[string]bool{}
	}
	if f.decided[decision.ClaimID] {
		return ErrVerifyClosed
	}
	f.decided[decision.ClaimID] = true
	f.claims.setState(decision.ClaimID, newState)
	if grant != nil {
		if f.grants == nil {
			f.grants = map[string]GrantRow{}
		}
		key := grant.AccountID + "|" + grant.StationID
		if _, ok := f.grants[key]; !ok {
			f.grants[key] = GrantRow{
				ID: grant.ID, AccountID: grant.AccountID, StationID: grant.StationID,
				OperatorCNPJ: grant.OperatorCNPJ, Role: grant.Role, Scopes: grant.Scopes,
				Status: "active", Version: 1,
			}
		}
	}
	_ = newState
	return nil
}

func (f *fakeDecisions) ActiveGrant(_ context.Context, accountID, stationID string) (GrantRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	grant, ok := f.grants[accountID+"|"+stationID]
	return grant, ok, nil
}

func reviewTestPorts(store *fakeClaimStore, decisions *fakeDecisions) ReviewPorts {
	decisions.claims = store
	n := 0
	return ReviewPorts{
		Claims:    store,
		Decisions: decisions,
		OperatorOf: func(context.Context, string) (string, bool, error) {
			return "04218406000104", true, nil
		},
		AccountLive: func(context.Context, string) (bool, error) { return true, nil },
		NewID: func() (string, error) {
			n++
			return "review-id-" + string(rune('0'+n)), nil
		},
	}
}

func openReviewClaim(t *testing.T, store *fakeClaimStore, key string) ClaimRow {
	t.Helper()
	ports := claimPorts(store)
	claim, created, err := OpenClaim(context.Background(), ports, "acc-1", "station-1", "administrator", []string{"profile.edit"}, key)
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", claim, created, err)
	}
	row, err := store.Claim(context.Background(), claim.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return row
}

func goodEvidence() ReviewEvidence {
	return ReviewEvidence{
		Signature: verify.Result{Outcome: verify.OutcomeValid, Signer: "CN=Tester"},
		Authority: authority.AuthorityResult{Outcome: authority.OutcomeSufficient},
	}
}

func TestReviewApproveGrantsNarrowly(t *testing.T) {
	store := &fakeClaimStore{}
	decisions := &fakeDecisions{}
	row := openReviewClaim(t, store, "review-1")
	ports := reviewTestPorts(store, decisions)

	if err := ReviewClaim(context.Background(), ports, row.ID, "op-1", true, "", goodEvidence()); err != nil {
		t.Fatalf("review: %v", err)
	}
	grant, found, err := decisions.ActiveGrant(context.Background(), "acc-1", "station-1")
	if err != nil || !found {
		t.Fatalf("grant = %+v, %v, %v", grant, found, err)
	}
	if grant.Role != "administrator" || grant.Status != "active" {
		t.Fatalf("grant = %+v", grant)
	}
}

func TestReviewRefusesSelfStaleDeniedAndBadEvidence(t *testing.T) {
	store := &fakeClaimStore{}
	decisions := &fakeDecisions{}
	row := openReviewClaim(t, store, "review-2")
	ports := reviewTestPorts(store, decisions)

	if err := ReviewClaim(context.Background(), ports, row.ID, "acc-1", true, "", goodEvidence()); err != ErrReviewSelf {
		t.Fatalf("self err = %v", err)
	}
	badSignature := goodEvidence()
	badSignature.Signature = verify.Result{Outcome: verify.OutcomeInvalid}
	if err := ReviewClaim(context.Background(), ports, row.ID, "op-1", true, "", badSignature); err != ErrReviewDenied {
		t.Fatalf("signature err = %v", err)
	}
	badAuthority := goodEvidence()
	badAuthority.Authority = authority.AuthorityResult{Outcome: authority.OutcomeDenied}
	if err := ReviewClaim(context.Background(), ports, row.ID, "op-1", true, "", badAuthority); err != ErrReviewDenied {
		t.Fatalf("authority err = %v", err)
	}
	stalePorts := ports
	stalePorts.OperatorOf = func(context.Context, string) (string, bool, error) {
		return "00428184000195", true, nil
	}
	if err := ReviewClaim(context.Background(), stalePorts, row.ID, "op-1", true, "", goodEvidence()); err != ErrReviewStale {
		t.Fatalf("stale err = %v", err)
	}
	if err := ReviewClaim(context.Background(), ports, row.ID, "op-1", false, "", goodEvidence()); err != ErrVerifyReason {
		t.Fatalf("reason err = %v", err)
	}
	if err := ReviewClaim(context.Background(), ports, row.ID, "op-1", false, "not eligible", goodEvidence()); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if err := ReviewClaim(context.Background(), ports, row.ID, "op-1", true, "", goodEvidence()); err == nil {
		t.Fatal("decided claim must refuse further review")
	}
	if err := ReviewClaim(context.Background(), ports, row.ID, "", true, "", goodEvidence()); err != ErrReviewOperator {
		t.Fatalf("reviewer err = %v", err)
	}
}

func TestReviewConcurrentApprovalsConverge(t *testing.T) {
	store := &fakeClaimStore{}
	decisions := &fakeDecisions{}
	row := openReviewClaim(t, store, "review-3")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	ports := reviewTestPorts(store, decisions)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = ReviewClaim(context.Background(), ports, row.ID, "op-1", true, "", goodEvidence())
		}(i)
	}
	wg.Wait()
	failed := 0
	for _, err := range errs {
		if err != nil && !errors.Is(err, ErrVerifyClosed) && !errors.Is(err, ErrClaimClosed) {
			t.Fatalf("unexpected err: %v", err)
		}
		if err != nil {
			failed++
		}
	}
	_ = failed
	grant, found, err := decisions.ActiveGrant(context.Background(), "acc-1", "station-1")
	if err != nil || !found {
		t.Fatalf("grant = %+v, %v, %v", grant, found, err)
	}
}
