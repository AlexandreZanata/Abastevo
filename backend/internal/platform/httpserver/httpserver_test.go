package httpserver

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

func testOptions(logBuf *bytes.Buffer) Options {
	logger, err := telemetry.New(logBuf, "info", telemetry.RoleAPI)
	if err != nil {
		panic(err)
	}
	return Options{
		Addr:              "127.0.0.1:0",
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   3 * time.Second,
		MaxBodyBytes:      64 * 1024,
		Logger:            logger,
	}
}

func startTestServer(t *testing.T, opts Options, routes func(r Router)) *Server {
	t.Helper()
	router := NewRouter(opts.Logger)
	if routes != nil {
		routes(router)
	}
	srv, err := New(opts, router)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run returned: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("server did not stop after cancel")
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", srv.Addr().String(), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return srv
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never became reachable: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestServesRouteAndPropagatesRequestID(t *testing.T) {
	var logs bytes.Buffer
	srv := startTestServer(t, testOptions(&logs), func(r Router) {
		r.Get("/ping", func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("pong"))
		})
	})
	base := "http://" + srv.Addr().String()

	req, _ := http.NewRequest(http.MethodGet, base+"/ping", nil)
	req.Header.Set("X-Request-ID", "req-abc")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "pong" {
		t.Fatalf("status = %d, body = %q", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Request-ID") != "req-abc" {
		t.Errorf("X-Request-ID = %q, want passthrough req-abc", resp.Header.Get("X-Request-ID"))
	}

	resp2, err := http.Get(base + "/ping")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	resp2.Body.Close()
	if resp2.Header.Get("X-Request-ID") == "" {
		t.Error("server must assign X-Request-ID when the client sends none")
	}
	if !strings.Contains(logs.String(), `"request_id":"req-abc"`) {
		t.Errorf("access log must carry the request id:\n%s", logs.String())
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	var logs bytes.Buffer
	echo := func(r Router) {
		r.Post("/echo", func(w http.ResponseWriter, req *http.Request) {
			if _, err := io.ReadAll(req.Body); err != nil {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	}
	srv := startTestServer(t, testOptions(&logs), echo)
	post := func(addr string) int {
		resp, err := http.Post("http://"+addr+"/echo", "text/plain",
			strings.NewReader(strings.Repeat("x", 64*1024+1)))
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(srv.Addr().String()); got != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", got)
	}

	// Same body passes when the cap allows it: the middleware cap rejects.
	var wideLogs bytes.Buffer
	wide := testOptions(&wideLogs)
	wide.MaxBodyBytes = 1 << 20
	wideSrv := startTestServer(t, wide, echo)
	if got := post(wideSrv.Addr().String()); got != http.StatusOK {
		t.Errorf("status with wide cap = %d, want 200", got)
	}
}

func TestSlowHeadersTimedOut(t *testing.T) {
	var logs bytes.Buffer
	opts := testOptions(&logs)
	opts.ReadHeaderTimeout = 100 * time.Millisecond
	opts.ReadTimeout = 100 * time.Millisecond
	srv := startTestServer(t, opts, nil)

	conn, err := net.DialTimeout("tcp", srv.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("GET /ping HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Never complete the headers; the server must give up on its own.
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Error("expected the server to close the slow connection, it stayed open")
	}
}

func TestGracefulDrainCompletesInFlight(t *testing.T) {
	var logs bytes.Buffer
	opts := testOptions(&logs)
	router := NewRouter(opts.Logger)
	entered := make(chan struct{})
	release := make(chan struct{})
	router.Get("/slow", func(w http.ResponseWriter, req *http.Request) {
		close(entered)
		<-release
		w.Write([]byte("done"))
	})
	srv, err := New(opts, router)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	respCh := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get("http://" + srv.Addr().String() + "/slow")
		if err != nil {
			return
		}
		respCh <- resp
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never entered")
	}
	cancel() // shutdown while the request is in flight; drain must wait.
	close(release)

	select {
	case resp := <-respCh:
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body) != "done" {
			t.Errorf("status = %d, body = %q, want 200 done", resp.StatusCode, body)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("in-flight request was not drained")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("server did not stop after drain")
	}
}

func TestAccessLogCarriesNoSensitiveData(t *testing.T) {
	var logs bytes.Buffer
	srv := startTestServer(t, testOptions(&logs), func(r Router) {
		r.Get("/stations/{id}", func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("ok"))
		})
	})
	secret := "tok-live-9f2secret"
	url := "http://" + srv.Addr().String() + "/stations/abc-123?lat=-23.5558&lon=-46.6396&token=" + secret
	req, _ := http.NewRequest(http.MethodGet, url, strings.NewReader(`{"price":599}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	out := logs.String()
	for _, leak := range []string{secret, "-23.5558", "-46.6396", `{"price":599}`, "abc-123"} {
		if strings.Contains(out, leak) {
			t.Errorf("access log leaks %q:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "/stations/{id}") {
		t.Errorf("access log must use the route template:\n%s", out)
	}
}

func TestNotFoundKeepsRequestIDAndAccessLog(t *testing.T) {
	var logs bytes.Buffer
	srv := startTestServer(t, testOptions(&logs), nil)
	resp, err := http.Get("http://" + srv.Addr().String() + "/undefined")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("unmatched routes must still carry X-Request-ID")
	}
	out := logs.String()
	if !strings.Contains(out, `"route":"unmatched"`) || !strings.Contains(out, `"status":404`) {
		t.Errorf("unmatched probe must be access-logged:\n%s", out)
	}
}

func TestNewRejectsBadOptions(t *testing.T) {
	var buf bytes.Buffer
	good := testOptions(&buf)
	router := NewRouter(good.Logger)
	cases := map[string]func(*Options){
		"empty addr":      func(o *Options) { o.Addr = "" },
		"negative read":   func(o *Options) { o.ReadTimeout = -time.Second },
		"zero header":     func(o *Options) { o.ReadHeaderTimeout = 0 },
		"negative write":  func(o *Options) { o.WriteTimeout = -time.Second },
		"zero shutdown":   func(o *Options) { o.ShutdownTimeout = 0 },
		"zero body cap":   func(o *Options) { o.MaxBodyBytes = 0 },
		"nil logger":      func(o *Options) { o.Logger = nil },
		"unusable listen": func(o *Options) { o.Addr = "256.256.256.256:1" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			opts := good
			mutate(&opts)
			if _, err := New(opts, router); err == nil {
				t.Errorf("expected error for %s, got nil", name)
			}
		})
	}
	if _, err := New(good, nil); err == nil {
		t.Error("expected error for nil handler, got nil")
	}
}
