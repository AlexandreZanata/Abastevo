package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func testPorts() Ports {
	return Ports{
		Clock: func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) { return "d6c74c23-63db-4c24-a2e5-408cb23bad27", nil },
		Attribution: func(_ context.Context, id string) (string, error) {
			return "tok-" + id[:8], nil
		},
		CheckQuota: func(context.Context, string, string) (time.Duration, error) { return 0, nil },
		Idempotent: func(_ context.Context, _ IdempotencyKey, _ []byte, run func(context.Context) (Outcome, error)) (Outcome, error) {
			return run(context.Background())
		},
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error { return nil },
		Store:      nil,
	}
}

func testCaller() Caller {
	return Caller{ContributorID: "c1", Fingerprint: "fp:x", KeyID: "k1", Token: "tok-c1"}
}

func testDTO() SubmitDTO {
	return SubmitDTO{
		ClientSubmissionID: "sub-1", StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999, RawText: "5,999",
		ConditionKind:     "STANDARD",
		ClaimedCapturedAt: time.Date(2026, 9, 28, 11, 0, 0, 0, time.UTC),
	}
}

func TestSubmitValidation(t *testing.T) {
	// Store is nil here: validation failures must precede any persistence.
	p := testPorts()
	if _, err := Submit(context.Background(), p, Caller{}, "POST", "/v1/observations", "k", []byte(`{}`), testDTO()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anonymous = %v", err)
	}
	badCaller := testCaller()
	badCaller.Token = ""
	if _, err := Submit(context.Background(), p, badCaller, "POST", "/v1/observations", "k", []byte(`{}`), testDTO()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("tokenless = %v", err)
	}
}

func TestSubmitQuotaAndConflictMapping(t *testing.T) {
	p := testPorts()
	p.CheckQuota = func(context.Context, string, string) (time.Duration, error) {
		return 0, &QuotaDeniedError{RetryAfter: 30 * time.Second}
	}
	if _, err := Submit(context.Background(), p, testCaller(), "POST", "/v1/observations", "k", []byte(`{}`), testDTO()); !errors.Is(err, ErrQuotaDenied) {
		t.Errorf("quota = %v", err)
	}
	// The denial carries the delay for 429 mapping.
	if _, err := Submit(context.Background(), p, testCaller(), "POST", "/v1/observations", "k", []byte(`{}`), testDTO()); err == nil {
		t.Error("quota allowed")
	} else {
		var denied *QuotaDeniedError
		if !errors.As(err, &denied) || denied.RetryAfter != 30*time.Second {
			t.Errorf("denial = %v", err)
		}
	}
	p.CheckQuota = func(context.Context, string, string) (time.Duration, error) { return 0, nil }
	p.Idempotent = func(_ context.Context, _ IdempotencyKey, _ []byte, _ func(context.Context) (Outcome, error)) (Outcome, error) {
		return Outcome{}, ErrConflict
	}
	if _, err := Submit(context.Background(), p, testCaller(), "POST", "/v1/observations", "k", []byte(`{}`), testDTO()); !errors.Is(err, ErrConflict) {
		t.Errorf("conflict = %v", err)
	}
}
