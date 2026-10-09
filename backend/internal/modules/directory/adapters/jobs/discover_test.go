package jobs

// RST-08 discover-job unit tests: payload gating, terminal outcomes,
// bounded provider retries and conditional skip on complete snapshots.
// Every vector is synthetic with owned [RST08-TEST] markers; the
// loopback provider never leaves the test process.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// discoverFakeStore replays snapshots like the unique key does: the
// second staging of one snapshot converges instead of duplicating.
type discoverFakeStore struct {
	runs       map[string]registry.Report
	assertions int
}

func newDiscoverFakeStore() *discoverFakeStore {
	return &discoverFakeStore{runs: map[string]registry.Report{}}
}

func (f *discoverFakeStore) CreateRun(_ context.Context, id, _, snapshot, _ string) (string, bool, error) {
	if _, ok := f.runs[snapshot]; ok {
		return "", false, nil
	}
	f.runs[snapshot] = registry.Report{RunID: id, State: "running"}
	return id, true, nil
}

func (f *discoverFakeStore) GetRun(_ context.Context, _, snapshot string) (registry.Report, error) {
	return f.runs[snapshot], nil
}

func (f *discoverFakeStore) FinishRun(_ context.Context, runID, state string, accepted, duplicates, rejected int64, errorCode string) error {
	for snapshot, report := range f.runs {
		if report.RunID == runID {
			report.State, report.Accepted, report.Duplicates, report.Rejected, report.ErrorCode =
				state, accepted, duplicates, rejected, errorCode
			f.runs[snapshot] = report
		}
	}
	return nil
}

func (f *discoverFakeStore) StageAssertion(_ context.Context, _ registry.Assertion) (bool, error) {
	f.assertions++
	return true, nil
}

func (f *discoverFakeStore) ListAssertions(_ context.Context, _ string) ([]registry.Assertion, error) {
	return nil, nil
}

func (f *discoverFakeStore) GetAssertion(_ context.Context, _ string) (registry.Assertion, string, error) {
	return registry.Assertion{}, "", fmt.Errorf("discover fake: no assertions by id")
}

func (f *discoverFakeStore) SetAssertionStation(_ context.Context, _, _ string) error { return nil }

func (f *discoverFakeStore) SetAssertionSuperseded(_ context.Context, _, _ string) error {
	return nil
}

func loopbackConfig(server *httptest.Server) registry.APIConfig {
	return registry.APIConfig{
		BaseURL:       server.URL,
		AllowedHost:   "127.0.0.1",
		AllowLoopback: true,
		PageSize:      100,
		MaxPages:      5,
		MaxRequests:   10,
	}
}

func discoverJob(snapshot string) jobs.Job {
	payload, err := EncodeDiscoverPayload(snapshot, "", "", "")
	if err != nil {
		panic(err)
	}
	return jobs.Job{Payload: payload}
}

const discoverPage = `{"items": [{"cnpj": "04218406000104", "razaoSocial": "[RST08-TEST] POSTO JOTA LTDA", "nomeFantasia": "Jota", "endereco": {"municipio": "SAO PAULO", "uf": "SP", "codigoIbge": "3550308"}, "situacao": "ATIVA", "location_quality": "unknown", "coordenadas": null}], "page": 1, "pageSize": 100, "nextCursor": null}`

func TestDiscoverJobCompletesBoundedSnapshot(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(discoverPage))
	}))
	defer server.Close()

	store := newDiscoverFakeStore()
	handler := Discover{Store: store, Config: loopbackConfig(server)}
	if err := handler.Handle(context.Background(), discoverJob("api:test-1")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if store.runs["api:test-1"].State != "complete" {
		t.Fatalf("run = %+v", store.runs["api:test-1"])
	}
	if store.assertions != 1 {
		t.Fatalf("assertions = %d, want 1", store.assertions)
	}

	// Replay of the complete snapshot skips the network entirely.
	if err := handler.Handle(context.Background(), discoverJob("api:test-1")); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if calls != 1 {
		t.Fatalf("requests = %d, want 1 (conditional skip)", calls)
	}
}

func TestDiscoverJobBoundsProviderRetries(t *testing.T) {
	for name, status := range map[string]int{"429": http.StatusTooManyRequests, "500": http.StatusInternalServerError} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(status)
			}))
			defer server.Close()

			store := newDiscoverFakeStore()
			handler := Discover{Store: store, Config: loopbackConfig(server)}
			if err := handler.Handle(context.Background(), discoverJob("api:test-retry")); err == nil {
				t.Fatal("sick provider succeeded, want handler error for backoff")
			}
			if calls != 3 {
				t.Fatalf("requests = %d, want exactly the 3-attempt retry budget", calls)
			}
			if store.runs["api:test-retry"].State != "failed" {
				t.Fatalf("run = %+v, want failed", store.runs["api:test-retry"])
			}
		})
	}
}

func TestDiscoverJobQuarantineIsTerminal(t *testing.T) {
	pages := []string{
		`{"items": [], "nextCursor": "p2"}`,
		`{"items": [], "nextCursor": null}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(pages[0]))
			return
		}
		_, _ = w.Write([]byte(pages[1]))
	}))
	defer server.Close()

	store := newDiscoverFakeStore()
	cfg := loopbackConfig(server)
	cfg.MaxPages = 1
	handler := Discover{Store: store, Config: cfg}
	if err := handler.Handle(context.Background(), discoverJob("api:test-quota")); err != nil {
		t.Fatalf("quota handle: %v", err)
	}
	if store.runs["api:test-quota"].State != "quarantined" {
		t.Fatalf("run = %+v, want quarantined terminal state", store.runs["api:test-quota"])
	}
}

func TestDiscoverJobRejectsBadEnvelope(t *testing.T) {
	handler := Discover{Store: newDiscoverFakeStore(), Config: registry.APIConfig{}}
	for name, job := range map[string]jobs.Job{
		"garbage": {Payload: []byte("{nope")},
		"version": {Payload: []byte(`{"version": 9, "snapshot": "x"}`)},
		"no snap": {Payload: []byte(`{"version": 1}`)},
	} {
		store := newDiscoverFakeStore()
		handler.Store = store
		if err := handler.Handle(context.Background(), job); err == nil {
			t.Fatalf("%s accepted, want rejection before any run", name)
		}
		if len(store.runs) != 0 {
			t.Fatalf("%s created %d runs, want zero", name, len(store.runs))
		}
	}
	if !strings.HasPrefix(DiscoverDedupeKey("s"), "registry-discover:") {
		t.Fatal("dedupe key must namespace the snapshot")
	}
}

func TestDiscoverJobRequiresStoreAndTarget(t *testing.T) {
	handler := Discover{Store: newDiscoverFakeStore(), Config: registry.APIConfig{}}
	handler.Store = nil
	if err := handler.Handle(context.Background(), discoverJob("api:test-nostore")); err == nil {
		t.Fatal("storeless handler accepted, want rejection")
	}
	handler.Store = newDiscoverFakeStore()
	handler.Config = registry.APIConfig{BaseURL: "https://example.com", AllowedHost: "example.com"}
	if err := handler.Handle(context.Background(), discoverJob("api:test-badtarget")); err == nil {
		t.Fatal("unreachable target succeeded, want transport failure")
	} else if handler.Store.(*discoverFakeStore).runs["api:test-badtarget"].State != "failed" {
		t.Fatal("transport failure must finish the run as failed, never complete")
	}
}
