//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/runtimegrants"
)

// runtimeScriptMode runs the public process suites with the API (and the
// worker they start) logged in as a role provisioned only by the reviewed
// infra/db/runtime-role.sql (EDR-365).
func runtimeScriptMode() bool { return os.Getenv("ANPFUEL_TEST_RUNTIME_GRANTS") == "script" }

type runtimeLogin struct{ role, secret string }

func syntheticHex(t *testing.T, size int) string {
	t.Helper()
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw)
}

// newRuntimeLogin creates an unprivileged cluster login whose privileges the
// script grants later. Its removal runs before the owner connection and the
// disposable database are closed and dropped (cleanups run last-in first-out).
func newRuntimeLogin(t *testing.T, owner *pgx.Conn) runtimeLogin {
	t.Helper()
	login := runtimeLogin{role: "api_runtime_" + syntheticHex(t, 6), secret: syntheticHex(t, 24)}
	name := pgx.Identifier{login.role}.Sanitize()
	// Both values are generated here: a quoted identifier and a hexadecimal secret.
	if _, err := owner.Exec(context.Background(), "CREATE ROLE "+name+" LOGIN PASSWORD '"+login.secret+"'"); err != nil {
		t.Fatal("create runtime login", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := owner.Exec(ctx, "DROP OWNED BY "+name+"; DROP ROLE IF EXISTS "+name); err != nil {
			t.Errorf("runtime login cleanup: %v", err)
		}
	})
	return login
}

// applyRuntimeScript provisions the login with the checked-in script, as the
// migration owner, after the database reached the head schema.
func applyRuntimeScript(t *testing.T, owner *pgx.Conn, login runtimeLogin) {
	t.Helper()
	script, err := os.ReadFile(filepath.Join("..", "..", "..", runtimegrants.ScriptPath))
	if err != nil {
		t.Fatal(err)
	}
	sql, err := runtimegrants.ForRole(script, login.role)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(context.Background(), sql); err != nil {
		t.Fatal("apply runtime script", err)
	}
}

func (login runtimeLogin) dsn(t *testing.T, ownerDSN string) string {
	t.Helper()
	parsed, err := url.Parse(ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword(login.role, login.secret)
	return parsed.String()
}

// startAPIProcess runs the real production process (run()) with the given
// database and switches, exactly as testPublicProcessIdentityAndSignedWrites.
func startAPIProcess(t *testing.T, databaseURL string, extra ...string) (apiHarness, []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAPIHelperProcess$")
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "ANPFUEL_") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	// No private bucket: with the enterprise switch on, the production
	// composition accepts only an HTTPS object store (orgexport.NewS3), so a
	// loopback fixture bucket would refuse startup by design.
	cmd.Env = append(cmd.Env, "ANPFUEL_ANP_DISCOVERY_ENABLED=false", "ANPFUEL_API_HELPER=1", "ANPFUEL_ENV=development", "ANPFUEL_DATABASE_URL="+databaseURL, "ANPFUEL_HTTP_ADDR="+addr, "ANPFUEL_METRICS_ADDR=", "ANPFUEL_CANONICAL_HOST=test.invalid", "ANPFUEL_CURSOR_SECRET="+strings.Repeat("synthetic", 8))
	cmd.Env = append(cmd.Env, extra...)
	log, err := os.Create(filepath.Join(t.TempDir(), "api.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		if err := cmd.Wait(); err != nil {
			t.Errorf("API did not drain: %v", err)
		}
	})
	h := apiHarness{t: t, base: "http://" + addr, client: &http.Client{Timeout: 10 * time.Second}}
	deadline := time.Now().Add(20 * time.Second)
	for {
		resp, err := h.client.Get(h.base + "/health/live")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			data, _ := os.ReadFile(log.Name())
			t.Fatalf("API did not start: %s", data)
		}
		time.Sleep(25 * time.Millisecond)
	}
	return h, cmd.Env
}

// companyClient drives the company cookie plane of the real process.
type companyClient struct {
	h            apiHarness
	cookie, csrf string
}

func (c *companyClient) post(path string, body any, want int) map[string]any {
	c.h.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		c.h.t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, c.h.base+path, bytes.NewReader(raw))
	if err != nil {
		c.h.t.Fatal(err)
	}
	req.Host = "test.invalid"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://test.invalid")
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
		req.Header.Set("X-CSRF", c.csrf)
	}
	resp, err := c.h.client.Do(req)
	if err != nil {
		c.h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.h.t.Fatal(err)
	}
	if resp.StatusCode != want || resp.Header.Get("Cache-Control") != "no-store" {
		c.h.t.Fatalf("%s: got %d want %d: %s", path, resp.StatusCode, want, data)
	}
	var out map[string]any
	if json.Unmarshal(data, &out) != nil {
		c.h.t.Fatalf("%s: non-JSON response", path)
	}
	if path == "/v1/org-access/session/login" && want == 200 {
		for _, value := range resp.Header.Values("Set-Cookie") {
			if pair, _, _ := strings.Cut(value, ";"); strings.HasPrefix(pair, "__Host-empresas-session=") {
				c.cookie = pair
			}
		}
		c.csrf, _ = out["csrf_token"].(string)
		if c.cookie == "" || c.csrf == "" {
			c.h.t.Fatal("company login set no cookie or CSRF token")
		}
	}
	return out
}

func errorCode(out map[string]any) string {
	failure, _ := out["error"].(map[string]any)
	code, _ := failure["code"].(string)
	return code
}

func operationID(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	raw[6], raw[8] = raw[6]&0x0f|0x40, raw[8]&0x3f|0x80
	h := hex.EncodeToString(raw)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// accountLogin creates a key account and logs it in through the real routes.
func (h apiHarness) accountLogin(username string) (string, map[string]any) {
	h.t.Helper()
	raw, _ := json.Marshal(map[string]string{"username": username})
	created := h.call(h.req("POST", "/v1/accounts/keys", raw), 201)
	raw, _ = json.Marshal(map[string]string{"account_key": created["account_key"].(string)})
	logged := h.call(h.req("POST", "/v1/accounts/keys/login", raw), 200)
	return logged["account"].(map[string]any)["account_id"].(string), logged["session"].(map[string]any)
}

func syntheticOrganization(name string, t *testing.T) map[string]any {
	return map[string]any{"schema_version": 1, "operation_id": operationID(t), "name": name, "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic main unit",
		"terms": map[string]any{"version": orgdomain.SyntheticTermsVersion, "digest": orgdomain.SyntheticTermsDigest, "accepted": true}}
}

// TestRuntimeRoleServesProductionProcessAndEnterprise proves EMP-REL-002's
// activation precondition (B-BR-EMP-556..558): on a fresh head database the
// real API process, logged in as the role provisioned only by the reviewed
// script and with ANPFUEL_ENTERPRISE_ENABLED=true, serves the public reads,
// signed public writes, account sign-in and an enterprise flow end to end,
// while the superuser login the VPS uses today is refused by the
// organization role guard.
func TestRuntimeRoleServesProductionProcessAndEnterprise(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("ANPFUEL_TEST_DATABASE_URL must name disposable test infrastructure")
	}
	control, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close(context.Background()) })
	dbName := fmt.Sprintf("api_runtime_%d", time.Now().UnixNano())
	if _, err := control.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := control.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + dbName
	ownerDSN := parsed.String()
	if _, err := migrate.Apply(ctx, ownerDSN, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Close(context.Background()) })
	login := newRuntimeLogin(t, owner)
	applyRuntimeScript(t, owner, login)
	runtimeDSN := login.dsn(t, ownerDSN)

	// Current VPS state: the superuser login mounts the routes, but every
	// organization transaction is refused by requireSafeOrgRole.
	t.Run("superuser login is refused by the organization role guard", func(t *testing.T) {
		h, _ := startAPIProcess(t, ownerDSN, "ANPFUEL_ENTERPRISE_ENABLED=true")
		h.call(h.req("GET", "/health/ready", nil), 200)
		_, session := h.accountLogin("ownerdsnprobe")
		company := &companyClient{h: h}
		company.post("/v1/org-access/session/login", map[string]any{"family_id": session["family_id"], "access_token": session["access_token"]}, 200)
		if out := company.post("/v1/org-access/organizations/create", syntheticOrganization("Synthetic refused company", t), 503); errorCode(out) != "UNAVAILABLE" {
			t.Fatal("superuser organization write was not refused as unavailable", out)
		}
		var organizations int
		if err := owner.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&organizations); err != nil || organizations != 0 {
			t.Fatal("refused creation left an organization", organizations, err)
		}
	})

	h, env := startAPIProcess(t, runtimeDSN, "ANPFUEL_ENTERPRISE_ENABLED=true", "ANPFUEL_FORUM_ENABLED=true")
	h.call(h.req("GET", "/health/ready", nil), 200)

	// Public reads already exercised by the production process tests.
	station := "c0000000-0000-4000-8000-000000000011"
	if _, err := owner.Exec(ctx, "INSERT INTO directory_stations(id,display_name,municipality_code,state) VALUES ($1,'Runtime Validation Station','3550308','SP')", station); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/community/summary?state=SP&municipality_code=3550308", "/v1/community/feed?state=SP&municipality_code=3550308&fuel_product=GASOLINE_REGULAR", "/v1/stations?q=Runtime", "/v1/stations/" + station, "/v1/stations/" + station + "/prices?fuel_product=GASOLINE_REGULAR", "/v1/stations/" + station + "/official-prices", "/v1/stations/nearby?lat=-23.55&lon=-46.63&radius_m=1000", "/v1/map/assertions?min_lat=-16&min_lon=-48&max_lat=-15&max_lon=-47&source=ANP", "/v1/forum/categories", "/v1/forum/posts?limit=20", "/v1/charging/observations/history?place_id=11111111-1111-4111-8111-111111111111&limit=10"} {
		h.call(h.req("GET", path, nil), 200)
	}
	// Signed public writes with replay protection.
	a := h.enroll(newAPIKey(t))
	b := h.enroll(newAPIKey(t))
	raw := []byte(fmt.Sprintf(`{"client_submission_id":"runtime-submit-1","station_id":%q,"fuel_product":"GASOLINE_REGULAR","price":{"amount_milli_brl":5900,"currency":"BRL","unit":"L"},"condition":{"kind":"STANDARD"}}`, station))
	submit := h.signed(a, "POST", "/v1/observations", raw)
	submit.Header.Set("Idempotency-Key", "runtime-key-1")
	observation := h.call(submit, 201)["id"].(string)
	retry := h.signed(a, "POST", "/v1/observations", raw)
	retry.Header.Set("Idempotency-Key", "runtime-key-1")
	if h.call(retry, 201)["id"] != observation {
		t.Fatal("idempotent retry forked under the runtime role")
	}
	h.call(h.signed(a, "GET", "/v1/observations/"+observation, nil), 200)
	h.call(h.signed(b, "GET", "/v1/observations/"+observation, nil), 404)

	// Account sign-in, refresh and the account-owned private planes.
	ownerAccount, sessionA := h.accountLogin("runtimeowner")
	sessionBody, _ := json.Marshal(map[string]any{"family_id": sessionA["family_id"], "access_token": sessionA["access_token"]})
	h.call(h.req("POST", "/v1/premium/status", sessionBody), 200)
	h.call(h.req("POST", "/v1/cloud/status", sessionBody), 200)
	if inbox := h.call(h.req("POST", "/v1/notifications/list", sessionBody), 200); inbox["unread"] != float64(0) {
		t.Fatal("notification inbox under the runtime role", inbox)
	}
	_, refreshed := h.accountLogin("runtimerefresh")
	h.call(h.req("POST", "/v1/accounts/sessions/refresh", []byte(fmt.Sprintf(`{"family_id":%q,"refresh_token":%q}`, refreshed["family_id"], refreshed["refresh_token"]))), 200)

	// Enterprise end to end through the real cookie plane.
	company := &companyClient{h: h}
	company.post("/v1/org-access/organizations/setups/list", map[string]any{}, 401)
	company.post("/v1/org-access/session/login", map[string]any{"family_id": sessionA["family_id"], "access_token": sessionA["access_token"]}, 200)
	created := company.post("/v1/org-access/organizations/create", syntheticOrganization("Synthetic runtime company", t), 201)
	org := created["organization_id"].(string)
	unit := created["unit_id"].(string)
	base := "/v1/orgs/" + org
	if setups := company.post("/v1/org-access/organizations/setups/list", map[string]any{}, 200); len(setups["setups"].([]any)) != 1 {
		t.Fatal("own setup not rediscovered")
	}
	view := company.post(base+"/settings/read", map[string]any{}, 200)
	if view["context"].(map[string]any)["capability_enabled"] != false {
		t.Fatal("a new organization started with its capability enabled")
	}
	if out := company.post(base+"/units/create", map[string]any{"schema_version": 1, "operation_id": operationID(t), "name": "Synthetic refused unit", "code": "REFUSED"}, 403); errorCode(out) != "ENTITLEMENT_REQUIRED" {
		t.Fatal("capability-off write was not refused", out)
	}

	// Operator provisioning primitive (there is no production route or
	// command yet): the audited capability store, as the runtime login.
	pool, err := pgxpool.New(ctx, runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	catalog, err := audit.NewCatalog(cappg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := cap.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	flags := cappg.NewStore(pool, capabilities, auditpg.NewStore(pool, catalog))
	orgID, err := kernel.ParseOrgID(org)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(orgID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := flags.Set(ctx, scope, cap.Actor{AccountID: ownerAccount}, cap.Change{OperationID: operationID(t), Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal("enable capability as the runtime login", err)
	}
	company.post(base+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": operationID(t), "revision": 1}, 200)
	company.post(base+"/units/create", map[string]any{"schema_version": 1, "operation_id": operationID(t), "name": "Synthetic second unit", "code": "SECOND"}, 201)
	employee := company.post(base+"/employees/create", map[string]any{"schema_version": 1, "operation_id": operationID(t), "display_name": "Synthetic invited driver", "work_email": "driver@example.invalid"}, 201)["employee_id"].(string)
	invitation := company.post(base+"/invitations/issue", map[string]any{"schema_version": 1, "operation_id": operationID(t), "employee_id": employee, "grants": []any{map[string]any{"role": "DRIVER", "scope": "SELF"}}}, 200)
	if _, ok := invitation["delivery"].(map[string]any); !ok {
		t.Fatal("invitation issued without its one-time delivery")
	}
	asset := company.post(base+"/fleet/assets/create", map[string]any{"schema_version": 1, "operation_id": operationID(t), "kind": "VEHICLE", "name": "Synthetic runtime vehicle", "fleet_number": "RT-01", "plate": "RTM1T23", "serial_number": "", "usage_basis": "DISTANCE",
		"unit_id": unit, "category": map[string]any{"new_label": "Synthetic runtime fleet"}, "tanks": []any{map[string]any{"role": "MAIN", "capacity_milli": "275000", "products": []string{"DIESEL_S10"}}}}, 201)["asset_id"].(string)
	installedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	meter := company.post(base+"/fleet/assets/"+asset+"/meters/install", map[string]any{"schema_version": 1, "operation_id": operationID(t), "revision": 1, "opening_reading_id": operationID(t), "kind": "ODOMETER",
		"label": "Synthetic runtime odometer", "opening_value": "123456", "method": "TYPED", "occurred_at": installedAt.Format(time.RFC3339Nano), "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400}, 201)
	company.post(base+"/fleet/assets/"+asset+"/activate", map[string]any{"schema_version": 1, "operation_id": operationID(t), "revision": 2}, 200)
	latest := company.post(base+"/fleet/assets/"+asset+"/meters/"+meter["meter_id"].(string)+"/latest", map[string]any{}, 200)
	if latest["latest"].(map[string]any)["value"] != "123456" {
		t.Fatal("meter read under the runtime role", latest)
	}

	// A signed device: bind the public key to the account, enroll it in the
	// organization and deliver one meter reading through the sync intake.
	ownerKey := h.enroll(newAPIKey(t))
	if _, err := owner.Exec(ctx, "INSERT INTO account_contributor_bindings(account_id,contributor_id,key_fingerprint) VALUES($1,$2,$3)", ownerAccount, ownerKey.contributor, ownerKey.fp); err != nil {
		t.Fatal(err)
	}
	enroll, _ := json.Marshal(map[string]any{"family_id": sessionA["family_id"], "access_token": sessionA["access_token"], "schema_version": 1, "operation_id": operationID(t), "label": "Synthetic runtime device"})
	device := h.call(h.signedHost(ownerKey, base+"/devices/enroll", enroll), 200)["device"].(map[string]any)
	binding, err := protocol.NewBinding(ownerAccount, "https://test.invalid", org)
	if err != nil {
		t.Fatal(err)
	}
	opRaw, _ := json.Marshal(map[string]any{"schema_version": 1, "operation_id": operationID(t), "owner_account_id": ownerAccount, "api_origin": "https://test.invalid", "org_id": org, "membership_id": created["membership_id"],
		"enrollment_id": device["id"], "type": "meter.reading.record", "operation_schema": 1, "entity": map[string]any{"type": "meter_reading", "id": operationID(t), "expected_revision": nil},
		"occurred_at": time.Now().UTC().Format(time.RFC3339Nano), "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "depends_on": []string{}, "soft_references": []any{}, "attachments": []any{},
		"payload": map[string]any{"asset_id": asset, "meter_id": meter["meter_id"], "segment_id": meter["segment_id"], "kind": "ODOMETER", "value": "123500", "method": "TYPED"}})
	operation, err := protocol.Seal(opRaw, binding)
	if err != nil {
		t.Fatal(err)
	}
	batch, _ := json.Marshal(map[string]any{"schema_version": 1, "family_id": sessionA["family_id"], "access_token": sessionA["access_token"], "operations": []json.RawMessage{operation.CanonicalBytes()}})
	acks := h.call(h.signedHost(ownerKey, base+"/sync/operations", batch), 200)["acknowledgements"].([]any)
	if len(acks) != 1 || acks[0].(map[string]any)["result"] != protocol.Accepted {
		t.Fatal("signed meter reading not accepted under the runtime role", acks)
	}
	replayed := h.call(h.signedHost(ownerKey, base+"/sync/operations", batch), 200)["acknowledgements"].([]any)
	first, _ := json.Marshal(acks[0])
	again, _ := json.Marshal(replayed[0])
	if len(replayed) != 1 || !bytes.Equal(first, again) {
		t.Fatal("replayed operation changed its stored acknowledgement")
	}

	// Another organization stays not found.
	_, sessionB := h.accountLogin("runtimeforeign")
	foreign := &companyClient{h: h}
	foreign.post("/v1/org-access/session/login", map[string]any{"family_id": sessionB["family_id"], "access_token": sessionB["access_token"]}, 200)
	other := foreign.post("/v1/org-access/organizations/create", syntheticOrganization("Synthetic foreign company", t), 201)["organization_id"].(string)
	if out := company.post("/v1/orgs/"+other+"/settings/read", map[string]any{}, 404); errorCode(out) != "NOT_FOUND" {
		t.Fatal("foreign organization disclosed", out)
	}
	if out := foreign.post(base+"/fleet/assets/"+asset+"/read", map[string]any{}, 404); errorCode(out) != "NOT_FOUND" {
		t.Fatal("foreign member read another organization's asset", out)
	}

	// The worker with the same runtime login and switch settles a time-driven
	// enterprise job (EMP-FLEET-007 reservation outcome) from the queue.
	workerPath := filepath.Join(t.TempDir(), "worker")
	if out, err := exec.Command("go", "build", "-o", workerPath, "../worker").CombinedOutput(); err != nil {
		t.Fatalf("worker build: %v: %s", err, out)
	}
	worker := exec.Command(workerPath)
	worker.Env = env
	workerLog, err := os.Create(filepath.Join(t.TempDir(), "worker.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { workerLog.Close() })
	worker.Stdout, worker.Stderr = workerLog, workerLog
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = worker.Process.Signal(syscall.SIGTERM)
		if err := worker.Wait(); err != nil {
			t.Errorf("worker did not drain: %v", err)
		}
	})
	startsAt := time.Now().UTC().Add(4 * time.Second).Truncate(time.Second)
	company.post(base+"/fleet/assets/"+asset+"/reservations/request", map[string]any{"schema_version": 1, "operation_id": operationID(t), "starts_at": startsAt.Format(time.RFC3339Nano), "ends_at": startsAt.Add(time.Hour).Format(time.RFC3339Nano), "purpose": "Synthetic runtime reservation"}, 201)
	deadline := time.Now().Add(90 * time.Second)
	for {
		var stored string
		if err := owner.QueryRow(ctx, `SELECT state FROM org_asset_reservations WHERE org_id=$1 AND asset_id=$2`, org, asset).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored == "EXPIRED" {
			break
		}
		if time.Now().After(deadline) {
			data, _ := os.ReadFile(workerLog.Name())
			t.Fatalf("worker did not settle the reservation outcome under the runtime role (state %s): %s", stored, data)
		}
		time.Sleep(250 * time.Millisecond)
	}
	var done int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM job_queue WHERE kind='org-asset-reservation-outcome' AND status='done'`).Scan(&done); err != nil || done != 1 {
		t.Fatal("reservation outcome job not completed by the worker", done, err)
	}

	// The runtime login stays ordinary and the guard keeps refusing the owner.
	if err := database.WithOrgTx(ctx, pool, scope, func(pgx.Tx) error { return nil }); err != nil {
		t.Fatal("runtime login refused by the organization guard", err)
	}
}

// signedHost signs a private enterprise request for the canonical host and
// sends it to the local process (the device plane requires the canonical
// Host and Origin-free signed proof).
func (h apiHarness) signedHost(k apiKey, path string, body []byte) *http.Request {
	h.t.Helper()
	req := h.signed(k, "POST", path, body)
	req.Host = "test.invalid"
	req.Header.Set("If-None-Match", "*")
	return req
}
