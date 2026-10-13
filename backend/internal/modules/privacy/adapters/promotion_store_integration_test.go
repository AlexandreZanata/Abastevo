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
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// CS10B: derivative-inventory persistence over real disposable
// PostGIS. Writes converge on the image-plus-revision natural key:
// identical re-proposals replay, divergent hashes conflict, and
// revocation flips ACTIVE to REVOKED through a guarded transition.
// No row removal exists anywhere in this task (purge owns bytes in
// CS10C; receipts stay for the restore filter).

func promotionPG(t *testing.T) *PromotionStore {
	t.Helper()
	_, pool := freshStore(t)
	for _, owner := range []string{"c1", "c2"} {
		if _, _, err := NewConsentStore(pool).Put(context.Background(), mustConsent(t, owner, 1, time.Now().Add(-time.Hour))); err != nil {
			t.Fatal(err)
		}
	}
	return NewPromotionStore(pool)
}

func mustDerivative(t *testing.T, image, owner string, revision uint64, at time.Time) domain.Derivative {
	t.Helper()
	consent, _, err := domain.GrantConsent(nil, domain.ConsentParams{
		ContributorID: owner, ContributorRef: "tok-" + owner,
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1",
		Revision: revision, DecidedAt: at.Add(-time.Hour),
	}, domain.TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	d, _, err := domain.ProposeDerivative(domain.PromotionCandidate{
		ImageID: image, ContributorID: owner, ContributorRef: "tok-" + owner,
		CapturedAt: at, PerImageAllowed: true, SourceSHA256: "source-" + image,
		QueuedAtConsentRevision: revision,
	}, &consent, domain.VerdictPass, "derivative-"+image, at,
		domain.TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return d
}

func TestDerivativePutPersistsAndReadsBack(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	want := mustDerivative(t, "img-1", "c1", 1, base)
	stored, replayed, err := s.Put(ctx, want)
	if err != nil || !stored || replayed {
		t.Fatalf("put = %v %v %v", stored, replayed, err)
	}
	pending, err := s.Get(ctx, "img-1", 1)
	if err != nil || pending.OutputState != domain.OutputPending {
		t.Fatal("insert pretended bytes were ready", pending, err)
	}
	if selected, err := s.ListActiveByOwner(ctx, "c1"); err != nil || len(selected) != 0 {
		t.Fatal("pending output selected", selected, err)
	}
	readyInventoryFixture(t, s, "img-1")
	active, err := s.ListActiveByOwner(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].DerivativeSHA256 != want.DerivativeSHA256 {
		t.Fatalf("list = %+v", active)
	}
	foreign, err := s.ListActiveByOwner(ctx, "c2")
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign list = %+v, %v", foreign, err)
	}
}

func TestDerivativePutConvergesAndConflicts(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	first := mustDerivative(t, "img-1", "c1", 1, base)
	if _, _, err := s.Put(ctx, first); err != nil {
		t.Fatal(err)
	}
	stored, replayed, err := s.Put(ctx, first)
	if err != nil || stored || !replayed {
		t.Errorf("identical retry = %v %v %v, want replay", stored, replayed, err)
	}
	divergent := first
	divergent.DerivativeSHA256 = "derivative-evil"
	if _, _, err := s.Put(ctx, divergent); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("divergent retry = %v, want conflict", err)
	}
}

func TestDerivativeConcurrentPutsAgree(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = s.Put(ctx, mustDerivative(t, "img-1", "c1", 1, base))
		}()
	}
	wg.Wait()
	active, err := s.ListByOwner(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].State != domain.DerivativeActive {
		t.Fatalf("rows = %+v", active)
	}
}

func TestDerivativeRevokeFlow(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	if _, _, err := s.Put(ctx, mustDerivative(t, "img-1", "c1", 1, base)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Put(ctx, mustDerivative(t, "img-2", "c1", 1, base)); err != nil {
		t.Fatal(err)
	}
	readyInventoryFixture(t, s, "img-1")
	readyInventoryFixture(t, s, "img-2")
	if err := s.MarkRevoked(ctx, "img-1", 1, base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	active, err := s.ListActiveByOwner(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ImageID != "img-2" {
		t.Fatalf("active = %+v", active)
	}
	// Repeating the mark converges; unknown rows refuse.
	if err := s.MarkRevoked(ctx, "img-1", 1, base.Add(2*time.Hour)); err != nil {
		t.Errorf("repeat mark = %v", err)
	}
	if err := s.MarkRevoked(ctx, "img-ghost", 1, base); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ghost mark = %v, want not found", err)
	}
}

func TestDerivativeUpgradeAppliesOnly000057(t *testing.T) {
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	previous, target := fstest.MapFS{}, fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name > "000057_privacy_training_derivatives.sql" {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: data}
		if name < "000057_privacy_training_derivatives.sql" {
			previous[name] = &fstest.MapFile{Data: data}
		}
	}
	dsn := freshPre057DB(t, adminDSN)
	if _, err := migrate.Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `INSERT INTO privacy_training_consents(contributor_id,contributor_ref,terms_version,terms_digest,state,revision,decided_at) VALUES ('c1','tok-c1','training-terms-v1','digest-v1','GRANTED',1,now())`); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 1 || applied[0] != "000057" {
		t.Fatalf("upgrade %v %v", applied, err)
	}
	var tables int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename='privacy_training_derivatives'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("tables %d %v", tables, err)
	}
	var kept int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM privacy_training_consents WHERE contributor_id='c1'`).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("consent rows %d %v", kept, err)
	}
	again, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay %v %v", again, err)
	}
}

func freshPre057DB(t *testing.T, adminDSN string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("privacy_pre057_%d", time.Now().UnixNano())
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

func TestDerivativeQueriesStayNonDestructive(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "privacy", "derivatives.sql"))
	if err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(string(raw))
	// Exactly two mutating paths: the insert-once put and the guarded
	// ACTIVE-to-REVOKED mark. No row removal, no schema change, no
	// touch of sibling tables.
	if got := strings.Count(upper, "INSERT INTO PRIVACY_TRAINING_DERIVATIVES"); got != 1 {
		t.Errorf("derivatives.sql has %d INSERTs, want exactly the converged put", got)
	}
	if got := strings.Count(upper, "UPDATE PRIVACY_TRAINING_DERIVATIVES"); got != 1 {
		t.Errorf("derivatives.sql has %d UPDATEs, want exactly the guarded revoke mark", got)
	}
	for _, verb := range []string{"DELETE FROM", "TRUNCATE", "DROP ", "ALTER "} {
		if strings.Contains(upper, verb) {
			t.Errorf("derivatives.sql destroys or migrates: %s", verb)
		}
	}
	for _, other := range []string{"PRIVACY_REQUESTS", "PRIVACY_DELETION_LEDGER"} {
		if strings.Contains(upper, other) {
			t.Errorf("derivatives.sql crosses into %s", other)
		}
	}
	if !strings.Contains(upper, "PRIVACY_TRAINING_CONSENTS") || !strings.Contains(upper, "PRIVACY_TRAINING_REVOCATIONS") {
		t.Error("selection lost consent/restore fence")
	}
	if !strings.Contains(upper, "STATE = 'ACTIVE'") && !strings.Contains(upper, "D.STATE = 'ACTIVE'") {
		t.Error("revoke mark lost its state guard")
	}
}

// SQL-only selection fixture: byte verification is separately exercised by
// TestTrainingOutputRecoveryOnPrivateS3. A raw insert is never READY.
func readyInventoryFixture(t *testing.T, s *PromotionStore, image string) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), "UPDATE privacy_training_derivatives SET output_state='READY',output_ready_at=now() WHERE image_id=$1", image); err != nil {
		t.Fatal(err)
	}
}
