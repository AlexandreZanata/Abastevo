//go:build integration

package adapters_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestReviewedStationMeterSerializesAdmissionsAndRechecksRepresentation(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	fx := devicesFixture(t)
	ctx, orgID := t.Context(), fx.scope.OrgID().String()
	declared, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0))
	if err != nil {
		t.Fatal(err)
	}
	secondCNPJ := syntheticLegalCNPJ("EXEMPL010002")
	legalCommand, err := orgapp.AddEstablishmentCommand(declared.EntityID, onboardingID(t), 1,
		org.EstablishmentInput{CNPJ: secondCNPJ, Role: org.HeadOffice, TradeName: "Synthetic second station", MunicipalityCode: "5103403"})
	if err != nil {
		t.Fatal(err)
	}
	added, _, err := fx.write(t, legalCommand)
	if err != nil {
		t.Fatal(err)
	}
	establishments := []string{declared.EstablishmentID, added.EstablishmentID}
	cnpjs := []string{"EXEMPL01000170", secondCNPJ}
	operator := onboardingID(t)
	approvalIDs, stations, grants := [2]string{}, [2]string{}, [2]string{}
	for i := range stations {
		stations[i] = agreementStationForOperator(t, fx.f.Admin, fx.actor.AccountID, cnpjs[i], "price.publish")
		approvalIDs[i] = onboardingID(t)
		if err := fx.f.Admin.QueryRow(ctx, `SELECT id::text FROM representation_grants WHERE account_id=$1 AND station_id=$2`,
			fx.actor.AccountID, stations[i]).Scan(&grants[i]); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_billing_station_plan_reviews
(org_id,station_id,revision,approval_id,establishment_id,grant_id,grant_version,membership_id,membership_revision,
 operator_revision_id,approved_operator,decision,decision_ref)
SELECT $1::uuid,$2::uuid,1,$3::uuid,$4::uuid,g.id,g.version,m.id,m.revision,o.id,$7::uuid,'APPROVED',$8
FROM representation_grants g JOIN station_operator_revisions o USING(station_id)
JOIN org_memberships m ON m.org_id=$1::uuid AND m.id=$6::uuid AND m.account_id=g.account_id
WHERE g.id=$5::uuid AND o.valid_to IS NULL`, orgID, stations[i], approvalIDs[i], establishments[i], grants[i], fx.member, operator, "synthetic-station-review-"+approvalIDs[i]); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := audit.NewCatalog(billpg.StationPlanAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	pool := fx.f.RuntimePool(t, 3)
	audits := auditpg.NewStore(pool, catalog)
	plans := billpg.EntitlementStore{Pool: pool, Capabilities: known, Now: time.Now}
	flags := cappg.NewStore(pool, known, audits)
	flags.PlanWrites = plans
	store := billpg.StationMeter{Pool: pool, Plans: plans, Flags: flags, Audit: audits}
	if _, err := store.Admit(ctx, fx.scope, approvalIDs[0], operator, onboardingID(t)); !errors.Is(err, access.ErrEntitlementRequired) {
		t.Fatalf("pilot flag admitted station use without a reviewed plan: %v", err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog(product_code,plan_code,revision,display_name,capabilities,active_stations)
VALUES('station','reviewed-test',1,'Synthetic company station plan',ARRAY['organizations'],1)`); err != nil {
		t.Fatal(err)
	}
	now := lifecycleDBNow(t, fx.legalFixtureState)
	if err := database.WithOrgTx(ctx, pool, fx.scope, func(tx pgx.Tx) error {
		_, err := (billpg.SubscriptionLedger{Capabilities: known}).ApplyTx(ctx, tx, billing.PlanKey{Product: "station", Plan: "reviewed-test", Revision: 1},
			billing.LifecycleEvent{Scope: fx.scope, SubscriptionID: onboardingID(t), Kind: billing.ConfirmSettlement, Source: billing.VerifiedBilling,
				At: now, Reference: "synthetic-station-plan", Period: billing.Period{Start: now, End: now.Add(time.Hour)}})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, BillingEnabled: true, Origin: deviceTestOrigin,
		CursorSecret: bytes.Repeat([]byte{81}, 32), Pool: fx.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	assertCount := func(want string) {
		t.Helper()
		view := matrixCall(t, fx, router, owner, "/v1/orgs/"+orgID+"/subscriptions/plan/read", map[string]any{}, 200, "", "", nil)
		plan := view["plans"].([]any)[0].(map[string]any)
		if plan["active_stations_used"] != want || plan["limits"].(map[string]any)["active_stations"] != "1" {
			t.Fatalf("meter did not show the scoped active station count %s", want)
		}
	}
	assertCount("0")
	if _, err := store.Admit(ctx, fx.f.Organizations[1].Scope(t), approvalIDs[0], operator, onboardingID(t)); !errors.Is(err, billpg.ErrStationApprovalUnavailable) {
		t.Fatalf("foreign organization admitted a station: %v", err)
	}
	if _, err := store.Admit(ctx, fx.scope, approvalIDs[0], onboardingID(t), onboardingID(t)); !errors.Is(err, billpg.ErrStationApprovalUnavailable) {
		t.Fatalf("unreviewed operator admitted a station: %v", err)
	}
	type outcome struct {
		index     int
		operation string
		receipt   billpg.StationAdmission
		err       error
	}
	responses := make(chan outcome, 2)
	begin := make(chan struct{})
	for i, approval := range approvalIDs {
		operation := onboardingID(t)
		go func() {
			<-begin
			r, e := store.Admit(ctx, fx.scope, approval, operator, operation)
			responses <- outcome{i, operation, r, e}
		}()
	}
	close(begin)
	first, second := <-responses, <-responses
	if first.err != nil {
		first, second = second, first
	}
	if first.err != nil || !errors.Is(second.err, access.ErrLimitReached) || first.receipt.StationID != stations[first.index] {
		t.Fatalf("the concurrent last station slot did not settle: %v / %v", first.err, second.err)
	}
	assertCount("1")
	replayed, err := store.Admit(ctx, fx.scope, approvalIDs[first.index], operator, first.operation)
	expected := first.receipt
	expected.Replayed = true
	if err != nil || replayed != expected {
		t.Fatalf("matching station retry changed its admission: %v", err)
	}
	if _, err := store.Admit(ctx, fx.scope, approvalIDs[first.index], operator, onboardingID(t)); !errors.Is(err, billpg.ErrStationAdmissionExists) {
		t.Fatalf("the same station approval consumed a second allocation: %v", err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `UPDATE station_operator_revisions SET valid_to=clock_timestamp()
WHERE station_id=$1 AND valid_to IS NULL`, stations[first.index]); err != nil {
		t.Fatal(err)
	}
	assertCount("0")
	if _, err := store.Admit(ctx, fx.scope, approvalIDs[first.index], operator, onboardingID(t)); !errors.Is(err, billpg.ErrStationApprovalUnavailable) {
		t.Fatalf("an ended station operator remained eligible: %v", err)
	}
	other, err := store.Admit(ctx, fx.scope, approvalIDs[second.index], operator, second.operation)
	if err != nil || other.Replayed || other.StationID != stations[second.index] {
		t.Fatalf("an ended operator prevented another reviewed station from using the slot: %v", err)
	}
	assertCount("1")
	if _, err := fx.f.Admin.Exec(ctx, `UPDATE representation_grants SET status='revoked',version=version+1 WHERE id=$1`, grants[second.index]); err != nil {
		t.Fatal(err)
	}
	assertCount("0")
	var retained, changes int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM org_billing_station_plan_admissions WHERE org_id=$1),
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='subs.station.admitted')`, orgID).Scan(&retained, &changes); err != nil || retained != 2 || changes != 2 {
		t.Fatalf("admitted station history diverged: %d / %d / %v", retained, changes, err)
	}
}
