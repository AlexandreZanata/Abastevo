package main

import (
	"net/http/httptest"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

// The private document ingress alone may carry one exact 5 MiB JPEG plus the
// single byte that proves an oversized body; every other route keeps its cap.
func TestDocumentIngressBodyLimitIsExactAndNarrow(t *testing.T) {
	cfg := config.Config{Env: config.EnvProduction, MaxBodyBytes: 1 << 20}
	const path = "/v1/orgs/7b0e2a54-0000-4000-8000-000000020531/documents/7b0e2a54-0000-4000-8000-000000020532/content/upload"
	if got := requestBodyLimit(cfg)(httptest.NewRequest("POST", path, nil)); got != 5<<20+1 {
		t.Fatalf("document ingress cap = %d, want 5 MiB + 1", got)
	}
	for _, tc := range []struct{ method, path string }{
		{"PUT", path},
		{"POST", "/v1/orgs/7b0e2a54-0000-4000-8000-000000020531/documents/7b0e2a54-0000-4000-8000-000000020532/content/read"},
		{"POST", "/v1/orgs/7b0e2a54-0000-4000-8000-000000020531/documents/uploads/create"},
		{"POST", "/v1/orgs/7b0e2a54-0000-4000-8000-000000020531/fleet/assets/7b0e2a54-0000-4000-8000-000000020532/content/upload"},
		{"POST", "/v1/orgs/7b0e2a54-0000-4000-8000-000000020531/documents/7b0e2a54-0000-4000-8000-000000020532/content/upload/extra"},
		{"POST", "/v1/other/7b0e2a54-0000-4000-8000-000000020531/documents/7b0e2a54-0000-4000-8000-000000020532/content/upload"},
	} {
		if got := requestBodyLimit(cfg)(httptest.NewRequest(tc.method, tc.path, nil)); got != 1<<20 {
			t.Fatal("document ingress cap widened another route", tc)
		}
	}
}
