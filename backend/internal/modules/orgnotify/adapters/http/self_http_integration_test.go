//go:build integration

package http_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	eventapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/application"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	notifypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) NowUnix() int64 { return c.t.Unix() }

func id(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

func orgScope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	v, err := kernel.ParseOrgID(id(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(v)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// B-BR-EMP-218, BUC-EMP-111: the composed production routes over the real
// cookie plane, current AUTH-003 authority and forced-RLS storage.
func TestSelfNoticeRoutesOverRealCookiePlaneAndCurrentMembership(t *testing.T) {
	f := orgtest.New(t)
	f.GrantTeamReductions(t)
	f.GrantSettingsLifecycle(t)
	f.GrantOwnershipLifecycle(t)
	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+r+
		";GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships,org_role_assignments,identity_rate_windows TO "+r+
		";GRANT SELECT,UPDATE ON accounts,account_session_families TO "+r+";GRANT SELECT,INSERT ON payment_billing_exits TO "+r+
		";GRANT SELECT,INSERT,UPDATE,DELETE ON company_web_sessions TO "+r+
		";GRANT SELECT,INSERT ON org_audit_events,org_event_intents,org_event_receipts TO "+r+";GRANT SELECT,INSERT,UPDATE ON job_queue TO "+r+
		";GRANT SELECT,INSERT ON org_notification_recipients,org_notification_deliveries,org_notification_commands TO "+r+
		";GRANT SELECT,INSERT,UPDATE ON org_notification_requests,org_notification_notices,org_notification_preferences,org_notification_budgets TO "+r); err != nil {
		t.Fatal(err)
	}
	pool := f.RuntimePool(t, 4)
	accounts := &account.Service{Clock: fixedClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(f.Admin), Hasher: accountdomain.SHA256Hasher{},
		IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	login := func(alias string) (string, *account.AuthResult) {
		t.Helper()
		created, err := accounts.CreateKeyAccount(t.Context(), alias)
		if err != nil {
			t.Fatal(err)
		}
		auth, err := accounts.LoginWithKey(t.Context(), created.Key)
		if err != nil {
			t.Fatal(err)
		}
		return auth.Account.ID, &auth
	}
	ownerAccount, ownerAuth := login("noticeowner")
	driverAccount, driverAuth := login("noticedriver")
	a, b := orgScope(t, 301), orgScope(t, 302)
	for _, s := range []kernel.OrgScope{a, b} {
		v, err := org.NewPending(s, org.Profile{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := orgpg.NewStore(f.Runtime).Create(t.Context(), s, v); err != nil {
			t.Fatal(err)
		}
	}
	member := func(s kernel.OrgScope, n int, accountID, role string) {
		t.Helper()
		if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, s.OrgID().String(), id(n+200)); err != nil {
				return err
			}
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, s.OrgID().String(), id(n), id(n+200), accountID); err != nil {
				return err
			}
			kind, target := "SELF", id(n)
			if role == "OWNER" {
				kind, target = "ORG", s.OrgID().String()
			}
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$3,'synthetic notice role')`,
				s.OrgID().String(), id(n+300), id(n), role, kind, target, time.Now().UTC().Add(-time.Minute)); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, s.OrgID().String(), id(n))
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	member(a, 601, ownerAccount, "OWNER")
	member(a, 603, driverAccount, "DRIVER")
	member(b, 701, driverAccount, "OWNER")
	kinds := []domain.Kind{
		{Name: "test.report.ready", Permission: "rpt.export.run", Templates: []string{"test.report.ready.v1"},
			Params: map[string]domain.Param{"report_id": {Type: domain.ParamID}, "format": {Type: domain.ParamCode, Codes: []string{"CSV", "PDF"}}}, Link: "/reports/{report_id}"},
		{Name: "test.budget.alert", Templates: []string{"test.budget.alert.v1"}},
		{Name: "test.access.changed", Mandatory: true, Templates: []string{"test.access.changed.v1"}},
	}
	catalog, err := domain.NewCatalog(kinds)
	if err != nil {
		t.Fatal(err)
	}
	// Test-only producer through orgevent and the real queue handler.
	store := &notifypg.Store{Pool: pool, Catalog: catalog, Authority: accesspg.NewStore(pool), NewID: jobs.NewUUIDv4}
	ecat, err := event.NewCatalog([]event.Definition{{Name: "test.report.completed", TargetType: "test.report", Consumers: []string{domain.ConsumerName}}})
	if err != nil {
		t.Fatal(err)
	}
	acat, err := audit.NewCatalog([]audit.Definition{{Action: "test.report.completed", TargetType: "test.report"}})
	if err != nil {
		t.Fatal(err)
	}
	events := eventapp.Service{Enabled: true, Catalog: ecat, Audit: auditpg.NewStore(pool, acat), Writer: eventpg.NewStore(pool)}
	handler, err := eventpg.NewHandler(eventpg.NewStore(pool), ecat, map[string]eventpg.Effect{domain.ConsumerName: store.Dispatch})
	if err != nil {
		t.Fatal(err)
	}
	queue := jobs.NewQueue(pool)
	publish := func(s kernel.OrgScope, actor, target string, requests ...domain.Request) {
		t.Helper()
		op, _ := jobs.NewUUIDv4()
		sum := sha256.Sum256([]byte(op))
		e := event.Event{ID: op, Name: "test.report.completed", TargetType: "test.report", TargetID: target, Revision: 1}
		var inner error
		if _, err := events.Record(t.Context(), s, audit.Event{OperationID: op, ActorID: actor, Action: e.Name, TargetType: e.TargetType, TargetID: target, RequestHash: hex.EncodeToString(sum[:])}, e, []string{domain.ConsumerName},
			func(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx) error {
				_, inner = store.RequestTx(ctx, tx, scope, e, requests)
				return inner
			}); err != nil || inner != nil {
			t.Fatal(err, inner)
		}
		for {
			job, err := queue.Claim(t.Context(), event.JobKind, "synthetic-notify-http", time.Minute)
			if errors.Is(err, jobs.ErrNoJob) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := handler.Handle(t.Context(), job); err != nil {
				t.Fatal(err)
			}
			if err := queue.Complete(t.Context(), job.ID, job.LeaseToken); err != nil {
				t.Fatal(err)
			}
		}
	}
	report := id(900)
	publish(a, ownerAccount, report, domain.Request{Key: "report:" + report, Kind: "test.report.ready", Template: "test.report.ready.v1",
		Params: map[string]string{"report_id": report, "format": "PDF"}, Recipients: []string{id(601), id(603)}})
	publish(a, ownerAccount, id(901), domain.Request{Key: "access:1", Kind: "test.access.changed", Template: "test.access.changed.v1", Recipients: []string{id(601), id(603)}})

	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test location")
	}
	document, err := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(source), "../../../../../../contracts/openapi/v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{29}, 32), Pool: pool, Accounts: accounts, NoticeKinds: kinds}); err != nil {
		t.Fatal(err)
	}
	type session struct {
		cookie *http.Cookie
		csrf   string
	}
	call := func(s *session, path string, body any, want int, schema string, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		if s != nil && s.cookie != nil {
			req.AddCookie(s.cookie)
			req.Header.Set("X-CSRF", s.csrf)
		}
		if edit != nil {
			edit(req)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatalf("%s got %d want %d: %s", path, w.Code, want, w.Body.String())
		}
		var value map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal("response JSON", err)
		}
		if schema != "" {
			ref := document.Components.Schemas[schema]
			if ref == nil || ref.Value.VisitJSON(value) != nil {
				t.Fatal("actual response violates contract", schema, w.Body.String())
			}
		}
		for _, private := range []string{ownerAccount, driverAccount, ownerAuth.Session.AccessToken, driverAuth.Session.AccessToken} {
			if strings.Contains(path, "/notification") && strings.Contains(w.Body.String(), private) {
				t.Fatal("self notice response disclosed account or credential", path)
			}
		}
		if s != nil && path == "/v1/org-access/session/login" && want == 200 {
			s.cookie, s.csrf = w.Result().Cookies()[0], value["csrf_token"].(string)
		}
		return value
	}
	owner, driver := &session{}, &session{}
	call(owner, "/v1/org-access/session/login", map[string]any{"family_id": ownerAuth.Session.FamilyID, "access_token": ownerAuth.Session.AccessToken}, 200, "", nil)
	call(driver, "/v1/org-access/session/login", map[string]any{"family_id": driverAuth.Session.FamilyID, "access_token": driverAuth.Session.AccessToken}, 200, "", nil)
	inbox := func(org kernel.OrgScope, n int) string {
		return "/v1/orgs/" + org.OrgID().String() + "/memberships/" + id(n) + "/notifications/self"
	}
	prefs := func(org kernel.OrgScope, n int) string {
		return "/v1/orgs/" + org.OrgID().String() + "/memberships/" + id(n) + "/notification-preferences/self"
	}

	// Owner sees both own notices newest first with the re-authorizable link.
	page := call(owner, inbox(a, 601)+"/list", map[string]any{}, 200, "EnterpriseNoticePage", nil)
	notices := page["notices"].([]any)
	if len(notices) != 2 || page["next_cursor"] != "" {
		t.Fatal("owner inbox", page)
	}
	first, second := notices[0].(map[string]any), notices[1].(map[string]any)
	if first["kind"] != "test.access.changed" || first["link_path"] != nil || second["link_path"] != "/reports/"+report || second["params"].(map[string]any)["format"] != "PDF" {
		t.Fatal("notice views", notices)
	}
	// The driver lacked rpt.export.run at dispatch: only the mandatory notice.
	driverPage := call(driver, inbox(a, 603)+"/list", map[string]any{"unread_only": true}, 200, "EnterpriseNoticePage", nil)
	if items := driverPage["notices"].([]any); len(items) != 1 || items[0].(map[string]any)["kind"] != "test.access.changed" {
		t.Fatal("driver inbox", driverPage)
	}
	// Wrong recipient, wrong organization, foreign account and CSRF denials.
	call(driver, inbox(a, 601)+"/list", map[string]any{}, 404, "EnterpriseError", nil)
	call(owner, inbox(a, 603)+"/list", map[string]any{}, 404, "EnterpriseError", nil)
	call(owner, inbox(b, 601)+"/list", map[string]any{}, 404, "EnterpriseError", nil)
	call(owner, inbox(b, 701)+"/list", map[string]any{}, 404, "EnterpriseError", nil)
	call(driver, inbox(b, 701)+"/list", map[string]any{}, 200, "EnterpriseNoticePage", nil)
	call(owner, inbox(a, 601)+"/list", map[string]any{}, 403, "EnterpriseError", func(r *http.Request) { r.Header.Del("X-CSRF") })
	call(nil, inbox(a, 601)+"/list", map[string]any{}, 401, "EnterpriseError", nil)

	// Keyset cursor is bound to the member, route and filter.
	paged := call(owner, inbox(a, 601)+"/list", map[string]any{"limit": 1}, 200, "EnterpriseNoticePage", nil)
	cursor := paged["next_cursor"].(string)
	if cursor == "" || len(paged["notices"].([]any)) != 1 {
		t.Fatal("first page", paged)
	}
	rest := call(owner, inbox(a, 601)+"/list", map[string]any{"limit": 1, "cursor": cursor}, 200, "EnterpriseNoticePage", nil)
	if items := rest["notices"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != second["id"] || rest["next_cursor"] != "" {
		t.Fatal("second page", rest)
	}
	call(owner, inbox(a, 601)+"/list", map[string]any{"limit": 2, "cursor": cursor}, 400, "EnterpriseError", nil)
	call(owner, inbox(a, 601)+"/list", map[string]any{"limit": 1, "cursor": cursor, "unread_only": true}, 400, "EnterpriseError", nil)
	call(driver, inbox(a, 603)+"/list", map[string]any{"limit": 1, "cursor": cursor}, 400, "EnterpriseError", nil)

	// Mark read: replay returns the original result; reuse and foreign targets refused.
	notice := second["id"].(string)
	op := id(950)
	readBody := map[string]any{"schema_version": 1, "operation_id": op}
	marked := call(owner, inbox(a, 601)+"/"+notice+"/read", readBody, 200, "EnterpriseNoticeReadResult", nil)
	view := marked["notice"].(map[string]any)
	if marked["replayed"] != false || view["read_at"] == nil || marked["operation_id"] != op {
		t.Fatal("mark read", marked)
	}
	replayed := call(owner, inbox(a, 601)+"/"+notice+"/read", readBody, 200, "EnterpriseNoticeReadResult", nil)
	if replayed["replayed"] != true || replayed["notice"].(map[string]any)["read_at"] != view["read_at"] {
		t.Fatal("read replay", replayed)
	}
	call(owner, inbox(a, 601)+"/"+first["id"].(string)+"/read", readBody, 409, "EnterpriseError", nil)
	driverNotice := driverPage["notices"].([]any)[0].(map[string]any)["id"].(string)
	call(owner, inbox(a, 601)+"/"+driverNotice+"/read", map[string]any{"schema_version": 1, "operation_id": id(951)}, 404, "EnterpriseError", nil)
	call(driver, inbox(a, 601)+"/"+notice+"/read", map[string]any{"schema_version": 1, "operation_id": id(952)}, 404, "EnterpriseError", nil)
	unread := call(owner, inbox(a, 601)+"/list", map[string]any{"unread_only": true}, 200, "EnterpriseNoticePage", nil)
	if len(unread["notices"].([]any)) != 1 {
		t.Fatal("unread filter after read", unread)
	}

	// Preferences: every registered kind with defaults; mandatory kinds refuse muting.
	listed := call(owner, prefs(a, 601)+"/read", map[string]any{}, 200, "EnterpriseNoticePreferences", nil)
	expected := []struct {
		kind      string
		mandatory bool
	}{
		{"org.lifecycle.closure_requested", true},
		{"test.access.changed", true},
		{"test.budget.alert", false},
		{"test.report.ready", false},
	}
	items := listed["preferences"].([]any)
	if len(items) != len(expected) {
		t.Fatal("preference listing", listed)
	}
	for i, want := range expected {
		if item := items[i].(map[string]any); item["kind"] != want.kind || item["mandatory"] != want.mandatory || item["muted"] != false || item["revision"] != float64(0) {
			t.Fatal("preference defaults", item)
		}
	}
	update := map[string]any{"schema_version": 1, "operation_id": id(953), "kind": "test.budget.alert", "muted": true}
	changed := call(owner, prefs(a, 601)+"/update", update, 200, "EnterpriseNoticePreferenceResult", nil)
	if p := changed["preference"].(map[string]any); p["revision"] != float64(1) || p["muted"] != true || changed["replayed"] != false {
		t.Fatal("preference update", changed)
	}
	if again := call(owner, prefs(a, 601)+"/update", update, 200, "EnterpriseNoticePreferenceResult", nil); again["replayed"] != true {
		t.Fatal("preference replay", again)
	}
	update["muted"] = false
	call(owner, prefs(a, 601)+"/update", update, 409, "EnterpriseError", nil)
	call(owner, prefs(a, 601)+"/update", map[string]any{"schema_version": 1, "operation_id": id(954), "kind": "test.budget.alert", "muted": false}, 409, "EnterpriseError", nil)
	call(owner, prefs(a, 601)+"/update", map[string]any{"schema_version": 1, "operation_id": id(955), "kind": "test.access.changed", "muted": true}, 422, "EnterpriseError", nil)
	call(owner, prefs(a, 601)+"/update", map[string]any{"schema_version": 1, "operation_id": id(959), "kind": "org.lifecycle.closure_requested", "muted": true}, 422, "EnterpriseError", nil)
	call(owner, prefs(a, 601)+"/update", map[string]any{"schema_version": 1, "operation_id": id(956), "kind": "test.unknown.kind", "muted": true}, 422, "EnterpriseError", nil)
	call(driver, prefs(a, 601)+"/update", map[string]any{"schema_version": 1, "operation_id": id(957), "kind": "test.budget.alert", "muted": true}, 404, "EnterpriseError", nil)
	if done := call(owner, prefs(a, 601)+"/update", map[string]any{"schema_version": 1, "operation_id": id(958), "kind": "test.budget.alert", "muted": false, "revision": 1}, 200, "EnterpriseNoticePreferenceResult", nil); done["preference"].(map[string]any)["revision"] != float64(2) {
		t.Fatal("revisioned preference", done)
	}

	// Ending the membership applies to the next request (BR-AUTH-014).
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), id(603))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	call(driver, inbox(a, 603)+"/list", map[string]any{}, 404, "EnterpriseError", nil)
	call(driver, prefs(a, 603)+"/read", map[string]any{}, 404, "EnterpriseError", nil)
	call(driver, inbox(b, 701)+"/list", map[string]any{}, 200, "EnterpriseNoticePage", nil)
}
