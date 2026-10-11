//go:build integration

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/go-chi/chi/v5"
)

// Pinned Caddy image, identical to infra/compose.staging.yml.
const testCaddyImage = "caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d"

// SEC02-C local edge proof (B-BR-015, AUD-03): a real Caddy edge with
// the production header_up contract forwards the observed client
// address; the API bills that address when the edge peer is trusted
// and ignores forged headers on direct paths. Unit matrices prove
// two-client independence; this test proves the real header flow.
func TestLocalCaddyForwardsObservedClient(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable for the local edge proof")
	}
	var mu sync.Mutex
	var subjects []string
	proxies, err := ParseTrustedProxies("127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	h := Handler{
		Registrar:      noRegistrar{},
		Authority:      "test.invalid",
		QuotaSecret:    []byte(strings.Repeat("a", 32)),
		TrustedProxies: proxies,
		CheckQuota: func(_ context.Context, subject, _ string) (time.Duration, error) {
			mu.Lock()
			defer mu.Unlock()
			subjects = append(subjects, subject)
			return 0, nil
		},
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	api := httptest.NewServer(r)
	defer api.Close()
	apiPort := strings.TrimPrefix(api.URL, "http://127.0.0.1:")

	dir := t.TempDir()
	caddyfile := "{\n\tadmin off\n}\n\n:18081 {\n" +
		"\treverse_proxy 127.0.0.1:" + apiPort + " {\n" +
		"\t\ttrusted_proxies private_ranges\n" +
		"\t\theader_up Host {host}\n" +
		"\t\theader_up X-Real-IP {remote_host}\n" +
		"\t\theader_up X-Forwarded-For {remote_host}\n" +
		"\t\theader_up X-Forwarded-Proto {scheme}\n" +
		"\t}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "Caddyfile"), []byte(caddyfile), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edge := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "host",
		"-v", dir+":/etc/caddy:ro", testCaddyImage)
	edge.Stdout = os.Stderr
	edge.Stderr = os.Stderr
	if err := edge.Start(); err != nil {
		t.Fatalf("start caddy edge: %v", err)
	}
	defer func() {
		_ = edge.Process.Kill()
		_ = edge.Wait()
	}()

	fp := "fp:" + strings.Repeat("a", 64)
	post := func(url, xff string) int {
		t.Helper()
		var code int
		for i := 0; i < 50; i++ {
			req, _ := http.NewRequestWithContext(ctx, "POST", url+"/v1/identity/challenges",
				strings.NewReader(`{"fingerprint":"`+fp+`","purpose":"REGISTER"}`))
			req.Header.Set("Content-Type", "application/json")
			if xff != "" {
				req.Header.Set("X-Forwarded-For", xff)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				time.Sleep(300 * time.Millisecond)
				continue
			}
			code = resp.StatusCode
			resp.Body.Close()
			break
		}
		return code
	}
	// Through the edge with a forged header: Caddy replaces the chain
	// with the observed host (127.0.0.1 under host networking), and
	// the API bills exactly that address — never the forgery, never
	// the edge container as an opaque shared identity without a
	// forwarding address.
	if code := post("http://127.0.0.1:18081", "198.51.100.7"); code != 503 {
		t.Fatalf("edge challenge status = %d, want 503 (registrar offline, quota admitted)", code)
	}
	mu.Lock()
	if len(subjects) != 2 {
		mu.Unlock()
		t.Fatalf("edge subjects = %d, want 2 (ip + fingerprint)", len(subjects))
	}
	var ipSubject string
	for _, s := range subjects {
		if strings.HasPrefix(s, "ip:") {
			ipSubject = s
		}
	}
	mu.Unlock()
	if ipSubject == "" {
		t.Fatal("edge billed no IP subject")
	}
	day := time.Now().UTC().Format("2006-01-02")
	want, err := domain.HashIPSubject(day, []byte(strings.Repeat("a", 32)), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if ipSubject != want {
		t.Fatalf("edge billed %q, want the observed loopback subject", ipSubject)
	}
	// Direct to the API with the same forgery: the untrusted peer
	// bills itself (spoof ignored end-to-end).
	mu.Lock()
	subjects = nil
	mu.Unlock()
	if code := post(api.URL, "198.51.100.7"); code != 503 {
		t.Fatalf("direct challenge status = %d, want 503", code)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, s := range subjects {
		if strings.HasPrefix(s, "ip:") && s == want {
			t.Fatalf("direct path billed the loopback subject %q (header spoof accepted)", s)
		}
	}
}
