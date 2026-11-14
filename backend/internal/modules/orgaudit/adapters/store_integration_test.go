//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func newAuditFixture(t *testing.T) (*orgtest.Fixture, *adapters.Store) {
	t.Helper()
	f := orgtest.New(t)
	raw, err := os.ReadFile("../../../../db/templates/organization_private.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), string(raw)); err != nil {
		t.Fatal(err)
	}
	for _, org := range f.Organizations {
		if _, err := f.Admin.Exec(t.Context(), `INSERT INTO org_example_parent (org_id, id, number) VALUES ($1,$2,$3)`,
			org.ID, org.ParentID, org.Number); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Admin.Exec(t.Context(), `INSERT INTO org_example_record (org_id, id, parent_id, number) VALUES ($1,$2,$3,$4)`,
			org.ID, org.RecordID, org.ParentID, org.Number); err != nil {
			t.Fatal(err)
		}
	}
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+role+
		"; GRANT SELECT, INSERT ON org_audit_events TO "+role+
		"; GRANT SELECT, INSERT, UPDATE ON org_example_record TO "+role); err != nil {
		t.Fatal(err)
	}
	return f, adapters.NewStore(f.Runtime, auditCatalog(t))
}

func changeRevision(ctx context.Context, tx pgx.Tx) error {
	tag, err := tx.Exec(ctx, `UPDATE org_example_record SET revision=revision+1
		WHERE org_id = current_setting('anpfuel.org_id')::uuid AND id=$1`, auditEvent().TargetID)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("synthetic change did not affect one scoped row")
	}
	return err
}

func assertAuditState(t *testing.T, f *orgtest.Fixture, org orgtest.Organization, events, revision int) {
	t.Helper()
	var actualEvents, actualRevision int
	err := f.Admin.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM org_audit_events WHERE org_id=$1),
		(SELECT revision FROM org_example_record WHERE org_id=$1)`, org.ID).Scan(&actualEvents, &actualRevision)
	if err != nil || actualEvents != events || actualRevision != revision {
		t.Fatalf("audit state events=%d revision=%d err=%v, want %d/%d", actualEvents, actualRevision, err, events, revision)
	}
}

func TestAuditedChangesAreAtomicAndReplaySafe(t *testing.T) {
	f, store := newAuditFixture(t)
	a, b := f.Organizations[0], f.Organizations[1]
	scope := a.Scope(t)
	event := auditEvent()
	event.DelegationID = "7b0e2a54-0000-4000-8000-000000000013"
	calls := 0
	work := func(ctx context.Context, tx pgx.Tx) error { calls++; return changeRevision(ctx, tx) }
	for attempt := range 2 {
		replayed, err := store.RecordChange(t.Context(), scope, event, work)
		if err != nil || replayed != (attempt == 1) {
			t.Fatalf("attempt %d replay=%v err=%v", attempt, replayed, err)
		}
	}
	if calls != 1 {
		t.Fatal("replay repeated the business change")
	}
	assertAuditState(t, f, a, 1, 2)
	assertAuditState(t, f, b, 0, 1)
	rows, err := store.List(t.Context(), scope, domain.Filter{})
	if err != nil || len(rows) != 1 || rows[0].Event != event || rows[0].At.IsZero() {
		t.Fatalf("actor/delegation/event round trip failed: %v", err)
	}
	changedDigest, changedActor := event, event
	changedDigest.RequestHash = strings.Repeat("b", 64)
	changedActor.ActorID = event.TargetID
	for _, changed := range []domain.Event{changedDigest, changedActor} {
		if _, err := store.RecordChange(t.Context(), scope, changed, work); !errors.Is(err, domain.ErrReused) {
			t.Fatalf("changed replay returned %v", err)
		}
	}
	if calls != 1 {
		t.Fatal("conflicting operation reached the callback")
	}
	assertAuditState(t, f, a, 1, 2)

	t.Run("failed business change rolls back both and permits retry", func(t *testing.T) {
		next := event
		next.OperationID = "7b0e2a54-0000-4000-8000-000000000020"
		failure := errors.New("synthetic business failure")
		_, err := store.RecordChange(t.Context(), scope, next, func(ctx context.Context, tx pgx.Tx) error {
			if err := changeRevision(ctx, tx); err != nil {
				return err
			}
			return failure
		})
		if !errors.Is(err, failure) {
			t.Fatalf("business failure lost: %v", err)
		}
		assertAuditState(t, f, a, 1, 2)
		replayed, err := store.RecordChange(t.Context(), scope, next, changeRevision)
		if err != nil || replayed {
			t.Fatalf("failed operation could not retry: %v", err)
		}
		assertAuditState(t, f, a, 2, 3)
	})

	t.Run("audit failure prevents unaudited change", func(t *testing.T) {
		role := pgx.Identifier{f.RuntimeRole}.Sanitize()
		if _, err := f.Admin.Exec(t.Context(), "REVOKE INSERT ON org_audit_events FROM "+role); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := f.Admin.Exec(context.Background(), "GRANT INSERT ON org_audit_events TO "+role); err != nil {
				t.Error(err)
			}
		})
		next := event
		next.OperationID = "7b0e2a54-0000-4000-8000-000000000030"
		_, err := store.RecordChange(t.Context(), scope, next, func(context.Context, pgx.Tx) error {
			t.Error("audit failure reached business callback")
			return nil
		})
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("audit failure returned %v", err)
		}
		assertAuditState(t, f, a, 2, 3)
	})

	t.Run("cancellation rolls back both", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		next := event
		next.OperationID = "7b0e2a54-0000-4000-8000-000000000040"
		_, err := store.RecordChange(ctx, scope, next, func(ctx context.Context, tx pgx.Tx) error {
			if err := changeRevision(ctx, tx); err != nil {
				return err
			}
			cancel()
			return ctx.Err()
		})
		if err == nil {
			t.Fatal("cancelled change succeeded")
		}
		assertAuditState(t, f, a, 2, 3)
		if _, err := store.List(t.Context(), scope, domain.Filter{}); err != nil {
			t.Fatal("cancelled work retained the pool")
		}
	})

	t.Run("deferred commit failure rolls back both", func(t *testing.T) {
		ctx := t.Context()
		_, err := f.Admin.Exec(ctx, `CREATE FUNCTION test_audit_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'synthetic deferred failure'; END $$;
			CREATE CONSTRAINT TRIGGER test_audit_commit_failure AFTER UPDATE ON org_example_record
			DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION test_audit_commit_failure()`)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := f.Admin.Exec(cleanupCtx, `DROP TRIGGER test_audit_commit_failure ON org_example_record;
				DROP FUNCTION test_audit_commit_failure()`); err != nil {
				t.Error(err)
			}
		})
		next := event
		next.OperationID = "7b0e2a54-0000-4000-8000-000000000050"
		if replayed, err := store.RecordChange(ctx, scope, next, changeRevision); replayed || !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("deferred commit failure returned replay=%v err=%v", replayed, err)
		}
		assertAuditState(t, f, a, 2, 3)
	})
}

type auditReadPolicy struct {
	orgID   string
	allowed bool
}

func (policy *auditReadPolicy) AuthorizeAuditRead(_ context.Context, scope kernel.OrgScope, _ application.ReadAccess) error {
	if !policy.allowed || scope.OrgID().String() != policy.orgID {
		return domain.ErrDenied
	}
	return nil
}

// isolateAuditTruncateProbe follows the actual CASCADE closure, including newer
// command receipts. Only this disposable role gets those TRUNCATE grants, and
// only dependent TRUNCATE triggers are suspended: the audit guard stays active.
func isolateAuditTruncateProbe(t *testing.T, f *orgtest.Fixture) func() {
	t.Helper()
	rows, err := f.Admin.Query(t.Context(), `WITH RECURSIVE dependents(oid) AS (
 SELECT 'public.org_audit_events'::regclass::oid
 UNION
 SELECT c.conrelid FROM pg_constraint c JOIN dependents d ON c.confrelid=d.oid WHERE c.contype='f'
) SELECT n.nspname,c.relname,COALESCE(t.tgname,''),COALESCE(t.tgenabled::text,'')
 FROM dependents d JOIN pg_class c ON c.oid=d.oid JOIN pg_namespace n ON n.oid=c.relnamespace
 LEFT JOIN pg_trigger t ON t.tgrelid=c.oid AND NOT t.tgisinternal AND(t.tgtype&32)=32 AND t.tgenabled<>'D'
 WHERE c.oid<>'public.org_audit_events'::regclass ORDER BY n.nspname,c.relname,t.tgname`)
	if err != nil {
		t.Fatal(err)
	}
	type savedTrigger struct{ table, name, enabled string }
	var triggers []savedTrigger
	var tables []string
	last := ""
	for rows.Next() {
		var schema, table, name, enabled string
		if err := rows.Scan(&schema, &table, &name, &enabled); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		qualified := pgx.Identifier{schema, table}.Sanitize()
		if qualified != last {
			tables = append(tables, qualified)
			last = qualified
		}
		if name != "" {
			triggers = append(triggers, savedTrigger{qualified, pgx.Identifier{name}.Sanitize(), enabled})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(tables) == 0 {
		t.Fatal("audit CASCADE closure unexpectedly empty")
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT TRUNCATE ON "+strings.Join(tables, ",")+" TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, trigger := range triggers {
			mode := "ENABLE"
			switch trigger.enabled {
			case "O":
			case "A":
				mode = "ENABLE ALWAYS"
			case "R":
				mode = "ENABLE REPLICA"
			default:
				t.Fatalf("unknown saved trigger state %q", trigger.enabled)
			}
			if _, err := f.Admin.Exec(ctx, "ALTER TABLE "+trigger.table+" "+mode+" TRIGGER "+trigger.name); err != nil {
				t.Error("restore dependent truncate trigger", err)
			}
		}
		restored = true
	}
	t.Cleanup(restore)
	for _, trigger := range triggers {
		if _, err := f.Admin.Exec(t.Context(), "ALTER TABLE "+trigger.table+" DISABLE TRIGGER "+trigger.name); err != nil {
			t.Fatal(err)
		}
	}
	return restore
}

func TestAuditReadIsolationAndImmutability(t *testing.T) {
	f, store := newAuditFixture(t)
	a, b := f.Organizations[0], f.Organizations[1]
	event := auditEvent()
	for _, org := range f.Organizations {
		if _, err := store.RecordChange(t.Context(), org.Scope(t), event, changeRevision); err != nil {
			t.Fatal(err)
		}
	}
	policy := &auditReadPolicy{orgID: a.ID, allowed: true}
	service := application.Service{Reader: store, Authorizer: policy}
	access := application.ReadAccess{ActorID: event.ActorID}
	rows, err := service.List(t.Context(), a.Scope(t), access, domain.Filter{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("authorized read failed: %v", err)
	}
	if _, err := service.List(t.Context(), b.Scope(t), access, domain.Filter{}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("foreign read was allowed")
	}
	policy.allowed = false
	if _, err := service.List(t.Context(), a.Scope(t), access, domain.Filter{}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("revoked read was allowed")
	}
	var count int
	if err := f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_audit_events`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unscoped audit read exposed rows")
	}
	err = database.WithOrgTx(t.Context(), f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_audit_events WHERE org_id=$1`, b.ID).Scan(&count)
	})
	if err != nil || count != 0 {
		t.Fatal("RLS exposed foreign audit rows")
	}
	for _, reason := range []string{"Bearer synthetic", `{"document":"synthetic"}`} {
		err := database.WithOrgTx(t.Context(), f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `INSERT INTO org_audit_events
				(org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash)
				VALUES ($1,$2,$3,'org.settings.changed','org.settings',$4,$5,$6)`,
				a.ID, event.TargetID, event.ActorID, event.TargetID, reason, event.RequestHash)
			return err
		})
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "23514" {
			t.Fatalf("audit payload was not rejected: %v", err)
		}
	}
	// Even an accidental broad DML grant cannot mutate or truncate the trail.
	if _, err := f.Admin.Exec(t.Context(), "GRANT ALL ON org_audit_events TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	restore := isolateAuditTruncateProbe(t, f)
	for _, query := range []string{`UPDATE org_audit_events SET reason_code='correction'`, `DELETE FROM org_audit_events`, `TRUNCATE org_audit_events CASCADE`} {
		err := database.WithOrgTx(t.Context(), f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), query)
			return err
		})
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "55000" || failure.Message != "organization audit events are append-only" {
			t.Fatalf("immutable trail query returned %v", err)
		}
	}
	restore()
	err = database.WithOrgTx(t.Context(), f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_audit_events
			(org_id,operation_id,actor_id,action,target_type,target_id,request_hash)
			VALUES ($1,$2,$3,'org.settings.changed','org.settings',$4,$5)`,
			b.ID, event.TargetID, event.ActorID, event.TargetID, event.RequestHash)
		return err
	})
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "42501" {
		t.Fatalf("foreign insert returned %v", err)
	}
	assertAuditState(t, f, a, 1, 2)
	assertAuditState(t, f, b, 1, 2)
}

func TestAuditConcurrentReplayAndOrganizationScopes(t *testing.T) {
	f, _ := newAuditFixture(t)
	pool := f.RuntimePool(t, 4)
	store := adapters.NewStore(pool, auditCatalog(t))
	a := f.Organizations[0]
	scope := a.Scope(t)
	event := auditEvent()
	var calls, replays atomic.Int32
	start := make(chan struct{})
	results := make(chan error, 12)
	for range 12 {
		go func() {
			<-start
			replayed, err := store.RecordChange(t.Context(), scope, event, func(ctx context.Context, tx pgx.Tx) error {
				calls.Add(1)
				return changeRevision(ctx, tx)
			})
			if replayed {
				replays.Add(1)
			}
			results <- err
		}()
	}
	close(start)
	for range 12 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 || replays.Load() != 11 {
		t.Fatal("concurrent replay repeated the business change or event")
	}
	assertAuditState(t, f, a, 1, 2)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ready, release := make(chan struct{}, 2), make(chan struct{})
	results = make(chan error, 2)
	next := event
	next.OperationID = "7b0e2a54-0000-4000-8000-000000000020"
	for _, org := range f.Organizations {
		scope := org.Scope(t)
		go func() {
			_, err := store.RecordChange(ctx, scope, next, func(ctx context.Context, tx pgx.Tx) error {
				ready <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
				return changeRevision(ctx, tx)
			})
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case err := <-results:
			t.Fatalf("scoped transaction stopped before barrier: %v", err)
		case <-ctx.Done():
			t.Fatal("organizations shared an operation lock")
		}
	}
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	assertAuditState(t, f, a, 2, 3)
	assertAuditState(t, f, f.Organizations[1], 1, 2)
}

func TestAuditKeysetPaginationHandlesTiedTimes(t *testing.T) {
	f, store := newAuditFixture(t)
	a := f.Organizations[0]
	stamp := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	event := auditEvent()
	for index := 1; index <= 3; index++ {
		id := fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", index)
		if _, err := f.Admin.Exec(t.Context(), `INSERT INTO org_audit_events
			(org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash,recorded_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, a.ID, id, event.ActorID, event.Action, event.TargetType,
			event.TargetID, event.ReasonCode, event.RequestHash, stamp); err != nil {
			t.Fatal(err)
		}
	}
	filter := domain.Filter{Limit: 1}
	for index := 3; index >= 1; index-- {
		rows, err := store.List(t.Context(), a.Scope(t), filter)
		if err != nil || len(rows) != 1 || rows[0].Event.OperationID != fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", index) {
			t.Fatalf("tied-time page lost its ordering: %v", err)
		}
		filter.BeforeAt, filter.BeforeOperationID = rows[0].At, rows[0].Event.OperationID
	}
	if rows, err := store.List(t.Context(), a.Scope(t), filter); err != nil || len(rows) != 0 {
		t.Fatal("pagination repeated a row")
	}
}
