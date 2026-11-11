package http_test

import (
	web "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompanyHTTPRejectsAmbiguousTransportBeforeCredentials(t *testing.T) {
	service := &app.Service{Enabled: true}
	handler, err := web.New(service, "https://company.example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	for _, test := range []struct {
		name, body string
		status     int
		change     func(*http.Request)
	}{
		{"missing origin", "{}", 403, func(r *http.Request) { r.Header.Del("Origin") }},
		{"foreign origin", "{}", 403, func(r *http.Request) { r.Header.Set("Origin", "https://other.example.invalid") }},
		{"multiple origins", "{}", 403, func(r *http.Request) { r.Header.Add("Origin", "https://company.example.invalid") }},
		{"wrong host", "{}", 403, func(r *http.Request) { r.Host = "other.example.invalid" }},
		{"cross site", "{}", 403, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }},
		{"signed channel", "{}", 401, func(r *http.Request) { r.Header.Set("Signature", "synthetic") }},
		{"bearer channel", "{}", 401, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }},
		{"wrong type", "{}", 400, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
		{"ambiguous type", "{}", 400, func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }},
		{"query credentials", "{}", 400, func(r *http.Request) { r.URL.RawQuery = "token=synthetic" }},
		{"null", "null", 400, nil},
		{"null proof", `{"family_id":null,"access_token":null}`, 400, nil},
		{"unknown", "{\"family_id\":\"id\",\"access_token\":\"value\",\"account_id\":\"id\"}", 400, nil},
		{"duplicate", "{\"family_id\":\"a\",\"family_id\":\"b\",\"access_token\":\"value\"}", 400, nil},
		{"unicode alias", "{\"family_id\":\"id\",\"acceſſ_token\":\"value\"}", 400, nil},
		{"oversize", strings.Repeat(" ", web.MaxBodyBytes+1), 400, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://company.example.invalid"+web.PathPrefix+"login", strings.NewReader(test.body))
			r.Header.Set("Origin", "https://company.example.invalid")
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("If-None-Match", "*")
			if test.change != nil {
				test.change(r)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != test.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" || len(w.Result().Cookies()) != 0 {
				t.Fatalf("status=%d expected=%d", w.Code, test.status)
			}
		})
	}
	for _, origin := range []string{"http://company.example.invalid", "https://user:pass@example.invalid", "https://company.example.invalid/path", "https://company.example.invalid?key=value"} {
		if _, err := web.New(service, origin); err == nil {
			t.Fatal("invalid origin accepted")
		}
	}
	service.Enabled = false
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", web.PathPrefix+"login", nil))
	if w.Code != 404 {
		t.Fatal("disabled company routes exposed")
	}
}
