package application_test

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"testing"
	"time"
)

type rehireRepo struct {
	result app.RehireResult
	err    error
	calls  int
}

func (r *rehireRepo) Rehire(context.Context, kernel.OrgScope, app.EmployeeActor, string, app.RehireCommand) (app.RehireResult, error) {
	r.calls++
	return r.result, r.err
}
func TestRehireBindsCurrentRequestAndOriginalIntent(t *testing.T) {
	ctx, scope, actor, decision, finish := accessChangeDecision(t, app.MembershipReactivate)
	c := app.RehireCommand{OperationID: invID(1401), EmployeeID: invID(1402), ExpectedRevision: 2, Reason: "Synthetic rehire"}
	repo := &rehireRepo{result: app.RehireResult{OperationID: c.OperationID, EmployeeID: c.EmployeeID, Revision: 3, State: d.EmployeeActive, Reason: c.Reason, OccurredAt: time.Now().UTC()}}
	service := app.RehireService{Enabled: true, Repository: repo}
	if _, e := service.Rehire(ctx, scope, actor, decision, c); e != nil || repo.calls != 1 {
		t.Fatal("current authority", e)
	}
	for _, badctx := range []context.Context{context.Background()} {
		if _, e := service.Rehire(badctx, scope, actor, decision, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) || repo.calls != 1 {
			t.Fatal("detached decision", e)
		}
	}
	if _, e := service.Rehire(ctx, scope, actor, access.Decision{}, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("forged decision", e)
	}
	changed := c
	changed.Reason = "Changed reason"
	if c.Digest(scope) == changed.Digest(scope) {
		t.Fatal("unbound reason")
	}
	changed = c
	changed.ExpectedRevision++
	if c.Digest(scope) == changed.Digest(scope) {
		t.Fatal("unbound revision")
	}
	repo.result.State = d.EmployeeTerminated
	if out, e := service.Rehire(ctx, scope, actor, decision, c); !errors.Is(e, app.ErrRegisterUnavailable) || out.OperationID != "" {
		t.Fatal("corrupt acknowledgement", e)
	}
	repo.result.State = d.EmployeeActive
	repo.err = errors.New("synthetic failure")
	if out, e := service.Rehire(ctx, scope, actor, decision, c); e == nil || out.OperationID != "" {
		t.Fatal("provisional metadata")
	}
	repo.err = nil
	finish()
	if _, e := service.Rehire(ctx, scope, actor, decision, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("finished request", e)
	}
	for _, bad := range []app.RehireCommand{{}, {OperationID: c.OperationID, EmployeeID: c.EmployeeID, ExpectedRevision: 0, Reason: c.Reason}, {OperationID: c.OperationID, EmployeeID: c.EmployeeID, ExpectedRevision: 2, Reason: "\n"}} {
		if bad.Validate() == nil || bad.Digest(scope) != "" {
			t.Fatal("invalid intent")
		}
	}
}
