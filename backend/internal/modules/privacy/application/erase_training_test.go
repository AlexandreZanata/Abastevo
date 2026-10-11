package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// CS10D: account erasure covers training assets — consent ends
// REVOKED, every owned derivative flips to the tombstone, and every
// owned copy deletes immediately (erasure never waits for the 24 h
// sweeper). Re-runs converge to zero; failures error for the FAILED
// receipt and audited retry.

type fakeTrainingInventory struct {
	mu   sync.Mutex
	rows map[string]domain.Derivative
}

func trainingKey(image string, revision uint64) string {
	return image + "#" + string(rune('0'+revision))
}

func newFakeTrainingInventory() *fakeTrainingInventory {
	return &fakeTrainingInventory{rows: map[string]domain.Derivative{}}
}

func (f *fakeTrainingInventory) Put(_ context.Context, d domain.Derivative) (bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := trainingKey(d.ImageID, d.ConsentRevision)
	if _, ok := f.rows[key]; !ok {
		f.rows[key] = d
		return true, false, nil
	}
	return false, true, nil
}

func (f *fakeTrainingInventory) ListActiveByOwner(_ context.Context, contributorID string) ([]domain.Derivative, error) {
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

func (f *fakeTrainingInventory) MarkRevoked(_ context.Context, image string, revision uint64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := trainingKey(image, revision)
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

func (f *fakeTrainingInventory) ListRevokedBefore(_ context.Context, before time.Time) ([]domain.Derivative, error) {
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

func (f *fakeTrainingInventory) ListByOwner(_ context.Context, contributorID string) ([]domain.Derivative, error) {
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

type fakeTrainingObjects struct {
	mu      sync.Mutex
	objects map[string][]byte
	deleted []string
	fail    map[string]bool
}

func (f *fakeTrainingObjects) Exists(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok, nil
}

func (f *fakeTrainingObjects) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail[key] {
		return errors.New("namespace down")
	}
	f.deleted = append(f.deleted, key)
	delete(f.objects, key)
	return nil
}

func trainingErasePorts(consents *fakeConsentStore, inventory *fakeTrainingInventory, objects *fakeTrainingObjects) TrainingErasePorts {
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	return TrainingErasePorts{
		Clock: func() time.Time { return base },
		Attribution: func(_ context.Context, contributorID string) (string, error) {
			return "tok-" + contributorID, nil
		},
		Consent: ConsentPorts{
			Clock: func() time.Time { return base },
			Attribution: func(_ context.Context, contributorID string) (string, error) {
				return "tok-" + contributorID, nil
			},
			Store: consents,
			Terms: fakeTerms{version: domain.TrainingTermsV1, digest: "digest-v1"},
		},
		Inventory: inventory,
		Objects:   objects,
	}
}

func grantAndPromote(t *testing.T, consents *fakeConsentStore, inventory *fakeTrainingInventory, owner, image string) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if _, err := GrantConsent(ctx, ConsentPorts{
		Clock: func() time.Time { return base },
		Attribution: func(_ context.Context, id string) (string, error) {
			return "tok-" + id, nil
		},
		Store: consents,
		Terms: fakeTerms{version: domain.TrainingTermsV1, digest: "digest-v1"},
	}, Caller{ContributorID: owner}, GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	consent, err := consents.Get(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := domain.ProposeDerivative(domain.PromotionCandidate{
		ImageID: image, ContributorID: owner, ContributorRef: "tok-" + owner,
		CapturedAt: base.Add(time.Hour), PerImageAllowed: true, SourceSHA256: "source",
		QueuedAtConsentRevision: consent.Revision,
	}, &consent, domain.VerdictPass, "derivative-"+image, base.Add(2*time.Hour),
		domain.TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, _, err := inventory.Put(ctx, d); err != nil {
		t.Fatal(err)
	}
}

func TestEraseTrainingRevokesConsentAndPurges(t *testing.T) {
	consents := &fakeConsentStore{rows: map[string]domain.Consent{}}
	inventory := newFakeTrainingInventory()
	objects := &fakeTrainingObjects{objects: map[string][]byte{}}
	grantAndPromote(t, consents, inventory, "c1", "img-1")
	grantAndPromote(t, consents, inventory, "c1", "img-2")
	// Bytes land under the inventoried keys, like the job would store them.
	ctx := context.Background()
	for _, d := range inventory.rows {
		key := TrainingObjectKey(d.ImageID, d.ConsentRevision, d.PromotedAt)
		objects.objects[key] = []byte("minimized")
	}
	revoked, purged, err := EraseTrainingContributor(ctx, trainingErasePorts(consents, inventory, objects), "c1")
	if err != nil {
		t.Fatalf("erase training = %v", err)
	}
	if revoked != 2 || purged != 2 {
		t.Errorf("counts = %d/%d, want 2/2", revoked, purged)
	}
	consent, err := consents.Get(ctx, "c1")
	if err != nil || consent.State != domain.StateRevoked {
		t.Errorf("consent = %+v, %v", consent, err)
	}
	for _, d := range inventory.rows {
		if d.State != domain.DerivativeRevoked {
			t.Errorf("row = %+v, want tombstone", d)
		}
	}
	if len(objects.objects) != 0 {
		t.Errorf("bytes remain: %v", objects.objects)
	}
	// Selection stops everywhere after erasure.
	for _, d := range inventory.rows {
		if domain.SelectableForTraining(d, &consent, domain.TrainingTermsV1, "digest-v1") {
			t.Errorf("erased row still selectable: %+v", d)
		}
	}
}

func TestEraseTrainingConvergesWithoutAssets(t *testing.T) {
	consents := &fakeConsentStore{rows: map[string]domain.Consent{}}
	inventory := newFakeTrainingInventory()
	objects := &fakeTrainingObjects{objects: map[string][]byte{}}
	p := trainingErasePorts(consents, inventory, objects)
	// No consent, no derivatives: establishes the REVOKED baseline
	// with zero counts, no error.
	revoked, purged, err := EraseTrainingContributor(context.Background(), p, "c1")
	if err != nil || revoked != 0 || purged != 0 {
		t.Fatalf("empty erase = %d/%d, %v", revoked, purged, err)
	}
	// Re-run converges to zero: receipts stay, bytes stay gone.
	revoked, purged, err = EraseTrainingContributor(context.Background(), p, "c1")
	if err != nil || revoked != 0 || purged != 0 {
		t.Fatalf("repeat erase = %d/%d, %v", revoked, purged, err)
	}
	consent, err := consents.Get(context.Background(), "c1")
	if err != nil || consent.State != domain.StateRevoked {
		t.Errorf("baseline = %+v, %v", consent, err)
	}
}

func TestEraseTrainingFailsBytesForRetry(t *testing.T) {
	consents := &fakeConsentStore{rows: map[string]domain.Consent{}}
	inventory := newFakeTrainingInventory()
	objects := &fakeTrainingObjects{objects: map[string][]byte{}, fail: map[string]bool{}}
	grantAndPromote(t, consents, inventory, "c1", "img-1")
	ctx := context.Background()
	for _, d := range inventory.rows {
		key := TrainingObjectKey(d.ImageID, d.ConsentRevision, d.PromotedAt)
		objects.objects[key] = []byte("minimized")
		objects.fail[key] = true
	}
	if _, _, err := EraseTrainingContributor(ctx, trainingErasePorts(consents, inventory, objects), "c1"); err == nil {
		t.Error("byte outage accepted")
	}
}
