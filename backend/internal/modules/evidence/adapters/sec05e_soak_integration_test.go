//go:build integration

package adapters

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
)

// SEC05-E every-copy 24 h soak verification (AUD-06, B-BR-M01/M04).
// Env-gated: runs only against the named soak services seeded under
// docs/backend/evidence/community-safety/SEC05E_EVERY_COPY_SOAK.md
// (disposable MinIO + disposable PostGIS, seeded copies aging past
// their 24 h deadline). Without ANPFUEL_EVIDENCE_SOAK_DSN it skips:
// a fresh CI database holds no 24 h-old copies to observe.
//
// At due time the test runs one real Sweep (real PG + real MinIO)
// and asserts every copy deleted with markers set, then a quiet
// post-pass signal: the deletion receipt for the seeded set.
func TestSEC05ESoakVerify(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ANPFUEL_EVIDENCE_SOAK_DSN"))
	endpoint := strings.TrimSpace(os.Getenv("ANPFUEL_EVIDENCE_SOAK_S3"))
	bucket := strings.TrimSpace(os.Getenv("ANPFUEL_EVIDENCE_SOAK_BUCKET"))
	if dsn == "" || endpoint == "" || bucket == "" {
		t.Skip("soak services unnamed (ANPFUEL_EVIDENCE_SOAK_DSN/S3/BUCKET); see SEC05E evidence")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewStore(pool)

	before, err := application.AuditOverdue(ctx, store, time.Now(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if !before.Unhealthy() {
		t.Fatalf("soak copies must read overdue at due time, got %+v", before)
	}
	rep, err := application.Sweep(ctx, application.SweepDeps{
		Clock: time.Now,
		Batch: application.DefaultSweepBatch,
		Store: store,
		Jobs:  hasNoLiveJobs{},
		Storage: &soakStorage{
			endpoint: endpoint, bucket: bucket,
			user: os.Getenv("ANPFUEL_EVIDENCE_SOAK_S3_USER"),
			pass: os.Getenv("ANPFUEL_EVIDENCE_SOAK_S3_PASS"),
		},
	})
	if err != nil {
		t.Fatalf("soak sweep: %v", err)
	}
	if rep.StorageErrors != 0 {
		t.Fatalf("soak sweep storage errors = %d", rep.StorageErrors)
	}
	after, err := application.AuditOverdue(ctx, store, time.Now(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if after.Unhealthy() {
		t.Fatalf("post-sweep signal = %+v, want quiet zero: every copy past due", after)
	}
}

type hasNoLiveJobs struct{}

func (hasNoLiveJobs) HasLiveJob(context.Context, string) (bool, error) { return false, nil }

type soakStorage struct {
	endpoint, bucket, user, pass string
}

func (s *soakStorage) deleteKey(ctx context.Context, key, namespace string) error {
	cred := storage.Credentials{AccessKeyID: s.user, SecretAccessKey: s.pass}
	pre, err := storage.PresignDELETE(storage.PresignInput{
		Endpoint: s.endpoint, Bucket: s.bucket, Key: key, Namespace: namespace,
		TTL: storage.DefaultURLTTL,
	}, cred)
	if err != nil {
		return err
	}
	return storage.Delete(ctx, http.DefaultClient, pre.URL)
}

func (s *soakStorage) DeleteQuarantine(ctx context.Context, key string) error {
	return s.deleteKey(ctx, key, "q/")
}

func (s *soakStorage) DeleteFinal(ctx context.Context, key string) error {
	return s.deleteKey(ctx, key, "f/")
}
