package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// fakeCompleteStore mimics the SQL guards: conditional transitions only,
// natural misses as not-found, and one object per READY session.
type fakeCompleteStore struct {
	sessions map[string]domain.Session
	objects  map[string]string
	jobs     int
}

func newFakeCompleteStore() *fakeCompleteStore {
	return &fakeCompleteStore{sessions: map[string]domain.Session{}, objects: map[string]string{}}
}

func (f *fakeCompleteStore) Session(_ context.Context, id string) (domain.Session, error) {
	s, ok := f.sessions[id]
	if !ok {
		return domain.Session{}, errors.New("adapters: unknown session")
	}
	return s, nil
}

func (f *fakeCompleteStore) CompleteSession(_ context.Context, id string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) error {
	s, ok := f.sessions[id]
	if !ok {
		return errors.New("adapters: unknown session")
	}
	if s.Status != domain.StateIssued && s.Status != domain.StateVerifying {
		return domain.ErrBadTransition
	}
	if s.Status == domain.StateIssued {
		s.Status = domain.StateVerifying
		s.UpdatedAt = time.Now()
		f.sessions[id] = s
	}
	if enqueue != nil {
		if err := enqueue(context.Background(), nil, "verify-evidence", []byte(`{}`), "verify:"+id); err != nil {
			return err
		}
	}
	f.jobs++
	return nil
}

func (f *fakeCompleteStore) ObjectIDBySession(_ context.Context, sessionID string) (string, error) {
	id, ok := f.objects[sessionID]
	if !ok {
		return "", errors.New("evidence: no verified object for session")
	}
	return id, nil
}

func completePorts(f *fakeCompleteStore) CompletePorts {
	return CompletePorts{
		Store: f,
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error {
			return nil
		},
	}
}

func issuedFixture() domain.Session {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	s, _, err := domain.NewSession(domain.Params{
		ID: "e0000000-0000-4000-8000-000000000001", ContributorRef: "tok-c1",
		ClientSessionID: "upl-1", MIME: "image/jpeg", DeclaredBytes: 100,
		ClaimedSHA256: strings.Repeat("a", 64), QuarantineKey: "q/k",
		CreatedAt: now,
	})
	if err != nil {
		panic(err)
	}
	return s
}

func TestCompleteClaimsAndReplays(t *testing.T) {
	f := newFakeCompleteStore()
	f.sessions["e0000000-0000-4000-8000-000000000001"] = issuedFixture()
	caller := testCaller()
	first, err := Complete(context.Background(), completePorts(f), caller, "e0000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatalf("complete = %v", err)
	}
	if first.State != domain.StateVerifying || first.EvidenceID != "" {
		t.Errorf("view = %+v", first)
	}
	second, err := Complete(context.Background(), completePorts(f), caller, "e0000000-0000-4000-8000-000000000001")
	if err != nil || second.State != domain.StateVerifying {
		t.Errorf("replay = %+v, %v", second, err)
	}
}

func TestCompleteHidesForeignAndMissing(t *testing.T) {
	f := newFakeCompleteStore()
	f.sessions["e0000000-0000-4000-8000-000000000001"] = issuedFixture()
	other := testCaller()
	other.Token = "tok-other"
	for name, id := range map[string]string{"missing": "e0000000-0000-4000-8000-000000000099", "foreign-state": "e0000000-0000-4000-8000-000000000001"} {
		caller := testCaller()
		if name == "foreign-state" {
			caller = other
		}
		if _, err := Complete(context.Background(), completePorts(f), caller, id); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("%s = %v, want not-found", name, err)
		}
		if _, err := Status(context.Background(), f, caller, id); !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("status %s = %v, want not-found", name, err)
		}
	}
}

func TestCompleteReportsTerminal(t *testing.T) {
	f := newFakeCompleteStore()
	s := issuedFixture()
	s.Status = domain.StateReady
	f.sessions[s.ID] = s
	f.objects[s.ID] = "e0000000-0000-4000-8000-000000000002"
	view, err := Complete(context.Background(), completePorts(f), testCaller(), s.ID)
	if err != nil {
		t.Fatalf("complete = %v", err)
	}
	if view.State != domain.StateReady || view.EvidenceID != "e0000000-0000-4000-8000-000000000002" {
		t.Errorf("view = %+v", view)
	}
	if f.jobs != 0 {
		t.Errorf("terminal completion enqueued %d jobs", f.jobs)
	}
}

func TestExpiredPhotoCannotCompleteOrExposeReadyObject(t *testing.T) {
	f := newFakeCompleteStore()
	sess := issuedFixture()
	sess.PhotoCaptureID = "capture"
	sess.ExpiresAt = time.Now().Add(-time.Second)
	f.sessions[sess.ID] = sess
	got, err := Complete(context.Background(), completePorts(f), testCaller(), sess.ID)
	if err != nil || got.State != domain.StateExpired || f.jobs != 0 {
		t.Fatalf("expired complete=%+v %v jobs=%d", got, err, f.jobs)
	}
	sess.Status = domain.StateReady
	f.sessions[sess.ID] = sess
	f.objects[sess.ID] = "object"
	got, err = Status(context.Background(), f, testCaller(), sess.ID)
	if err != nil || got.State != domain.StateExpired || got.EvidenceID != "" {
		t.Fatalf("expired status=%+v %v", got, err)
	}
}
