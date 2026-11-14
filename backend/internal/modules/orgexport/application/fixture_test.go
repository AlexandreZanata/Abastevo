package application_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

func id(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

func scopeOf(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(id(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type currentAuthority struct{ value access.Authority }

func (s currentAuthority) LoadAuthority(context.Context, kernel.OrgScope, string) (access.Authority, error) {
	return s.value, nil
}

type root struct{}

func (root) Resolve(_ context.Context, s kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{OrgID: s.OrgID(), ID: s.OrgID().String()}, nil
}

type proof struct{}

func (proof) Confirm(context.Context, access.Subject, access.Policy) error { return nil }

type capabilityGate struct{}

func (capabilityGate) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return nil
}

type records struct{}

func (records) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return nil
}

const (
	accountN = 2
	sessionN = 3
	memberN  = 4
)

// decision completes the real orgaccess pipeline for a FINANCE member.
func decision(t *testing.T, action app.Action) (context.Context, kernel.OrgScope, app.Actor, access.Decision) {
	t.Helper()
	s := scopeOf(t, 1)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	role, err := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: id(6), MembershipID: id(memberN), AssignedBy: id(memberN), Role: permissions.Finance, Scope: permissions.OrgScope, TargetID: s.OrgID().String(), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic finance fixture", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := access.Authorization{Source: currentAuthority{access.Authority{OrgID: s.OrgID(), AccountID: id(accountN), MembershipID: id(memberN), EmployeeID: id(5), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: capabilityGate{}, Records: records{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	t.Cleanup(finish)
	prepared, err := a.Prepare(ctx, s, access.Actor{AccountID: id(accountN), Channel: "web"}, app.Policy(action), root{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Complete(ctx, prepared, proof{})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, s, app.Actor{AccountID: id(accountN), SessionID: id(sessionN)}, d
}

// rowsFormat is the synthetic test-only kind: it writes a header and the
// requested number of rows, deterministically.
type rowsFormat struct {
	kind   string
	limits domain.Limits
	block  chan struct{}
	err    error
	calls  int
	mu     sync.Mutex
}

func (f *rowsFormat) Kind() string { return f.kind }
func (*rowsFormat) MediaType() string {
	return "text/csv"
}
func (f *rowsFormat) Limits() domain.Limits { return f.limits }
func (f *rowsFormat) Canonical(raw []byte) ([]byte, error) {
	if bytes.Contains(raw, []byte(`"refuse"`)) {
		return nil, errors.New("synthetic format refusal")
	}
	return raw, nil
}
func (f *rowsFormat) Produce(ctx context.Context, in app.ProduceInput, sink *domain.Sink) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.err != nil {
		return f.err
	}
	rows := 3
	if bytes.Contains(in.Parameters, []byte(`"rows":`)) {
		_, _ = fmt.Sscanf(string(in.Parameters[bytes.Index(in.Parameters, []byte(`"rows":`))+7:]), "%d", &rows)
	}
	if _, err := sink.Write([]byte("row_id,value_centavos\n")); err != nil {
		return err
	}
	for i := 1; i <= rows; i++ {
		if err := sink.Row([]byte(fmt.Sprintf("%d,%d\n", i, i*100))); err != nil {
			return err
		}
	}
	return nil
}

func syntheticFormat() *rowsFormat {
	return &rowsFormat{kind: "test.synthetic_rows", limits: domain.Limits{Rows: 10, Bytes: 1024, Duration: time.Second}}
}

func registry(t *testing.T, formats ...app.Format) app.Registry {
	t.Helper()
	if len(formats) == 0 {
		formats = []app.Format{syntheticFormat()}
	}
	r, err := app.NewRegistry(formats...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type events struct {
	mu  sync.Mutex
	log []string
}

func (e *events) add(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.log = append(e.log, s)
}
func (e *events) String() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.Join(e.log, ",")
}

// fakeRepository serves a single job of the calling member.
type fakeRepository struct {
	events     *events
	view       app.View
	replayed   bool
	object     domain.Object
	err        error
	deliverErr []error
	requested  app.Command
	member     string
}

func (r *fakeRepository) Request(_ context.Context, _ kernel.OrgScope, _ app.Actor, member, exportID string, c app.Command) (app.View, bool, error) {
	r.events.add("request")
	r.requested, r.member = c, member
	if r.err != nil {
		return app.View{}, false, r.err
	}
	v := r.view
	if v.ExportID == "" {
		v.ExportID = exportID
	}
	return v, r.replayed, nil
}
func (r *fakeRepository) Read(_ context.Context, _ kernel.OrgScope, _ app.Actor, member, _ string) (app.View, error) {
	r.events.add("read")
	r.member = member
	return r.view, r.err
}
func (r *fakeRepository) List(_ context.Context, _ kernel.OrgScope, _ app.Actor, member string, _ app.Filter) (app.Page, error) {
	r.events.add("list")
	r.member = member
	return app.Page{Exports: []app.View{r.view}}, r.err
}
func (r *fakeRepository) Deliverable(context.Context, kernel.OrgScope, app.Actor, string, string) (domain.Object, error) {
	r.events.add("deliverable")
	if len(r.deliverErr) > 0 {
		err := r.deliverErr[0]
		r.deliverErr = r.deliverErr[1:]
		if err != nil {
			return domain.Object{}, err
		}
	}
	return r.object, r.err
}

type fakeObjects struct {
	events          *events
	stored          map[string][]byte
	putErr, getErr  error
	deleteErr       error
	corrupt         bool
	puts, deletes   int
	mu              sync.Mutex
	putHook         func()
	deletedPrefixes []string
}

func (o *fakeObjects) Put(_ context.Context, s kernel.OrgScope, object domain.Object, raw []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events.add("put")
	o.puts++
	if o.putHook != nil {
		o.putHook()
	}
	if o.putErr != nil {
		return o.putErr
	}
	key, err := object.Key(s)
	if err != nil {
		return err
	}
	if err := object.Check(raw); err != nil {
		return err
	}
	if o.stored == nil {
		o.stored = map[string][]byte{}
	}
	o.stored[key] = bytes.Clone(raw)
	return nil
}
func (o *fakeObjects) Get(_ context.Context, s kernel.OrgScope, object domain.Object) ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events.add("get")
	if o.getErr != nil {
		return nil, o.getErr
	}
	key, _ := object.Key(s)
	raw, ok := o.stored[key]
	if !ok {
		return nil, app.ErrUnavailable
	}
	if o.corrupt {
		raw = append([]byte("X"), raw[1:]...)
	}
	if err := object.Check(raw); err != nil {
		return nil, err
	}
	return bytes.Clone(raw), nil
}
func (o *fakeObjects) Delete(_ context.Context, s kernel.OrgScope, object domain.Object) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events.add("delete")
	o.deletes++
	if o.deleteErr != nil {
		return o.deleteErr
	}
	key, _ := object.Key(s)
	delete(o.stored, key)
	o.deletedPrefixes = append(o.deletedPrefixes, key)
	return nil
}

type fakeTokens struct {
	events    *events
	issued    string
	verifyErr error
	issueErr  error
}

func (k *fakeTokens) Issue(_ kernel.OrgScope, r app.Requester, o domain.Object, accessID string, now time.Time) (app.Capability, error) {
	k.events.add("mint")
	if k.issueErr != nil {
		return app.Capability{}, k.issueErr
	}
	k.issued = accessID
	return app.Capability{Token: "synthetic-capability:" + accessID, ExpiresAt: now.Add(domain.CapabilityTTL)}, nil
}
func (k *fakeTokens) Verify(_ kernel.OrgScope, r app.Requester, o domain.Object, token string, _ time.Time) (string, error) {
	k.events.add("verify")
	if k.verifyErr != nil || !strings.HasPrefix(token, "synthetic-capability:") {
		return "", errors.New("synthetic refusal")
	}
	return strings.TrimPrefix(token, "synthetic-capability:"), nil
}

type fakeRecorder struct {
	events  *events
	records []audit.SensitiveAccess
	err     error
}

func (r *fakeRecorder) RecordAccess(_ context.Context, _ kernel.OrgScope, a audit.SensitiveAccess) error {
	r.events.add("record:" + string(a.Stage))
	if r.err != nil {
		return r.err
	}
	if err := a.Validate(); err != nil {
		return err
	}
	r.records = append(r.records, a)
	return nil
}

func sequence(start int) func() (string, error) {
	var mu sync.Mutex
	n := start
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return id(n), nil
	}
}

// readyView is a READY job of the fixture member.
func readyView(t *testing.T, object domain.Object) app.View {
	t.Helper()
	at := time.Date(2026, 11, 14, 11, 0, 0, 0, time.UTC)
	return app.View{ExportID: object.ExportID, Kind: "test.synthetic_rows", Parameters: []byte(`{}`), ParametersDigest: strings.Repeat("a", 64), State: domain.Ready, Attempts: 1, MaxAttempts: 3,
		Rows: object.Rows, Bytes: object.Bytes, SHA256: object.SHA256, MediaType: object.MediaType, RequestedAt: at, AsOf: at, ReadyAt: at.Add(time.Minute), ExpiresAt: at.Add(domain.DefaultRetention + time.Minute)}
}

func fileObject(t *testing.T, s kernel.OrgScope, exportID string, rows int) (domain.Object, []byte) {
	t.Helper()
	sink, err := domain.NewSink(domain.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = sink.Write([]byte("row_id,value_centavos\n"))
	for i := 1; i <= rows; i++ {
		_ = sink.Row([]byte(fmt.Sprintf("%d,%d\n", i, i*100)))
	}
	file, err := sink.Result()
	if err != nil {
		t.Fatal(err)
	}
	object, err := domain.ObjectFor(s, exportID, "text/csv", file)
	if err != nil {
		t.Fatal(err)
	}
	return object, file.Bytes
}
