//go:build integration

package adapters_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	exportpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	exportapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// syntheticPlanRows is a second test-only kind whose data permission
// (subs.plan.read) AUDITOR does not hold while FINANCE does (EMP-AUTH-011).
type syntheticPlanRows struct{ syntheticExportRows }

func (syntheticPlanRows) Kind() string                           { return "test.synthetic_plan_rows" }
func (syntheticPlanRows) ReadPermission() permissions.Permission { return "subs.plan.read" }

// auditorRouter mounts the complete enterprise suite with private document
// and export storage, both synthetic kinds and the runtime grants every
// routed module needs, and returns the production export build dispatcher.
func auditorRouter(t *testing.T, fx deviceFixtureState) (chi.Router, *jobs.Dispatcher) {
	t.Helper()
	grantEmployeeTables(t, fx.legalFixtureState)
	_ = attachmentStore(t, fx.legalFixtureState)
	grantChecklistTables(t, fx.legalFixtureState)
	grantMissionTables(t, fx.legalFixtureState)
	_ = meterStore(t, fx.legalFixtureState)
	fx.f.GrantMeterReviews(t)
	fx.f.GrantMembershipAccessCommands(t)
	periodGrant(t, fx.f)
	exportGrant(t, fx)
	documentGrants(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands,org_role_commands TO "+role+"; GRANT SELECT,INSERT,UPDATE ON org_ownership_transfers,identity_rate_windows TO "+role+
		"; GRANT SELECT,INSERT ON org_notification_recipients,org_notification_deliveries,org_notification_commands TO "+role+
		"; GRANT SELECT,INSERT,UPDATE ON org_notification_requests,org_notification_notices,org_notification_preferences,org_notification_budgets TO "+role); err != nil {
		t.Fatal(err)
	}
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return value
	}
	documents := mediapg.StorageConfig{Endpoint: required("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: required("ANPFUEL_TEST_STORAGE_BUCKET"), Region: required("ANPFUEL_TEST_STORAGE_REGION"),
		Credentials: storage.Credentials{AccessKeyID: required("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: required("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	options := enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{67}, 32), Pool: fx.f.Runtime, Accounts: &runtimeAccounts,
		DocumentStorage: &documents, ExportStorage: exportStorageConfig(t), ExportFormats: []exportapp.Format{syntheticExportRows{}, syntheticPlanRows{}}}
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, options); err != nil {
		t.Fatal(err)
	}
	handlers, err := enterprisesuite.ExportHandlers(options)
	if err != nil || len(handlers) != 2 {
		t.Fatal("export queue handlers", err)
	}
	registered := map[string]jobs.Handler{}
	for _, h := range handlers {
		registered[h.Kind()] = h
	}
	return router, &jobs.Dispatcher{Queue: jobs.NewQueue(fx.f.Runtime), Handlers: registered, WorkerID: "auditor-http-test", Kind: exportpg.BuildKind, LeaseTTL: time.Minute, RetryDelay: time.Millisecond}
}

// auditorAddRole adds one more ORG assignment without ending the others, so a
// member can hold the union of two roles (BR-AUTH-003).
func auditorAddRole(t *testing.T, fx deviceFixtureState, actor matrixPerson, role string) string {
	t.Helper()
	id, org := onboardingID(t), fx.scope.OrgID().String()
	if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,'ORG',$1,$5,$6,'Synthetic auditor gating')`, org, id, actor.member, role, time.Now().UTC(), fx.member); err != nil {
		t.Fatal(err)
	}
	return id
}

func auditorEndRole(t *testing.T, fx deviceFixtureState, assignment string) {
	t.Helper()
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2 AND ended_at IS NULL`, fx.scope.OrgID().String(), assignment); err != nil {
		t.Fatal(err)
	}
}

// auditorStoredDocument declares an owner document and uploads its real bytes
// through the capability ingress, so content routes serve actual bytes.
func auditorStoredDocument(t *testing.T, fx deviceFixtureState, router chi.Router, owner matrixPerson) (string, []byte) {
	t.Helper()
	raw, err := os.ReadFile("../../evidence/adapters/media/testdata/valid-4x4.jpg")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	base := "/v1/orgs/" + fx.scope.OrgID().String() + "/documents/"
	id := onboardingID(t)
	declaration := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "document_id": id, "kind": "RECEIPT", "original": map[string]any{"sha256": hex.EncodeToString(sum[:]), "bytes": len(raw), "media_type": "image/jpeg"}}
	created := matrixCall(t, fx, router, owner, base+"uploads/create", declaration, 201, "", "", nil)
	capability, _ := created["upload"].(map[string]any)["capability"].(string)
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+base+id+"/content/upload", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "image/jpeg")
	r.Header.Set(orgapi.DocumentCapabilityHeader, capability)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("owner document upload", w.Code)
	}
	return id, raw
}

// auditorExtraState digests every other organization table a routed write
// could touch (structure, legal, invitations, identity, devices, lifecycle,
// settings heads, capabilities, notices, outbox and the roots), extending the
// shared matrix snapshot for the auditor walk.
func auditorExtraState(t *testing.T, fx deviceFixtureState) string {
	t.Helper()
	var parts []string
	for _, table := range []string{"organizations", "org_capabilities", "org_settings_heads", "org_units", "org_unit_commands", "org_cost_centres", "org_cost_centre_codes", "org_cost_centre_commands",
		"org_teams", "org_team_links", "org_team_commands", "org_legal_entities", "org_establishments", "org_legal_commands", "org_invitations", "org_invitation_commands",
		"org_employee_identities", "org_employee_commands", "org_employee_merge_commands", "org_employee_rehire_commands", "org_membership_access_commands", "org_offboarding_commands",
		"org_device_enrollments", "org_device_commands", "org_asset_lifecycle_events", "org_asset_unit_periods", "org_asset_cost_centre_periods", "org_notification_notices",
		"org_notification_preferences", "org_event_intents"} {
		var digest string
		query := `SELECT md5(COALESCE(string_agg(to_jsonb(x)::text, '|' ORDER BY to_jsonb(x)::text), ''))||':'||count(*) FROM ` + pgx.Identifier{table}.Sanitize() + ` x`
		if err := fx.f.Admin.QueryRow(t.Context(), query).Scan(&digest); err != nil {
			t.Fatal(table, err)
		}
		parts = append(parts, table+"="+digest)
	}
	return strings.Join(parts, ",")
}

type auditorRoute struct {
	permission, template string
	write                bool
	example              map[string]any
}

// auditorRoutes reads every normal routed organization route of the
// independent inventory with its OpenAPI write flag and request example.
func auditorRoutes(t *testing.T) []auditorRoute {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("auditor test location unavailable")
	}
	contracts := filepath.Join(filepath.Dir(source), "../../../../../contracts")
	raw, err := os.ReadFile(filepath.Join(contracts, "testdata", "enterprise", "authorization-routes-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Permissions []struct {
			Permission, Status string
			Routes             []string `json:"routes"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &inventory); err != nil {
		t.Fatal(err)
	}
	document, err := openapi3.NewLoader().LoadFromFile(filepath.Join(contracts, "openapi", "v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var routes []auditorRoute
	for _, entry := range inventory.Permissions {
		if entry.Status != "ROUTED" {
			continue
		}
		for _, route := range entry.Routes {
			template := strings.TrimPrefix(route, "POST ")
			item := document.Paths.Value(template)
			if item == nil || item.Post == nil {
				t.Fatal("inventoried route absent from the contract", route)
			}
			var rule struct {
				Permission string `json:"permission"`
				Write      bool   `json:"write"`
			}
			encoded, err := json.Marshal(item.Post.Extensions["x-abastevo-organization"])
			if err != nil || json.Unmarshal(encoded, &rule) != nil || rule.Permission != entry.Permission {
				t.Fatal("contract rule and inventory disagree", route)
			}
			example := map[string]any{}
			if body := item.Post.RequestBody; body != nil && body.Value != nil {
				if media := body.Value.Content.Get("application/json"); media != nil && media.Example != nil {
					encoded, err := json.Marshal(media.Example)
					if err != nil || json.Unmarshal(encoded, &example) != nil {
						t.Fatal("contract request example", route)
					}
				}
			}
			routes = append(routes, auditorRoute{permission: entry.Permission, template: template, write: rule.Write, example: example})
		}
	}
	return routes
}

var auditorTemplateParam = regexp.MustCompile(`\{[a-z_]+\}`)

// auditorPositiveRoutes is the independent literal of the routes the
// documented AUDITOR column grants at this head (B-BR-EMP-247).
var auditorPositiveRoutes = []string{
	"/v1/orgs/{org_id}/checklists/assets/{asset_id}/applicable",
	"/v1/orgs/{org_id}/documents/{document_id}/content/access",
	"/v1/orgs/{org_id}/documents/{document_id}/content/read",
	"/v1/orgs/{org_id}/employees/list",
	"/v1/orgs/{org_id}/employees/{employee_id}/read",
	"/v1/orgs/{org_id}/fleet/assets/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/assignments/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/attachments/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/availability/read",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/blocks/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/cost-centre-allocations/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/lifecycle/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/latest",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/readings/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/read",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/reservations/list",
	"/v1/orgs/{org_id}/fleet/assets/{asset_id}/unit-allocations/list",
	"/v1/orgs/{org_id}/fleet/assignments/mine/list",
	"/v1/orgs/{org_id}/ownership-transfer/accept",
	"/v1/orgs/{org_id}/ownership-transfer/cancel",
	"/v1/orgs/{org_id}/ownership-transfer/read",
	"/v1/orgs/{org_id}/reports/exports/list",
	"/v1/orgs/{org_id}/reports/exports/request",
	"/v1/orgs/{org_id}/reports/exports/{export_id}/access",
	"/v1/orgs/{org_id}/reports/exports/{export_id}/content",
	"/v1/orgs/{org_id}/reports/exports/{export_id}/read",
	"/v1/orgs/{org_id}/settings/history",
	"/v1/orgs/{org_id}/settings/read",
	"/v1/orgs/{org_id}/settings/revision/read",
	"/v1/orgs/{org_id}/trips/{trip_id}/track/read",
}

// The only positive AUDITOR write routes: the export request (an audited
// read-derived job) and the personal ownership consent (EDR-166).
var auditorPositiveWrites = []string{
	"/v1/orgs/{org_id}/ownership-transfer/accept",
	"/v1/orgs/{org_id}/ownership-transfer/cancel",
	"/v1/orgs/{org_id}/reports/exports/request",
}

// B-BR-EMP-247, BUC-EMP-126, EDR-166: the AUDITOR on every routed normal
// organization route of the actual composed company HTTP entry and ordinary
// runtime PostGIS, plus the personal boundaries and foreign records.
func TestAuditorReadWriteMatrixActualCompanyHTTP(t *testing.T) {
	_, oracle := matrixOracle(t)
	routes := auditorRoutes(t)
	fx := devicesFixture(t)
	router, dispatcher := auditorRouter(t, fx)
	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	auditor, target, colleague := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, colleague, "FINANCE", "ORG", "ACTIVE")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	otherOrg := other.scope.OrgID().String()

	// Real source records for every positive route, created by the owner.
	assetBody := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "VEHICLE", "name": "Synthetic audited vehicle", "fleet_number": "AUD-01", "plate": "AUD1T23", "serial_number": "", "usage_basis": "DISTANCE",
		"category": map[string]any{"new_label": "Synthetic audited fleet"}, "tanks": []any{map[string]any{"role": "MAIN", "capacity_milli": "275000", "products": []string{"DIESEL_S10"}}}}
	asset, _ := matrixCall(t, fx, router, owner, base+"/fleet/assets/create", assetBody, 201, "", "", nil)["asset_id"].(string)
	installed := matrixCall(t, fx, router, owner, base+"/fleet/assets/"+asset+"/meters/install", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "opening_reading_id": onboardingID(t), "kind": "ODOMETER",
		"label": "Synthetic audited odometer", "opening_value": "123456", "method": "TYPED", "occurred_at": time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Format(time.RFC3339Nano), "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400}, 201, "", "", nil)
	meter, _ := installed["meter_id"].(string)
	document, original := auditorStoredDocument(t, fx, router, owner)
	if asset == "" || meter == "" {
		t.Fatal("owner fixtures missing")
	}
	reading, _ := installed["opening_reading_id"].(string)
	values := map[string]string{"{org_id}": org, "{membership_id}": target.member, "{employee_id}": target.employee, "{asset_id}": asset, "{meter_id}": meter, "{reading_id}": reading, "{document_id}": document, "{period}": "2019-03"}
	fill := func(template string) string {
		return auditorTemplateParam.ReplaceAllStringFunc(template, func(name string) string {
			if value, ok := values[name]; ok {
				return value
			}
			return onboardingID(t) // A refused route never resolves its target.
		})
	}
	exercised := map[string]int{}
	call := func(template string, body any, want int, code, key string) map[string]any {
		t.Helper()
		exercised[template]++
		return matrixCall(t, fx, router, auditor, fill(template), body, want, code, key, nil)
	}

	// Documented reads: success with an unchanged private source snapshot.
	before, beforeExtra := matrixPrivateState(t, fx), auditorExtraState(t, fx)
	if items, _ := call("/v1/orgs/{org_id}/employees/list", map[string]any{"limit": 20}, 200, "", "")["items"].([]any); len(items) < 3 {
		t.Fatal("auditor employee list", len(items))
	}
	call("/v1/orgs/{org_id}/employees/{employee_id}/read", map[string]any{}, 200, "", "")
	call("/v1/orgs/{org_id}/settings/read", map[string]any{}, 200, "", "")
	call("/v1/orgs/{org_id}/settings/history", map[string]any{"limit": 1}, 200, "", "")
	call("/v1/orgs/{org_id}/settings/revision/read", map[string]any{"revision": 1}, 200, "", "")
	if items, _ := call("/v1/orgs/{org_id}/fleet/assets/list", map[string]any{"limit": 20}, 200, "", "")["items"].([]any); len(items) != 1 {
		t.Fatal("auditor asset list", len(items))
	}
	if read := call("/v1/orgs/{org_id}/fleet/assets/{asset_id}/read", map[string]any{}, 200, "", ""); read["id"] != asset {
		t.Fatal("auditor asset read", read["id"])
	}
	for _, template := range []string{"/v1/orgs/{org_id}/fleet/assets/{asset_id}/assignments/list", "/v1/orgs/{org_id}/fleet/assignments/mine/list", "/v1/orgs/{org_id}/fleet/assets/{asset_id}/lifecycle/list", "/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/list", "/v1/orgs/{org_id}/fleet/assets/{asset_id}/unit-allocations/list", "/v1/orgs/{org_id}/fleet/assets/{asset_id}/cost-centre-allocations/list", "/v1/orgs/{org_id}/fleet/assets/{asset_id}/reservations/list", "/v1/orgs/{org_id}/fleet/assets/{asset_id}/blocks/list", "/v1/orgs/{org_id}/fleet/assets/{asset_id}/attachments/list"} {
		call(template, map[string]any{"limit": 20}, 200, "", "")
	}
	// EMP-CHECK-001: the applicable template read is a fleet.asset.read; no
	// template exists, so no checklist is required.
	if applicable := call("/v1/orgs/{org_id}/checklists/assets/{asset_id}/applicable", map[string]any{"context": "DEPARTURE"}, 200, "", ""); applicable["required"] != false || applicable["match"] != "NONE" {
		t.Fatal("auditor applicable checklist read", applicable)
	}
	if availability := call("/v1/orgs/{org_id}/fleet/assets/{asset_id}/availability/read", map[string]any{}, 200, "", ""); availability["availability"] != "UNAVAILABLE" {
		t.Fatal("auditor availability of the draft asset", availability)
	}
	if latest, _ := call("/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/latest", map[string]any{}, 200, "", "")["latest"].(map[string]any); latest["value"] != "123456" {
		t.Fatal("auditor latest reading", latest)
	}
	// EMP-METER-004: the reading history is a documented fleet.asset.read.
	if items, _ := call("/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/readings/list", map[string]any{"limit": 20}, 200, "", "")["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != reading || items[0].(map[string]any)["state"] != "VALID" {
		t.Fatal("auditor reading history", items)
	}
	if matrixPrivateState(t, fx) != before || auditorExtraState(t, fx) != beforeExtra {
		t.Fatal("a documented auditor read changed private source state")
	}

	// EMP-GPS-012: the AUDITOR reads a recorded track (RECORDED_TRACKS_ONLY) of
	// another member's trip, recorded as issued and accessed before it is
	// served; it holds no live location grant.
	tracked := trackingFixtureOn(t, tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, fx))))
	tracked.recordedTrack(t)
	trackReadGrants(t, tracked)
	values["{trip_id}"] = tracked.trip
	if page := call("/v1/orgs/{org_id}/trips/{trip_id}/track/read", map[string]any{}, 200, "", ""); page["trip_id"] != tracked.trip || len(page["segments"].([]any)) != 2 {
		t.Fatal("auditor recorded-track read", page["trip_id"])
	}
	if issued, served := tracked.trackAccesses(t, auditor.member, tracked.trip); issued != 1 || served != 1 {
		t.Fatal("auditor track read not recorded once per stage", issued, served)
	}
	if cell := oracle["gps.location.read_live"]["AUDITOR"]; cell != "-" {
		t.Fatal("an auditor holds a live location grant", cell)
	}

	// B-BR-EMP-250: document bytes only under the content permission, audited.
	contentEvents := func(member, stage string) int {
		t.Helper()
		var n int
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_sensitive_access_events WHERE org_id=$1 AND membership_id=$2 AND category='DOCUMENT_CONTENT' AND stage=$3`, org, member, stage).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	grant, _ := call("/v1/orgs/{org_id}/documents/{document_id}/content/access", map[string]any{}, 200, "", "")["capability"].(string)
	served := call("/v1/orgs/{org_id}/documents/{document_id}/content/read", map[string]any{"capability": grant}, 200, "", "")
	if bytes, err := base64.StdEncoding.DecodeString(served["content_base64"].(string)); err != nil || !slices.Equal(bytes, original) || contentEvents(auditor.member, "ISSUED") != 1 || contentEvents(auditor.member, "SERVED") != 1 {
		t.Fatal("auditor content read was not exact and recorded once per stage", err)
	}

	// B-BR-EMP-248/249: an export of a kind whose data permission the auditor
	// holds; job metadata, capability and content are recorded per stage.
	created := call("/v1/orgs/{org_id}/reports/exports/request", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "test.synthetic_rows", "parameters": map[string]any{"rows": 2}}, 202, "", "")
	export, _ := created["export"].(map[string]any)["export_id"].(string)
	values["{export_id}"] = export
	if found, err := dispatcher.RunOnce(t.Context()); err != nil || !found {
		t.Fatal("auditor export not dispatched", err)
	}
	if items, _ := call("/v1/orgs/{org_id}/reports/exports/list", map[string]any{"limit": 20}, 200, "", "")["items"].([]any); len(items) != 1 {
		t.Fatal("auditor export list", len(items))
	}
	if read := call("/v1/orgs/{org_id}/reports/exports/{export_id}/read", map[string]any{}, 200, "", ""); read["state"] != "READY" || read["row_count"] != float64(2) {
		t.Fatal("auditor export read", read["state"])
	}
	capability, _ := call("/v1/orgs/{org_id}/reports/exports/{export_id}/access", map[string]any{}, 200, "", "")["capability"].(string)
	file := call("/v1/orgs/{org_id}/reports/exports/{export_id}/content", map[string]any{"capability": capability}, 200, "", "")
	if raw, err := base64.StdEncoding.DecodeString(file["content_base64"].(string)); err != nil || string(raw) != "row_id,value_centavos\n1,100\n2,200\n" {
		t.Fatal("auditor export content", err)
	}
	var issued, delivered int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE stage='ISSUED'),count(*) FILTER(WHERE stage='SERVED') FROM org_sensitive_access_events WHERE category='EXPORT_FILE' AND membership_id=$1`, auditor.member).Scan(&issued, &delivered); err != nil || issued != 1 || delivered != 1 {
		t.Fatal("auditor export download not recorded once per stage", issued, delivered)
	}

	// EDR-166: the consent key reaches only a proposal addressed to the
	// auditor; any other proposal is opaque NOT_FOUND and unchanged.
	propose := func(p matrixPerson) string {
		t.Helper()
		v := matrixCall(t, fx, router, owner, base+"/ownership-transfer/propose", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "target_membership_id": p.member, "step_down_to_admin": true}, 200, "", "", nil)
		return v["transfer"].(map[string]any)["transfer_id"].(string)
	}
	foreignProposal := propose(target)
	call("/v1/orgs/{org_id}/ownership-transfer/read", map[string]any{}, 404, "NOT_FOUND", "")
	call("/v1/orgs/{org_id}/ownership-transfer/read", map[string]any{"transfer_id": foreignProposal}, 404, "NOT_FOUND", "")
	call("/v1/orgs/{org_id}/ownership-transfer/accept", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "transfer_id": foreignProposal}, 404, "NOT_FOUND", "")
	call("/v1/orgs/{org_id}/ownership-transfer/cancel", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "transfer_id": foreignProposal}, 404, "NOT_FOUND", "")
	matrixCall(t, fx, router, owner, base+"/ownership-transfer/cancel", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "transfer_id": foreignProposal}, 200, "", "", nil)
	ownProposal := propose(auditor)
	if read := call("/v1/orgs/{org_id}/ownership-transfer/read", map[string]any{"transfer_id": ownProposal}, 200, "", ""); read["state"] != "PENDING" || read["target_membership_id"] != auditor.member {
		t.Fatal("auditor cannot see the proposal addressed to it", read["state"])
	}
	if declined := call("/v1/orgs/{org_id}/ownership-transfer/cancel", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "transfer_id": ownProposal}, 200, "", ""); declined["transfer"].(map[string]any)["state"] != "DECLINED" {
		t.Fatal("auditor could not decline its own proposal", declined["transfer"])
	}
	call("/v1/orgs/{org_id}/ownership-transfer/accept", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "transfer_id": ownProposal}, 409, "CONFLICT", "")
	var roles []string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT array_agg(role ORDER BY role) FROM org_role_assignments WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, org, auditor.member).Scan(&roles); err != nil || !slices.Equal(roles, []string{"AUDITOR"}) {
		t.Fatal("auditor role set changed", roles, err)
	}

	// Every other routed normal route: FORBIDDEN with the route key and an
	// unchanged private-state snapshot (checked by matrixCall), bodies taken
	// from the canonical contract examples.
	positive, denied, deniedWrites := 0, 0, 0
	var oraclePositive []string
	for _, route := range routes {
		cell := oracle[route.permission]["AUDITOR"]
		if cell != "-" {
			positive++
			oraclePositive = append(oraclePositive, route.template)
			if exercised[route.template] == 0 {
				t.Fatal("documented auditor route lacks a delivered-record case", route.template)
			}
			if route.write && !slices.Contains(auditorPositiveWrites, route.template) {
				t.Fatal("documented auditor permission reaches an unexpected write route", route.template)
			}
			continue
		}
		if exercised[route.template] != 0 {
			t.Fatal("denied route exercised as positive", route.template)
		}
		t.Run("denied"+strings.TrimPrefix(route.template, "/v1/orgs/{org_id}"), func(t *testing.T) {
			extra := auditorExtraState(t, fx)
			matrixCall(t, fx, router, auditor, fill(route.template), route.example, 403, "FORBIDDEN", route.permission, nil)
			if auditorExtraState(t, fx) != extra {
				t.Fatal("refused auditor request changed another module's private state")
			}
		})
		denied++
		if route.write {
			deniedWrites++
		}
	}
	slices.Sort(oraclePositive)
	if !slices.Equal(oraclePositive, auditorPositiveRoutes) {
		t.Fatal("independent PSV and literal auditor route sets disagree", oraclePositive)
	}
	if len(routes) != 157 || positive != 31 || denied != 126 {
		t.Fatal("auditor route coverage drift", len(routes), positive, denied)
	}

	// BR-TEN-006: foreign records and organizations stay NOT_FOUND.
	var foreignEmployee string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 ORDER BY id LIMIT 1`, otherOrg).Scan(&foreignEmployee); err != nil {
		t.Fatal(err)
	}
	colleagueExport := matrixCall(t, fx, router, colleague, base+"/reports/exports/request", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "test.synthetic_rows", "parameters": map[string]any{"rows": 1}}, 202, "", "", nil)["export"].(map[string]any)["export_id"].(string)
	for path, body := range map[string]any{
		"/v1/orgs/" + otherOrg + "/employees/list":                      map[string]any{},
		"/v1/orgs/" + otherOrg + "/settings/read":                       map[string]any{},
		base + "/employees/" + foreignEmployee + "/read":                map[string]any{},
		base + "/fleet/assets/" + onboardingID(t) + "/read":             map[string]any{},
		base + "/documents/" + onboardingID(t) + "/content/access":      map[string]any{},
		base + "/reports/exports/" + colleagueExport + "/read":          map[string]any{},
		base + "/reports/exports/" + colleagueExport + "/access":        map[string]any{},
		"/v1/orgs/" + otherOrg + "/reports/exports/" + export + "/read": map[string]any{},
	} {
		matrixCall(t, fx, router, auditor, path, body, 404, "NOT_FOUND", "", nil)
	}

	// Personal boundaries are role-independent: the auditor reaches its own
	// inbox and never another member's inbox, devices or leave.
	member := base + "/memberships/"
	matrixCall(t, fx, router, auditor, member+auditor.member+"/notifications/self/list", map[string]any{"limit": 20, "unread_only": false}, 200, "", "", nil)
	matrixCall(t, fx, router, auditor, member+auditor.member+"/notification-preferences/self/read", map[string]any{}, 200, "", "", nil)
	matrixCall(t, fx, router, auditor, member+target.member+"/notifications/self/list", map[string]any{"limit": 20, "unread_only": false}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, auditor, member+target.member+"/notification-preferences/self/read", map[string]any{}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, auditor, member+target.member+"/devices/self/list", map[string]any{"limit": 20}, 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, auditor, member+target.member+"/leave/read", map[string]any{}, 404, "NOT_FOUND", "", nil)
	t.Logf("AUDITOR over the actual composed HTTP entry: %d routed normal routes, %d documented positive (%d write-shaped: export request and own consent), %d FORBIDDEN (%d writes, %d reads) with unchanged private state; foreign records/organizations NOT_FOUND; personal boundaries confined to the own membership",
		len(routes), positive, len(auditorPositiveWrites), denied, deniedWrites, denied-deniedWrites)
}

// B-BR-EMP-248/249/250, BUC-EMP-127: export and sensitive-content gates on
// the actual composed HTTP entry, the actual company authority and the
// production export worker.
func TestExportAndContentGatesActualCompanyHTTP(t *testing.T) {
	fx := devicesFixture(t)
	router, dispatcher := auditorRouter(t, fx)
	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org
	exports := base + "/reports/exports"
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	auditor, supervisor, unitAuditor, dual, revoked := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, supervisor, "SUPERVISOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, unitAuditor, "AUDITOR", "UNIT", "ACTIVE")
	matrixAssign(t, fx, dual, "AUDITOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, revoked, "AUDITOR", "ORG", "ACTIVE")
	request := func(kind string, rows int) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": kind, "parameters": map[string]any{"rows": rows}}
	}
	state := func(export string) (string, string, bool) {
		t.Helper()
		var current, failure string
		var object bool
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT state,COALESCE(failure_code,''),object_sha256 IS NOT NULL FROM org_export_jobs WHERE id=$1`, export).Scan(&current, &failure, &object); err != nil {
			t.Fatal(err)
		}
		return current, failure, object
	}
	run := func() {
		t.Helper()
		if found, err := dispatcher.RunOnce(t.Context()); err != nil || !found {
			t.Fatal("export build not dispatched", err)
		}
	}
	accepted := func(actor matrixPerson, body map[string]any) string {
		t.Helper()
		return matrixCall(t, fx, router, actor, exports+"/request", body, 202, "", "", nil)["export"].(map[string]any)["export_id"].(string)
	}

	// Broad read never implies export; export needs organization coverage.
	matrixCall(t, fx, router, supervisor, exports+"/request", request("test.synthetic_rows", 1), 403, "FORBIDDEN", "rpt.export.run", nil)
	matrixCall(t, fx, router, unitAuditor, exports+"/request", request("test.synthetic_rows", 1), 403, "FORBIDDEN", "rpt.export.run", nil)
	// rpt.export.run never implies the kind's data permission: refused in the
	// request transaction, nothing written (matrixCall compares the snapshot).
	matrixCall(t, fx, router, auditor, exports+"/request", request("test.synthetic_plan_rows", 1), 403, "FORBIDDEN", "subs.plan.read", nil)
	stale := request("test.synthetic_rows", 1)
	if _, err := fx.f.Runtime.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=authenticated_at-interval '20 minutes',created_at=created_at-interval '20 minutes',expires_at=expires_at-interval '20 minutes' WHERE id=$1`, auditor.session); err != nil {
		t.Fatal(err)
	}
	matrixCall(t, fx, router, auditor, exports+"/request", stale, 401, "FRESH_AUTH_REQUIRED", "", nil)
	if _, err := fx.f.Runtime.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=authenticated_at+interval '20 minutes',created_at=created_at+interval '20 minutes',expires_at=expires_at+interval '20 minutes' WHERE id=$1`, auditor.session); err != nil {
		t.Fatal(err)
	}

	// The union of AUDITOR and FINANCE holds both; losing only the data
	// permission before the worker attempt fails the job closed.
	finance := auditorAddRole(t, fx, dual, "FINANCE")
	lost := accepted(dual, request("test.synthetic_plan_rows", 1))
	auditorEndRole(t, fx, finance)
	run()
	if current, failure, object := state(lost); current != "FAILED" || failure != "EXPORT_RIGHTS_LOST" || object {
		t.Fatal("worker produced without the kind's data permission", current, failure)
	}
	// Losing the data permission after READY closes access and content.
	finance = auditorAddRole(t, fx, dual, "FINANCE")
	ready := accepted(dual, request("test.synthetic_plan_rows", 2))
	run()
	if current, _, _ := state(ready); current != "READY" {
		t.Fatal("export with both permissions not published", current)
	}
	capability := matrixCall(t, fx, router, dual, exports+"/"+ready+"/access", map[string]any{}, 200, "", "", nil)["capability"].(string)
	auditorEndRole(t, fx, finance)
	matrixCall(t, fx, router, dual, exports+"/"+ready+"/access", map[string]any{}, 403, "FORBIDDEN", "subs.plan.read", nil)
	matrixCall(t, fx, router, dual, exports+"/"+ready+"/content", map[string]any{"capability": capability}, 403, "FORBIDDEN", "subs.plan.read", nil)
	if read := matrixCall(t, fx, router, dual, exports+"/"+ready+"/read", map[string]any{}, 200, "", "", nil); read["state"] != "READY" {
		t.Fatal("job metadata needs rpt.export.run only", read["state"])
	}
	auditorAddRole(t, fx, dual, "FINANCE")
	content := matrixCall(t, fx, router, dual, exports+"/"+ready+"/content", map[string]any{"capability": capability}, 200, "", "", nil)
	if raw, err := base64.StdEncoding.DecodeString(content["content_base64"].(string)); err != nil || string(raw) != "row_id,value_centavos\n1,100\n2,200\n" {
		t.Fatal("restored permission did not serve the exact file", err)
	}
	var issued, served int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE stage='ISSUED'),count(*) FILTER(WHERE stage='SERVED') FROM org_sensitive_access_events WHERE category='EXPORT_FILE' AND membership_id=$1`, dual.member).Scan(&issued, &served); err != nil || issued != 1 || served != 1 {
		t.Fatal("refused delivery was recorded", issued, served)
	}

	// The auditor role revoked between request and worker run: fails closed.
	gone := accepted(revoked, request("test.synthetic_rows", 1))
	matrixAssign(t, fx, revoked, "AUDITOR", "ORG", "NONE")
	run()
	if current, failure, object := state(gone); current != "FAILED" || failure != "EXPORT_RIGHTS_LOST" || object {
		t.Fatal("revoked auditor export processed", current, failure)
	}
	matrixCall(t, fx, router, revoked, exports+"/"+gone+"/read", map[string]any{}, 403, "FORBIDDEN", "rpt.export.run", nil)

	// B-BR-EMP-250: broad read authority never reaches document bytes.
	document, _ := auditorStoredDocument(t, fx, router, owner)
	matrixCall(t, fx, router, supervisor, base+"/documents/"+document+"/content/access", map[string]any{}, 403, "FORBIDDEN", "doc.document.view_content", nil)
	matrixCall(t, fx, router, unitAuditor, base+"/documents/"+document+"/content/access", map[string]any{}, 403, "FORBIDDEN", "doc.document.view_content", nil)
	matrixCall(t, fx, router, auditor, base+"/documents/"+document+"/content/access", map[string]any{}, 200, "", "", nil)
	var supervisorRecords, auditorRecords int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE membership_id=$2),count(*) FILTER(WHERE membership_id=$3) FROM org_sensitive_access_events WHERE org_id=$1 AND category='DOCUMENT_CONTENT'`, org, supervisor.member, auditor.member).Scan(&supervisorRecords, &auditorRecords); err != nil || supervisorRecords != 0 || auditorRecords != 1 {
		t.Fatal("document content access not recorded exactly for its holder", supervisorRecords, auditorRecords)
	}
}

// B-BR-EMP-248/249, EDR-167: the actual export authority on ordinary runtime
// PostGIS fails closed for every input it does not fully prove.
func TestExportDataAuthorityFailsClosed(t *testing.T) {
	fx := devicesFixture(t)
	authority := &orgpg.ExportAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}
	person := func(role, scope string) matrixPerson {
		t.Helper()
		key, err := fx.accounts.CreateKeyAccount(t.Context(), "synauditor"+onboardingID(t)[:8])
		if err != nil {
			t.Fatal(err)
		}
		employee, member := teamDriver(t, fx.legalFixtureState, key.Account.ID)
		p := matrixPerson{account: key.Account.ID, member: member, employee: employee}
		matrixAssign(t, fx, p, role, scope, "ACTIVE")
		return p
	}
	auditor, unitAuditor, other := person("AUDITOR", "ORG"), person("AUDITOR", "UNIT"), person("FINANCE", "ORG")
	actor := func(p matrixPerson) exportapp.Actor {
		return exportapp.Actor{AccountID: p.account, SessionID: onboardingID(t)}
	}
	data := func(scope kernel.OrgScope, a exportapp.Actor, member string, read permissions.Permission) error {
		t.Helper()
		var result error
		if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			result = authority.CheckDataTx(t.Context(), tx, scope, a, member, read)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return result
	}
	requester := func(p matrixPerson, kind string, read permissions.Permission) error {
		t.Helper()
		var result error
		if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			result = authority.CheckRequesterTx(t.Context(), tx, fx.scope, exportapp.Requester{AccountID: p.account, MembershipID: p.member}, kind, read)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return result
	}
	denied := func(err error, key permissions.Permission) bool {
		var denial *permissions.PermissionDenial
		return errors.As(err, &denial) && denial.Permission() == key
	}
	if err := data(fx.scope, actor(auditor), auditor.member, "exp.expense.read"); err != nil {
		t.Fatal("documented auditor read refused", err)
	}
	if err := data(fx.scope, actor(auditor), auditor.member, "subs.plan.read"); !denied(err, "subs.plan.read") {
		t.Fatal("undocumented data permission admitted", err)
	}
	// A structural grant never covers an organization-level export.
	if err := data(fx.scope, actor(unitAuditor), unitAuditor.member, "exp.expense.read"); !denied(err, "exp.expense.read") {
		t.Fatal("UNIT-scoped read covered an organization export", err)
	}
	for name, read := range map[string]permissions.Permission{"content": "doc.document.view_content", "export": "rpt.export.run", "track": "gps.track.read", "write": "fleet.asset.manage", "unknown": "invented.record.read"} {
		if err := data(fx.scope, actor(auditor), auditor.member, read); !errors.Is(err, exportapp.ErrUnavailable) {
			t.Fatal("non-data permission checked as export data:", name, err)
		}
	}
	if err := data(fx.scope, actor(auditor), other.member, "exp.expense.read"); !errors.Is(err, access.ErrNoMembership) {
		t.Fatal("borrowed membership admitted", err)
	}
	if err := data(fx.scope, exportapp.Actor{}, auditor.member, "exp.expense.read"); !errors.Is(err, exportapp.ErrUnauthenticated) {
		t.Fatal("missing actor admitted", err)
	}
	foreignID, err := kernel.ParseOrgID(onboardingID(t))
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := kernel.NewOrgScope(foreignID)
	if err != nil {
		t.Fatal(err)
	}
	if err := data(foreign, actor(auditor), auditor.member, "exp.expense.read"); !errors.Is(err, exportapp.ErrUnavailable) {
		t.Fatal("transaction of another organization admitted", err)
	}
	if err := requester(auditor, "test.synthetic_rows", "exp.expense.read"); err != nil {
		t.Fatal("requester with both permissions refused", err)
	}
	if err := requester(auditor, "test.synthetic_plan_rows", "subs.plan.read"); !errors.Is(err, exportapp.ErrRightsLost) {
		t.Fatal("requester without the data permission kept its rights", err)
	}
	if err := requester(auditor, "test.synthetic_rows", "doc.document.view_content"); !errors.Is(err, exportapp.ErrUnavailable) {
		t.Fatal("content permission re-checked as export data", err)
	}
	if err := requester(unitAuditor, "test.synthetic_rows", "exp.expense.read"); !errors.Is(err, exportapp.ErrRightsLost) {
		t.Fatal("UNIT-scoped requester kept organization export rights", err)
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), auditor.member); err != nil {
		t.Fatal(err)
	}
	if err := data(fx.scope, actor(auditor), auditor.member, "exp.expense.read"); !errors.Is(err, access.ErrNoMembership) {
		t.Fatal("suspended membership admitted", err)
	}
	if err := requester(auditor, "test.synthetic_rows", "exp.expense.read"); !errors.Is(err, exportapp.ErrRightsLost) {
		t.Fatal("suspended requester kept its rights", err)
	}
}
