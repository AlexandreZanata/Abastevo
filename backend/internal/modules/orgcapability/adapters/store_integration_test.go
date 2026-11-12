//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditstore "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

// The concrete membership/operator schema belongs to AUTH. This fixture supplies
// that narrow policy port; all capability, audit and business SQL is real.
type authority struct {
	mu       sync.Mutex
	denied   bool
	operator bool
	calls    int
}

func (a *authority) Authorize(_ context.Context, _ kernel.OrgScope, _ domain.Actor, _ string, intent domain.Intent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	if a.denied || (intent == domain.Manage && !a.operator) {
		return domain.ErrDenied
	}
	return nil
}
func actor() domain.Actor {
	return domain.Actor{AccountID: operation(100), DelegationID: operation(101)}
}
func operation(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012x", n) }
func change(n int, enabled bool, revision int64) domain.Change {
	return domain.Change{OperationID: operation(n), Capability: "fuelings", Enabled: enabled, ExpectedRevision: revision}
}
func fixture(t *testing.T) (*orgtest.Fixture, *adapters.Store, application.Service, *authority) {
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
		if _, err := f.Admin.Exec(t.Context(), `INSERT INTO org_example_parent(org_id,id,number) VALUES($1,$2,$3);
`, org.ID, org.ParentID, org.Number); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Admin.Exec(t.Context(), `INSERT INTO org_example_record(org_id,id,parent_id,number) VALUES($1,$2,$3,$4)`, org.ID, org.RecordID, org.ParentID, org.Number); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT, INSERT ON org_audit_events TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()+
		"; GRANT SELECT,INSERT,UPDATE ON org_capabilities TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()+
		"; GRANT SELECT,UPDATE ON org_example_record TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	catalog, err := domain.NewCatalog([]string{"fuelings", "expenses"})
	if err != nil {
		t.Fatal(err)
	}
	definitions := append(adapters.AuditDefinitions(), audit.Definition{Action: "fixture.changed", TargetType: "record"})
	auditCatalog, err := audit.NewCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}
	// Separate connections are needed for the lock-order tests.
	pool := f.RuntimePool(t, 8)
	store := adapters.NewStore(pool, catalog, auditstore.NewStore(pool, auditCatalog))
	a := &authority{operator: true}
	return f, store, application.Service{Enabled: true, Catalog: catalog, Authorizer: a, Repository: store}, a
}
func mutate(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `UPDATE org_example_record SET revision=revision+1`)
	return err
}
func check(t *testing.T, f *orgtest.Fixture, index int, enabled bool, revision, events, business int64) {
	t.Helper()
	org := f.Organizations[index]
	var actualEnabled bool
	var actualRevision, actualEvents, actualBusiness int64
	err := f.Admin.QueryRow(t.Context(), `SELECT
		COALESCE((SELECT enabled FROM org_capabilities WHERE org_id=$1 AND capability='fuelings'),false),
		COALESCE((SELECT revision FROM org_capabilities WHERE org_id=$1 AND capability='fuelings'),0),
		(SELECT count(*) FROM org_audit_events WHERE org_id=$1),
		(SELECT revision FROM org_example_record WHERE org_id=$1)`, org.ID).Scan(&actualEnabled, &actualRevision, &actualEvents, &actualBusiness)
	if err != nil || actualEnabled != enabled || actualRevision != revision || actualEvents != events || actualBusiness != business {
		t.Fatalf("state=%v/%d audit=%d business=%d err=%v; want %v/%d/%d/%d", actualEnabled, actualRevision, actualEvents, actualBusiness, err, enabled, revision, events, business)
	}
}

func TestCapabilityDisableReenableAndGlobalRollback(t *testing.T) {
	f, _, service, policy := fixture(t)
	ctx := t.Context()
	a, b := f.Organizations[0].Scope(t), f.Organizations[1].Scope(t)
	for _, scope := range []kernel.OrgScope{a, b} {
		state, err := service.State(ctx, scope, actor(), "fuelings")
		if err != nil || state != (domain.State{}) {
			t.Fatal(state, err)
		}
		if err := service.Run(ctx, scope, actor(), "fuelings", domain.Write, mutate); !errors.Is(err, domain.ErrEntitlementRequired) {
			t.Fatal("missing flag opened write", err)
		}
		if _, err := service.Set(ctx, scope, actor(), change(1, true, 0)); err != nil {
			t.Fatal(err)
		}
	}
	staleClientState, err := service.State(ctx, a, actor(), "fuelings")
	if err != nil || !staleClientState.Enabled {
		t.Fatal(err)
	}
	if _, err := service.Set(ctx, a, actor(), change(2, false, 1)); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(ctx, a, actor(), "fuelings", domain.Write, mutate); !errors.Is(err, domain.ErrEntitlementRequired) {
		t.Fatal("stale client bypassed disable", err)
	}
	for _, intent := range []domain.Intent{domain.Read, domain.Export} {
		if err := service.Run(ctx, a, actor(), "fuelings", intent, func(ctx context.Context, tx pgx.Tx) error {
			var count int
			err := tx.QueryRow(ctx, `SELECT count(*) FROM org_example_record`).Scan(&count)
			if count != 1 {
				return fmt.Errorf("expected preserved private record, got %d", count)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.Run(ctx, b, actor(), "fuelings", domain.Write, mutate); err != nil {
		t.Fatal("A's disable affected B", err)
	}
	if _, err := service.Set(ctx, a, actor(), change(3, true, 2)); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(ctx, a, actor(), "fuelings", domain.Write, mutate); err != nil {
		t.Fatal(err)
	}
	service.Enabled = false
	for _, intent := range []domain.Intent{domain.Read, domain.Export, domain.Write} {
		if err := service.Run(ctx, a, actor(), "fuelings", intent, mutate); !errors.Is(err, domain.ErrSurfaceDisabled) {
			t.Fatal(err)
		}
	}
	check(t, f, 0, true, 3, 3, 2)
	check(t, f, 1, true, 1, 1, 2)
	service.Enabled = true
	if _, err := service.State(ctx, a, actor(), "fuelings"); err != nil {
		t.Fatal(err)
	}
	policy.denied = true
	for _, intent := range []domain.Intent{domain.Read, domain.Export, domain.Write} {
		if err := service.Run(ctx, a, actor(), "fuelings", intent, mutate); !errors.Is(err, domain.ErrDenied) {
			t.Fatal("capability granted membership", err)
		}
	}
	if _, err := service.Set(ctx, a, actor(), change(4, false, 3)); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("revoked operator could change flag", err)
	}
	check(t, f, 0, true, 3, 3, 2)
}

func TestCapabilityChangesAreAuditedIdempotentAndScoped(t *testing.T) {
	f, store, service, policy := fixture(t)
	ctx := t.Context()
	a := f.Organizations[0].Scope(t)
	b := f.Organizations[1].Scope(t)
	command := change(1, true, 0)
	for attempt := range 2 {
		replayed, err := service.Set(ctx, a, actor(), command)
		if err != nil || replayed != (attempt == 1) {
			t.Fatal(replayed, err)
		}
	}
	for _, altered := range []domain.Change{
		change(1, false, 0),
		{OperationID: operation(1), Capability: "expenses", Enabled: true},
		change(1, true, 1),
	} {
		if _, err := service.Set(ctx, a, actor(), altered); !errors.Is(err, domain.ErrReused) {
			t.Fatal("changed operation accepted", err)
		}
	}
	otherActor := actor()
	otherActor.AccountID = operation(102)
	if _, err := service.Set(ctx, a, otherActor, command); !errors.Is(err, domain.ErrReused) {
		t.Fatal("changed actor replay accepted", err)
	}
	for _, command := range []domain.Change{change(2, false, 0), change(3, false, 2)} {
		if _, err := service.Set(ctx, a, actor(), command); !errors.Is(err, domain.ErrConflict) {
			t.Fatal("stale revision accepted", err)
		}
	}
	if _, err := service.Set(ctx, b, actor(), command); err != nil {
		t.Fatal(err)
	}
	var auditActor, auditDelegation, action, reason, hash string
	err := f.Admin.QueryRow(ctx, `SELECT actor_id::text,delegation_id::text,action,reason_code,request_hash FROM org_audit_events WHERE org_id=$1`, a.OrgID().String()).Scan(&auditActor, &auditDelegation, &action, &reason, &hash)
	digest, _ := command.Digest(a)
	if err != nil || auditActor != actor().AccountID || auditDelegation != actor().DelegationID || action != "capability.enabled" || reason != "operator.request" || hash != digest {
		t.Fatal("audit metadata/digest mismatch", err)
	}
	check(t, f, 0, true, 1, 1, 1)
	check(t, f, 1, true, 1, 1, 1)
	if err := database.WithOrgTx(ctx, f.Runtime, a, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_capabilities WHERE org_id=$1`, b.OrgID().String()).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("foreign state visible")
		}
		tag, err := tx.Exec(ctx, `UPDATE org_capabilities SET enabled=false WHERE org_id=$1`, b.OrgID().String())
		if tag.RowsAffected() != 0 {
			return errors.New("foreign state changed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(ctx, f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_capabilities(org_id,capability) VALUES($1,'expenses')`, b.OrgID().String())
		return err
	}); err == nil {
		t.Fatal("foreign insert accepted")
	}
	var count int
	if err := f.Runtime.QueryRow(ctx, `SELECT count(*) FROM org_capabilities`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unscoped connection leaked flags", count, err)
	}
	if _, err := store.State(ctx, kernel.OrgScope{}, "fuelings"); err == nil {
		t.Fatal("zero scope accepted")
	}
	if _, err := store.State(ctx, a, "unknown"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unregistered capability accepted", err)
	}
	if err := store.GuardWrite(ctx, nil, a, "fuelings"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	policy.operator = false
	if _, err := service.Set(ctx, a, actor(), change(2, false, 1)); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("member provisioned a capability", err)
	}
	check(t, f, 0, true, 1, 1, 1)
}

func TestCapabilityAuditAndCommitFailuresRollBackTogether(t *testing.T) {
	f, _, service, _ := fixture(t)
	ctx := t.Context()
	a := f.Organizations[0].Scope(t)
	if _, err := service.Set(ctx, a, actor(), change(1, true, 0)); err != nil {
		t.Fatal(err)
	}
	runtimeRole := pgx.Identifier{f.RuntimeRole}.Sanitize()
	exec := func(sql string) {
		t.Helper()
		if _, err := f.Admin.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec("REVOKE UPDATE ON org_capabilities FROM " + runtimeRole)
	if _, err := service.Set(ctx, a, actor(), change(2, false, 1)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("state failure accepted", err)
	}
	check(t, f, 0, true, 1, 1, 1)
	exec("GRANT UPDATE ON org_capabilities TO " + runtimeRole)
	if _, err := service.Set(ctx, a, actor(), change(2, false, 1)); err != nil {
		t.Fatal("state failure prevented safe retry", err)
	}
	exec("REVOKE INSERT ON org_audit_events FROM " + runtimeRole)
	if _, err := service.Set(ctx, a, actor(), change(3, true, 2)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("audit failure accepted", err)
	}
	check(t, f, 0, false, 2, 2, 1)
	exec("GRANT INSERT ON org_audit_events TO " + runtimeRole)
	if _, err := service.Set(ctx, a, actor(), change(3, true, 2)); err != nil {
		t.Fatal(err)
	}
	exec(`CREATE FUNCTION capability_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN RAISE EXCEPTION 'synthetic deferred commit failure'; END $$;
	CREATE CONSTRAINT TRIGGER capability_commit_failure AFTER INSERT OR UPDATE ON org_capabilities
	DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION capability_commit_failure()`)
	if _, err := service.Set(ctx, a, actor(), change(4, false, 3)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("failed commit accepted", err)
	}
	check(t, f, 0, true, 3, 3, 1)
	exec("DROP TRIGGER capability_commit_failure ON org_capabilities")
	if _, err := service.Set(ctx, a, actor(), change(4, false, 3)); err != nil {
		t.Fatal("failed commit prevented safe retry", err)
	}
	if _, err := service.Set(ctx, a, actor(), change(5, true, 4)); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic business failure")
	if err := service.Run(ctx, a, actor(), "fuelings", domain.Write, func(ctx context.Context, tx pgx.Tx) error {
		if err := mutate(ctx, tx); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := service.Run(cancelCtx, a, actor(), "fuelings", domain.Write, func(ctx context.Context, tx pgx.Tx) error {
		if err := mutate(ctx, tx); err != nil {
			return err
		}
		cancel()
		return cancelCtx.Err()
	}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled write succeeded", err)
	}
	check(t, f, 0, true, 5, 5, 1)
	exec(`CREATE CONSTRAINT TRIGGER record_commit_failure AFTER UPDATE ON org_example_record
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION capability_commit_failure()`)
	if err := service.Run(ctx, a, actor(), "fuelings", domain.Write, mutate); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("failed business commit accepted", err)
	}
	check(t, f, 0, true, 5, 5, 1)
}

func TestCapabilityDisableWaitsForAcceptedWrites(t *testing.T) {
	f, _, service, _ := fixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	a := f.Organizations[0].Scope(t)
	if _, err := service.Set(ctx, a, actor(), change(1, true, 0)); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	written := make(chan error, 1)
	go func() {
		written <- service.Run(ctx, a, actor(), "fuelings", domain.Write, func(ctx context.Context, tx pgx.Tx) error {
			if err := mutate(ctx, tx); err != nil {
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
	}()
	select {
	case <-entered:
	case err := <-written:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	disabled := make(chan error, 1)
	go func() { _, err := service.Set(ctx, a, actor(), change(2, false, 1)); disabled <- err }()
	// Observe the actual row-lock wait, rather than assume goroutine scheduling.
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		err := f.Admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database()
			AND wait_event_type='Lock' AND query LIKE '%UPDATE org_capabilities%')`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-disabled:
			t.Fatal("disable did not wait", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	check(t, f, 0, true, 1, 1, 1)
	close(release)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if err := <-disabled; err != nil {
		t.Fatal(err)
	}
	if err := service.Run(ctx, a, actor(), "fuelings", domain.Write, mutate); !errors.Is(err, domain.ErrEntitlementRequired) {
		t.Fatal("post-disable write accepted", err)
	}
	check(t, f, 0, false, 2, 2, 2)
}

func TestConcurrentCapabilityReplayAndRevisionConflict(t *testing.T) {
	f, _, service, _ := fixture(t)
	a, b := f.Organizations[0].Scope(t), f.Organizations[1].Scope(t)
	start := make(chan struct{})
	results := make(chan struct {
		replay bool
		err    error
	}, 12)
	for range 12 {
		go func() {
			<-start
			replay, err := service.Set(t.Context(), a, actor(), change(1, true, 0))
			results <- struct {
				replay bool
				err    error
			}{replay, err}
		}()
	}
	close(start)
	replays := 0
	for range 12 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.replay {
			replays++
		}
	}
	if replays != 11 {
		t.Fatalf("replays=%d", replays)
	}
	check(t, f, 0, true, 1, 1, 1)
	start = make(chan struct{})
	conflicts := make(chan error, 2)
	for _, n := range []int{2, 3} {
		go func(n int) {
			<-start
			_, err := service.Set(t.Context(), a, actor(), change(n, false, 1))
			conflicts <- err
		}(n)
	}
	close(start)
	success, stale := 0, 0
	for range 2 {
		err := <-conflicts
		switch {
		case err == nil:
			success++
		case errors.Is(err, domain.ErrConflict):
			stale++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatal(success, stale)
	}
	check(t, f, 0, false, 2, 2, 1)
	// The same operation in another organization does not collide.
	if _, err := service.Set(t.Context(), b, actor(), change(1, true, 0)); err != nil {
		t.Fatal(err)
	}
	check(t, f, 1, true, 1, 1, 1)
}

func TestCapabilityGuardJoinsExistingAuditTransaction(t *testing.T) {
	f, store, service, policy := fixture(t)
	ctx := t.Context()
	a := f.Organizations[0].Scope(t)
	if _, err := service.Set(ctx, a, actor(), change(1, true, 0)); err != nil {
		t.Fatal(err)
	}
	catalog, _ := audit.NewCatalog([]audit.Definition{{Action: "fixture.changed", TargetType: "record"}})
	runner := auditstore.NewStore(f.Runtime, catalog)
	event := audit.Event{OperationID: operation(2), ActorID: actor().AccountID, Action: "fixture.changed", TargetType: "record", TargetID: f.Organizations[0].RecordID, RequestHash: fmt.Sprintf("%064x", 1)}
	if _, err := runner.RecordChange(ctx, a, event, func(ctx context.Context, tx pgx.Tx) error {
		if err := service.GuardWrite(ctx, tx, a, actor(), "fuelings"); err != nil {
			return err
		}
		return mutate(ctx, tx)
	}); err != nil {
		t.Fatal(err)
	}
	check(t, f, 0, true, 1, 2, 2)
	if _, err := service.Set(ctx, a, actor(), change(3, false, 1)); err != nil {
		t.Fatal(err)
	}
	event.OperationID = operation(4)
	if _, err := runner.RecordChange(ctx, a, event, func(ctx context.Context, tx pgx.Tx) error {
		if err := service.GuardWrite(ctx, tx, a, actor(), "fuelings"); err != nil {
			return err
		}
		return mutate(ctx, tx)
	}); !errors.Is(err, domain.ErrEntitlementRequired) {
		t.Fatal("audited command bypassed flag", err)
	}
	check(t, f, 0, false, 2, 3, 2)
	policy.denied = true
	if err := database.WithOrgTx(ctx, f.Runtime, a, func(tx pgx.Tx) error {
		return service.GuardWrite(ctx, tx, a, actor(), "fuelings")
	}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("guard ignored current authority", err)
	}
	unsafeStore := adapters.NewStore(f.Admin, service.Catalog, nil)
	if _, err := unsafeStore.State(ctx, a, "fuelings"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("privileged runtime allowed", err)
	}
	if _, err := unsafeStore.Set(ctx, a, actor(), change(5, true, 2)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("missing audit runner allowed", err)
	}
	if err := store.Run(ctx, a, "fuelings", domain.Manage, mutate); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, "REVOKE SELECT ON org_capabilities FROM "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.State(ctx, a, "fuelings"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("unavailable state read accepted", err)
	}
	if err := database.WithOrgTx(ctx, f.Runtime, a, func(tx pgx.Tx) error {
		return store.GuardWrite(ctx, tx, a, "fuelings")
	}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("unavailable guard opened a write", err)
	}
}
