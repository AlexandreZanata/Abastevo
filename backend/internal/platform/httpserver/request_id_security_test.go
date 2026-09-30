package httpserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

func TestRequestIDRejectsPrivateUnboundedAndControlValues(t *testing.T) {
	var logs bytes.Buffer
	opts := testOptions(&logs)
	router := NewRouter(opts.Logger)
	router.Get("/error", func(w http.ResponseWriter, r *http.Request) { httpapi.WriteError(w, r, 400, "test.error", "safe", nil) })
	for _, raw := range []string{"client-id_1", "gps=-23.55555,-46.66666", strings.Repeat("x", 1000), "control\nvalue"} {
		req := httptest.NewRequest("GET", "/error", nil)
		req.Header.Set("X-Request-ID", raw)
		w := httptest.NewRecorder()
		requestID(router).ServeHTTP(w, req)
		chosen := w.Header().Get("X-Request-ID")
		if !safeRequestID(chosen) || !strings.Contains(w.Body.String(), `"trace_id":"`+chosen+`"`) {
			t.Fatal("unsafe or inconsistent trace ID")
		}
		if raw != "client-id_1" && chosen == raw {
			t.Fatal("private request ID accepted")
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing nosniff")
		}
	}
}
