//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	membershippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestIdentitySQLGuardsRetainPrivateHistory(t *testing.T) {
	f, s, a, _ := fixture(t)
	m := bound(t, s, a, 501, 601, 401)
	for _, table := range []string{"org_employee_identities", "org_memberships"} {
		if _, err := f.Admin.Exec(t.Context(), "DELETE FROM "+pgx.Identifier{table}.Sanitize()); err == nil {
			t.Fatal("history delete succeeded", table)
		}
		if _, err := f.Admin.Exec(t.Context(), "TRUNCATE "+pgx.Identifier{table}.Sanitize()+" CASCADE"); err == nil {
			t.Fatal("history truncate succeeded", table)
		}
	}
	for _, sql := range []string{
		`UPDATE org_memberships SET revision=revision+1 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_memberships SET revision=revision+2 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_memberships SET id='7b0e2a54-0000-4000-8000-000000000900',revision=revision+1 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_memberships SET account_id=NULL,revision=revision+1 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_memberships SET account_tombstone=gen_random_uuid(),revision=revision+1 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_memberships SET access_model='MANAGED',revision=revision+1 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_memberships SET suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`,
	} {
		err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql, a.OrgID().String(), key(601)); return err })
		if err == nil {
			t.Fatal("unsafe update succeeded", sql)
		}
	}
	err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_employee_identities SET state='TERMINATED',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(501))
		return err
	})
	if err == nil {
		t.Fatal("employee termination bypassed membership effects")
	}
	if _, err := f.Runtime.Exec(t.Context(), `SELECT set_config('anpfuel.erasure_account',$1,false)`, key(401)); err != nil {
		t.Fatal(err)
	}
	err = database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET account_id=NULL,state='ENDED',account_tombstone=gen_random_uuid(),tombstoned_at=now(),end_reason='ACCOUNT_DELETED',ended_at=now(),revision=revision+1 WHERE org_id=$1`, a.OrgID().String())
		return err
	})
	if err == nil {
		t.Fatal("runtime impersonated erasure owner")
	}
	if _, err := f.Runtime.Exec(t.Context(), `RESET anpfuel.erasure_account`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(t.Context(), a, key(601))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := m.Snapshot(a)
	after, _ := got.Snapshot(a)
	if before != after {
		t.Fatal("refused SQL changed private value")
	}
	noScope(t, f)
}

func TestAuditedMembershipCompositionIsAtomic(t *testing.T) {
	f, s, a, _ := fixture(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_audit_events TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog([]audit.Definition{{Action: "idn.membership.created", TargetType: "idn.membership"}})
	if err != nil {
		t.Fatal(err)
	}
	runner := auditpg.NewStore(f.Runtime, catalog)
	e, err := d.NewEmployee(a, key(501))
	if err != nil {
		t.Fatal(err)
	}
	m, err := d.NewInvited(a, key(601), e, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	event := audit.Event{OperationID: key(701), ActorID: key(401), TargetID: key(601), Action: "idn.membership.created", TargetType: "idn.membership", RequestHash: strings.Repeat("a", 64)}
	calls := 0
	work := func(ctx context.Context, tx pgx.Tx) error {
		calls++
		if _, err := s.CreateEmployeeTx(ctx, tx, a, e); err != nil {
			return err
		}
		_, err := s.CreateTx(ctx, tx, a, m)
		return err
	}
	for i := range 2 {
		replay, err := runner.RecordChange(t.Context(), a, event, work)
		if err != nil || replay != (i == 1) {
			t.Fatal(replay, err)
		}
	}
	if calls != 1 {
		t.Fatal("audit replay repeated private writes")
	}
	event.OperationID = key(702)
	failure := errors.New("synthetic transaction failure")
	end := ended(t, a, m)
	if _, err := runner.RecordChange(t.Context(), a, event, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := s.SaveTx(ctx, tx, a, 1, end); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	got, err := s.Get(t.Context(), a, key(601))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := got.Snapshot(a)
	if snapshot.State != d.MembershipInvited || snapshot.Revision != 1 {
		t.Fatal("callback failure retained change")
	}
	var events int
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_audit_events WHERE org_id=$1`, a.OrgID().String()).Scan(&events); err != nil || events != 1 {
		t.Fatal("failed callback retained audit", err)
	}
}

func observeLock(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("did not observe runtime lock wait")
			return
		case <-tick.C:
			var waiting bool
			if err := f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock')`, f.RuntimeRole).Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting {
				return
			}
		}
	}
}

func TestMembershipCommitFailureAndLockCancellationReturnNoSuccess(t *testing.T) {
	f, s, a, _ := fixture(t)
	e := employee(t, s, a, 501)
	if _, err := f.Admin.Exec(t.Context(), `CREATE FUNCTION test_member_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic private database detail'; END $$;
 CREATE CONSTRAINT TRIGGER test_member_commit_failure AFTER INSERT ON org_memberships DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION test_member_commit_failure()`); err != nil {
		t.Fatal(err)
	}
	initial, err := d.NewInvited(a, key(601), e, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Create(t.Context(), a, initial); changed || err != app.ErrUnavailable {
		t.Fatal("failed commit acknowledged or leaked cause", changed, err)
	}
	if _, err := s.Get(t.Context(), a, key(601)); err != app.ErrNotFound {
		t.Fatal("failed commit persisted period", err)
	}
	if _, err := f.Admin.Exec(t.Context(), `DROP TRIGGER test_member_commit_failure ON org_memberships`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(t.Context(), a, initial); err != nil {
		t.Fatal(err)
	}
	block, err := f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = block.Rollback(context.Background()) }()
	if _, err := block.Exec(t.Context(), `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, a.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	ref, err := d.PersonalAccount(key(401))
	if err != nil {
		t.Fatal(err)
	}
	next, err := initial.BindAccount(a, 1, ref)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		changed, err := s.Save(ctx, a, 1, next)
		if changed {
			done <- errors.New("cancelled write acknowledged")
		} else {
			done <- err
		}
	}()
	observeLock(t, f)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled write stayed blocked")
	}
	if err := block.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(t.Context(), a, key(601))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := got.Snapshot(a)
	if snapshot.Revision != 1 || snapshot.Account != (d.AccountRef{}) {
		t.Fatal("cancelled write persisted")
	}
	noScope(t, f)
}

func TestBindingLosesToObservedAccountDeletion(t *testing.T) {
	f, s, a, _ := fixture(t)
	m := invited(t, s, a, 501, 601)
	block, err := f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = block.Rollback(context.Background()) }()
	if _, err := block.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, key(401)); err != nil {
		t.Fatal(err)
	}
	ref, err := d.PersonalAccount(key(401))
	if err != nil {
		t.Fatal(err)
	}
	next, err := m.BindAccount(a, 1, ref)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Save(t.Context(), a, 1, next); done <- err }()
	observeLock(t, f)
	if err := block.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != d.ErrInvalid {
			t.Fatal("deleted account bound", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("binding stayed blocked")
	}
	got, err := s.Get(t.Context(), a, key(601))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := got.Snapshot(a)
	if snapshot.Account != (d.AccountRef{}) || snapshot.Revision != 1 {
		t.Fatal("deletion race retained access")
	}
}

func TestTwoOwnersCannotEndConcurrently(t *testing.T) {
	f, s, a, _ := fixture(t)
	policy(t, f)
	values := []d.Membership{activate(t, f, s, a, bound(t, s, a, 501, 601, 401), "OWNER"), activate(t, f, s, a, bound(t, s, a, 502, 602, 402), "OWNER")}
	store := membershippg.NewStore(f.RuntimePool(t, 4))
	results := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, value := range values {
		next := ended(t, a, value)
		wg.Go(func() { <-start; _, err := store.Save(t.Context(), a, 3, next); results <- err })
	}
	close(start)
	wg.Wait()
	close(results)
	allowed, protected := 0, 0
	for err := range results {
		switch err {
		case nil:
			allowed++
		case kernel.ErrLastOrgOwner:
			protected++
		default:
			t.Fatal(err)
		}
	}
	if allowed != 1 || protected != 1 {
		t.Fatal("owner race", allowed, protected)
	}
}
