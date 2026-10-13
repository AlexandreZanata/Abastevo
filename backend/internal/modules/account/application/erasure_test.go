package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recoveryStore struct {
	claim                 ErasureClaim
	found                 bool
	ackCalled, ackSuccess bool
	ackErr                error
}

func (s *recoveryStore) ClaimErasure(context.Context, string, int64) (ErasureClaim, bool, error) {
	return s.claim, s.found, nil
}
func (s *recoveryStore) FinishErasure(ctx context.Context, _ ErasureClaim, success bool, _ int64) error {
	s.ackCalled = true
	s.ackSuccess = success
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.ackErr
}
func TestRecoveryAcknowledgesFailureAfterCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &recoveryStore{claim: ErasureClaim{AccountID: "a1", ContributorID: "c1", Nonce: "n1"}, found: true}
	scopeErr := errors.New("reply lost after effect")
	r := ErasureRecovery{Store: store, NewNonce: func() (string, error) { return "n1", nil }, Now: time.Now, EraseFeedback: func(context.Context, string) error { cancel(); return scopeErr }, EraseContributor: func(context.Context, string, string) error { t.Fatal("subsequent scope ran after failure"); return nil }}
	if err := r.Reconcile(ctx); !errors.Is(err, scopeErr) {
		t.Fatal(err)
	}
	if !store.ackCalled || store.ackSuccess {
		t.Fatal("failed scope falsely completed or acknowledgement was lost")
	}
}
func TestRecoveryDoesNotCompleteWhenAcknowledgementFails(t *testing.T) {
	store := &recoveryStore{claim: ErasureClaim{AccountID: "a1", Nonce: "n1"}, found: true, ackErr: errors.New("database unavailable")}
	r := ErasureRecovery{Store: store, NewNonce: func() (string, error) { return "n1", nil }, Now: time.Now, EraseFeedback: func(context.Context, string) error { return nil }, EraseContributor: func(context.Context, string, string) error { t.Fatal("binding-free account expanded"); return nil }}
	if err := r.Reconcile(context.Background()); !errors.Is(err, store.ackErr) {
		t.Fatal(err)
	}
}
