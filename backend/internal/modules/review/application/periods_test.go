package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/jackc/pgx/v5"
)

func id(n int) string { return fmt.Sprintf("a6000000-0000-4000-8000-%012d", n) }
func scope(t *testing.T) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(id(1))
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

type capability struct{}

func (capability) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return nil
}

type records struct{}

func (records) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return nil
}

func decision(t *testing.T, action app.PeriodAction) (context.Context, func(), kernel.OrgScope, app.PeriodActor, access.Decision) {
	t.Helper()
	s := scope(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	role, err := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: id(6), MembershipID: id(4), AssignedBy: id(4), Role: permissions.Finance, Scope: permissions.OrgScope, TargetID: s.OrgID().String(), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic finance fixture", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := access.Authorization{Source: currentAuthority{access.Authority{OrgID: s.OrgID(), AccountID: id(2), MembershipID: id(4), EmployeeID: id(5), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: capability{}, Records: records{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	prepared, err := a.Prepare(ctx, s, access.Actor{AccountID: id(2), Channel: "web"}, app.PeriodPolicy(action), root{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Complete(ctx, prepared, proof{})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, finish, s, app.PeriodActor{AccountID: id(2), SessionID: id(3)}, d
}

func command(t *testing.T, in app.PeriodCommandInput) app.PeriodCommand {
	t.Helper()
	if in.OperationID == "" {
		in.OperationID = id(30)
	}
	c, err := app.NewPeriodCommand(in)
	if err != nil {
		t.Fatal("fixture command", err)
	}
	return c
}

func TestPeriodPoliciesAreFixedSourcePolicies(t *testing.T) {
	want := map[app.PeriodAction]access.Policy{
		app.PeriodRead:   {Permission: "fin.period.close", Capability: "organizations", Effect: access.Read},
		app.PeriodClose:  {Permission: "fin.period.close", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true},
		app.PeriodReopen: {Permission: "fin.period.reopen", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true},
	}
	for action, policy := range want {
		if got := app.PeriodPolicy(action); got != policy || got.Validate() != nil {
			t.Fatal("period policy drift", action, got)
		}
	}
	if app.PeriodPolicy(0).Validate() == nil || app.PeriodPolicy(9).Validate() == nil {
		t.Fatal("unknown period action produced a valid policy")
	}
	for raw, action := range map[period.Action]app.PeriodAction{period.ActionClose: app.PeriodClose, period.ActionReopen: app.PeriodReopen} {
		if got, err := app.ActionPolicy(raw); err != nil || got != action {
			t.Fatal("command action policy drift", raw)
		}
	}
	if _, err := app.ActionPolicy("RELOCK"); !errors.Is(err, period.ErrInvalid) {
		t.Fatal("unknown command action accepted")
	}
}

func TestPeriodActorAndCommandValidation(t *testing.T) {
	if (app.PeriodActor{AccountID: id(2), SessionID: id(3)}).Validate() != nil {
		t.Fatal("valid actor refused")
	}
	for _, a := range []app.PeriodActor{{}, {AccountID: id(2)}, {AccountID: strings.ToUpper(id(2)), SessionID: id(3)}, {AccountID: "00000000-0000-0000-0000-000000000000", SessionID: id(3)}} {
		if a.Validate() == nil {
			t.Fatal("invalid actor accepted")
		}
	}
	close := command(t, app.PeriodCommandInput{Action: period.ActionClose, Month: "2026-10", ExpectedRevision: 1})
	snap, err := close.Snapshot()
	if err != nil || snap.Month != "2026-10" || close.Month().String() != "2026-10" || snap.Action != period.ActionClose || snap.ExpectedRevision != 1 {
		t.Fatal("close command snapshot", err)
	}
	reason := "  Synthetic correction  "
	reopen := command(t, app.PeriodCommandInput{Action: period.ActionReopen, Month: "2026-10", ExpectedRevision: 2, Reason: reason})
	if snap, err := reopen.Snapshot(); err != nil || snap.Reason != reason {
		t.Fatal("reopen reason not kept verbatim", err)
	}
	if _, err := (app.PeriodCommand{}).Snapshot(); !errors.Is(err, period.ErrInvalid) {
		t.Fatal("zero command has a snapshot")
	}
	for _, c := range []struct {
		in   app.PeriodCommandInput
		want error
	}{
		{app.PeriodCommandInput{Action: period.ActionClose, OperationID: "bad", Month: "2026-10", ExpectedRevision: 1}, period.ErrInvalid},
		{app.PeriodCommandInput{Action: period.ActionClose, OperationID: id(30), Month: "2026-13", ExpectedRevision: 1}, period.ErrInvalid},
		{app.PeriodCommandInput{Action: period.ActionClose, OperationID: id(30), Month: "2026-10", ExpectedRevision: 0}, period.ErrInvalid},
		{app.PeriodCommandInput{Action: period.ActionClose, OperationID: id(30), Month: "2026-10", ExpectedRevision: 1, Reason: "x"}, period.ErrInvalid},
		{app.PeriodCommandInput{Action: "LOCK", OperationID: id(30), Month: "2026-10", ExpectedRevision: 1}, period.ErrInvalid},
		{app.PeriodCommandInput{Action: period.ActionReopen, OperationID: id(30), Month: "2026-10", ExpectedRevision: 2}, period.ErrReasonRequired},
		{app.PeriodCommandInput{Action: period.ActionReopen, OperationID: id(30), Month: "2026-10", ExpectedRevision: 2, Reason: "\t"}, period.ErrReasonRequired},
		{app.PeriodCommandInput{Action: period.ActionReopen, OperationID: id(30), Month: "2026-10", ExpectedRevision: 2, Reason: "Synthetic\treason"}, period.ErrReasonInvalid},
		{app.PeriodCommandInput{Action: period.ActionReopen, OperationID: id(30), Month: "2026-10", ExpectedRevision: 2, Reason: strings.Repeat("a", 281)}, period.ErrReasonInvalid},
	} {
		if _, err := app.NewPeriodCommand(c.in); !errors.Is(err, c.want) {
			t.Fatal("invalid command accepted or misclassified", c.in.Action, c.in.Month, err)
		}
	}
	// The digest binds every command field and the organization.
	s := scope(t)
	base := close.Digest(s)
	if len(base) != 64 || strings.Trim(base, "0123456789abcdef") != "" || base != close.Digest(s) {
		t.Fatal("digest is not a stable lower-hex SHA-256")
	}
	other, _ := kernel.ParseOrgID(id(99))
	otherScope, _ := kernel.NewOrgScope(other)
	for _, changed := range []string{
		close.Digest(otherScope),
		command(t, app.PeriodCommandInput{Action: period.ActionClose, OperationID: id(31), Month: "2026-10", ExpectedRevision: 1}).Digest(s),
		command(t, app.PeriodCommandInput{Action: period.ActionClose, Month: "2026-11", ExpectedRevision: 1}).Digest(s),
		command(t, app.PeriodCommandInput{Action: period.ActionClose, Month: "2026-10", ExpectedRevision: 3}).Digest(s),
		reopen.Digest(s),
		command(t, app.PeriodCommandInput{Action: period.ActionReopen, Month: "2026-10", ExpectedRevision: 2, Reason: "Synthetic correction"}).Digest(s),
	} {
		if changed == base || changed == "" {
			t.Fatal("digest ignored a command field")
		}
	}
	if (app.PeriodCommand{}).Digest(s) != "" {
		t.Fatal("zero command produced a digest")
	}
	if strings.Contains(fmt.Sprintf("%v %+v", reopen, snap), "Synthetic") {
		t.Fatal("command formatting leaked private reason")
	}
}

func TestPeriodFilterValidation(t *testing.T) {
	for _, f := range []app.PeriodFilter{{Limit: 1}, {Limit: 100, AfterMonth: "2026-10"}, {Limit: 20, State: period.Locked}, {Limit: 20, State: period.Reopened}} {
		if f.Validate() != nil {
			t.Fatal("valid filter refused", f.Limit, f.State)
		}
	}
	for _, f := range []app.PeriodFilter{{}, {Limit: 101}, {Limit: -1}, {Limit: 20, AfterMonth: "2026-13"}, {Limit: 20, State: period.Open}, {Limit: 20, State: "locked"}} {
		if !errors.Is(f.Validate(), period.ErrInvalid) {
			t.Fatal("invalid filter accepted", f.Limit, f.State)
		}
	}
}

func reopenedFact(reason string) app.PeriodFact {
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	return app.PeriodFact{Day: "2026-10-31", Month: "2026-10", PeriodID: id(40), State: period.Reopened, Revision: 3, CheckedAt: at.Add(time.Hour),
		Reopening: &app.ReopeningFact{Revision: 3, Authority: period.ReopenPermission, Reason: reason, ActorAccountID: id(2), ActorMembershipID: id(4), At: at}}
}

// B-BR-EMP-214: the gate's fact is exactly the kernel's period input, and only
// a stored fin.period.reopen transition with its reason proves a reopening.
func TestPeriodFactMapsToAmendmentKernel(t *testing.T) {
	checked := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	open := app.PeriodFact{Day: "2026-10-31", Month: "2026-10", State: period.Open, Revision: 1, CheckedAt: checked}
	locked := app.PeriodFact{Day: "2026-10-31", Month: "2026-10", PeriodID: id(40), State: period.Locked, Revision: 2, CheckedAt: checked}
	reopened := reopenedFact("Synthetic VAL-19 correction")
	for _, c := range []struct {
		fact     app.PeriodFact
		want     amend.Period
		writable bool
	}{
		{open, amend.Period{State: amend.PeriodOpen}, true},
		{locked, amend.Period{State: amend.PeriodLocked}, false},
		{reopened, amend.Period{State: amend.PeriodReopened, Reopening: &amend.Reopening{AuthorityProven: true, Reason: "Synthetic VAL-19 correction"}}, true},
	} {
		got, err := c.fact.Amendment()
		if err != nil || c.fact.Validate() != nil || c.fact.Writable() != c.writable || got.State != c.want.State || (got.Reopening == nil) != (c.want.Reopening == nil) || got.Reopening != nil && *got.Reopening != *c.want.Reopening {
			t.Fatal("period fact mapping drift", c.fact.State, err)
		}
	}
	// VAL-19 (a)/(b)/(c) through the kernel with these exact facts.
	facts := amend.Facts{RecordID: id(50), State: amend.StateApproved, Effective: 1, Latest: 1, ReportingDay: "2026-10-31"}
	amendReq := amend.Request{Intent: amend.IntentAmend, Expected: 1, Revision: 2, Base: 1, ActorID: id(2), At: checked, Reason: "Synthetic volume correction", Authority: amend.Authority{Amend: true}}
	for _, c := range []struct {
		fact app.PeriodFact
		want error
	}{{locked, amend.ErrPeriodLocked}, {reopened, nil}, {open, nil}} {
		p, err := c.fact.Amendment()
		if err != nil {
			t.Fatal(err)
		}
		facts.Period = p
		if _, err := amend.Decide(facts, amendReq); !errors.Is(err, c.want) && !(c.want == nil && err == nil) {
			t.Fatal("amendment outcome drift", c.fact.State, err)
		}
	}
	november := app.PeriodFact{Day: "2026-11-01", Month: "2026-11", State: period.Open, Revision: 1, CheckedAt: checked}
	posting, err := november.Amendment()
	if err != nil {
		t.Fatal(err)
	}
	lockedPeriod, _ := locked.Amendment()
	facts.Period = lockedPeriod
	adjust := amend.Request{Intent: amend.IntentAdjust, Expected: 1, Base: 1, ActorID: id(2), At: checked, Reason: "Synthetic adjustment", Authority: amend.Authority{Amend: true}, PostingDay: november.Day, PostingPeriod: &posting}
	if d, err := amend.Decide(facts, adjust); err != nil || d.Outcome != amend.OutcomeAdjustment || d.PostingDay != "2026-11-01" {
		t.Fatal("adjustment into the open period refused", err)
	}

	for name, edit := range map[string]func(*app.PeriodFact){
		"close authority":      func(f *app.PeriodFact) { f.Reopening.Authority = period.ClosePermission },
		"blank reason":         func(f *app.PeriodFact) { f.Reopening.Reason = "  " },
		"missing reopening":    func(f *app.PeriodFact) { f.Reopening = nil },
		"other revision":       func(f *app.PeriodFact) { f.Reopening.Revision = 5 },
		"invalid actor":        func(f *app.PeriodFact) { f.Reopening.ActorAccountID = "x" },
		"zero time":            func(f *app.PeriodFact) { f.Reopening.At = time.Time{} },
		"parity":               func(f *app.PeriodFact) { f.Revision = 4; f.Reopening.Revision = 4 },
		"day outside month":    func(f *app.PeriodFact) { f.Day = "2026-11-01" },
		"missing period id":    func(f *app.PeriodFact) { f.PeriodID = "" },
		"unchecked":            func(f *app.PeriodFact) { f.CheckedAt = time.Time{} },
		"zone without source":  func(f *app.PeriodFact) { f.Zone = "America/Sao_Paulo" },
		"unknown zone source":  func(f *app.PeriodFact) { f.Zone, f.ZoneSource = "America/Sao_Paulo", "device" },
		"reopening after read": func(f *app.PeriodFact) { f.Reopening.At = f.CheckedAt.Add(time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			f := reopenedFact("Synthetic reason")
			copied := *f.Reopening
			f.Reopening = &copied
			edit(&f)
			if !errors.Is(f.Validate(), app.ErrUnavailable) || f.Writable() {
				t.Fatal("unproven period fact accepted")
			}
			if _, err := f.Amendment(); !errors.Is(err, app.ErrUnavailable) {
				t.Fatal("unproven fact mapped to the kernel")
			}
		})
	}
	for name, f := range map[string]app.PeriodFact{
		"locked with reopening": {Day: "2026-10-31", Month: "2026-10", PeriodID: id(40), State: period.Locked, Revision: 2, CheckedAt: checked, Reopening: reopened.Reopening},
		"open with id":          {Day: "2026-10-31", Month: "2026-10", PeriodID: id(40), State: period.Open, Revision: 1, CheckedAt: checked},
		"open revision 2":       {Day: "2026-10-31", Month: "2026-10", State: period.Open, Revision: 2, CheckedAt: checked},
		"zero":                  {},
	} {
		if !errors.Is(f.Validate(), app.ErrUnavailable) {
			t.Fatal("invalid fact accepted", name)
		}
	}
	zoned := open
	zoned.Zone, zoned.ZoneSource = "America/Sao_Paulo", "organization"
	if zoned.Validate() != nil {
		t.Fatal("attributed fact refused")
	}
}

type fakeRepository struct {
	ack      app.PeriodAck
	view     app.PeriodView
	page     app.PeriodPage
	err      error
	replayed bool
	calls    int
	newID    string
	member   string
}

func (r *fakeRepository) TransitionPeriod(_ context.Context, _ kernel.OrgScope, _ app.PeriodActor, member, newID string, _ app.PeriodCommand) (app.PeriodAck, bool, error) {
	r.calls++
	r.member, r.newID = member, newID
	return r.ack, r.replayed, r.err
}
func (r *fakeRepository) ReadPeriod(_ context.Context, _ kernel.OrgScope, _ app.PeriodActor, member string, _ period.Month) (app.PeriodView, error) {
	r.calls++
	r.member = member
	return r.view, r.err
}
func (r *fakeRepository) ListPeriods(_ context.Context, _ kernel.OrgScope, _ app.PeriodActor, member string, _ app.PeriodFilter) (app.PeriodPage, error) {
	r.calls++
	r.member = member
	return r.page, r.err
}

func TestPeriodServiceBindsDecisionAndValidatesResults(t *testing.T) {
	recorded := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	ctx, finish, s, actor, closeDecision := decision(t, app.PeriodClose)
	defer finish()
	repo := &fakeRepository{ack: app.PeriodAck{Month: "2026-10", PeriodID: id(40), Kind: period.Lock, State: period.Locked, Revision: 2, RecordedAt: recorded}}
	service := app.PeriodService{Enabled: true, Repository: repo, NewID: func() (string, error) { return id(40), nil }}
	closeCmd := command(t, app.PeriodCommandInput{Action: period.ActionClose, Month: "2026-10", ExpectedRevision: 1})
	ack, replayed, err := service.Transition(ctx, s, actor, closeDecision, closeCmd)
	if err != nil || replayed || ack.Revision != 2 || repo.newID != id(40) || repo.member != id(4) {
		t.Fatal("close through the service", err)
	}
	repo.replayed = true
	if _, replayed, err := service.Transition(ctx, s, actor, closeDecision, closeCmd); err != nil || !replayed {
		t.Fatal("replay flag lost", err)
	}
	repo.replayed = false
	reopenCmd := command(t, app.PeriodCommandInput{Action: period.ActionReopen, Month: "2026-10", ExpectedRevision: 2, Reason: "Synthetic reason"})
	calls := repo.calls
	if _, _, err := service.Transition(ctx, s, actor, closeDecision, reopenCmd); !errors.Is(err, app.ErrUnauthenticated) || repo.calls != calls {
		t.Fatal("reopen ran under the close decision", err)
	}
	for name, ack := range map[string]app.PeriodAck{
		"wrong revision": {Month: "2026-10", PeriodID: id(40), Kind: period.Lock, State: period.Locked, Revision: 3, RecordedAt: recorded},
		"wrong month":    {Month: "2026-11", PeriodID: id(40), Kind: period.Lock, State: period.Locked, Revision: 2, RecordedAt: recorded},
		"wrong state":    {Month: "2026-10", PeriodID: id(40), Kind: period.Lock, State: period.Reopened, Revision: 2, RecordedAt: recorded},
		"reopen kind":    {Month: "2026-10", PeriodID: id(40), Kind: period.Reopen, State: period.Locked, Revision: 2, RecordedAt: recorded},
		"missing id":     {Month: "2026-10", Kind: period.Lock, State: period.Locked, Revision: 2, RecordedAt: recorded},
		"no time":        {Month: "2026-10", PeriodID: id(40), Kind: period.Lock, State: period.Locked, Revision: 2},
	} {
		repo.ack = ack
		if _, _, err := service.Transition(ctx, s, actor, closeDecision, closeCmd); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("impossible repository result accepted", name)
		}
	}
	repo.err = app.ErrNotFound
	if _, _, err := service.Transition(ctx, s, actor, closeDecision, closeCmd); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("repository refusal lost", err)
	}
	for _, broken := range []app.PeriodService{{Repository: repo, NewID: service.NewID}, {Enabled: true, NewID: service.NewID}, {Enabled: true, Repository: repo}, {Enabled: true, Repository: repo, NewID: func() (string, error) { return "x", nil }}} {
		_, _, err := broken.Transition(ctx, s, actor, closeDecision, closeCmd)
		if err == nil || (!broken.Enabled && !errors.Is(err, app.ErrSurfaceDisabled)) {
			t.Fatal("incomplete service accepted a command", err)
		}
	}
	if _, _, err := service.Transition(ctx, s, app.PeriodActor{AccountID: id(9), SessionID: id(3)}, closeDecision, closeCmd); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("rebound actor accepted", err)
	}
	if _, _, err := service.Transition(ctx, s, actor, closeDecision, app.PeriodCommand{}); !errors.Is(err, period.ErrInvalid) {
		t.Fatal("zero command accepted", err)
	}

	rctx, rfinish, _, _, reopenDecision := decision(t, app.PeriodReopen)
	defer rfinish()
	repo.err = nil
	repo.ack = app.PeriodAck{Month: "2026-10", PeriodID: id(40), Kind: period.Reopen, State: period.Reopened, Revision: 3, RecordedAt: recorded}
	if ack, _, err := service.Transition(rctx, s, actor, reopenDecision, reopenCmd); err != nil || ack.State != period.Reopened {
		t.Fatal("reopen through the service", err)
	}

	readCtx, readFinish, _, _, readDecision := decision(t, app.PeriodRead)
	defer readFinish()
	history := []app.PeriodTransitionView{
		{Revision: 2, Kind: period.Lock, From: period.Open, To: period.Locked, Authority: period.ClosePermission, ActorAccountID: id(2), ActorMembershipID: id(4), OperationID: id(30), RecordedAt: recorded},
		{Revision: 3, Kind: period.Reopen, From: period.Locked, To: period.Reopened, Authority: period.ReopenPermission, Reason: "Synthetic reason", ActorAccountID: id(2), ActorMembershipID: id(4), OperationID: id(31), RecordedAt: recorded.Add(time.Minute)},
	}
	repo.view = app.PeriodView{Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", PeriodID: id(40), State: period.Reopened, Revision: 3, History: history}
	view, err := service.Read(readCtx, s, actor, readDecision, "2026-10")
	if err != nil || len(view.History) != 2 || view.State != period.Reopened {
		t.Fatal("period read", err)
	}
	view.History[0].Reason = "mutated"
	if repo.view.History[0].Reason != "" {
		t.Fatal("read result shares the repository history")
	}
	if _, err := service.Read(readCtx, s, actor, readDecision, "2026-13"); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("non-canonical month not opaque not-found", err)
	}
	if _, err := service.Read(ctx, s, actor, closeDecision, "2026-10"); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("read ran under a write decision", err)
	}
	for name, v := range map[string]app.PeriodView{
		"broken chain": {Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", PeriodID: id(40), State: period.Reopened, Revision: 3, History: history[1:]},
		"head drift":   {Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", PeriodID: id(40), State: period.Locked, Revision: 2, History: history},
		"other month":  {Month: "2026-11", Start: "2026-11-01", End: "2026-12-01", PeriodID: id(40), State: period.Reopened, Revision: 3, History: history},
		"wrong range":  {Month: "2026-10", Start: "2026-10-01", End: "2026-10-31", PeriodID: id(40), State: period.Reopened, Revision: 3, History: history},
		"open with id": {Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", PeriodID: id(40), State: period.Open, Revision: 1},
	} {
		repo.view = v
		if _, err := service.Read(readCtx, s, actor, readDecision, "2026-10"); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("inconsistent period view accepted", name)
		}
	}
	repo.view = app.PeriodView{Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", State: period.Open, Revision: 1, History: []app.PeriodTransitionView{}}
	if v, err := service.Read(readCtx, s, actor, readDecision, "2026-10"); err != nil || v.PeriodID != "" || v.Revision != 1 {
		t.Fatal("implicit open period read", err)
	}

	page := app.PeriodPage{Periods: []app.PeriodView{
		{Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", PeriodID: id(40), State: period.Locked, Revision: 2},
		{Month: "2026-09", Start: "2026-09-01", End: "2026-10-01", PeriodID: id(41), State: period.Reopened, Revision: 3},
	}, NextMonth: "2026-09"}
	repo.page = page
	if got, err := service.List(readCtx, s, actor, readDecision, app.PeriodFilter{Limit: 2}); err != nil || len(got.Periods) != 2 || got.NextMonth != "2026-09" {
		t.Fatal("period list", err)
	}
	for name, bad := range map[string]app.PeriodPage{
		"ascending":  {Periods: []app.PeriodView{page.Periods[1], page.Periods[0]}},
		"over limit": {Periods: append(append([]app.PeriodView{}, page.Periods...), app.PeriodView{Month: "2026-08", Start: "2026-08-01", End: "2026-09-01", PeriodID: id(42), State: period.Locked, Revision: 2})},
		"open row":   {Periods: []app.PeriodView{{Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", State: period.Open, Revision: 1}}},
		"bad cursor": {Periods: page.Periods, NextMonth: "2026-10"},
		"after":      {Periods: page.Periods},
	} {
		repo.page = bad
		filter := app.PeriodFilter{Limit: 2}
		if name == "after" {
			filter.AfterMonth = "2026-09"
		}
		if _, err := service.List(readCtx, s, actor, readDecision, filter); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("inconsistent period page accepted", name)
		}
	}
	repo.page = app.PeriodPage{Periods: []app.PeriodView{page.Periods[0]}}
	if _, err := service.List(readCtx, s, actor, readDecision, app.PeriodFilter{Limit: 2, State: period.Reopened}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("state filter not enforced on results")
	}
	if _, err := service.List(readCtx, s, actor, readDecision, app.PeriodFilter{}); !errors.Is(err, period.ErrInvalid) {
		t.Fatal("invalid filter reached the repository")
	}
}

// The gate port signature is a transaction-scoped server port; this
// compile-time check keeps it from growing an HTTP or actor parameter.
var _ = func(g app.PeriodGate, tx pgx.Tx) {
	_, _ = g.StateForDayTx(context.Background(), tx, kernel.OrgScope{}, "")
	_, _ = g.StateForOccurrenceTx(context.Background(), tx, kernel.OrgScope{}, time.Time{}, "")
}
