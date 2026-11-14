//go:build integration

package adapters_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry/metrics"
)

func documentGrants(t *testing.T, fx deviceFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_documents TO "+role+"; GRANT SELECT,INSERT ON org_audit_events,org_sensitive_access_events TO "+role+"; GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role+
		// EMP-DOC-014: declarations and received uploads queue their safety jobs.
		"; GRANT SELECT,INSERT ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
}

func matrixDocumentDeclaration(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "document_id": onboardingID(t), "kind": "RECEIPT",
		"original": map[string]any{"sha256": strings.Repeat("ab", 32), "bytes": 1024, "media_type": "image/jpeg"}}
}

// matrixStoredDocument declares an owner document through the actual route and
// advances its storage by the guarded transition graph; the authorization
// matrix needs a stored target, not a storage provider.
func matrixStoredDocument(t *testing.T, fx deviceFixtureState, router chi.Router, owner matrixPerson) string {
	t.Helper()
	documentGrants(t, fx)
	declaration := matrixDocumentDeclaration(t)
	matrixCall(t, fx, router, owner, "/v1/orgs/"+fx.scope.OrgID().String()+"/documents/uploads/create", declaration, 201, "", "", nil)
	id := declaration["document_id"].(string)
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_documents SET storage_state='RECEIVED',storage_revision=2 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), id); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_documents SET storage_state='STORED',storage_revision=3 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), id); err != nil {
		t.Fatal(err)
	}
	return id
}

type documentHTTP struct {
	fx       deviceFixtureState
	router   chi.Router
	logs     *bytes.Buffer
	registry *metrics.Registry
	raw      []byte
	org      string
}

func newDocumentHTTP(t *testing.T) *documentHTTP {
	t.Helper()
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return value
	}
	fx := devicesFixture(t)
	grantEmployeeTables(t, fx.legalFixtureState)
	documentGrants(t, fx)
	raw, err := os.ReadFile("../../evidence/adapters/media/testdata/valid-4x4.jpg")
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("SYNTHETIC-HTTP-DOCUMENT-ORIGINAL")
	raw = append(append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(marker) + 2)}, marker...), raw[2:]...)
	d := &documentHTTP{fx: fx, logs: &bytes.Buffer{}, registry: metrics.NewRegistry(), raw: raw, org: fx.scope.OrgID().String()}
	logger, err := telemetry.New(d.logs, "debug", telemetry.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	observe, err := orgapi.Observe(d.registry)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(observe, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(telemetry.ContextWithPrivateRequest(r.Context()))
			recorder := &statusWriter{ResponseWriter: w, status: 200}
			start := time.Now()
			next.ServeHTTP(recorder, r)
			route := "unmatched"
			if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
				route = rc.RoutePattern()
			}
			logger.Request(r.Context(), r.Method, route, recorder.status, time.Since(start))
		})
	})
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	storageConfig := mediapg.StorageConfig{Endpoint: required("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: required("ANPFUEL_TEST_STORAGE_BUCKET"), Region: required("ANPFUEL_TEST_STORAGE_REGION"),
		Credentials: storage.Credentials{AccessKeyID: required("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: required("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{61}, 32), Pool: fx.f.Runtime, Accounts: &runtimeAccounts, DocumentStorage: &storageConfig}); err != nil {
		t.Fatal(err)
	}
	d.router = router
	return d
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (d *documentHTTP) declare(t *testing.T, actor matrixPerson, raw []byte) (map[string]any, map[string]any) {
	t.Helper()
	sum := sha256.Sum256(raw)
	declaration := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "document_id": onboardingID(t), "kind": "RECEIPT",
		"original": map[string]any{"sha256": hex.EncodeToString(sum[:]), "bytes": len(raw), "media_type": "image/jpeg"}}
	return declaration, matrixCall(t, d.fx, d.router, actor, "/v1/orgs/"+d.org+"/documents/uploads/create", declaration, 201, "", "", nil)
}

func (d *documentHTTP) upload(t *testing.T, organization, document, capability, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+"/v1/orgs/"+organization+"/documents/"+document+"/content/upload", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	if capability != "" {
		r.Header.Set(orgapi.DocumentCapabilityHeader, capability)
	}
	w := httptest.NewRecorder()
	d.router.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
		t.Fatal("document ingress response is cacheable")
	}
	return w
}

func (d *documentHTTP) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := d.fx.f.Admin.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func errorBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(w.Body.Bytes(), &envelope) != nil {
		t.Fatal("refusal is not the private error envelope")
	}
	return envelope.Error.Code
}

func TestDocumentUploadSessionAndAuditedContentOverTheCompanyAPI(t *testing.T) {
	d := newDocumentHTTP(t)
	fx, router := d.fx, d.router
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	driver, manager, supervisor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, driver, "DRIVER", "ORG", "ACTIVE")
	matrixAssign(t, fx, manager, "FLEET_MANAGER", "ORG", "ACTIVE")
	matrixAssign(t, fx, supervisor, "SUPERVISOR", "ORG", "ACTIVE")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	otherOrg := other.scope.OrgID().String()
	base := "/v1/orgs/" + d.org + "/documents/"
	var secrets []string

	// BUC-EMP-112: declare, refuse unverified bytes, store verified bytes once.
	declaration, created := d.declare(t, driver, d.raw)
	id := declaration["document_id"].(string)
	upload := created["upload"].(map[string]any)
	capability := upload["capability"].(string)
	secrets = append(secrets, capability)
	document := created["document"].(map[string]any)
	if created["replayed"] != false || document["statuses"].(map[string]any)["storage"] != "UPLOADING" || document["id"] != id {
		t.Fatal("declaration is not a pending upload session")
	}
	expires, err := time.Parse(time.RFC3339Nano, upload["expires_at"].(string))
	if err != nil || time.Until(expires) > 5*time.Minute {
		t.Fatal("upload capability exceeds five minutes", err)
	}
	again := matrixCall(t, fx, router, driver, base+"uploads/create", declaration, 201, "", "", nil)
	if again["replayed"] != true || again["upload"] == nil {
		t.Fatal("identical declaration retry did not return the first record")
	}
	secrets = append(secrets, again["upload"].(map[string]any)["capability"].(string))
	changed := map[string]any{}
	for k, v := range declaration {
		changed[k] = v
	}
	changed["original"] = map[string]any{"sha256": strings.Repeat("cd", 32), "bytes": len(d.raw), "media_type": "image/jpeg"}
	matrixCall(t, fx, router, driver, base+"uploads/create", changed, 409, "OPERATION_ID_REUSED", "", nil)
	matrixCall(t, fx, router, manager, base+"uploads/create", declaration, 409, "OPERATION_ID_REUSED", "", nil)
	png := matrixDocumentDeclaration(t)
	png["original"].(map[string]any)["media_type"] = "image/png"
	matrixCall(t, fx, router, driver, base+"uploads/create", png, 422, "VALIDATION_FAILED", "", nil)
	var wg sync.WaitGroup
	concurrent := matrixDocumentDeclaration(t)
	statuses := make([]int, 6)
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw, _ := json.Marshal(concurrent)
			r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+base+"uploads/create", bytes.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", deviceTestOrigin)
			r.AddCookie(driver.cookie)
			r.Header.Set("X-CSRF", driver.csrf)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			statuses[i] = w.Code
		}(i)
	}
	wg.Wait()
	// The company session plane may refuse racing uses of one browser session
	// (monotonic last-seen); no race may surface as a conflict or a second row.
	accepted := 0
	for _, status := range statuses {
		switch status {
		case 201:
			accepted++
		case 401:
		default:
			t.Fatal("concurrent identical declaration surfaced a non-session refusal", statuses)
		}
	}
	if accepted == 0 {
		t.Fatal("no concurrent identical declaration was accepted", statuses)
	}
	if replay := matrixCall(t, fx, router, driver, base+"uploads/create", concurrent, 201, "", "", nil); replay["replayed"] != true {
		t.Fatal("sequential retry after concurrent declarations was not a replay")
	}
	if d.count(t, `SELECT count(*) FROM org_documents WHERE org_id=$1 AND id=$2`, d.org, concurrent["document_id"]) != 1 || d.count(t, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2`, d.org, concurrent["document_id"]) != 1 {
		t.Fatal("concurrent duplicates wrote more than one record")
	}
	mismatch := bytes.Clone(d.raw)
	mismatch[len(mismatch)-3] ^= 0x33
	for _, refusal := range []struct {
		name, organization, capability, contentType string
		body                                        []byte
		status                                      int
		code                                        string
	}{
		{"missing-capability", d.org, "", "image/jpeg", d.raw, 401, "UNAUTHENTICATED"},
		{"forged-capability", d.org, "v1.forged", "image/jpeg", d.raw, 404, "NOT_FOUND"},
		{"other-organization", otherOrg, capability, "image/jpeg", d.raw, 404, "NOT_FOUND"},
		{"wrong-type-header", d.org, capability, "image/png", d.raw, 422, "VALIDATION_FAILED"},
		{"digest-mismatch", d.org, capability, "image/jpeg", mismatch, 422, "VALIDATION_FAILED"},
		{"oversized", d.org, capability, "image/jpeg", append(bytes.Clone(d.raw), 0xd9), 422, "VALIDATION_FAILED"},
		{"truncated", d.org, capability, "image/jpeg", d.raw[:len(d.raw)-1], 422, "VALIDATION_FAILED"},
	} {
		w := d.upload(t, refusal.organization, id, refusal.capability, refusal.contentType, refusal.body)
		if w.Code != refusal.status || errorBody(t, w) != refusal.code {
			t.Fatal("ingress refusal", refusal.name, w.Code, w.Body.String())
		}
		if d.count(t, `SELECT count(*) FROM org_documents WHERE org_id=$1 AND id=$2 AND storage_state='UPLOADING' AND storage_revision=1`, d.org, id) != 1 {
			t.Fatal("refused transfer left the pending state", refusal.name)
		}
	}
	// Another document's id with this capability is refused before any read.
	if w := d.upload(t, d.org, concurrent["document_id"].(string), capability, "image/jpeg", d.raw); w.Code != 404 {
		t.Fatal("capability crossed documents", w.Code)
	}
	w := d.upload(t, d.org, id, capability, "image/jpeg", d.raw)
	if w.Code != 200 {
		t.Fatal("verified upload refused", w.Code, w.Body.String())
	}
	var stored map[string]any
	if json.Unmarshal(w.Body.Bytes(), &stored) != nil || stored["replayed"] != false || stored["document"].(map[string]any)["statuses"].(map[string]any)["storage"] != "STORED" {
		t.Fatal("verified upload did not report STORED", w.Body.String())
	}
	if w := d.upload(t, d.org, id, capability, "image/jpeg", d.raw); w.Code != 200 || !strings.Contains(w.Body.String(), `"replayed":true`) {
		t.Fatal("retried upload did not return the stored record", w.Code)
	}
	if d.count(t, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2 AND action='document.upload.stored'`, d.org, id) != 1 {
		t.Fatal("stored transition not recorded exactly once")
	}
	if stale := matrixCall(t, fx, router, driver, base+"uploads/create", declaration, 201, "", "", nil); stale["upload"] != nil {
		t.Fatal("stored document minted another upload capability")
	}

	// BUC-EMP-113: audited content through member-bound capabilities.
	access := func(actor matrixPerson, organization, document string, want int, code, key string) string {
		t.Helper()
		v := matrixCall(t, fx, router, actor, "/v1/orgs/"+organization+"/documents/"+document+"/content/access", map[string]any{}, want, code, key, nil)
		if want != 200 {
			return ""
		}
		value := v["capability"].(string)
		secrets = append(secrets, value)
		if v["document_id"] != document {
			t.Fatal("content capability response")
		}
		return value
	}
	read := func(actor matrixPerson, organization, document, value string, want int, code, key string) {
		t.Helper()
		v := matrixCall(t, fx, router, actor, "/v1/orgs/"+organization+"/documents/"+document+"/content/read", map[string]any{"capability": value}, want, code, key, nil)
		if want == 200 {
			raw, err := base64.StdEncoding.DecodeString(v["content_base64"].(string))
			if err != nil || !bytes.Equal(raw, d.raw) || v["media_type"] != "image/jpeg" || v["bytes"] != float64(len(d.raw)) || v["data_mode"] != "SYNTHETIC" {
				t.Fatal("audited read did not return the exact original")
			}
		}
	}
	served := `SELECT count(*) FROM org_sensitive_access_events WHERE org_id=$1 AND category='DOCUMENT_CONTENT' AND stage=$2`
	managerCapability := access(manager, d.org, id, 200, "", "")
	read(manager, d.org, id, managerCapability, 200, "", "")
	driverCapability := access(driver, d.org, id, 200, "", "")
	read(driver, d.org, id, driverCapability, 200, "", "")
	if d.count(t, served, d.org, "ISSUED") != 2 || d.count(t, served, d.org, "SERVED") != 2 {
		t.Fatal("permitted reads not recorded once each")
	}
	ownerDocument := matrixStoredDocument(t, fx, router, owner)
	access(supervisor, d.org, id, 403, "FORBIDDEN", "doc.document.view_content")
	access(driver, d.org, ownerDocument, 403, "FORBIDDEN", "doc.document.view_content")
	access(manager, otherOrg, id, 404, "NOT_FOUND", "")
	access(manager, d.org, onboardingID(t), 404, "NOT_FOUND", "")
	access(manager, d.org, concurrent["document_id"].(string), 404, "NOT_FOUND", "")
	read(driver, d.org, id, managerCapability, 404, "NOT_FOUND", "")
	read(owner, d.org, id, managerCapability, 404, "NOT_FOUND", "")
	read(manager, d.org, ownerDocument, managerCapability, 404, "NOT_FOUND", "")
	read(manager, otherOrg, id, managerCapability, 404, "NOT_FOUND", "")
	if d.count(t, served, d.org, "ISSUED") != 2 || d.count(t, served, d.org, "SERVED") != 2 {
		t.Fatal("refused content requests wrote access records")
	}
	// An unavailable access log mints and serves nothing.
	open := access(manager, d.org, id, 200, "", "")
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "REVOKE INSERT ON org_sensitive_access_events FROM "+role); err != nil {
		t.Fatal(err)
	}
	access(manager, d.org, id, 503, "UNAVAILABLE", "")
	read(manager, d.org, id, open, 503, "UNAVAILABLE", "")
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT INSERT ON org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
	read(manager, d.org, id, open, 200, "", "")
	// Ending the content role closes the open capability.
	late := access(manager, d.org, id, 200, "", "")
	matrixAssign(t, fx, manager, "SUPERVISOR", "ORG", "ACTIVE")
	read(manager, d.org, id, late, 403, "FORBIDDEN", "doc.document.view_content")
	if d.count(t, served, d.org, "ISSUED") != 4 || d.count(t, served, d.org, "SERVED") != 3 {
		t.Fatal("audit failure or revocation changed the access log")
	}

	// B-BR-EMP-222: logs and metrics carry route templates and codes only.
	sum := sha256.Sum256(d.raw)
	exposition := d.registry.Expose()
	for _, surface := range []string{d.logs.String(), exposition} {
		for _, value := range append([]string{"SYNTHETIC-HTTP-DOCUMENT-ORIGINAL", base64.StdEncoding.EncodeToString(d.raw)[:40], hex.EncodeToString(sum[:]), "/doc/", "org/" + d.org, id, "X-Amz"}, secrets...) {
			if strings.Contains(surface, value) {
				t.Fatalf("private document value reached telemetry: %.24q", value)
			}
		}
	}
	if !strings.Contains(exposition, "/v1/orgs/{org_id}/documents/{document_id}/content/upload") || !strings.Contains(d.logs.String(), "/v1/orgs/{org_id}/documents/{document_id}/content/read") {
		t.Fatal("document routes were not observed through their templates")
	}
}
