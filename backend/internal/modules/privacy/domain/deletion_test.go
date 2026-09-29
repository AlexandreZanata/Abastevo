package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLedgerEntryValid(t *testing.T) {
	e, err := NewLedgerEntry(LedgerParams{
		ID:            "d0000000-0000-4000-8000-000000000001",
		ContributorID: "c1", ContributorRef: "tok-c1",
		Scope: ScopeCommunity, Reason: "owner erasure request",
		OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("ledger entry rejected: %v", err)
	}
	if e.PolicyVersion != PolicyV1 || !e.ReplayedAt.IsZero() {
		t.Errorf("entry = %+v", e)
	}
	if len(AllScopes()) != 5 {
		t.Errorf("scopes = %v", AllScopes())
	}
	for _, scope := range AllScopes() {
		p := LedgerParams{
			ID:            "d0000000-0000-4000-8000-000000000001",
			ContributorID: "c1", ContributorRef: "tok-c1",
			Scope: scope, Reason: "r", OccurredAt: time.Now(),
		}
		if _, err := NewLedgerEntry(p); err != nil {
			t.Errorf("scope %s rejected: %v", scope, err)
		}
	}
	if _, err := NewLedgerEntry(LedgerParams{
		ID: "x", ContributorID: "c1", ContributorRef: "tok",
		Scope: "BACKUPS", Reason: "r", OccurredAt: time.Now(),
	}); !errors.Is(err, ErrUnknownScope) {
		t.Errorf("unknown scope = %v", err)
	}
	if _, err := NewLedgerEntry(LedgerParams{
		ID: "x", ContributorID: "", ContributorRef: "tok",
		Scope: ScopeIdentity, Reason: "r", OccurredAt: time.Now(),
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("ownerless ledger = %v", err)
	}
	if _, err := NewLedgerEntry(LedgerParams{
		ID: "x", ContributorID: "c1", ContributorRef: "tok",
		Scope: ScopeIdentity, Reason: strings.Repeat("r", MaxReasonChars+1),
		OccurredAt: time.Now(),
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("long reason accepted")
	}
}

func TestCompleteDeletion(t *testing.T) {
	r, err := NewRequest(RequestParams{
		ID:            "d0000000-0000-4000-8000-000000000001",
		ContributorID: "c1", ContributorRef: "tok-c1",
		ClientSubmissionID: "erase-1", Type: TypeDeletion,
		RequestedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	done, evt, err := CompleteDeletion(r, at)
	if err != nil {
		t.Fatalf("complete deletion = %v", err)
	}
	if done.Status != StatusReady || !done.CompletedAt.Equal(at) {
		t.Errorf("completed = %+v", done)
	}
	if evt.Name() != "PrivacyRequestCompleted" || evt.Type != TypeDeletion {
		t.Errorf("event = %+v", evt)
	}
	if _, _, err := CompleteDeletion(done, at); err != ErrBadState {
		t.Errorf("double complete = %v", err)
	}
	exp, err := NewRequest(RequestParams{
		ID:            "e0000000-0000-4000-8000-000000000001",
		ContributorID: "c1", ContributorRef: "tok-c1",
		ClientSubmissionID: "export-1", Type: TypeExport,
		RequestedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CompleteDeletion(exp, at); err != ErrInvalidRequest {
		t.Errorf("export completed as deletion = %v", err)
	}
}
