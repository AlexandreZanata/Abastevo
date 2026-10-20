package http

import (
	"bytes"
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type reportFake struct {
	calls   int
	owner   string
	command domain.ReportCommand
}

func (f *reportFake) Execute(_ context.Context, a adapters.ReportAccess, c domain.ReportCommand) (domain.ReportReceipt, error) {
	f.calls++
	f.owner = a.Owner
	f.command = c
	return domain.ReportReceipt{ReportID: c.ReportID, Revision: 1, Status: "PENDING_REVIEW"}, nil
}
func (f *reportFake) Get(_ context.Context, a adapters.ReportAccess, id string) (domain.Report, error) {
	f.calls++
	f.owner = a.Owner
	return domain.Report{}, domain.ErrReportNotFound
}
func (f *reportFake) List(_ context.Context, a adapters.ReportAccess, after string) ([]domain.Report, error) {
	f.calls++
	f.owner = a.Owner
	return []domain.Report{}, nil
}

func TestReportsHTTPAuthBodyBindingAndRecovery(t *testing.T) {
	const family = "11111111-1111-4111-8111-111111111111"
	raw := `{"family_id":"` + family + `","access_token":"fixture","command_id":"22222222-2222-4222-8222-222222222222","report_id":"33333333-3333-4333-8333-333333333333","action":"SUBMIT","kind":"EXPERIENCE","station_id":"44444444-4444-4444-8444-444444444444","fuel":"ETHANOL","occurred_on":"2026-10-13","symptom":"OTHER","text":"Private synthetic experience"}`
	for _, tc := range []struct {
		name, body           string
		sessionErr, proofErr error
		enabled              bool
		want                 int
		calls                int
	}{
		{"signed", raw, nil, nil, true, 200, 1},
		{"guest", `{}`, nil, nil, true, 401, 0},
		{"revoked session", raw, domain.ErrReportUnauthorized, nil, true, 401, 0},
		{"foreign proof", raw, nil, domain.ErrReportUnauthorized, true, 401, 0},
		{"forged owner", strings.TrimSuffix(raw, "}") + `,"account_id":"forged"}`, nil, nil, true, 400, 0},
		{"document field", strings.TrimSuffix(raw, "}") + `,"receipt_url":"private"}`, nil, nil, true, 400, 0},
		{"closed intake", raw, nil, nil, false, 503, 0},
		{"appeal while closed", `{"family_id":"` + family + `","access_token":"fixture","command_id":"22222222-2222-4222-8222-222222222222","report_id":"33333333-3333-4333-8333-333333333333","action":"APPEAL","expected_revision":2,"text":"Synthetic appeal"}`, nil, nil, false, 200, 1},
		{"withdraw while closed", `{"family_id":"` + family + `","access_token":"fixture","command_id":"22222222-2222-4222-8222-222222222222","report_id":"33333333-3333-4333-8333-333333333333","action":"WITHDRAW","expected_revision":1}`, nil, nil, false, 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &reportFake{}
			h := ReportsHandler{Service: f, Enabled: func() bool { return tc.enabled }, Sessions: func(context.Context, string, string) (string, error) { return "server-derived-owner", tc.sessionErr }, SignedOwner: func(r *http.Request, owner string) (string, string, error) {
				b, e := io.ReadAll(r.Body)
				if e != nil || !bytes.Equal(b, []byte(tc.body)) {
					t.Fatal("signature body changed")
				}
				return "contributor", "key", tc.proofErr
			}, Guard: func(context.Context, pgx.Tx, string, string, string, string, string) error { return nil }}
			router := chi.NewRouter()
			h.RegisterRoutes(router)
			req := httptest.NewRequest("POST", "/v1/quality/reports/commands", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want || f.calls != tc.calls {
				t.Fatal(w.Code, w.Body.String(), f.calls)
			}
			if f.calls > 0 && f.owner != "server-derived-owner" {
				t.Fatal("owner not derived")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private response cached")
			}
			if strings.Contains(w.Body.String(), "fixture") || strings.Contains(w.Body.String(), "server-derived-owner") {
				t.Fatal("credential/owner leak")
			}
		})
	}
}

func TestReportsHTTPOwnerReadsStaySignedAndNoStore(t *testing.T) {
	for _, path := range []string{"get", "list"} {
		f := &reportFake{}
		h := ReportsHandler{Service: f, Sessions: func(context.Context, string, string) (string, error) { return "owner", nil }, SignedOwner: func(*http.Request, string) (string, string, error) { return "", "", domain.ErrReportUnauthorized }, Guard: func(context.Context, pgx.Tx, string, string, string, string, string) error { return nil }}
		r := chi.NewRouter()
		h.RegisterRoutes(r)
		req := httptest.NewRequest("POST", "/v1/quality/reports/"+path, strings.NewReader(`{"family_id":"11111111-1111-4111-8111-111111111111","access_token":"fixture"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 401 || f.calls != 0 {
			t.Fatal(path, w.Code)
		}
	}
}

func TestReportQuotaHTTPPrivateRetry(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/quality/reports/commands", nil)
	reportFail(w, r, &domain.ReportQuotaError{RetryAfterSeconds: 42})
	if w.Code != 429 || w.Header().Get("Retry-After") != "42" || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "owner") {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
}
