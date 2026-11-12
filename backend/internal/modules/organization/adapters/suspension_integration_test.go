//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	adminpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admin "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

type suspensionFixtureState struct {
	legalFixtureState
	runner       *adminpg.Store
	adminService *adminapp.Service
	session      adminapp.Session
	credentials  adminapp.LoginResult
	key          string
	store        *adapters.SuspensionStore
	service      app.SuspensionService
}

func suspensionFixture(t *testing.T) suspensionFixtureState {
	t.Helper()
	return suspensionOnFixture(t, legalFixture(t))
}
func suspensionOnFixture(t *testing.T, fx legalFixtureState) suspensionFixtureState {
	t.Helper()
	r := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON admin_sessions,admin_login_attempts,admin_idempotency,org_suspension_periods TO "+r+";GRANT DELETE ON admin_idempotency TO "+r+";GRANT SELECT,INSERT ON admin_audit_events,org_suspension_notices TO "+r+";GRANT USAGE,SELECT ON SEQUENCE admin_audit_events_id_seq TO "+r); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	runner := adminpg.NewStore(fx.f.Runtime)
	key := admin.KeyPrefix + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{81}, 32))
	hash := admin.HashKey(key)
	adminService := &adminapp.Service{Store: runner, Keys: []admin.ConfiguredKey{{Hash: hash, Fingerprint: admin.Fingerprint(hash)}}, Clock: time.Now, NewID: jobs.NewUUIDv4}
	credentials, e := adminService.Login(t.Context(), key, admin.HashKey("synthetic-operator-ip"), "synthetic-request")
	if e != nil {
		t.Fatal(e)
	}
	session, e := adminService.Authenticate(t.Context(), credentials.Token)
	if e != nil {
		t.Fatal(e)
	}
	store := &adapters.SuspensionStore{Roots: adapters.NewStore(fx.f.Runtime), Proof: runner, NewID: jobs.NewUUIDv4}
	return suspensionFixtureState{fx, runner, adminService, session, credentials, key, store, app.SuspensionService{Enabled: true, Runner: runner, Repository: store}}
}
func (fx suspensionFixtureState) change(t *testing.T, action app.SuspensionAction, rev int64, key string) (adminapp.MutationResult, error) {
	t.Helper()
	if key == "" {
		key = onboardingID(t)
	}
	command := app.SuspensionCommand{Action: action, ExpectedRevision: rev, Category: d.Security}
	raw, _ := json.Marshal([]any{action, rev, command.Category})
	m := adminapp.Mutation{SessionID: fx.session.ID, Action: action.AdminAction(), TargetType: "organization", TargetID: fx.scope.OrgID().String(), Key: key, RequestHash: admin.HashKey(string(raw)), Reason: "Synthetic operator cause resolution", ClientIPHash: admin.HashKey("synthetic-operator-ip")}
	return fx.service.Change(t.Context(), fx.scope, m, command)
}
func (fx suspensionFixtureState) root(t *testing.T) d.Snapshot {
	t.Helper()
	root, e := adapters.NewStore(fx.f.Runtime).Get(t.Context(), fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	v, e := root.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestSuspensionAtomicHistoryReplayCurrentProofAndArchivedCauses(t *testing.T) {
	fx := suspensionFixture(t)
	key := onboardingID(t)
	first, e := fx.change(t, app.SuspendOrganization, 2, key)
	if e != nil || first.Replayed || first.Status != 200 {
		t.Fatal("suspend", e)
	}
	root := fx.root(t)
	if root.State != d.Suspended || root.Revision != 3 || root.SuspensionReason != d.Security {
		t.Fatal("root suspension")
	}
	if _, e := fx.change(t, app.ReactivateOrganization, 3, ""); e != nil {
		t.Fatal("reactivate", e)
	}
	replay, e := fx.change(t, app.SuspendOrganization, 2, key)
	if e != nil || !replay.Replayed || !bytes.Equal(replay.Body, first.Body) || fx.root(t).State != d.Active {
		t.Fatal("original retry changed state/ack", e)
	}
	if _, e := fx.change(t, app.SuspendOrganization, 4, key); !errors.Is(e, admin.ErrConflict) {
		t.Fatal("altered replay", e)
	}
	var valid bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_suspension_periods WHERE org_id=$1 AND ended_at IS NOT NULL AND ended_revision=4)=1 AND(SELECT count(*) FROM org_suspension_notices WHERE org_id=$1)=2 AND(SELECT count(*) FROM admin_audit_events WHERE action LIKE 'admin.organization.%')=2`, fx.scope.OrgID().String()).Scan(&valid); e != nil || !valid {
		t.Fatal("atomic audit/history/notices", e)
	}
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.change(t, app.SuspendOrganization, 5, ""); e != nil {
		t.Fatal("archived suspension cause", e)
	}
	if fx.root(t).State != d.Archived {
		t.Fatal("archive reopened")
	}
	if _, e := fx.change(t, app.ReactivateOrganization, 6, ""); e != nil {
		t.Fatal(e)
	}
	if root := fx.root(t); root.State != d.Archived || root.SuspensionReason != "" {
		t.Fatal("resolution reopened archive")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE admin_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, fx.session.ID); e != nil {
		t.Fatal(e)
	}
	if r, e := fx.change(t, app.SuspendOrganization, 2, key); !errors.Is(e, admin.ErrUnauthorized) || len(r.Body) != 0 {
		t.Fatal("revoked session replayed", e)
	}
}
func TestSuspensionFreshProofLateFailureAndForeignScopeLeaveNoPartialState(t *testing.T) {
	fx := suspensionFixture(t)
	key := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_suspension_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic late suspension outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_suspension_outage AFTER INSERT ON org_suspension_notices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_suspension_outage()`); e != nil {
		t.Fatal(e)
	}
	if r, e := fx.change(t, app.SuspendOrganization, 2, key); !errors.Is(e, admin.ErrUnavailable) || len(r.Body) != 0 {
		t.Fatal("provisional commit", e)
	}
	if root := fx.root(t); root.Revision != 2 || root.State != d.Active {
		t.Fatal("partial root")
	}
	var clean bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_suspension_periods) AND NOT EXISTS(SELECT 1 FROM org_suspension_notices) AND NOT EXISTS(SELECT 1 FROM admin_audit_events WHERE action LIKE 'admin.organization.%') AND NOT EXISTS(SELECT 1 FROM admin_idempotency WHERE key=$1)`, key).Scan(&clean); e != nil || !clean {
		t.Fatal("partial history/audit/receipt", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_suspension_outage ON org_suspension_notices`); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.change(t, app.SuspendOrganization, 2, key); e != nil {
		t.Fatal("failure recovery", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE admin_sessions SET created_at=clock_timestamp()-interval '16 minutes' WHERE id=$1`, fx.session.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.change(t, app.SuspendOrganization, 2, key); !errors.Is(e, admin.ErrUnauthorized) {
		t.Fatal("stale fresh replay", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE admin_sessions SET created_at=clock_timestamp()-interval '1 minute' WHERE id=$1`, fx.session.ID); e != nil {
		t.Fatal(e)
	}
	foreign := fx
	foreign.scope = fx.f.Organizations[0].Scope(t)
	if _, e := foreign.change(t, app.SuspendOrganization, 1, ""); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("metadata anchor acquired lifecycle", e)
	}
	var scope string
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')`).Scan(&scope); e != nil || scope != "" {
		t.Fatal("scope leaked", e)
	}
}
func TestSuspensionConcurrentCASReplayAndOwningIntake(t *testing.T) {
	fx := suspensionFixture(t)
	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		key := onboardingID(t)
		wg.Go(func() { _, e := fx.change(t, app.SuspendOrganization, 2, key); errs <- e })
	}
	wg.Wait()
	close(errs)
	count := 0
	for e := range errs {
		if e == nil {
			count++
		} else if !errors.Is(e, d.ErrConflict) {
			t.Fatal(e)
		}
	}
	if count != 1 {
		t.Fatal("lost CAS", count)
	}
	key := onboardingID(t)
	acks := make(chan adminapp.MutationResult, n)
	errs = make(chan error, n)
	for range n {
		wg.Go(func() { r, e := fx.change(t, app.ReactivateOrganization, 3, key); acks <- r; errs <- e })
	}
	wg.Wait()
	close(acks)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var body []byte
	for ack := range acks {
		if body == nil {
			body = ack.Body
		}
		if !bytes.Equal(body, ack.Body) {
			t.Fatal("retry differed")
		}
	}
	// A business command pins root history through its commit. Later suspension
	// cannot change the already received fact's classification.
	captured, release, done := make(chan app.SuspensionIntake, 1), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			v, e := fx.store.AssessSuspensionTx(t.Context(), tx, fx.scope, time.Now(), time.Time{})
			if e != nil {
				return e
			}
			captured <- v
			<-release
			return nil
		})
	}()
	var intake app.SuspensionIntake
	select {
	case intake = <-captured:
	case e := <-done:
		t.Fatal(e)
	case <-time.After(10 * time.Second):
		t.Fatal("intake stalled")
	}
	pending := make(chan error, 1)
	go func() { _, e := fx.change(t, app.SuspendOrganization, 4, ""); pending <- e }()
	select {
	case e := <-pending:
		t.Fatal("suspension passed the locked intake", e)
	case <-time.After(80 * time.Millisecond):
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := <-pending; e != nil {
		t.Fatal(e)
	}
	if intake.Assessment.Disposition != d.SuspensionAllow || intake.RootRevision != 4 {
		t.Fatal("old intake changed")
	}
}

func TestSuspensionPreservesPersistedFieldReviewAfterReactivation(t *testing.T) {
	fx := suspensionFixture(t)
	declared := time.Now().UTC().Add(-time.Hour)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_suspension_facts(org_id uuid NOT NULL,id uuid NOT NULL,disposition text NOT NULL,root_revision bigint NOT NULL,received_at timestamptz NOT NULL,PRIMARY KEY(org_id,id));ALTER TABLE synthetic_suspension_facts ENABLE ROW LEVEL SECURITY;ALTER TABLE synthetic_suspension_facts FORCE ROW LEVEL SECURITY;CREATE POLICY synthetic_suspension_facts_scope ON synthetic_suspension_facts USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);GRANT SELECT,INSERT ON synthetic_suspension_facts TO `+role); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.change(t, app.SuspendOrganization, 2, ""); e != nil {
		t.Fatal(e)
	}
	var held app.SuspensionIntake
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		v, e := fx.store.AssessSuspensionTx(t.Context(), tx, fx.scope, declared, time.Time{})
		if e != nil {
			return e
		}
		held = v
		_, e = tx.Exec(t.Context(), `INSERT INTO synthetic_suspension_facts VALUES($1,$2,$3,$4,$5)`, fx.scope.OrgID().String(), onboardingID(t), string(v.Assessment.Disposition), v.RootRevision, v.ReceivedAt)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if held.Assessment.Disposition != d.SuspensionReview || held.Assessment.Code != "ORG_SUSPENDED_GRACE" {
		t.Fatal("pre-suspension fact discarded")
	}
	if _, e := fx.change(t, app.ReactivateOrganization, 3, ""); e != nil {
		t.Fatal(e)
	}
	var retained bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT disposition='REVIEW' AND root_revision=3 FROM synthetic_suspension_facts WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&retained); e != nil || !retained {
		t.Fatal("held fact silently promoted", e)
	}
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		v, e := fx.store.AssessSuspensionTx(t.Context(), tx, fx.scope, declared, time.Time{})
		if e != nil {
			return e
		}
		if v.Assessment.Disposition != d.SuspensionReview {
			t.Fatal("reactivated history bypassed")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	// Opened work uses server-owned history and is still review-only, never a
	// general override allowing a client to create work during suspension.
	if _, e := fx.change(t, app.SuspendOrganization, 4, ""); e != nil {
		t.Fatal(e)
	}
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		fresh, e := fx.store.AssessSuspensionTx(t.Context(), tx, fx.scope, time.Now(), time.Time{})
		if e != nil {
			return e
		}
		if fresh.Assessment.Disposition != d.SuspensionRefuse {
			t.Fatal("new suspended work accepted")
		}
		finish, e := fx.store.AssessSuspensionTx(t.Context(), tx, fx.scope, time.Now(), declared)
		if e != nil {
			return e
		}
		if finish.Assessment.Disposition != d.SuspensionReview {
			t.Fatal("existing completion lost")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}

func TestSuspensionSQLGuardsKeepHistoryPrivateAndRequireRealOperatorProof(t *testing.T) {
	fx := suspensionFixture(t)
	if _, e := fx.change(t, app.SuspendOrganization, 2, ""); e != nil {
		t.Fatal(e)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT DELETE,TRUNCATE ON org_suspension_periods TO "+role+";GRANT UPDATE,DELETE,TRUNCATE ON org_suspension_notices TO "+role); e != nil {
		t.Fatal(e)
	}
	org := fx.scope.OrgID().String()
	teamSQLCode(t, unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_suspension_periods SET reason='LEGAL' WHERE org_id=$1`, org), "P8401")
	teamSQLCode(t, unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_suspension_periods SET started_at=started_at-interval '1 hour' WHERE org_id=$1`, org), "P8401")
	teamSQLCode(t, unitSchemaExec(t, fx.legalFixtureState, `DELETE FROM org_suspension_periods WHERE org_id=$1`, org), "55000")
	teamSQLCode(t, unitSchemaExec(t, fx.legalFixtureState, `TRUNCATE org_suspension_periods CASCADE`), "55000")
	teamSQLCode(t, unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_suspension_notices SET delivery_state='PENDING_NOTIFICATION_CORE' WHERE org_id=$1`, org), "55000")
	if _, e := fx.change(t, app.ReactivateOrganization, 3, ""); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_suspension_periods SET ended_at=clock_timestamp() WHERE org_id=$1`, org), "P8401")
	// An existing suspended snapshot without an interval is not evidence of an
	// operator. The SQL guard verifies its actual session instead of trusting IDs.
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='SUSPENDED',suspension_reason='SECURITY',revision=revision+1 WHERE org_id=$1`, org); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_suspension_periods(org_id,id,reason,started_revision,started_session_id) VALUES($1,$2,'SECURITY',5,$3)`, org, onboardingID(t), onboardingID(t)), "P8403")
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		_, e := fx.store.AssessSuspensionTx(t.Context(), tx, fx.scope, time.Now(), time.Time{})
		if !errors.Is(e, app.ErrUnavailable) {
			t.Fatal("unknown historical cause fabricated", e)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
