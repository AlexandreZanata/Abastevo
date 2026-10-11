//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// CS09B: training-consent persistence over real disposable PostGIS.
// The singleton upsert carries the CS09A revision guard into SQL:
// strictly greater revisions move state, equal + identical replays,
// equal + divergent conflicts, smaller is stale. No per-image rows
// land here (promotion inventory is CS10); no HTTP in this task.

func mustConsent(t *testing.T, contributorID string, revision uint64, at time.Time) domain.Consent {
	t.Helper()
	c, _, err := domain.GrantConsent(nil, domain.ConsentParams{
		ContributorID: contributorID, ContributorRef: "tok-" + contributorID,
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1",
		Revision: revision, DecidedAt: at,
	}, domain.TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatalf("grant consent: %v", err)
	}
	return c
}

func consentPG(t *testing.T) *ConsentStore {
	t.Helper()
	_, pool := freshStore(t)
	return NewConsentStore(pool)
}

func consentAppPorts(s *ConsentStore) privacyapp.ConsentPorts {
	return privacyapp.ConsentPorts{
		Clock:       time.Now,
		Attribution: func(_ context.Context, contributorID string) (string, error) { return "tok-" + contributorID, nil },
		Store:       s,
		Terms:       fakeConsentTerms{},
	}
}

type fakeConsentTerms struct{}

func (fakeConsentTerms) CurrentVersion() string { return domain.TrainingTermsV1 }
func (fakeConsentTerms) CurrentDigest() string  { return "digest-v1" }

func TestConsentGrantPersistsAndReadsBack(t *testing.T) {
	s := consentPG(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	want := mustConsent(t, "c1", 1, base)
	stored, replayed, err := s.Put(ctx, want)
	if err != nil || !stored || replayed {
		t.Fatalf("put = %v %v %v", stored, replayed, err)
	}
	got, err := s.Get(ctx, "c1")
	if err != nil {
		t.Fatalf("get = %v", err)
	}
	if got != want {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestConsentRevisionGuardConverges(t *testing.T) {
	s := consentPG(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	first := mustConsent(t, "c1", 2, base)
	if _, _, err := s.Put(ctx, first); err != nil {
		t.Fatal(err)
	}
	// Identical revision + identical row converges without a write.
	stored, replayed, err := s.Put(ctx, first)
	if err != nil || stored || !replayed {
		t.Errorf("identical retry = %v %v %v, want replay", stored, replayed, err)
	}
	// Same revision with a divergent decision conflicts.
	divergent := first
	divergent.TermsDigest = "digest-evil"
	if _, _, err := s.Put(ctx, divergent); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("divergent retry = %v, want conflict", err)
	}
	// A smaller revision is stale, even with current terms.
	older := mustConsent(t, "c1", 1, base.Add(-time.Hour))
	if _, _, err := s.Put(ctx, older); !errors.Is(err, domain.ErrStaleConsent) {
		t.Errorf("older revision = %v, want stale", err)
	}
	// A greater revision moves state (revoke wins over the grant).
	revoked, _, err := domain.RevokeConsent(&first, "c1", "tok-c1", 3, base.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Put(ctx, revoked); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "c1")
	if err != nil || got.State != domain.StateRevoked || got.Revision != 3 {
		t.Errorf("revoked row = %+v, %v", got, err)
	}
}

func TestConsentConcurrentUpsertsAgree(t *testing.T) {
	s := consentPG(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	var wg sync.WaitGroup
	for rev := uint64(1); rev <= 10; rev++ {
		wg.Add(1)
		go func(r uint64) {
			defer wg.Done()
			c := mustConsent(t, "c1", r, base.Add(time.Duration(r)*time.Second))
			_, _, _ = s.Put(ctx, c)
		}(rev)
	}
	wg.Wait()
	got, err := s.Get(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	// Concurrent first-writes race on the same singleton: every writer
	// uses a distinct revision, so the surviving row is one of the ten
	// attempted revisions with its exact (revision, instant) pair.
	if got.Revision < 1 || got.Revision > 10 {
		t.Fatalf("revision = %d", got.Revision)
	}
	if !got.DecidedAt.Equal(base.Add(time.Duration(got.Revision) * time.Second)) {
		t.Errorf("row = %+v, revision/instant pair torn", got)
	}
}

func TestConsentOwnerIsolation(t *testing.T) {
	s := consentPG(t)
	ctx := context.Background()
	if _, err := s.Get(ctx, "nobody"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("absent get = %v, want not found", err)
	}
	if _, _, err := s.Put(ctx, mustConsent(t, "owner-a", 1, time.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "owner-b"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign get = %v, want not found", err)
	}
}

func TestConsentApplicationFlowOnPostgres(t *testing.T) {
	s := consentPG(t)
	ctx := context.Background()
	p := consentAppPorts(s)
	grant, err := privacyapp.GrantConsent(ctx, p, privacyapp.Caller{ContributorID: "c1"}, privacyapp.GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 1,
	})
	if err != nil || grant.Replayed || grant.State != domain.StateGranted {
		t.Fatalf("grant = %+v, %v", grant, err)
	}
	view, err := privacyapp.ReadConsent(ctx, p, privacyapp.Caller{ContributorID: "c1"})
	if err != nil || !view.Granted || view.Revision != 1 {
		t.Fatalf("status = %+v, %v", view, err)
	}
	rev, err := privacyapp.RevokeConsent(ctx, p, privacyapp.Caller{ContributorID: "c1"}, privacyapp.RevokeDTO{Revision: 2})
	if err != nil || !rev.DeleteOwnedTrainingAssets {
		t.Fatalf("revoke = %+v, %v", rev, err)
	}
	// A stale offline ON queued at the revoked revision cannot reactivate.
	if _, err := privacyapp.GrantConsent(ctx, p, privacyapp.Caller{ContributorID: "c1"}, privacyapp.GrantDTO{
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1", Revision: 2,
	}); !errors.Is(err, domain.ErrStaleConsent) && !errors.Is(err, domain.ErrConflict) {
		t.Errorf("stale grant = %v", err)
	}
	ok, err := privacyapp.CanSelectTraining(ctx, p, privacyapp.Caller{ContributorID: "c1"}, privacyapp.ImageSelectionDTO{
		ImageID: "img-1", CapturedAt: time.Now(), PerImageAllowed: true,
	})
	if err != nil || ok {
		t.Errorf("post-revoke selection = %v, %v", ok, err)
	}
}

func TestConsentUpgradeAppliesOnly000056(t *testing.T) {
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	previous := fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name == "000056_privacy_training_consent.sql" {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: data}
	}
	dsn := freshPre056DB(t, adminDSN)
	if _, err := migrate.Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	// Pre-existing privacy history survives the upgrade untouched.
	if _, err := conn.Exec(ctx, `INSERT INTO privacy_requests(id,contributor_id,contributor_ref,client_submission_id,type,status,requested_at,policy_version) VALUES ('e0000000-0000-4000-8000-000000000001','c1','tok-c1','export-1','EXPORT','REQUESTED',now(),'privacy-v1')`); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(applied) != 1 || applied[0] != "000056" {
		t.Fatalf("upgrade %v %v", applied, err)
	}
	var tables int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename='privacy_training_consents'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("tables %d %v", tables, err)
	}
	var kept int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM privacy_requests WHERE client_submission_id='export-1'`).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("history rows %d %v", kept, err)
	}
	again, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay %v %v", again, err)
	}
}

func freshPre056DB(t *testing.T, adminDSN string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("privacy_pre056_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

func TestConsentQueriesStayNonDestructive(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "privacy", "consent.sql"))
	if err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(string(raw))
	// Exactly one mutating path: the revision-guarded singleton upsert
	// (ON CONFLICT ... DO UPDATE with the revision guard below).
	// No row removal, no schema change, no touch of sibling tables.
	if got := strings.Count(upper, "DO UPDATE SET"); got != 1 {
		t.Errorf("consent.sql has %d upserts, want exactly the guarded singleton write", got)
	}
	for _, verb := range []string{"DELETE FROM", "TRUNCATE", "DROP ", "ALTER "} {
		if strings.Contains(upper, verb) {
			t.Errorf("consent.sql destroys or migrates: %s", verb)
		}
	}
	for _, other := range []string{"PRIVACY_REQUESTS", "PRIVACY_DELETION_LEDGER", "FEEDBACK_", "RATING", "CONSENSUS"} {
		if strings.Contains(upper, other) {
			t.Errorf("consent.sql crosses into %s", other)
		}
	}
	if !strings.Contains(upper, "EXCLUDED.REVISION >") {
		t.Error("upsert lost its revision guard")
	}
}
