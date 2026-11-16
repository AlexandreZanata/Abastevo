//go:build integration

package adapters_test

import (
	"bytes"
	"errors"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	docapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	docdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// EMP-FUEL-011 (B-BR-EMP-459, EDR-297): a fueling receipt is an EMP-DOC-003
// link of kind FUELING, resolved by the fueling owner under
// fuel.fueling.record (an S cell): the author links a document to the
// fueling it recorded, carried by the typed fueling_id reference; another
// member's fueling is FORBIDDEN with the owner's key; a missing fueling, a
// fueling's own embedded reading named as a fueling and another
// organization's fueling are NOT_FOUND; refusals write nothing.
func TestFuelingReceiptLinksOnlyTheAuthorsStoredFuelingThroughComposedCompanyHTTP(t *testing.T) {
	x := fuelingFixture(t)
	fx := x.deviceFixtureState
	linkGrants(t, fx)
	op := x.op(t, x.syncFixtureState, x.declaredNow(t), nil)
	ack, err := x.process(t, x.syncFixtureState, op)
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("synthetic fueling was not accepted", err, ack.Result(), ack.Code())
	}
	fueling := op.Snapshot().Entity.ID
	var embeddedReading string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT reading_id::text FROM org_fuelings WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), fueling).Scan(&embeddedReading); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{37}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual fueling receipt composition failed", err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	author := incidentLogin(t, x.syncFixtureState.person, router)
	org := x.scope.OrgID().String()
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
		call(actor, docs+"/"+document+"/links/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "target": map[string]any{"kind": "FUELING", "id": target}}, want, code, key)
	}
	links := func() int {
		var n int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_document_links WHERE org_id=$1`, org).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	receipt := declare(author)
	link(author, receipt, fueling, 201, "", "")
	var bound bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT target_kind='FUELING' AND fueling_id=target_id AND fueling_id=$3 AND num_nonnulls(asset_id,employee_id,reading_id,held_reading_id,incident_report_id)=0
 FROM org_document_links WHERE org_id=$1 AND document_id=$2`, org, receipt, fueling).Scan(&bound); err != nil || !bound {
		t.Fatal("the FUELING link is not carried by its typed reference", err)
	}
	before := links()
	ownerReceipt := declare(owner)
	// fuel.fueling.record is an S cell: even the owner may not document someone else's fueling.
	link(owner, ownerReceipt, fueling, 403, "FORBIDDEN", "fuel.fueling.record")
	link(author, declare(author), onboardingID(t), 404, "NOT_FOUND", "")
	// The fueling's embedded reading is a METER_READING, never a fueling.
	link(author, declare(author), embeddedReading, 404, "NOT_FOUND", "")
	if links() != before {
		t.Fatal("a refused FUELING link was stored")
	}
	byTarget := call(author, docs+"/links/by-target", map[string]any{"target": map[string]any{"kind": "FUELING", "id": fueling}, "limit": 10}, 200, "", "")
	if items, _ := byTarget["items"].([]any); len(items) != 1 {
		t.Fatal("links of the fueling", byTarget)
	}
	call(owner, docs+"/links/by-target", map[string]any{"target": map[string]any{"kind": "FUELING", "id": fueling}}, 403, "FORBIDDEN", "fuel.fueling.record")
	// Another organization: the same fueling identifier resolves to nothing.
	other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := kernel.NewOrgScope(other.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, foreign, func(tx pgx.Tx) error {
		_, err := fuelpg.FuelingDocumentTargets{}.ResolveDocumentTargetTx(t.Context(), tx, foreign, docdomain.Fueling, fueling)
		if !errors.Is(err, docapp.ErrNotFound) {
			t.Fatal("another organization resolved the fueling", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// In its own organization the resolver names the author for SELF coverage.
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		resolved, err := fuelpg.FuelingDocumentTargets{}.ResolveDocumentTargetTx(t.Context(), tx, x.scope, docdomain.Fueling, fueling)
		if err != nil || resolved.Binding != docapp.BindFueling || resolved.AuthoredBy != x.syncFixtureState.person.member {
			t.Fatal("own fueling resolution", err, resolved.Binding)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
