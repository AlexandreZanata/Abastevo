package application_test

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"strings"
	"testing"
	"time"
)

type offboardRepo struct {
	out   app.OffboardingResult
	calls int
	err   error
}

func (r *offboardRepo) ReadOffboarding(context.Context, kernel.OrgScope, app.EmployeeActor, string, string, bool) (app.OffboardingView, error) {
	r.calls++
	return r.out.View, r.err
}
func (r *offboardRepo) CompleteOffboarding(context.Context, kernel.OrgScope, app.EmployeeActor, string, app.OffboardingCommand) (app.OffboardingResult, error) {
	r.calls++
	return r.out, r.err
}
func TestOffboardingIntentAndCurrentRequestBinding(t *testing.T) {
	ctx, scope, a, decision, finish := accessChangeDecision(t, app.MembershipSuspend)
	c := app.OffboardingCommand{OperationID: invID(1201), MembershipID: invID(1202), ExpectedRevision: 2, EmployeeRevision: 1, Reason: "Synthetic ending", HandoverDigest: strings.Repeat("a", 64), Flags: []string{}}
	if c.Validate() != nil {
		t.Fatal("valid intent")
	}
	h := c.Digest(scope)
	changed := c
	changed.TerminateEmployee = true
	if changed.Digest(scope) == h {
		t.Fatal("employee intent unbound")
	}
	changed = c
	changed.Reason = "Changed"
	if changed.Digest(scope) == h {
		t.Fatal("reason unbound")
	}
	repo := &offboardRepo{out: app.OffboardingResult{OperationID: c.OperationID, Reason: c.Reason, OccurredAt: time.Now().UTC(), View: app.OffboardingView{State: d.AccessState{OrgID: scope.OrgID(), ID: c.MembershipID, EmployeeID: invID(1203), Model: d.ExistingAccount, State: d.MembershipEnded, EndReason: d.Offboarded, Revision: 3}, EmployeeState: d.EmployeeActive, EmployeeRevision: 1, Digest: c.HandoverDigest, Items: []d.HandoverItem{}}}}
	s := app.OffboardingService{Enabled: true, Repository: repo}
	if _, e := s.Complete(ctx, scope, a, decision, c); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []context.Context{context.Background()} {
		if _, e := s.Complete(bad, scope, a, decision, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
			t.Fatal("detached decision", e)
		}
	}
	if _, e := s.Complete(ctx, scope, a, access.Decision{}, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("forged decision", e)
	}
	repo.out.View.State.Revision = 4
	if out, e := s.Complete(ctx, scope, a, decision, c); !errors.Is(e, app.ErrRegisterUnavailable) || out.OperationID != "" {
		t.Fatal("provisional metadata")
	}
	repo.out.View.State.Revision = 3
	repo.err = errors.New("synthetic failure")
	if out, e := s.Complete(ctx, scope, a, decision, c); e == nil || out.OperationID != "" {
		t.Fatal("result on error")
	}
	repo.err = nil
	finish()
	if _, e := s.Complete(ctx, scope, a, decision, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("finished decision", e)
	}
	for _, mutate := range []func(*app.OffboardingCommand){func(c *app.OffboardingCommand) { c.Self = true; c.TerminateEmployee = true }, func(c *app.OffboardingCommand) { c.Flags = []string{"unknown"} }, func(c *app.OffboardingCommand) { c.EmployeeRevision = 0 }, func(c *app.OffboardingCommand) { c.Reason = "\n" }, func(c *app.OffboardingCommand) { c.HandoverDigest = "invalid" }} {
		bad := c
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid intent")
		}
	}
}
