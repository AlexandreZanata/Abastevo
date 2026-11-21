//go:build integration

package adapters_test

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestCompanyUploadStorageCeilingSerializesReservations(t *testing.T) {
	fx := syncFixture(t).deviceFixtureState
	ctx := t.Context()
	documentGrants(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog,enterprise_billing_offers,org_billing_charge_bindings,org_billing_settlement_receipts TO "+role+
		"; GRANT SELECT,INSERT ON org_billing_subscription_events TO "+role); err != nil {
		t.Fatal(err)
	}
	const ceiling = 1024
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,storage_bytes)
VALUES('fleet','synthetic',1,'Synthetic original storage',ARRAY['organizations'],$1)`, ceiling); err != nil {
		t.Fatal(err)
	}
	catalog, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	ledger := billpg.SubscriptionLedger{Capabilities: catalog}
	key := billing.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	at := lifecycleDBNow(t, fx.legalFixtureState)
	active := billing.LifecycleEvent{
		Scope: fx.scope, SubscriptionID: onboardingID(t), Kind: billing.ConfirmSettlement, Source: billing.VerifiedBilling,
		At: at, Reference: "synthetic-paid-storage", Period: billing.Period{Start: at, End: at.Add(time.Hour)},
	}
	proof := fx.f.PrepareSubscriptionOpening(t, key, active)
	if err := database.WithOrgTx(ctx, fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, err := ledger.ApplyTx(ctx, tx, key, active); err != nil {
			return err
		}
		return proof(tx)
	}); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{
		Enabled: true, BillingEnabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{67}, 32),
		Pool: fx.f.Runtime, Accounts: &accounts,
	}); err != nil {
		t.Fatal(err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	org := fx.scope.OrgID().String()
	path := "/v1/orgs/" + org + "/documents/uploads/create"
	type reply struct {
		command map[string]any
		status  int
	}
	replies := make(chan reply, 2)
	start := make(chan struct{})
	for range 2 {
		command := matrixDocumentDeclaration(t)
		go func() {
			<-start
			replies <- reply{command, matrixCallStatus(t, fx, router, owner, path, command)}
		}()
	}
	close(start)
	a, b := <-replies, <-replies
	if a.status == http.StatusForbidden {
		a, b = b, a
	}
	if a.status != http.StatusCreated || b.status != http.StatusForbidden {
		t.Fatalf("one reservation must win: %d %d", a.status, b.status)
	}
	again := matrixCall(t, fx, router, owner, path, a.command, http.StatusCreated, "", "", nil)
	if again["replayed"] != true {
		t.Fatal("declared upload retry spent capacity a second time")
	}
	matrixCall(t, fx, router, owner, path, b.command, http.StatusForbidden, "LIMIT_REACHED", "", nil)
	overview := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/subscriptions/plan/read", map[string]any{}, http.StatusOK, "", "", nil)
	plans, _ := overview["plans"].([]any)
	if len(plans) != 1 || plans[0].(map[string]any)["storage_used_bytes"] != "1024" {
		t.Fatal("plan read did not show the same exact storage allocation")
	}
	var documents, auditEntries int
	var totalBytes int64
	if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*),COALESCE(sum(original_bytes),0)::bigint,
  (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='document.upload.created')
  FROM org_documents WHERE org_id=$1`, org).Scan(&documents, &totalBytes, &auditEntries); err != nil ||
		documents != 1 || totalBytes != ceiling || auditEntries != 1 {
		t.Fatalf("storage allocation went past its exact budget: files=%d bytes=%d audits=%d err=%v", documents, totalBytes, auditEntries, err)
	}
}
