package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDevelopmentStorageProxyRefusesAnonymousWrongBucketConsoleAndMethods(t *testing.T) {
	for _, tc := range []struct{ method, path, bucket string }{
		{"GET", "/abastevo-dev-photos/private", "abastevo-dev-photos"},
		{"POST", "/abastevo-dev-photos/private?X-Amz-Signature=fake", "abastevo-dev-photos"},
		{"GET", "/other/private?X-Amz-Signature=fake", "abastevo-dev-photos"},
		{"GET", "/abastevo-dev-photos/?X-Amz-Signature=fake", "other"},
		{"GET", "/console/?X-Amz-Signature=fake", "abastevo-dev-photos"},
	} {
		w := httptest.NewRecorder()
		DevelopmentStorageProxy(tc.bucket).ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s=%d", tc.path, w.Code)
		}
	}
}
