package http

import (
	"net/http"
	"testing"
)

// TestCompleteIsNeverSharedCached proves the finalize intent carries
// no-store: completion acknowledgments must never sit in shared
// caches (reserve and status coverage lives in http_test.go).
func TestCompleteIsNeverSharedCached(t *testing.T) {
	w := serve(testHandler(), http.MethodPost, "/v1/uploads/e0000000-0000-4000-8000-000000000001/complete", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("complete = %d, want 202", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("complete cache = %q, want no-store", cc)
	}
}
