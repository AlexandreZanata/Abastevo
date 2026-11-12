package application_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	a "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

func TestAuthorizedHTTPAlwaysCreatesAndExpiresRequestContext(t *testing.T) {
	f := setup(t)
	r := chi.NewRouter()
	calls := 0
	var retained a.Decision
	var old context.Context
	gateway := orgapi.Gateway{Enabled: true, Origin: "https://api.example.invalid", CursorSecret: []byte(strings.Repeat("s", 32)), Sessions: func(context.Context, string, string) (string, error) { return account, nil }, SessionConfirm: func(context.Context, string, string, bool) (string, error) { return account, nil }, SignedOwner: func(*http.Request, string) (string, string, error) { return member, employee, nil }}
	if err := guarded.Register(r, "/v1/orgs/{org_id}/synthetic/read", gateway, orgapi.Rule{Permission: string(f.policy.Permission)}, f.service, f.policy, f.resource, func(ctx context.Context, p orgapi.Principal, d a.Decision, _ orgapi.Session) (any, error) {
		calls++
		retained = d
		old = ctx
		if err := d.Check(ctx, p.Scope, p.AccountID, f.policy.Permission, target); err != nil {
			t.Fatal(err)
		}
		return map[string]bool{"allowed": true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	call := func(parent context.Context) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "https://api.example.invalid/v1/orgs/"+f.scope.OrgID().String()+"/synthetic/read", strings.NewReader(`{"family_id":"`+member+`","access_token":"synthetic"}`)).WithContext(parent)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := call(t.Context()); w.Code != 200 || calls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := retained.Check(old, f.scope, account, f.policy.Permission, target); err != a.ErrInvalidContext {
		t.Fatal("old request remained live", err)
	}
	f.source.value.Assignments = nil
	if w := call(old); w.Code != 403 || calls != 1 || !strings.Contains(w.Body.String(), `"code":"idn.employee.read"`) {
		t.Fatal("old context extended revoked authority", w.Code, w.Body.String())
	}
}
