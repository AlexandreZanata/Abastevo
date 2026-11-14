package application_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

// fakeWorkerStore holds one job and fences writes by lease.
type fakeWorkerStore struct {
	mu          sync.Mutex
	events      *events
	claim       app.Claim
	state       domain.State
	lease       string
	failure     domain.FailureCode
	provisional *domain.Object
	published   *domain.Object
	claimErr    error
	rightsErr   error
	publishErr  error
	retire      app.Retirement
	retireErr   error
	deleted     []domain.Object
	stealLease  bool
}

func (s *fakeWorkerStore) Claim(_ context.Context, _ kernel.OrgScope, exportID, lease string, ttl time.Duration) (app.Claim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.add("claim")
	if s.claimErr != nil {
		return app.Claim{}, s.claimErr
	}
	if s.state != domain.Queued {
		if s.state == domain.Running {
			return app.Claim{}, app.ErrBusy
		}
		return app.Claim{}, app.ErrSettled
	}
	if ttl != app.DefaultLeaseTTL || !domain.ValidID(lease) {
		return app.Claim{}, errors.New("unexpected lease")
	}
	s.state, s.lease = domain.Running, lease
	c := s.claim
	c.ExportID, c.Lease, c.Attempts = exportID, lease, s.claim.Attempts+1
	s.claim.Attempts++
	c.Provisional = s.provisional
	return c, nil
}
func (s *fakeWorkerStore) fenced(c app.Claim) error {
	if s.stealLease || s.state != domain.Running || s.lease != c.Lease {
		return app.ErrBusy
	}
	return nil
}
func (s *fakeWorkerStore) CheckRequester(context.Context, kernel.OrgScope, app.Claim) error {
	s.events.add("check")
	return s.rightsErr
}
func (s *fakeWorkerStore) Provisional(_ context.Context, _ kernel.OrgScope, c app.Claim, o domain.Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.add("provisional")
	if err := s.fenced(c); err != nil {
		return err
	}
	s.provisional = &o
	return nil
}
func (s *fakeWorkerStore) Publish(_ context.Context, _ kernel.OrgScope, c app.Claim, o domain.Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.add("publish")
	if err := s.fenced(c); err != nil {
		return err
	}
	if s.publishErr != nil {
		return s.publishErr
	}
	if s.provisional == nil || *s.provisional != o {
		return errors.New("published object was not provisional")
	}
	s.state, s.lease, s.published = domain.Ready, "", &o
	return nil
}
func (s *fakeWorkerStore) Release(_ context.Context, _ kernel.OrgScope, c app.Claim) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.add("release")
	if err := s.fenced(c); err != nil {
		return err
	}
	s.state, s.lease = domain.Queued, ""
	return nil
}
func (s *fakeWorkerStore) Fail(_ context.Context, _ kernel.OrgScope, c app.Claim, code domain.FailureCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.add("fail:" + string(code))
	if err := s.fenced(c); err != nil {
		return err
	}
	s.state, s.lease, s.failure = domain.Failed, "", code
	return nil
}
func (s *fakeWorkerStore) Retire(context.Context, kernel.OrgScope, string) (app.Retirement, error) {
	s.events.add("retire")
	return s.retire, s.retireErr
}
func (s *fakeWorkerStore) MarkDeleted(_ context.Context, _ kernel.OrgScope, _ string, o domain.Object) error {
	s.events.add("mark-deleted")
	s.deleted = append(s.deleted, o)
	return nil
}

func workerFixture(t *testing.T, parameters string) (kernel.OrgScope, *events, *fakeWorkerStore, *fakeObjects, *rowsFormat, app.Worker) {
	t.Helper()
	s := scopeOf(t, 1)
	e := &events{}
	format := syntheticFormat()
	at := time.Date(2026, 11, 14, 11, 0, 0, 0, time.UTC)
	store := &fakeWorkerStore{events: e, state: domain.Queued, claim: app.Claim{Requester: app.Requester{AccountID: id(2), MembershipID: id(4)}, Kind: format.kind, Parameters: []byte(parameters), AsOf: at,
		Limits: format.limits, Retention: domain.DefaultRetention, MaxAttempts: domain.MaxAttempts}}
	objects := &fakeObjects{events: e}
	w := app.Worker{Store: store, Formats: registry(t, format), Objects: objects, NewID: sequence(200), LeaseTTL: app.DefaultLeaseTTL}
	return s, e, store, objects, format, w
}

// BUC-EMP-116, B-BR-EMP-229: the worker rechecks, produces within ceilings,
// records the provisional object, uploads and verifies, then publishes.
func TestBuildPublishesOnlyAVerifiedProvisionalObject(t *testing.T) {
	s, e, store, objects, format, w := workerFixture(t, `{"rows":2}`)
	if err := w.Build(t.Context(), s, id(50)); err != nil {
		t.Fatal(err)
	}
	if e.String() != "claim,check,provisional,put,publish" || store.state != domain.Ready || store.published == nil || store.published.Rows != 2 || format.calls != 1 {
		t.Fatal("build order or result", e)
	}
	key, _ := store.published.Key(s)
	if string(objects.stored[key]) != "row_id,value_centavos\n1,100\n2,200\n" {
		t.Fatal("stored bytes differ from the produced file")
	}
	e.log = nil
	if err := w.Build(t.Context(), s, id(50)); err != nil || e.String() != "claim" || format.calls != 1 {
		t.Fatal("a settled job was worked again", err, e)
	}
}

// B-BR-EMP-228: lost authority, a missing format and a ceiling are settled
// failures that never produce or store a file.
func TestBuildFailsLoudlyWithoutPublishing(t *testing.T) {
	for name, tc := range map[string]struct {
		parameters string
		setup      func(*fakeWorkerStore, *app.Worker, *rowsFormat)
		code       domain.FailureCode
		produced   bool
	}{
		"rights lost": {`{}`, func(s *fakeWorkerStore, _ *app.Worker, _ *rowsFormat) { s.rightsErr = app.ErrRightsLost }, domain.RightsLost, false},
		"format removed": {`{}`, func(_ *fakeWorkerStore, w *app.Worker, _ *rowsFormat) {
			w.Formats, _ = app.NewRegistry()
		}, domain.FormatUnavailable, false},
		"row ceiling":  {`{"rows":11}`, nil, domain.TooLarge, true},
		"byte ceiling": {`{"rows":10}`, func(s *fakeWorkerStore, _ *app.Worker, _ *rowsFormat) { s.claim.Limits.Bytes = 40 }, domain.TooLarge, true},
		"time ceiling": {`{}`, func(_ *fakeWorkerStore, _ *app.Worker, f *rowsFormat) { f.block = make(chan struct{}) }, domain.TooLarge, true},
	} {
		t.Run(name, func(t *testing.T) {
			s, e, store, objects, format, w := workerFixture(t, tc.parameters)
			if tc.setup != nil {
				tc.setup(store, &w, format)
			}
			started := time.Now()
			if err := w.Build(t.Context(), s, id(50)); err != nil {
				t.Fatal("settled failure was retried", err)
			}
			if store.state != domain.Failed || store.failure != tc.code || store.published != nil || store.provisional != nil || objects.puts != 0 || (format.calls == 1) != tc.produced {
				t.Fatal("failure published or produced", e, store.failure)
			}
			if name == "time ceiling" && time.Since(started) > 5*time.Second {
				t.Fatal("time ceiling not applied")
			}
			if strings.Contains(e.String(), "put") || strings.Contains(e.String(), "publish") {
				t.Fatal("a refused export reached storage", e)
			}
		})
	}
}

// B-BR-EMP-228/229: storage failure releases and retries; the third failed
// attempt ends FAILED; a published object is never one that failed to store.
func TestBuildRetriesTransientFailuresWithinTheAttemptBudget(t *testing.T) {
	s, e, store, objects, _, w := workerFixture(t, `{}`)
	objects.putErr = app.ErrUnavailable
	for attempt := 1; attempt <= 2; attempt++ {
		if err := w.Build(t.Context(), s, id(50)); err == nil {
			t.Fatal("transient storage failure was not retried")
		}
		if store.state != domain.Queued || store.published != nil {
			t.Fatal("failed upload left the job running or published", attempt)
		}
	}
	if err := w.Build(t.Context(), s, id(50)); err != nil {
		t.Fatal("exhausted attempts must settle", err)
	}
	if store.state != domain.Failed || store.failure != domain.AttemptsExhausted || store.published != nil || store.claim.Attempts != 3 {
		t.Fatal("third failure did not end FAILED", e)
	}
	// A transient producer error is retried the same way.
	s, _, store, _, format, w := workerFixture(t, `{}`)
	format.err = errors.New("synthetic transient reader failure")
	if err := w.Build(t.Context(), s, id(50)); err == nil || store.state != domain.Queued {
		t.Fatal("transient producer failure not retried")
	}
	// Shutdown cancellation is not a time ceiling.
	s, _, store, _, format, w = workerFixture(t, `{}`)
	format.block = make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	if err := w.Build(ctx, s, id(50)); err == nil || store.state == domain.Failed {
		t.Fatal("worker shutdown was recorded as a ceiling failure", store.failure)
	}
}

// B-BR-EMP-229: a different provisional object of an earlier attempt is
// deleted before the new one is recorded; the same one is reused.
func TestBuildReplacesOnlyADifferentProvisionalObject(t *testing.T) {
	s, e, store, objects, _, w := workerFixture(t, `{"rows":2}`)
	stale, _ := fileObject(t, s, id(50), 5)
	store.provisional = &stale
	if err := w.Build(t.Context(), s, id(50)); err != nil {
		t.Fatal(err)
	}
	if e.String() != "claim,check,delete,provisional,put,publish" || objects.deletes != 1 || store.published.SHA256 == stale.SHA256 {
		t.Fatal("stale provisional object kept or deleted late", e)
	}
	s, e, store, objects, _, w = workerFixture(t, `{"rows":2}`)
	same, _ := fileObject(t, s, id(50), 2)
	store.provisional = &same
	if err := w.Build(t.Context(), s, id(50)); err != nil || objects.deletes != 0 || e.String() != "claim,check,provisional,put,publish" {
		t.Fatal("identical provisional object was deleted", err, e)
	}
	s, _, store, objects, _, w = workerFixture(t, `{"rows":2}`)
	store.provisional = &stale
	objects.deleteErr = app.ErrUnavailable
	if err := w.Build(t.Context(), s, id(50)); err == nil || store.provisional.SHA256 != stale.SHA256 || objects.puts != 0 {
		t.Fatal("new object recorded while the stale one could not be deleted", err)
	}
}

// B-BR-EMP-228: busy and lost leases never publish; settled jobs are left alone.
func TestBuildRespectsLeasesAndSettledJobs(t *testing.T) {
	s, _, store, objects, format, w := workerFixture(t, `{}`)
	store.state = domain.Running
	if err := w.Build(t.Context(), s, id(50)); !errors.Is(err, app.ErrBusy) || format.calls != 0 {
		t.Fatal("busy job worked twice", err)
	}
	for _, state := range []domain.State{domain.Ready, domain.Failed, domain.Expired} {
		store.state = state
		if err := w.Build(t.Context(), s, id(50)); err != nil || format.calls != 0 {
			t.Fatal("settled job retried or worked", state, err)
		}
	}
	store.state, store.claimErr = domain.Queued, app.ErrNotFound
	if err := w.Build(t.Context(), s, id(50)); err != nil {
		t.Fatal("missing job retried", err)
	}
	if objects.puts != 0 || objects.deletes != 0 {
		t.Fatal("busy, settled or missing jobs touched storage")
	}
	s, e, store, objects, _, w := workerFixture(t, `{}`)
	objects.putHook = func() { store.stealLease = true }
	if err := w.Build(t.Context(), s, id(50)); !errors.Is(err, app.ErrBusy) || store.published != nil {
		t.Fatal("stale holder published", err, e)
	}
	s, _, store, _, _, w = workerFixture(t, `{}`)
	store.claimErr = app.ErrUnavailable
	if err := w.Build(t.Context(), s, id(50)); err == nil {
		t.Fatal("unavailable store settled the job")
	}
	s, _, store, _, _, w = workerFixture(t, `{}`)
	store.rightsErr = app.ErrUnavailable
	if err := w.Build(t.Context(), s, id(50)); err == nil || store.state != domain.Queued {
		t.Fatal("authority outage was not retried", err)
	}
	if err := (app.Worker{}).Build(t.Context(), s, id(50)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unconfigured worker", err)
	}
	if err := w.Build(t.Context(), s, "bad"); err != nil {
		t.Fatal("malformed reference must settle without retry", err)
	}
}

// B-BR-EMP-230: cleanup deletes and confirms before recording the deletion.
func TestCleanupDeletesBeforeRecording(t *testing.T) {
	s, e, store, objects, _, w := workerFixture(t, `{}`)
	object, raw := fileObject(t, s, id(50), 1)
	key, _ := object.Key(s)
	objects.stored = map[string][]byte{key: raw}
	store.retire = app.Retirement{Object: &object}
	if err := w.Cleanup(t.Context(), s, id(50)); err != nil || e.String() != "retire,delete,mark-deleted" || len(store.deleted) != 1 || objects.stored[key] != nil {
		t.Fatal("cleanup", err, e)
	}
	e.log = nil
	objects.deleteErr = app.ErrUnavailable
	if err := w.Cleanup(t.Context(), s, id(50)); err == nil || strings.Contains(e.String(), "mark-deleted") {
		t.Fatal("deletion recorded without confirmed absence", e)
	}
	e.log, objects.deleteErr = nil, nil
	store.retire = app.Retirement{Pending: true}
	if err := w.Cleanup(t.Context(), s, id(50)); err == nil || e.String() != "retire" {
		t.Fatal("cleanup before expiry must retry later", err)
	}
	store.retire = app.Retirement{}
	if err := w.Cleanup(t.Context(), s, id(50)); err != nil {
		t.Fatal("nothing to delete must settle", err)
	}
	store.retireErr = app.ErrUnavailable
	if err := w.Cleanup(t.Context(), s, id(50)); err == nil {
		t.Fatal("unavailable store settled cleanup")
	}
	if err := w.Cleanup(t.Context(), s, "bad"); err != nil {
		t.Fatal("malformed reference must settle", err)
	}
}
