package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPrivateStorageForwardingPreservesSignedHostPathAndQuery(t *testing.T) {
	const signedPath = "/abastevo-dev-photos/q/synthetic%20object?X-Amz-Signature=synthetic&X-Amz-SignedHeaders=host"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "public.example.invalid" || r.URL.RequestURI() != signedPath {
			t.Errorf("signed authority/path/query changed")
		}
		if r.Method != http.MethodPut {
			t.Errorf("method changed")
		}
		w.WriteHeader(204)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	proxy := httptest.NewServer(developmentStorageProxy("abastevo-dev-photos", target))
	defer proxy.Close()
	r, _ := http.NewRequest(http.MethodPut, proxy.URL+signedPath, nil)
	r.Host = "public.example.invalid"
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != 204 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal(resp.StatusCode)
	}
}
