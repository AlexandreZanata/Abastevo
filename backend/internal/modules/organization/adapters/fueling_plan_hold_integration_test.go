//go:build integration

package adapters_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestSignedFuelingPlanLossKeepsEarlierFactsForReview(t *testing.T) {
	x := fuelingFixture(t)
	ctx := t.Context()
	org := x.scope.OrgID().String()
	x.f.GrantStateIntake(t)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog TO "+role+
		"; GRANT SELECT,INSERT ON org_billing_subscription_events,org_sync_review_items,org_held_fuelings TO "+role+
		"; GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet','synthetic',1,'Synthetic fleet',ARRAY['organizations'],10)`); err != nil {
		t.Fatal(err)
	}
	ledger := billpg.SubscriptionLedger{Capabilities: known}
	plan := billing.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	subscription := onboardingID(t)
	apply := func(event billing.LifecycleEvent, want billing.SubscriptionState) {
		t.Helper()
		event.Scope, event.SubscriptionID = x.scope, subscription
		err := database.WithOrgTx(ctx, x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			result, err := ledger.ApplyTx(ctx, tx, plan, event)
			if err == nil && result.To != want {
				t.Fatalf("synthetic plan transitioned to %s, want %s", result.To, want)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	activeAt := lifecycleDBNow(t, x.legalFixtureState)
	apply(billing.LifecycleEvent{
		Kind: billing.ConfirmSettlement, Source: billing.VerifiedBilling, At: activeAt,
		Reference: "synthetic-paid-term", Period: billing.Period{Start: activeAt, End: activeAt.Add(time.Hour)},
	}, billing.Active)
	earlier := x.op(t, x.syncFixtureState, x.declaredNow(t), nil)
	foreign := x.op(t, x.syncFixtureState, x.declaredNow(t), func(_, payload map[string]any) {
		payload["asset_id"] = onboardingID(t)
	})
	beforePlan := x.op(t, x.syncFixtureState, activeAt.Add(-time.Second), nil)
	apply(billing.LifecycleEvent{
		Kind: billing.SuspendAccess, Source: billing.ReviewedOperator, ExpectedRevision: 1, At: lifecycleDBNow(t, x.legalFixtureState),
		Reference: "synthetic-reviewed-plan-suspension",
	}, billing.Suspended)

	router := chi.NewRouter()
	accounts := *x.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(x.f.Runtime), syncHTTPClock{}
	if err := enterprisesuite.Register(router, enterprisesuite.Options{
		Enabled: true, BillingEnabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{83}, 32),
		Pool: x.f.Runtime, Accounts: &accounts,
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(router)
	t.Cleanup(server.Close)
	server.Client().Timeout = 8 * time.Second
	fx := meterIntakeHTTPFixture{syncFixtureState: x.syncFixtureState, server: server, path: "/v1/orgs/" + org + "/sync/operations"}
	acks, wire, _ := lateHTTPBatch(t, fx, http.StatusOK, earlier)
	if acks[0].Result() != protocol.AcceptedForReview || acks[0].Code() != syncapp.StatePlanCode || acks[0].Snapshot().ReviewItemID == "" {
		t.Fatalf("eligible fueling was not held: %s %s", acks[0].Result(), acks[0].Code())
	}
	_, replay, _ := lateHTTPBatch(t, fx, http.StatusOK, earlier)
	if !bytes.Equal(wire, replay) {
		t.Fatal("the exact held receipt changed after replay")
	}
	_, _, code := lateHTTPBatch(t, fx, http.StatusNotFound, foreign)
	if code != "NOT_FOUND" {
		t.Fatalf("unknown vehicle returned %s", code)
	}
	afterPlan := x.op(t, x.syncFixtureState, x.declaredNow(t), nil)
	for _, op := range []protocol.Operation{beforePlan, afterPlan} {
		if _, _, code := lateHTTPBatch(t, fx, http.StatusForbidden, op); code != "ENTITLEMENT_REQUIRED" {
			t.Fatalf("fueling outside the eligible term returned %s", code)
		}
	}
	var held, reviews, actualFacts int
	var exact bool
	err = x.f.Admin.QueryRow(ctx, `SELECT
    (SELECT count(*) FROM org_held_fuelings WHERE org_id=$1),
    (SELECT count(*) FROM org_sync_review_items WHERE org_id=$1),
    (SELECT count(*) FROM org_fuelings WHERE org_id=$1)+(SELECT count(*) FROM org_expenses WHERE org_id=$1)
        +(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND origin='FUELING'),
    EXISTS(SELECT 1 FROM org_held_fuelings h JOIN org_sync_state_intakes s
        ON (s.org_id,s.operation_id)=(h.org_id,h.operation_id)
        WHERE h.org_id=$1 AND h.operation_id=$2 AND h.canonical_operation=$3
          AND s.subscription_id=$4 AND s.subscription_revision=1 AND h.review_item_id=$5)`,
		org, earlier.ID(), earlier.CanonicalBytes(), subscription, acks[0].Snapshot().ReviewItemID).Scan(&held, &reviews, &actualFacts, &exact)
	if err != nil || held != 1 || reviews != 1 || actualFacts != 0 || !exact {
		t.Fatalf("unexpected held state: held=%d reviews=%d applied=%d exact=%v err=%v", held, reviews, actualFacts, exact, err)
	}
}
