package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loopbackAllowlist builds entries pointing at a TLS test server. Loopback
// is unreachable under the default policy, so tests pass an explicit
// permissive policy and prove the strict default separately.
func loopbackAllowlist(t *testing.T, serverURL, prefix string) []Entry {
	t.Helper()
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	port := 0
	if p := u.Port(); p != "" {
		if _, err := fmt.Sscanf(p, "%d", &port); err != nil {
			t.Fatal(err)
		}
	}
	return []Entry{
		{ID: "t-listing", Host: u.Hostname(), PathPrefix: prefix, Kind: KindListing, Ports: []int{port}},
		{ID: "t-detail", Host: u.Hostname(), PathPrefix: prefix, Kind: KindStationDetail, FileGlob: "*.xlsx", Ports: []int{port}},
	}
}

func testFetcher(allow []Entry, dir string) Fetcher {
	f := NewFetcher(allow)
	f.TempDir = dir
	f.Timeout = 5 * time.Second
	f.InsecureTLS = true
	f.IPAllow = func(netip.Addr) bool { return true }
	return f
}

func tempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func dirEmpty(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// os.ReadDir lists; t.TempDir starts empty, residuals prove leaks.
	return len(entries) == 0
}

func TestFetchChecksumAndCleanup(t *testing.T) {
	body := "fake-workbook-bytes"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	dir := tempDir(t)
	f := testFetcher(loopbackAllowlist(t, srv.URL, "/anp/"), dir)
	res, err := f.Fetch(context.Background(), srv.URL+"/anp/file.xlsx", "", "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	sum := sha256.Sum256([]byte(body))
	if res.SHA256 != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Errorf("checksum = %q", res.SHA256)
	}
	if res.Bytes != int64(len(body)) || res.ETag != `"v1"` {
		t.Errorf("result = %+v", res)
	}
	stored, err := os.ReadFile(res.Path)
	if err != nil || string(stored) != body {
		t.Errorf("stored bytes mismatch: %v", err)
	}
	if err := res.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(res.Path); !os.IsNotExist(err) {
		t.Error("temp file survives Close")
	}
	if err := res.Close(); err != nil {
		t.Errorf("double close: %v", err)
	}
	if !dirEmpty(t, dir) {
		t.Error("temp dir has residuals")
	}
}

func TestConditionalNotModified(t *testing.T) {
	var gotMatch, gotSince string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMatch, gotSince = r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")
		if r.Header.Get("If-None-Match") == `"v9"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte("new"))
	}))
	defer srv.Close()
	dir := tempDir(t)
	f := testFetcher(loopbackAllowlist(t, srv.URL, "/anp/"), dir)
	res, err := f.Fetch(context.Background(), srv.URL+"/anp/file.xlsx", `"v9"`, "Mon, 02 Jan 2006 15:04:05 GMT")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !res.NotModified || res.Path != "" {
		t.Errorf("expected clean 304, got %+v", res)
	}
	if gotMatch != `"v9"` || gotSince == "" {
		t.Errorf("conditional headers not sent: %q %q", gotMatch, gotSince)
	}
	if !dirEmpty(t, dir) {
		t.Error("304 left a file behind")
	}
}

func TestErrorStatusLeavesNothing(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	dir := tempDir(t)
	f := testFetcher(loopbackAllowlist(t, srv.URL, "/anp/"), dir)
	if _, err := f.Fetch(context.Background(), srv.URL+"/anp/file.xlsx", "", ""); !errors.Is(err, ErrBadStatus) {
		t.Errorf("want ErrBadStatus, got %v", err)
	}
	if !dirEmpty(t, dir) {
		t.Error("error left a file behind")
	}
}

func TestSizeLimits(t *testing.T) {
	big := strings.Repeat("x", 3000)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/anp/declared" {
			w.Header().Set("Content-Length", "1000000")
			_, _ = w.Write([]byte("small"))
			return
		}
		// Chunked body without length.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()
	dir := tempDir(t)
	f := testFetcher(loopbackAllowlist(t, srv.URL, "/anp/"), dir)
	f.MaxBytes = 1024
	if _, err := f.Fetch(context.Background(), srv.URL+"/anp/declared", "", ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("lying length accepted: %v", err)
	}
	if _, err := f.Fetch(context.Background(), srv.URL+"/anp/stream", "", ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("streaming overflow accepted: %v", err)
	}
	if !dirEmpty(t, dir) {
		t.Error("oversize left a file behind")
	}
}

func TestTimeoutAndCancelCleanup(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/anp/slow" {
			time.Sleep(2 * time.Second)
		}
		fl, _ := w.(http.Flusher)
		for i := 0; i < 50; i++ {
			_, _ = w.Write([]byte(strings.Repeat("y", 8192)))
			if fl != nil {
				fl.Flush()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()
	dir := tempDir(t)
	f := testFetcher(loopbackAllowlist(t, srv.URL, "/anp/"), dir)
	f.Timeout = 300 * time.Millisecond
	if _, err := f.Fetch(context.Background(), srv.URL+"/anp/slow", "", ""); err == nil {
		t.Error("slow server accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := f.Fetch(ctx, srv.URL+"/anp/drip", "", "")
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	if err := <-done; err == nil {
		t.Error("cancelled fetch accepted")
	}
	// Give the deferred remover a beat, then assert no residuals.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if dirEmpty(t, dir) {
			break
		}
		if time.Now().After(deadline) {
			t.Error("timeout/cancel left files behind")
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRedirectPolicy(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("arrived"))
	}))
	defer target.Close()
	src := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/anp/ok":
			http.Redirect(w, r, target.URL+"/anp/file.xlsx", http.StatusFound)
		case "/anp/downgrade":
			http.Redirect(w, r, "http://example.com/x", http.StatusFound)
		case "/anp/loop":
			http.Redirect(w, r, "/anp/loop", http.StatusFound)
		}
	}))
	defer src.Close()
	dir := tempDir(t)
	su, _ := url.Parse(src.URL)
	tu, _ := url.Parse(target.URL)
	var sp, tp int
	_, _ = fmt.Sscanf(su.Port(), "%d", &sp)
	_, _ = fmt.Sscanf(tu.Port(), "%d", &tp)
	allow := []Entry{
		{ID: "t", Host: su.Hostname(), PathPrefix: "/anp/", Kind: KindListing, Ports: []int{sp}},
		{ID: "t2", Host: tu.Hostname(), PathPrefix: "/anp/", Kind: KindStationDetail, FileGlob: "*.xlsx", Ports: []int{tp}},
	}
	f := testFetcher(allow, dir)
	res, err := f.Fetch(context.Background(), src.URL+"/anp/ok", "", "")
	if err != nil {
		t.Fatalf("allowlisted redirect refused: %v", err)
	}
	_ = res.Close()
	if _, err := f.Fetch(context.Background(), src.URL+"/anp/downgrade", "", ""); !errors.Is(err, ErrRedirectTarget) {
		t.Errorf("https downgrade accepted: %v", err)
	}
	f.MaxRedirects = 2
	if _, err := f.Fetch(context.Background(), src.URL+"/anp/loop", "", ""); !errors.Is(err, ErrRedirectLimit) {
		t.Errorf("redirect loop accepted: %v", err)
	}
}

func TestStrictPolicyRefusesLoopback(t *testing.T) {
	// Default fetcher, no test bypass: loopback must fail at dial time even
	// when the URL itself is allowlisted.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	f := NewFetcher(loopbackAllowlist(t, srv.URL, "/anp/"))
	f.TempDir = tempDir(t)
	f.Timeout = 5 * time.Second
	f.InsecureTLS = true
	if _, err := f.Fetch(context.Background(), srv.URL+"/anp/file.xlsx", "", ""); !errors.Is(err, ErrPrivateTarget) && !errors.Is(err, ErrNoAddress) {
		t.Errorf("loopback dialed under strict policy: %v", err)
	}
}

func TestProbeChangedUnchangedError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("If-None-Match") == `"same"`:
			w.WriteHeader(http.StatusNotModified)
		case r.URL.Path == "/anp/broken":
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.Header().Set("ETag", `"new"`)
			w.Header().Set("Last-Modified", "Tue, 03 Jan 2006 15:04:05 GMT")
			_, _ = w.Write([]byte("listing"))
		}
	}))
	defer srv.Close()
	dir := tempDir(t)
	f := testFetcher(loopbackAllowlist(t, srv.URL, "/anp/"), dir)
	changed, etag, _, err := f.Probe(context.Background(), srv.URL+"/anp/page", `"same"`)
	if err != nil || changed || etag != `"same"` {
		t.Errorf("unchanged probe = %v %q %v", changed, etag, err)
	}
	changed, etag, lastMod, err := f.Probe(context.Background(), srv.URL+"/anp/page", `"old"`)
	if err != nil || !changed || etag != `"new"` || lastMod == "" {
		t.Errorf("changed probe = %v %q %q %v", changed, etag, lastMod, err)
	}
	if _, _, _, err := f.Probe(context.Background(), srv.URL+"/anp/broken", `"old"`); !errors.Is(err, ErrBadStatus) {
		t.Errorf("broken probe accepted: %v", err)
	}
	if changed, _, _, err := f.Probe(context.Background(), srv.URL+"/anp/file.xlsx", `"old"`); err != nil || !changed {
		t.Errorf("file probe = %v, %v", changed, err)
	}
}

func TestResultCloseIdempotent(t *testing.T) {
	dir := tempDir(t)
	p := filepath.Join(dir, "x.tmp")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := Result{Path: p}
	if err := res.Close(); err != nil {
		t.Fatal(err)
	}
	if err := res.Close(); err != nil {
		t.Errorf("double close: %v", err)
	}
	var nilRes *Result
	if err := nilRes.Close(); err != nil {
		t.Errorf("nil close: %v", err)
	}
}

func TestDisallowedURLNeverDials(t *testing.T) {
	// No server involved: refusal happens before any socket.
	f := NewFetcher(DefaultAllowlist())
	f.Timeout = 5 * time.Second
	for _, raw := range []string{
		"http://www.gov.br/anp/x",
		"https://evil.www.gov.br/anp/x",
		"https://www.gov.br.evil.com/anp/x",
	} {
		if _, err := f.Fetch(context.Background(), raw, "", ""); err == nil {
			t.Errorf("dialed %q", raw)
		}
	}
	if got := fmt.Sprint(len(DefaultAllowlist())); got != "4" {
		t.Errorf("allowlist changed size = %s", got)
	}
}
