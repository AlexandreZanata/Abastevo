package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/go-chi/chi/v5"
)

const family = "11111111-1111-4111-8111-111111111111"
const owner = "22222222-2222-4222-8222-222222222222"
const foreignOwner = "33333333-3333-4333-8333-333333333333"
const operation = "44444444-4444-4444-8444-444444444444"

type apiProbe struct {
	calls     int
	principal app.Principal
	command   app.Command
	rows      []app.ExportRow
	after     time.Time
	key       string
	limit     int
	err       error
}

func (s *apiProbe) Status(_ context.Context, p app.Principal) (domain.Status, error) {
	s.calls++
	s.principal = p
	return domain.Evaluate(nil, time.Now(), true), s.err
}
func (s *apiProbe) Admit(_ context.Context, p app.Principal, c app.Command) (app.Receipt, error) {
	s.calls++
	s.principal = p
	s.command = c
	return app.Receipt{OperationID: c.OperationID, Status: domain.Evaluate(nil, time.Now(), true)}, s.err
}
func (s *apiProbe) Export(_ context.Context, p app.Principal, n int, after time.Time, key string) ([]app.ExportRow, error) {
	s.calls++
	s.principal = p
	s.after = after
	s.key = key
	s.limit = n
	return s.rows, s.err
}
func request(h Handler, path, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-None-Match", "*")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}
func handler(s *apiProbe) Handler {
	return Handler{Service: s, CursorSecret: []byte(strings.Repeat("s", 32)), Sessions: func(_ context.Context, f, a string) (string, error) {
		if f != family || a != "synthetic-access" {
			return "", domain.ErrUnauthorized
		}
		return owner, nil
	}}
}
func sessionBody() string { return `{"family_id":"` + family + `","access_token":"synthetic-access"}` }
func TestPrivateStatusDerivesOwnerAndNeverCaches(t *testing.T) {
	s := &apiProbe{}
	h := handler(s)
	w := request(h, "/v1/premium/status", sessionBody())
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" || s.principal.AccountID != owner {
		t.Fatalf("private status: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Management string `json:"management_url"`
		Retained   bool   `json:"retained_data_access"`
		Payments   bool   `json:"payments_configured"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Management != "https://play.google.com/store/account/subscriptions" || !response.Retained || response.Payments {
		t.Fatal("free status contract", err)
	}
	for _, body := range []string{
		strings.Replace(sessionBody(), "synthetic-access", "forged", 1),
		strings.Replace(sessionBody(), `"access_token":`, `"account_id":"`+foreignOwner+`","access_token":`, 1),
		strings.Replace(sessionBody(), `"access_token":`, `"is_premium":true,"access_token":`, 1),
		strings.Replace(sessionBody(), `"access_token":`, `"access_token":"duplicate","access_token":`, 1),
	} {
		before := s.calls
		w = request(h, "/v1/premium/status", body)
		if w.Code != 400 && w.Code != 401 {
			t.Fatalf("forged request accepted %d", w.Code)
		}
		if s.calls != before || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("invalid request called service/cached")
		}
	}
}
func TestPurchaseRestoreRequiresSignedSameOwnerAndExactBody(t *testing.T) {
	body := strings.TrimSuffix(sessionBody(), "}") + `,"operation_id":"` + operation + `","purchase_token":"synthetic-token"}`
	for _, kind := range []string{"verify", "restore"} {
		s := &apiProbe{}
		h := handler(s)
		w := request(h, "/v1/premium/purchases/"+kind, body)
		if w.Code != 401 || s.calls != 0 {
			t.Fatal("unsigned command reached admission")
		}
		h.SignedOwner = func(r *http.Request, account string) (string, string, error) {
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != body || account != owner {
				t.Fatal("signature body/owner changed")
			}
			return foreignOwner, operation, nil
		}
		w = request(h, "/v1/premium/purchases/"+kind, body)
		if w.Code != 200 || s.command.Kind != kind || s.command.Token != "synthetic-token" || s.principal.AccountID != owner || s.principal.ContributorID != foreignOwner {
			t.Fatal("signed command lost binding", w.Code)
		}
		s.err = domain.ErrDenied
		w = request(h, "/v1/premium/purchases/"+kind, body)
		if w.Code != 403 {
			t.Fatal("foreign purchase proof accepted")
		}
		s.err = domain.ErrUnavailable
		w = request(h, "/v1/premium/purchases/"+kind, body)
		if w.Code != 503 {
			t.Fatal("provider outage hidden")
		}
	}
}
func TestFreeExportPaginationCursorBoundToOwner(t *testing.T) {
	now := time.Now().UTC()
	s := &apiProbe{rows: []app.ExportRow{
		{State: domain.Expired, VerifiedAt: now, Digest: strings.Repeat("a", 64)},
		{State: domain.Revoked, VerifiedAt: now.Add(-time.Hour), Digest: strings.Repeat("b", 64)},
	}}
	h := handler(s)
	body := strings.TrimSuffix(sessionBody(), "}") + `,"limit":1}`
	w := request(h, "/v1/premium/export", body)
	if w.Code != 200 || s.limit != 2 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("free export refused", w.Code)
	}
	var response struct {
		Items []app.ExportRow `json:"items"`
		Next  string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || len(response.Items) != 1 || response.Next == "" {
		t.Fatal("page contract", err)
	}
	if strings.Contains(w.Body.String(), strings.Repeat("a", 64)) || strings.Contains(w.Body.String(), "purchase_token") || strings.Contains(w.Body.String(), "Digest") {
		t.Fatal("internal token material leaked")
	}
	page := strings.TrimSuffix(body, "}") + `,"cursor":"` + response.Next + `"}`
	s.rows = nil
	w = request(h, "/v1/premium/export", page)
	if w.Code != 200 || s.key != strings.Repeat("a", 64) || !s.after.Equal(now) || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("cursor position/empty page", w.Code)
	}
	for _, bad := range []string{
		strings.Replace(body, `"limit":1`, `"limit":0`, 1),
		strings.Replace(page, response.Next, "forged", 1),
		strings.Replace(body, `"limit":1`, `"limit":101`, 1),
		strings.Replace(body, `"limit":1`, `"limit":-1`, 1),
	} {
		before := s.calls
		w = request(h, "/v1/premium/export", bad)
		if w.Code != 400 || s.calls != before {
			t.Fatal("bad pagination reached store", w.Code)
		}
	}
	h.Sessions = func(context.Context, string, string) (string, error) { return foreignOwner, nil }
	before := s.calls
	w = request(h, "/v1/premium/export", page)
	if w.Code != 400 || s.calls != before {
		t.Fatal("cursor crossed owners")
	}
}
