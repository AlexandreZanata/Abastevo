package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func gatePorts(block error, quotaCalled *bool) Ports {
	p := testPorts()
	p.CheckAccount = func(context.Context, string) error { return block }
	p.CheckQuota = func(context.Context, string, string) (time.Duration, error) {
		*quotaCalled = true
		return 0, nil
	}
	return p
}

func TestSubmitAccountGate(t *testing.T) {
	ctx := context.Background()
	var quotaCalled bool

	// Blocked contributors refuse before quota windows burn.
	p := gatePorts(ErrAccountBlocked, &quotaCalled)
	if _, err := Submit(ctx, p, testCaller(), "POST", "/v1/observations", "k", []byte(`{}`), testDTO()); !errors.Is(err, ErrAccountBlocked) {
		t.Errorf("blocked submit = %v", err)
	}
	if quotaCalled {
		t.Error("blocked submit must not reach quota")
	}

	// Gate errors pass through untouched.
	p = gatePorts(errors.New("gate unavailable"), &quotaCalled)
	if _, err := Submit(ctx, p, testCaller(), "POST", "/v1/observations", "k", []byte(`{}`), testDTO()); err == nil || errors.Is(err, ErrAccountBlocked) {
		t.Errorf("gate error must propagate, got %v", err)
	}

	// Nil gate preserves the anonymous baseline: the write proceeds
	// to quota as before.
	p = testPorts()
	p.CheckAccount = nil
	p.CheckQuota = func(context.Context, string, string) (time.Duration, error) {
		return 0, &QuotaDeniedError{RetryAfter: time.Second}
	}
	if _, err := Submit(ctx, p, testCaller(), "POST", "/v1/observations", "k", []byte(`{}`), testDTO()); !errors.Is(err, ErrQuotaDenied) {
		t.Errorf("nil gate must preserve baseline, got %v", err)
	}
}

func gateVotePorts(block error) VotePorts {
	return VotePorts{
		Clock: func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) { return "d6c74c23-63db-4c24-a2e5-408cb23bad27", nil },
		CheckQuota: func(context.Context, string, string) (time.Duration, error) {
			return 0, nil
		},
		CheckAccount: func(context.Context, string) error { return block },
		EnqueueJob:   func(context.Context, pgx.Tx, string, []byte, string) error { return nil },
		Store:        nil,
	}
}

func TestVotesAccountGate(t *testing.T) {
	ctx := context.Background()
	caller := Caller{ContributorID: "c1", Fingerprint: "fp:x", KeyID: "k1", Token: "tok-c1"}

	p := gateVotePorts(ErrAccountBlocked)
	if _, err := Confirm(ctx, p, caller, ConfirmDTO{ObservationID: "o1", ClientSubmissionID: "c"}); !errors.Is(err, ErrAccountBlocked) {
		t.Errorf("blocked confirm = %v", err)
	}
	if _, err := Dispute(ctx, p, caller, DisputeDTO{TargetObservationID: "o1", ClientSubmissionID: "c", Reason: "OTHER"}); !errors.Is(err, ErrAccountBlocked) {
		t.Errorf("blocked dispute = %v", err)
	}

	// Nil gates preserve the baseline: every pre-existing vote test
	// constructs VotePorts without CheckAccount and keeps passing.
}
