//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
)

// incidentLogin opens a company session for an existing device person.
func incidentLogin(t *testing.T, p devicePerson, router chi.Router) matrixPerson {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"family_id": p.auth.Session.FamilyID, "access_token": p.auth.Session.AccessToken})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+"/v1/org-access/session/login", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", deviceTestOrigin)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var result struct {
		CSRF string `json:"csrf_token"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.CSRF == "" || len(w.Result().Cookies()) != 1 {
		t.Fatal("company login of the device person failed", w.Code)
	}
	return matrixPerson{account: p.auth.Account.ID, member: p.member, employee: p.employee, cookie: w.Result().Cookies()[0], csrf: result.CSRF}
}

func (x handlingState) reportedBy(t *testing.T, d syncFixtureState, payload map[string]any) string {
	t.Helper()
	op := x.reportOp(t, d, payload, x.declaredNow(t), nil)
	ack, err := x.process(t, d, op)
	x.reportAccepted(t, op, ack, err)
	return reportOf(op)
}

// costRows is the cost snapshot without the document rows the test creates.
func (x handlingState) costRows(t *testing.T) map[string]int64 {
	t.Helper()
	out := x.costSnapshot(t)
	delete(out, "org_documents")
	delete(out, "org_document_links")
	return out
}

// BUC-EMP-187/188 and B-BR-EMP-372..374 through the actual composed company
// routes: the queue, one report, the own list and the handling step, and the
// EMP-DOC-003 link routes with the INCIDENT kind. Photos keep their own
// storage state, a failed upload stays visibly failed, no read serves content
// and refusals change nothing.
func TestIncidentReportsThroughComposedCompanyHTTP(t *testing.T) {
	x := handlingFixture(t)
	fx := x.deviceFixtureState
	linkGrants(t, fx)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{31}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual incident composition failed", err)
	}
	owner, auditor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	before := x.costRows(t)
	blocking := damagePayload(x.vehicle.AssetID, nil)
	blocking["blocks_use"] = true
	report := x.reported(t, blocking)
	driver := x.driver(t)
	x.assignDriver(t, driver)
	driven := x.reportedBy(t, driver, damagePayload(x.vehicle.AssetID, nil))
	reporter := incidentLogin(t, x.syncFixtureState.person, router)
	driverWeb := incidentLogin(t, driver.person, router)
	org := x.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/incidents/reports"
	docs := "/v1/orgs/" + org + "/documents"
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	items := func(v map[string]any) []map[string]any {
		raw, _ := v["items"].([]any)
		out := []map[string]any{}
		for _, item := range raw {
			out = append(out, item.(map[string]any))
		}
		return out
	}
	queue := items(call(owner, base+"/list", map[string]any{"limit": 10}, 200, "", ""))
	if len(queue) != 2 {
		t.Fatal("queue", len(queue))
	}
	for _, item := range queue {
		for _, private := range []string{"description", "location", "unit_id", "cost_centre_id", "history", "attachments"} {
			if _, ok := item[private]; ok {
				t.Fatal("the queue exposed", private)
			}
		}
	}
	// AUDITOR holds neither key (AUTH-011), the driver has no manage key.
	for _, path := range []string{base + "/list", base + "/" + report + "/read"} {
		call(auditor, path, map[string]any{}, 403, "FORBIDDEN", "check.incident.manage")
		call(driverWeb, path, map[string]any{}, 403, "FORBIDDEN", "check.incident.manage")
	}
	decision := func(revision any, action string, note any, op string) map[string]any {
		v := map[string]any{"schema_version": 1, "operation_id": op, "revision": revision, "action": action}
		if note != nil {
			v["note"] = note
		}
		return v
	}
	call(auditor, base+"/"+report+"/decide", decision(1, "ACKNOWLEDGE", nil, onboardingID(t)), 403, "FORBIDDEN", "check.incident.manage")
	call(auditor, base+"/mine", map[string]any{}, 403, "FORBIDDEN", "check.incident.report")
	mine := items(call(driverWeb, base+"/mine", map[string]any{"limit": 10}, 200, "", ""))
	if len(mine) != 1 || mine[0]["report_id"] != driven || mine[0]["description"] != "Lanterna traseira trincada ao manobrar na doca" {
		t.Fatal("the driver's own list", mine)
	}
	if own := items(call(reporter, base+"/mine", map[string]any{"limit": 10}, 200, "", "")); len(own) != 1 || own[0]["report_id"] != report || own[0]["blocks_use"] != true {
		t.Fatal("the reporter's own list", own)
	}
	// Photos are EMP-DOC-003 links with the INCIDENT kind (EDR-239).
	declare := func(actor matrixPerson) string {
		v := matrixDocumentDeclaration(t)
		call(actor, docs+"/uploads/create", v, 201, "", "")
		return v["document_id"].(string)
	}
	link := func(actor matrixPerson, document, target string, want int, code, key string) {
		t.Helper()
		call(actor, docs+"/"+document+"/links/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "target": map[string]any{"kind": "INCIDENT", "id": target}}, want, code, key)
	}
	photo := declare(reporter)
	link(reporter, photo, report, 201, "", "")
	ownerPhoto, driverPhoto := declare(owner), declare(driverWeb)
	link(reporter, ownerPhoto, report, 403, "FORBIDDEN", "doc.document.attach")
	link(owner, ownerPhoto, report, 403, "FORBIDDEN", "check.incident.report")
	link(driverWeb, driverPhoto, report, 403, "FORBIDDEN", "check.incident.report")
	link(reporter, photo, onboardingID(t), 404, "NOT_FOUND", "")
	link(reporter, photo, x.foreignReport(t), 404, "NOT_FOUND", "")
	read := func() map[string]any { return call(owner, base+"/"+report+"/read", map[string]any{}, 200, "", "") }
	attachments := func(v map[string]any) map[string]string {
		out := map[string]string{}
		for _, a := range v["attachments"].([]any) {
			m := a.(map[string]any)
			for key := range m {
				if key != "document_id" && key != "storage_state" && key != "linked_at" {
					t.Fatal("an attachment exposed", key)
				}
			}
			out[m["document_id"].(string)] = m["storage_state"].(string)
		}
		return out
	}
	view := read()
	if got := attachments(view); len(got) != 1 || got[photo] != "UPLOADING" || view["state"] != "REPORTED" || view["revision"] != float64(1) || view["data_mode"] != "SYNTHETIC" {
		t.Fatal("pending photo", got, view["state"])
	}
	for _, private := range []string{"content_base64", "capability", "sha256", "unit_id", "cost_centre_id", "operation_id", "payload_digest"} {
		if _, ok := view[private]; ok {
			t.Fatal("the report read exposed", private)
		}
	}
	// A failed upload stays visibly failed; the report does not change.
	for _, step := range []string{`UPDATE org_documents SET storage_state='RECEIVED',storage_revision=2 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_documents SET storage_state='FAILED',storage_revision=3,storage_reason='UNSUPPORTED_CONTENT' WHERE org_id=$1 AND id=$2`} {
		if err := unitSchemaExec(t, x.legalFixtureState, step, org, photo); err != nil {
			t.Fatal(err)
		}
	}
	stored := declare(reporter)
	link(reporter, stored, report, 201, "", "")
	for _, step := range []string{`UPDATE org_documents SET storage_state='RECEIVED',storage_revision=2 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_documents SET storage_state='STORED',storage_revision=3 WHERE org_id=$1 AND id=$2`} {
		if err := unitSchemaExec(t, x.legalFixtureState, step, org, stored); err != nil {
			t.Fatal(err)
		}
	}
	retry := declare(reporter)
	if err := unitSchemaExec(t, x.legalFixtureState, `UPDATE org_documents SET storage_state='RECEIVED',storage_revision=2 WHERE org_id=$1 AND id=$2`, org, retry); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, x.legalFixtureState, `UPDATE org_documents SET storage_state='FAILED',storage_revision=3,storage_reason='UNSUPPORTED_CONTENT' WHERE org_id=$1 AND id=$2`, org, retry); err != nil {
		t.Fatal(err)
	}
	link(reporter, retry, report, 409, "CONFLICT", "")
	view = read()
	if got := attachments(view); len(got) != 2 || got[photo] != "FAILED" || got[stored] != "STORED" || view["state"] != "REPORTED" || view["revision"] != float64(1) {
		t.Fatal("photo states", got, view["state"])
	}
	byTarget := items(call(reporter, docs+"/links/by-target", map[string]any{"target": map[string]any{"kind": "INCIDENT", "id": report}, "limit": 10}, 200, "", ""))
	if len(byTarget) != 2 {
		t.Fatal("links of the report", byTarget)
	}
	call(owner, docs+"/links/by-target", map[string]any{"target": map[string]any{"kind": "INCIDENT", "id": report}}, 403, "FORBIDDEN", "check.incident.report")
	// Handling through the composed route (B-BR-EMP-373).
	decide := base + "/" + report + "/decide"
	op := onboardingID(t)
	acknowledged := call(owner, decide, decision(1, "ACKNOWLEDGE", nil, op), 200, "", "")
	if acknowledged["state"] != "ACKNOWLEDGED" || acknowledged["revision"] != float64(2) || acknowledged["replayed"] != false || acknowledged["action"] != "ACKNOWLEDGE" {
		t.Fatal("acknowledged", acknowledged)
	}
	if replay := call(owner, decide, decision(1, "ACKNOWLEDGE", nil, op), 200, "", ""); replay["replayed"] != true || replay["recorded_at"] != acknowledged["recorded_at"] {
		t.Fatal("replay", replay)
	}
	call(owner, decide, decision(1, "TREAT", "Oficina agendada", op), 409, "OPERATION_ID_REUSED", "")
	call(owner, decide, decision(1, "TREAT", "Oficina agendada", onboardingID(t)), 409, "CONFLICT", "")
	call(owner, decide, decision(2, "ACKNOWLEDGE", nil, onboardingID(t)), 409, "CONFLICT", "")
	call(owner, decide, decision(2, "ACKNOWLEDGE", "Ciente", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, decide, decision(2, "RESOLVE", "   ", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, decide, decision(2, "RESOLVE", strings.Repeat("n", 501), onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, decide, decision(2, "OPEN", nil, onboardingID(t)), 422, "VALIDATION_FAILED", "")
	dismissItself := decision(2, "DISMISS", "Duplicado", onboardingID(t))
	dismissItself["duplicate_of"] = report
	call(owner, decide, dismissItself, 422, "VALIDATION_FAILED", "")
	call(owner, decide, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "action": "ACKNOWLEDGE"}, 400, "MALFORMED", "")
	call(owner, decide, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "action": "RESOLVE", "note": nil}, 400, "MALFORMED", "")
	call(reporter, decide, decision(2, "RESOLVE", "Resolvido", onboardingID(t)), 409, "SELF_APPROVAL", "")
	call(owner, "/v1/orgs/"+onboardingID(t)+"/incidents/reports/"+report+"/decide", decision(2, "RESOLVE", "Resolvido", onboardingID(t)), 404, "NOT_FOUND", "")
	call(owner, base+"/"+onboardingID(t)+"/read", map[string]any{}, 404, "NOT_FOUND", "")
	call(owner, base+"/"+x.foreignReport(t)+"/decide", decision(1, "ACKNOWLEDGE", nil, onboardingID(t)), 404, "NOT_FOUND", "")
	if resolved := call(owner, decide, decision(2, "RESOLVE", "Lanterna trocada", onboardingID(t)), 200, "", ""); resolved["state"] != "RESOLVED" || resolved["revision"] != float64(3) {
		t.Fatal("resolved", resolved)
	}
	view = read()
	history, _ := view["history"].([]any)
	if view["state"] != "RESOLVED" || len(history) != 2 || history[1].(map[string]any)["note"] != "Lanterna trocada" || history[0].(map[string]any)["action"] != "ACKNOWLEDGE" {
		t.Fatal("handling history", view["state"], history)
	}
	if got := attachments(view); got[photo] != "FAILED" || got[stored] != "STORED" {
		t.Fatal("handling changed a document state", got)
	}
	// The reporter sees its report's new state, never the handler's notes.
	if own := items(call(reporter, base+"/mine", map[string]any{"limit": 10}, 200, "", "")); own[0]["state"] != "RESOLVED" {
		t.Fatal("own list state", own)
	} else if _, ok := own[0]["history"]; ok {
		t.Fatal("the own list exposed the handling history")
	}
	// BR-CHECK-018: nothing was booked.
	if after := x.costRows(t); !sameCounts(before, after) {
		t.Fatal("the incident routes changed cost rows", before, after)
	}
}

// assignDriver gives the driver an assignment on the vehicle.
func (x handlingState) assignDriver(t *testing.T, d syncFixtureState) {
	t.Helper()
	x.assign(t, d.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
}
