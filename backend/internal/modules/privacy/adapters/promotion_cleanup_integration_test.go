//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

type deletionOutage struct{ *TrainingObjects }

func (deletionOutage) Delete(context.Context, string) error {
	return errors.New("synthetic deletion outage")
}
func TestTrainingCleanupOnRealSQLAndPrivateS3(t *testing.T) {
	objects := outputObjectsFixture(t)
	t.Run("failure-restart-and-terminal-ack", func(t *testing.T) {
		s := promotionPG(t)
		ctx := context.Background()
		at := time.Now().Truncate(time.Millisecond)
		r, body := outputRow(t, s, "cleanup", at)
		t.Cleanup(func() { _ = objects.Delete(ctx, r.ObjectKey) })
		if err := app.Publish(ctx, outputPorts(s, objects, func() time.Time { return at }), r, body); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkRevoked(ctx, r.ImageID, 1, at); err != nil {
			t.Fatal(err)
		}
		rep, err := app.SweepRevoked(ctx, app.PurgePorts{Clock: func() time.Time { return at }, Inventory: s, Objects: deletionOutage{objects}})
		if err == nil || rep.Checked != 1 || len(rep.Failures) != 1 {
			t.Fatal("failed deletion acknowledged", rep, err)
		}
		outputState(t, s, r, domain.OutputPurging, 1)
		if exists, err := objects.Exists(ctx, r.ObjectKey); err != nil || !exists {
			t.Fatal("fault did not preserve bytes", exists, err)
		}
		later := at.Add(2 * time.Minute)
		restarted := NewPromotionStore(s.pool)
		rep, err = app.SweepRevoked(ctx, app.PurgePorts{Clock: func() time.Time { return later }, Inventory: restarted, Objects: objects})
		if err != nil || rep.Purged != 1 {
			t.Fatal("restart did not recover", rep, err)
		}
		outputState(t, s, r, domain.OutputPurged, 1)
		if exists, err := objects.Exists(ctx, r.ObjectKey); err != nil || exists {
			t.Fatal("purge byte proof", exists, err)
		}
		rep, err = app.SweepRevoked(ctx, app.PurgePorts{Clock: func() time.Time { return later }, Inventory: restarted, Objects: objects})
		if err != nil || rep.Checked != 0 {
			t.Fatal("purged work remained hot", rep, err)
		}
	})
	t.Run("abandoned-active-writer-stale-ack-and-repair", func(t *testing.T) {
		s := promotionPG(t)
		ctx := context.Background()
		at := time.Now().Truncate(time.Millisecond)
		r, body := outputRow(t, s, "orphan", at)
		t.Cleanup(func() { _ = objects.Delete(ctx, r.ObjectKey) })
		nonce, _ := nonceForOutput()
		if err := s.BeginOutput(ctx, r, nonce, at, domain.TrainingTermsV1, "digest-v1"); err != nil {
			t.Fatal(err)
		}
		if err := objects.Put(ctx, r.ObjectKey, body); err != nil {
			t.Fatal(err)
		}
		rep, err := app.SweepRevoked(ctx, app.PurgePorts{Clock: func() time.Time { return at }, Inventory: s, Objects: objects})
		if err != nil || rep.Checked != 0 {
			t.Fatal("live writer purged", rep, err)
		}
		later := at.Add(2 * time.Minute)
		rep, err = app.SweepRevoked(ctx, app.PurgePorts{Clock: func() time.Time { return later }, Inventory: s, Objects: objects})
		if err != nil || rep.Purged != 1 {
			t.Fatal("abandoned write escaped", rep, err)
		}
		outputState(t, s, r, domain.OutputPending, 1)
		if err := s.FinishOutput(ctx, r, nonce, true, later, domain.TrainingTermsV1, "digest-v1"); !errors.Is(err, domain.ErrBadState) {
			t.Fatal("old writer resurrected output", err)
		}
		if err := s.PrepareOutputCleanup(ctx, r, nonce, later); !errors.Is(err, domain.ErrBadState) {
			t.Fatal("stale cleanup authority survived", err)
		}
		if err := app.Publish(ctx, outputPorts(s, objects, func() time.Time { return later }), r, body); err != nil {
			t.Fatal("cleaned output not repairable", err)
		}
		outputState(t, s, r, domain.OutputReady, 2)
	})
}
func TestTrainingPurgeClaimConcurrencyExpiryAndBacklogOnSQL(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	at := time.Now().Truncate(time.Millisecond)
	r, _ := outputRow(t, s, "lease-purge", at)
	if err := s.MarkRevoked(ctx, r.ImageID, 1, at); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var wins atomic.Int32
	result := make(chan app.PurgeClaim, 1)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, ok, err := s.ClaimPurge(ctx, at)
			if err != nil {
				t.Error(err)
			}
			if ok {
				wins.Add(1)
				result <- c
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("concurrent purge authorities", wins.Load())
	}
	old := <-result
	later := at.Add(2 * time.Minute)
	current, ok, err := s.ClaimPurge(ctx, later)
	if err != nil || !ok || current.Token == old.Token {
		t.Fatal("expired claim not recoverable", ok, err)
	}
	if err := s.CompletePurge(ctx, old, later); !errors.Is(err, domain.ErrBadState) {
		t.Fatal("stale purge acknowledged", err)
	}
	if err := s.CompletePurge(ctx, current, later); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.ClaimPurge(ctx, later); err != nil || ok {
		t.Fatal("completed claim hot", ok, err)
	}
	// Revocation materialization itself is bounded to 32: the recurring
	// reconciler exposes the remainder, while selection is already fenced.
	for i := 0; i < 65; i++ {
		if _, _, err := s.Put(ctx, mustDerivative(t, fmt.Sprintf("cleanup-batch-%03d", i), "c2", 1, at)); err != nil {
			t.Fatal(err)
		}
	}
	c, err := NewConsentStore(s.pool).Get(ctx, "c2")
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
	if _, err := s.ReconcileRevocations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcileRevocations(ctx); err != nil {
		t.Fatal(err)
	}
	empty := &absentCleanupObjects{}
	for _, want := range []int{32, 32, 1, 0} {
		rep, err := app.SweepRevoked(ctx, app.PurgePorts{Clock: func() time.Time { return later }, Inventory: s, Objects: empty})
		if err != nil || rep.Checked != want || len(rep.Missing) != want {
			t.Fatal("unbounded/repeated backlog", rep, err)
		}
	}
}

type absentCleanupObjects struct{}

func (*absentCleanupObjects) Exists(context.Context, string) (bool, error) { return false, nil }
func (*absentCleanupObjects) Delete(context.Context, string) error         { return nil }

func TestTrainingCleanupHotQueryUsesPartialIndexOnSQL(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	at := time.Now().Truncate(time.Millisecond)
	// 10k terminal receipts must not enlarge the hot disposal query.
	if _, err := s.pool.Exec(ctx, `INSERT INTO privacy_training_derivatives(image_id,consent_revision,contributor_id,contributor_ref,source_sha256,derivative_sha256,captured_at,terms_version,terms_digest,state,promoted_at,revoked_at,object_key,output_state,output_purged_at)
 SELECT 'cold-'||n,1,'cold-owner','synthetic-ref','src','sha',now(),'training-terms-v1','digest-v1','REVOKED',now(),now(),'training/v1/cold-'||n||'.jpg','PURGED',now() FROM generate_series(1,10000) n`); err != nil {
		t.Fatal(err)
	}
	r, _ := outputRow(t, s, "hot", at)
	if err := s.MarkRevoked(ctx, r.ImageID, 1, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `ANALYZE privacy_training_derivatives`); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../db/queries/privacy/training_cleanup.sql"))
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Split(string(raw), "-- name: CompleteTrainingPurge")[0]
	query = strings.NewReplacer("@nonce", "$1", "@lease_until", "$2", "@at", "$3").Replace(query)
	token, _ := nonceForOutput()
	nonce, err := mustUUID(token)
	if err != nil {
		t.Fatal(err)
	}
	var plan []byte
	if err := s.pool.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+query, nonce, pgTime(at.Add(time.Minute)), pgTime(at)).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	var parsed any
	if err := json.Unmarshal(plan, &parsed); err != nil {
		t.Fatal(err)
	}
	indexed := false
	var visit func(any)
	visit = func(value any) {
		switch x := value.(type) {
		case []any:
			for _, v := range x {
				visit(v)
			}
		case map[string]any:
			if x["Index Name"] == "privacy_training_cleanup_due_idx" {
				indexed = true
			}
			if x["Relation Name"] == "privacy_training_derivatives" && x["Node Type"] == "Seq Scan" {
				t.Errorf("hot inventory scanned terminal history: %s", plan)
			}
			for _, v := range x {
				visit(v)
			}
		}
	}
	visit(parsed)
	if !indexed {
		t.Fatalf("hot cleanup did not use partial index: %s", plan)
	}
}
func TestTrainingCleanupUpgradeAppliesOnly000066(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := freshPre057DB(t, testDSN(t))
	previous, target := fstest.MapFS{}, fstest.MapFS{}
	names, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name > "000066_privacy_training_cleanup.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000066_privacy_training_cleanup.sql" {
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
	if _, err := conn.Exec(ctx, `INSERT INTO privacy_training_derivatives(image_id,consent_revision,contributor_id,contributor_ref,source_sha256,derivative_sha256,captured_at,terms_version,terms_digest,state,promoted_at,object_key) VALUES ('kept-output',1,'owner','synthetic-ref','src','sha',now(),'training-terms-v1','digest-v1','ACTIVE',now(),'training/v1/kept.jpg')`); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 1 || applied[0] != "000066" {
		t.Fatal(applied, err)
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM privacy_training_derivatives WHERE image_id='kept-output' AND output_state='PENDING'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("upgrade lost output history", count, err)
	}
	applied, err = migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 0 {
		t.Fatal("upgrade replay", applied, err)
	}
}
