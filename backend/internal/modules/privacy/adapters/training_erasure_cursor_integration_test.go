//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
)

func ownerErasurePorts(s *PromotionStore, objects app.ObjectStore, at time.Time) app.TrainingErasePorts {
	return app.TrainingErasePorts{Clock: func() time.Time { return at }, Attribution: func(_ context.Context, owner string) (string, error) { return "tok-" + owner, nil }, Consent: consentAppPorts(NewConsentStore(s.pool)), Inventory: s, Objects: objects}
}
func TestOwnerTrainingErasureBoundedRestartAndRestoreOnRealS3(t *testing.T) {
	objects := outputObjectsFixture(t)
	s := promotionPG(t)
	ctx := context.Background()
	at := time.Now().Truncate(time.Millisecond)
	var restoredKey string
	for i := 0; i < 65; i++ {
		d := mustDerivative(t, fmt.Sprintf("owner-copy-%d-%03d", at.UnixNano(), i), "c1", 1, at)
		if _, _, err := s.Put(ctx, d); err != nil {
			t.Fatal(err)
		}
		if i%32 == 0 {
			key := app.TrainingObjectKey(d.ImageID, 1, at)
			if err := objects.Put(ctx, key, []byte("synthetic owner bytes")); err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				restoredKey = key
			}
			t.Cleanup(func() { _ = objects.Delete(ctx, key) })
		}
	}
	control, body := outputRow(t, s, "owner-control", at)
	// Control belongs to another owner and must survive this owner's erasure.
	if _, err := s.pool.Exec(ctx, `UPDATE privacy_training_derivatives SET contributor_id='c2',contributor_ref='tok-c2' WHERE image_id=$1`, control.ImageID); err != nil {
		t.Fatal(err)
	}
	if err := objects.Put(ctx, control.ObjectKey, body); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = objects.Delete(ctx, control.ObjectKey) })
	for i, want := range []int{32, 32, 1} {
		restarted := NewPromotionStore(s.pool)
		covered, purged, err := app.EraseTrainingContributor(ctx, ownerErasurePorts(restarted, objects, at), "c1")
		if covered != want || purged != 1 || ((i < 2) != errors.Is(err, domain.ErrBadState)) {
			t.Fatal("bounded owner continuation", i, covered, purged, err)
		}
		c, err := NewConsentStore(s.pool).Get(ctx, "c1")
		if err != nil || c.Revision != 2 || !c.RevokedAt.Equal(at) {
			t.Fatal("retry moved OFF anchor", c, err)
		}
	}
	var cursors int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM privacy_training_erasure_cursors`).Scan(&cursors); err != nil || cursors != 0 {
		t.Fatal("completed cursor retained", cursors, err)
	}
	if exists, err := objects.Exists(ctx, control.ObjectKey); err != nil || !exists {
		t.Fatal("foreign owner erased", exists, err)
	}
	// Simulate storage restored beside an independent retained PURGED receipt.
	if err := objects.Put(ctx, restoredKey, []byte("synthetic restored bytes")); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{32, 32, 1} {
		_, purged, err := app.EraseTrainingContributor(ctx, ownerErasurePorts(NewPromotionStore(s.pool), objects, at), "c1")
		if ((i < 2) != errors.Is(err, domain.ErrBadState)) || (i == 0 && purged != 1) {
			t.Fatal("restore replay did not resume", want, purged, err)
		}
	}
	if exists, err := objects.Exists(ctx, restoredKey); err != nil || exists {
		t.Fatal("retained receipt hid restored bytes", exists, err)
	}
}
func TestOwnerErasureClaimAndCursorRollbackOnSQL(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	at := time.Now().Truncate(time.Millisecond)
	r, _ := outputRow(t, s, "cursor-rollback", at)
	c, err := NewConsentStore(s.pool).Get(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	off, _, err := domain.RevokeConsent(&c, c.ContributorID, c.ContributorRef, 2, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewConsentStore(s.pool).Put(ctx, off); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var wins atomic.Int32
	claims := make(chan app.PurgeClaim, 1)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, ok, err := s.ClaimOwnerPurge(ctx, "c1", at)
			if err != nil {
				t.Error(err)
			}
			if ok {
				wins.Add(1)
				claims <- claim
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("duplicate owner claims", wins.Load())
	}
	claim := <-claims
	if _, err := s.pool.Exec(ctx, `CREATE FUNCTION refuse_cursor() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic cursor outage'; END $$; CREATE TRIGGER refuse_cursor BEFORE INSERT ON privacy_training_erasure_cursors FOR EACH ROW EXECUTE FUNCTION refuse_cursor()`); err != nil {
		t.Fatal(err)
	}
	if err := s.CompletePurge(ctx, claim, at); err == nil {
		t.Fatal("cursor failure acknowledged")
	}
	outputState(t, s, r, domain.OutputPurging, 0)
	if _, err := s.pool.Exec(ctx, `DROP TRIGGER refuse_cursor ON privacy_training_erasure_cursors`); err != nil {
		t.Fatal(err)
	}
	later := at.Add(2 * time.Minute)
	newClaim, ok, err := NewPromotionStore(s.pool).ClaimOwnerPurge(ctx, "c1", later)
	if err != nil || !ok {
		t.Fatal("rollback lost pending key", ok, err)
	}
	if err := s.CompletePurge(ctx, claim, later); !errors.Is(err, domain.ErrBadState) {
		t.Fatal("old cursor authority accepted", err)
	}
	if err := s.CompletePurge(ctx, newClaim, later); err != nil {
		t.Fatal(err)
	}
	if pending, err := s.HasRemainingErasure(ctx, "c1"); err != nil || pending {
		t.Fatal(pending, err)
	}
	outputState(t, s, r, domain.OutputPurged, 0)
}
func TestTrainingErasureCursorUpgradeAppliesOnly000067(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := freshPre057DB(t, testDSN(t))
	previous, target := fstest.MapFS{}, fstest.MapFS{}
	names, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name > "000067_privacy_training_erasure_cursor.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000067_privacy_training_erasure_cursor.sql" {
			previous[name] = &fstest.MapFile{Data: raw}
		}
	}
	if _, err := migrate.Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `INSERT INTO privacy_training_consents(contributor_id,contributor_ref,terms_version,terms_digest,state,revision,decided_at) VALUES ('kept-owner','synthetic-ref','training-terms-v1','digest-v1','REVOKED',1,now())`); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 1 || applied[0] != "000067" {
		t.Fatal(applied, err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO privacy_training_erasure_cursors VALUES ('kept-owner','image',1,now())`); err != nil {
		t.Fatal(err)
	}
	var kept int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM privacy_training_consents WHERE contributor_id='kept-owner'`).Scan(&kept); err != nil || kept != 1 {
		t.Fatal("upgrade lost history", kept, err)
	}
	applied, err = migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 0 {
		t.Fatal("upgrade replay", applied, err)
	}
}
