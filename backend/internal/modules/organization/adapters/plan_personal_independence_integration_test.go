//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	paypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	pay "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	subpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	subapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestCompanyPlanLimitAndHoldKeepPaidPersonalPremiumIndependent(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	x := fuelingFixture(t)
	fx := x.deviceFixtureState
	ctx, org := t.Context(), x.scope.OrgID().String()
	router := chi.NewRouter()
	accounts := *x.accounts
	accounts.Store = accountpg.NewPGStore(x.f.Runtime)
	o := enterprisesuite.Options{Enabled: true, BillingEnabled: true, Origin: deviceTestOrigin,
		CursorSecret: bytes.Repeat([]byte{80}, 32), Pool: x.f.Runtime, Accounts: &accounts, ExportStorage: exportStorageConfig(t)}
	if err := enterprisesuite.Register(router, o); err != nil {
		t.Fatal(err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	driverDevice := x.driver(t)
	x.assign(t, driverDevice.person.employee, x.vehicle.AssetID, fleet.PrimaryAssignment, nil, nil)
	driver := incidentLogin(t, driverDevice.person, router)
	base := "/v1/orgs/" + org
	personalStore := &subapp.Service{Store: subpg.NewStore(x.f.Runtime)}
	personalProof := func(who matrixPerson) subapp.Principal {
		t.Helper()
		var family string
		if err := x.f.Admin.QueryRow(ctx, `SELECT family_id::text FROM company_web_sessions WHERE id=$1 AND account_id=$2`, who.session, who.account).Scan(&family); err != nil {
			t.Fatal(err)
		}
		return subapp.Principal{AccountID: who.account, FamilyID: family}
	}
	driverProof := subapp.Principal{AccountID: driver.account, FamilyID: driverDevice.person.auth.Session.FamilyID}
	ownerProof := personalProof(owner)
	checkPersonal := func(proof subapp.Principal, premium bool) {
		t.Helper()
		effective, err := personalStore.Status(ctx, proof)
		if err != nil || slices.Contains(effective.Capabilities, "cloud.sync") != premium || !effective.RetainedDataAccess {
			t.Fatalf("personal rights diverged: Premium=%t, status=%+v err=%v", premium, effective, err)
		}
	}
	snapshot := func(table string) string {
		t.Helper()
		var state string
		statement := `SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY to_jsonb(s)::text)::text,'[]') FROM ` + pgx.Identifier{table}.Sanitize() + ` s WHERE org_id=$1`
		if err := x.f.Admin.QueryRow(ctx, statement, org).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	roles := snapshot("org_role_assignments")
	payments := paypg.NewStore(x.f.Runtime)
	checkPersonal(driverProof, false)
	createPersonalPremiumFixture(t, payments, driver.account)
	ownerOrder := createPersonalPremiumFixture(t, payments, owner.account)
	checkPersonal(driverProof, true)
	checkPersonal(ownerProof, true)
	if snapshot("org_role_assignments") != roles || snapshot("org_billing_subscription_events") != "[]" {
		t.Fatal("personal Premium created company authority or billing history")
	}
	matrixCall(t, fx, router, driver, base+"/subscriptions/plan/read", map[string]any{}, 403, "FORBIDDEN", "subs.plan.read", nil)

	known, err := capability.NewCatalog(enterprisesuite.CapabilityCodes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet','acceptance',1,'Synthetic fleet acceptance',ARRAY['organizations'],10)`); err != nil {
		t.Fatal(err)
	}
	plans := billpg.EntitlementStore{Pool: x.f.Runtime, Capabilities: known, Now: time.Now}
	ledger := billpg.SubscriptionLedger{Capabilities: known}
	key := billing.PlanKey{Product: "fleet", Plan: "acceptance", Revision: 1}
	orgSubscription := onboardingID(t)
	change := func(event billing.LifecycleEvent, expected billing.SubscriptionState) {
		t.Helper()
		event.Scope, event.SubscriptionID = x.scope, orgSubscription
		if err := database.WithOrgTx(ctx, x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			applied, err := ledger.ApplyTx(ctx, tx, key, event)
			if err == nil && applied.To != expected {
				t.Fatalf("organization transition went to %s, expected %s", applied.To, expected)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	at := lifecycleDBNow(t, x.legalFixtureState)
	change(billing.LifecycleEvent{Kind: billing.ConfirmSettlement, Source: billing.VerifiedBilling, At: at,
		Reference: "synthetic-fleet-acceptance", Period: billing.Period{Start: at, End: at.Add(time.Hour)}}, billing.Active)
	companyTerm := snapshot("org_billing_subscription_events")
	if err := payments.RevokeGrant(ctx, ownerOrder, lifecycleDBNow(t, x.legalFixtureState)); err != nil {
		t.Fatal(err)
	}
	checkPersonal(ownerProof, false)
	checkPersonal(driverProof, true)
	if snapshot("org_billing_subscription_events") != companyTerm || snapshot("org_role_assignments") != roles {
		t.Fatal("revoking personal Premium changed the company's paid term or roles")
	}
	panel := matrixCall(t, fx, router, owner, base+"/subscriptions/plan/read", map[string]any{}, 200, "", "", nil)
	raw, err := json.Marshal(panel)
	if err != nil || bytes.Contains(raw, []byte("cloud.sync")) || bytes.Contains(raw, []byte(ownerOrder)) ||
		!bytes.Contains(raw, []byte("Synthetic fleet acceptance")) {
		t.Fatal("the company panel mixed personal payment data or omitted its company plan")
	}

	x.flags.PlanWrites, x.assets.VehicleSlots = plans, plans
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 11; i++ {
		in := fleetCreateInput()
		in.Profile.Kind, in.Profile.UsageBasis, in.UnitID = fleet.Vehicle, fleet.Distance, x.unit
		in.Profile.FleetNumber, in.Profile.Tanks = fmt.Sprintf("ACCEPTANCE-%d", i), []fleet.Tank{tank}
		draft, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, in))
		if err != nil {
			t.Fatal(err)
		}
		meter, _, err := meterInstall(t, x.legalFixtureState, x.meters, meterInstallCommand(t, meterInstallInput(t, draft.AssetID, at.Add(-time.Hour))))
		if err != nil {
			t.Fatal(err)
		}
		if i == 11 {
			before := snapshot("org_assets")
			matrixCall(t, fx, router, owner, base+"/fleet/assets/"+draft.AssetID+"/activate", map[string]any{
				"schema_version": 1, "operation_id": onboardingID(t), "revision": meter.AssetRevision,
			}, 403, "LIMIT_REACHED", "", nil)
			if snapshot("org_assets") != before {
				t.Fatal("the eleventh vehicle changed the ten active vehicles")
			}
			continue
		}
		if _, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fleetapp.AssetWriteInput{
			Kind: fleetapp.ActivateAsset, AssetID: draft.AssetID, ExpectedRevision: meter.AssetRevision,
		})); err != nil {
			t.Fatal(err)
		}
	}
	beforeHold := snapshot("org_assets")
	assetWork := func() int64 {
		t.Helper()
		var operations int64
		if err := database.WithOrgTx(ctx, x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT (org_asset_current_usage($1,$2)->>'operations')::bigint`, org, x.vehicle.AssetID).Scan(&operations)
		}); err != nil {
			t.Fatalf("vehicle usage is unavailable: %v", err)
		}
		return operations
	}
	workBeforeHold := assetWork()
	fuelingsBeforeHold, expensesBeforeHold := snapshot("org_fuelings"), snapshot("org_expenses")
	earlier := x.op(t, driverDevice, x.declaredNow(t), nil)
	change(billing.LifecycleEvent{Kind: billing.SuspendAccess, Source: billing.ReviewedOperator, ExpectedRevision: 1,
		At: lifecycleDBNow(t, x.legalFixtureState), Reference: "synthetic-reviewed-hold"}, billing.Suspended)
	matrixCall(t, fx, router, owner, base+"/fleet/assets/create", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic held create",
		"fleet_number": "F8-HOLD", "usage_basis": "HOURS", "category": map[string]any{"new_label": "Held create"},
	}, 403, "ENTITLEMENT_REQUIRED", "", nil)
	matrixCall(t, fx, router, owner, base+"/fleet/assets/list", map[string]any{}, 200, "", "", nil)
	server := httptest.NewTLSServer(router)
	defer server.Close()
	server.Client().Timeout = 8 * time.Second
	httpFX := meterIntakeHTTPFixture{syncFixtureState: driverDevice, server: server, path: base + "/sync/operations"}
	acks, first, _ := lateHTTPBatch(t, httpFX, http.StatusOK, earlier)
	if acks[0].Result() != protocol.AcceptedForReview || acks[0].Code() != "PLAN_CHANGED" {
		t.Fatalf("earlier signed work was not held after the plan changed: %s", acks[0].Code())
	}
	_, again, _ := lateHTTPBatch(t, httpFX, http.StatusOK, earlier)
	if !bytes.Equal(first, again) {
		t.Fatal("the plan hold retry changed the receipt")
	}
	if got := assetWork(); got != workBeforeHold+1 {
		t.Fatalf("the replayed plan hold changed vehicle work from %d to %d, want %d", workBeforeHold, got, workBeforeHold+1)
	}
	if snapshot("org_fuelings") != fuelingsBeforeHold || snapshot("org_expenses") != expensesBeforeHold {
		t.Fatal("the held vehicle work posted a fueling or expense")
	}
	retainedCompanyExport(t, fx, router, o, owner, base)
	if snapshot("org_role_assignments") != roles || snapshot("org_assets") != beforeHold {
		t.Fatal("plan hold rewrote a company role or vehicle")
	}
	checkPersonal(ownerProof, false)
	checkPersonal(driverProof, true)
	before := driverDevice.person.auth
	if value, err := accounts.ValidateAccess(ctx, before.Session.FamilyID, before.Session.AccessToken); err != nil || value != driver.account {
		t.Fatalf("organization plan changed the driver's personal account proof: %v", err)
	}
}

func retainedCompanyExport(t *testing.T, fx deviceFixtureState, router chi.Router, o enterprisesuite.Options, owner matrixPerson, base string) {
	t.Helper()
	exports := base + "/reports/exports"
	result := matrixCall(t, fx, router, owner, exports+"/request", map[string]any{"schema_version": 1, "operation_id": onboardingID(t),
		"kind": "org.full_export.zip", "parameters": map[string]any{}}, 202, "", "", nil)
	id := result["export"].(map[string]any)["export_id"].(string)
	handlers, err := enterprisesuite.ExportHandlers(o)
	if err != nil {
		t.Fatal(err)
	}
	worker := jobs.Dispatcher{Queue: jobs.NewQueue(fx.f.Runtime), Kind: exportpg.BuildKind,
		Handlers: make(map[string]jobs.Handler), WorkerID: "company-plan-acceptance"}
	for _, handler := range handlers {
		worker.Handlers[handler.Kind()] = handler
	}
	if found, err := worker.RunOnce(t.Context()); err != nil || !found {
		t.Fatalf("the private export job was unavailable: found=%t err=%v", found, err)
	}
	grant := matrixCall(t, fx, router, owner, exports+"/"+id+"/access", map[string]any{}, 200, "", "", nil)["capability"].(string)
	result = matrixCall(t, fx, router, owner, exports+"/"+id+"/content", map[string]any{"capability": grant}, 200, "", "", nil)
	contents, err := base64.StdEncoding.DecodeString(result["content_base64"].(string))
	if err != nil || !bytes.HasPrefix(contents, []byte("PK")) {
		t.Fatal("an authorized company member could not export after the plan hold")
	}
}

func createPersonalPremiumFixture(t *testing.T, store *paypg.Store, account string) string {
	t.Helper()
	ctx := t.Context()
	const merchant = "synthetic-personal-merchant"
	const amount = 1234
	offer := pay.Offer{ID: "premium.monthly", Family: "premium", Term: pay.TermMonthly, Price: pay.BRL(amount),
		Capabilities: pay.PremiumCapabilityList(), Channels: map[pay.Channel]bool{pay.ChannelPixManual: true}, SaleEnabled: true}
	catalog, err := pay.NewCatalog("pay-catalog-v1", offer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(pay.QuoteRequest{OfferID: offer.ID, Channel: pay.ChannelPixManual}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := pay.NewOrder(onboardingID(t), account, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	if err := store.MapCustomer(ctx, paypg.CustomerMapping{AccountID: account, Provider: pay.ProviderAsaas,
		Environment: pay.EnvironmentTest, Merchant: merchant, RemoteCustomerID: "synthetic-customer-" + account}); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(ctx, paypg.ClaimAttemptInput{AccountID: account, Provider: pay.ProviderAsaas, Environment: pay.EnvironmentTest,
		Merchant: merchant, OperationKey: "personal-premium-" + account, Kind: "create_attempt", PayloadHash: strings.Repeat("b", 64),
		OrderID: order.ID, LeaseUntil: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordSettlement(ctx, paypg.SettlementInput{Offer: catalog.Offers[offer.ID], Settlement: pay.Settlement{
		ID: onboardingID(t), OrderID: order.ID, AttemptID: claim.AttemptID, AccountID: account, OfferID: order.OfferID,
		Channel: pay.ChannelPixManual, Provider: pay.ProviderAsaas, Environment: pay.EnvironmentTest, MerchantID: merchant,
		RemotePaymentID: "synthetic-payment-" + order.ID, ProviderStatus: "RECEIVED", Amount: pay.BRL(amount),
		Status: pay.SettlementSettled, Authority: pay.AuthorityProviderVerified, SettledAt: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
	return order.ID
}
