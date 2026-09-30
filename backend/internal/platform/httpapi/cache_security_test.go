package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConditionalNeverSuppressesPrivateOrMutationResponse(t *testing.T) {
	for _, tc := range []struct {
		method, cache string
		status        int
	}{
		{"POST", "no-store", 201}, {"GET", "no-store", 200}, {"POST", "public, max-age=60", 201},
	} {
		t.Run(tc.method+tc.cache, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/v1/observations", nil)
			r.Header.Set("If-None-Match", "*")
			w := httptest.NewRecorder()
			WriteJSON(w, r, tc.status, tc.cache, []byte(`{"id":"created"}`))
			if w.Code != tc.status || w.Body.Len() == 0 {
				t.Fatalf("response suppressed: %d", w.Code)
			}
		})
	}
}
func TestETagUsesHTTPWeakSyntax(t *testing.T) {
	if !strings.HasPrefix(ETag([]byte("body")), `W/"`) {
		t.Fatal("invalid weak entity tag")
	}
}
