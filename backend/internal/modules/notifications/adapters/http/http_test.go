package http

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

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

const (
	family      = "11111111-1111-4111-8111-111111111111"
	owner       = "22222222-2222-4222-8222-222222222222"
	other       = "99999999-9999-4999-8999-999999999999"
	notifA      = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	notifB      = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	opID        = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	secret      = "0123456789abcdef0123456789abcdef"
	sessionJSON = `"family_id":"` + family + `","access_token":"synthetic"`
)

var fixed = time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

type probe struct {
	limit     int
	before    *domain.Position
	page      application.Page
	selection domain.Selection
	marks     int
	err       error
}

func (p *probe) List(_ context.Context, account string, before *domain.Position, limit int) (application.Page, error) {
	if account != owner {
		return application.Page{}, errors.New("wrong owner")
	}
	p.limit, p.before = limit, before
	return p.page, p.err
}

func (p *probe) MarkRead(_ context.Context, account string, sel domain.Selection) (int64, error) {
	if account != owner {
		return 0, errors.New("wrong owner")
	}
	p.marks++
	p.selection = sel
	return int64(len(sel.IDs)), p.err
}

func handler(p *probe, who *string, signedBody *string) Handler {
	return Handler{
		Service:      p,
		CursorSecret: []byte(secret),
		Origin:       "api.test",
		Clock:        func() time.Time { return fixed },
		Sessions: func(_ context.Context, f, a string) (string, error) {
			if f != family || a != "synthetic" {
				return "", errors.New("session store says no")
			}
			return *who, nil
		},
		SignedOwner: func(r *http.Request, account string) (string, string, error) {
			raw, _ := io.ReadAll(r.Body)
			if signedBody != nil {
				*signedBody = string(raw)
			}
			if account != owner {
				return "", "", errors.New("key bound to another account")
			}
			return "contributor", "key", nil
		},
	}
}

func call(h Handler, path, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body %s", w.Body.String())
	}
	return body.Error.Code, body.Error.Message
}

func TestListReturnsOwnedPageNoStore(t *testing.T) {
	who := owner
	p := &probe{page: application.Page{Unread: 1, Items: []application.Item{{
		NotificationID: notifA, Kind: domain.KindForumReply, CreatedAt: "2026-11-28T11:00:00Z",
		Payload: json.RawMessage(`{"topic_id":"x"}`),
	}}}}
	w := call(handler(p, &who, nil), "/v1/notifications/list", `{`+sessionJSON+`}`)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
		t.Fatalf("list %d %v", w.Code, w.Header())
	}
	if p.limit != domain.DefaultListLimit || p.before != nil {
		t.Fatalf("default limit %d before %v", p.limit, p.before)
	}
	want := `{"items":[{"notification_id":"` + notifA + `","kind":"forum.reply","created_at":"2026-11-28T11:00:00Z","read":false,"payload":{"topic_id":"x"}}],"next_before":null,"unread":1}`
	if got := w.Body.String(); got != want {
		t.Fatalf("body\n got %s\nwant %s", got, want)
	}
	p.page = application.Page{}
	if w := call(handler(p, &who, nil), "/v1/notifications/list", `{`+sessionJSON+`,"limit":50}`); w.Code != 200 || w.Body.String() != `{"items":[],"next_before":null,"unread":0}` || p.limit != 50 {
		t.Fatalf("empty page %d %s", w.Code, w.Body.String())
	}
}

func TestListCursorIsSealedAndOwnerBound(t *testing.T) {
	who := owner
	next := domain.Position{CreatedAt: fixed.Add(-time.Hour), ID: notifB}
	p := &probe{page: application.Page{Items: []application.Item{}, Next: &next}}
	h := handler(p, &who, nil)
	w := call(h, "/v1/notifications/list", `{`+sessionJSON+`,"limit":1}`)
	var first struct {
		NextBefore *string `json:"next_before"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil || first.NextBefore == nil || strings.Contains(*first.NextBefore, notifB) {
		t.Fatalf("cursor must be opaque: %s", w.Body.String())
	}
	if w := call(h, "/v1/notifications/list", `{`+sessionJSON+`,"limit":1,"before":"`+*first.NextBefore+`"}`); w.Code != 200 {
		t.Fatalf("cursor replay %d", w.Code)
	}
	if p.before == nil || !p.before.CreatedAt.Equal(next.CreatedAt) || p.before.ID != notifB {
		t.Fatalf("decoded position %+v", p.before)
	}
	tampered := (*first.NextBefore)[:len(*first.NextBefore)-2] + "AA"
	for name, body := range map[string]string{
		"tampered": `{` + sessionJSON + `,"before":"` + tampered + `"}`,
		"garbage":  `{` + sessionJSON + `,"before":"n1|1|` + notifB + `"}`,
		"empty":    `{` + sessionJSON + `,"before":""}`,
		"huge":     `{` + sessionJSON + `,"before":"` + strings.Repeat("A", maxCursor+1) + `"}`,
	} {
		if w := call(h, "/v1/notifications/list", body); w.Code != 400 {
			t.Errorf("%s cursor admitted: %d", name, w.Code)
		}
	}
	// Another account cannot reuse the cursor: the filter binds the owner.
	who = other
	if w := call(handler(&probe{}, &who, nil), "/v1/notifications/list", `{`+sessionJSON+`,"before":"`+*first.NextBefore+`"}`); w.Code != 400 {
		t.Fatalf("foreign cursor admitted: %d", w.Code)
	}
	// An expired cursor fails closed.
	later := h
	later.Clock = func() time.Time { return fixed.Add(time.Hour) }
	if w := call(later, "/v1/notifications/list", `{`+sessionJSON+`,"before":"`+*first.NextBefore+`"}`); w.Code != 400 {
		t.Fatalf("expired cursor admitted: %d", w.Code)
	}
}

func TestListRefusesMalformedRequests(t *testing.T) {
	who := owner
	h := handler(&probe{}, &who, nil)
	for name, body := range map[string]string{
		"limit zero":    `{` + sessionJSON + `,"limit":0}`,
		"limit 51":      `{` + sessionJSON + `,"limit":51}`,
		"unknown field": `{` + sessionJSON + `,"cursor":"x"}`,
		"duplicate key": `{` + sessionJSON + `,"limit":1,"limit":2}`,
		"short family":  `{"family_id":"x","access_token":"synthetic"}`,
		"no token":      `{"family_id":"` + family + `","access_token":""}`,
		"not object":    `[]`,
		"oversized":     `{` + sessionJSON + `,"before":"` + strings.Repeat("A", maxBody) + `"}`,
	} {
		if w := call(h, "/v1/notifications/list", body); w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, "/v1/notifications/list", strings.NewReader(`{`+sessionJSON+`}`))
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("content type admitted: %d", w.Code)
	}
}

func TestAuthFailuresShareOneGenericError(t *testing.T) {
	who := owner
	p := &probe{}
	h := handler(p, &who, nil)
	badSession := `{"family_id":"` + family + `","access_token":"stale"}`
	sessionFail := call(h, "/v1/notifications/list", badSession)
	readSessionFail := call(h, "/v1/notifications/read", `{"family_id":"`+family+`","access_token":"stale","operation_id":"`+opID+`","all":true}`)
	unsigned := h
	unsigned.SignedOwner = nil
	noProof := call(unsigned, "/v1/notifications/read", `{`+sessionJSON+`,"operation_id":"`+opID+`","all":true}`)
	rejecting := h
	rejecting.SignedOwner = func(*http.Request, string) (string, string, error) { return "", "", errors.New("nonce replayed") }
	badProof := call(rejecting, "/v1/notifications/read", `{`+sessionJSON+`,"operation_id":"`+opID+`","all":true}`)
	who = "not-a-uuid"
	badOwner := call(h, "/v1/notifications/list", `{`+sessionJSON+`}`)
	var code, msg string
	for name, w := range map[string]*httptest.ResponseRecorder{
		"list session": sessionFail, "read session": readSessionFail, "missing proof": noProof,
		"rejected proof": badProof, "malformed owner": badOwner,
	} {
		if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d", name, w.Code)
		}
		c, m := errorCode(t, w)
		if code == "" {
			code, msg = c, m
		}
		if c != "notifications.auth-required" || c != code || m != msg {
			t.Fatalf("%s leaks a distinct error: %s %s", name, c, m)
		}
	}
	if p.marks != 0 {
		t.Fatal("unauthenticated write reached the service")
	}
}

func TestReadIsSignedAndBounded(t *testing.T) {
	who := owner
	p := &probe{}
	var signed string
	h := handler(p, &who, &signed)
	body := `{` + sessionJSON + `,"operation_id":"` + opID + `","notification_ids":["` + notifA + `","` + notifA + `","` + notifB + `"]}`
	w := call(h, "/v1/notifications/read", body)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Body.String() != `{"marked":2}` {
		t.Fatalf("read %d %s", w.Code, w.Body.String())
	}
	if signed != body {
		t.Fatal("the proof must cover the exact transmitted body")
	}
	if len(p.selection.IDs) != 2 || p.selection.All {
		t.Fatalf("selection %+v", p.selection)
	}
	if w := call(h, "/v1/notifications/read", `{`+sessionJSON+`,"operation_id":"`+opID+`","all":true}`); w.Code != 200 || !p.selection.All {
		t.Fatalf("all %d %+v", w.Code, p.selection)
	}
	fifty := make([]string, 51)
	for i := range fifty {
		fifty[i] = `"` + notifA + `"`
	}
	marks := p.marks
	for name, b := range map[string]string{
		"no operation":  `{` + sessionJSON + `,"all":true}`,
		"bad operation": `{` + sessionJSON + `,"operation_id":"op-1","all":true}`,
		"no selection":  `{` + sessionJSON + `,"operation_id":"` + opID + `"}`,
		"empty ids":     `{` + sessionJSON + `,"operation_id":"` + opID + `","notification_ids":[]}`,
		"51 ids":        `{` + sessionJSON + `,"operation_id":"` + opID + `","notification_ids":[` + strings.Join(fifty, ",") + `]}`,
		"both forms":    `{` + sessionJSON + `,"operation_id":"` + opID + `","all":true,"notification_ids":["` + notifA + `"]}`,
		"all false":     `{` + sessionJSON + `,"operation_id":"` + opID + `","all":false}`,
		"bad id":        `{` + sessionJSON + `,"operation_id":"` + opID + `","notification_ids":["1"]}`,
		"unknown field": `{` + sessionJSON + `,"operation_id":"` + opID + `","all":true,"ids":[]}`,
	} {
		if w := call(h, "/v1/notifications/read", b); w.Code != 400 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if p.marks != marks {
		t.Fatal("invalid writes reached the service")
	}
}

func TestStoreFailuresAreUnavailable(t *testing.T) {
	who := owner
	p := &probe{err: application.ErrUnavailable}
	h := handler(p, &who, nil)
	if w := call(h, "/v1/notifications/list", `{`+sessionJSON+`}`); w.Code != 503 {
		t.Fatalf("list %d", w.Code)
	}
	if w := call(h, "/v1/notifications/read", `{`+sessionJSON+`,"operation_id":"`+opID+`","all":true}`); w.Code != 503 {
		t.Fatalf("read %d", w.Code)
	}
	noSecret := handler(&probe{}, &who, nil)
	noSecret.CursorSecret = nil
	if w := call(noSecret, "/v1/notifications/list", `{`+sessionJSON+`}`); w.Code != 503 {
		t.Fatalf("missing cursor secret %d", w.Code)
	}
	if w := call(Handler{}, "/v1/notifications/list", `{`+sessionJSON+`}`); w.Code != 503 {
		t.Fatalf("unwired handler %d", w.Code)
	}
}
