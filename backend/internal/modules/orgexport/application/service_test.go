package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

// EDR-157: every export policy is the fresh rpt.export.run key; only the
// request writes.
func TestPoliciesAreFreshExportRun(t *testing.T) {
	request, read := app.Policy(app.ActionRequest), app.Policy(app.ActionRead)
	if request != (access.Policy{Permission: "rpt.export.run", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}) ||
		read != (access.Policy{Permission: "rpt.export.run", Capability: "organizations", Effect: access.Read, Fresh: true}) {
		t.Fatal("export policies drifted", request, read)
	}
	if request.Validate() != nil || read.Validate() != nil {
		t.Fatal("export policies are not admitted by the catalog")
	}
	for _, action := range []app.Action{0, 3, 255} {
		if app.Policy(action) != (access.Policy{}) {
			t.Fatal("unknown action has a policy")
		}
	}
	for _, actor := range []app.Actor{{}, {AccountID: id(1)}, {AccountID: "bad", SessionID: id(2)}} {
		if !errors.Is(actor.Validate(), app.ErrUnauthenticated) {
			t.Fatal("invalid actor admitted")
		}
	}
	if (app.Actor{AccountID: id(1), SessionID: id(2)}).Validate() != nil || (app.Requester{AccountID: id(1), MembershipID: id(2)}).Validate() != nil {
		t.Fatal("valid identities refused")
	}
	for _, requester := range []app.Requester{{}, {AccountID: id(1)}, {AccountID: id(1), MembershipID: strings.ToUpper(id(2))}} {
		if !errors.Is(requester.Validate(), domain.ErrInvalid) {
			t.Fatal("invalid requester admitted")
		}
	}
	if fmt.Sprint(app.Actor{AccountID: id(1), SessionID: id(2)}) != "OrgExport(redacted)" {
		t.Fatal("actor formatting leaks identifiers")
	}
}

// EDR-156: the registry is closed; production registers no format.
func TestRegistryIsClosed(t *testing.T) {
	empty, err := app.NewRegistry()
	if err != nil || len(empty.Kinds()) != 0 {
		t.Fatal("empty production registry refused", err)
	}
	if _, ok := empty.Lookup("test.synthetic_rows"); ok {
		t.Fatal("unregistered kind found")
	}
	a, b := syntheticFormat(), &rowsFormat{kind: "test.other_rows", limits: domain.DefaultLimits()}
	r, err := app.NewRegistry(b, a)
	if err != nil || strings.Join(r.Kinds(), ",") != "test.other_rows,test.synthetic_rows" {
		t.Fatal("registry kinds", err)
	}
	if f, ok := r.Lookup("test.synthetic_rows"); !ok || f != a {
		t.Fatal("registered kind not found")
	}
	for _, bad := range [][]app.Format{{nil}, {a, a}, {&rowsFormat{kind: "invalid", limits: domain.DefaultLimits()}}, {&rowsFormat{kind: "test.bad_limits", limits: domain.Limits{Rows: domain.MaxRows + 1, Bytes: 1, Duration: time.Second}}}, {mediaFormat{syntheticFormat()}}} {
		if _, err := app.NewRegistry(bad...); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid registry admitted")
		}
	}
}

type mediaFormat struct{ *rowsFormat }

func (mediaFormat) MediaType() string { return "text/csv; charset=utf-8" }

// B-BR-EMP-227: only a registered kind with canonical parameters becomes a
// command; its ceilings come from the format and its digest binds requester.
func TestCommandAdmitsOnlyRegisteredKindsAndCanonicalParameters(t *testing.T) {
	s := scopeOf(t, 1)
	r := registry(t)
	c, err := app.NewCommand(s, r, app.CommandInput{OperationID: id(30), Kind: "test.synthetic_rows", Parameters: []byte(`{"rows": 2}`)})
	if err != nil || !c.Valid() || c.OperationID() != id(30) || c.Kind() != "test.synthetic_rows" || string(c.Parameters()) != `{"rows":2}` || c.MediaType() != "text/csv" ||
		c.Limits() != (domain.Limits{Rows: 10, Bytes: 1024, Duration: time.Second}) || c.Retention() != domain.DefaultRetention || len(c.ParametersDigest()) != 64 {
		t.Fatal("command", err)
	}
	c.Parameters()[0] = 'X'
	if c.Parameters()[0] != '{' {
		t.Fatal("command parameters are mutable")
	}
	expected, _ := domain.ParametersDigest(s, "test.synthetic_rows", []byte(`{"rows":2}`))
	if c.ParametersDigest() != expected {
		t.Fatal("parameters digest is not the domain digest")
	}
	one, two := app.Requester{AccountID: id(2), MembershipID: id(4)}, app.Requester{AccountID: id(2), MembershipID: id(9)}
	if d := c.Digest(s, one); len(d) != 64 || d == c.Digest(s, two) || d == c.Digest(scopeOf(t, 7), one) || c.Digest(kernel.OrgScope{}, one) != "" || c.Digest(s, app.Requester{}) != "" {
		t.Fatal("command digest does not bind organization and requester")
	}
	if _, err := app.NewCommand(s, r, app.CommandInput{OperationID: id(30), Kind: "test.unknown_rows", Parameters: []byte(`{}`)}); !errors.Is(err, app.ErrFormatUnavailable) {
		t.Fatal("unregistered kind admitted", err)
	}
	if _, err := app.NewCommand(s, app.Registry{}, app.CommandInput{OperationID: id(30), Kind: "test.synthetic_rows", Parameters: []byte(`{}`)}); !errors.Is(err, app.ErrFormatUnavailable) {
		t.Fatal("empty registry admitted a kind", err)
	}
	for _, in := range []app.CommandInput{
		{OperationID: "bad", Kind: "test.synthetic_rows", Parameters: []byte(`{}`)},
		{OperationID: id(30), Kind: "Invalid", Parameters: []byte(`{}`)},
		{OperationID: id(30), Kind: "test.synthetic_rows", Parameters: []byte(`[]`)},
		{OperationID: id(30), Kind: "test.synthetic_rows", Parameters: []byte(`{"a":null}`)},
		{OperationID: id(30), Kind: "test.synthetic_rows", Parameters: []byte(`{"refuse":1}`)},
	} {
		if _, err := app.NewCommand(s, r, in); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid command admitted", in.Kind, err)
		}
	}
	if (app.Command{}).Valid() || (app.Command{}).Digest(s, one) != "" {
		t.Fatal("zero command is usable")
	}
}

func serviceFixture(t *testing.T, action app.Action) (context.Context, kernel.OrgScope, app.Actor, access.Decision, *events, *fakeRepository, *fakeObjects, *fakeTokens, *fakeRecorder, app.Service, domain.Object, []byte) {
	t.Helper()
	ctx, s, actor, d := decision(t, action)
	e := &events{}
	object, raw := fileObject(t, s, id(50), 2)
	repo := &fakeRepository{events: e, object: object, view: readyView(t, object)}
	objects := &fakeObjects{events: e, stored: map[string][]byte{}}
	key, _ := object.Key(s)
	objects.stored[key] = raw
	tokens := &fakeTokens{events: e}
	recorder := &fakeRecorder{events: e}
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	service := app.Service{Enabled: true, Repository: repo, Formats: registry(t), Objects: objects, Tokens: tokens, Recorder: recorder, NewID: sequence(100), Now: func() time.Time { return now }}
	return ctx, s, actor, d, e, repo, objects, tokens, recorder, service, object, raw
}

func TestServiceRequestBindsDecisionAndValidatesTheStoredJob(t *testing.T) {
	ctx, s, actor, d, e, repo, _, _, _, service, _, _ := serviceFixture(t, app.ActionRequest)
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	in := app.CommandInput{OperationID: id(30), Kind: "test.synthetic_rows", Parameters: []byte(`{"rows":2}`)}
	c, _ := app.NewCommand(s, service.Formats, in)
	repo.view = app.View{Kind: "test.synthetic_rows", Parameters: c.Parameters(), ParametersDigest: c.ParametersDigest(), State: domain.Queued, MaxAttempts: 3, RequestedAt: at, AsOf: at}
	v, replayed, err := service.Request(ctx, s, actor, d, in)
	if err != nil || replayed || v.ExportID != id(101) || v.State != domain.Queued || repo.member != id(memberN) || repo.requested.ParametersDigest() != c.ParametersDigest() || e.String() != "request" {
		t.Fatal("request", err, e)
	}
	repo.replayed = true
	if _, replayed, err := service.Request(ctx, s, actor, d, in); err != nil || !replayed {
		t.Fatal("replay lost", err)
	}
	if _, _, err := service.Request(ctx, s, actor, d, app.CommandInput{OperationID: id(30), Kind: "test.unknown_rows", Parameters: []byte(`{}`)}); !errors.Is(err, app.ErrFormatUnavailable) || strings.Count(e.String(), "request") != 2 {
		t.Fatal("unregistered kind reached the repository", err)
	}
	for name, view := range map[string]app.View{
		"other kind":   {ExportID: id(101), Kind: "test.other_rows", Parameters: c.Parameters(), ParametersDigest: c.ParametersDigest(), State: domain.Queued, MaxAttempts: 3, RequestedAt: at, AsOf: at},
		"other digest": {ExportID: id(101), Kind: "test.synthetic_rows", Parameters: c.Parameters(), ParametersDigest: strings.Repeat("b", 64), State: domain.Queued, MaxAttempts: 3, RequestedAt: at, AsOf: at},
		"invalid":      {ExportID: id(101), Kind: "test.synthetic_rows", Parameters: c.Parameters(), ParametersDigest: c.ParametersDigest(), State: domain.Ready, MaxAttempts: 3, RequestedAt: at, AsOf: at},
	} {
		repo.view, repo.replayed = view, false
		if _, _, err := service.Request(ctx, s, actor, d, in); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("inconsistent stored job returned:", name, err)
		}
	}
	repo.err = app.ErrQueueFull
	if _, _, err := service.Request(ctx, s, actor, d, in); !errors.Is(err, app.ErrQueueFull) {
		t.Fatal("queue full lost", err)
	}
	_, _, readActor, readDecision := decision(t, app.ActionRead)
	if _, _, err := service.Request(ctx, s, readActor, readDecision, in); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("read decision authorized a request", err)
	}
	if _, _, err := (app.Service{}).Request(ctx, s, actor, d, in); !errors.Is(err, app.ErrSurfaceDisabled) {
		t.Fatal("disabled surface", err)
	}
	broken := service
	broken.Repository = nil
	if _, _, err := broken.Request(ctx, s, actor, d, in); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("missing repository", err)
	}
	if _, _, err := service.Request(ctx, scopeOf(t, 7), actor, d, in); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("decision of another organization accepted", err)
	}
	if _, _, err := service.Request(ctx, s, app.Actor{AccountID: id(9), SessionID: id(3)}, d, in); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("decision of another account accepted", err)
	}
}

func TestServiceReadAndListAreRequesterBoundAndValidated(t *testing.T) {
	ctx, s, actor, d, _, repo, _, _, _, service, _, _ := serviceFixture(t, app.ActionRead)
	v, err := service.Read(ctx, s, actor, d, id(50))
	if err != nil || v.ExportID != id(50) || repo.member != id(memberN) {
		t.Fatal("read", err)
	}
	if _, err := service.Read(ctx, s, actor, d, "bad"); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("non-canonical export identifier", err)
	}
	page, err := service.List(ctx, s, actor, d, app.Filter{Limit: 20})
	if err != nil || len(page.Exports) != 1 {
		t.Fatal("list", err)
	}
	for _, f := range []app.Filter{{}, {Limit: 101}, {Limit: 1, State: "DONE"}, {Limit: 1, After: app.Key{ExportID: id(1)}}} {
		if _, err := service.List(ctx, s, actor, d, f); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid filter admitted", f, err)
		}
	}
	repo.view.SHA256 = ""
	if _, err := service.Read(ctx, s, actor, d, id(50)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("inconsistent READY job returned", err)
	}
	if _, err := service.List(ctx, s, actor, d, app.Filter{Limit: 20}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("inconsistent listed job returned", err)
	}
	repo.err = app.ErrNotFound
	if _, err := service.Read(ctx, s, actor, d, id(50)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("not found lost", err)
	}
	_, _, writeActor, writeDecision := decision(t, app.ActionRequest)
	if _, err := service.Read(ctx, s, writeActor, writeDecision, id(50)); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("request decision authorized a read", err)
	}
}

func TestListPagesAreNewestFirstWithAConsistentNextKey(t *testing.T) {
	ctx, s, actor, d, _, _, _, _, _, service, object, _ := serviceFixture(t, app.ActionRead)
	newer, older := readyView(t, object), readyView(t, object)
	older.ExportID, older.RequestedAt, older.AsOf = id(51), newer.RequestedAt.Add(-time.Hour), newer.AsOf.Add(-time.Hour)
	repo := &pageRepository{page: app.Page{Exports: []app.View{newer, older}, Next: app.KeyOf(older)}}
	service.Repository = repo
	page, err := service.List(ctx, s, actor, d, app.Filter{Limit: 2})
	if err != nil || len(page.Exports) != 2 || page.Next != app.KeyOf(older) {
		t.Fatal("ordered page", err)
	}
	key, err := app.ParseKey(page.Next.String())
	if err != nil || key != app.KeyOf(older) {
		t.Fatal("key round trip", err)
	}
	for _, raw := range []string{"", "x", id(1), "2026-11-14T11:00:00Z/" + id(1) + "/x", "2026-11-14T11:00:00+01:00/" + id(1), "2026-11-14T11:00:00Z/bad"} {
		if _, err := app.ParseKey(raw); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid key parsed", raw)
		}
	}
	for name, p := range map[string]app.Page{
		"oldest first":   {Exports: []app.View{older, newer}},
		"next mismatch":  {Exports: []app.View{newer, older}, Next: app.KeyOf(newer)},
		"short with key": {Exports: []app.View{newer}, Next: app.KeyOf(newer)},
		"too many":       {Exports: []app.View{newer, older, older}},
	} {
		repo.page = p
		if _, err := service.List(ctx, s, actor, d, app.Filter{Limit: 2}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("inconsistent page returned:", name, err)
		}
	}
	repo.page = app.Page{Exports: []app.View{older}}
	if _, err := service.List(ctx, s, actor, d, app.Filter{Limit: 2, After: app.KeyOf(older)}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("page did not start after the cursor", err)
	}
	repo.page = app.Page{Exports: []app.View{newer}}
	if _, err := service.List(ctx, s, actor, d, app.Filter{Limit: 2, State: domain.Failed}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("page ignored the state filter", err)
	}
}

type pageRepository struct {
	fakeRepository
	page app.Page
}

func (r *pageRepository) List(context.Context, kernel.OrgScope, app.Actor, string, app.Filter) (app.Page, error) {
	return r.page, nil
}

// B-BR-EMP-230: access records ISSUED before minting; nothing is minted when
// the record or a recheck fails.
func TestAccessRecordsIssuanceBeforeMintingAndRechecks(t *testing.T) {
	ctx, s, actor, d, e, repo, _, tokens, recorder, service, object, _ := serviceFixture(t, app.ActionRead)
	capability, err := service.Access(ctx, s, actor, d, id(50))
	if err != nil || capability.Token == "" || !capability.ExpiresAt.Equal(time.Date(2026, 11, 14, 12, 5, 0, 0, time.UTC)) {
		t.Fatal("access", err)
	}
	if e.String() != "deliverable,record:ISSUED,mint,deliverable" {
		t.Fatal("access did not record before minting and recheck after", e)
	}
	record := recorder.records[0]
	if record.Category != audit.ExportFile || record.Stage != audit.AccessIssued || record.AccountID != id(accountN) || record.MembershipID != id(memberN) || record.TargetID != object.ExportID ||
		record.AccessID != record.RequestID || record.AccessID != tokens.issued || len(record.Binding) != 64 || strings.Contains(record.Binding, object.SHA256) {
		t.Fatal("issued record lost a reference or exposed the file digest", record)
	}
	if fmt.Sprint(capability) != "orgexport: capability redacted" || fmt.Sprintf("%#v", capability) != "orgexport: capability redacted" {
		t.Fatal("capability can be formatted")
	}
	for name, setup := range map[string]func(){
		"audit outage":   func() { recorder.err = audit.ErrUnavailable },
		"audit replayed": func() { recorder.err = audit.ErrReplayed },
		"not ready":      func() { repo.deliverErr = []error{app.ErrNotReady} },
		"expired":        func() { repo.deliverErr = []error{app.ErrExpired} },
		"recheck lost":   func() { repo.deliverErr = []error{nil, app.ErrNotFound} },
		"mint failure":   func() { tokens.issueErr = errors.New("synthetic") },
	} {
		recorder.err, repo.deliverErr, tokens.issueErr = nil, nil, nil
		e.log = nil
		setup()
		capability, err := service.Access(ctx, s, actor, d, id(50))
		if err == nil || capability.Token != "" {
			t.Fatal("capability issued despite", name)
		}
		if strings.HasPrefix(name, "audit") && strings.Contains(e.String(), "mint") {
			t.Fatal("capability minted before a successful record:", name)
		}
		if name == "not ready" && !errors.Is(err, app.ErrNotReady) || name == "expired" && !errors.Is(err, app.ErrExpired) {
			t.Fatal("state refusal lost its code", name, err)
		}
	}
	recorder.err, repo.deliverErr, tokens.issueErr = nil, nil, nil
	repo.object.SHA256 = "invalid"
	if _, err := service.Access(ctx, s, actor, d, id(50)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("invalid deliverable object issued", err)
	}
}

// B-BR-EMP-230: content verifies the capability, reads and verifies the
// object, records SERVED against the grant and only then returns bytes.
func TestOpenVerifiesRecordsThenServesExactBytes(t *testing.T) {
	ctx, s, actor, d, e, repo, objects, tokens, recorder, service, object, raw := serviceFixture(t, app.ActionRead)
	capability, err := service.Access(ctx, s, actor, d, id(50))
	if err != nil {
		t.Fatal(err)
	}
	e.log = nil
	content, err := service.Open(ctx, s, actor, d, id(50), capability.Token)
	if err != nil || string(content.Bytes) != string(raw) || content.SHA256 != object.SHA256 || content.MediaType != "text/csv" || content.Rows != 2 || content.ExportID != id(50) {
		t.Fatal("open", err)
	}
	if e.String() != "deliverable,verify,get,record:SERVED,deliverable,verify" {
		t.Fatal("open did not verify, read, record and recheck in order", e)
	}
	served := recorder.records[1]
	if served.Stage != audit.AccessServed || served.AccessID != tokens.issued || served.RequestID == served.AccessID || served.Binding != recorder.records[0].Binding {
		t.Fatal("served record is not bound to its grant", served)
	}
	if fmt.Sprint(content) != "orgexport: content redacted" {
		t.Fatal("content can be formatted")
	}
	for name, setup := range map[string]func(){
		"bad token":      func() { tokens.verifyErr = errors.New("synthetic") },
		"audit outage":   func() { recorder.err = audit.ErrUnavailable },
		"storage outage": func() { objects.getErr = app.ErrUnavailable },
		"corrupt object": func() { objects.corrupt = true },
		"expired":        func() { repo.deliverErr = []error{app.ErrExpired} },
		"recheck lost":   func() { repo.deliverErr = []error{nil, app.ErrNotFound} },
	} {
		tokens.verifyErr, recorder.err, objects.getErr, objects.corrupt, repo.deliverErr = nil, nil, nil, false, nil
		before := len(recorder.records)
		setup()
		content, err := service.Open(ctx, s, actor, d, id(50), capability.Token)
		if err == nil || content.Bytes != nil {
			t.Fatal("bytes served despite", name)
		}
		switch name {
		case "bad token":
			if !errors.Is(err, app.ErrNotFound) || len(recorder.records) != before {
				t.Fatal("bad capability was not an opaque refusal without record", err)
			}
		case "storage outage", "corrupt object":
			if !errors.Is(err, app.ErrUnavailable) || len(recorder.records) != before {
				t.Fatal("unverified object was recorded as served", err)
			}
		case "expired":
			if !errors.Is(err, app.ErrExpired) {
				t.Fatal("expiry lost its code", err)
			}
		}
	}
	tokens.verifyErr, recorder.err, objects.getErr, objects.corrupt, repo.deliverErr = nil, nil, nil, false, nil
	if _, err := service.Open(ctx, s, actor, d, id(50), ""); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("empty capability", err)
	}
	if _, err := service.Open(ctx, s, actor, d, id(50), strings.Repeat("x", 2049)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("oversized capability", err)
	}
	if _, err := (app.Service{}).Open(ctx, s, actor, d, id(50), capability.Token); !errors.Is(err, app.ErrSurfaceDisabled) {
		t.Fatal("disabled surface served", err)
	}
	_, _, writeActor, writeDecision := decision(t, app.ActionRequest)
	if _, err := service.Open(ctx, s, writeActor, writeDecision, id(50), capability.Token); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("request decision opened content", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.Open(canceled, s, actor, d, id(50), capability.Token); err == nil {
		t.Fatal("canceled request served")
	}
}

func TestViewValidationMatchesTheLifecycle(t *testing.T) {
	s := scopeOf(t, 1)
	object, _ := fileObject(t, s, id(50), 1)
	ready := readyView(t, object)
	if ready.Validate() != nil {
		t.Fatal("ready view refused")
	}
	at := ready.RequestedAt
	queued := app.View{ExportID: id(50), Kind: "test.synthetic_rows", Parameters: []byte(`{}`), ParametersDigest: strings.Repeat("a", 64), State: domain.Queued, MaxAttempts: 3, RequestedAt: at, AsOf: at}
	failed := queued
	failed.State, failed.FailureCode, failed.FailedAt, failed.Attempts = domain.Failed, domain.TooLarge, at.Add(time.Minute), 1
	expired := ready
	expired.State, expired.ExpiredAt = domain.Expired, ready.ExpiresAt
	for _, v := range []app.View{queued, failed, expired} {
		if v.Validate() != nil {
			t.Fatal("consistent view refused", v.State)
		}
	}
	for name, change := range map[string]func(*app.View){
		"ready without digest":    func(v *app.View) { v.SHA256 = "" },
		"ready without expiry":    func(v *app.View) { v.ExpiresAt = time.Time{} },
		"ready with failure":      func(v *app.View) { v.FailureCode = domain.TooLarge },
		"expiry before ready":     func(v *app.View) { v.ExpiresAt = v.ReadyAt },
		"unknown state":           func(v *app.View) { v.State = "DONE" },
		"bad kind":                func(v *app.View) { v.Kind = "x" },
		"bad id":                  func(v *app.View) { v.ExportID = "x" },
		"attempts over budget":    func(v *app.View) { v.Attempts = 4 },
		"as of differs":           func(v *app.View) { v.AsOf = v.AsOf.Add(time.Second) },
		"bad parameters":          func(v *app.View) { v.Parameters = []byte(`[]`) },
		"bad parameters digest":   func(v *app.View) { v.ParametersDigest = "x" },
		"rows above bytes budget": func(v *app.View) { v.Bytes = 0 },
	} {
		v := ready
		change(&v)
		if v.Validate() == nil {
			t.Fatal("inconsistent view admitted:", name)
		}
	}
	bad := failed
	bad.FailureCode = ""
	if bad.Validate() == nil {
		t.Fatal("failed view without code admitted")
	}
	bad = queued
	bad.ReadyAt = at
	if bad.Validate() == nil {
		t.Fatal("queued view with ready instant admitted")
	}
}
