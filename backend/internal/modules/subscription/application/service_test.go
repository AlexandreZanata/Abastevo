package application

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"testing"
)

type probeStore struct{ called bool }

func (s *probeStore) Begin(context.Context, Attempt) (Attempt, error) {
	s.called = true
	return Attempt{}, domain.ErrUnavailable
}
func (s *probeStore) Commit(context.Context, Attempt, VerifiedPurchase) error {
	s.called = true
	return nil
}
func (s *probeStore) Reject(context.Context, Attempt) error                       { return nil }
func (s *probeStore) Status(context.Context, Principal) (*domain.Snapshot, error) { return nil, nil }
func TestUnconfiguredPaymentsCannotPersistOrGrant(t *testing.T) {
	store := &probeStore{}
	svc := &Service{Store: store}
	p := Principal{AccountID: "11111111-1111-4111-8111-111111111111", FamilyID: "22222222-2222-4222-8222-222222222222", ContributorID: "33333333-3333-4333-8333-333333333333", KeyID: "44444444-4444-4444-8444-444444444444"}
	_, err := svc.Admit(context.Background(), p, Command{OperationID: "55555555-5555-4555-8555-555555555555", Token: "synthetic", Kind: "verify"})
	if !errors.Is(err, domain.ErrUnavailable) || store.called {
		t.Fatal("disabled provider persisted/granted")
	}
	if _, err := svc.Reconcile(context.Background()); !errors.Is(err, domain.ErrUnavailable) || store.called {
		t.Fatal("disabled reconciliation started")
	}
	got, err := svc.Status(context.Background(), p)
	if err != nil || got.PaymentsConfigured || len(got.Capabilities) != 0 || !got.RetainedDataAccess {
		t.Fatal("disabled/free-exit status invalid")
	}
}
