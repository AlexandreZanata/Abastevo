//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	storage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
)

func TestPrivateStorageServiceForwardingRealS3(t *testing.T) {
	endpoint := startPrivateProxyS3(t, "abastevo-dev-photos")
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(developmentStorageProxy("abastevo-dev-photos", u))
	t.Cleanup(proxy.Close)
	cred := storage.Credentials{AccessKeyID: testMinioUser, SecretAccessKey: testMinioPass}
	in := storage.PresignInput{Endpoint: proxy.URL, Bucket: "abastevo-dev-photos", Key: "q/rollout-fixture", Namespace: "q/", ContentType: "image/jpeg", MaxBytes: 1024, TTL: time.Minute, Region: "us-east-1", Now: time.Now()}
	client := &http.Client{Timeout: 5 * time.Second}
	do := func(method, raw string, body []byte) (int, []byte) {
		r, _ := http.NewRequest(method, raw, bytes.NewReader(body))
		r.Header.Set("Content-Type", "image/jpeg")
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, data
	}
	put, err := storage.PresignPUT(in, cred)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := do("PUT", put.URL, []byte("synthetic non-photo fixture")); status != 200 {
		t.Fatalf("signed PUT: %d", status)
	}
	del, err := storage.PresignDELETE(in, cred)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { do("DELETE", del.URL, nil) })
	get, err := storage.PresignGET(in, cred)
	if err != nil {
		t.Fatal(err)
	}
	if status, body := do("GET", get.URL, nil); status != 200 || string(body) != "synthetic non-photo fixture" {
		t.Fatalf("signed GET: %d", status)
	}
	if status, _ := do("GET", proxy.URL+"/abastevo-dev-photos/"+in.Key, nil); status != 403 {
		t.Fatalf("anonymous read: %d", status)
	}
	bad, _ := url.Parse(get.URL)
	q := bad.Query()
	q.Set("X-Amz-Signature", "invalid")
	bad.RawQuery = q.Encode()
	if status, _ := do("GET", bad.String(), nil); status != 403 {
		t.Fatalf("forged signature: %d", status)
	}
}

const testMinioImage = "bitnamilegacy/minio@sha256:451fe6858cb770cc9d0e77ba811ce287420f781c7c1b806a386f6896471a349c"
const testMCImage = "bitnamilegacy/minio-client@sha256:00dcc4e58ada0df45bb7d9ee435af98295f96c27c3c68292ce78ec700a87b511"

const (
	testMinioUser = "testadmin"
	testMinioPass = "testadmin123456"
)

// startPrivateProxyS3 boots a disposable loopback MinIO with one bucket and
// returns its endpoint. Callers stop it via cleanup. Small synthetic
// payloads only; the anonymous volume dies with the container.
func startPrivateProxyS3(t *testing.T, bucket string) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("docker unavailable for the S3-compatible matrix")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	var startupStderr bytes.Buffer
	start := exec.CommandContext(ctx, "docker", "run", "-d", "--rm",
		"-p", "127.0.0.1::9000",
		"--memory", "512m", "--cpus", "2",
		"--tmpfs", "/data:rw,size=64m,mode=1777",
		"--entrypoint", "/opt/bitnami/minio/bin/minio",
		"-e", "MINIO_ROOT_USER="+testMinioUser,
		"-e", "MINIO_ROOT_PASSWORD="+testMinioPass,
		testMinioImage, "server", "/data")
	start.Stderr = &startupStderr
	out, err := start.Output()
	if err != nil {
		cancel()
		t.Fatalf("start minio: %v (%s)", err, startupStderr.String())
	}
	id := strings.TrimSpace(string(out))
	cleanup := func() {
		defer cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = exec.CommandContext(stopCtx, "docker", "stop", id).Run()
	}
	t.Cleanup(cleanup)
	portOut, err := exec.CommandContext(ctx, "docker", "port", id, "9000").CombinedOutput()
	if err != nil {
		t.Fatalf("minio port: %v (%s)", err, portOut)
	}
	port := strings.TrimSpace(string(portOut))
	if i := strings.LastIndex(port, ":"); i >= 0 {
		port = port[i+1:]
	}
	endpoint := "http://127.0.0.1:" + port
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint + "/")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	mk := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "host",
		"--entrypoint", "sh", testMCImage, "-c",
		fmt.Sprintf("mc alias set local %s %s %s && mc mb local/%s",
			endpoint, testMinioUser, testMinioPass, bucket))
	if out, err := mk.CombinedOutput(); err != nil {
		t.Fatalf("make bucket: %v (%s)", err, out)
	}
	return endpoint
}
