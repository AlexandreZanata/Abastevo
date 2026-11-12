//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	auditdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func rootFixture(t *testing.T) (*orgtest.Fixture, *adapters.Store, kernel.OrgScope, kernel.OrgScope) {
	t.Helper()
	f := orgtest.New(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT,UPDATE ON organizations TO "+role); err != nil {
		t.Fatal(err)
	}
	// Root closure invokes the installed ownership safety reduction even when
	// this historical fixture has never admitted an ownership proposal.
	f.GrantOwnershipLifecycle(t)
	return f, adapters.NewStore(f.Runtime), scopeFor(t, "7b0e2a54-0000-4000-8000-000000000101"), scopeFor(t, "7b0e2a54-0000-4000-8000-000000000102")
}
func assertNoScope(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	var scope string
	if err := f.Runtime.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')`).Scan(&scope); err != nil || scope != "" {
		t.Fatal("pooled connection retained private scope", err)
	}
}
func TestRootRoundTripReplayIsolationAndLegacyRefusal(t *testing.T) {
	f, s, a, b := rootFixture(t)
	ctx := t.Context()
	value := pending(t, a, "Same synthetic name")
	if _, err := s.Get(ctx, a); err != app.ErrNotFound {
		t.Fatal(err)
	}
	for i := range 2 {
		changed, err := s.Create(ctx, a, value)
		if err != nil || changed != (i == 0) {
			t.Fatal("create/replay", changed, err)
		}
	}
	if changed, err := s.Create(ctx, a, pending(t, a, "Changed intent")); changed || err != domain.ErrConflict {
		t.Fatal(changed, err)
	}
	if changed, err := s.Create(ctx, b, pending(t, b, "Same synthetic name")); !changed || err != nil {
		t.Fatal("name leaked another root", changed, err)
	}
	got, err := s.Get(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := got.Snapshot(a)
	if err != nil || snap.Profile.Name != "Same synthetic name" || snap.Profile.TimeZone != "America/Cuiaba" || fmt.Sprint(snap.Profile.Kinds) != "[CARRIER OTHER]" || snap.Revision != 1 || snap.State != domain.PendingSetup {
		t.Fatal("explicit persistence mapping failed", err)
	}
	if _, err := got.Snapshot(b); err != domain.ErrScope {
		t.Fatal(err)
	}
	next := configured(t, a, value, "Second")
	for i := range 2 {
		changed, err := s.Save(ctx, a, 1, next)
		if err != nil || changed != (i == 0) {
			t.Fatal("CAS/replay", changed, err)
		}
	}
	if changed, err := s.Save(ctx, a, 1, configured(t, a, value, "Competitor")); changed || err != domain.ErrConflict {
		t.Fatal(changed, err)
	}
	legacy := f.Organizations[0].Scope(t)
	if _, err := s.Get(ctx, legacy); err != app.ErrUninitialized {
		t.Fatal("legacy fabricated a snapshot", err)
	}
	initial := pending(t, legacy, "Do not adopt")
	if changed, err := s.Create(ctx, legacy, initial); changed || err != app.ErrUninitialized {
		t.Fatal(changed, err)
	}
	if changed, err := s.Save(ctx, legacy, 1, configured(t, legacy, initial, "Do not adopt")); changed || err != app.ErrUninitialized {
		t.Fatal(changed, err)
	}

	// Exercise the actual ordinary role, not only scoped repository predicates.
	err = database.WithOrgTx(ctx, f.Runtime, a, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE org_id=$1`, b.OrgID().String()).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("RLS exposed another root")
		}
		tag, err := tx.Exec(ctx, `UPDATE organizations SET revision=revision+1 WHERE org_id=$1`, b.OrgID().String())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			return errors.New("RLS updated another root")
		}
		if _, err := s.GetTx(ctx, tx, b); err != domain.ErrScope {
			return errors.New("transaction accepted mismatched scope")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithOrgTx(ctx, f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO organizations(org_id) VALUES($1)`, "7b0e2a54-0000-4000-8000-000000000103")
		return err
	})
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "42501" {
		t.Fatal("foreign insert not denied", err)
	}
	assertNoScope(t, f)
	if _, err := adapters.NewStore(f.Admin).Get(ctx, a); err != app.ErrUnavailable {
		t.Fatal("privileged runtime accepted", err)
	}
	tx, err := f.Runtime.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTx(ctx, tx, a); err != domain.ErrScope {
		t.Fatal("missing transaction scope accepted", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	adminTx, err := f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTx(ctx, adminTx, a); err != app.ErrUnavailable {
		t.Fatal("privileged composition accepted", err)
	}
	if err := adminTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	assertNoScope(t, f)
}

func TestRootConcurrentCreateAndCompareAndSwap(t *testing.T) {
	f, s, a, _ := rootFixture(t)
	pool := f.RuntimePool(t, 12)
	s = adapters.NewStore(pool)
	initial := pending(t, a, "Concurrent synthetic root")
	run := func(work func(int) (bool, error)) int {
		t.Helper()
		type result struct {
			changed bool
			err     error
		}
		results := make(chan result, 12)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range 12 {
			wg.Go(func() { <-start; changed, err := work(i); results <- result{changed, err} })
		}
		close(start)
		wg.Wait()
		close(results)
		wins := 0
		for r := range results {
			if r.err != nil && !errors.Is(r.err, domain.ErrConflict) {
				t.Fatal(r.err)
			}
			if r.changed {
				wins++
			}
		}
		return wins
	}
	if wins := run(func(int) (bool, error) { return s.Create(t.Context(), a, initial) }); wins != 1 {
		t.Fatal("create winners", wins)
	}
	updates := make([]domain.Organization, 12)
	for i := range updates {
		updates[i] = configured(t, a, initial, fmt.Sprintf("Synthetic update %d", i))
	}
	if wins := run(func(i int) (bool, error) { return s.Save(t.Context(), a, 1, updates[i]) }); wins != 1 {
		t.Fatal("CAS winners", wins)
	}
	got, err := s.Get(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := got.Snapshot(a)
	if err != nil || snap.Revision != 2 {
		t.Fatal("concurrency revision", err)
	}
}

func TestRootTerminalStateAndSQLGuards(t *testing.T) {
	f, s, a, _ := rootFixture(t)
	ctx := t.Context()
	value := pending(t, a, "Closing synthetic root")
	if _, err := s.Create(ctx, a, value); err != nil {
		t.Fatal(err)
	}
	closed, err := value.Transition(a, 1, domain.Closed, domain.Conditions{NoOperationalRecords: true})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Save(ctx, a, 1, closed); !changed || err != nil {
		t.Fatal(changed, err)
	}
	if changed, err := s.Save(ctx, a, 1, closed); changed || err != nil {
		t.Fatal("closure replay", changed, err)
	}
	if changed, err := s.Save(ctx, a, 1, configured(t, a, value, "reopen")); changed || err != domain.ErrClosed {
		t.Fatal(changed, err)
	}
	for _, query := range []string{
		`UPDATE organizations SET state='PENDING_SETUP',revision=revision+1 WHERE org_id=$1`,
		`UPDATE organizations SET org_id='7b0e2a54-0000-4000-8000-000000000111',revision=revision+1 WHERE org_id=$1`,
		`UPDATE organizations SET revision=revision+2 WHERE org_id=$1`,
		`DELETE FROM organizations WHERE org_id=$1`,
	} {
		if _, err := f.Admin.Exec(ctx, query, a.OrgID().String()); err == nil {
			t.Fatal("unsafe SQL succeeded", query)
		}
	}
	if _, err := f.Admin.Exec(ctx, `TRUNCATE organizations CASCADE`); err == nil {
		t.Fatal("identity truncate succeeded")
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO organizations(org_id,has_snapshot,display_name,time_zone,kind_tags,state,suspension_reason) VALUES('7b0e2a54-0000-4000-8000-000000000112',true,'Synthetic','UTC','{}','ACTIVE','')`); err == nil {
		t.Fatal("active initial insert succeeded")
	}
	var constraints int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname IN ('org_audit_events_root_fk','org_capabilities_root_fk','org_event_intents_root_fk','org_event_receipts_root_fk') AND convalidated AND condeferrable AND condeferred`).Scan(&constraints); err != nil || constraints != 4 {
		t.Fatal("root constraints incomplete", constraints, err)
	}
	// The FK is checked at commit, allowing the audit reservation to precede creation.
	tx, err := f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO org_capabilities(org_id,capability,enabled,revision) VALUES('7b0e2a54-0000-4000-8000-000000000113','synthetic',false,1)`)
	if err != nil {
		t.Fatal(err)
	}
	var constraintError *pgconn.PgError
	if err := tx.Commit(ctx); !errors.As(err, &constraintError) || constraintError.Code != "23503" {
		t.Fatal("orphan root committed", err)
	}
}

func TestRootIncompleteSetupAndDatabaseShapeConstraints(t *testing.T) {
	f, s, a, b := rootFixture(t)
	value, err := domain.NewPending(a, domain.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Create(t.Context(), a, value); !changed || err != nil {
		t.Fatal(changed, err)
	}
	got, err := s.Get(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := got.Snapshot(a)
	if err != nil || snap.State != domain.PendingSetup || snap.Profile.Name != "" || snap.Profile.TimeZone != "" || len(snap.Profile.Kinds) != 0 {
		t.Fatal("incomplete setup fabricated defaults", err)
	}
	for _, query := range []string{
		`UPDATE organizations SET state='SUSPENDED',suspension_reason='SECURITY',revision=revision+1 WHERE org_id=$1`,
		`UPDATE organizations SET kind_tags=ARRAY['OTHER','CARRIER'],revision=revision+1 WHERE org_id=$1`,
		`UPDATE organizations SET kind_tags=ARRAY['OTHER','OTHER'],revision=revision+1 WHERE org_id=$1`,
		`UPDATE organizations SET kind_tags=ARRAY[NULL]::text[],revision=revision+1 WHERE org_id=$1`,
		`UPDATE organizations SET display_name=repeat('x',121),revision=revision+1 WHERE org_id=$1`,
		`UPDATE organizations SET display_name=E'line\ncontrol',revision=revision+1 WHERE org_id=$1`,
		`UPDATE organizations SET has_snapshot=false,revision=revision+1 WHERE org_id=$1`,
	} {
		if _, err := f.Admin.Exec(t.Context(), query, a.OrgID().String()); err == nil {
			t.Fatal("invalid snapshot SQL succeeded", query)
		}
	}
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO organizations(org_id,revision) VALUES($1,2)`, b.OrgID().String()); err == nil {
		t.Fatal("invalid initial revision succeeded")
	}
}

func TestRootAuditedCompositionRollbackAndCommitFailure(t *testing.T) {
	f, s, a, b := rootFixture(t)
	ctx := t.Context()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT,INSERT ON org_audit_events TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := auditdomain.NewCatalog([]auditdomain.Definition{{Action: "org.created", TargetType: "org.root"}})
	if err != nil {
		t.Fatal(err)
	}
	runner := audit.NewStore(f.Runtime, catalog)
	event := auditdomain.Event{OperationID: "7b0e2a54-0000-4000-8000-000000000120", ActorID: "7b0e2a54-0000-4000-8000-000000000121", TargetID: a.OrgID().String(), Action: "org.created", TargetType: "org.root", RequestHash: strings.Repeat("a", 64)}
	value := pending(t, a, "Audited synthetic creation")
	calls := 0
	work := func(ctx context.Context, tx pgx.Tx) error {
		calls++
		_, err := s.CreateTx(ctx, tx, a, value)
		return err
	}
	for i := range 2 {
		replay, err := runner.RecordChange(ctx, a, event, work)
		if err != nil || replay != (i == 1) {
			t.Fatal("audit composition", replay, err)
		}
	}
	if calls != 1 {
		t.Fatal("audit replay repeated root creation")
	}
	event.OperationID = "7b0e2a54-0000-4000-8000-000000000122"
	event.TargetID = b.OrgID().String()
	rollback := errors.New("synthetic command rollback")
	other := pending(t, b, "Rollback synthetic creation")
	if _, err := runner.RecordChange(ctx, b, event, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := s.CreateTx(ctx, tx, b, other); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, b); err != app.ErrNotFound {
		t.Fatal("failed audit retained root", err)
	}
	var events int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1`, b.OrgID().String()).Scan(&events); err != nil || events != 0 {
		t.Fatal("failed root retained audit", err)
	}
	if _, err := f.Admin.Exec(ctx, `CREATE FUNCTION synthetic_root_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic private database detail'; END $$;
 CREATE CONSTRAINT TRIGGER synthetic_root_commit_failure AFTER INSERT ON organizations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_root_commit_failure()`); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Create(ctx, b, other); changed || err != app.ErrUnavailable {
		t.Fatal("commit failure reported success or leaked cause", changed, err)
	}
	if _, err := s.Get(ctx, b); err != app.ErrNotFound {
		t.Fatal("failed commit retained root", err)
	}
	assertNoScope(t, f)
}

func TestRootCancellationUnderObservedLockAndCorruptMetadata(t *testing.T) {
	f, s, a, _ := rootFixture(t)
	initial := pending(t, a, "Locked synthetic root")
	if _, err := s.Create(t.Context(), a, initial); err != nil {
		t.Fatal(err)
	}
	blocker, err := f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	if _, err := blocker.Exec(t.Context(), `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, a.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	next := configured(t, a, initial, "Cancelled update")
	type result struct {
		changed bool
		err     error
	}
	done := make(chan result, 1)
	go func() { changed, err := s.Save(ctx, a, 1, next); done <- result{changed, err} }()
	observed := false
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !observed {
		select {
		case r := <-done:
			t.Fatal("write did not wait for lock", r.err)
		case <-deadline.C:
			t.Fatal("did not observe a blocked root write")
		case <-ticker.C:
			if err := f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock' AND query LIKE '%%UPDATE organizations%%')`, f.RuntimeRole).Scan(&observed); err != nil {
				t.Fatal(err)
			}
		}
	}
	cancel()
	select {
	case r := <-done:
		if r.changed || !errors.Is(r.err, context.Canceled) {
			t.Fatal(r.changed, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled write remained blocked")
	}
	if err := blocker.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := got.Snapshot(a)
	if err != nil || snap.Revision != 1 {
		t.Fatal("cancelled update persisted", err)
	}
	if _, err := f.Admin.Exec(t.Context(), `UPDATE organizations SET time_zone='Synthetic/Private',revision=revision+1 WHERE org_id=$1`, a.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(t.Context(), a); err != app.ErrUnavailable {
		t.Fatal("corrupt metadata escaped domain validation", err)
	}
	assertNoScope(t, f)
}
