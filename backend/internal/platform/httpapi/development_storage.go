package httpapi

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// DevelopmentStorageProxy exposes only one private S3 bucket through the
// already owned HTTPS origin. S3 authenticates every presigned/client request;
// there is no anonymous bucket policy or console route. Composition enables
// this only in the explicitly bounded non-production development environment.
func DevelopmentStorageProxy(bucket string) http.Handler {
	target, _ := url.Parse("http://127.0.0.1:9000")
	return developmentStorageProxy(bucket, target)
}

func developmentStorageProxy(bucket string, target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, _ error) {
		WriteError(w, r, 503, "development.media-unavailable", "development media unavailable", nil)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bucket != "abastevo-dev-photos" || !strings.HasPrefix(r.URL.Path, "/"+bucket+"/") ||
			(r.Method != http.MethodPut && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodDelete) ||
			(r.URL.Query().Get("X-Amz-Signature") == "" && !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ")) {
			WriteError(w, r, 403, "development.media-private", "private development media", nil)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		proxy.ServeHTTP(w, r)
	})
}
