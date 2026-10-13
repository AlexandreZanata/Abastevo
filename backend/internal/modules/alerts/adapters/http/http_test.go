package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/go-chi/chi/v5"
)

const family = "11111111-1111-4111-8111-111111111111"
const owner = "22222222-2222-4222-8222-222222222222"

type probe struct {
	commands int
	deny     bool
	err      error
}

func (p *probe) Apply(_ context.Context, _ string, _ application.Batch) (application.BatchResult, error) {
	p.commands++
	if p.deny {
		return application.BatchResult{}, &probeError{"alerts: capability required"}
	}
	if p.err != nil {
		return application.BatchResult{}, p.err
	}
	return application.BatchResult{Results: []application.CommandResult{{Revision: 1, Status: "accepted"}}}, nil
}

func (p *probe) ListRules(_ context.Context, _ string) ([]application.RuleRecord, error) {
	if p.err != nil {
		return nil, p.err
	}
	return []application.RuleRecord{}, nil
}

func (p *probe) ListInbox(_ context.Context, _ string, _ int) ([]application.InboxItem, error) {
	if p.err != nil {
		return nil, p.err
	}
	return []application.InboxItem{}, nil
}

func (p *probe) EraseAll(_ context.Context, _ string) error { return p.err }

type probeError struct{ msg string }

func (e *probeError) Error() string { return e.msg }

func call(h Handler, path, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func testHandler(p *probe, who *string) Handler {
	return Handler{
		Service: p,
		Sessions: func(_ context.Context, f, a string) (string, error) {
			if f != family || a != "synthetic" {
				return "", &probeError{"alerts: authentication required"}
			}
			return *who, nil
		},
		SignedOwner: func(_ *http.Request, _ string) (string, string, error) {
			return "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", nil
		},
	}
}

func ruleBody() string {
	return `{"family_id":"` + family + `","access_token":"synthetic","commands":[{"operation_id":"66666666-6666-4666-8666-666666666666","rule_id":"77777777-7777-4777-8777-777777777777","expected_revision":0,"payload":{"predicates":[{"kind":"threshold","fuel":"ETHANOL","unit":"L","condition":"STANDARD","source":"community","threshold_milli":6200}],"operator":"AND","target":{"station_ids":["88888888-8888-4888-8888-888888888888"]},"schedule":{"days":[1],"window_start":"08:00","window_end":"20:00","timezone":"America/Sao_Paulo","quiet_start":"22:00","quiet_end":"07:00","cooldown_hours":1}}}]}`
}

func TestAlertsAuthExpiryValidation(t *testing.T) {
	who := owner
	p := &probe{}
	h := testHandler(p, &who)
	if w := call(h, "/v1/alerts/commands", ruleBody()); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("commands %d", w.Code)
	}
	unsigned := Handler{Service: p, Sessions: testHandler(p, &who).Sessions}
	if w := call(unsigned, "/v1/alerts/commands", ruleBody()); w.Code != 401 || p.commands != 1 {
		t.Fatal("unsigned admitted")
	}
	p.deny = true
	if w := call(h, "/v1/alerts/commands", ruleBody()); w.Code != 403 {
		t.Fatal("expired admitted")
	}
	p.deny = false
	if w := call(h, "/v1/alerts/rules", `{"family_id":"`+family+`","access_token":"synthetic"}`); w.Code != 200 {
		t.Fatalf("free rules %d", w.Code)
	}
	if w := call(h, "/v1/alerts/inbox", `{"family_id":"`+family+`","access_token":"synthetic","limit":20}`); w.Code != 200 {
		t.Fatalf("free inbox %d", w.Code)
	}
	if w := call(h, "/v1/alerts/erase", `{"family_id":"`+family+`","access_token":"synthetic"}`); w.Code != 200 {
		t.Fatalf("erase %d", w.Code)
	}
	for _, bad := range []string{
		strings.Replace(ruleBody(), `"commands"`, `"operations"`, 1),
		strings.Replace(ruleBody(), `"expected_revision":0`, `"expected_revision":0,"expected_revision":1`, 1),
		`{"family_id":"` + family + `","access_token":"synthetic","limit":0}`,
	} {
		path := "/v1/alerts/commands"
		if strings.Contains(bad, `"limit"`) {
			path = "/v1/alerts/inbox"
		}
		if w := call(h, path, bad); w.Code != 400 {
			t.Fatalf("bad admitted %d %s", w.Code, bad[:60])
		}
	}
}
