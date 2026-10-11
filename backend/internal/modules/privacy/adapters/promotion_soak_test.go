//go:build soak

package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// Controlled real 24-hour soak for the revoked-copy SLO (CS10C,
// AT05): minimized derivatives land in the private MinIO namespace
// with real wall-clock revocation instants, and the daily sweeper
// must purge every due copy within 24 hours of its receipt while
// live rows stay untouched. Run TestSoakSeed once to start the
// clock, then TestSoakVerify after 24 hours to measure the SLO.
// Nothing here uses virtual time: injected-clock unit tests own the
// mechanics, this file owns the wall clock.

type soakManifest struct {
	SeededAt string       `json:"seeded_at"`
	DSN      string       `json:"dsn"`
	Bucket   string       `json:"bucket"`
	Objects  []soakObject `json:"objects"`
	Control  soakObject   `json:"control"`
}

type soakObject struct {
	Image     string `json:"image"`
	Revision  uint64 `json:"revision"`
	Key       string `json:"key"`
	RevokedAt string `json:"revoked_at"`
	DueAt     string `json:"due_at"`
}

func soakEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func soakSetup(t *testing.T) (context.Context, *pgxpool.Pool, *TrainingObjects, *PromotionStore, *ConsentStore) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	adminDSN := soakEnv("SOAK_DSN", "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable")
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	var exists bool
	if err := admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='training_soak')").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		if _, err := admin.Exec(ctx, `CREATE DATABASE training_soak`); err != nil {
			t.Fatalf("create database: %v", err)
		}
	}
	dsn := adminDSN
	if i := len(dsn) - len("/anpfuel?sslmode=disable"); i > 0 {
		dsn = dsn[:i] + "/training_soak?sslmode=disable"
	}
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	objects := NewTrainingObjects(TrainingObjectConfig{
		Endpoint:        soakEnv("SOAK_S3_ENDPOINT", "http://127.0.0.1:9002"),
		Bucket:          soakEnv("SOAK_S3_BUCKET", "training-soak-private"),
		Region:          soakEnv("SOAK_S3_REGION", "us-east-1"),
		AccessKeyID:     soakEnv("SOAK_S3_ACCESS", "soakuser"),
		SecretAccessKey: soakEnv("SOAK_S3_SECRET", "soak-password-local-only-20261011"),
	})
	return ctx, pool, objects, NewPromotionStore(pool), NewConsentStore(pool)
}

func soakJPEG(t *testing.T, seed byte) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x) + seed, G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSoakSeed(t *testing.T) {
	ctx, _, objects, inventory, consents := soakSetup(t)
	now := time.Now().UTC().Truncate(time.Second)
	manifest := soakManifest{
		SeededAt: now.Format(time.RFC3339),
		Bucket:   soakEnv("SOAK_S3_BUCKET", "training-soak-private"),
	}
	owner := "soak-owner"
	grant := func(revision uint64, at time.Time) domain.Consent {
		c, _, err := domain.GrantConsent(nil, domain.ConsentParams{
			ContributorID: owner, ContributorRef: "tok-" + owner,
			TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1",
			Revision: revision, DecidedAt: at,
		}, domain.TrainingTermsV1, "digest-v1")
		if err != nil {
			t.Fatalf("grant: %v", err)
		}
		if _, _, err := consents.Put(ctx, c); err != nil {
			t.Fatalf("put consent: %v", err)
		}
		return c
	}
	promote := func(image string, consent domain.Consent, captured, promoted time.Time) domain.Derivative {
		candidate := domain.PromotionCandidate{
			ImageID: image, ContributorID: owner, ContributorRef: "tok-" + owner,
			CapturedAt: captured, PerImageAllowed: true, SourceSHA256: "soak-source",
			QueuedAtConsentRevision: consent.Revision,
		}
		minimized, verdict, err := SanitizeImage(ctx, soakJPEG(t, image[len(image)-1]))
		if err != nil || verdict != domain.VerdictPass {
			t.Fatalf("sanitize %s: %v %q", image, err, verdict)
		}
		d, _, err := domain.ProposeDerivative(candidate, &consent, verdict, "soak-derivative",
			promoted, domain.TrainingTermsV1, "digest-v1")
		if err != nil {
			t.Fatalf("propose %s: %v", image, err)
		}
		if _, _, err := inventory.Put(ctx, d); err != nil {
			t.Fatalf("put %s: %v", image, err)
		}
		key := privacyapp.TrainingObjectKey(image, d.ConsentRevision, d.PromotedAt)
		if key == "" {
			t.Fatalf("empty key for %s", image)
		}
		if err := objects.Put(ctx, key, minimized); err != nil {
			t.Fatalf("put bytes %s: %v", image, err)
		}
		return d
	}
	// Three copies revoked now: the wall-clock SLO proof, due in 24h.
	grant(1, now)
	for _, image := range []string{"soak-fresh-1", "soak-fresh-2", "soak-fresh-3"} {
		d := promote(image, mustSoakConsent(t, ctx, consents, owner), now.Add(time.Minute), now)
		revokedAt := now
		if err := inventory.MarkRevoked(ctx, image, d.ConsentRevision, revokedAt); err != nil {
			t.Fatalf("revoke %s: %v", image, err)
		}
		manifest.Objects = append(manifest.Objects, soakObject{
			Image: image, Revision: d.ConsentRevision,
			Key:       privacyapp.TrainingObjectKey(image, d.ConsentRevision, d.PromotedAt),
			RevokedAt: revokedAt.Format(time.RFC3339), DueAt: revokedAt.Add(24 * time.Hour).Format(time.RFC3339),
		})
	}
	// One copy revoked 25h ago: already due, the sweeper must catch it
	// on the first pass (overdue path, no wall-clock wait needed).
	old := now.Add(-28 * time.Hour)
	oldConsent, _, err := domain.GrantConsent(nil, domain.ConsentParams{
		ContributorID: "soak-old", ContributorRef: "tok-soak-old",
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1",
		Revision: 1, DecidedAt: old,
	}, domain.TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := consents.Put(ctx, oldConsent); err != nil {
		t.Fatal(err)
	}
	oldD, _, err := domain.ProposeDerivative(domain.PromotionCandidate{
		ImageID: "soak-overdue", ContributorID: "soak-old", ContributorRef: "tok-soak-old",
		CapturedAt: old.Add(time.Hour), PerImageAllowed: true, SourceSHA256: "soak-source",
		QueuedAtConsentRevision: 1,
	}, &oldConsent, domain.VerdictPass, "soak-derivative", old.Add(2*time.Hour),
		domain.TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := inventory.Put(ctx, oldD); err != nil {
		t.Fatal(err)
	}
	oldMin, _, err := SanitizeImage(ctx, soakJPEG(t, 'o'))
	if err != nil {
		t.Fatal(err)
	}
	oldKey := privacyapp.TrainingObjectKey("soak-overdue", 1, oldD.PromotedAt)
	if err := objects.Put(ctx, oldKey, oldMin); err != nil {
		t.Fatal(err)
	}
	oldRevoked := old.Add(3 * time.Hour)
	if err := inventory.MarkRevoked(ctx, "soak-overdue", 1, oldRevoked); err != nil {
		t.Fatal(err)
	}
	manifest.Objects = append(manifest.Objects, soakObject{
		Image: "soak-overdue", Revision: 1, Key: oldKey,
		RevokedAt: oldRevoked.Format(time.RFC3339), DueAt: oldRevoked.Add(24 * time.Hour).Format(time.RFC3339),
	})
	// One live control with granted consent: must survive every sweep.
	ctrlConsent := grant(2, now)
	ctrl := promote("soak-control", ctrlConsent, now.Add(time.Minute), now)
	manifest.Control = soakObject{
		Image: "soak-control", Revision: ctrl.ConsentRevision,
		Key: privacyapp.TrainingObjectKey("soak-control", ctrl.ConsentRevision, ctrl.PromotedAt),
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := soakEnv("SOAK_MANIFEST", "/tmp/training-soak-manifest.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("soak seeded at %s: %d objects + 1 control; manifest %s", manifest.SeededAt, len(manifest.Objects), path)
}

func mustSoakConsent(t *testing.T, ctx context.Context, consents *ConsentStore, owner string) domain.Consent {
	t.Helper()
	c, err := consents.Get(ctx, owner)
	if err != nil {
		t.Fatalf("get consent: %v", err)
	}
	return c
}

func TestSoakVerify(t *testing.T) {
	ctx, _, objects, inventory, _ := soakSetup(t)
	path := soakEnv("SOAK_MANIFEST", "/tmp/training-soak-manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest (run TestSoakSeed first): %v", err)
	}
	var manifest soakManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	report, err := privacyapp.SweepRevoked(ctx, privacyapp.PurgePorts{
		Clock:     func() time.Time { return now },
		Inventory: inventory,
		Objects:   objects,
	})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	t.Logf("sweep at %s: checked=%d purged=%d missing=%d failures=%d",
		now.Format(time.RFC3339), report.Checked, report.Purged, len(report.Missing), len(report.Failures))
	if len(report.Failures) != 0 {
		t.Errorf("sweep failures: %v", report.Failures)
	}
	for _, o := range manifest.Objects {
		due, err := time.Parse(time.RFC3339, o.DueAt)
		if err != nil {
			t.Fatal(err)
		}
		exists, err := objects.Exists(ctx, o.Key)
		if err != nil {
			t.Errorf("exists %s: %v", o.Key, err)
			continue
		}
		if now.After(due) {
			if exists {
				t.Errorf("LATE OBJECT (failed gate): %s due %s still present at %s", o.Key, o.DueAt, now.Format(time.RFC3339))
			} else {
				revoked, _ := time.Parse(time.RFC3339, o.RevokedAt)
				t.Logf("SLO met for %s: revoked %s, gone by %s (%.1fh)", o.Key, o.RevokedAt, now.Format(time.RFC3339), now.Sub(revoked).Hours())
			}
		} else {
			if !exists {
				t.Errorf("not-yet-due copy vanished early: %s (due %s)", o.Key, o.DueAt)
			} else {
				t.Logf("not yet due, correctly retained: %s (due %s)", o.Key, o.DueAt)
			}
		}
	}
	ctrlExists, err := objects.Exists(ctx, manifest.Control.Key)
	if err != nil || !ctrlExists {
		t.Errorf("live control missing: exists=%v err=%v", ctrlExists, err)
	}
	active, err := inventory.ListActiveByOwner(ctx, "soak-owner")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range active {
		if d.ImageID == "soak-control" {
			found = true
		}
	}
	if !found {
		t.Errorf("live control row not ACTIVE: %+v", active)
	}
	t.Log("soak verify complete")
}
