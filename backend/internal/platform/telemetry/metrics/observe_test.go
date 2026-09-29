package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestObserveCountsTemplateLabels(t *testing.T) {
	reg := NewRegistry()
	reqs, err := reg.Counter("http_requests_total", "h", "method", "route", "class")
	if err != nil {
		t.Fatal(err)
	}
	secs, err := reg.Counter("http_request_seconds_total", "h", "method", "route", "class")
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Use(Observe(reg, reqs, secs))
	r.Get("/v1/stations/{station_id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/v1/boom", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	if _, err := http.Get(srv.URL + "/v1/stations/45d0e8bc-01b1-441e-9b28-f8c16df7cb35"); err != nil {
		t.Fatal(err)
	}
	if _, err := http.Get(srv.URL + "/v1/boom"); err != nil {
		t.Fatal(err)
	}
	if _, err := http.Get(srv.URL + "/nope"); err != nil {
		t.Fatal(err)
	}
	out := reg.Expose()
	for _, want := range []string{
		`http_requests_total{class="2xx",method="GET",route="/v1/stations/{station_id}"} 1`,
		`http_requests_total{class="5xx",method="GET",route="/v1/boom"} 1`,
		`http_requests_total{class="4xx",method="GET",route="unmatched"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n%s", want, out)
		}
	}
	// Raw identifiers must never become label values: only the
	// template travels.
	if strings.Contains(out, "45d0e8bc") {
		t.Error("raw station ID leaked into labels")
	}
}
