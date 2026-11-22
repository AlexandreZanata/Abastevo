package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestServerPrivatePUTRetainsCommunityPolicyAndConditions(t *testing.T) {
	in := testInput("https://storage.example.invalid")
	in.Namespace = "org/example/doc/example/"
	in.Key = in.Namespace + "snapshot"
	in.ContentType = "application/pdf"
	in.MaxBytes = 10 << 20
	if _, err := PresignPUT(in, testCred()); err == nil {
		t.Fatal("community upload policy widened")
	}
	if _, err := PresignServerPUT(in, testCred()); err != nil {
		t.Fatal(err)
	}
	for _, mime := range []string{"", "text/plain\r\nX-ACL: public", "image/jpeg; x=1"} {
		copy := in
		copy.ContentType = mime
		if _, err := PresignServerPUT(copy, testCred()); err == nil {
			t.Fatal("unsafe type accepted")
		}
	}
	in.MaxBytes = 0
	if _, err := PresignServerPUT(in, testCred()); err == nil {
		t.Fatal("absent bound accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "*" || r.Header.Get("Content-Type") != "image/jpeg" {
			t.Error("missing conditional/type header")
		}
		switch r.URL.Path {
		case "/created":
			w.WriteHeader(200)
		case "/exists":
			w.WriteHeader(412)
		case "/redirect":
			w.Header().Set("Location", "/created")
			w.WriteHeader(307)
		default:
			w.WriteHeader(503)
			_, _ = w.Write([]byte("PRIVATE-REMOTE-BODY"))
		}
	}))
	defer server.Close()
	for _, path := range []string{"/created", "/exists", "/redirect", "/error"} {
		created, err := PutIfAbsent(context.Background(), server.Client(), server.URL+path, "image/jpeg", []byte("synthetic"))
		switch path {
		case "/created":
			if err != nil || !created {
				t.Fatal(created, err)
			}
		case "/exists":
			if err != nil || created {
				t.Fatal(created, err)
			}
		default:
			if err == nil || strings.Contains(err.Error(), "PRIVATE-REMOTE-BODY") {
				t.Fatal("unsafe remote error")
			}
		}
	}
}

func TestServerPrivatePUTNeverSignsCommunityNamespaces(t *testing.T) {
	for _, namespace := range []string{QuarantinePrefix, "f/", "org/", "organization/", ""} {
		in := testInput("https://storage.example.invalid")
		in.Namespace = namespace
		in.Key = namespace + "snapshot"
		in.ContentType = "image/jpeg"
		in.MaxBytes = 1 << 20
		if _, err := PresignServerPUT(in, testCred()); !errors.Is(err, ErrServerNamespace) {
			t.Fatalf("server PUT signed namespace %q: %v", namespace, err)
		}
	}
}

// flakyTransport fails selected attempts with a transport error, optionally
// after the request already reached the server (response lost).
type flakyTransport struct {
	next     http.RoundTripper
	attempts int
	fail     func(attempt int) (failBefore, failAfter bool)
	bodies   []string
}

func (f *flakyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.attempts++
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		f.bodies = append(f.bodies, string(raw))
		r.Body = io.NopCloser(bytes.NewReader(raw))
	}
	before, after := f.fail(f.attempts)
	if before {
		return nil, errors.New("http: server closed idle connection")
	}
	response, err := f.next.RoundTrip(r)
	if after {
		if response != nil {
			_ = response.Body.Close()
		}
		return nil, errors.New("read: connection reset by peer")
	}
	return response, err
}

// A pooled keep-alive connection closed by the provider fails a PUT that Go
// will not replay on its own. The conditional create (If-None-Match: *) is
// safe to send once more: an earlier success answers 412 and is then
// verified by the caller's authenticated read.
func TestPutIfAbsentReplaysOnceAfterTransportFailure(t *testing.T) {
	stored := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "*" {
			t.Error("conditional header missing on replay")
		}
		if _, ok := stored[r.URL.Path]; ok {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		stored[r.URL.Path] = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stale := &flakyTransport{next: server.Client().Transport, fail: func(n int) (bool, bool) { return n == 1, false }}
	created, err := PutIfAbsent(context.Background(), &http.Client{Transport: stale}, server.URL+"/stale", "image/jpeg", []byte("synthetic"))
	if err != nil || !created || stale.attempts != 2 || stored["/stale"] != "synthetic" || stale.bodies[1] != "synthetic" {
		t.Fatalf("stale connection not replayed: created=%v err=%v attempts=%d stored=%q", created, err, stale.attempts, stored["/stale"])
	}

	lost := &flakyTransport{next: server.Client().Transport, fail: func(n int) (bool, bool) { return false, n == 1 }}
	created, err = PutIfAbsent(context.Background(), &http.Client{Transport: lost}, server.URL+"/lost", "image/jpeg", []byte("synthetic"))
	if err != nil || created || lost.attempts != 2 || stored["/lost"] != "synthetic" {
		t.Fatalf("lost response replay must converge on 412: created=%v err=%v attempts=%d", created, err, lost.attempts)
	}

	down := &flakyTransport{next: server.Client().Transport, fail: func(int) (bool, bool) { return true, false }}
	if _, err = PutIfAbsent(context.Background(), &http.Client{Transport: down}, server.URL+"/down", "image/jpeg", []byte("synthetic")); err == nil || down.attempts != 2 {
		t.Fatalf("persistent transport failure must stop after one replay: err=%v attempts=%d", err, down.attempts)
	}
	var status *StatusError
	if !errors.As(err, &status) || status.Status != http.StatusServiceUnavailable {
		t.Fatalf("transport failure classification changed: %v", err)
	}
}

// staleConn is a pooled connection the provider closed while it sat idle and
// the client had not noticed yet: the next request is written, and only then
// does the connection turn out to be dead.
type staleConn struct {
	net.Conn
	stale atomic.Bool
}

func (c *staleConn) Write(p []byte) (int, error) {
	if c.stale.Load() {
		_ = c.Conn.Close()
		return len(p), nil
	}
	return c.Conn.Write(p)
}

// A provider closes its idle keep-alive connections by age, so several pooled
// connections go stale together. One replay is not enough when the replay
// draws the next stale connection: eight concurrent conditional writes failed
// that way on main (orgmedia, "unavailable"). The conditional create must
// reach a fresh connection however many stale ones are pooled, and the marker
// that lets the transport replay it must never be sent to the provider.
func TestPutIfAbsentSurvivesEveryStalePooledConnection(t *testing.T) {
	const pooled = 3
	var mu sync.Mutex
	stored := map[string]string{}
	arrived := make(chan struct{}, pooled)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Warm-up: hold every request so each one needs its own connection.
			arrived <- struct{}{}
			<-release
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if _, sent := r.Header["Idempotency-Key"]; sent {
			t.Error("the replay marker reached the provider")
		}
		if r.Header.Get("If-None-Match") != "*" {
			t.Error("conditional header missing")
		}
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		if _, ok := stored[r.URL.Path]; ok {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		stored[r.URL.Path] = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var conns []*staleConn
	dialer := &net.Dialer{}
	transport := &http.Transport{MaxIdleConnsPerHost: pooled, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		tracked := &staleConn{Conn: conn}
		mu.Lock()
		conns = append(conns, tracked)
		mu.Unlock()
		return tracked, nil
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}

	var warm sync.WaitGroup
	for range pooled {
		warm.Add(1)
		go func() {
			defer warm.Done()
			response, err := client.Get(server.URL + "/warm")
			if err != nil {
				t.Error("warm-up request failed")
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}()
	}
	for range pooled {
		<-arrived
	}
	close(release)
	warm.Wait()

	// Every pooled connection is idle now, and every one of them is stale.
	mu.Lock()
	if len(conns) != pooled {
		mu.Unlock()
		t.Fatalf("warm-up pooled %d connections, want %d", len(conns), pooled)
	}
	for _, conn := range conns {
		conn.stale.Store(true)
	}
	mu.Unlock()

	created, err := PutIfAbsent(context.Background(), client, server.URL+"/object", "image/jpeg", []byte("synthetic"))
	mu.Lock()
	defer mu.Unlock()
	if err != nil || !created || stored["/object"] != "synthetic" {
		t.Fatalf("conditional create lost to stale pooled connections: created=%v err=%v stored=%q", created, err, stored["/object"])
	}
	if len(conns) != pooled+1 {
		t.Fatalf("the create used %d connections, want the %d stale ones and one fresh", len(conns), pooled)
	}
}
