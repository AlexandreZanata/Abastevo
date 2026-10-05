package application

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClaimStore struct {
	mu     sync.Mutex
	claims map[string]ClaimRow
	decls  map[string]DeclarationRow
	active map[string]string
	quota  int64
}

func (f *fakeClaimStore) CreateClaim(_ context.Context, id, accountID, stationID, operatorCNPJ, operatorSource, role, scopes, clientKey string) (ClaimRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.claims {
		if row.AccountID == accountID && row.ClientKey == clientKey {
			return ClaimRow{}, false, nil
		}
	}
	row := ClaimRow{ID: id, AccountID: accountID, StationID: stationID, OperatorCNPJ: operatorCNPJ, OperatorSource: operatorSource, Role: role, Scopes: splitScopes(scopes), PolicyVersion: "profile-v1", State: "draft", ClientKey: clientKey}
	if f.claims == nil {
		f.claims = map[string]ClaimRow{}
	}
	f.claims[id] = row
	return row, true, nil
}

func (f *fakeClaimStore) Claim(_ context.Context, id string) (ClaimRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.claims[id]
	if !ok {
		return ClaimRow{}, ErrClaimNotFound
	}
	return row, nil
}

func (f *fakeClaimStore) ClaimByKey(_ context.Context, accountID, clientKey string) (ClaimRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.claims {
		if row.AccountID == accountID && row.ClientKey == clientKey {
			return row, nil
		}
	}
	return ClaimRow{}, ErrClaimNotFound
}

func (f *fakeClaimStore) ListOwnedClaims(_ context.Context, accountID string, _, _ int) ([]ClaimRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ClaimRow
	for _, row := range f.claims {
		if row.AccountID == accountID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeClaimStore) CountOpenClaims(_ context.Context, _ string) (int64, error) {
	return f.quota, nil
}

func (f *fakeClaimStore) SetClaimState(_ context.Context, id, expected, state string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.claims[id]
	if !ok || row.State != expected {
		return 0, nil
	}
	row.State = state
	f.claims[id] = row
	return 1, nil
}

func (f *fakeClaimStore) CreateDeclaration(_ context.Context, id, claimID string, version int, nonceDigest, expectedDigest, declaration string, expiresAt time.Time) (DeclarationRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := DeclarationRow{ID: id, ClaimID: claimID, Version: version, NonceDigest: nonceDigest, ExpectedDigest: expectedDigest, Declaration: declaration, State: "active", ExpiresAt: expiresAt}
	if f.decls == nil {
		f.decls = map[string]DeclarationRow{}
		f.active = map[string]string{}
	}
	f.decls[id] = row
	f.active[claimID] = id
	return row, nil
}

func (f *fakeClaimStore) SupersedeDeclarations(_ context.Context, claimID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, row := range f.decls {
		if row.ClaimID == claimID && row.State == "active" {
			row.State = "superseded"
			f.decls[id] = row
		}
	}
	return nil
}

func (f *fakeClaimStore) ActiveDeclaration(_ context.Context, claimID string) (DeclarationRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.active[claimID]
	if !ok {
		return DeclarationRow{}, ErrClaimNotFound
	}
	if row := f.decls[id]; row.State == "active" {
		return row, nil
	}
	return DeclarationRow{}, ErrClaimNotFound
}

func splitScopes(scopes string) []string {
	if scopes == "" {
		return nil
	}
	return strings.Split(scopes, ",")
}

func claimPorts(store *fakeClaimStore) ClaimPorts {
	n := 0
	return ClaimPorts{
		Store: store,
		Clock: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			n++
			return "claim-id-" + string(rune('0'+n)), nil
		},
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
}

func openTestClaim(t *testing.T, ports ClaimPorts, key string) Claim {
	t.Helper()
	claim, created, err := OpenClaim(context.Background(), ports, "acc-1", "station-1", "administrator", []string{"profile.edit"}, key)
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", claim, created, err)
	}
	return claim
}

func TestOpenClaimBindsDeclaration(t *testing.T) {
	store := &fakeClaimStore{}
	claim := openTestClaim(t, claimPorts(store), "key-1")
	if claim.OperatorCNPJ != "04218406000104" || claim.Version != 1 {
		t.Fatalf("claim = %+v", claim)
	}
	if claim.Declaration == "" || claim.ExpiresAt.IsZero() {
		t.Fatal("declaration must be exportable with expiry")
	}
	if claim.PolicyVersion != "profile-v1" || len(claim.Scopes) != 1 {
		t.Fatalf("claim = %+v", claim)
	}
}

func TestOpenClaimIdempotentConflictQuota(t *testing.T) {
	store := &fakeClaimStore{}
	ports := claimPorts(store)
	first := openTestClaim(t, ports, "key-1")
	second, created, err := OpenClaim(context.Background(), ports, "acc-1", "station-1", "administrator", []string{"profile.edit"}, "key-1")
	if err != nil || created || second.ID != first.ID || second.Declaration != first.Declaration {
		t.Fatalf("replay = %+v, %v, %v", second, created, err)
	}
	if _, _, err := OpenClaim(context.Background(), ports, "acc-1", "station-1", "manager", []string{"profile.edit"}, "key-1"); err != ErrClaimConflict {
		t.Fatalf("changed err = %v", err)
	}
	store.quota = 3
	if _, _, err := OpenClaim(context.Background(), ports, "acc-1", "station-1", "administrator", []string{"profile.edit"}, "key-2"); err != ErrClaimQuota {
		t.Fatalf("quota err = %v", err)
	}
	if _, _, err := OpenClaim(context.Background(), ports, "", "station-1", "administrator", []string{"profile.edit"}, "key-3"); err != ErrClaimAuth {
		t.Fatalf("auth err = %v", err)
	}
	if _, _, err := OpenClaim(context.Background(), ports, "acc-1", "station-1", "owner", []string{"profile.edit"}, "key-4"); err == nil {
		t.Fatal("owner role must fail")
	}
}

func TestReissueVersionsAndSupersedes(t *testing.T) {
	store := &fakeClaimStore{}
	ports := claimPorts(store)
	first := openTestClaim(t, ports, "key-1")
	second, err := ReissueDeclaration(context.Background(), ports, "acc-1", first.ID)
	if err != nil {
		t.Fatalf("reissue: %v", err)
	}
	if second.Version != 2 || second.Declaration == first.Declaration {
		t.Fatalf("reissue = %+v", second)
	}
	active, err := store.ActiveDeclaration(context.Background(), first.ID)
	if err != nil || active.Version != 2 {
		t.Fatalf("active = %+v, err = %v", active, err)
	}
	// Foreign owners cannot reissue (IDOR-safe).
	if _, err := ReissueDeclaration(context.Background(), ports, "acc-2", first.ID); err == nil {
		t.Fatal("foreign reissue must fail")
	}
}

func TestStatusCancelAreOwnerOnly(t *testing.T) {
	store := &fakeClaimStore{}
	ports := claimPorts(store)
	first := openTestClaim(t, ports, "key-1")
	if _, err := ClaimStatus(context.Background(), store, "acc-2", first.ID); err == nil {
		t.Fatal("foreign status must fail")
	}
	if err := CancelClaim(context.Background(), store, "acc-2", first.ID); err == nil {
		t.Fatal("foreign cancel must fail")
	}
	if err := CancelClaim(context.Background(), store, "acc-1", first.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := CancelClaim(context.Background(), store, "acc-1", first.ID); err != ErrClaimClosed {
		t.Fatalf("double cancel err = %v", err)
	}
}
