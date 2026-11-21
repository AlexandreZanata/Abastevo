//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	orgd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// stateFixture is the actual meter owner and signed intake store composed as
// the runtime composes EMP-OFFL-012: the current source falls back to the
// suspended-organization source, and ORG-009's assessment plus the plan rule
// run before the owner. The ORG-009 operator fixture suspends/reactivates.
type stateFixture struct {
	lateFixture
	ops   suspensionFixtureState
	state *syncpg.Store
}

func stateIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := lateIntakeConfig(fx)
	config.ConfirmSource = func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (time.Time, error) {
		_, at, err := fx.devices.CheckCurrentIntakeDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
		if errors.Is(err, access.ErrOrgSuspended) {
			_, at, err = fx.devices.CheckSuspendedIntakeDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
		}
		if err != nil {
			return time.Time{}, syncapp.SourceDenied(err)
		}
		return at, nil
	}
	assessor := orgpg.IntakeStateAssessor{Suspensions: &orgpg.SuspensionStore{Roots: orgpg.NewStore(fx.f.Runtime)}, Plans: fx.flags}
	config.AssessState = assessor.AssessStateTx
	return config
}

func stateIntakeFixture(t *testing.T) stateFixture {
	t.Helper()
	x := lateIntakeFixture(t)
	x.f.GrantStateIntake(t)
	ops := suspensionOnFixture(t, x.legalFixtureState)
	return stateFixture{lateFixture: x, ops: ops, state: newStateStore(x)}
}

func newStateStore(x lateFixture) *syncpg.Store {
	return syncpg.NewStore(stateIntakeConfig(x.syncFixtureState))
}

func (x stateFixture) process(t *testing.T, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	return x.state.Process(t.Context(), x.source, op, protocol.DefaultLimits(), x.handler)
}

func (x stateFixture) suspend(t *testing.T) time.Time {
	t.Helper()
	if _, err := x.ops.change(t, app.SuspendOrganization, x.ops.root(t).Revision, ""); err != nil {
		t.Fatal("operator suspension", err)
	}
	var started time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT started_at FROM org_suspension_periods WHERE org_id=$1 AND ended_at IS NULL`, x.scope.OrgID().String()).Scan(&started); err != nil {
		t.Fatal(err)
	}
	return started.UTC()
}

func (x stateFixture) reactivate(t *testing.T) time.Time {
	t.Helper()
	if _, err := x.ops.change(t, app.ReactivateOrganization, x.ops.root(t).Revision, ""); err != nil {
		t.Fatal("operator reactivation", err)
	}
	var ended time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT max(ended_at) FROM org_suspension_periods WHERE org_id=$1`, x.scope.OrgID().String()).Scan(&ended); err != nil {
		t.Fatal(err)
	}
	return ended.UTC()
}

func (x stateFixture) stateRow(t *testing.T, op protocol.Operation) (condition, disposition, code string, ok bool) {
	t.Helper()
	err := x.f.Admin.QueryRow(t.Context(), `SELECT condition,disposition,code FROM org_sync_state_intakes WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), op.ID()).Scan(&condition, &disposition, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return condition, disposition, code, true
}

func (x stateFixture) heldState(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, condition, code string) {
	t.Helper()
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != code || ack.Snapshot().ReviewItemID == "" || ack.Snapshot().EntityRevision != nil {
		t.Fatal("state work was not quarantined", code, err, ack.Result(), ack.Code())
	}
	if claims, acks, readings, held, effects, items := x.counts(t, op); claims != 1 || acks != 1 || readings != 0 || held != 1 || effects != 0 || items != 1 {
		t.Fatal("state quarantine applied or lost a fact", claims, acks, readings, held, effects, items)
	}
	if c, d, k, ok := x.stateRow(t, op); !ok || c != condition || d != syncapp.StateHold || k != code {
		t.Fatal("state decision row missing or different", c, d, k)
	}
}

func (x stateFixture) refusedState(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != syncapp.StateSuspendedCode {
		t.Fatal("new work in a suspension was not refused", err, ack.Result(), ack.Code())
	}
	if claims, acks, readings, held, effects, items := x.counts(t, op); claims != 1 || acks != 1 || readings+held+effects+items != 0 {
		t.Fatal("state refusal stored a fact", claims, acks, readings, held, effects, items)
	}
	if c, d, k, ok := x.stateRow(t, op); !ok || c != syncapp.StateSuspension || d != syncapp.StateRefuse || k != syncapp.StateSuspendedCode {
		t.Fatal("state refusal row missing or different", c, d, k)
	}
}

func (x stateFixture) replaysState(t *testing.T, op protocol.Operation, original protocol.Acknowledgement) {
	t.Helper()
	for range 2 {
		again, err := x.process(t, op)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), original.CanonicalBytes()) {
			t.Fatal("state replay did not return the stored acknowledgement byte for byte", err)
		}
	}
}

func (x stateFixture) authority(t *testing.T) string {
	t.Helper()
	var value string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(m) ORDER BY id) FROM org_memberships m WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM org_role_assignments r WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM org_device_enrollments e WHERE org_id=$1))::text`, x.scope.OrgID().String()).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// VAL-22 part 2 on the actual meter owner: declared before the suspension and
// inside the grace → held ORG_SUSPENDED_GRACE; declared at or after → final
// ORG_SUSPENDED; resends replay (also while suspended); after reactivation the
// resolved interval still holds its work and new work applies.
func TestStateIntakeSuspensionGraceNewWorkAndReactivation(t *testing.T) {
	x := stateIntakeFixture(t)
	before := x.authority(t)
	pre := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	started := x.suspend(t)
	held, err := x.process(t, pre)
	x.heldState(t, pre, held, err, syncapp.StateSuspension, syncapp.StateGraceCode)
	x.replaysState(t, pre, held)
	for _, at := range []time.Time{started, started.Add(time.Nanosecond), started.Add(time.Minute)} {
		op := x.op(t, at)
		ack, err := x.process(t, op)
		x.refusedState(t, op, ack, err)
		x.replaysState(t, op, ack)
	}
	forged := x.op(t, x.installedAt(t).Add(time.Nanosecond))
	ack, err := x.process(t, forged)
	x.heldState(t, forged, ack, err, syncapp.StateSuspension, syncapp.StateGraceCode)
	received, _ := orgtime.ParseInstant(ack.Snapshot().ReceivedAt)
	if !received.After(started) {
		t.Fatal("the receipt instant came from the device declaration")
	}
	t.Run("reused-identifier", func(t *testing.T) {
		changed := lateRedeclared(t, x.lateFixture, pre, started.Add(-time.Minute))
		reused, err := x.process(t, changed)
		if err != nil || reused.Code() != "OPERATION_ID_REUSED" {
			t.Fatal("changed intent under a held state identifier", err, reused.Code())
		}
		x.replaysState(t, pre, held)
	})
	ended := x.reactivate(t)
	during := x.op(t, started.Add(time.Microsecond))
	if !started.Add(time.Microsecond).Before(ended) {
		t.Fatal("fixture suspension interval too short")
	}
	ack, err = x.process(t, during)
	x.heldState(t, during, ack, err, syncapp.StateSuspension, syncapp.StateGraceCode)
	fresh := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	applied, err := x.process(t, fresh)
	if err != nil || applied.Result() != protocol.Accepted {
		t.Fatal("new work after reactivation did not apply", err, applied.Code())
	}
	if _, _, _, ok := x.stateRow(t, fresh); ok {
		t.Fatal("applied work recorded a state decision")
	}
	x.replaysState(t, pre, held)
	if x.authority(t) != before {
		t.Fatal("suspension or reactivation changed a membership, role or enrollment")
	}
}

// VAL-24/35 through the capability row: declared before the downgrade → held
// PLAN_CHANGED; declared at or after it → the owner's ENTITLEMENT_REQUIRED
// with nothing stored; no role, membership or enrollment changes.
func TestStateIntakePlanDowngradeHoldsEarlierWorkOnly(t *testing.T) {
	x := stateIntakeFixture(t)
	before := x.authority(t)
	pre := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	revision := stateCapabilityRevision(t, x)
	if _, err := x.flags.Set(t.Context(), x.scope, capability.Actor{AccountID: x.actor.AccountID}, capability.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: revision, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	var changed time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT updated_at FROM org_capabilities WHERE org_id=$1 AND capability='organizations'`, x.scope.OrgID().String()).Scan(&changed); err != nil {
		t.Fatal(err)
	}
	held, err := x.process(t, pre)
	x.heldState(t, pre, held, err, syncapp.StatePlan, syncapp.StatePlanCode)
	x.replaysState(t, pre, held)
	var instant time.Time
	var rev int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT plan_changed_at,capability_revision FROM org_sync_state_intakes WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), pre.ID()).Scan(&instant, &rev); err != nil || !instant.Equal(changed) || rev != revision+1 {
		t.Fatal("plan evidence not recorded", err, instant, rev)
	}
	for _, at := range []time.Time{changed.UTC(), changed.UTC().Add(time.Second)} {
		post := x.op(t, at)
		ack, err := x.process(t, post)
		if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != orgapi.EntitlementRequired || len(ack.CanonicalBytes()) != 0 {
			t.Fatal("new work after the downgrade was not refused by the owner", err)
		}
		x.untouched(t, post)
	}
	if _, err := x.flags.Set(t.Context(), x.scope, capability.Actor{AccountID: x.actor.AccountID}, capability.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: revision + 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	fresh := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	if ack, err := x.process(t, fresh); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("work after the plan returned did not apply", err)
	}
	x.replaysState(t, pre, held)
	if x.authority(t) != before {
		t.Fatal("a plan change changed a membership, role or enrollment")
	}
}

func TestStateIntakeSubscriptionLossKeepsOnlyPreviouslyEntitledWork(t *testing.T) {
	x := stateIntakeFixture(t)
	ctx := t.Context()
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog TO "+role+
		"; GRANT SELECT, INSERT ON org_billing_subscription_events TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities)
VALUES('fleet','synthetic',1,'Synthetic plan',ARRAY['organizations'])`); err != nil {
		t.Fatal(err)
	}
	x.flags.PlanWrites = billpg.EntitlementStore{Pool: x.f.Runtime, Capabilities: known, Now: time.Now}
	config := stateIntakeConfig(x.syncFixtureState)
	assessor := orgpg.IntakeStateAssessor{
		Suspensions: &orgpg.SuspensionStore{Roots: orgpg.NewStore(x.f.Runtime)}, Plans: x.flags, SubscriptionPlans: true,
	}
	config.AssessState = assessor.AssessStateTx
	x.state = syncpg.NewStore(config)

	ledger := billpg.SubscriptionLedger{Capabilities: known}
	key := billing.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	subscription := onboardingID(t)
	at := lifecycleDBNow(t, x.legalFixtureState)
	startTrial := billing.LifecycleEvent{
		Scope: x.scope, SubscriptionID: subscription, Kind: billing.StartTrial, Source: billing.TrialPolicy,
		Reference: "synthetic-trial-rule", At: at, Period: billing.Period{Start: at, End: at.Add(time.Hour)},
	}
	beforeActivation := x.op(t, at.Add(-time.Minute))
	apply := func(event billing.LifecycleEvent) {
		t.Helper()
		if err := database.WithOrgTx(ctx, x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			_, err := ledger.ApplyTx(ctx, tx, key, event)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	apply(startTrial)
	earlier := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	before := x.authority(t)
	suspend := billing.LifecycleEvent{
		Scope: x.scope, SubscriptionID: subscription, Kind: billing.SuspendAccess, Source: billing.ReviewedOperator,
		ExpectedRevision: 1, Reference: "synthetic-reviewed-suspension", At: lifecycleDBNow(t, x.legalFixtureState),
	}
	apply(suspend)
	held, err := x.process(t, earlier)
	x.heldState(t, earlier, held, err, syncapp.StatePlan, syncapp.StatePlanCode)
	x.replaysState(t, earlier, held)
	var recordedSubscription string
	var recordedRevision int64
	if err := x.f.Admin.QueryRow(ctx, `SELECT subscription_id::text,subscription_revision FROM org_sync_state_intakes
WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), earlier.ID()).Scan(&recordedSubscription, &recordedRevision); err != nil ||
		recordedSubscription != subscription || recordedRevision != 1 {
		t.Fatalf("earlier work has no historical plan witness: %s revision %d: %v", recordedSubscription, recordedRevision, err)
	}
	for _, denied := range []protocol.Operation{beforeActivation, x.op(t, lifecycleDBNow(t, x.legalFixtureState))} {
		ack, err := x.process(t, denied)
		if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != orgapi.EntitlementRequired || len(ack.CanonicalBytes()) != 0 {
			t.Fatalf("work outside the effective subscription was not refused: %v", err)
		}
		x.untouched(t, denied)
	}
	if x.authority(t) != before {
		t.Fatal("subscription loss or operation intake changed organization authority")
	}
}

func stateCapabilityRevision(t *testing.T, x stateFixture) int64 {
	t.Helper()
	var revision int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_capabilities WHERE org_id=$1 AND capability='organizations'`, x.scope.OrgID().String()).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

// Twelve identical deliveries during a suspension leave one claim, decision
// and held fact (V-IDEM; replay never adds a cost).
func TestStateIntakeConcurrentIdenticalDeliveriesKeepOneDecision(t *testing.T) {
	x := stateIntakeFixture(t)
	op := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	x.suspend(t)
	acks, errs := make([]protocol.Acknowledgement, 12), make([]error, 12)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range acks {
		wg.Go(func() {
			<-start
			acks[i], errs[i] = x.state.Process(t.Context(), x.source, op, protocol.DefaultLimits(), x.handler)
		})
	}
	close(start)
	wg.Wait()
	var first protocol.Acknowledgement
	for i, err := range errs {
		if errors.Is(err, syncapp.ErrSourceDenied) {
			t.Fatal("concurrent state delivery withheld the source", err)
		}
		if err == nil {
			if first.CanonicalBytes() == nil {
				first = acks[i]
			} else if !bytes.Equal(first.CanonicalBytes(), acks[i].CanonicalBytes()) {
				t.Fatal("concurrent state deliveries produced different acknowledgements")
			}
		}
	}
	if first.CanonicalBytes() == nil {
		t.Fatal("no concurrent state delivery completed")
	}
	x.heldState(t, op, first, nil, syncapp.StateSuspension, syncapp.StateGraceCode)
}

// The operator suspension and a delivery race in root-lock order. Suspension
// first: the delivery declared before it is held; delivery first: it applies.
func TestStateIntakeRacesTheSuspensionInBothOrders(t *testing.T) {
	for _, suspendFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "suspend-first-held", false: "intake-first-applied"}[suspendFirst], func(t *testing.T) {
			x := stateIntakeFixture(t)
			org := x.scope.OrgID().String()
			op := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
			revision := x.ops.root(t).Revision
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			holder, err := x.f.Runtime.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback(ctx)
			if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
				t.Fatal(err)
			}
			if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
				t.Fatal(err)
			}
			waiting := func(n int) {
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				for {
					var count int
					if err := x.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock'`, x.f.RuntimeRole).Scan(&count); err != nil {
						t.Fatal(err)
					}
					if count >= n {
						return
					}
					select {
					case <-ticker.C:
					case <-ctx.Done():
						t.Fatal("no lock wait observed", n)
					}
				}
			}
			suspended := make(chan error, 1)
			type result struct {
				ack protocol.Acknowledgement
				err error
			}
			intake := make(chan result, 1)
			startSuspend := func() {
				go func() { _, err := x.ops.change(t, app.SuspendOrganization, revision, ""); suspended <- err }()
			}
			startIntake := func() {
				go func() {
					ack, err := x.state.Process(ctx, x.source, op, protocol.DefaultLimits(), x.handler)
					intake <- result{ack, err}
				}()
			}
			if suspendFirst {
				startSuspend()
				waiting(1)
				startIntake()
			} else {
				startIntake()
				waiting(1)
				startSuspend()
			}
			waiting(2)
			if err := holder.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-suspended; err != nil {
				t.Fatal("concurrent suspension failed", err)
			}
			got := <-intake
			if suspendFirst {
				x.heldState(t, op, got.ack, got.err, syncapp.StateSuspension, syncapp.StateGraceCode)
				return
			}
			if got.err != nil || got.ack.Result() != protocol.Accepted {
				t.Fatal("delivery committed before the suspension did not apply", got.err)
			}
			if x.ops.root(t).State != orgd.Suspended {
				t.Fatal("suspension did not commit after the delivery")
			}
		})
	}
}

// Late codes stay with OFFL-009; state codes exist only with their row, the
// row is append-only and re-derived, and another organization's suspension
// never decides this organization's work.
func TestStateIntakeSchemaGuardsAndForeignHistory(t *testing.T) {
	x := stateIntakeFixture(t)
	org := x.scope.OrgID().String()
	proof, _ := x.handler.(syncapp.ReviewProofProvider)
	for _, code := range []string{syncapp.StateSuspendedCode} {
		op := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
		forged := lateForgedOutcome{Handler: x.handler, proof: proof.ReviewProof(), hold: func(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation) (syncapp.Outcome, error) {
			return syncapp.Outcome{Result: protocol.Rejected, Code: code}, nil
		}}
		if ack, err := x.intake.Process(t.Context(), x.source, op, protocol.DefaultLimits(), forged); err == nil || len(ack.CanonicalBytes()) != 0 {
			t.Fatal("the owner path published a state code without its decision", code, err)
		}
		x.untouched(t, op)
	}
	// Another organization's suspension never decides this one.
	other := legalFixtureFor(t, x.f, x.onboarding, x.actor, x.flags)
	otherOps := suspensionOnFixture(t, other)
	if _, err := otherOps.change(t, app.SuspendOrganization, otherOps.root(t).Revision, ""); err != nil {
		t.Fatal("foreign suspension", err)
	}
	local := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
	if ack, err := x.process(t, local); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("another organization's suspension decided this one", err)
	}
	started := x.suspend(t)
	held := x.op(t, started.Add(-time.Nanosecond))
	ack, err := x.process(t, held)
	x.heldState(t, held, ack, err, syncapp.StateSuspension, syncapp.StateGraceCode)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_sync_state_intakes TO "+role); err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT TRUNCATE ON org_held_fuelings TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{
		`UPDATE org_sync_state_intakes SET code='ORG_SUSPENDED'`,
		`DELETE FROM org_sync_state_intakes`,
		`TRUNCATE org_sync_state_intakes,org_held_fuelings`,
	} {
		lateSQLCode(t, unitSchemaExec(t, x.legalFixtureState, verb), "55000")
	}
	insert := func(op protocol.Operation, condition, disposition, result, code string, declared time.Time) error {
		return database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			v := op.Snapshot()
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest) VALUES($1,$2,$3,$4,$5,$6,$7,1,'meter_reading',$8,$9)`, org, op.ID(), v.OwnerAccountID, v.APIOrigin, v.MembershipID, v.EnrollmentID, v.Type, v.Entity.ID, v.PayloadDigest); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `INSERT INTO org_sync_state_intakes(org_id,operation_id,condition,disposition,declared_at,declared_submicro,received_at,result,code) VALUES($1,$2,$3,$4,$5,0,clock_timestamp(),$6,$7)`, org, op.ID(), condition, disposition, declared, result, code)
			return err
		})
	}
	for name, attempt := range map[string]func() error{
		"refusal-declared-before": func() error {
			return insert(x.op(t, started), "SUSPENSION", "REFUSE", protocol.Rejected, "ORG_SUSPENDED", started.Add(-time.Hour))
		},
		"grace-declared-after": func() error {
			return insert(x.op(t, started), "SUSPENSION", "HOLD", protocol.AcceptedForReview, "ORG_SUSPENDED_GRACE", started.Add(time.Hour))
		},
		"plan-without-capability": func() error {
			return insert(x.op(t, started), "PLAN", "HOLD", protocol.AcceptedForReview, "PLAN_CHANGED", started.Add(-time.Hour))
		},
		"plan-while-suspended-hold": func() error {
			return insert(x.op(t, started), "SUSPENSION", "HOLD", protocol.AcceptedForReview, "PLAN_CHANGED", started.Add(-time.Hour))
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := attempt()
			var pe *pgconn.PgError
			if !errors.As(err, &pe) || pe.Code != "23514" {
				t.Fatal("fabricated state decision accepted", err)
			}
		})
	}
	t.Run("consistent-row-without-acknowledgement", func(t *testing.T) {
		op := x.op(t, started.Add(time.Hour))
		err := insert(op, "SUSPENSION", "REFUSE", protocol.Rejected, "ORG_SUSPENDED", started.Add(time.Hour))
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "23503" && pe.Code != "23514" {
			t.Fatal("a state decision committed without its acknowledgement", err)
		}
		x.untouched(t, op)
	})
}

// lateRedeclared seals another digest under an existing operation identifier.
func lateRedeclared(t *testing.T, x lateFixture, original protocol.Operation, declared time.Time) protocol.Operation {
	t.Helper()
	in, _ := meterObservedCommand(t, x.installed, declared, 9).Snapshot()
	return syncHTTPOperation(t, x.deviceFixtureState, x.source.EnrollmentID, func(raw map[string]any) {
		raw["operation_id"] = original.ID()
		raw["type"] = original.Snapshot().Type
		raw["entity"] = map[string]any{"type": "meter_reading", "id": original.Snapshot().Entity.ID, "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(in.Declared.OccurredAt)
		raw["payload"] = map[string]any{"asset_id": in.AssetID, "meter_id": in.MeterID, "segment_id": in.SegmentID, "kind": "ODOMETER", "value": "9", "method": "TYPED"}
	})
}

// The SQL mirror of ORG-009 AssessSuspension agrees with the Go domain for a
// generated matrix of controlled instants (no fixture time is invented).
func TestStateIntakeSQLSuspensionMirrorsAssessSuspension(t *testing.T) {
	x := lateIntakeFixture(t)
	start := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	grace := 7 * 24 * time.Hour
	instants := []time.Time{start.Add(-time.Hour), start.Add(-time.Microsecond), start, start.Add(time.Microsecond), end.Add(-time.Microsecond), end, end.Add(time.Minute), start.Add(grace), start.Add(grace + time.Microsecond)}
	cases := 0
	for _, open := range []bool{true, false} {
		for _, declared := range instants {
			for _, received := range instants {
				period := orgd.SuspensionPeriod{StartedAt: start}
				ended := []any{nil}
				state := orgd.Suspended
				if !open {
					period.EndedAt = end
					ended = []any{end}
					state = orgd.Active
					if received.Before(end) {
						continue
					}
				}
				if received.Before(start) {
					continue
				}
				want, err := orgd.AssessSuspension(state, []orgd.SuspensionPeriod{period}, orgd.SuspensionIntakeFacts{DeclaredAt: declared, ReceivedAt: received})
				if err != nil {
					t.Fatal(err)
				}
				var got string
				if err := x.f.Admin.QueryRow(t.Context(), `SELECT org_sync_suspension_disposition_of(ARRAY[$1::timestamptz],ARRAY[$2::timestamptz],$3,$4)`, start, ended[0], declared, received).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != string(want.Disposition) {
					t.Fatalf("SQL suspension mirror %s, domain %s (open=%v declared=%v received=%v)", got, want.Disposition, open, declared, received)
				}
				cases++
			}
		}
	}
	if cases < 60 {
		t.Fatal("suspension mirror matrix too small", cases)
	}
}
