package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type fakeSessions struct {
	status   string
	verified int
	rejected int
	reasons  []string
}

func (f *fakeSessions) Session(_ context.Context, id string) (domain.Session, error) {
	if id == "" {
		return domain.Session{}, errors.New("unknown session")
	}
	now := time.Now()
	s, _, err := domain.NewSession(domain.Params{
		ID: id, ContributorRef: "tok-c1",
		ClientSessionID: "upl-1", MIME: "image/jpeg", DeclaredBytes: 100,
		ClaimedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		QuarantineKey: "q/k", CreatedAt: now,
	})
	if err != nil {
		panic(err)
	}
	s.Status = f.status
	return s, nil
}

func (f *fakeSessions) RecordVerified(_ context.Context, sessionID string, obj parent.ObjectData) (string, error) {
	f.verified++
	return "e0000000-0000-4000-8000-000000000009", nil
}

func (f *fakeSessions) RecordRejected(_ context.Context, sessionID string, reasons []string) error {
	f.rejected++
	f.reasons = append([]string{}, reasons...)
	return nil
}

func testHandler(store *fakeSessions, verify func(context.Context, domain.Session) (evidenceapp.Outcome, error)) Verify {
	return Verify{
		Store:  store,
		Verify: verify,
		NewID:  func() (string, error) { return "e0000000-0000-4000-8000-000000000009", nil },
	}
}

func okOutcome() evidenceapp.Outcome {
	return evidenceapp.Outcome{FinalKey: "f/x", SourceSHA256: "a", Width: 4, Height: 4}
}

func TestHandleRejectsBadPayload(t *testing.T) {
	h := testHandler(&fakeSessions{}, func(context.Context, domain.Session) (evidenceapp.Outcome, error) {
		return okOutcome(), nil
	})
	for name, raw := range map[string]string{
		"malformed":  `{`,
		"version":    `{"version":2,"upload_id":"s1"}`,
		"empty-id":   `{"version":1,"upload_id":"  "}`,
		"missing-id": `{"version":1}`,
	} {
		if err := h.Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(raw)}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := (Verify{}).Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(`{"version":1,"upload_id":"s1"}`)}); err == nil {
		t.Error("nil dependencies accepted")
	}
}

func TestHandleTerminalIsIdempotent(t *testing.T) {
	for _, state := range []string{domain.StateReady, domain.StateRejected, domain.StateExpired} {
		store := &fakeSessions{status: state}
		called := false
		h := testHandler(store, func(context.Context, domain.Session) (evidenceapp.Outcome, error) {
			called = true
			return okOutcome(), nil
		})
		if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"upload_id":"s1"}`)}); err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		if called || store.verified+store.rejected != 0 {
			t.Errorf("%s replay caused work", state)
		}
	}
}

func TestHandleSpuriousIssuedFails(t *testing.T) {
	// A verify job for a never-completed session is a bug, not a pass:
	// fail toward DEAD with cause instead of succeeding silently.
	h := testHandler(&fakeSessions{status: domain.StateIssued}, func(context.Context, domain.Session) (evidenceapp.Outcome, error) {
		return okOutcome(), nil
	})
	if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"upload_id":"s1"}`)}); err == nil {
		t.Error("issued session completed")
	}
}

func TestHandleVerifiesAndPersists(t *testing.T) {
	store := &fakeSessions{status: domain.StateVerifying}
	var got domain.Session
	h := testHandler(store, func(_ context.Context, s domain.Session) (evidenceapp.Outcome, error) {
		got = s
		return okOutcome(), nil
	})
	if err := h.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"upload_id":"s1"}`)}); err != nil {
		t.Fatalf("handle = %v", err)
	}
	if got.ID != "s1" || store.verified != 1 {
		t.Errorf("verified = %d for %q", store.verified, got.ID)
	}
}

func TestHandleMapsRejectionAndTransient(t *testing.T) {
	store := &fakeSessions{status: domain.StateVerifying}
	rejecting := testHandler(store, func(context.Context, domain.Session) (evidenceapp.Outcome, error) {
		return evidenceapp.Outcome{}, &evidenceapp.RejectionError{Reasons: []string{"invalid-image"}}
	})
	if err := rejecting.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"upload_id":"s1"}`)}); err != nil {
		t.Fatalf("rejection = %v", err)
	}
	if store.rejected != 1 || len(store.reasons) != 1 || store.reasons[0] != "invalid-image" {
		t.Errorf("rejected = %d %v", store.rejected, store.reasons)
	}
	boom := errors.New("r2 down")
	failing := testHandler(store, func(context.Context, domain.Session) (evidenceapp.Outcome, error) {
		return evidenceapp.Outcome{}, boom
	})
	if err := failing.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"upload_id":"s1"}`)}); !errors.Is(err, boom) {
		t.Errorf("transient = %v", err)
	}
}
