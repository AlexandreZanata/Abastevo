package httpserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOrganizationNamespaceErrorsArePrivateAndUniform(t *testing.T) {
	var logs bytes.Buffer
	router := NewRouter(testOptions(&logs).Logger)
	router.Post("/v1/orgs/{org_id}/registered", func(http.ResponseWriter, *http.Request) { panic("synthetic private failure") })
	for _, test := range []struct {
		method, path, code string
		status             int
	}{
		{"POST", "/v1/orgs/foreign/missing", "NOT_FOUND", 404},
		{"GET", "/v1/orgs/foreign/registered", "NOT_FOUND", 404},
		{"POST", "/v1/org-access/missing", "NOT_FOUND", 404},
		{"POST", "/v1/orgs/foreign/registered", "UNAVAILABLE", 503},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, nil)
		request.Header.Set("If-None-Match", "*")
		router.ServeHTTP(response, request)
		if response.Code != test.status || response.Header().Get("Cache-Control") != "no-store" ||
			response.Header().Get("ETag") != "" || !strings.Contains(response.Body.String(), test.code) ||
			strings.Contains(response.Body.String(), "synthetic private failure") {
			t.Fatalf("unsafe namespace response %d %s", response.Code, response.Body.String())
		}
	}
}
