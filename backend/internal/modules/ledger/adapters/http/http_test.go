package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/go-chi/chi/v5"
)

const family = "11111111-1111-4111-8111-111111111111"
const owner = "22222222-2222-4222-8222-222222222222"

type probe struct {
	commands, lists, deletes int
	denyWrites               bool
	err                      error
	records                  []application.Record
}

func (p *probe) Apply(_ context.Context, _ string, _ application.Batch) (application.BatchResult, error) {
	p.commands++
	if p.denyWrites {
		return application.BatchResult{}, deniedError()
	}
	if p.err != nil {
		return application.BatchResult{}, p.err
	}
	return application.BatchResult{Results: []application.CommandResult{{Revision: 1, Status: "accepted"}}}, nil
}

func (p *probe) List(_ context.Context, _ string, _ string, _ string, _ string, _ string, _ string, _ string, _ string, _ int) ([]application.Record, error) {
	p.lists++
	if p.err != nil {
		return nil, p.err
	}
	if p.records != nil {
		return p.records, nil
	}
	return []application.Record{{Dataset: "refuel", ID: "33333333-3333-4333-8333-333333333333", Revision: 1}, {Dataset: "refuel", ID: "44444444-4444-4444-8444-444444444444", Revision: 1}}, nil
}

func (p *probe) Delete(_ context.Context, _ string, _ string, _ string) (int64, error) {
	p.deletes++
	if p.err != nil {
		return 0, p.err
	}
	return 2, nil
}

func (p *probe) SetConsent(_ context.Context, _ string, _ []string, _ bool) (application.ConsentStatus, error) {
	return application.ConsentStatus{}, p.err
}

func (p *probe) EraseAll(_ context.Context, _ string) error { return p.err }

func deniedError() error { return errDenied() }

func errDenied() error { return &ledgerError{"ledger: capability required"} }

type ledgerError struct{ msg string }

func (e *ledgerError) Error() string { return e.msg }

func call(h Handler, path, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-None-Match", "*")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func testHandler(p *probe, who *string) Handler {
	return Handler{
		Service: p, Origin: "one.example.test", CursorSecret: []byte(strings.Repeat("s", 32)),
		Sessions: func(_ context.Context, f, a string) (string, error) {
			if f != family || a != "synthetic" {
				return "", &ledgerError{"ledger: authentication required"}
			}
			return *who, nil
		},
		SignedOwner: func(_ *http.Request, _ string) (string, string, error) {
			return "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", nil
		},
	}
}

func TestLedgerAuthFreeReadsAndExpiry(t *testing.T) {
	who := owner
	p := &probe{}
	h := testHandler(p, &who)
	commands := `{"family_id":"` + family + `","access_token":"synthetic","commands":[{"operation_id":"66666666-6666-4666-8666-666666666666","dataset":"refuel","id":"77777777-7777-4777-8777-777777777777","expected_revision":0,"payload":{}}]}`
	if w := call(h, "/v1/ledger/commands", commands); w.Code != 200 || w.Header().Get("ETag") != "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("commands %d", w.Code)
	}
	unsigned := Handler{Service: p, Origin: h.Origin, CursorSecret: h.CursorSecret, Sessions: h.Sessions}
	if w := call(unsigned, "/v1/ledger/commands", commands); w.Code != 401 || p.commands != 1 {
		t.Fatal("unsigned write admitted")
	}
	p.denyWrites = true
	if w := call(h, "/v1/ledger/commands", commands); w.Code != 403 {
		t.Fatal("expired write admitted")
	}
	p.denyWrites = false
	p.records = []application.Record{}
	list := `{"family_id":"` + family + `","access_token":"synthetic","dataset":"refuel","limit":20}`
	p.denyWrites = true
	if w := call(h, "/v1/ledger/records", list); w.Code != 200 {
		t.Fatalf("free read after lapse %d", w.Code)
	}
	deletion := `{"family_id":"` + family + `","access_token":"synthetic","dataset":"refuel","id":"77777777-7777-4777-8777-777777777777"}`
	if w := call(h, "/v1/ledger/delete", deletion); w.Code != 200 {
		t.Fatalf("free delete after lapse %d", w.Code)
	}
	p.denyWrites = false
	who = "55555555-5555-4555-8555-555555555555"
	if w := call(h, "/v1/ledger/records", list); w.Code != 200 {
		t.Fatal("cross-account read failed")
	}
}

func TestLedgerBoundsFiltersPagesAndErrors(t *testing.T) {
	who := owner
	p := &probe{}
	h := testHandler(p, &who)
	base := `{"family_id":"` + family + `","access_token":"synthetic","dataset":"refuel","limit":1}`
	w := call(h, "/v1/ledger/records", base)
	if w.Code != 200 {
		t.Fatalf("list %d", w.Code)
	}
	var page struct {
		Next string `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Next == "" {
		t.Skip("single page")
	}
	next := strings.TrimSuffix(base, "}") + `,"cursor":"` + page.Next + `"}`
	if w = call(h, "/v1/ledger/records", next); w.Code != 200 {
		t.Fatal("paged list", w.Code)
	}
	who = owner
	for _, bad := range []string{
		strings.Replace(base, `"limit":1`, `"limit":0`, 1),
		strings.Replace(base, `"limit":1`, `"limit":101`, 1),
		strings.Replace(base, `"dataset"`, `"account_id"`, 1),
		strings.Replace(base, `"limit":1`, `"limit":1,"limit":2`, 1),
		strings.Replace(next, page.Next, "forged", 1),
	} {
		if w = call(h, "/v1/ledger/records", bad); w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("bad list admitted %d", w.Code)
		}
	}
	who = "55555555-5555-4555-8555-555555555555"
	if w = call(h, "/v1/ledger/records", next); w.Code != 400 {
		t.Fatal("cursor crossover leaked")
	}
	who = owner
}

func TestLedgerConsentEraseValidation(t *testing.T) {
	who := owner
	p := &probe{}
	h := testHandler(p, &who)
	consent := `{"family_id":"` + family + `","access_token":"synthetic","datasets":["refuel"],"enabled":true}`
	if w := call(h, "/v1/ledger/consent", consent); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("consent %d", w.Code)
	}
	unsigned := Handler{Service: p, Origin: h.Origin, CursorSecret: h.CursorSecret, Sessions: h.Sessions}
	if w := call(unsigned, "/v1/ledger/consent", consent); w.Code != 401 {
		t.Fatal("unsigned consent admitted")
	}
	for _, bad := range []string{
		strings.Replace(consent, `"enabled":true`, `"enabled":true,"enabled":false`, 1),
		strings.Replace(consent, `"datasets"`, `"mask"`, 1),
		`{"family_id":"` + family + `","access_token":"synthetic","datasets":["refuel"]}`,
	} {
		if w := call(h, "/v1/ledger/consent", bad); w.Code != 400 {
			t.Fatalf("bad consent admitted %d", w.Code)
		}
	}
	erase := `{"family_id":"` + family + `","access_token":"synthetic"}`
	if w := call(h, "/v1/ledger/erase", erase); w.Code != 200 {
		t.Fatalf("erase %d", w.Code)
	}
	if w := call(unsigned, "/v1/ledger/erase", erase); w.Code != 401 {
		t.Fatal("unsigned erase admitted")
	}
}
