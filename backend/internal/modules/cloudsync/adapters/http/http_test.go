package http

import (
	"context"
	"encoding/json"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const family = "11111111-1111-4111-8111-111111111111"
const owner = "22222222-2222-4222-8222-222222222222"

type probe struct {
	epoch           int64
	lists, commands int
	pulls           int
	pullFeed        app.Feed
	lastPrincipal   app.Principal
	lastPos         app.Position
	lastLimit       int
	err             error
}

func (p *probe) Status(context.Context, app.Principal) (app.Status, error) {
	return app.Status{Epoch: p.epoch, EnabledDatasets: []string{}}, p.err
}
func (p *probe) List(context.Context, app.Principal, string, int, string) ([]app.Record, error) {
	p.lists++
	return []app.Record{{Dataset: "vehicles", ID: "33333333-3333-4333-8333-333333333333", Revision: 1, Payload: json.RawMessage(`{}`)}, {Dataset: "vehicles", ID: "44444444-4444-4444-8444-444444444444", Revision: 1, Payload: json.RawMessage(`{}`)}}, p.err
}
func (p *probe) Devices(context.Context, app.Principal, int, string) ([]app.Device, error) {
	return nil, p.err
}
func (p *probe) Erase(context.Context, app.Principal, string, string) (int64, error) { return 2, p.err }
func (p *probe) RegisterDevice(context.Context, app.Principal) (app.Device, error) {
	return app.Device{}, p.err
}
func (p *probe) CheckDevice(context.Context, app.Principal) error          { return p.err }
func (p *probe) RevokeDevice(context.Context, app.Principal, string) error { return p.err }
func (p *probe) Apply(context.Context, app.Principal, app.Batch) (app.BatchResult, error) {
	p.commands++
	return app.BatchResult{}, p.err
}
func (p *probe) Pull(_ context.Context, principal app.Principal, pos app.Position, limit int) (app.Feed, error) {
	p.pulls++
	p.lastPrincipal = principal
	p.lastPos = pos
	p.lastLimit = limit
	if p.err != nil {
		return app.Feed{}, p.err
	}
	return p.pullFeed, nil
}
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
func TestOwnerPageNoETagAndCursorIsolation(t *testing.T) {
	p := &probe{epoch: 1}
	who := owner
	h := Handler{Service: p, Origin: "one.example.test", CursorSecret: []byte(strings.Repeat("s", 32)), Sessions: func(_ context.Context, f, a string) (string, error) {
		if f != family || a != "synthetic" {
			return "", domain.ErrUnauthorized
		}
		return who, nil
	}}
	body := `{"family_id":"` + family + `","access_token":"synthetic","dataset":"vehicles","limit":1}`
	w := call(h, "/v1/cloud/export", body)
	if w.Code != 200 || w.Header().Get("ETag") != "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private response cache", w.Code)
	}
	var page struct {
		Next string `json:"next_cursor"`
	}
	if json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Next == "" {
		t.Fatal("page cursor")
	}
	next := strings.TrimSuffix(body, "}") + `,"cursor":"` + page.Next + `"}`
	if w = call(h, "/v1/cloud/export", next); w.Code != 200 {
		t.Fatal("valid page", w.Code)
	}
	for _, change := range []func(){func() { who = "55555555-5555-4555-8555-555555555555" }, func() { who = owner; p.epoch = 2 }, func() { p.epoch = 1; h.Origin = "two.example.test" }} {
		change()
		before := p.lists
		w = call(h, "/v1/cloud/export", next)
		if w.Code != 400 || p.lists != before {
			t.Fatal("cursor scope leaked", w.Code)
		}
	}
	h.Origin = "one.example.test"
	for _, bad := range []string{strings.Replace(body, `"limit":1`, `"limit":0`, 1), strings.Replace(body, `"limit":1`, `"limit":101`, 1), strings.Replace(body, `"dataset":`, `"account_id":"forged","dataset":`, 1), strings.Replace(body, `"limit":1`, `"limit":1,"limit":2`, 1), strings.Replace(next, page.Next, "forged", 1)} {
		before := p.lists
		w = call(h, "/v1/cloud/export", bad)
		if w.Code != 400 || p.lists != before || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("bad owner page admitted", w.Code)
		}
	}
}
func TestCloudSignedCommandsKeepExactBodyAndOutage(t *testing.T) {
	p := &probe{epoch: 1}
	h := Handler{Service: p, Sessions: func(context.Context, string, string) (string, error) { return owner, nil }}
	body := `{"family_id":"` + family + `","access_token":"synthetic","device_id":"33333333-3333-4333-8333-333333333333","epoch":1,"consent_revision":1,"commands":[]}`
	if w := call(h, "/v1/cloud/commands", body); w.Code != 401 || p.commands != 0 {
		t.Fatal("unsigned command admitted")
	}
	h.SignedOwner = func(r *http.Request, account string) (string, string, error) {
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != body || account != owner {
			t.Fatal("signed bytes/owner changed")
		}
		return family, owner, nil
	}
	p.err = domain.ErrUnavailable
	if w := call(h, "/v1/cloud/commands", body); w.Code != 503 || p.commands != 1 {
		t.Fatal("outage hidden")
	}
}

func pullHandler(p *probe, who *string) Handler {
	return Handler{
		Service:      p,
		Origin:       "one.example.test",
		CursorSecret: []byte(strings.Repeat("s", 32)),
		Sessions: func(_ context.Context, f, a string) (string, error) {
			if f != family || a != "synthetic" {
				return "", domain.ErrUnauthorized
			}
			return *who, nil
		},
		SignedOwner: func(_ *http.Request, account string) (string, string, error) {
			if account != *who {
				return "", "", domain.ErrUnauthorized
			}
			return "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", nil
		},
	}
}

func TestSyncPullCursorLifecycleAndIsolation(t *testing.T) {
	device := "55555555-5555-4555-8555-555555555555"
	who := owner
	p := &probe{pullFeed: app.Feed{
		Changes:         []app.Change{{Sequence: 1, Epoch: 1, Dataset: "vehicles", ID: "66666666-6666-4666-8666-666666666666", Revision: 1, Payload: json.RawMessage(`{}`)}},
		Through:         1,
		Epoch:           1,
		ConsentRevision: 1,
	}}
	h := pullHandler(p, &who)
	base := `{"family_id":"` + family + `","access_token":"synthetic","device_id":"` + device + `","limit":10}`
	w := call(h, "/v1/cloud/sync/pull", base)
	if w.Code != 200 || w.Header().Get("ETag") != "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("initial pull refused %d", w.Code)
	}
	var first struct {
		Changes []app.Change `json:"changes"`
		Through int64        `json:"through"`
		Next    string       `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil || len(first.Changes) != 1 || first.Through != 1 || first.Next == "" {
		t.Fatal("initial feed", err)
	}
	if p.pulls != 1 || p.lastPos.Version != 1 || p.lastPos.After != 0 || p.lastPrincipal.DeviceID != device {
		t.Fatal("initial position not bound to device")
	}
	nextBody := strings.TrimSuffix(base, "}") + `,"cursor":"` + first.Next + `"}`
	before := p.pulls
	p.pullFeed = app.Feed{Changes: []app.Change{}, Through: 1, Epoch: 1, ConsentRevision: 1}
	if w = call(h, "/v1/cloud/sync/pull", nextBody); w.Code != 200 {
		t.Fatal("pinned page", w.Code)
	}
	if p.pulls != before+1 || p.lastPos.After != 1 || p.lastPos.Through != 1 || p.lastPos.Epoch != 1 {
		t.Fatalf("pinned cursor lost watermark %+v", p.lastPos)
	}
	for _, mutate := range []func(){func() { who = "77777777-7777-4777-8777-777777777777" }, func() { h.Origin = "two.example.test" }} {
		mutate()
		before = p.pulls
		if w = call(h, "/v1/cloud/sync/pull", nextBody); w.Code != 400 || p.pulls != before {
			t.Fatalf("pull cursor scope leaked %d", w.Code)
		}
	}
	who = owner
	h.Origin = "one.example.test"
	otherDevice := `{"family_id":"` + family + `","access_token":"synthetic","device_id":"88888888-8888-4888-8888-888888888888","limit":10,"cursor":"` + first.Next + `"}`
	if w = call(h, "/v1/cloud/sync/pull", otherDevice); w.Code != 400 {
		t.Fatal("device crossover cursor accepted")
	}
	if w = call(h, "/v1/cloud/sync/pull", strings.Replace(nextBody, first.Next, "forged", 1)); w.Code != 400 {
		t.Fatal("forged cursor accepted")
	}
	for _, bad := range []string{
		strings.Replace(base, `"limit":10`, `"limit":0`, 1),
		strings.Replace(base, `"limit":10`, `"limit":101`, 1),
		strings.Replace(base, `"device_id"`, `"account_id"`, 1),
		strings.Replace(base, `"limit":10`, `"limit":10,"limit":11`, 1),
		strings.Replace(base, `"device_id":"`+device+`"`, `"device_id":"not-a-uuid"`, 1),
	} {
		before = p.pulls
		if w = call(h, "/v1/cloud/sync/pull", bad); w.Code != 400 || p.pulls != before {
			t.Fatalf("bad pull admitted %d %s", w.Code, bad)
		}
	}
	unsigned := Handler{Service: p, Origin: h.Origin, CursorSecret: h.CursorSecret, Sessions: h.Sessions}
	if w = call(unsigned, "/v1/cloud/sync/pull", base); w.Code != 401 {
		t.Fatal("unsigned pull admitted")
	}
	p.err = domain.ErrResnapshot
	if w = call(h, "/v1/cloud/sync/pull", nextBody); w.Code != 409 {
		t.Fatal("resnapshot hidden")
	}
	p.err = domain.ErrUnauthorized
	if w = call(h, "/v1/cloud/sync/pull", nextBody); w.Code != 401 {
		t.Fatal("revoked pull hidden")
	}
	p.err = nil
}

func TestSyncPullExpiredCursorRequiresResnapshot(t *testing.T) {
	device := "55555555-5555-4555-8555-555555555555"
	who := owner
	p := &probe{pullFeed: app.Feed{Through: 1, Epoch: 1, ConsentRevision: 1}}
	h := pullHandler(p, &who)
	filter := pullFilter(owner, h.Origin, device)
	expired := sealPull(h.CursorSecret, filter, 1, 1, 1, 1, time.Now().Add(-16*time.Minute))
	body := `{"family_id":"` + family + `","access_token":"synthetic","device_id":"` + device + `","limit":10,"cursor":"` + expired + `"}`
	before := p.pulls
	if w := call(h, "/v1/cloud/sync/pull", body); w.Code != 409 || p.pulls != before {
		t.Fatalf("expired cursor did not require resnapshot %d", w.Code)
	}
}
