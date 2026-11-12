package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry/metrics"
)

const privateOrg = "7b0e2a54-0000-4000-8000-000000000001"
const privateRoute = "/v1/orgs/{org_id}/example/read"

type privateDTO struct {
	orgapi.Session
	Document string `json:"document"`
}

func TestPrivateHTTPPipelineRedactsEverySinkAndCountsOutcomes(t *testing.T) {
	var logs bytes.Buffer
	logger, err := telemetry.New(&logs, "info", telemetry.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	reg := metrics.NewRegistry()
	observe, err := orgapi.Observe(reg)
	if err != nil {
		t.Fatal(err)
	}
	requests, _ := reg.Counter("http_requests_total", "h", "method", "route", "class")
	seconds, _ := reg.Counter("http_request_seconds_total", "h", "method", "route", "class")
	router := NewRouter(logger.WithRoute("PRIVATE-BOUND-ROUTE"))
	router.Use(metrics.Observe(reg, requests, seconds), observe)
	gateway := orgapi.Gateway{Enabled: true, Origin: "https://api.example.invalid", CursorSecret: bytes.Repeat([]byte{1}, 32),
		Sessions:    func(context.Context, string, string) (string, error) { return privateOrg, nil },
		SignedOwner: func(*http.Request, string) (string, string, error) { return privateOrg, privateOrg, nil },
		Resolve: func(_ context.Context, _ orgapi.Actor, s kernel.OrgScope, _ orgapi.Rule) (orgapi.Membership, error) {
			return orgapi.Membership{ID: privateOrg, AccountID: privateOrg, OrgID: s.OrgID()}, nil
		}}
	var failure error
	if err := orgapi.Register(router, privateRoute, gateway, orgapi.Rule{Permission: "org.settings.read"}, func(ctx context.Context, p orgapi.Principal, body privateDTO) (any, error) {
		logger.WithRoute(body.Document).Error(telemetry.ContextWithJob(ctx, body.Document, p.MembershipID), body.Document, body.Document, "payload", body, "actor", p)
		if body.Document == "PRIVATE-PANIC" {
			panic(body.Document)
		}
		return struct{}{}, failure
	}); err != nil {
		t.Fatal(err)
	}
	denied := gateway
	denied.Enabled = false
	if err := orgapi.Register(router, "/v1/orgs/{org_id}/disabled/read", denied, orgapi.Rule{Permission: "org.settings.read"}, func(context.Context, orgapi.Principal, privateDTO) (any, error) {
		t.Fatal("disabled dispatch")
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"family_id":"` + privateOrg + `","access_token":"PRIVATE-TOKEN","document":"PRIVATE-DOCUMENT"}`
	dispatch := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Request-ID", "PRIVATE-CLIENT-TRACE")
		r.Header.Set("Signature", "PRIVATE-SIGNATURE")
		r.Header.Set("Cookie", "__Host-empresas-session=PRIVATE-COOKIE")
		r.Header.Set("X-CSRF", "PRIVATE-CSRF")
		w := httptest.NewRecorder()
		requestID(router).ServeHTTP(w, r)
		if strings.HasPrefix(path, "/v1/") && w.Header().Get("X-Request-ID") == "PRIVATE-CLIENT-TRACE" {
			t.Fatal("client trace retained")
		}
		return w
	}
	path := "/v1/orgs/" + privateOrg + "/example/read"
	for _, test := range []struct {
		failure error
		body    string
		status  int
	}{
		{nil, body, 200}, {orgapi.Conflict, body, 409}, {errors.New("PRIVATE-PROVIDER"), body, 503},
		{nil, strings.Replace(body, "PRIVATE-DOCUMENT", "PRIVATE-PANIC", 1), 503},
		{nil, "{PRIVATE-MALFORMED", 400},
	} {
		failure = test.failure
		if w := dispatch("POST", path, test.body); w.Code != test.status {
			t.Fatal("status", w.Code, test.status)
		}
	}
	if w := dispatch("PRIVATE-METHOD", path, body); w.Code != 404 {
		t.Fatal(w.Code)
	}
	dispatch("POST", "/v1/orgs/"+privateOrg+"/PRIVATE-MISSING?document=PRIVATE-QUERY&gps=-23.5558,-46.6396", body)
	dispatch("POST", "/v1/orgs/"+privateOrg+"/disabled/read", body)
	dispatch("POST", "/v1/org-access/PRIVATE-MISSING", body)
	out := reg.Expose()
	for _, text := range []string{logs.String(), out} {
		for _, secret := range []string{privateOrg, "PRIVATE-", "-23.5558", "-46.6396"} {
			if strings.Contains(text, secret) {
				t.Fatal("sensitive sink output", secret)
			}
		}
	}
	for _, want := range []string{
		`org_requests_total{code="OK",result="success",route="` + privateRoute + `"} 1`,
		`org_requests_total{code="UNAVAILABLE",result="unavailable",route="` + privateRoute + `"} 2`,
		`org_requests_total{code="MALFORMED",result="refused",route="` + privateRoute + `"} 1`,
		`org_requests_total{code="NOT_FOUND",result="refused",route="/v1/orgs/{org_id}/disabled/read"} 1`,
		`method="OTHER"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatal("missing bounded series", want)
		}
	}
	accesses := 0
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if record["code"] == "http.request" {
			accesses++
		}
	}
	if accesses != 9 {
		t.Fatal("duplicate or missing access logs", accesses)
	}
}

func TestPrivateValuesRedactFormattingAndStructuredLogging(t *testing.T) {
	org, _ := kernel.ParseOrgID(privateOrg)
	scope, _ := kernel.NewOrgScope(org)
	for _, value := range []any{
		orgapi.Session{FamilyID: privateOrg, AccessToken: "PRIVATE-TOKEN"},
		orgapi.Command{Session: orgapi.Session{AccessToken: "PRIVATE-TOKEN"}, OperationID: privateOrg},
		orgapi.Actor{AccountID: privateOrg}, orgapi.Membership{ID: privateOrg, OrgID: org},
		orgapi.Principal{Actor: orgapi.Actor{AccountID: privateOrg}, Scope: scope, MembershipID: privateOrg},
		orgapi.Page{Cursor: "PRIVATE-CURSOR"}, orgapi.Gateway{CursorSecret: []byte("PRIVATE-KEY")},
		privateDTO{Session: orgapi.Session{AccessToken: "PRIVATE-TOKEN"}, Document: "PRIVATE-DOCUMENT"},
	} {
		var log bytes.Buffer
		slog.New(slog.NewJSONHandler(&log, nil)).Info("value", "private", value)
		out := fmt.Sprintf("%v %+v %#v", value, value, value) + log.String()
		if strings.Contains(out, privateOrg) || strings.Contains(out, "PRIVATE-") {
			t.Fatal("default formatting leaked")
		}
	}
}

type lockedLog struct {
	sync.Mutex
	bytes.Buffer
}

func (b *lockedLog) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}

func TestConcurrentPrivateRequestsKeepObservationsIsolated(t *testing.T) {
	var logs lockedLog
	logger, _ := telemetry.New(&logs, "info", telemetry.RoleAPI)
	reg := metrics.NewRegistry()
	observe, _ := orgapi.Observe(reg)
	router := NewRouter(logger)
	router.Use(observe)
	router.Post(privateRoute, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.Header.Get("X-Test"), "denied") {
			orgapi.WriteError(w, r, orgapi.Forbidden)
			return
		}
		_, _ = w.Write([]byte("{}"))
	})
	var group sync.WaitGroup
	for i := range 100 {
		group.Go(func() {
			org := privateOrg
			if i%2 != 0 {
				org = "7b0e2a54-0000-4000-8000-000000000002"
			}
			r := httptest.NewRequest("POST", "/v1/orgs/"+org+"/example/read", nil)
			if i%2 == 0 {
				r.Header.Set("X-Test", "denied")
			}
			w := httptest.NewRecorder()
			requestID(router).ServeHTTP(w, r)
		})
	}
	group.Wait()
	if strings.Contains(logs.String(), privateOrg) || strings.Contains(logs.String(), "7b0e2a54-0000-4000-8000-000000000002") {
		t.Fatal("concurrent scopes entered logs")
	}
	for _, want := range []string{
		`org_requests_total{code="OK",result="success",route="` + privateRoute + `"} 50`,
		`org_requests_total{code="FORBIDDEN",result="refused",route="` + privateRoute + `"} 50`,
	} {
		if !strings.Contains(reg.Expose(), want) {
			t.Fatal("concurrent observation lost", want)
		}
	}
}

func TestCommittedStatusAndResponseControllerSurviveWrappers(t *testing.T) {
	var logs bytes.Buffer
	reg := metrics.NewRegistry()
	observe, _ := orgapi.Observe(reg)
	requests, _ := reg.Counter("http_requests_total", "h", "method", "route", "class")
	seconds, _ := reg.Counter("http_request_seconds_total", "h", "method", "route", "class")
	router := NewRouter(testOptions(&logs).Logger)
	router.Use(metrics.Observe(reg, requests, seconds), observe)
	router.Post(privateRoute, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		w.WriteHeader(500)
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
		}
	})
	r := httptest.NewRequest("POST", "/v1/orgs/"+privateOrg+"/example/read", nil)
	w := httptest.NewRecorder()
	requestID(router).ServeHTTP(w, r)
	if w.Code != 202 || !strings.Contains(reg.Expose(), `class="2xx"`) || !strings.Contains(reg.Expose(), `code="OK"`) {
		t.Fatal("attempted status replaced committed status")
	}
}

func TestFlushCommitsImplicitStatusAcrossObservationWrappers(t *testing.T) {
	var logs bytes.Buffer
	reg := metrics.NewRegistry()
	observe, _ := orgapi.Observe(reg)
	requests, _ := reg.Counter("http_requests_total", "h", "method", "route", "class")
	seconds, _ := reg.Counter("http_request_seconds_total", "h", "method", "route", "class")
	router := NewRouter(testOptions(&logs).Logger)
	router.Use(metrics.Observe(reg, requests, seconds), observe)
	router.Post(privateRoute, func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
		}
		w.WriteHeader(500)
	})
	w := httptest.NewRecorder()
	requestID(router).ServeHTTP(w, httptest.NewRequest("POST", "/v1/orgs/"+privateOrg+"/example/read", nil))
	if w.Code != 200 || !strings.Contains(reg.Expose(), `class="2xx"`) || !strings.Contains(logs.String(), `"status":200`) {
		t.Fatal("flush status was overwritten")
	}
}

func TestClientCorrelationIsNeverPromotedToTrustedPrivateMetadata(t *testing.T) {
	var logs bytes.Buffer
	logger := testOptions(&logs).Logger
	router := NewRouter(logger)
	router.Get("/public", func(_ http.ResponseWriter, r *http.Request) {
		logger.Info(telemetry.ContextWithPrivateRequest(r.Context()), "private.event", "PRIVATE-MESSAGE")
	})
	r := httptest.NewRequest("GET", "/public", nil)
	r.Header.Set("X-Request-ID", "PRIVATE-CLIENT-TRACE")
	requestID(router).ServeHTTP(httptest.NewRecorder(), r)
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if record["code"] == "private.event" && (record["request_id"] != nil || strings.Contains(string(line), "PRIVATE-")) {
			t.Fatal("client correlation was trusted by a private event")
		}
	}
}
