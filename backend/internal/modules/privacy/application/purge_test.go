package application

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

type fakePurgeInventory struct {
	rows []domain.Derivative
}

func (f *fakePurgeInventory) ListRevokedBefore(_ context.Context, before time.Time) ([]domain.Derivative, error) {
	var out []domain.Derivative
	for _, d := range f.rows {
		if d.State == domain.DerivativeRevoked && d.RevokedAt.Before(before) {
			out = append(out, d)
		}
	}
	return out, nil
}

type fakeObjectStore struct {
	objects   map[string][]byte
	deleted   []string
	getErr    map[string]error
	deleteErr map[string]error
}

func (f *fakeObjectStore) Exists(_ context.Context, key string) (bool, error) {
	if err, ok := f.getErr[key]; ok {
		return false, err
	}
	_, ok := f.objects[key]
	return ok, nil
}

func (f *fakeObjectStore) Delete(_ context.Context, key string) error {
	if err, ok := f.deleteErr[key]; ok {
		return err
	}
	f.deleted = append(f.deleted, key)
	delete(f.objects, key)
	return nil
}

func revokedDerivative(image string, revokedAt time.Time) domain.Derivative {
	return domain.Derivative{
		ImageID: image, ContributorID: "c1", ContributorRef: "tok-c1",
		ObjectKey:    TrainingObjectKey(image, 1, revokedAt.Add(-48*time.Hour)),
		SourceSHA256: "source", DerivativeSHA256: "derivative",
		CapturedAt:   revokedAt.Add(-48 * time.Hour),
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1",
		State: domain.DerivativeRevoked, ConsentRevision: 1,
		PromotedAt: revokedAt.Add(-48 * time.Hour), RevokedAt: revokedAt,
	}
}

func TestSweepPurgesDueObjects(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-25 * time.Hour)
	key := TrainingObjectKey("img-1", 1, revokedAt.Add(-48*time.Hour))
	inv := &fakePurgeInventory{rows: []domain.Derivative{
		revokedDerivative("img-1", revokedAt),
		revokedDerivative("img-fresh", now.Add(-time.Hour)),
	}}
	store := &fakeObjectStore{objects: map[string][]byte{key: []byte("bytes")}}
	report, err := SweepRevoked(context.Background(), PurgePorts{
		Clock:     func() time.Time { return now },
		Inventory: inv,
		Objects:   store,
	})
	if err != nil {
		t.Fatalf("sweep = %v", err)
	}
	if report.Checked != 1 || report.Purged != 1 || len(report.Missing) != 0 || len(report.Failures) != 0 {
		t.Errorf("report = %+v", report)
	}
	if _, ok := store.objects[key]; ok {
		t.Error("due bytes still present after sweep")
	}
}

func TestSweepReportsMissingAndFailures(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-25 * time.Hour)
	stuck := TrainingObjectKey("img-stuck", 1, revokedAt.Add(-48*time.Hour))
	inv := &fakePurgeInventory{rows: []domain.Derivative{
		revokedDerivative("img-gone", revokedAt),
		revokedDerivative("img-stuck", revokedAt),
	}}
	store := &fakeObjectStore{
		objects:   map[string][]byte{stuck: []byte("bytes")},
		deleteErr: map[string]error{stuck: errors.New("namespace down")},
	}
	report, err := SweepRevoked(context.Background(), PurgePorts{
		Clock:     func() time.Time { return now },
		Inventory: inv,
		Objects:   store,
	})
	if err == nil {
		t.Fatal("partial cleanup was acknowledged")
	}
	if report.Checked != 2 || report.Purged != 0 {
		t.Errorf("report = %+v", report)
	}
	if len(report.Missing) != 1 {
		t.Errorf("missing = %v, want the already-gone copy", report.Missing)
	}
	if len(report.Failures) != 1 {
		t.Errorf("failures = %v, want the stuck copy", report.Failures)
	}
}

func TestTrainingObjectKeyStaysInNamespace(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	key := TrainingObjectKey("img-1", 3, now)
	if key != "training/v1/2026-10-11/img-1-3.jpg" {
		t.Errorf("key = %q", key)
	}
	for _, bad := range []struct {
		image string
		rev   uint64
	}{
		{"", 1}, {"../escape", 1}, {`back\slash`, 1}, {"img-1", 0},
	} {
		if got := TrainingObjectKey(bad.image, bad.rev, now); got != "" {
			t.Errorf("key(%q,%d) = %q, want empty", bad.image, bad.rev, got)
		}
	}
}

func TestRestoreDrillKeepsRevokedExcluded(t *testing.T) {
	// Drill mechanics: rows rebuilt from a backup dump re-run the
	// shared selection filter — revoked receipts stay excluded under
	// a fresh grant, active rows reselect.
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	consent, _, err := domain.GrantConsent(nil, domain.ConsentParams{
		ContributorID: "c1", ContributorRef: "tok-c1",
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1",
		Revision: 2, DecidedAt: grantedAt,
	}, domain.TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	active := domain.Derivative{
		ImageID: "img-active", ContributorID: "c1", ContributorRef: "tok-c1",
		SourceSHA256: "s", DerivativeSHA256: "d", CapturedAt: grantedAt.Add(time.Hour),
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1",
		State: domain.DerivativeActive, OutputState: domain.OutputReady, ConsentRevision: 2, PromotedAt: grantedAt.Add(2 * time.Hour),
	}
	revoked := active
	revoked.ImageID = "img-revoked"
	revoked.State = domain.DerivativeRevoked
	revoked.RevokedAt = grantedAt.Add(3 * time.Hour)
	// "Restored" copies behave exactly like the originals.
	if !domain.SelectableForTraining(active, &consent, domain.TrainingTermsV1, "digest-v1") {
		t.Error("restored active row not selectable")
	}
	if domain.SelectableForTraining(revoked, &consent, domain.TrainingTermsV1, "digest-v1") {
		t.Error("restored revoked row resurrected")
	}
}
