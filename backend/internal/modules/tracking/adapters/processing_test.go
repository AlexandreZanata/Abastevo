package adapters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const (
	orgA     = "a7500000-0000-4000-8000-000000000001"
	sessionA = "a7500000-0000-4000-8000-000000000002"
	jobA     = "a7500000-0000-4000-8000-000000000003"
)

type fakeProcessor struct {
	calls   int
	scope   kernel.OrgScope
	session string
	lease   app.Lease
	err     error
}

func (p *fakeProcessor) Process(_ context.Context, scope kernel.OrgScope, session string, lease app.Lease) (app.ProcessOutcome, int, error) {
	p.calls++
	p.scope, p.session, p.lease = scope, session, lease
	return app.ProcessWritten, 1, p.err
}

func scopeA(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(orgA)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// B-BR-EMP-461: the job carries references only, its key names the input,
// and the handler runs only its own exact envelope under the job's lease.
func TestProcessJobEnvelopeKeyAndHandler(t *testing.T) {
	s := scopeA(t)
	key := trackingpg.ProcessDedupe(s, sessionA, app.Watermark{Segments: 2, Transitions: 3})
	if key != app.ProcessKind+":"+orgA+":"+sessionA+":2:3" {
		t.Fatal("dedupe key", key)
	}
	for name, w := range map[string]app.Watermark{"negative segments": {Segments: -1, Transitions: 3}, "no transition": {Segments: 1, Transitions: 1}} {
		if trackingpg.ProcessDedupe(s, sessionA, w) != "" {
			t.Fatal("an impossible input has a key", name)
		}
	}
	if trackingpg.ProcessDedupe(s, "x", app.Watermark{Transitions: 2}) != "" || trackingpg.ProcessDedupe(kernel.OrgScope{}, sessionA, app.Watermark{Transitions: 2}) != "" {
		t.Fatal("a malformed reference has a key")
	}
	payload := []byte(`{"version":1,"org_id":"` + orgA + `","session_id":"` + sessionA + `"}`)
	if got, session, err := trackingpg.ParseProcess(payload); err != nil || got != s || session != sessionA {
		t.Fatal("envelope", err)
	}
	for name, raw := range map[string]string{
		"version 2":   `{"version":2,"org_id":"` + orgA + `","session_id":"` + sessionA + `"}`,
		"extra field": `{"version":1,"org_id":"` + orgA + `","session_id":"` + sessionA + `","lat_e7":1}`,
		"no session":  `{"version":1,"org_id":"` + orgA + `"}`,
		"upper id":    `{"version":1,"org_id":"A7500000-0000-4000-8000-000000000001","session_id":"` + sessionA + `"}`,
		"not json":    `[1]`,
	} {
		if _, _, err := trackingpg.ParseProcess([]byte(raw)); err == nil {
			t.Fatal("a malformed envelope was admitted", name)
		}
	}
	p := &fakeProcessor{}
	h := trackingpg.ProcessHandler{Processor: p}
	if h.Kind() != app.ProcessKind || h.Version() != 1 {
		t.Fatal("handler identity")
	}
	job := jobs.Job{ID: jobA, Kind: app.ProcessKind, Payload: payload, DedupeKey: key, LeaseToken: 7}
	if err := h.Handle(t.Context(), job); err != nil || p.calls != 1 || p.scope != s || p.session != sessionA || p.lease != (app.Lease{JobID: jobA, Token: 7}) {
		t.Fatal("handled job", err, p.lease)
	}
	for name, bad := range map[string]jobs.Job{
		"other kind":      {ID: jobA, Kind: "org-export-build", Payload: payload, DedupeKey: key, LeaseToken: 7},
		"no key":          {ID: jobA, Kind: app.ProcessKind, Payload: payload, LeaseToken: 7},
		"other session":   {ID: jobA, Kind: app.ProcessKind, Payload: payload, DedupeKey: trackingpg.ProcessDedupe(s, jobA, app.Watermark{Segments: 2, Transitions: 3}), LeaseToken: 7},
		"bare prefix key": {ID: jobA, Kind: app.ProcessKind, Payload: payload, DedupeKey: app.ProcessKind + ":" + orgA + ":" + sessionA + ":", LeaseToken: 7},
	} {
		p.calls = 0
		if err := h.Handle(t.Context(), bad); err == nil || p.calls != 0 {
			t.Fatal("a foreign or malformed job reached the processor", name)
		}
	}
	p.err = app.ErrStaleLease
	if err := h.Handle(t.Context(), job); !errors.Is(err, jobs.ErrStaleLease) {
		t.Fatal("stale lease mapping", err)
	}
	p.err = errors.New("provider detail with -100000000")
	if err := h.Handle(t.Context(), job); !errors.Is(err, app.ErrUnavailable) || err.Error() != app.ErrUnavailable.Error() {
		t.Fatal("a provider error reached the queue", err)
	}
	if err := (trackingpg.ProcessHandler{}).Handle(t.Context(), job); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an absent processor ran", err)
	}
	if _, _, err := (&trackingpg.ProcessingStore{}).Process(t.Context(), s, sessionA, app.Lease{JobID: jobA, Token: 1}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an unconfigured processor ran", err)
	}
}
