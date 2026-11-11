package orgapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type readDTO struct {
	orgapi.Session
	Value string `json:"value"`
}

const (
	orgA         = "7b0e2a54-0000-4000-8000-000000000001"
	orgB         = "7b0e2a54-0000-4000-8000-000000000002"
	accountID    = "7b0e2a54-0000-4000-8000-000000000011"
	membershipID = "7b0e2a54-0000-4000-8000-000000000012"
	familyID     = "7b0e2a54-0000-4000-8000-000000000013"
	route        = "/v1/orgs/{org_id}/example/read"
	origin       = "https://api.example.invalid"
)

type pageDTO struct {
	orgapi.Session
	orgapi.Page
}
type commandDTO struct {
	orgapi.Command
	Value string `json:"value"`
}

type state struct {
	work, auth, web, resolved int
	proofBody                 string
	denied                    error
	rule                      orgapi.Rule
}

func gatewayFixture(t *testing.T) (orgapi.Gateway, *state) {
	t.Helper()
	state := &state{}
	return orgapi.Gateway{
		Enabled: true,
		Origin:  origin, CursorSecret: []byte(strings.Repeat("synthetic", 8)),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) },
		Sessions: func(_ context.Context, family, token string) (string, error) {
			state.auth++
			if family != familyID || token != "synthetic-access" {
				return "", orgapi.Unauthenticated
			}
			return accountID, nil
		},
		SignedOwner: func(r *http.Request, owner string) (string, string, error) {
			body, err := io.ReadAll(r.Body)
			state.proofBody = string(body)
			if err != nil || owner != accountID || r.Header.Get("Signature") != "synthetic-proof" {
				return "", "", orgapi.Unauthenticated
			}
			return familyID, membershipID, nil
		},
		WebSession: func(r *http.Request) (string, error) {
			state.web++
			cookie, err := r.Cookie("__Host-empresas-session")
			if err != nil || cookie.Value != "synthetic-cookie" || r.Header.Get("X-CSRF") != "synthetic-csrf" {
				return "", orgapi.Unauthenticated
			}
			return accountID, nil
		},
		Resolve: func(_ context.Context, actor orgapi.Actor, scope kernel.OrgScope, rule orgapi.Rule) (orgapi.Membership, error) {
			state.resolved++
			state.rule = rule
			if state.denied != nil {
				return orgapi.Membership{}, state.denied
			}
			if scope.OrgID().String() != orgA {
				return orgapi.Membership{}, orgapi.NotFound
			}
			return orgapi.Membership{ID: membershipID, AccountID: actor.AccountID, OrgID: scope.OrgID()}, nil
		},
	}, state
}

func call(router chi.Router, target, body string, change func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Signature", "synthetic-proof")
	request.Header.Set("If-None-Match", "*")
	request.Header.Set("X-Request-ID", "synthetic-trace")
	if change != nil {
		change(request)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func checkError(t *testing.T, response *httptest.ResponseRecorder, code orgapi.Code, status int) {
	t.Helper()
	if response.Code != status || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
		t.Fatalf("private error status=%d headers=%v", response.Code, response.Header())
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error.Code != string(code) || body.Error.Message == "" {
		t.Fatalf("invalid error envelope %s", response.Body.String())
	}
}

func TestGatewayValidatesBeforeCallingModuleAndResolvesEveryRequest(t *testing.T) {
	gateway, state := gatewayFixture(t)
	router := chi.NewRouter()
	err := orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.read"},
		func(_ context.Context, principal orgapi.Principal, body readDTO) (any, error) {
			state.work++
			if principal.AccountID != accountID || principal.MembershipID != membershipID || principal.Scope.OrgID().String() != orgA || principal.Channel != "android" {
				t.Error("unresolved authority reached module")
			}
			return map[string]string{"value": body.Value}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","value":"visible"}`
	response := call(router, "/v1/orgs/"+orgA+"/example/read", body, nil)
	if response.Code != 200 || state.work != 1 || state.proofBody != body || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
		t.Fatal("valid exact-body request failed")
	}
	for _, invalid := range []string{
		"null", body + body, strings.Replace(body, `"value":"visible"`, `"value":"one","VALUE":"two"`, 1),
		strings.Replace(body, `"value"`, `"unknown"`, 1),
		strings.Replace(body, `"family_id"`, `"FAMILY_ID"`, 1),
		strings.Replace(body, `"access_token"`, `"acceſſ_token"`, 1),
		strings.Replace(body, `"synthetic-access"`, "null", 1),
		`{"value":"` + strings.Repeat("x", orgapi.MaxBodyBytes) + `"}`,
	} {
		checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", invalid, nil), orgapi.Malformed, 400)
	}
	if state.work != 1 || state.auth != 1 {
		t.Fatal("malformed request reached identity or module")
	}
	foreign := call(router, "/v1/orgs/"+orgB+"/example/read", body, nil)
	absent := call(router, "/v1/orgs/not-a-uuid/example/read", body, nil)
	checkError(t, foreign, orgapi.NotFound, 404)
	checkError(t, absent, orgapi.NotFound, 404)
	if foreign.Body.String() != absent.Body.String() {
		t.Fatal("foreign scope revealed existence")
	}
	state.denied = orgapi.MembershipSuspended
	checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", body, nil), orgapi.MembershipSuspended, 403)
	if state.work != 1 {
		t.Fatal("revoked authority used an earlier decision")
	}
}

func TestAuthenticationChannelsCannotDowngrade(t *testing.T) {
	gateway, state := gatewayFixture(t)
	router := chi.NewRouter()
	if err := orgapi.Register(router, route, gateway, orgapi.Rule{Permission: "org.settings.read"},
		func(context.Context, orgapi.Principal, readDTO) (any, error) { state.work++; return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
	android := `{"family_id":"` + familyID + `","access_token":"synthetic-access","value":""}`
	web := func(r *http.Request) {
		r.Header.Del("Signature")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF", "synthetic-csrf")
		r.AddCookie(&http.Cookie{Name: "__Host-empresas-session", Value: "synthetic-cookie"})
	}
	checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", android, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.invalid") }), orgapi.Unauthenticated, 401)
	checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", android, func(r *http.Request) { r.Header.Del("Signature") }), orgapi.Unauthenticated, 401)
	for _, body := range []string{`{"family_id":"","access_token":"","value":""}`, `{"family_id":"` + familyID + `","value":""}`} {
		checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", body, web), orgapi.Unauthenticated, 401)
	}
	checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", `{"value":""}`, func(r *http.Request) { web(r); r.Header.Set("Signature-Nonce", "partial") }), orgapi.Unauthenticated, 401)
	checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", `{"value":""}`, func(r *http.Request) { web(r); r.Header.Del("X-CSRF") }), orgapi.Unauthenticated, 401)
	if response := call(router, "/v1/orgs/"+orgA+"/example/read", `{"value":""}`, web); response.Code != 200 || state.work != 1 {
		t.Fatal("dedicated company session failed")
	}
}

func TestCommandRequirementsAndSafeErrors(t *testing.T) {
	gateway, state := gatewayFixture(t)
	router := chi.NewRouter()
	rule := orgapi.Rule{Permission: "org.settings.manage", Write: true, Fresh: true, RequireRevision: true}
	if err := orgapi.Register(router, route, gateway, rule, func(_ context.Context, _ orgapi.Principal, input commandDTO) (any, error) {
		state.work++
		if input.Revision == nil || *input.Revision != 4 {
			return nil, orgapi.Conflict
		}
		return map[string]int{"revision": 4}, nil
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"family_id":"` + familyID + `","access_token":"synthetic-access","schema_version":1,"operation_id":"` + membershipID + `","revision":3,"value":""}`
	checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", body, nil), orgapi.Conflict, 409)
	if !state.rule.Fresh || !state.rule.Write || !state.rule.RequireRevision {
		t.Fatal("authority requirements were lost")
	}
	for _, invalid := range []string{strings.Replace(body, `"schema_version":1`, `"schema_version":2`, 1), strings.Replace(body, `"revision":3`, `"revision":0`, 1), strings.Replace(body, `"revision":3,`, "", 1)} {
		checkError(t, call(router, "/v1/orgs/"+orgA+"/example/read", invalid, nil), orgapi.Malformed, 400)
	}
	if state.work != 1 {
		t.Fatal("malformed command ran work")
	}
	if response := call(router, "/v1/orgs/"+orgA+"/example/read", strings.Replace(body, `"revision":3`, `"revision":4`, 1), nil); response.Code != 200 {
		t.Fatal("current revision was refused")
	}
	for _, code := range []orgapi.Code{orgapi.Forbidden, orgapi.OrgSuspended, orgapi.OrgArchived, orgapi.FreshAuthRequired, orgapi.EntitlementRequired, orgapi.LimitReached, orgapi.OperationReused, orgapi.Unavailable} {
		response := httptest.NewRecorder()
		orgapi.WriteError(response, httptest.NewRequest("POST", "/", nil), code)
		if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "synthetic-access") {
			t.Fatal("unsafe error")
		}
	}
	response := httptest.NewRecorder()
	orgapi.WriteError(response, httptest.NewRequest("POST", "/", nil), errors.New("private SQL and credential synthetic-access"))
	checkError(t, response, orgapi.Unavailable, 503)
	if strings.Contains(response.Body.String(), "credential") {
		t.Fatal("provider error escaped")
	}
}

func TestUnconfiguredOrganizationGatewayFailsClosed(t *testing.T) {
	router := chi.NewRouter()
	err := orgapi.Register(router, "/v1/orgs/{org_id}/example/read", orgapi.Gateway{}, orgapi.Rule{Permission: "org.settings.read"},
		func(context.Context, orgapi.Principal, readDTO) (any, error) {
			t.Fatal("unconfigured gateway ran work")
			return nil, nil
		})
	if err == nil {
		t.Fatal("unconfigured gateway was registered")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/orgs/7b0e2a54-0000-4000-8000-000000000001/example/read", strings.NewReader(`{}`)))
	if response.Code != http.StatusNotFound {
		t.Fatal("failed registration mounted a route")
	}
}
