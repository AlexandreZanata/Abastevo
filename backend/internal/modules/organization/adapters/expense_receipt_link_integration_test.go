//go:build integration

package adapters_test

import (
	"bytes"
	"errors"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	docapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	docdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	expensepg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// EMP-EXP-004 (B-BR-EMP-520, EDR-338): an expense receipt is an EMP-DOC-003
// link of kind EXPENSE, resolved by the expense owner under
// exp.expense.record (an S cell): the author links a document to the direct
// expense it recorded, carried by the typed expense_id reference; another
// member's expense is FORBIDDEN with the owner's key; a missing expense, a
// FUELING-origin expense (documented through its fueling) and another
// organization's expense are NOT_FOUND; refusals write nothing, and a link
// never changes the expense's amounts or creates a cost.
func TestExpenseReceiptLinksOnlyTheAuthorsDirectExpenseThroughComposedCompanyHTTP(t *testing.T) {
	x := expenseFixture(t)
	fx := x.deviceFixtureState
	linkGrants(t, fx)
	d := x.syncFixtureState
	op := x.directOp(t, d, x.declaredNow(t), nil)
	ack, err := x.processDirect(t, d, op)
	x.directAccepted(t, op, ack, err)
	expense := op.Snapshot().Entity.ID
	fuelingOp := x.op(t, d, x.declaredNow(t), nil)
	if ack, err := x.process(t, d, fuelingOp); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("synthetic fueling was not accepted", err, ack.Result(), ack.Code())
	}
	n, fuelingCost := x.costOf(t, fuelingOp.Snapshot().Entity.ID)
	if n != 1 {
		t.Fatal("fueling without its one cost", n)
	}
	org := x.scope.OrgID().String()
	costs := func() (int, int64, int64) {
		var count int
		var paid, company int64
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*),COALESCE(sum(paid_centavos),0),COALESCE(sum(company_centavos),0) FROM org_expenses WHERE org_id=$1`, org).Scan(&count, &paid, &company); err != nil {
			t.Fatal(err)
		}
		return count, paid, company
	}
	beforeCount, beforePaid, beforeCompany := costs()
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{38}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual expense receipt composition failed", err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	author := incidentLogin(t, d.person, router)
	docs := "/v1/orgs/" + org + "/documents"
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	declare := func(actor matrixPerson) string {
		v := matrixDocumentDeclaration(t)
		call(actor, docs+"/uploads/create", v, 201, "", "")
		return v["document_id"].(string)
	}
	link := func(actor matrixPerson, document, target string, want int, code, key string) {
		t.Helper()
		call(actor, docs+"/"+document+"/links/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "target": map[string]any{"kind": "EXPENSE", "id": target}}, want, code, key)
	}
	links := func() int {
		var n int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_document_links WHERE org_id=$1`, org).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	receipt := declare(author)
	link(author, receipt, expense, 201, "", "")
	var bound bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT target_kind='EXPENSE' AND expense_id=target_id AND expense_id=$3 AND num_nonnulls(asset_id,employee_id,reading_id,held_reading_id,incident_report_id,fueling_id)=0
 FROM org_document_links WHERE org_id=$1 AND document_id=$2`, org, receipt, expense).Scan(&bound); err != nil || !bound {
		t.Fatal("the EXPENSE link is not carried by its typed reference", err)
	}
	before := links()
	ownerReceipt := declare(owner)
	// exp.expense.record is an S cell: even the owner may not document someone else's expense.
	link(owner, ownerReceipt, expense, 403, "FORBIDDEN", "exp.expense.record")
	link(author, declare(author), onboardingID(t), 404, "NOT_FOUND", "")
	// A FUELING-origin expense is documented through its fueling, never as EXPENSE.
	link(author, declare(author), fuelingCost, 404, "NOT_FOUND", "")
	// The asset an expense names is not an expense.
	link(author, declare(author), x.vehicle.AssetID, 404, "NOT_FOUND", "")
	if links() != before {
		t.Fatal("a refused EXPENSE link was stored")
	}
	byTarget := call(author, docs+"/links/by-target", map[string]any{"target": map[string]any{"kind": "EXPENSE", "id": expense}, "limit": 10}, 200, "", "")
	if items, _ := byTarget["items"].([]any); len(items) != 1 {
		t.Fatal("links of the expense", byTarget)
	}
	call(owner, docs+"/links/by-target", map[string]any{"target": map[string]any{"kind": "EXPENSE", "id": expense}}, 403, "FORBIDDEN", "exp.expense.record")
	// A link is evidence, never a cost: no expense row or amount changed.
	if c, p, co := costs(); c != beforeCount || p != beforePaid || co != beforeCompany {
		t.Fatal("a receipt link changed the costs", c, p, co)
	}
	// Another organization: the same expense identifier resolves to nothing.
	other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := kernel.NewOrgScope(other.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, foreign, func(tx pgx.Tx) error {
		_, err := expensepg.ExpenseDocumentTargets{}.ResolveDocumentTargetTx(t.Context(), tx, foreign, docdomain.Expense, expense)
		if !errors.Is(err, docapp.ErrNotFound) {
			t.Fatal("another organization resolved the expense", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// In its own organization the resolver names the author for SELF coverage.
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		resolved, err := expensepg.ExpenseDocumentTargets{}.ResolveDocumentTargetTx(t.Context(), tx, x.scope, docdomain.Expense, expense)
		if err != nil || resolved.Binding != docapp.BindExpense || resolved.AuthoredBy != d.person.member {
			t.Fatal("own expense resolution", err, resolved.Binding)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
