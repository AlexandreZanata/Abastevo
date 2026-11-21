//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestFuelCardPreviewAndConfirmationPreserveOneCost(t *testing.T) {
	x := fuelingFixture(t)
	ctx := t.Context()
	fx := x.deviceFixtureState
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(ctx, "GRANT SELECT ON org_connector_approvals,org_finding_settings TO "+role+
		"; GRANT SELECT,INSERT,UPDATE ON org_connections,identity_rate_windows TO "+role+
		"; GRANT SELECT,INSERT ON org_card_statement_lines TO "+role); err != nil {
		t.Fatal(err)
	}
	declared := x.declaredNow(t)
	fueling := x.op(t, x.syncFixtureState, declared, nil)
	ack, err := x.process(t, x.syncFixtureState, fueling)
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatalf("normal card fueling refused: %s %v", ack.Code(), err)
	}
	org, source, approval := x.scope.OrgID().String(), onboardingID(t), onboardingID(t)
	catalog, err := audit.NewCatalog(cappg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	keys, err := capability.NewCatalog([]string{"organizations", "integrations"})
	if err != nil {
		t.Fatal(err)
	}
	flags := cappg.NewStore(x.f.Runtime, keys, auditpg.NewStore(x.f.Runtime, catalog))
	if _, err := flags.Set(ctx, x.scope, capability.Actor{AccountID: x.actor.AccountID}, capability.Change{
		OperationID: onboardingID(t), Capability: "integrations", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.Admin.Exec(ctx, `INSERT INTO org_connector_approvals
(org_id,id,revision,kind,provider_code,resource_id,maximum_bytes,maximum_items,decision,decision_ref,ready_at)
VALUES($1,$2,1,'FUEL_CARD_STATEMENT','synthetic-card',$3,4096,3,'APPROVED','synthetic-statement-v1',clock_timestamp())`,
		org, approval, source); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	accounts := *x.accounts
	accounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{
		Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{26}, 32), Pool: x.f.Runtime, Accounts: &accounts,
	}); err != nil {
		t.Fatal(err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	driver := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, driver, "DRIVER", "ORG", "ACTIVE")
	base := "/v1/orgs/" + org + "/integrations/connections"
	connectionID := onboardingID(t)
	matrixCall(t, fx, router, owner, base+"/configure", map[string]any{
		"schema_version": 1, "operation_id": connectionID, "approval_id": approval, "approval_revision": 1, "label": "Card statements",
	}, 200, "", "", nil)
	base += "/" + connectionID
	matrixCall(t, fx, router, owner, base+"/activate", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 1,
	}, 200, "", "", nil)
	const header = "transaction_id,fueling_id,occurred_at,currency,paid_centavos\n"
	transaction := onboardingID(t)
	data := header + fmt.Sprintf("%s,%s,%s,BRL,97350\n", transaction, fueling.Snapshot().Entity.ID, declared.Format(time.RFC3339Nano))
	statement := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "source_account_id": source, "csv": data}
	path := base + "/fuel-card"
	matrixCall(t, fx, router, driver, path+"/preview", statement, 403, "FORBIDDEN", "import.batch.run", nil)
	matrixCall(t, fx, router, owner, path+"/confirm", statement, 400, "MALFORMED", "", nil)
	statement["source_account_id"] = onboardingID(t)
	matrixCall(t, fx, router, owner, path+"/preview", statement, 404, "NOT_FOUND", "", nil)
	statement["source_account_id"] = source
	preview := matrixCall(t, fx, router, owner, path+"/preview", statement, 200, "", "", nil)
	raw, err := json.Marshal(preview)
	if err != nil || !bytes.Contains(raw, []byte("REVIEW_REQUIRED")) || bytes.Contains(raw, []byte(transaction)) ||
		bytes.Contains(raw, []byte(fueling.Snapshot().Entity.ID)) || bytes.Contains(raw, []byte(source)) {
		t.Fatal("statement preview exposed source references or treated a submitted fueling as approved")
	}
	var reconciled, costs, audits int
	wantCosts := 1
	count := func(wantRows, wantAudits int) {
		t.Helper()
		err := x.f.Admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM org_card_statement_lines WHERE org_id=$1),
 (SELECT count(*) FROM org_expenses WHERE org_id=$1),
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='integ.card_statement.confirmed')`, org).
			Scan(&reconciled, &costs, &audits)
		if err != nil || reconciled != wantRows || costs != wantCosts || audits != wantAudits {
			t.Fatalf("unexpected statement side effects: rows=%d costs=%d audits=%d err=%v", reconciled, costs, audits, err)
		}
	}
	count(0, 0)
	statement["preview_token"] = preview["preview_token"]
	statement["csv"] = strings.Replace(data, "97350", "97351", 1)
	matrixCall(t, fx, router, owner, path+"/confirm", statement, 400, "MALFORMED", "", nil)
	statement["csv"] = data
	command, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	replies := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, deviceTestOrigin+path+"/confirm", bytes.NewReader(command))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", deviceTestOrigin)
		request.Header.Set("X-CSRF", owner.csrf)
		request.AddCookie(owner.cookie)
		go func() {
			reply := httptest.NewRecorder()
			router.ServeHTTP(reply, request)
			replies <- reply
		}()
	}
	replayed := 0
	for range 2 {
		reply := <-replies
		var result struct {
			Replayed bool   `json:"replayed"`
			Token    string `json:"preview_token"`
		}
		if reply.Code != http.StatusOK || json.Unmarshal(reply.Body.Bytes(), &result) != nil || result.Token != "" {
			t.Fatalf("concurrent confirmation returned %d", reply.Code)
		}
		if result.Replayed {
			replayed++
		}
	}
	if replayed != 1 {
		t.Fatal("retry did not replay the bounded confirmation")
	}
	count(1, 1)
	statement["operation_id"] = onboardingID(t)
	matrixCall(t, fx, router, owner, path+"/confirm", statement, 200, "", "", nil)
	count(1, 2)
	delete(statement, "preview_token")
	statement["csv"] = strings.Replace(data, "97350", "97351", 1)
	matrixCall(t, fx, router, owner, path+"/preview", statement, 409, "CONFLICT", "", nil)
	statement["csv"] = strings.Replace(data, "BRL", "USD", 1)
	matrixCall(t, fx, router, owner, path+"/preview", statement, 422, "VALIDATION_FAILED", "", nil)

	approvedAt := x.declaredNow(t)
	secondFueling := x.op(t, x.syncFixtureState, approvedAt, func(_, payload map[string]any) {
		payload["quantity"], payload["paid_total"] = "140000", "94000"
		payload["reading"], payload["missing_reading_reason"] = nil, "Meter was unavailable"
	})
	accepted, err := x.process(t, x.syncFixtureState, secondFueling)
	if err != nil || accepted.Result() != protocol.Accepted {
		t.Fatalf("second card fueling was refused: %s %v", accepted.Code(), err)
	}
	wantCosts = 2
	secondFuelingID := secondFueling.Snapshot().Entity.ID
	decisions := "/v1/orgs/" + org + "/fuelings/" + secondFuelingID + "/review"
	author := incidentLogin(t, x.syncFixtureState.person, router)
	commandReview := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "decision": "OPEN"}
	matrixCall(t, fx, router, author, decisions+"/decide", commandReview, 409, "SELF_APPROVAL", "", nil)
	matrixCall(t, fx, router, owner, decisions+"/decide", commandReview, 200, "", "", nil)
	commandReview["operation_id"], commandReview["decision"] = onboardingID(t), "APPROVE"
	matrixCall(t, fx, router, owner, decisions+"/decide", commandReview, 409, "CONFLICT", "", nil)
	commandReview["revision"] = 2
	matrixCall(t, fx, router, owner, decisions+"/decide", commandReview, 422, "VALIDATION_FAILED", "", nil)
	commandReview["warning_notes"] = map[string]string{"ARITHMETIC_MISMATCH": "Paid amount and receipt were checked"}
	approved := matrixCall(t, fx, router, owner, decisions+"/decide", commandReview, 200, "", "", nil)
	if approved["state"] != "APPROVED" {
		t.Fatal("authorized reviewer did not record the approval")
	}
	if replay := matrixCall(t, fx, router, owner, decisions+"/decide", commandReview, 200, "", "", nil); replay["replayed"] != true {
		t.Fatal("approval retry repeated the decision")
	}
	view := matrixCall(t, fx, router, owner, decisions+"/read", map[string]any{}, 200, "", "", nil)
	if view["state"] != "APPROVED" || view["sequence"] != float64(3) {
		t.Fatal("read did not show the stored review decision")
	}
	approvedStatement := map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "source_account_id": source,
		"csv": header + fmt.Sprintf("%s,%s,%s,BRL,94000\n", onboardingID(t), secondFuelingID, approvedAt.Format(time.RFC3339Nano)),
	}
	approvedPreview := matrixCall(t, fx, router, owner, path+"/preview", approvedStatement, 200, "", "", nil)
	approvedStatement["preview_token"] = approvedPreview["preview_token"]
	confirmed := matrixCall(t, fx, router, owner, path+"/confirm", approvedStatement, 200, "", "", nil)
	rows := confirmed["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["status"] != "LINKED" {
		t.Fatal("verified same-company card match did not link")
	}
	count(2, 3)
	var matched bool
	if err := x.f.Admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_card_statement_lines
 WHERE org_id=$1 AND fueling_id=$2 AND paid_centavos=94000)`, org, secondFuelingID).Scan(&matched); err != nil || !matched {
		t.Fatalf("confirmed card match has no exact retained basis: %v", err)
	}

	matrixCall(t, fx, router, owner, base+"/disable", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 2,
	}, 200, "", "", nil)
	statement["csv"], statement["preview_token"] = data, preview["preview_token"]
	matrixCall(t, fx, router, owner, path+"/confirm", statement, 404, "NOT_FOUND", "", nil)
	count(2, 3)
}
