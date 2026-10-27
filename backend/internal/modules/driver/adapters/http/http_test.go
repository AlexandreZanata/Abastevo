package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/go-chi/chi/v5"
)

type earningsStub struct{ records []app.StoredEarning }

func (s earningsStub) List(_ context.Context, _, _ string, limit int) (app.Page, error) {
	out := app.Page{Records: s.records}
	if len(out.Records) > limit {
		out.Records = out.Records[:limit]
	}
	return out, nil
}

type reportStub struct {
	report app.Report
	err    error
	getErr error
}

func (s reportStub) Build(context.Context, string, app.ReportRequest) (app.Report, error) {
	return s.report, s.err
}
func (s reportStub) Get(context.Context, string, app.ReportRequest) (app.Report, error) {
	return s.report, s.getErr
}

func driverHandler(owner string) Handler {
	return Handler{
		Earnings: earningsStub{records: []app.StoredEarning{
			{Record: domain.EarningRecord{ID: "11111111-1111-4111-8111-111111111111", VehicleID: "22222222-2222-4222-8222-222222222222", Platform: "UBER", Coverage: "SUMMARY", Currency: "BRL", From: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC), Timezone: "UTC", Money: domain.Earnings{Basis: "INCLUSIVE_NET", Net: ptr(45_000)}}, Revision: 1},
			{Record: domain.EarningRecord{ID: "33333333-3333-4333-8333-333333333333", VehicleID: "22222222-2222-4222-8222-222222222222", Platform: "UBER", Coverage: "SUMMARY", Currency: "BRL", From: time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC), Timezone: "UTC", Money: domain.Earnings{Basis: "INCLUSIVE_NET", Net: ptr(10_000)}}, Revision: 1},
		}},
		Reports: reportStub{report: app.Report{Revision: strings.Repeat("a", 64), SnapshotVersion: 1}},
		Sessions: func(_ context.Context, family, _ string) (string, error) {
			if family == "44444444-4444-4444-8444-444444444444" {
				return owner, nil
			}
			return "55555555-5555-4555-8555-555555555555", nil
		},
		SignedOwner: func(r *http.Request, _ string) (string, string, error) {
			if r.Header.Get("X-Signed") != "yes" {
				return "", "", errors.New("unsigned")
			}
			return "66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777", nil
		},
		CursorSecret: []byte(strings.Repeat("s", 32)), Origin: "one.example.test",
	}
}

func ptr(v int64) *int64 { return &v }

func call(h Handler, path, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Signed", "yes")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestDriverHTTPRequiresSignedOwnerAndUsesNoStore(t *testing.T) {
	h := driverHandler("88888888-8888-4888-8888-888888888888")
	unsigned := httptest.NewRequest(http.MethodPost, "/v1/driver/reports/get", strings.NewReader(`{"family_id":"44444444-4444-4444-8444-444444444444","access_token":"x","vehicle_id":"22222222-2222-4222-8222-222222222222","timezone":"UTC","period":"DAY","anchor":"2026-10-26","basis":"ODOMETER"}`))
	unsigned.Header.Set("Content-Type", "application/json")
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, unsigned)
	if w.Code != 401 {
		t.Fatal("unsigned", w.Code)
	}
	h.Enabled = func() bool { return false }
	w = call(h, "/v1/driver/reports/get", `{"family_id":"44444444-4444-4444-8444-444444444444","access_token":"x"}`)
	if w.Code != 404 {
		t.Fatal("disabled", w.Code)
	}
	h.Enabled = nil
	h.Reports = reportStub{err: app.ErrStale}
	w = call(h, "/v1/driver/reports/build", `{"family_id":"44444444-4444-4444-8444-444444444444","access_token":"x","vehicle_id":"22222222-2222-4222-8222-222222222222","timezone":"UTC","period":"DAY","anchor":"2026-10-26","basis":"ODOMETER"}`)
	if w.Code != 409 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
		t.Fatal("stale mapping", w.Code, w.Header())
	}
}

func TestDriverHTTPEarningsCursorIsOwnerBoundAndNoStore(t *testing.T) {
	h := driverHandler("88888888-8888-4888-8888-888888888888")
	w := call(h, "/v1/driver/earnings", `{"family_id":"44444444-4444-4444-8444-444444444444","access_token":"x","limit":1}`)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("page", w.Code, w.Body.String())
	}
	var first struct {
		Items []earningItem `json:"items"`
		Next  string        `json:"next_cursor"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &first); e != nil || len(first.Items) != 1 || first.Next == "" {
		t.Fatal("page body", first, e)
	}
	other := driverHandler("99999999-9999-4999-8999-999999999999")
	w = call(other, "/v1/driver/earnings", `{"family_id":"55555555-5555-4555-8555-555555555555","access_token":"x","limit":1,"cursor":"`+first.Next+`"}`)
	if w.Code != 400 {
		t.Fatal("cursor crossed owner", w.Code, w.Body.String())
	}
}
