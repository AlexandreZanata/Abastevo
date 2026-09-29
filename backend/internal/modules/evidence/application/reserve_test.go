package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// fakeStore mimics the T04 persistence contract: natural-key lookup with a
// typed miss, and save converging on identical retries while conflicting
// on divergent payloads under the same key (B-BR-005).
type fakeStore struct {
	mu    sync.Mutex
	rows  map[string]domain.Session
	saves int
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]domain.Session{}} }

func naturalKey(ref, client string) string { return ref + "\x00" + client }

func (f *fakeStore) ByNaturalKey(_ context.Context, ref, client string) (domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.rows[naturalKey(ref, client)]
	if !ok {
		return domain.Session{}, ErrSessionNotFound
	}
	return s, nil
}

func (f *fakeStore) Save(_ context.Context, s domain.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := naturalKey(s.ContributorRef, s.ClientSessionID)
	if prev, ok := f.rows[k]; ok {
		if prev.MIME != s.MIME || prev.DeclaredBytes != s.DeclaredBytes || prev.ClaimedSHA256 != s.ClaimedSHA256 {
			return ErrConflict
		}
		return nil
	}
	f.rows[k] = s
	f.saves++
	return nil
}

func testPorts(store *fakeStore) Ports {
	return Ports{
		Clock: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) { return "e0000000-0000-4000-8000-000000000001", nil },
		NewKey: func() (string, error) {
			return "q/e0000000000000000000000000000001", nil
		},
		CheckQuota: func(context.Context, string, string) (time.Duration, error) { return 0, nil },
		Store:      store,
	}
}

func testCaller() Caller {
	return Caller{ContributorID: "c1", Fingerprint: "fp:x", Token: "tok-c1"}
}

func testIntent() Intent {
	return Intent{
		ClientSessionID: "upl-1", MIME: "image/jpeg", DeclaredBytes: 512 << 10,
		ClaimedSHA256: strings.Repeat("a", 64),
	}
}

func TestReserveRequiresAuthentication(t *testing.T) {
	store := newFakeStore()
	if _, err := Reserve(context.Background(), testPorts(store), Caller{}, testIntent()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anonymous = %v", err)
	}
	bad := testCaller()
	bad.Token = ""
	if _, err := Reserve(context.Background(), testPorts(store), bad, testIntent()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("tokenless = %v", err)
	}
	if store.saves != 0 {
		t.Errorf("saves = %d after refused auth", store.saves)
	}
}

func TestReserveChecksQuotaBeforeWork(t *testing.T) {
	store := newFakeStore()
	p := testPorts(store)
	p.CheckQuota = func(context.Context, string, string) (time.Duration, error) {
		return 0, &QuotaDeniedError{RetryAfter: 30 * time.Second}
	}
	if _, err := Reserve(context.Background(), p, testCaller(), testIntent()); !errors.Is(err, ErrQuotaDenied) {
		t.Errorf("quota = %v", err)
	} else {
		var denied *QuotaDeniedError
		if !errors.As(err, &denied) || denied.RetryAfter != 30*time.Second {
			t.Errorf("denial = %v", err)
		}
	}
	// Exhausted quota grants no reservation: nothing reaches the store.
	if store.saves != 0 {
		t.Errorf("saves = %d despite quota denial", store.saves)
	}
}

func TestReserveRejectsInvalidIntentEarly(t *testing.T) {
	store := newFakeStore()
	p := testPorts(store)
	bad := testIntent()
	bad.MIME = "image/png"
	if _, err := Reserve(context.Background(), p, testCaller(), bad); !errors.Is(err, domain.ErrUnsupportedMedia) {
		t.Errorf("png = %v", err)
	}
	bad = testIntent()
	bad.DeclaredBytes = domain.MaxUploadBytes + 1
	if _, err := Reserve(context.Background(), p, testCaller(), bad); !errors.Is(err, domain.ErrSizeOutOfBounds) {
		t.Errorf("oversize = %v", err)
	}
	if store.saves != 0 {
		t.Errorf("saves = %d for invalid intent", store.saves)
	}
}

func TestReserveHappyPathUsesServerKey(t *testing.T) {
	store := newFakeStore()
	p := testPorts(store)
	res, err := Reserve(context.Background(), p, testCaller(), testIntent())
	if err != nil {
		t.Fatalf("reserve = %v", err)
	}
	if res.Replayed {
		t.Error("fresh reservation marked replayed")
	}
	got, err := store.ByNaturalKey(context.Background(), "tok-c1", "upl-1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	// The object key is server-generated: the client never supplies it.
	if got.QuarantineKey != "q/e0000000000000000000000000000001" {
		t.Errorf("key = %q", got.QuarantineKey)
	}
	if got.Status != domain.StateIssued || !got.ExpiresAt.Equal(got.CreatedAt.Add(domain.SessionTTL)) {
		t.Errorf("session = %+v", got)
	}
	if res.SessionID != got.ID || !res.ExpiresAt.Equal(got.ExpiresAt) || res.MaxBytes != domain.MaxUploadBytes {
		t.Errorf("result = %+v", res)
	}
}

func TestReserveRetryConvergesAndDivergenceConflicts(t *testing.T) {
	store := newFakeStore()
	p := testPorts(store)
	first, err := Reserve(context.Background(), p, testCaller(), testIntent())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Reserve(context.Background(), p, testCaller(), testIntent())
	if err != nil || !second.Replayed || second.SessionID != first.SessionID {
		t.Fatalf("retry = %+v, %v", second, err)
	}
	if store.saves != 1 {
		t.Errorf("saves = %d, want exactly one reservation", store.saves)
	}
	diverged := testIntent()
	diverged.DeclaredBytes++
	if _, err := Reserve(context.Background(), p, testCaller(), diverged); !errors.Is(err, ErrConflict) {
		t.Errorf("divergent retry = %v, want conflict", err)
	}
}

func TestReserveIsolatesContributors(t *testing.T) {
	// The natural key scopes per contributor: the same client ID under a
	// different attribution never collides and never sees foreign intent.
	store := newFakeStore()
	p := testPorts(store)
	if _, err := Reserve(context.Background(), p, testCaller(), testIntent()); err != nil {
		t.Fatal(err)
	}
	other := Caller{ContributorID: "c2", Fingerprint: "fp:y", Token: "tok-c2"}
	res, err := Reserve(context.Background(), p, other, testIntent())
	if err != nil || res.Replayed {
		t.Fatalf("other contributor = %+v, %v", res, err)
	}
	if store.saves != 2 {
		t.Errorf("saves = %d, want isolated reservations", store.saves)
	}
}
