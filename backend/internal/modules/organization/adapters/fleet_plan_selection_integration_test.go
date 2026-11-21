//go:build integration

package adapters_test

import (
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func prepareFleetDowngrade(t *testing.T, fx legalFixtureState, priorCeiling, newCeiling int, at time.Time) (string, func() int64) {
	t.Helper()
	ctx := t.Context()
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet','larger',1,'Synthetic larger fleet',ARRAY['organizations'],$1),
      ('fleet','reduced',1,'Synthetic reduced fleet',ARRAY['organizations'],$2)`, priorCeiling, newCeiling); err != nil {
		t.Fatal(err)
	}
	offer, sub, org := onboardingID(t), onboardingID(t), fx.scope.OrgID().String()
	prior, term := billing.Period{Start: at.AddDate(0, -1, 0), End: at}, billing.Period{Start: at, End: at.AddDate(0, 1, 0)}
	ledger := billpg.SubscriptionLedger{Capabilities: known}
	priorPlan := billing.PlanKey{Product: "fleet", Plan: "larger", Revision: 1}
	opening := billing.LifecycleEvent{
		Scope: fx.scope, SubscriptionID: sub, Kind: billing.ConfirmSettlement, Source: billing.VerifiedBilling,
		At: prior.Start, Reference: "synthetic-prior-paid-term", Period: prior,
	}
	proof := fx.f.PrepareSubscriptionOpening(t, priorPlan, opening)
	if err := database.WithOrgTx(ctx, fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, err := ledger.ApplyTx(ctx, tx, priorPlan, opening); err != nil {
			return err
		}
		return proof(tx)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_billing_offers
(id,product_code,plan_code,plan_revision,channel,environment,merchant,provider_offer_ref,amount_centavos,term,approval_ref)
VALUES($1,'fleet','reduced',1,'stripe_card','test','synthetic-fleet','synthetic-lower-price',1723,'P1M','synthetic-vehicle-downgrade')`, offer); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_billing_charge_bindings
(org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end)
VALUES($1,'stripe','test','synthetic-fleet','synthetic-fleet-invoice',$2,$3,'synthetic-fleet-customer',
       'synthetic-fleet-contract',$4,$5)`, org, sub, offer, term.Start, term.End); err != nil {
		t.Fatal(err)
	}
	notice := billing.BillingNotice{Provider: payment.ProviderStripe, Environment: payment.EnvironmentTest, Merchant: "synthetic-fleet",
		EventID: "synthetic-fleet-downgrade", PayloadHash: strings.Repeat("d", 64), ResourceRef: "synthetic-fleet-invoice"}
	canonical := billing.CanonicalBillingFact{Notice: notice, Scope: fx.scope, SubscriptionID: sub,
		CustomerRef: "synthetic-fleet-customer", ContractRef: "synthetic-fleet-contract", ProviderOfferRef: "synthetic-lower-price",
		Channel: payment.ChannelCard, ResourceRef: notice.ResourceRef, ChargeRef: "synthetic-fleet-charge", Outcome: billing.BillingSettled,
		At: at, Gross: payment.BRL(1723), PaidTerm: term}
	intake := billpg.BillingIntake{Pool: fx.f.Runtime, Ledger: ledger, Now: time.Now}
	return sub, func() int64 {
		t.Helper()
		receipt, err := intake.RecordSettlement(ctx, notice, canonical)
		if err != nil || receipt.Replayed || receipt.Revision != 2 {
			t.Fatalf("synthetic verified downgrade settlement was unavailable: %v", err)
		}
		return receipt.Revision
	}
}

func TestPaidFleetDowngradeKeepsTenChosenVehiclesThroughCompanyHTTP(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	trip := tripFixture(t)
	fx := trip.deviceFixtureState
	ctx, org := t.Context(), fx.scope.OrgID().String()
	legacy, meters := trip.assets, trip.meters
	proof := fleetActor(fx.legalFixtureState)
	var unit string
	if err := fx.f.Admin.QueryRow(ctx, `SELECT id::text FROM org_units WHERE org_id=$1`, org).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	const originalCount, newLimit = 14, 10
	running := trip.startOp(t, trip.syncFixtureState, trip.declaredNow(t), nil)
	ack, err := trip.process(t, trip.syncFixtureState, running)
	trip.accepted(t, running, ack, err, 1, 1)
	current, err := legacy.ReadAsset(ctx, fx.scope, proof, fx.member, trip.vehicle.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	all := []string{current.Ref.ID().Value()}
	original := map[string]int64{all[0]: current.Revision}
	for i := 1; i < originalCount; i++ {
		in := fleetCreateInput()
		in.Profile.Kind, in.Profile.UsageBasis = fleet.Vehicle, fleet.Distance
		in.Profile.FleetNumber, in.UnitID = fmt.Sprintf("DOWNGRADE-%d", i+1), unit
		in.Profile.Tanks = []fleet.Tank{tank}
		draft, _, err := legacy.WriteAsset(ctx, fx.scope, proof, fx.member, onboardingID(t), fleetCommand(t, in))
		if err != nil {
			t.Fatal(err)
		}
		meter, _, err := meterInstall(t, fx.legalFixtureState, meters,
			meterInstallCommand(t, meterInstallInput(t, draft.AssetID, lifecycleDBNow(t, fx.legalFixtureState).Add(-time.Hour))))
		if err != nil {
			t.Fatal(err)
		}
		saved, _, err := fleetWrite(t, fx.legalFixtureState, legacy, fleetCommand(t, fleetapp.AssetWriteInput{
			Kind: fleetapp.ActivateAsset, AssetID: draft.AssetID, ExpectedRevision: meter.AssetRevision,
		}))
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, saved.AssetID)
		original[saved.AssetID] = saved.Revision
	}
	sub, settle := prepareFleetDowngrade(t, fx.legalFixtureState, 30, newLimit, lifecycleDBNow(t, fx.legalFixtureState))
	revision := settle()
	router := chi.NewRouter()
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{
		Enabled: true, BillingEnabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{79}, 32), Pool: fx.f.Runtime, Accounts: &accounts,
	}); err != nil {
		t.Fatal(err)
	}
	owner, finance := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, finance, "FINANCE", "ORG", "ACTIVE")
	base := "/v1/orgs/" + org + "/subscriptions"
	read := func() map[string]any {
		t.Helper()
		result := matrixCall(t, fx, router, owner, base+"/plan/read", map[string]any{}, 200, "", "", nil)
		return result["plans"].([]any)[0].(map[string]any)
	}
	basis := read()["vehicle_selection"].(map[string]any)
	if basis["subscription_id"] != sub || basis["revision"] != float64(revision) || basis["active"] != float64(originalCount) || basis["ceiling"] != float64(newLimit) {
		t.Fatal("plan overview did not expose the exact allowed selection")
	}
	kept := append([]string{}, all[:newLimit]...)
	op := onboardingID(t)
	path := base + "/vehicles/select"
	payload := map[string]any{"schema_version": 1, "operation_id": op, "subscription_id": sub, "revision": revision, "keep_active": kept}
	matrixCall(t, fx, router, finance, path, payload, 403, "FORBIDDEN", "subs.plan.manage", nil)
	payload["revision"] = revision + 1
	matrixCall(t, fx, router, owner, path, payload, 404, "NOT_FOUND", "", nil)
	payload["revision"] = revision
	wrong := append([]string{}, kept...)
	wrong[0] = onboardingID(t)
	payload["keep_active"] = wrong
	matrixCall(t, fx, router, owner, path, payload, 409, "CONFLICT", "", nil)
	wouldArchiveOpenTrip := append([]string{}, all[1:newLimit+1]...)
	payload["keep_active"] = wouldArchiveOpenTrip
	if status := matrixCallStatus(t, fx, router, owner, path, payload); status != http.StatusConflict {
		t.Fatalf("selection did not protect open trip: %d", status)
	}
	var activeBefore, changesBefore int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM org_assets WHERE org_id=$1 AND kind='VEHICLE' AND state='ACTIVE'),
 (SELECT count(*) FROM org_billing_vehicle_plan_selections WHERE org_id=$1)+
 (SELECT count(*) FROM org_asset_lifecycle_events WHERE org_id=$1 AND command='DEACTIVATE')`, org).Scan(&activeBefore, &changesBefore); err != nil || activeBefore != originalCount || changesBefore != 0 {
		t.Fatalf("selection with open work committed a partial result: active=%d changes=%d error=%v", activeBefore, changesBefore, err)
	}
	payload["keep_active"] = kept
	created := matrixCall(t, fx, router, owner, path, payload, http.StatusOK, "", "", nil)
	if created["replayed"] != false || len(created["archived"].([]any)) != originalCount-newLimit {
		t.Fatal("vehicle selection did not archive exactly four vehicles")
	}
	slices.Reverse(kept)
	again := matrixCall(t, fx, router, owner, path, payload, http.StatusOK, "", "", nil)
	if again["replayed"] != true || fmt.Sprint(again["archived"]) != fmt.Sprint(created["archived"]) {
		t.Fatal("reordered matching retry did not return the original selection")
	}
	wrong = append([]string{}, kept...)
	wrong[0] = all[newLimit]
	payload["keep_active"] = wrong
	matrixCall(t, fx, router, owner, path, payload, 409, "OPERATION_ID_REUSED", "", nil)
	if read()["vehicle_selection"] != nil {
		t.Fatal("resolved overage still requested an action")
	}
	for _, asset := range all {
		row, err := legacy.ReadAsset(ctx, fx.scope, proof, fx.member, asset)
		wantState, wantRev := fleet.Active, original[asset]
		if !slices.Contains(kept, asset) {
			wantState, wantRev = fleet.Inactive, wantRev+1
		}
		if err != nil || row.State != wantState || row.Revision != wantRev || row.Disposal != nil {
			t.Fatalf("selection changed the wrong vehicle or invented a disposal: %v", err)
		}
	}
	var selected, archive, histories int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM org_billing_vehicle_plan_selections WHERE org_id=$1),
 (SELECT count(*) FROM org_billing_vehicle_plan_archives WHERE org_id=$1),
 (SELECT count(*) FROM org_asset_lifecycle_events WHERE org_id=$1 AND command='DEACTIVATE')`, org).Scan(&selected, &archive, &histories); err != nil || selected != 1 || archive != 4 || histories != 4 {
		t.Fatalf("fleet archive evidence diverged: selections=%d archives=%d history=%d error=%v", selected, archive, histories, err)
	}
	matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fleet/assets/"+all[newLimit]+"/reactivate",
		map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": original[all[newLimit]] + 1}, 403, "LIMIT_REACHED", "", nil)
}
