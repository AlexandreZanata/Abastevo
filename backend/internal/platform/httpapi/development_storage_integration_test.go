//go:build integration

package httpapi

import (
	"bytes"
	storage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// The integration always runs. An isolated S3 protocol peer asserts that the
// proxy preserves the signed authority, query, path, MIME and bytes; it needs
// no fixed workstation port. Actual MinIO/API/worker bytes are additionally
// exercised by the scoped process matrix before deployment.
func TestDevelopmentStorageSignedProxyRoundTrip(t *testing.T) {
	var mutex sync.Mutex
	expected := map[string]string{}
	var originalHost string
	var object []byte
	calls := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		calls++
		if r.Host != originalHost || r.URL.Path != "/abastevo-dev-photos/q/proxy.jpg" || r.URL.RawQuery != expected[r.Method] {
			t.Error("proxy changed S3 signed authority/path/query")
			w.WriteHeader(403)
			return
		}
		switch r.Method {
		case "PUT":
			if r.Header.Get("Content-Type") != "image/jpeg" {
				t.Error("changed signed MIME")
			}
			object, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
		case "GET":
			w.WriteHeader(200)
			_, _ = w.Write(object)
		case "DELETE":
			object = nil
			w.WriteHeader(204)
		}
	}))
	defer peer.Close()
	target, _ := url.Parse(peer.URL)
	server := httptest.NewServer(developmentStorageProxy("abastevo-dev-photos", target))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	originalHost = endpoint.Host
	cred := storage.Credentials{AccessKeyID: "synthetic-upload", SecretAccessKey: "synthetic-local-only"}
	in := storage.PresignInput{Endpoint: server.URL, Bucket: "abastevo-dev-photos", Key: "q/proxy.jpg", Namespace: "q/", ContentType: "image/jpeg", MaxBytes: 10, Region: "us-east-1", Now: time.Now()}
	put, err := storage.PresignPUT(in, cred)
	if err != nil {
		t.Fatal(err)
	}
	get, err := storage.PresignGET(in, cred)
	if err != nil {
		t.Fatal(err)
	}
	del, err := storage.PresignDELETE(in, cred)
	if err != nil {
		t.Fatal(err)
	}
	for method, value := range map[string]string{"PUT": put.URL, "GET": get.URL, "DELETE": del.URL} {
		parsed, _ := url.Parse(value)
		expected[method] = parsed.RawQuery
	}
	req, _ := http.NewRequest("PUT", put.URL, bytes.NewReader([]byte("test-photo")))
	req.Header.Set("Content-Type", "image/jpeg")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("PUT", resp.StatusCode)
	}
	resp, err = server.Client().Get(get.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "test-photo" {
		t.Fatal("GET", resp.StatusCode)
	}
	resp, err = server.Client().Get(server.URL + "/abastevo-dev-photos/" + in.Key)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("public", resp.StatusCode)
	}
	req, _ = http.NewRequest("DELETE", del.URL, nil)
	resp, err = server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatal("DELETE", resp.StatusCode)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if calls != 3 {
		t.Fatal("unsigned request reached private S3 peer")
	}
}
