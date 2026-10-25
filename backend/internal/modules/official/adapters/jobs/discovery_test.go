package jobs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type discoveryServer struct {
	mu       sync.Mutex
	bodies   map[string]string
	statuses map[string]int
	requests []string
}

func newDiscoveryServer(t *testing.T, bodies map[string]string, statuses map[string]int) (*discoveryServer, *httptest.Server) {
	t.Helper()
	s := &discoveryServer{bodies: bodies, statuses: statuses}
	srv := httptest.NewTLSServer(s)
	t.Cleanup(srv.Close)
	return s, srv
}

func (s *discoveryServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.URL.Path)
	if code, ok := s.statuses[r.URL.Path]; ok {
		w.WriteHeader(code)
		return
	}
	body, ok := s.bodies[r.URL.Path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// A deliberately unreliable validator: content changes while the ETag
	// stays fixed, so only the content hash can detect the correction.
	w.Header().Set("ETag", `"fixed"`)
	_, _ = w.Write([]byte(body))
}

func (s *discoveryServer) set(path, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bodies == nil {
		s.bodies = map[string]string{}
	}
	s.bodies[path] = body
}

func (s *discoveryServer) requested(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, got := range s.requests {
		if got == path {
			return true
		}
	}
	return false
}

func discoveryFetcher(t *testing.T, raw string) *source.Fetcher {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	allow := []source.Entry{
		{ID: "detail", Host: u.Hostname(), PathPrefix: "/arquivos-lpc/", Kind: source.KindStationDetail, FileGlob: "revendas_lpc_*.xlsx", Ports: []int{port}},
		{ID: "summary", Host: u.Hostname(), PathPrefix: "/arquivos-lpc/", Kind: source.KindWeeklySummary, FileGlob: "resumo_semanal_lpc_*.xlsx", Ports: []int{port}},
	}
	f := source.NewFetcher(allow)
	f.Timeout = 5 * time.Second
	f.InsecureTLS = true
	f.IPAllow = func(netip.Addr) bool { return true }
	return &f
}

func runDiscovery(t *testing.T, now time.Time, srv *httptest.Server) ([]ImportPayload, []string) {
	t.Helper()
	return runDiscoveryPayload(t, now, srv, "")
}

func runDiscoveryPayload(t *testing.T, now time.Time, srv *httptest.Server, payload string) ([]ImportPayload, []string) {
	t.Helper()
	var payloads []ImportPayload
	var keys []string
	d := Discovery{
		Fetch:       discoveryFetcher(t, srv.URL),
		ListingBase: srv.URL + "/arquivos-lpc/",
		Enqueue: func(_ context.Context, p ImportPayload, key string) error {
			payloads = append(payloads, p)
			keys = append(keys, key)
			return nil
		},
		Now: func() time.Time { return now },
	}
	job := jobs.Job{}
	if payload != "" {
		job.Payload = []byte(payload)
	}
	if err := d.Handle(context.Background(), job); err != nil {
		t.Fatalf("discover: %v", err)
	}
	return payloads, keys
}

func TestWeekFileNames(t *testing.T) {
	got := weekFileNames(detailPattern, "2026-10-11", "2026-10-17")
	want := []string{
		"revendas_lpc_2026_10_11-2026_10_17.xlsx",
		"revendas_lpc_2026-10-11_2026-10-17.xlsx",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("names = %v, want %v", got, want)
	}
}

func TestDiscoveryAcceptsCurrentFilenameVariant(t *testing.T) {
	const base = "/arquivos-lpc/2026/"
	detail := base + "revendas_lpc_2026_10_11-2026_10_17.xlsx"
	summary := base + "resumo_semanal_lpc_2026_10_11-2026_10_17.xlsx"
	server, srv := newDiscoveryServer(t, map[string]string{detail: "detail", summary: "summary"}, nil)
	payloads, _ := runDiscovery(t, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), srv)

	if len(payloads) != 2 {
		t.Fatalf("enqueued %d, want detail plus summary", len(payloads))
	}
	urls := map[string]bool{}
	for _, p := range payloads {
		urls[p.SourceURL] = true
		if p.SurveyStart != "2026-10-11" || p.SurveyEnd != "2026-10-17" {
			t.Errorf("survey window = %s..%s", p.SurveyStart, p.SurveyEnd)
		}
	}
	if !urls[srv.URL+detail] || !urls[srv.URL+summary] {
		t.Errorf("URLs = %v", urls)
	}
	legacy := base + "revendas_lpc_2026-10-11_2026-10-17.xlsx"
	if server.requested(legacy) {
		t.Error("legacy variant probed after the current variant succeeded")
	}
}

func TestDiscoveryContentHashDedupeAndCorrection(t *testing.T) {
	path := "/arquivos-lpc/2026/revendas_lpc_2026_10_11-2026_10_17.xlsx"
	server, srv := newDiscoveryServer(t, map[string]string{path: "v1"}, nil)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

	_, first := runDiscovery(t, now, srv)
	_, second := runDiscovery(t, now, srv)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("keys = %v / %v, want one detail job per run", first, second)
	}
	if first[0] != second[0] {
		t.Errorf("unchanged content keys differ: %q vs %q", first[0], second[0])
	}
	if !strings.Contains(first[0], ":sha256:") {
		t.Errorf("key = %q, want content hash", first[0])
	}

	server.set(path, "v2")
	_, corrected := runDiscovery(t, now, srv)
	if len(corrected) != 1 {
		t.Fatalf("corrected keys = %v, want one job", corrected)
	}
	if corrected[0] == first[0] {
		t.Errorf("replaced content reused key %q", corrected[0])
	}
}

func TestDiscoveryBoundsRecentCorrections(t *testing.T) {
	oldest := "/arquivos-lpc/2026/revendas_lpc_2026_09_20-2026_09_26.xlsx"
	tooOld := "/arquivos-lpc/2026/revendas_lpc_2026_09_13-2026_09_19.xlsx"
	server, srv := newDiscoveryServer(t, map[string]string{
		oldest: "oldest",
		tooOld: "too-old",
	}, nil)
	payloads, _ := runDiscoveryPayload(t, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), srv, `{"weeks":5}`)

	if len(payloads) != 1 {
		t.Fatalf("enqueued %d, want only the fifth recent week", len(payloads))
	}
	if !strings.HasSuffix(payloads[0].SourceURL, oldest) {
		t.Errorf("URL = %s, want %s", payloads[0].SourceURL, oldest)
	}
	if server.requested(tooOld) {
		t.Error("probed a week outside the bounded correction window")
	}
}

func TestDiscoverySkipsUnpublishedAndRetriesOtherErrors(t *testing.T) {
	t.Run("404 is not an error", func(t *testing.T) {
		_, srv := newDiscoveryServer(t, nil, nil)
		payloads, _ := runDiscovery(t, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), srv)
		if len(payloads) != 0 {
			t.Fatalf("enqueued %d unpublished files", len(payloads))
		}
	})

	t.Run("429 fails for retry", func(t *testing.T) {
		path := "/arquivos-lpc/2026/revendas_lpc_2026_10_18-2026_10_24.xlsx"
		_, srv := newDiscoveryServer(t, nil, map[string]int{path: http.StatusTooManyRequests})
		d := Discovery{
			Fetch:       discoveryFetcher(t, srv.URL),
			ListingBase: srv.URL + "/arquivos-lpc/",
			Enqueue: func(context.Context, ImportPayload, string) error {
				t.Error("enqueued behind a rate limit")
				return nil
			},
			Now: func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) },
		}
		err := d.Handle(context.Background(), jobs.Job{})
		if !errors.Is(err, source.ErrBadStatus) {
			t.Fatalf("error = %v, want retryable ErrBadStatus", err)
		}
	})
}

func TestDiscoveryClampsCorrectionWindow(t *testing.T) {
	tooOld := "/arquivos-lpc/2026/revendas_lpc_2026_09_13-2026_09_19.xlsx"
	server, srv := newDiscoveryServer(t, map[string]string{tooOld: "too-old"}, nil)
	payloads, _ := runDiscoveryPayload(t, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), srv, `{"weeks":99}`)
	if len(payloads) != 0 {
		t.Fatalf("enqueued %d outside the bounded window", len(payloads))
	}
	if server.requested(tooOld) {
		t.Error("requested a week beyond the five-week clamp")
	}
}

func TestDiscoveryRejectsBadPayload(t *testing.T) {
	_, srv := newDiscoveryServer(t, nil, nil)
	d := Discovery{
		Fetch:       discoveryFetcher(t, srv.URL),
		ListingBase: srv.URL + "/arquivos-lpc/",
		Enqueue:     func(context.Context, ImportPayload, string) error { return nil },
	}
	if err := d.Handle(context.Background(), jobs.Job{Payload: []byte(`not-json`)}); err == nil {
		t.Fatal("bad payload accepted")
	}
}
