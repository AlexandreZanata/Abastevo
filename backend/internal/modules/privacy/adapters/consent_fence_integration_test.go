//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func TestRevocationImmediatelyExcludesAndTombstonesDerivativeOnSQL(t *testing.T) {
	_, pool := freshStore(t)
	ctx := context.Background()
	consents := NewConsentStore(pool)
	inventory := NewPromotionStore(pool)
	now := time.Now().Truncate(time.Millisecond)
	consent := mustConsent(t, "fenced-owner", 1, now.Add(-time.Hour))
	if _, _, err := consents.Put(ctx, consent); err != nil {
		t.Fatal(err)
	}
	d := mustDerivative(t, "fenced-image", "fenced-owner", 1, now)
	if _, _, err := inventory.Put(ctx, d); err != nil {
		t.Fatal(err)
	}
	revoked, _, err := domain.RevokeConsent(&consent, consent.ContributorID, consent.ContributorRef, 2, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := consents.Put(ctx, revoked); err != nil {
		t.Fatal(err)
	}
	active, err := inventory.ListActiveByOwner(ctx, consent.ContributorID)
	if err != nil || len(active) != 0 {
		t.Fatalf("revoked derivative remains active: %+v %v", active, err)
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT state FROM privacy_training_derivatives WHERE image_id=$1", d.ImageID).Scan(&state); err != nil || state != "REVOKED" {
		t.Fatal(state, err)
	}
}

func TestStaleDerivativeCommitAfterRevocationRefusesOnSQL(t *testing.T) {
	_, pool := freshStore(t)
	ctx := context.Background()
	consents := NewConsentStore(pool)
	inventory := NewPromotionStore(pool)
	now := time.Now().Truncate(time.Millisecond)
	consent := mustConsent(t, "race-owner", 1, now.Add(-time.Hour))
	if _, _, err := consents.Put(ctx, consent); err != nil {
		t.Fatal(err)
	}
	stale := mustDerivative(t, "race-image", "race-owner", 1, now)
	revoked, _, err := domain.RevokeConsent(&consent, consent.ContributorID, consent.ContributorRef, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := consents.Put(ctx, revoked); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inventory.Put(ctx, stale); !errors.Is(err, domain.ErrConsentRequired) {
		t.Fatalf("stale commit accepted: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM privacy_training_derivatives").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestRevocationBatchesAndRestoredGrantStayFencedOnSQL(t *testing.T) {
	_, pool := freshStore(t)
	ctx := context.Background()
	consents := NewConsentStore(pool)
	inventory := NewPromotionStore(pool)
	now := time.Now().Truncate(time.Millisecond)
	c := mustConsent(t, "batch-owner", 1, now.Add(-time.Hour))
	if _, _, err := consents.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 65; i++ {
		if _, _, err := inventory.Put(ctx, mustDerivative(t, fmt.Sprintf("batch-%03d", i), "batch-owner", 1, now)); err != nil {
			t.Fatal(err)
		}
	}
	revoked, _, err := domain.RevokeConsent(&c, c.ContributorID, c.ContributorRef, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := consents.Put(ctx, revoked); err != nil {
		t.Fatal(err)
	}
	var tombstones int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM privacy_training_derivatives WHERE state='REVOKED'").Scan(&tombstones); err != nil || tombstones != 32 {
		t.Fatal("unbounded revoke", tombstones, err)
	}
	if active, err := inventory.ListActiveByOwner(ctx, c.ContributorID); err != nil || len(active) != 0 {
		t.Fatal("unmaterialized rows selectable", len(active), err)
	}
	// Simulate a stale restored grant while the independent tombstone survives.
	if _, err := pool.Exec(ctx, "UPDATE privacy_training_consents SET state='GRANTED',revision=1 WHERE contributor_id=$1", c.ContributorID); err != nil {
		t.Fatal(err)
	}
	if active, err := inventory.ListActiveByOwner(ctx, c.ContributorID); err != nil || len(active) != 0 {
		t.Fatal("restore resurrected selection", len(active), err)
	}
	if _, _, err := inventory.Put(ctx, mustDerivative(t, "stale-restore", "batch-owner", 1, now)); !errors.Is(err, domain.ErrConsentRequired) {
		t.Fatal("restore admitted stale derivative", err)
	}
	for _, want := range []int64{32, 1, 0} {
		n, err := NewPromotionStore(pool).ReconcileRevocations(ctx)
		if err != nil || n != want {
			t.Fatal("bounded repair", n, want, err)
		}
	}
	// Re-grant may admit only its own future images; old revisions stay excluded.
	fresh := mustConsent(t, "batch-owner", 3, now.Add(time.Hour))
	if _, _, err := consents.Put(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inventory.Put(ctx, mustDerivative(t, "new-grant", "batch-owner", 3, now.Add(2*time.Hour))); err != nil {
		t.Fatal(err)
	}
	active, err := inventory.ListActiveByOwner(ctx, c.ContributorID)
	if err != nil || len(active) != 1 || active[0].ImageID != "new-grant" {
		t.Fatal(active, err)
	}
	nextOff, _, err := domain.RevokeConsent(&fresh, fresh.ContributorID, fresh.ContributorRef, 4, now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := consents.Put(ctx, nextOff); err != nil {
		t.Fatal(err)
	}
	var oldAt, newAt time.Time
	if err := pool.QueryRow(ctx, "SELECT revoked_at FROM privacy_training_derivatives WHERE image_id='batch-000'").Scan(&oldAt); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT revoked_at FROM privacy_training_derivatives WHERE image_id='new-grant'").Scan(&newAt); err != nil {
		t.Fatal(err)
	}
	if !oldAt.Equal(now) || !newAt.Equal(now.Add(3*time.Hour)) {
		t.Fatal("revocation deadline moved across grant cycles", oldAt, newAt)
	}
}

type pausedSanitizer struct{ entered, resume chan struct{} }

func (s pausedSanitizer) Sanitize(ctx context.Context, _ []byte) ([]byte, domain.SanitizeVerdict, error) {
	close(s.entered)
	select {
	case <-s.resume:
		return []byte("synthetic minimized bytes"), domain.VerdictPass, nil
	case <-ctx.Done():
		return nil, domain.VerdictFail, ctx.Err()
	}
}
func TestRevokeDuringSanitizerRefusesCommitOnSQL(t *testing.T) {
	_, pool := freshStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	consents := NewConsentStore(pool)
	inventory := NewPromotionStore(pool)
	now := time.Now().Truncate(time.Millisecond)
	c := mustConsent(t, "paused-owner", 1, now.Add(-time.Hour))
	if _, _, err := consents.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	sanitizer := pausedSanitizer{entered: make(chan struct{}), resume: make(chan struct{})}
	p := privacyapp.PromotePorts{ConsentPorts: consentAppPorts(consents), Inventory: inventory, Sanitizer: sanitizer}
	result := make(chan error, 1)
	go func() {
		_, _, err := privacyapp.Propose(ctx, p, privacyapp.Caller{ContributorID: c.ContributorID}, privacyapp.PromotionDTO{ImageID: "paused-image", CapturedAt: now, PerImageAllowed: true, Source: []byte("synthetic source")})
		result <- err
	}()
	select {
	case <-sanitizer.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	revoked, _, err := domain.RevokeConsent(&c, c.ContributorID, c.ContributorRef, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := consents.Put(ctx, revoked); err != nil {
		t.Fatal(err)
	}
	close(sanitizer.resume)
	select {
	case err := <-result:
		if !errors.Is(err, domain.ErrConsentRequired) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM privacy_training_derivatives").Scan(&rows); err != nil || rows != 0 {
		t.Fatal(rows, err)
	}
}
func TestConcurrentRevocationAndAdmissionsConvergeOnSQL(t *testing.T) {
	_, pool := freshStore(t)
	ctx := context.Background()
	consents := NewConsentStore(pool)
	inventory := NewPromotionStore(pool)
	now := time.Now().Truncate(time.Millisecond)
	c := mustConsent(t, "parallel-owner", 1, now.Add(-time.Hour))
	if _, _, err := consents.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, _, err := inventory.Put(ctx, mustDerivative(t, fmt.Sprintf("parallel-%02d", i), c.ContributorID, 1, now))
			if err != nil && !errors.Is(err, domain.ErrConsentRequired) {
				t.Error(err)
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		revoked, _, err := domain.RevokeConsent(&c, c.ContributorID, c.ContributorRef, 2, now)
		if err != nil {
			t.Error(err)
			return
		}
		if _, _, err := consents.Put(ctx, revoked); err != nil {
			t.Error(err)
		}
	}()
	close(start)
	wg.Wait()
	active, err := inventory.ListActiveByOwner(ctx, c.ContributorID)
	if err != nil || len(active) != 0 {
		t.Fatal(active, err)
	}
	var unfenced int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM privacy_training_derivatives WHERE state='ACTIVE'").Scan(&unfenced); err != nil || unfenced != 0 {
		t.Fatal(unfenced, err)
	}
}
func TestTrainingRevocationUpgradeBackfillsAndReplaysOnSQL(t *testing.T) {
	before, target := fstest.MapFS{}, fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name > "000064_privacy_training_revocation_fence.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000064_privacy_training_revocation_fence.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	dsn := freshPre057DB(t, testDSN(t))
	ctx := context.Background()
	if _, err := migrate.Apply(ctx, dsn, before); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "INSERT INTO privacy_training_consents(contributor_id,contributor_ref,terms_version,terms_digest,state,revision,decided_at,revoked_at) VALUES('migration-owner','tok-migration','v1','digest','REVOKED',9,now(),now())"); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 1 || applied[0] != "000064" {
		t.Fatal(applied, err)
	}
	var revision int64
	if err := conn.QueryRow(ctx, "SELECT revision FROM privacy_training_revocations WHERE contributor_id='migration-owner'").Scan(&revision); err != nil || revision != 9 {
		t.Fatal(revision, err)
	}
	if applied, err := migrate.Apply(ctx, dsn, target); err != nil || len(applied) != 0 {
		t.Fatal(applied, err)
	}
}

func TestRevocationRollbackKeepsDecisionAndInventoryAtomicOnSQL(t *testing.T) {
	_, pool := freshStore(t)
	ctx := context.Background()
	consents := NewConsentStore(pool)
	inventory := NewPromotionStore(pool)
	now := time.Now().Truncate(time.Millisecond)
	c := mustConsent(t, "rollback-owner", 1, now.Add(-time.Hour))
	if _, _, err := consents.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inventory.Put(ctx, mustDerivative(t, "rollback-image", c.ContributorID, 1, now)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION refuse_revocation_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected revocation failure'; END $$;
 CREATE TRIGGER refuse_revocation_fixture BEFORE INSERT ON privacy_training_revocations FOR EACH ROW EXECUTE FUNCTION refuse_revocation_fixture()`); err != nil {
		t.Fatal(err)
	}
	off, _, err := domain.RevokeConsent(&c, c.ContributorID, c.ContributorRef, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := consents.Put(ctx, off); err == nil {
		t.Fatal("injected failure accepted")
	}
	stored, err := consents.Get(ctx, c.ContributorID)
	if err != nil || stored.State != domain.StateGranted || stored.Revision != 1 {
		t.Fatal("decision partially committed", stored, err)
	}
	active, err := inventory.ListActiveByOwner(ctx, c.ContributorID)
	if err != nil || len(active) != 1 {
		t.Fatal("inventory partially revoked", active, err)
	}
}
