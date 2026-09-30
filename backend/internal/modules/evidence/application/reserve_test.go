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

func (f *fakeStore) ReserveSession(_ context.Context, s domain.Session) (domain.Session, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := naturalKey(s.ContributorRef, s.ClientSessionID)
	if prev, ok := f.rows[k]; ok {
		if prev.MIME != s.MIME || prev.DeclaredBytes != s.DeclaredBytes ||
			!strings.EqualFold(prev.ClaimedSHA256, s.ClaimedSHA256) {
			return domain.Session{}, false, domain.ErrConflict
		}
		return prev, true, nil
	}
	f.rows[k] = s
	f.saves++
	return s, false, nil
}

func (f *fakeStore) CountSince(_ context.Context, ref string, since time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, s := range f.rows {
		if s.ContributorRef == ref && !s.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func testPorts(store *fakeStore) Ports {
	return Ports{
		Clock: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) { return "e0000000-0000-4000-8000-000000000001", nil },
		NewKey: func() (string, error) {
			return "q/e0000000000000000000000000000001", nil
		},
		CheckQuota: func(context.Context, string, string) (time.Duration, error) { return 0, nil },
		Presign: func(_ context.Context, key, mime string, maxBytes int64) (string, map[string]string, time.Time, error) {
			if key == "" || mime == "" || maxBytes < 1 {
				return "", nil, time.Time{}, errors.New("bad presign args")
			}
			return "https://storage.example.invalid/" + key + "?sig=test",
				map[string]string{"Content-Type": mime},
				time.Date(2026, 9, 30, 12, 5, 0, 0, time.UTC), nil
		},
		Store: store,
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
	got, ok := store.rows[naturalKey("tok-c1", "upl-1")]
	if !ok {
		t.Fatal("reservation missing from store")
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
	if !strings.Contains(res.URL, got.QuarantineKey) || res.RequiredHeaders["Content-Type"] != "image/jpeg" {
		t.Errorf("bundle = %+v", res)
	}
	if !res.URLExpiresAt.Equal(time.Date(2026, 9, 30, 12, 5, 0, 0, time.UTC)) {
		t.Errorf("bundle expiry = %v", res.URLExpiresAt)
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

func TestReserveEnforcesDailyPhotoCap(t *testing.T) {
	// Ten reservations a day per contributor: the eleventh is denied
	// with a retry delay pointing past UTC midnight, before any work.
	store := newFakeStore()
	p := testPorts(store)
	caller := testCaller()
	for i := 0; i < MaxReservesPerDay; i++ {
		intent := testIntent()
		intent.ClientSessionID = strings.Repeat("u", 8) + string(rune('a'+i))
		if _, err := Reserve(context.Background(), p, caller, intent); err != nil {
			t.Fatalf("reservation %d: %v", i, err)
		}
	}
	overflow := testIntent()
	overflow.ClientSessionID = "upl-overflow"
	_, err := Reserve(context.Background(), p, caller, overflow)
	if !errors.Is(err, ErrQuotaDenied) {
		t.Fatalf("overflow = %v, want quota denial", err)
	}
	var denied *QuotaDeniedError
	if !errors.As(err, &denied) || denied.RetryAfter <= 0 || denied.RetryAfter > 24*time.Hour {
		t.Errorf("denial delay = %v", err)
	}
}

func TestReserveReplayMintsFreshBundle(t *testing.T) {
	// Replays converge on the session but mint a fresh short URL: the
	// previous one may have expired while the reservation stands.
	store := newFakeStore()
	p := testPorts(store)
	calls := 0
	p.Presign = func(_ context.Context, key, mime string, maxBytes int64) (string, map[string]string, time.Time, error) {
		calls++
		return "https://storage.example.invalid/" + key + "?sig=" + string(rune('a'+calls)),
			map[string]string{"Content-Type": mime},
			time.Date(2026, 9, 30, 12, 5, 0, 0, time.UTC), nil
	}
	first, err := Reserve(context.Background(), p, testCaller(), testIntent())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Reserve(context.Background(), p, testCaller(), testIntent())
	if err != nil || !second.Replayed || second.SessionID != first.SessionID {
		t.Fatalf("replay = %+v, %v", second, err)
	}
	if calls != 2 || first.URL == second.URL {
		t.Errorf("presign calls = %d, urls %q %q", calls, first.URL, second.URL)
	}
}

func TestReserveSurfacesStorageOutage(t *testing.T) {
	store := newFakeStore()
	p := testPorts(store)
	p.Presign = func(context.Context, string, string, int64) (string, map[string]string, time.Time, error) {
		return "", nil, time.Time{}, ErrStorageUnavailable
	}
	if _, err := Reserve(context.Background(), p, testCaller(), testIntent()); !errors.Is(err, ErrStorageUnavailable) {
		t.Errorf("outage = %v, want storage-unavailable", err)
	}
}

func TestReserveBreakerStopsIntakeWhileOverdue(t *testing.T) {
	// Overdue copies mean the enforcement loop is behind: intake
	// refuses before quota burn and before any store write.
	store := newFakeStore()
	p := testPorts(store)
	p.Enforcement = func(context.Context) error { return ErrEnforcementUnhealthy }
	if _, err := Reserve(context.Background(), p, testCaller(), testIntent()); !errors.Is(err, ErrEnforcementUnhealthy) {
		t.Errorf("unhealthy enforcement = %v, want enforcement-unhealthy", err)
	}
	if store.saves != 0 {
		t.Errorf("breaker must write nothing, saves = %d", store.saves)
	}
	// Nil enforcement preserves every existing caller: healthy intake.
	if _, err := Reserve(context.Background(), testPorts(newFakeStore()), testCaller(), testIntent()); err != nil {
		t.Errorf("nil enforcement must stay healthy, got %v", err)
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
