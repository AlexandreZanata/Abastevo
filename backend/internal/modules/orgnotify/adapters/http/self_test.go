package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	companyweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	notifyweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const origin = "https://company.example.invalid"

type nopRepo struct{ calls int }

func (r *nopRepo) ListInbox(context.Context, kernel.OrgScope, app.Self, app.InboxQuery) (app.InboxPage, error) {
	r.calls++
	return app.InboxPage{}, nil
}
func (r *nopRepo) MarkRead(context.Context, kernel.OrgScope, app.Self, app.ReadCommand) (app.ReadResult, error) {
	r.calls++
	return app.ReadResult{}, nil
}
func (r *nopRepo) Preferences(context.Context, kernel.OrgScope, app.Self) ([]app.Preference, error) {
	r.calls++
	return nil, nil
}
func (r *nopRepo) SetPreference(context.Context, kernel.OrgScope, app.Self, app.PreferenceCommand) (app.PreferenceResult, error) {
	r.calls++
	return app.PreferenceResult{}, nil
}

type nopAdmission struct{}

func (nopAdmission) Validate(r orgapi.Rule) error {
	if r != notifyweb.WriteRule() {
		return orgapi.Unavailable
	}
	return nil
}
func (nopAdmission) Check(context.Context, orgapi.Principal, orgapi.Rule) error { return nil }

func options(t *testing.T, repo *nopRepo) notifyweb.Options {
	t.Helper()
	cookies, err := companyweb.New(&company.Service{Enabled: true}, origin)
	if err != nil {
		t.Fatal(err)
	}
	c, err := domain.NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	return notifyweb.Options{Enabled: true, Service: app.Service{Enabled: true, Catalog: c, Repository: repo}, Cookies: cookies,
		Gateway: orgapi.Gateway{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{3}, 32)}, Admission: nopAdmission{}}
}

const base = "/v1/orgs/7b0e2a54-0000-4000-8000-000000002301/memberships/7b0e2a54-0000-4000-8000-000000002302"

func TestSelfNoticeRoutesRegisterOnlyCompleteSpecialBoundary(t *testing.T) {
	repo := &nopRepo{}
	router := chi.NewRouter()
	if err := notifyweb.Register(router, options(t, repo)); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{notifyweb.ListPath: "list", notifyweb.ReadPath: "read", notifyweb.PreferencesReadPath: "preferences_read", notifyweb.PreferencesUpdatePath: "preferences_update"}
	seen := 0
	if err := chi.Walk(router, func(method, path string, h http.Handler, _ ...func(http.Handler) http.Handler) error {
		action, ok := want[path]
		descriptor, guarded := h.(orgapi.SelfNoticeDescriptor)
		if !ok || method != http.MethodPost || !guarded {
			t.Fatal("unexpected route", method, path)
		}
		rule := descriptor.SelfNoticeRule()
		if rule != (orgapi.SelfNoticeRule{Action: action, Cookie: true, CSRF: true, Binding: true, Success: 200}) {
			t.Fatal("self notice rule drift", path, rule)
		}
		if _, normal := h.(orgapi.Descriptor); normal {
			t.Fatal("self notice route claims an organization permission")
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != 4 {
		t.Fatal("routes", seen)
	}
	if w := notifyweb.WriteRule(); w.Permission != notifyweb.WriteClass || !w.Write || w.Fresh || w.RequireRevision || w.Success != 200 {
		t.Fatal("write admission rule drift", w)
	}
	disabled := chi.NewRouter()
	o := options(t, repo)
	o.Enabled = false
	if err := notifyweb.Register(disabled, o); err != nil {
		t.Fatal(err)
	}
	if err := chi.Walk(disabled, func(string, string, http.Handler, ...func(http.Handler) http.Handler) error {
		t.Fatal("disabled composition mounted a route")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*notifyweb.Options){
		"insecure origin":    func(o *notifyweb.Options) { o.Gateway.Origin = "http://company.example.invalid" },
		"short secret":       func(o *notifyweb.Options) { o.Gateway.CursorSecret = []byte("short") },
		"no admission":       func(o *notifyweb.Options) { o.Admission = nil },
		"no repository":      func(o *notifyweb.Options) { o.Service.Repository = nil },
		"disabled service":   func(o *notifyweb.Options) { o.Service.Enabled = false },
		"no cookie service":  func(o *notifyweb.Options) { o.Cookies = companyweb.Handler{} },
		"unrelated admisson": func(o *notifyweb.Options) { o.Admission = refuseAdmission{} },
	} {
		t.Run(name, func(t *testing.T) {
			o := options(t, repo)
			change(&o)
			if err := notifyweb.Register(chi.NewRouter(), o); err == nil {
				t.Fatal("incomplete self notice composition accepted")
			}
		})
	}
}

type refuseAdmission struct{ nopAdmission }

func (refuseAdmission) Validate(orgapi.Rule) error { return orgapi.Unavailable }

func TestSelfNoticeRoutesRefuseBeforeStorage(t *testing.T) {
	repo := &nopRepo{}
	router := chi.NewRouter()
	if err := notifyweb.Register(router, options(t, repo)); err != nil {
		t.Fatal(err)
	}
	call := func(path, body string, edit func(*http.Request)) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, origin+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if edit != nil {
			edit(r)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatal("private response policy", path)
		}
		return w
	}
	op := `"operation_id":"7b0e2a54-0000-4000-8000-000000002303"`
	for name, c := range map[string]struct {
		path, body string
		edit       func(*http.Request)
		code       int
	}{
		"text body":           {base + "/notifications/self/list", `{}`, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 400},
		"query string":        {base + "/notifications/self/list?limit=1", `{}`, nil, 400},
		"unknown field":       {base + "/notifications/self/list", `{"limit":1,"membership_id":"x"}`, nil, 400},
		"null limit":          {base + "/notifications/self/list", `{"limit":null}`, nil, 400},
		"duplicate field":     {base + "/notifications/self/list", `{"limit":1,"limit":2}`, nil, 400},
		"oversized limit":     {base + "/notifications/self/list", `{"limit":101}`, nil, 400},
		"string unread":       {base + "/notifications/self/list", `{"unread_only":"yes"}`, nil, 400},
		"read without op":     {base + "/notifications/self/7b0e2a54-0000-4000-8000-000000002304/read", `{"schema_version":1}`, nil, 400},
		"read bad schema":     {base + "/notifications/self/7b0e2a54-0000-4000-8000-000000002304/read", `{"schema_version":2,` + op + `}`, nil, 400},
		"preferences body":    {base + "/notification-preferences/self/read", `{"kind":"x"}`, nil, 400},
		"update without kind": {base + "/notification-preferences/self/update", `{"schema_version":1,` + op + `,"muted":true}`, nil, 400},
		"update null muted":   {base + "/notification-preferences/self/update", `{"schema_version":1,` + op + `,"kind":"test.a.b","muted":null}`, nil, 400},
		"update zero rev":     {base + "/notification-preferences/self/update", `{"schema_version":1,` + op + `,"kind":"test.a.b","muted":true,"revision":0}`, nil, 400},
		"credentials in body": {base + "/notifications/self/list", `{"family_id":"7b0e2a54-0000-4000-8000-000000002305"}`, nil, 400},
		"no cookie":           {base + "/notifications/self/list", `{}`, nil, 401},
		"foreign origin":      {base + "/notifications/self/list", `{}`, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example.invalid") }, 403},
		"signed headers":      {base + "/notifications/self/list", `{}`, func(r *http.Request) { r.Header.Set("Signature", "x") }, 401},
	} {
		t.Run(name, func(t *testing.T) {
			w := call(c.path, c.body, c.edit)
			if w.Code != c.code {
				t.Fatal("status", w.Code, w.Body.String())
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code == "" {
				t.Fatal("private error body", w.Body.String())
			}
		})
	}
	if repo.calls != 0 {
		t.Fatal("storage reached before current session proof", repo.calls)
	}
	disabled := chi.NewRouter()
	o := options(t, repo)
	o.Gateway.Enabled = false
	if err := notifyweb.Register(disabled, o); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, origin+base+"/notifications/self/list", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	disabled.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("gateway-disabled entry exposed", w.Code)
	}
}
