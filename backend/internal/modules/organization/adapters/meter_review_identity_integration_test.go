//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func meterReviewEmployee(t *testing.T, fx syncFixtureState, member string) string {
	t.Helper()
	var id string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), member).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func meterReviewDecisionRows(t *testing.T, fx syncFixtureState, review string) int {
	t.Helper()
	var n int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_resolutions WHERE org_id=$1 AND review_item_id=$2)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='offl.review.resolved' AND target_id=$2)+(SELECT count(*) FROM org_meter_readings r JOIN org_meter_held_readings h ON(h.org_id,h.reading_id)=(r.org_id,r.id) WHERE h.org_id=$1 AND h.review_item_id=$2)`, fx.scope.OrgID().String(), review).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The author leaves through the actual offboarding command and the actual
// employee merge folds that identity into the current reviewer's employee. The
// reviewer differs by account and membership but is the same canonical person.
func TestMeterReviewCanonicalEmployeeMergeRefusesSelfReviewAcrossAccounts(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	_, ref, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	author, reviewer := meterReviewEmployee(t, fx, fx.person.member), meterReviewEmployee(t, fx, fx.member)
	if author == reviewer || fx.person.auth.Account.ID == fx.actor.AccountID || fx.person.member == fx.member {
		t.Fatal("fixture reviewer is not a distinct account, membership and employee")
	}
	grantEmployeeTables(t, fx.legalFixtureState)
	people := membershipAccessOn(t, invitationFixtureState{legalFixtureState: fx.legalFixtureState})
	offboarding := offboardingStore(t, people)
	if out, err := offboarding.CompleteOffboarding(t.Context(), fx.scope, accessActor(people), fx.member, offboardingCommand(t, offboardingRead(t, people, offboarding, fx.person.member))); err != nil || out.View.State.EndReason != d.Offboarded {
		t.Fatal("actual author offboarding", err)
	}
	merges := employeeMergeStore(t, people)
	if _, err := mergeWrite(t, people, merges, mergeCommand(t, people, mergePreview(t, people, merges, author, reviewer))); err != nil {
		t.Fatal("actual employee merge", err)
	}
	var canonical string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT org_sync_review_employee_canonical($1,$2)::text`, fx.scope.OrgID().String(), author).Scan(&canonical); err != nil || canonical != reviewer {
		t.Fatal("merged author did not resolve to the reviewer's canonical employee", err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision); !errors.Is(err, orgapi.SelfApproval) {
		t.Fatal("merged author approved own held record through another account", err)
	}
	if n := meterReviewDecisionRows(t, fx, ref.ReviewItemID); n != 0 {
		t.Fatal("refused self review left a decision, audit or reading", n)
	}
	// The database guard independently follows the same canonical chain.
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_sync_review_resolutions(org_id,id,review_item_id,expected_revision,outcome,reason,reviewer_account_id,reviewer_membership_id,reviewer_employee_id,reviewer_session_id,request_hash,resolved_at,canonical_resolution)VALUES($1,$2,$3,1,'REFUSED','Synthetic direct self review',$4,$5,$6,$7,repeat('a',64),clock_timestamp(),'{}'::bytea)`, fx.scope.OrgID().String(), onboardingID(t), ref.ReviewItemID, fx.actor.AccountID, fx.member, reviewer, fx.actor.SessionID)
		return err
	})
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "P9006" {
		t.Fatal("SQL accepted a merged-identity self review", err)
	}
	// A genuinely different current person can still decide the same record.
	other := deviceNewPerson(t, fx.deviceFixtureState)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: other.member}, "ADMIN", "ORG", "ACTIVE")
	refusal, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewRefuse, Reason: "Synthetic independent refusal"})
	if err != nil {
		t.Fatal(err)
	}
	if result, _, err := store.ResolveMeterReview(t.Context(), fx.scope, app.FleetActor{AccountID: other.web.AccountID, SessionID: other.web.SessionID}, other.member, refusal); err != nil || result.Snapshot().Outcome != protocol.ReviewRefused {
		t.Fatal("independent canonical reviewer could not decide", err)
	}
}

func meterReviewAwaitLock(t *testing.T, ctx context.Context, fx syncFixtureState, user string, holder int, query string, done <-chan error) int {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiter int
		err := fx.f.Admin.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND $2=ANY(pg_blocking_pids(pid)) AND query LIKE $3 LIMIT 1`, user, holder, query).Scan(&waiter)
		if err == nil {
			return waiter
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("lock wait observation failed", err)
		}
		select {
		case err := <-done:
			t.Fatal("expected actual lock wait did not happen", err)
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("no actual lock wait observed")
		}
	}
}

func meterReviewWaitFixture(t *testing.T) (syncFixtureState, string, devicePerson, app.MeterReviewDecision, func(context.Context) error) {
	t.Helper()
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	_, ref, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	reviewer := deviceNewPerson(t, fx.deviceFixtureState)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: reviewer.member}, "ADMIN", "ORG", "ACTIVE")
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(ctx context.Context) error {
		_, _, err := store.ResolveMeterReview(ctx, fx.scope, app.FleetActor{AccountID: reviewer.web.AccountID, SessionID: reviewer.web.SessionID}, reviewer.member, decision)
		return err
	}
	return fx, ref.ReviewItemID, reviewer, decision, resolve
}

const meterReviewAgeSession = `UPDATE company_web_sessions SET authenticated_at=authenticated_at-interval '20 minutes',created_at=created_at-interval '20 minutes',expires_at=expires_at-interval '20 minutes' WHERE id=$1`

// A decision blocked on the reviewer's proof row re-evaluates freshness on the
// committed row after the wait; the expired proof publishes nothing.
func TestMeterReviewFreshProofIsReevaluatedAfterProofLockWait(t *testing.T) {
	fx, review, reviewer, _, resolve := meterReviewWaitFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	change, err := fx.f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = change.Rollback(context.Background()) }()
	var holder int
	if err := change.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if _, err := change.Exec(ctx, meterReviewAgeSession, reviewer.web.SessionID); err != nil {
		t.Fatal(err)
	}
	resolved := make(chan error, 1)
	go func() { resolved <- resolve(ctx) }()
	meterReviewAwaitLock(t, ctx, fx, fx.f.RuntimeRole, holder, "%name: LockLegalProof%", resolved)
	if err := change.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-resolved; !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("decision kept a proof that expired during its wait", err)
	}
	if n := meterReviewDecisionRows(t, fx, review); n != 0 {
		t.Fatal("expired proof left a decision, audit or reading", n)
	}
}

// While a decision waits on the review row it retains its account/proof/root
// authority locks, so a concurrent proof change cannot commit inside the
// decision window: it waits for the decision and then applies afterwards.
func TestMeterReviewAuthorityChangeCannotInterleaveReviewLockWait(t *testing.T) {
	fx, review, reviewer, _, resolve := meterReviewWaitFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	locked, release, blocker := make(chan int, 1), make(chan struct{}), make(chan error, 1)
	var releaseOnce sync.Once
	unlock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unlock()
	go func() {
		blocker <- database.WithOrgTx(ctx, fx.f.RuntimePool(t, 1), fx.scope, func(tx pgx.Tx) error {
			var pid int
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid() FROM org_sync_review_items WHERE org_id=$1 AND id=$2 FOR UPDATE`, fx.scope.OrgID().String(), review).Scan(&pid); err != nil {
				return err
			}
			locked <- pid
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var holder int
	select {
	case holder = <-locked:
	case err := <-blocker:
		t.Fatal("review row lock unavailable", err)
	case <-ctx.Done():
		t.Fatal("review row lock timed out")
	}
	resolved := make(chan error, 1)
	go func() { resolved <- resolve(ctx) }()
	decider := meterReviewAwaitLock(t, ctx, fx, fx.f.RuntimeRole, holder, "%name: LockMeterReview%", resolved)
	var admin string
	if err := fx.f.Admin.QueryRow(ctx, `SELECT current_user`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	aged := make(chan error, 1)
	go func() {
		_, err := fx.f.Admin.Exec(ctx, meterReviewAgeSession, reviewer.web.SessionID)
		aged <- err
	}()
	meterReviewAwaitLock(t, ctx, fx, admin, decider, "%UPDATE company_web_sessions%", aged)
	unlock()
	if err := <-blocker; err != nil {
		t.Fatal(err)
	}
	if err := <-resolved; err != nil {
		t.Fatal("decision with retained current authority failed", err)
	}
	if err := <-aged; err != nil {
		t.Fatal("proof change after the decision failed", err)
	}
	if n := meterReviewDecisionRows(t, fx, review); n != 3 {
		t.Fatal("serialized decision did not publish exactly one decision, audit and reading", n)
	}
}
