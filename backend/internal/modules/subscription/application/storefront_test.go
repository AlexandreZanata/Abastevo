package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

// memoryStore is a single-purchase in-memory store for routing tests.
type memoryStore struct {
	begun     []Attempt
	committed []VerifiedPurchase
	snapshot  *domain.Snapshot
	jobRow    StoredToken
	jobErr    error
}

func (s *memoryStore) Begin(_ context.Context, a Attempt) (Attempt, error) {
	s.begun = append(s.begun, a)
	a.Generation = 1
	return a, nil
}
func (s *memoryStore) Commit(_ context.Context, a Attempt, v VerifiedPurchase) error {
	s.committed = append(s.committed, v)
	snap := v.Snapshot
	s.snapshot = &snap
	return nil
}
func (s *memoryStore) Reject(context.Context, Attempt) error { return nil }
func (s *memoryStore) Status(context.Context, Principal) (*domain.Snapshot, error) {
	return s.snapshot, nil
}
func (s *memoryStore) ForJob(context.Context, string) (StoredToken, error)   { return s.jobRow, s.jobErr }
func (s *memoryStore) MarkAcknowledged(context.Context, string, int64) error { return nil }
func (s *memoryStore) ReceiveNotification(context.Context, Notification) (bool, error) {
	return false, nil
}
func (s *memoryStore) BeginRefresh(_ context.Context, row StoredToken, op string) (Attempt, error) {
	return Attempt{Principal: Principal{AccountID: row.AccountID}, Provider: row.Provider, OperationID: op, Digest: row.Digest, Environment: row.Environment, PackageName: row.PackageName, Generation: 1, Background: true}, nil
}

type identityCodec struct{}

func (identityCodec) Seal(token, _ string) ([]byte, error) { return []byte(token), nil }
func (identityCodec) Open(raw []byte, _ string) (string, error) {
	return string(raw), nil
}

type countingVerifier struct {
	calls    int
	provider string
	err      error
	now      time.Time
}

func (v *countingVerifier) Verify(_ context.Context, r ProviderRequest) (ProviderPurchase, error) {
	v.calls++
	if v.err != nil {
		return ProviderPurchase{}, v.err
	}
	return ProviderPurchase{Snapshot: domain.Snapshot{Provider: v.provider, State: domain.Active, ValidUntil: v.now.Add(time.Hour), ProductID: "p", BasePlanID: "p", Acknowledged: true}}, nil
}

var owner = Principal{AccountID: "11111111-1111-4111-8111-111111111111", FamilyID: "22222222-2222-4222-8222-222222222222", ContributorID: "33333333-3333-4333-8333-333333333333", KeyID: "44444444-4444-4444-8444-444444444444"}

func storefrontFixture(now time.Time) (Storefront, *memoryStore, *countingVerifier, *countingVerifier) {
	store := &memoryStore{}
	clock := func() time.Time { return now }
	playVerifier := &countingVerifier{provider: domain.ProviderPlay, now: now}
	appleVerifier := &countingVerifier{provider: domain.ProviderAppStore, now: now}
	playService := &Service{Store: store, Provider: playVerifier, Tokens: identityCodec{}, Clock: clock, Environment: "test", PackageName: "com.anpfuel.app",
		AccountBinding: func(string) string { return "hex-binding" }, StoreName: domain.ProviderPlay,
		Catalog: []Product{{ProductID: "abastevo_premium", PlanIDs: []string{"monthly"}}}, ManagementURL: "https://play.google.com/store/account/subscriptions?package=com.anpfuel.app"}
	appleService := &Service{Store: store, Provider: appleVerifier, Tokens: identityCodec{}, Clock: clock, Environment: "test", PackageName: "com.anpfuel.app",
		AccountBinding: func(string) string { return "6f1c2b8e-6d2a-8c3e-9a41-2b7d5e0c1f34" }, StoreName: domain.ProviderAppStore,
		Catalog: []Product{{ProductID: "com.anpfuel.app.premium.monthly"}}, ManagementURL: "https://apps.apple.com/account/subscriptions"}
	return Storefront{Services: []*Service{playService, appleService}}, store, playVerifier, appleVerifier
}

func TestStorefrontRoutesCommandsToTheIssuingStore(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	front, store, playVerifier, appleVerifier := storefrontFixture(now)
	receipt, err := front.Admit(context.Background(), owner, Command{OperationID: "55555555-5555-4555-8555-555555555555", Token: "2000000123456789", Kind: "verify", Provider: domain.ProviderAppStore})
	if err != nil {
		t.Fatal(err)
	}
	if appleVerifier.calls != 1 || playVerifier.calls != 0 {
		t.Fatalf("calls play=%d apple=%d", playVerifier.calls, appleVerifier.calls)
	}
	begun := store.begun[0]
	if begun.Provider != domain.ProviderAppStore || begun.Digest != domain.StoreTokenDigest(domain.ProviderAppStore, "test", "com.anpfuel.app", "2000000123456789") {
		t.Fatalf("attempt %+v", begun)
	}
	if !receipt.Status.PaymentsConfigured || receipt.Status.State != domain.Active || receipt.Status.Tiers[0].Source != domain.ProviderAppStore {
		t.Fatalf("receipt %+v", receipt.Status)
	}
	if _, err := front.Admit(context.Background(), owner, Command{OperationID: "55555555-5555-4555-8555-555555555556", Token: "purchase-token", Kind: "verify"}); err != nil || playVerifier.calls != 1 {
		t.Fatalf("legacy commands must reach Play: %v", err)
	}
	if _, err := front.Admit(context.Background(), owner, Command{OperationID: "55555555-5555-4555-8555-555555555557", Token: "x", Kind: "verify", Provider: "web"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown store: %v", err)
	}
}

func TestStorefrontOffersOnlyConfiguredStores(t *testing.T) {
	front, _, _, _ := storefrontFixture(time.Now())
	offers := front.Offers(owner)
	if len(offers) != 2 || offers[0].Provider != domain.ProviderPlay || offers[1].AccountBinding != "6f1c2b8e-6d2a-8c3e-9a41-2b7d5e0c1f34" {
		t.Fatalf("offers %+v", offers)
	}
	front.Services[1].Provider = nil
	if offers = front.Offers(owner); len(offers) != 1 || offers[0].Provider != domain.ProviderPlay {
		t.Fatalf("unconfigured App Store offered: %+v", offers)
	}
	if _, err := front.Admit(context.Background(), owner, Command{OperationID: "55555555-5555-4555-8555-555555555555", Token: "1", Kind: "verify", Provider: domain.ProviderAppStore}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("unconfigured store admitted: %v", err)
	}
	if offers = front.Offers(Principal{AccountID: "bad"}); len(offers) != 0 {
		t.Fatal("offers for an invalid account")
	}
}

func TestStorefrontStatusKeepsAnotherStoresAccess(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	front, store, _, _ := storefrontFixture(now)
	store.snapshot = &domain.Snapshot{Provider: domain.ProviderAppStore, State: domain.Active, VerifiedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Hour), ProductID: "p", BasePlanID: "p"}
	front.Services[0].Provider = nil // Play off, App Store on.
	status, err := front.Status(context.Background(), owner)
	if err != nil || !status.PaymentsConfigured || len(status.Capabilities) == 0 || status.Reason != "verified" {
		t.Fatalf("status %+v %v", status, err)
	}
	front.Services[1].Provider = nil // both off: rights are not served.
	if status, _ = front.Status(context.Background(), owner); status.PaymentsConfigured || len(status.Capabilities) != 0 {
		t.Fatalf("disabled stores still grant %+v", status)
	}
}

func TestStorefrontJobsUseThePurchaseStore(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	front, store, playVerifier, appleVerifier := storefrontFixture(now)
	digest := domain.StoreTokenDigest(domain.ProviderAppStore, "test", "com.anpfuel.app", "2000000123456789")
	store.jobRow = StoredToken{Provider: domain.ProviderAppStore, AccountID: owner.AccountID, Environment: "test", PackageName: "com.anpfuel.app", Digest: digest, CipherToken: []byte("2000000123456789")}
	if err := front.Refresh(context.Background(), digest, "66666666-6666-4666-8666-666666666666"); err != nil {
		t.Fatal(err)
	}
	if appleVerifier.calls != 1 || playVerifier.calls != 0 || len(store.committed) != 1 {
		t.Fatalf("refresh routed play=%d apple=%d commits=%d", playVerifier.calls, appleVerifier.calls, len(store.committed))
	}
	front.Services[1].Provider = nil
	if err := front.Refresh(context.Background(), digest, "66666666-6666-4666-8666-666666666667"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("refresh with the store off must stay retryable: %v", err)
	}
	store.jobErr = domain.ErrNotFound
	if err := front.Acknowledge(context.Background(), digest); err != nil {
		t.Fatalf("gone purchase: %v", err)
	}
	// A Play service must refuse an App Store row even if called directly.
	store.jobErr = nil
	if err := front.Services[0].Refresh(context.Background(), digest, "66666666-6666-4666-8666-666666666668"); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("cross-store refresh: %v", err)
	}
}
