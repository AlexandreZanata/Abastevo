package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

type fakePromoteStore struct {
	mu   sync.Mutex
	rows map[string]domain.Derivative // key: image + revision
}

func promoteKey(image string, revision uint64) string {
	return image + "#" + string(rune('0'+revision))
}

func newFakePromoteStore() *fakePromoteStore {
	return &fakePromoteStore{rows: map[string]domain.Derivative{}}
}

func (f *fakePromoteStore) Put(_ context.Context, d domain.Derivative) (bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := promoteKey(d.ImageID, d.ConsentRevision)
	existing, ok := f.rows[key]
	if !ok {
		f.rows[key] = d
		return true, false, nil
	}
	if existing.DerivativeSHA256 == d.DerivativeSHA256 && existing.SourceSHA256 == d.SourceSHA256 {
		return false, true, nil
	}
	return false, false, domain.ErrConflict
}

func (f *fakePromoteStore) ListActiveByOwner(_ context.Context, contributorID string) ([]domain.Derivative, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Derivative
	for _, d := range f.rows {
		if d.ContributorID == contributorID && d.State == domain.DerivativeActive {
			out = append(out, d)
		}
	}
	return out, nil
}
func (f *fakePromoteStore) MarkRevoked(_ context.Context, image string, revision uint64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := promoteKey(image, revision)
	d, ok := f.rows[key]
	if !ok {
		return domain.ErrNotFound
	}
	revoked, _, err := domain.RevokeDerivative(d, at)
	if err != nil {
		return err
	}
	f.rows[key] = revoked
	return nil
}

func (f *fakePromoteStore) ListRevokedBefore(_ context.Context, before time.Time) ([]domain.Derivative, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Derivative
	for _, d := range f.rows {
		if d.State == domain.DerivativeRevoked && d.RevokedAt.Before(before) {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakePromoteStore) ListByOwner(_ context.Context, contributorID string) ([]domain.Derivative, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Derivative
	for _, d := range f.rows {
		if d.ContributorID == contributorID {
			out = append(out, d)
		}
	}
	return out, nil
}

func promotePorts(store *fakePromoteStore, verdict domain.SanitizeVerdict, sanitizeErr error) PromotePorts {
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	return PromotePorts{
		ConsentPorts: ConsentPorts{
			Clock: func() time.Time { return base },
			Attribution: func(_ context.Context, contributorID string) (string, error) {
				return "tok-" + contributorID, nil
			},
			Store: &fakeConsentStore{rows: map[string]domain.Consent{}},
			Terms: fakeTerms{version: domain.TrainingTermsV1, digest: "digest-v1"},
		},
		Inventory: store,
		Sanitizer: fakeSanitizer{verdict: verdict, err: sanitizeErr},
	}
}

type fakeSanitizer struct {
	verdict domain.SanitizeVerdict
	err     error
}

func (f fakeSanitizer) Sanitize(_ context.Context, source []byte) ([]byte, domain.SanitizeVerdict, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	if len(source) == 0 {
		return nil, "", errors.New("empty source")
	}
	return []byte("sanitized:" + string(source)), f.verdict, nil
}

func grantPromoteConsent(t *testing.T, p PromotePorts, owner string, revision uint64) {
	t.Helper()
	if _, err := GrantConsent(context.Background(), p.ConsentPorts, Caller{ContributorID: owner}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: revision,
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
}

func TestProposeHappyPath(t *testing.T) {
	store := newFakePromoteStore()
	p := promotePorts(store, domain.VerdictPass, nil)
	grantPromoteConsent(t, p, "c1", 1)
	res, resBytes, err := Propose(context.Background(), p, Caller{ContributorID: "c1"}, PromotionDTO{
		ImageID: "img-1", CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
		PerImageAllowed: true, Source: []byte("raw-bytes"),
	})
	if err != nil || res.Replayed || res.DerivativeSHA256 == "" || len(resBytes) == 0 {
		t.Fatalf("propose = %+v, %v", res, err)
	}
	// Identical re-proposal converges without duplicating inventory.
	second, secondBytes, err := Propose(context.Background(), p, Caller{ContributorID: "c1"}, PromotionDTO{
		ImageID: "img-1", CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
		PerImageAllowed: true, Source: []byte("raw-bytes"),
	})
	if err != nil || !second.Replayed || second.DerivativeSHA256 != res.DerivativeSHA256 || secondBytes != nil {
		t.Fatalf("re-propose = %+v, %v", second, err)
	}
}

func TestProposeRefusesWithoutConsentOrScope(t *testing.T) {
	p := promotePorts(newFakePromoteStore(), domain.VerdictPass, nil)
	dto := PromotionDTO{
		ImageID: "img-1", CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
		PerImageAllowed: true, Source: []byte("raw"),
	}
	if _, _, err := Propose(context.Background(), p, Caller{ContributorID: "c1"}, dto); !errors.Is(err, domain.ErrConsentRequired) {
		t.Errorf("absent consent = %v", err)
	}
	grantPromoteConsent(t, p, "c1", 1)
	optOut := dto
	optOut.PerImageAllowed = false
	if _, _, err := Propose(context.Background(), p, Caller{ContributorID: "c1"}, optOut); !errors.Is(err, domain.ErrImageNotCovered) {
		t.Errorf("opt-out = %v", err)
	}
	if _, _, err := Propose(context.Background(), p, Caller{}, dto); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anonymous = %v", err)
	}
}

func TestProposeFailsClosedAndWritesNothing(t *testing.T) {
	for _, verdict := range []domain.SanitizeVerdict{domain.VerdictFail, domain.VerdictUncertain} {
		store := newFakePromoteStore()
		p := promotePorts(store, verdict, nil)
		grantPromoteConsent(t, p, "c1", 1)
		if _, _, err := Propose(context.Background(), p, Caller{ContributorID: "c1"}, PromotionDTO{
			ImageID: "img-1", CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
			PerImageAllowed: true, Source: []byte("raw"),
		}); !errors.Is(err, domain.ErrNotSanitizable) {
			t.Errorf("verdict %s = %v", verdict, err)
		}
		if len(store.rows) != 0 {
			t.Errorf("verdict %s wrote inventory", verdict)
		}
	}
	store := newFakePromoteStore()
	p := promotePorts(store, domain.VerdictPass, errors.New("sanitizer down"))
	grantPromoteConsent(t, p, "c1", 1)
	if _, _, err := Propose(context.Background(), p, Caller{ContributorID: "c1"}, PromotionDTO{
		ImageID: "img-1", CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
		PerImageAllowed: true, Source: []byte("raw"),
	}); err == nil {
		t.Error("sanitizer outage accepted")
	}
	if len(store.rows) != 0 {
		t.Error("outage wrote inventory")
	}
}

func TestProposeLosesGrantRevokeRace(t *testing.T) {
	store := newFakePromoteStore()
	p := promotePorts(store, domain.VerdictPass, nil)
	grantPromoteConsent(t, p, "c1", 1)
	// Revocation lands between queueing and the worker commit: the
	// commit-time re-check refuses with no inventory write.
	if _, err := RevokeConsent(context.Background(), p.ConsentPorts, Caller{ContributorID: "c1"}, RevokeDTO{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Propose(context.Background(), p, Caller{ContributorID: "c1"}, PromotionDTO{
		ImageID: "img-1", CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
		PerImageAllowed: true, Source: []byte("raw"),
	}); !errors.Is(err, domain.ErrConsentRequired) {
		t.Errorf("post-revoke propose = %v", err)
	}
	if len(store.rows) != 0 {
		t.Error("revoked proposal wrote inventory")
	}
}

func TestRevokeOwnerStopsSelection(t *testing.T) {
	store := newFakePromoteStore()
	p := promotePorts(store, domain.VerdictPass, nil)
	grantPromoteConsent(t, p, "c1", 1)
	for _, img := range []string{"img-1", "img-2"} {
		if _, _, err := Propose(context.Background(), p, Caller{ContributorID: "c1"}, PromotionDTO{
			ImageID: img, CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
			PerImageAllowed: true, Source: []byte("raw-" + img),
		}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := RevokeOwner(context.Background(), p, Caller{ContributorID: "c1"})
	if err != nil || n != 2 {
		t.Fatalf("revoke owner = %d, %v", n, err)
	}
	again, err := RevokeOwner(context.Background(), p, Caller{ContributorID: "c1"})
	if err != nil || again != 0 {
		t.Fatalf("repeat revoke = %d, %v", again, err)
	}
	// Another owner's inventory is untouched.
	grantPromoteConsent(t, p, "c2", 1)
	if _, _, err := Propose(context.Background(), p, Caller{ContributorID: "c2"}, PromotionDTO{
		ImageID: "img-9", CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
		PerImageAllowed: true, Source: []byte("raw"),
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := RevokeOwner(context.Background(), p, Caller{ContributorID: "c1"}); err != nil || n != 0 {
		t.Fatalf("cross-owner revoke = %d, %v", n, err)
	}
}

func TestConcurrentProposesAgreeUnderRace(t *testing.T) {
	store := newFakePromoteStore()
	p := promotePorts(store, domain.VerdictPass, nil)
	grantPromoteConsent(t, p, "c1", 1)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = Propose(context.Background(), p, Caller{ContributorID: "c1"}, PromotionDTO{
				ImageID: "img-1", CapturedAt: time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC),
				PerImageAllowed: true, Source: []byte("raw-bytes"),
			})
		}()
	}
	wg.Wait()
	if len(store.rows) != 1 {
		t.Fatalf("rows = %d, want exactly the converged singleton", len(store.rows))
	}
}
