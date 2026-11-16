//go:build integration

package adapters_test

import (
	"strings"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
)

func TestDocumentLinksOverTheCompanyAPI(t *testing.T) {
	d := newDocumentHTTP(t)
	fx, router := d.fx, d.router
	linkGrants(t, fx)
	owner, manager, driver, supervisor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, manager, "FLEET_MANAGER", "ORG", "ACTIVE")
	matrixAssign(t, fx, driver, "DRIVER", "ORG", "ACTIVE")
	matrixAssign(t, fx, supervisor, "SUPERVISOR", "ORG", "ACTIVE")
	assets := lifecycleStore(t, fx.legalFixtureState)
	active, _ := lifecycleAssetIn(t, fx.legalFixtureState, assets, allocationUnit(t, fx.legalFixtureState, "LINKHTTP"), fleet.Active)
	draft, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	// The organization creator installs a meter; its opening reading is a
	// real reading authored by another member than the people below.
	installed, _, err := meterInstall(t, fx.legalFixtureState, meterStore(t, fx.legalFixtureState), meterInstallCommand(t, meterInstallInput(t, draft.AssetID, time.Now().UTC().Add(-4*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var otherEmployee string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, other.scope.OrgID().String(), other.member).Scan(&otherEmployee); err != nil {
		t.Fatal(err)
	}
	base := "/v1/orgs/" + d.org + "/documents/"
	target := func(kind, id string) map[string]any { return map[string]any{"kind": kind, "id": id} }
	create := func(op, kind, id string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": op, "target": target(kind, id)}
	}
	row := func(id string) string {
		t.Helper()
		var out string
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT to_jsonb(d)::text FROM org_documents d WHERE id=$1`, id).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// BUC-EMP-123: a manager links an own pending receipt to an asset.
	declaration, created := d.declare(t, manager, d.raw)
	managerDoc := declaration["document_id"].(string)
	capability := created["upload"].(map[string]any)["capability"].(string)
	before := row(managerDoc)
	op := onboardingID(t)
	first := matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(op, "ASSET", active), 201, "", "", nil)
	link := first["link"].(map[string]any)
	if first["replayed"] != false || link["document_id"] != managerDoc || link["document_storage"] != "UPLOADING" || link["target"].(map[string]any)["id"] != active || len(link) != 4 {
		t.Fatal("asset link response", first)
	}
	if again := matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(op, "ASSET", active), 201, "", "", nil); again["replayed"] != true || again["link"].(map[string]any)["created_at"] != link["created_at"] {
		t.Fatal("identical retry is not the first link", again)
	}
	matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(op, "METER_READING", installed.OpeningReadingID), 409, "OPERATION_ID_REUSED", "", nil)
	if existing := matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(onboardingID(t), "ASSET", active), 201, "", "", nil); existing["replayed"] != true {
		t.Fatal("duplicate association wrote", existing)
	}
	matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(onboardingID(t), "METER_READING", installed.OpeningReadingID), 201, "", "", nil)
	matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(onboardingID(t), "ASSET", draft.AssetID), 409, "CONFLICT", "", nil)
	if d.count(t, `SELECT count(*) FROM org_document_links WHERE document_id=$1`, managerDoc) != 2 || d.count(t, `SELECT count(*) FROM org_audit_events WHERE action='document.link.created' AND target_id=$1`, managerDoc) != 2 {
		t.Fatal("manager links not stored once each")
	}
	if row(managerDoc) != before {
		t.Fatal("linking changed the document's statuses, revisions or timestamps")
	}

	// Each endpoint's permission is enforced with its own key; forged,
	// unregistered and foreign references are refused without writing.
	driverDeclaration, _ := d.declare(t, driver, d.raw)
	driverDoc := driverDeclaration["document_id"].(string)
	matrixCall(t, fx, router, driver, base+driverDoc+"/links/create", create(onboardingID(t), "METER_READING", installed.OpeningReadingID), 403, "FORBIDDEN", "meter.reading.record", nil)
	matrixCall(t, fx, router, driver, base+driverDoc+"/links/create", create(onboardingID(t), "ASSET", active), 403, "FORBIDDEN", "fleet.asset.manage", nil)
	matrixCall(t, fx, router, driver, base+managerDoc+"/links/create", create(onboardingID(t), "ASSET", active), 403, "FORBIDDEN", "doc.document.attach", nil)
	for _, kind := range []string{"WORK_ORDER", "TRIP", "BOGUS"} {
		matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(onboardingID(t), kind, active), 422, "VALIDATION_FAILED", "", nil)
	}
	// FUELING has an owner since EMP-FUEL-011: an asset identifier is not a fueling.
	matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(onboardingID(t), "FUELING", active), 404, "NOT_FOUND", "", nil)
	// EXPENSE has an owner since EMP-EXP-004: an asset identifier is not an expense.
	matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(onboardingID(t), "EXPENSE", active), 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, manager, base+managerDoc+"/links/create", create(onboardingID(t), "METER_READING", active), 404, "NOT_FOUND", "", nil)
	ownerDeclaration, _ := d.declare(t, owner, d.raw)
	ownerDoc := ownerDeclaration["document_id"].(string)
	matrixCall(t, fx, router, owner, base+ownerDoc+"/links/create", create(onboardingID(t), "EMPLOYEE", otherEmployee), 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, base+ownerDoc+"/links/create", create(onboardingID(t), "EMPLOYEE", driver.employee), 201, "", "", nil)
	matrixCall(t, fx, router, owner, "/v1/orgs/"+other.scope.OrgID().String()+"/documents/"+ownerDoc+"/links/create", create(onboardingID(t), "EMPLOYEE", otherEmployee), 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, base+onboardingID(t)+"/links/create", create(onboardingID(t), "EMPLOYEE", driver.employee), 404, "NOT_FOUND", "", nil)

	// Lists page through sealed cursors and are scoped to both endpoints.
	firstPage := matrixCall(t, fx, router, manager, base+managerDoc+"/links/list", map[string]any{"limit": 1}, 200, "", "", nil)
	items := firstPage["items"].([]any)
	next, _ := firstPage["next_cursor"].(string)
	if len(items) != 1 || next == "" || items[0].(map[string]any)["target"].(map[string]any)["kind"] != "ASSET" {
		t.Fatal("first document link page", firstPage)
	}
	secondPage := matrixCall(t, fx, router, manager, base+managerDoc+"/links/list", map[string]any{"limit": 1, "cursor": next}, 200, "", "", nil)
	if items := secondPage["items"].([]any); len(items) != 1 || secondPage["next_cursor"] != "" || items[0].(map[string]any)["target"].(map[string]any)["kind"] != "METER_READING" {
		t.Fatal("second document link page", secondPage)
	}
	// The cursor is bound to the route, document and limit.
	matrixCall(t, fx, router, manager, base+managerDoc+"/links/list", map[string]any{"limit": 2, "cursor": next}, 400, "MALFORMED", "", nil)
	matrixCall(t, fx, router, manager, base+"links/by-target", map[string]any{"target": target("ASSET", active), "limit": 1, "cursor": next}, 400, "MALFORMED", "", nil)
	matrixCall(t, fx, router, driver, base+managerDoc+"/links/list", map[string]any{}, 403, "FORBIDDEN", "doc.document.attach", nil)
	if empty := matrixCall(t, fx, router, driver, base+driverDoc+"/links/list", map[string]any{}, 200, "", "", nil); len(empty["items"].([]any)) != 0 || empty["next_cursor"] != "" {
		t.Fatal("driver's unlinked document", empty)
	}
	byAsset := matrixCall(t, fx, router, manager, base+"links/by-target", map[string]any{"target": target("ASSET", active)}, 200, "", "", nil)
	if items := byAsset["items"].([]any); len(items) != 1 || items[0].(map[string]any)["document_id"] != managerDoc {
		t.Fatal("asset links", byAsset)
	}
	matrixCall(t, fx, router, supervisor, base+"links/by-target", map[string]any{"target": target("ASSET", active)}, 403, "FORBIDDEN", "fleet.asset.manage", nil)
	matrixCall(t, fx, router, manager, base+"links/by-target", map[string]any{"target": target("WORK_ORDER", active)}, 422, "VALIDATION_FAILED", "", nil)
	matrixCall(t, fx, router, manager, base+"links/by-target", map[string]any{"target": target("EXPENSE", active)}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, manager, base+"links/by-target", map[string]any{"target": target("FUELING", active)}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, base+"links/by-target", map[string]any{"target": target("EMPLOYEE", otherEmployee)}, 404, "NOT_FOUND", "", nil)

	// B-BR-EMP-242: the supervisor sees the reading's link but not the
	// receipt, the driver cannot open another member's receipt, and nobody
	// opens a pending original.
	readingLinks := matrixCall(t, fx, router, supervisor, base+"links/by-target", map[string]any{"target": target("METER_READING", installed.OpeningReadingID)}, 200, "", "", nil)
	if items := readingLinks["items"].([]any); len(items) != 1 || items[0].(map[string]any)["document_id"] != managerDoc || items[0].(map[string]any)["document_storage"] != "UPLOADING" {
		t.Fatal("reading links", readingLinks)
	}
	matrixCall(t, fx, router, supervisor, base+managerDoc+"/content/access", map[string]any{}, 403, "FORBIDDEN", "doc.document.view_content", nil)
	matrixCall(t, fx, router, driver, base+managerDoc+"/content/access", map[string]any{}, 403, "FORBIDDEN", "doc.document.view_content", nil)
	matrixCall(t, fx, router, manager, base+managerDoc+"/content/access", map[string]any{}, 404, "NOT_FOUND", "", nil)
	if w := d.upload(t, d.org, managerDoc, capability, "image/jpeg", d.raw); w.Code != 200 {
		t.Fatal("verified upload", w.Code)
	}
	stored := matrixCall(t, fx, router, supervisor, base+"links/by-target", map[string]any{"target": target("METER_READING", installed.OpeningReadingID)}, 200, "", "", nil)
	if stored["items"].([]any)[0].(map[string]any)["document_storage"] != "STORED" {
		t.Fatal("stored receipt status not reflected", stored)
	}
	matrixCall(t, fx, router, supervisor, base+managerDoc+"/content/access", map[string]any{}, 403, "FORBIDDEN", "doc.document.view_content", nil)
	matrixCall(t, fx, router, driver, base+managerDoc+"/content/access", map[string]any{}, 403, "FORBIDDEN", "doc.document.view_content", nil)
	if granted := matrixCall(t, fx, router, manager, base+managerDoc+"/content/access", map[string]any{}, 200, "", "", nil); granted["capability"] == "" {
		t.Fatal("content permission holder lost content")
	}
	if d.count(t, `SELECT count(*) FROM org_sensitive_access_events`) != 1 {
		t.Fatal("only the content holder's issuance is audited")
	}
	if links := d.count(t, `SELECT count(*) FROM org_document_links`); links != 3 || d.count(t, `SELECT count(*) FROM org_audit_events WHERE action='document.link.created'`) != links {
		t.Fatal("link rows and events differ", links)
	}
	if strings.Contains(d.logs.String(), managerDoc) || strings.Contains(d.logs.String(), capability) {
		t.Fatal("request logs carry a document identifier or capability")
	}
}
