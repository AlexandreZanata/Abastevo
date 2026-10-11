package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

type fakeConsentStore struct {
	mu   sync.Mutex
	rows map[string]domain.Consent
}

func newFakeConsentStore() *fakeConsentStore {
	return &fakeConsentStore{rows: map[string]domain.Consent{}}
}

func (f *fakeConsentStore) Get(_ context.Context, contributorID string) (domain.Consent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.rows[contributorID]
	if !ok {
		return domain.Consent{}, domain.ErrNotFound
	}
	return c, nil
}

func (f *fakeConsentStore) Put(_ context.Context, c domain.Consent) (bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.rows[c.ContributorID]
	if !ok {
		f.rows[c.ContributorID] = c
		return true, false, nil
	}
	if c.Revision < existing.Revision {
		return false, false, domain.ErrStaleConsent
	}
	if c.Revision == existing.Revision {
		if c == existing {
			return false, true, nil
		}
		return false, false, domain.ErrConflict
	}
	f.rows[c.ContributorID] = c
	return true, false, nil
}

type fakeTerms struct{ version, digest string }

func (f fakeTerms) CurrentVersion() string { return f.version }
func (f fakeTerms) CurrentDigest() string  { return f.digest }

func consentPorts(store *fakeConsentStore) ConsentPorts {
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	return ConsentPorts{
		Clock: func() time.Time { return base },
		Attribution: func(_ context.Context, contributorID string) (string, error) {
			return "tok-" + contributorID, nil
		},
		Store: store,
		Terms: fakeTerms{version: domain.TrainingTermsV1, digest: "digest-v1"},
	}
}

func TestGrantThenStatusIsGranted(t *testing.T) {
	store := newFakeConsentStore()
	p := consentPorts(store)
	res, err := GrantConsent(context.Background(), p, Caller{ContributorID: "c1"}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	})
	if err != nil || res.Replayed || res.State != domain.StateGranted {
		t.Fatalf("grant = %+v, %v", res, err)
	}
	st, err := ReadConsent(context.Background(), p, Caller{ContributorID: "c1"})
	if err != nil || !st.Granted || st.Revision != 1 {
		t.Fatalf("status = %+v, %v", st, err)
	}
}

func TestAbsentConsentDefaultsOff(t *testing.T) {
	p := consentPorts(newFakeConsentStore())
	st, err := ReadConsent(context.Background(), p, Caller{ContributorID: "nobody"})
	if err != nil || st.Granted || st.NeedsReGrant {
		t.Fatalf("absent status = %+v, %v", st, err)
	}
	ok, err := CanSelectTraining(context.Background(), p, Caller{ContributorID: "nobody"}, ImageSelectionDTO{
		ImageID: "img-1", CapturedAt: time.Now(), PerImageAllowed: true,
	})
	if err != nil || ok {
		t.Fatalf("absent can-select = %v, %v", ok, err)
	}
}

func TestGrantRefusesOldTermsAndAnonymous(t *testing.T) {
	p := consentPorts(newFakeConsentStore())
	if _, err := GrantConsent(context.Background(), p, Caller{ContributorID: "c1"}, GrantDTO{
		TermsVersion: "training-terms-v0", TermsDigest: "digest-v1", Revision: 1,
	}); !errors.Is(err, domain.ErrTermsOutdated) {
		t.Errorf("old terms grant = %v", err)
	}
	if _, err := GrantConsent(context.Background(), p, Caller{}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	}); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anonymous grant = %v", err)
	}
	if _, err := RevokeConsent(context.Background(), p, Caller{}, RevokeDTO{Revision: 1}); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anonymous revoke = %v", err)
	}
}

func TestRevokeStopsSelectionAtomically(t *testing.T) {
	store := newFakeConsentStore()
	p := consentPorts(store)
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if _, err := GrantConsent(context.Background(), p, Caller{ContributorID: "c1"}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	ok, err := CanSelectTraining(context.Background(), p, Caller{ContributorID: "c1"}, ImageSelectionDTO{
		ImageID: "img-1", CapturedAt: now.Add(time.Hour), PerImageAllowed: true,
	})
	if err != nil || !ok {
		t.Fatalf("pre-revoke can-select = %v, %v", ok, err)
	}
	rev, err := RevokeConsent(context.Background(), p, Caller{ContributorID: "c1"}, RevokeDTO{Revision: 2})
	if err != nil || !rev.DeleteOwnedTrainingAssets || rev.Replayed {
		t.Fatalf("revoke = %+v, %v", rev, err)
	}
	ok, err = CanSelectTraining(context.Background(), p, Caller{ContributorID: "c1"}, ImageSelectionDTO{
		ImageID: "img-1", CapturedAt: now.Add(3 * time.Hour), PerImageAllowed: true,
	})
	if err != nil || ok {
		t.Fatalf("post-revoke can-select = %v, %v", ok, err)
	}
	st, err := ReadConsent(context.Background(), p, Caller{ContributorID: "c1"})
	if err != nil || st.Granted {
		t.Fatalf("post-revoke status = %+v, %v", st, err)
	}
}

func TestStaleOfflineGrantCannotReactivate(t *testing.T) {
	p := consentPorts(newFakeConsentStore())
	if _, err := GrantConsent(context.Background(), p, Caller{ContributorID: "c1"}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := RevokeConsent(context.Background(), p, Caller{ContributorID: "c1"}, RevokeDTO{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	// Offline ON queued at revision 2 while revocation already holds 2.
	if _, err := GrantConsent(context.Background(), p, Caller{ContributorID: "c1"}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 2,
	}); !errors.Is(err, domain.ErrStaleConsent) && !errors.Is(err, domain.ErrConflict) {
		t.Errorf("stale offline grant = %v", err)
	}
	st, err := ReadConsent(context.Background(), p, Caller{ContributorID: "c1"})
	if err != nil || st.Granted {
		t.Fatalf("revocation resurrected: %+v, %v", st, err)
	}
}

func TestRepeatedOffConvergesWithoutDuplicateDelete(t *testing.T) {
	p := consentPorts(newFakeConsentStore())
	first, err := RevokeConsent(context.Background(), p, Caller{ContributorID: "c1"}, RevokeDTO{Revision: 1})
	if err != nil || !first.DeleteOwnedTrainingAssets {
		t.Fatalf("first revoke = %+v, %v", first, err)
	}
	second, err := RevokeConsent(context.Background(), p, Caller{ContributorID: "c1"}, RevokeDTO{Revision: 1})
	if err != nil || !second.Replayed || second.DeleteOwnedTrainingAssets {
		t.Fatalf("repeated revoke = %+v, %v", second, err)
	}
}

func TestOwnerIsolationAcrossAccounts(t *testing.T) {
	store := newFakeConsentStore()
	p := consentPorts(store)
	if _, err := GrantConsent(context.Background(), p, Caller{ContributorID: "owner-a"}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// Another account sees its own default OFF, never the owner's grant.
	st, err := ReadConsent(context.Background(), p, Caller{ContributorID: "owner-b"})
	if err != nil || st.Granted {
		t.Fatalf("foreign status = %+v, %v", st, err)
	}
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	ok, err := CanSelectTraining(context.Background(), p, Caller{ContributorID: "owner-b"}, ImageSelectionDTO{
		ImageID: "img-1", CapturedAt: now.Add(time.Hour), PerImageAllowed: true,
	})
	if err != nil || ok {
		t.Fatalf("foreign can-select = %v, %v", ok, err)
	}
	// A grant as the second account creates its own row and leaves the first intact.
	if _, err := GrantConsent(context.Background(), p, Caller{ContributorID: "owner-b"}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	st, err = ReadConsent(context.Background(), p, Caller{ContributorID: "owner-a"})
	if err != nil || !st.Granted {
		t.Fatalf("owner-a status after owner-b grant = %+v, %v", st, err)
	}
}

func TestPerImageScopeNeedsFutureCaptureAndOptIn(t *testing.T) {
	p := consentPorts(newFakeConsentStore())
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if _, err := GrantConsent(context.Background(), p, Caller{ContributorID: "c1"}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	for name, dto := range map[string]ImageSelectionDTO{
		"pre-grant capture": {ImageID: "img-old", CapturedAt: now.Add(-time.Hour), PerImageAllowed: true},
		"opt-out capture":   {ImageID: "img-out", CapturedAt: now.Add(time.Hour), PerImageAllowed: false},
	} {
		if ok, err := CanSelectTraining(context.Background(), p, Caller{ContributorID: "c1"}, dto); err != nil || ok {
			t.Errorf("%s = %v, %v", name, ok, err)
		}
	}
	if ok, err := CanSelectTraining(context.Background(), p, Caller{ContributorID: "c1"}, ImageSelectionDTO{
		ImageID: "img-ok", CapturedAt: now.Add(time.Hour), PerImageAllowed: true,
	}); err != nil || !ok {
		t.Errorf("eligible capture = %v, %v", ok, err)
	}
}

func TestConcurrentRevokesConvergeUnderRace(t *testing.T) {
	store := newFakeConsentStore()
	p := consentPorts(store)
	if _, err := GrantConsent(context.Background(), p, Caller{ContributorID: "c1"}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for rev := uint64(2); rev <= 6; rev++ {
		wg.Add(1)
		go func(r uint64) {
			defer wg.Done()
			_, _ = RevokeConsent(context.Background(), p, Caller{ContributorID: "c1"}, RevokeDTO{Revision: r})
		}(rev)
	}
	wg.Wait()
	st, err := ReadConsent(context.Background(), p, Caller{ContributorID: "c1"})
	if err != nil || st.Granted || st.Revision < 2 || st.Revision > 6 {
		t.Fatalf("converged status = %+v, %v", st, err)
	}
}
