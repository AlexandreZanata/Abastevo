package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivateProfileContainment(t *testing.T) {
	for _, body := range []string{"", `{"family_id":"stolen","access_token":"token"}`, strings.Repeat("x", 7<<20)} {
		touched := false
		h := privateProfileUnavailable(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { touched = true }))
		r := httptest.NewRequest(http.MethodPost, "/v1/profile/claims/mine", strings.NewReader(body))
		r.Header.Set("Signature", "unverified")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if touched || w.Code != http.StatusServiceUnavailable || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("private handler escaped containment: touched=%v status=%d cache=%s", touched, w.Code, w.Header().Get("Cache-Control"))
		}
		if !strings.Contains(w.Body.String(), "profile.integration-pending") || strings.Contains(w.Body.String(), "stolen") {
			t.Fatal("unexpected refusal or private input echoed")
		}
	}
}
