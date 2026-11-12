//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditstore "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	db      *orgtest.Fixture
	pool    *pgxpool.Pool
	store   *adapters.Store
	service application.Service
	catalog domain.Catalog
	queue   *jobs.Queue
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{db: orgtest.New(t)}
	raw, err := os.ReadFile("../../../../db/templates/organization_private.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Admin.Exec(t.Context(), string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Admin.Exec(t.Context(), `CREATE TABLE synthetic_org_projection(org_id uuid NOT NULL,record_id uuid NOT NULL,total integer NOT NULL DEFAULT 0,PRIMARY KEY(org_id,record_id));
	ALTER TABLE synthetic_org_projection ENABLE ROW LEVEL SECURITY;ALTER TABLE synthetic_org_projection FORCE ROW LEVEL SECURITY;
	CREATE POLICY synthetic_scope ON synthetic_org_projection
	USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
	WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);`); err != nil {
		t.Fatal(err)
	}
	for _, org := range f.db.Organizations {
		if _, err := f.db.Admin.Exec(t.Context(), "INSERT INTO org_example_parent(org_id,id,number) VALUES($1,$2,$3)", org.ID, org.ParentID, org.Number); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Admin.Exec(t.Context(), "INSERT INTO org_example_record(org_id,id,parent_id,number) VALUES($1,$2,$3,$4)", org.ID, org.RecordID, org.ParentID, org.Number); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Admin.Exec(t.Context(), "INSERT INTO synthetic_org_projection(org_id,record_id) VALUES($1,$2)", org.ID, org.RecordID); err != nil {
			t.Fatal(err)
		}
	}
	role := pgx.Identifier{f.db.RuntimeRole}.Sanitize()
	if _, err := f.db.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_audit_events,org_event_intents,org_event_receipts TO "+role+
		"; GRANT SELECT,INSERT,UPDATE ON job_queue,org_example_record,synthetic_org_projection TO "+role); err != nil {
		t.Fatal(err)
	}
	f.pool = f.db.RuntimePool(t, 12)
	f.store = adapters.NewStore(f.pool)
	f.queue = jobs.NewQueue(f.pool)
	f.catalog, err = domain.NewCatalog([]domain.Definition{{Name: "example.record.changed", TargetType: "example.record", Consumers: []string{"projection.refresh", "review.prepare"}}})
	if err != nil {
		t.Fatal(err)
	}
	ac, err := audit.NewCatalog([]audit.Definition{{Action: "example.record.changed", TargetType: "example.record"}})
	if err != nil {
		t.Fatal(err)
	}
	f.service = application.Service{Enabled: true, Catalog: f.catalog, Audit: auditstore.NewStore(f.pool, ac), Writer: f.store}
	return f
}
func (f *fixture) intent(t *testing.T, org orgtest.Organization) (audit.Event, domain.Event) {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return audit.Event{OperationID: id, ActorID: org.ParentID, Action: "example.record.changed", TargetType: "example.record", TargetID: org.RecordID, RequestHash: strings.Repeat("a", 64)},
		domain.Event{ID: id, Name: "example.record.changed", TargetType: "example.record", TargetID: org.RecordID, Revision: 2}
}
func sourceChange(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx) error {
	tag, err := tx.Exec(ctx, "UPDATE org_example_record SET revision=revision+1 WHERE org_id=$1", scope.OrgID().String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("synthetic missing source")
	}
	return nil
}
func projection(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx, e domain.Event) error {
	tag, err := tx.Exec(ctx, "UPDATE synthetic_org_projection SET total=total+1 WHERE org_id=$1 AND record_id=$2", scope.OrgID().String(), e.TargetID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("synthetic foreign projection")
	}
	return nil
}
func (f *fixture) handler(t *testing.T, effect adapters.Effect) *adapters.Handler {
	t.Helper()
	h, err := adapters.NewHandler(f.store, f.catalog, map[string]adapters.Effect{"projection.refresh": effect, "review.prepare": effect})
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func (f *fixture) publish(t *testing.T, org orgtest.Organization, consumers ...string) {
	t.Helper()
	a, e := f.intent(t, org)
	if replay, err := f.service.Record(t.Context(), org.Scope(t), a, e, consumers, sourceChange); err != nil || replay {
		t.Fatal(replay, err)
	}
}
func (f *fixture) state(t *testing.T, org orgtest.Organization, auditCount, intents, receipts, revision, projected int) {
	t.Helper()
	var a, i, r, v, p int
	err := f.db.Admin.QueryRow(t.Context(), `SELECT
	(SELECT count(*) FROM org_audit_events WHERE org_id=$1),
	(SELECT count(*) FROM org_event_intents WHERE org_id=$1),
	(SELECT count(*) FROM org_event_receipts WHERE org_id=$1),
	(SELECT revision FROM org_example_record WHERE org_id=$1),
	(SELECT total FROM synthetic_org_projection WHERE org_id=$1)`, org.ID).Scan(&a, &i, &r, &v, &p)
	if err != nil || a != auditCount || i != intents || r != receipts || v != revision || p != projected {
		t.Fatalf("state %d/%d/%d/%d/%d error=%v expected %d/%d/%d/%d/%d", a, i, r, v, p, err, auditCount, intents, receipts, revision, projected)
	}
}
func (f *fixture) jobs(t *testing.T, want int) {
	t.Helper()
	var n int
	if err := f.db.Admin.QueryRow(t.Context(), "SELECT count(*) FROM job_queue").Scan(&n); err != nil || n != want {
		t.Fatal("queue count", n, want, err)
	}
}
func (f *fixture) claim(t *testing.T) jobs.Job {
	t.Helper()
	job, err := f.queue.Claim(t.Context(), domain.JobKind, "synthetic-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestExpiredEffectRetainsQueueFenceUntilRollback(t *testing.T) {
	f := setup(t)
	org := f.db.Organizations[0]
	f.publish(t, org, "projection.refresh")
	job := f.claim(t)
	if _, err := f.db.Admin.Exec(t.Context(), "UPDATE job_queue SET lease_expires_at=clock_timestamp()+interval '500 milliseconds' WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	h := f.handler(t, func(ctx context.Context, s kernel.OrgScope, tx pgx.Tx, e domain.Event) error {
		if err := projection(ctx, s, tx, e); err != nil {
			return err
		}
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	done := make(chan error, 1)
	go func() { done <- h.Handle(t.Context(), job) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("effect never entered")
	}
	// Real clock expiry while the consumer retains the queue row lock.
	if _, err := f.db.Admin.Exec(t.Context(), "SELECT pg_sleep(0.6)"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.queue.Claim(t.Context(), domain.JobKind, "competing-worker", time.Minute); !errors.Is(err, jobs.ErrNoJob) {
		t.Fatal("claim superseded locked effect", err)
	}
	unblock()
	select {
	case err := <-done:
		if !errors.Is(err, jobs.ErrStaleLease) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("effect did not stop")
	}
	f.state(t, org, 1, 1, 0, 2, 0)
	retry := f.claim(t)
	if retry.LeaseToken <= job.LeaseToken {
		t.Fatal("retry reused lease")
	}
	if err := f.handler(t, projection).Handle(t.Context(), retry); err != nil {
		t.Fatal(err)
	}
	f.state(t, org, 1, 1, 1, 2, 1)
}

func TestUnknownQueueProvenanceAndUnwiredEffectsFailClosed(t *testing.T) {
	f := setup(t)
	org := f.db.Organizations[0]
	a, e := f.intent(t, org)
	env := domain.Envelope{Version: 1, OrgID: org.ID, Consumer: "projection.refresh", Event: e}
	if _, err := jobs.Enqueue(t.Context(), f.db.Admin, "synthetic-unrelated", []byte(`{"version":1}`), env.DedupeKey(), 5, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Record(t.Context(), org.Scope(t), a, e, []string{"projection.refresh"}, sourceChange); !errors.Is(err, domain.ErrReused) {
		t.Fatal("unknown queue row adopted", err)
	}
	f.state(t, org, 0, 0, 0, 1, 0)
	f.jobs(t, 1)
	for _, effects := range []map[string]adapters.Effect{
		nil, {"projection.refresh": projection},
		{"projection.refresh": projection, "unknown.consumer": projection},
		{"projection.refresh": projection, "review.prepare": nil},
	} {
		if _, err := adapters.NewHandler(f.store, f.catalog, effects); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("incomplete registration accepted", err)
		}
	}
	h := f.handler(t, projection)
	valid := jobs.Job{ID: a.OperationID, Kind: domain.JobKind, Status: "leased", LeaseToken: 1, WorkerID: "synthetic", MaxAttempts: 5, Attempts: 1, Payload: env.Payload(), DedupeKey: env.DedupeKey()}
	for _, kind := range []string{"id", "kind", "status", "token", "worker", "attempts", "payload", "dedupe", "consumer"} {
		job := valid
		switch kind {
		case "id":
			job.ID = "private-invalid-reference"
		case "kind":
			job.Kind = "unknown"
		case "status":
			job.Status = "ready"
		case "token":
			job.LeaseToken = 0
		case "worker":
			job.WorkerID = ""
		case "attempts":
			job.MaxAttempts = 0
		case "payload":
			job.Payload = nil
		case "dedupe":
			job.DedupeKey = "unknown"
		case "consumer":
			changed := env
			changed.Consumer = "unknown.consumer"
			job.Payload = changed.Payload()
		}
		if err := h.Handle(t.Context(), job); !errors.Is(err, domain.ErrInvalid) && !errors.Is(err, domain.ErrDenied) {
			t.Fatal(kind, err)
		}
	}
	// Borrowing a scope different from the transaction setting is refused
	// before a queue or intent write, even with colliding source IDs.
	if err := database.WithOrgTx(t.Context(), f.pool, org.Scope(t), func(tx pgx.Tx) error {
		foreign := f.db.Organizations[1]
		changed := env
		changed.OrgID = foreign.ID
		if err := f.store.Enqueue(t.Context(), org.Scope(t), tx, changed, a.RequestHash); !errors.Is(err, domain.ErrInvalid) {
			return errors.New("scope mismatch accepted")
		}
		if err := f.store.Enqueue(t.Context(), foreign.Scope(t), tx, changed, a.RequestHash); !errors.Is(err, domain.ErrDenied) {
			return errors.New("transaction scope mismatch accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicAuditedPublicationAndScopedReplay(t *testing.T) {
	f := setup(t)
	a, b := f.db.Organizations[0], f.db.Organizations[1]
	command, event := f.intent(t, a)
	var calls atomic.Int32
	work := func(ctx context.Context, s kernel.OrgScope, tx pgx.Tx) error {
		calls.Add(1)
		return sourceChange(ctx, s, tx)
	}
	for attempt := range 2 {
		replay, err := f.service.Record(t.Context(), a.Scope(t), command, event, []string{"review.prepare", "projection.refresh"}, work)
		if err != nil || replay != (attempt == 1) {
			t.Fatal(replay, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("replay repeated source change")
	}
	f.state(t, a, 1, 2, 0, 2, 0)
	f.state(t, b, 0, 0, 0, 1, 0)
	f.jobs(t, 2)
	event.Revision++
	if _, err := f.service.Record(t.Context(), a.Scope(t), command, event, []string{"projection.refresh", "review.prepare"}, work); !errors.Is(err, domain.ErrReused) {
		t.Fatal("changed event replayed", err)
	}
	event.Revision--
	if _, err := f.service.Record(t.Context(), a.Scope(t), command, event, []string{"projection.refresh"}, work); !errors.Is(err, domain.ErrReused) {
		t.Fatal("changed consumers replayed", err)
	}
	other := command
	other.ActorID = b.ParentID
	other.TargetID = b.RecordID
	otherEvent := event
	otherEvent.TargetID = b.RecordID
	if _, err := f.service.Record(t.Context(), b.Scope(t), other, otherEvent, []string{"projection.refresh"}, sourceChange); err != nil {
		t.Fatal("foreign org ID collision", err)
	}
	f.state(t, b, 1, 1, 0, 2, 0)
	f.jobs(t, 3)
}

func TestPublicationFailureRollsBackEveryRequiredEffect(t *testing.T) {
	for _, kind := range []string{"business", "refusal", "queue", "second consumer", "intent", "commit"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			org := f.db.Organizations[0]
			a, e := f.intent(t, org)
			role := pgx.Identifier{f.db.RuntimeRole}.Sanitize()
			work := application.Work(sourceChange)
			want := error(domain.ErrUnavailable)
			var cleanup string
			switch kind {
			case "business":
				work = func(ctx context.Context, s kernel.OrgScope, tx pgx.Tx) error {
					if err := sourceChange(ctx, s, tx); err != nil {
						return err
					}
					return errors.New("PRIVATE-BUSINESS-CONTENT")
				}
			case "refusal":
				// A stale-revision refusal after the source write keeps its 409 and rolls back.
				work = func(ctx context.Context, s kernel.OrgScope, tx pgx.Tx) error {
					if err := sourceChange(ctx, s, tx); err != nil {
						return err
					}
					return fmt.Errorf("PRIVATE-REVISION: %w", orgapi.Conflict)
				}
				want = orgapi.Conflict
			case "queue":
				cleanup = "GRANT INSERT ON job_queue TO " + role
				if _, err := f.db.Admin.Exec(t.Context(), "REVOKE INSERT ON job_queue FROM "+role); err != nil {
					t.Fatal(err)
				}
			case "intent":
				cleanup = "GRANT INSERT ON org_event_intents TO " + role
				if _, err := f.db.Admin.Exec(t.Context(), "REVOKE INSERT ON org_event_intents FROM "+role); err != nil {
					t.Fatal(err)
				}
			case "second consumer":
				cleanup = "DROP TRIGGER synthetic_queue_failure ON job_queue"
				if _, err := f.db.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_queue_failure() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN IF NEW.payload->>'consumer'='review.prepare' THEN RAISE EXCEPTION 'PRIVATE-QUEUE-CONTENT'; END IF; RETURN NEW; END $$;
				CREATE TRIGGER synthetic_queue_failure BEFORE INSERT ON job_queue FOR EACH ROW EXECUTE FUNCTION synthetic_queue_failure();`); err != nil {
					t.Fatal(err)
				}
			case "commit":
				cleanup = "DROP TRIGGER synthetic_commit_failure ON org_audit_events"
				if _, err := f.db.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'PRIVATE-COMMIT-CONTENT'; END $$;
				CREATE CONSTRAINT TRIGGER synthetic_commit_failure AFTER INSERT ON org_audit_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_commit_failure();`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.service.Record(t.Context(), org.Scope(t), a, e, []string{"projection.refresh", "review.prepare"}, work); err != want {
				t.Fatal("failure released success", err)
			}
			f.state(t, org, 0, 0, 0, 1, 0)
			f.jobs(t, 0)
			if cleanup != "" {
				if _, err := f.db.Admin.Exec(t.Context(), cleanup); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.service.Record(t.Context(), org.Scope(t), a, e, []string{"projection.refresh", "review.prepare"}, sourceChange); err != nil {
				t.Fatal("retry did not recover", err)
			}
			f.state(t, org, 1, 2, 0, 2, 0)
			f.jobs(t, 2)
		})
	}
}

func TestConsumerAcknowledgementLossAndLeaseFencing(t *testing.T) {
	f := setup(t)
	org := f.db.Organizations[0]
	f.publish(t, org, "projection.refresh")
	h := f.handler(t, projection)
	old := f.claim(t)
	if err := h.Handle(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	f.state(t, org, 1, 1, 1, 2, 1)
	if _, err := f.db.Admin.Exec(t.Context(), "UPDATE job_queue SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", old.ID); err != nil {
		t.Fatal(err)
	}
	current := f.claim(t)
	if current.ID != old.ID || current.LeaseToken <= old.LeaseToken {
		t.Fatal("not reclaimed")
	}
	if err := h.Handle(t.Context(), old); !errors.Is(err, jobs.ErrStaleLease) {
		t.Fatal("old lease applied", err)
	}
	if err := h.Handle(t.Context(), current); err != nil {
		t.Fatal(err)
	}
	if err := f.queue.Complete(t.Context(), old.ID, old.LeaseToken); !errors.Is(err, jobs.ErrStaleLease) {
		t.Fatal("old ack accepted", err)
	}
	if err := f.queue.Complete(t.Context(), current.ID, current.LeaseToken); err != nil {
		t.Fatal(err)
	}
	f.state(t, org, 1, 1, 1, 2, 1)
}

func TestConcurrentPublicationAndRedeliveryConverge(t *testing.T) {
	f := setup(t)
	org := f.db.Organizations[0]
	a, e := f.intent(t, org)
	var workCount atomic.Int32
	scope := org.Scope(t)
	work := func(ctx context.Context, s kernel.OrgScope, tx pgx.Tx) error {
		workCount.Add(1)
		return sourceChange(ctx, s, tx)
	}
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := f.service.Record(t.Context(), scope, a, e, []string{"projection.refresh"}, work); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if workCount.Load() != 1 {
		t.Fatal("duplicate source effects")
	}
	job := f.claim(t)
	var effects atomic.Int32
	h := f.handler(t, func(ctx context.Context, s kernel.OrgScope, tx pgx.Tx, e domain.Event) error {
		effects.Add(1)
		return projection(ctx, s, tx, e)
	})
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := h.Handle(t.Context(), job); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if effects.Load() != 1 {
		t.Fatal("duplicate consumer effects")
	}
	f.state(t, org, 1, 1, 1, 2, 1)
	f.jobs(t, 1)
}

func TestConsumerFailureCommitAndExpiredLeaseLeaveNoReceipt(t *testing.T) {
	for _, kind := range []string{"callback", "receipt", "commit", "lease", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			org := f.db.Organizations[0]
			f.publish(t, org, "projection.refresh")
			job := f.claim(t)
			effect := adapters.Effect(projection)
			ctx := t.Context()
			cleanup := ""
			switch kind {
			case "callback":
				effect = func(ctx context.Context, s kernel.OrgScope, tx pgx.Tx, e domain.Event) error {
					if err := projection(ctx, s, tx, e); err != nil {
						return err
					}
					return errors.New("PRIVATE-PROVIDER-CONTENT")
				}
			case "receipt":
				role := pgx.Identifier{f.db.RuntimeRole}.Sanitize()
				cleanup = "GRANT INSERT ON org_event_receipts TO " + role
				if _, err := f.db.Admin.Exec(ctx, "REVOKE INSERT ON org_event_receipts FROM "+role); err != nil {
					t.Fatal(err)
				}
			case "commit":
				cleanup = "DROP TRIGGER synthetic_receipt_failure ON org_event_receipts"
				if _, err := f.db.Admin.Exec(ctx, `CREATE FUNCTION synthetic_receipt_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'PRIVATE-COMMIT-CONTENT'; END $$;
				CREATE CONSTRAINT TRIGGER synthetic_receipt_failure AFTER INSERT ON org_event_receipts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_receipt_failure();`); err != nil {
					t.Fatal(err)
				}
			case "lease":
				effect = func(ctx context.Context, s kernel.OrgScope, tx pgx.Tx, e domain.Event) error {
					if err := projection(ctx, s, tx, e); err != nil {
						return err
					}
					_, err := tx.Exec(ctx, "UPDATE job_queue SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", job.ID)
					return err
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := f.handler(t, effect).Handle(ctx, job)
			if kind == "lease" {
				if !errors.Is(err, jobs.ErrStaleLease) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, domain.ErrUnavailable) {
				t.Fatal(err)
			}
			f.state(t, org, 1, 1, 0, 2, 0)
			if cleanup != "" {
				if _, err := f.db.Admin.Exec(t.Context(), cleanup); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.handler(t, projection).Handle(t.Context(), job); err != nil {
				t.Fatal("consumer retry failed", err)
			}
			f.state(t, org, 1, 1, 1, 2, 1)
		})
	}
}

func TestTamperedAndForeignJobCannotSelectAnOrganization(t *testing.T) {
	for _, kind := range []string{"scope", "target", "revision", "consumer", "name", "field"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			org := f.db.Organizations[0]
			f.publish(t, org, "projection.refresh")
			job := f.claim(t)
			env, err := adapters.DecodePayload(job.Payload)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "scope":
				env.OrgID = f.db.Organizations[1].ID
			case "target":
				env.TargetID = f.db.Organizations[1].ParentID
			case "revision":
				env.Revision++
			case "consumer":
				env.Consumer = "review.prepare"
			case "name":
				env.Name = "example.record.deleted"
			}
			job.Payload = env.Payload()
			job.DedupeKey = env.DedupeKey()
			if kind == "field" {
				job.Payload = []byte(strings.TrimSuffix(string(job.Payload), "}") + `,"secret":"PRIVATE"}`)
			}
			if _, err := f.db.Admin.Exec(t.Context(), "UPDATE job_queue SET payload=$2::jsonb,dedupe_key=$3 WHERE id=$1", job.ID, job.Payload, job.DedupeKey); err != nil {
				t.Fatal(err)
			}
			err = f.handler(t, projection).Handle(t.Context(), job)
			if !errors.Is(err, domain.ErrDenied) && !errors.Is(err, domain.ErrInvalid) {
				t.Fatal("tampered intent accepted", err)
			}
			f.state(t, org, 1, 1, 0, 2, 0)
			f.state(t, f.db.Organizations[1], 0, 0, 0, 1, 0)
		})
	}
}

func TestDispatcherRetriesDeadReplayAndPrivateErrors(t *testing.T) {
	f := setup(t)
	org := f.db.Organizations[0]
	f.publish(t, org, "projection.refresh")
	h := f.handler(t, func(context.Context, kernel.OrgScope, pgx.Tx, domain.Event) error {
		return errors.New("PRIVATE-CREDENTIAL-FAILURE")
	})
	d := jobs.Dispatcher{Queue: f.queue, Handlers: map[string]jobs.Handler{h.Kind(): h}, Kind: h.Kind(), WorkerID: "synthetic-dispatcher", RetryDelay: time.Microsecond}
	for range domain.MaxAttempts {
		found, err := d.RunOnce(t.Context())
		if err != nil || !found {
			t.Fatal(found, err)
		}
	}
	var id, status, lastError string
	var attempts int
	if err := f.db.Admin.QueryRow(t.Context(), "SELECT id::text,status,last_error,attempts FROM job_queue").Scan(&id, &status, &lastError, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "dead" || attempts != 5 || lastError != domain.ErrUnavailable.Error() {
		t.Fatal("unbounded or unsafe failure", status, attempts, lastError)
	}
	f.state(t, org, 1, 1, 0, 2, 0)
	found, err := d.RunOnce(t.Context())
	if err != nil || found {
		t.Fatal("dead job reclaimed", err)
	}
	if err := f.queue.ReplayDead(t.Context(), id, "synthetic operator recovery"); err != nil {
		t.Fatal(err)
	}
	good := f.handler(t, projection)
	d.Handlers = map[string]jobs.Handler{good.Kind(): good}
	if found, err := d.RunOnce(t.Context()); err != nil || !found {
		t.Fatal(found, err)
	}
	f.state(t, org, 1, 1, 1, 2, 1)
}

func TestForcedScopeAndAppendOnlyRecords(t *testing.T) {
	f := setup(t)
	org := f.db.Organizations[0]
	f.publish(t, org, "projection.refresh")
	job := f.claim(t)
	if err := f.handler(t, projection).Handle(t.Context(), job); err != nil {
		t.Fatal(err)
	}
	var count int
	role := pgx.Identifier{f.db.RuntimeRole}.Sanitize()
	if _, err := f.db.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_event_intents,org_event_receipts TO "+role); err != nil {
		t.Fatal(err)
	}
	// Each table's own trigger must refuse: the other is disabled while it is
	// checked, and intents are truncated together with receipts so their FK
	// cannot be the failure that satisfies the test.
	others := map[string]string{"org_event_intents": "org_event_receipts", "org_event_receipts": "org_event_intents"}
	for _, table := range []string{"org_event_intents", "org_event_receipts"} {
		if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal("scope leaked", count, err)
		}
		if err := database.WithOrgTx(t.Context(), f.pool, f.db.Organizations[1].Scope(t), func(tx pgx.Tx) error {
			return tx.QueryRow(t.Context(), "SELECT count(*) FROM "+table+" WHERE org_id=$1", org.ID).Scan(&count)
		}); err != nil || count != 0 {
			t.Fatal("foreign rows visible", count, err)
		}
		other := others[table]
		if _, err := f.db.Admin.Exec(t.Context(), "ALTER TABLE "+other+" DISABLE TRIGGER "+other+"_immutable"); err != nil {
			t.Fatal(err)
		}
		for _, statement := range []string{"DELETE FROM " + table + " WHERE false", "UPDATE " + table + " SET consumer=consumer WHERE false", "TRUNCATE org_event_intents, org_event_receipts"} {
			err := database.WithOrgTx(t.Context(), f.pool, org.Scope(t), func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), statement); return err })
			var failure *pgconn.PgError
			if !errors.As(err, &failure) || failure.Code != "55000" || failure.Message != "organization event records are append-only" {
				t.Fatalf("%s: immutable mutation returned %v", statement, err)
			}
		}
		if _, err := f.db.Admin.Exec(t.Context(), "ALTER TABLE "+other+" ENABLE TRIGGER "+other+"_immutable"); err != nil {
			t.Fatal(err)
		}
	}
	ac, err := audit.NewCatalog([]audit.Definition{{Action: "example.record.changed", TargetType: "example.record"}})
	if err != nil {
		t.Fatal(err)
	}
	unsafe := f.service
	unsafe.Audit = auditstore.NewStore(f.db.Admin, ac)
	a, e := f.intent(t, org)
	if _, err := unsafe.Record(t.Context(), org.Scope(t), a, e, []string{"projection.refresh"}, sourceChange); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("privileged producer accepted", err)
	}
	f.state(t, org, 1, 1, 1, 2, 1)
}

func TestExpiredClaimBudgetParksWithoutAnEffect(t *testing.T) {
	f := setup(t)
	org := f.db.Organizations[0]
	f.publish(t, org, "projection.refresh")
	for range domain.MaxAttempts {
		job := f.claim(t)
		if _, err := f.db.Admin.Exec(t.Context(), "UPDATE job_queue SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", job.ID); err != nil {
			t.Fatal(err)
		}
	}
	h := f.handler(t, projection)
	d := jobs.Dispatcher{Queue: f.queue, Handlers: map[string]jobs.Handler{h.Kind(): h}, Kind: h.Kind(), WorkerID: "synthetic-recovery"}
	if found, err := d.RunOnce(t.Context()); err != nil || !found {
		t.Fatal(found, err)
	}
	var status, lastError string
	if err := f.db.Admin.QueryRow(t.Context(), "SELECT status,last_error FROM job_queue").Scan(&status, &lastError); err != nil || status != "dead" || lastError != domain.ErrInvalid.Error() {
		t.Fatal("expired budget executed work", err, status, lastError)
	}
	f.state(t, org, 1, 1, 0, 2, 0)
}
