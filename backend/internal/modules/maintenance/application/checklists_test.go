package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func id(n int) string { return fmt.Sprintf("a7000000-0000-4000-8000-%012d", n) }
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

type target struct{ id string }

func (r target) Resolve(_ context.Context, s kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if r.id == "" {
		return permissions.ResourceFacts{OrgID: s.OrgID(), ID: s.OrgID().String()}, nil
	}
	return permissions.ResourceFacts{OrgID: s.OrgID(), ID: r.id}, nil
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

func decision(t *testing.T, action app.ChecklistAction, resource string) (context.Context, kernel.OrgScope, app.ChecklistActor, access.Decision) {
	t.Helper()
	s := scope(t)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	role, err := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: id(6), MembershipID: id(4), AssignedBy: id(4), Role: permissions.FleetManager, Scope: permissions.OrgScope, TargetID: s.OrgID().String(), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic fleet manager fixture", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := access.Authorization{Source: currentAuthority{access.Authority{OrgID: s.OrgID(), AccountID: id(2), MembershipID: id(4), EmployeeID: id(5), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: capability{}, Records: records{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	t.Cleanup(finish)
	prepared, err := a.Prepare(ctx, s, access.Actor{AccountID: id(2), Channel: "web"}, app.Policy(action), target{resource})
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Complete(ctx, prepared, proof{})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, s, app.ChecklistActor{AccountID: id(2), SessionID: id(3)}, d
}

type repository struct {
	ack        app.TemplateAck
	page       app.TemplatePage
	view       app.TemplateView
	applicable app.ApplicableView
	err        error
	calls      int
	newID      string
}

func (r *repository) CommandTemplate(_ context.Context, _ kernel.OrgScope, _ app.ChecklistActor, _, newID string, _ app.TemplateCommand) (app.TemplateAck, bool, error) {
	r.calls++
	r.newID = newID
	return r.ack, false, r.err
}
func (r *repository) ListTemplates(context.Context, kernel.OrgScope, app.ChecklistActor, string, app.TemplateFilter) (app.TemplatePage, error) {
	r.calls++
	return r.page, r.err
}
func (r *repository) ReadTemplate(context.Context, kernel.OrgScope, app.ChecklistActor, string, string, int64) (app.TemplateView, error) {
	r.calls++
	return r.view, r.err
}
func (r *repository) ReadApplicable(context.Context, kernel.OrgScope, app.ChecklistActor, string, string, checklist.Context) (app.ApplicableView, error) {
	r.calls++
	return r.applicable, r.err
}

func command(t *testing.T, in app.TemplateCommandInput) app.TemplateCommand {
	t.Helper()
	if in.OperationID == "" {
		in.OperationID = id(30)
	}
	c, err := app.NewTemplateCommand(in)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var items = []checklist.Item{{Label: "Pneus", Critical: true}, {Label: "Estepe", Optional: true}}

func TestPoliciesAreFixedAndTheApplicableReadIsTheFleetAssetRead(t *testing.T) {
	if app.Policy(app.ApplicableRead) != fleet.Policy(false) {
		t.Fatal("the applicable read must be exactly fleet.asset.read")
	}
	manage, read := app.Policy(app.TemplateManage), app.Policy(app.TemplateRead)
	if manage.Permission != "check.template.manage" || !manage.Fresh || manage.Effect != access.ConfigurationWrite || read.Permission != "check.template.manage" || read.Fresh || read.Effect != access.Read || manage.Validate() != nil || read.Validate() != nil {
		t.Fatal("template policies", manage, read)
	}
	if app.Policy(0) != (access.Policy{}) {
		t.Fatal("unknown action has a policy")
	}
}

func TestTemplateCommandsAreExactPerKind(t *testing.T) {
	org := checklist.Target{Kind: checklist.TargetOrganization}
	create := command(t, app.TemplateCommandInput{Kind: app.CommandCreate, Context: checklist.Departure, Name: "Saída", Target: checklist.Target{Kind: checklist.TargetCategories, IDs: []string{id(12), id(11)}}, Items: items})
	in, err := create.Snapshot()
	if err != nil || in.Policy != checklist.DefaultPolicy() || in.Target.IDs[0] != id(11) {
		t.Fatal("create defaults and canonical target", in, err)
	}
	for name, bad := range map[string]app.TemplateCommandInput{
		"create with template":       {Kind: app.CommandCreate, OperationID: id(30), TemplateID: id(7), Context: checklist.Departure, Name: "x", Target: org, Items: items},
		"create with revision":       {Kind: app.CommandCreate, OperationID: id(30), ExpectedRevision: 1, Context: checklist.Departure, Name: "x", Target: org, Items: items},
		"create without context":     {Kind: app.CommandCreate, OperationID: id(30), Name: "x", Target: org, Items: items},
		"create with invalid policy": {Kind: app.CommandCreate, OperationID: id(30), Context: checklist.Departure, Name: "x", Target: org, Items: items, Policy: checklist.Policy{Mode: checklist.Warn, ValiditySeconds: 60}},
		"create critical optional":   {Kind: app.CommandCreate, OperationID: id(30), Context: checklist.Departure, Name: "x", Target: org, Items: []checklist.Item{{Label: "x", Critical: true, Optional: true}}},
		"publish with context":       {Kind: app.CommandPublish, OperationID: id(30), TemplateID: id(7), ExpectedRevision: 1, Context: checklist.Return, Name: "x", Target: org, Items: items},
		"publish with policy":        {Kind: app.CommandPublish, OperationID: id(30), TemplateID: id(7), ExpectedRevision: 1, Name: "x", Target: org, Items: items, Policy: checklist.DefaultPolicy()},
		"publish without revision":   {Kind: app.CommandPublish, OperationID: id(30), TemplateID: id(7), Name: "x", Target: org, Items: items},
		"policy with content":        {Kind: app.CommandPolicy, OperationID: id(30), TemplateID: id(7), ExpectedRevision: 1, Name: "x", Policy: checklist.DefaultPolicy()},
		"policy invalid":             {Kind: app.CommandPolicy, OperationID: id(30), TemplateID: id(7), ExpectedRevision: 1, Policy: checklist.Policy{Mode: "ALLOW", ValiditySeconds: 3600}},
		"archive with policy":        {Kind: app.CommandArchive, OperationID: id(30), TemplateID: id(7), ExpectedRevision: 1, Policy: checklist.DefaultPolicy()},
		"archive with items":         {Kind: app.CommandArchive, OperationID: id(30), TemplateID: id(7), ExpectedRevision: 1, Items: items},
		"unknown kind":               {Kind: "DELETE", OperationID: id(30), TemplateID: id(7), ExpectedRevision: 1},
		"bad operation":              {Kind: app.CommandArchive, OperationID: "x", TemplateID: id(7), ExpectedRevision: 1},
		"bad template":               {Kind: app.CommandArchive, OperationID: id(30), TemplateID: "X", ExpectedRevision: 1},
	} {
		if _, err := app.NewTemplateCommand(bad); !errors.Is(err, checklist.ErrInvalid) {
			t.Fatal("accepted", name, err)
		}
	}
	s := scope(t)
	digest := create.Digest(s)
	again := command(t, app.TemplateCommandInput{Kind: app.CommandCreate, Context: checklist.Departure, Name: "Saída", Target: checklist.Target{Kind: checklist.TargetCategories, IDs: []string{id(11), id(12)}}, Items: items})
	if len(digest) != 64 || again.Digest(s) != digest {
		t.Fatal("digest is canonical over the target order", digest)
	}
	for name, other := range map[string]app.TemplateCommandInput{
		"name":    {Kind: app.CommandCreate, Context: checklist.Departure, Name: "Outra", Target: checklist.Target{Kind: checklist.TargetCategories, IDs: []string{id(11), id(12)}}, Items: items},
		"context": {Kind: app.CommandCreate, Context: checklist.Return, Name: "Saída", Target: checklist.Target{Kind: checklist.TargetCategories, IDs: []string{id(11), id(12)}}, Items: items},
		"flag":    {Kind: app.CommandCreate, Context: checklist.Departure, Name: "Saída", Target: checklist.Target{Kind: checklist.TargetCategories, IDs: []string{id(11), id(12)}}, Items: []checklist.Item{{Label: "Pneus"}, {Label: "Estepe", Optional: true}}},
		"policy":  {Kind: app.CommandCreate, Context: checklist.Departure, Name: "Saída", Target: checklist.Target{Kind: checklist.TargetCategories, IDs: []string{id(11), id(12)}}, Items: items, Policy: checklist.Policy{Mode: checklist.Warn, ValiditySeconds: 43200}},
	} {
		if command(t, other).Digest(s) == digest {
			t.Fatal("digest ignores", name)
		}
	}
	if (app.TemplateCommand{}).Digest(s) != "" {
		t.Fatal("zero command digested")
	}
	d, err := create.Definition(checklist.Return)
	if err != nil || d.Context != checklist.Return {
		t.Fatal("definition under the template context", err)
	}
	archive := command(t, app.TemplateCommandInput{Kind: app.CommandArchive, TemplateID: id(7), ExpectedRevision: 1})
	if _, err := archive.Definition(checklist.Departure); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("archive has no definition", err)
	}
}

func TestChecklistServiceBindsDecisionsAndValidatesResults(t *testing.T) {
	ctx, s, actor, manage := decision(t, app.TemplateManage, "")
	repo := &repository{ack: app.TemplateAck{TemplateID: id(40), Revision: 1, Version: 1, PolicyRevision: 1, State: app.TemplateActive}}
	service := app.ChecklistService{Enabled: true, Repository: repo, NewID: func() (string, error) { return id(40), nil }}
	create := command(t, app.TemplateCommandInput{Kind: app.CommandCreate, Context: checklist.Departure, Name: "Saída", Target: checklist.Target{Kind: checklist.TargetOrganization}, Items: items})
	if ack, _, err := service.Command(ctx, s, actor, manage, create); err != nil || ack.TemplateID != id(40) || repo.newID != id(40) {
		t.Fatal("create", ack, err)
	}
	for name, bad := range map[string]app.TemplateAck{
		"wrong revision": {TemplateID: id(40), Revision: 2, Version: 1, PolicyRevision: 1, State: app.TemplateActive},
		"archived":       {TemplateID: id(40), Revision: 1, Version: 1, PolicyRevision: 1, State: app.TemplateArchived},
		"bad identifier": {TemplateID: "x", Revision: 1, Version: 1, PolicyRevision: 1, State: app.TemplateActive},
	} {
		repo.ack = bad
		if _, _, err := service.Command(ctx, s, actor, manage, create); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid acknowledgement accepted", name, err)
		}
	}
	publish := command(t, app.TemplateCommandInput{Kind: app.CommandPublish, TemplateID: id(40), ExpectedRevision: 3, Name: "Nova", Target: checklist.Target{Kind: checklist.TargetOrganization}, Items: items})
	repo.ack = app.TemplateAck{TemplateID: id(40), Revision: 4, Version: 3, PolicyRevision: 2, State: app.TemplateActive}
	if _, _, err := service.Command(ctx, s, actor, manage, publish); err != nil || repo.newID != "" {
		t.Fatal("publish carries no new identifier", err)
	}
	repo.ack.TemplateID = id(41)
	if _, _, err := service.Command(ctx, s, actor, manage, publish); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("acknowledgement of another template", err)
	}
	repo.err = app.ErrConflict
	if _, _, err := service.Command(ctx, s, actor, manage, publish); !errors.Is(err, app.ErrConflict) {
		t.Fatal("repository refusal", err)
	}
	repo.err = nil
	_, _, _, read := decision(t, app.TemplateRead, "")
	calls := repo.calls
	if _, _, err := service.Command(ctx, s, actor, read, publish); !errors.Is(err, app.ErrUnauthenticated) || repo.calls != calls {
		t.Fatal("a read decision authorized a command", err)
	}
	if _, _, err := (app.ChecklistService{Repository: repo}).Command(ctx, s, actor, manage, publish); !errors.Is(err, app.ErrSurfaceDisabled) {
		t.Fatal("disabled surface", err)
	}
	if _, _, err := (app.ChecklistService{Enabled: true, Repository: repo}).Command(ctx, s, actor, manage, create); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("missing identifier source", err)
	}
	if _, _, err := service.Command(ctx, s, app.ChecklistActor{AccountID: id(9), SessionID: id(3)}, manage, publish); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("another actor", err)
	}

	readCtx, _, _, readDecision := decision(t, app.TemplateRead, "")
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	repo.view = app.TemplateView{TemplateID: id(40), Context: checklist.Departure, State: app.TemplateActive, Revision: 2, Version: 2, PolicyRevision: 1, CreatedAt: now,
		Selected: app.VersionView{Version: 1, Name: "Saída", Target: checklist.Target{Kind: checklist.TargetOrganization}, Items: items, PublishedAt: now},
		Policy:   app.PolicyView{Revision: 1, Policy: checklist.DefaultPolicy(), EffectiveAt: now}}
	if v, err := service.Read(readCtx, s, actor, readDecision, id(40), 1); err != nil || v.Selected.Version != 1 {
		t.Fatal("read version 1", err)
	}
	if _, err := service.Read(readCtx, s, actor, readDecision, id(40), 0); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("current read answered an older version", err)
	}
	if _, err := service.Read(readCtx, s, actor, readDecision, "x", 0); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("malformed template", err)
	}
	repo.view.ArchivedAt = &now
	if _, err := service.Read(readCtx, s, actor, readDecision, id(40), 1); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an ACTIVE template with an archive instant", err)
	}
	repo.page = app.TemplatePage{Templates: []app.TemplateSummary{{TemplateID: id(42), Context: checklist.Return, State: app.TemplateActive, Revision: 1, Version: 1, PolicyRevision: 1, Name: "x", TargetKind: checklist.TargetAssets, CreatedAt: now}, {TemplateID: id(41), Context: checklist.Return, State: app.TemplateActive, Revision: 1, Version: 1, PolicyRevision: 1, Name: "x", TargetKind: checklist.TargetAssets, CreatedAt: now}}}
	if _, err := service.List(readCtx, s, actor, readDecision, app.TemplateFilter{Limit: 10}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unordered page", err)
	}
	if _, err := service.List(readCtx, s, actor, readDecision, app.TemplateFilter{}); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("zero limit", err)
	}

	asset := id(50)
	appCtx, _, _, assetDecision := decision(t, app.ApplicableRead, asset)
	repo.applicable = app.ApplicableView{AssetID: asset, Context: checklist.Departure, Match: checklist.MatchCategory, At: now.Add(time.Minute),
		Template: &app.ApplicableTemplate{TemplateID: id(40), Name: "Saída", Version: 2, PublishedAt: now, Items: items},
		Policy:   &app.PolicyView{Revision: 1, Policy: checklist.DefaultPolicy(), EffectiveAt: now}}
	if v, err := service.Applicable(appCtx, s, actor, assetDecision, asset, checklist.Departure); err != nil || v.Template.Version != 2 {
		t.Fatal("applicable", err)
	}
	if _, err := service.Applicable(appCtx, s, actor, assetDecision, id(51), checklist.Departure); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("decision bound to another asset", err)
	}
	if _, err := service.Applicable(readCtx, s, actor, readDecision, asset, checklist.Departure); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("template read decision used for the asset read", err)
	}
	repo.applicable.Policy.EffectiveAt = now.Add(time.Hour)
	if _, err := service.Applicable(appCtx, s, actor, assetDecision, asset, checklist.Departure); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("policy effective after the read instant", err)
	}
	repo.applicable = app.ApplicableView{AssetID: asset, Context: checklist.Departure, Match: checklist.MatchNone, At: now, Policy: &app.PolicyView{Revision: 1, Policy: checklist.DefaultPolicy(), EffectiveAt: now}}
	if _, err := service.Applicable(appCtx, s, actor, assetDecision, asset, checklist.Departure); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("NONE with a policy", err)
	}
	if _, err := service.Applicable(appCtx, s, actor, assetDecision, asset, "PARKING"); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("unknown context", err)
	}
}

func TestChecklistValuesAreRedacted(t *testing.T) {
	v := app.TemplateCommandInput{Kind: app.CommandCreate, Name: "Nome secreto de teste"}
	if got := fmt.Sprintf("%v %+v", v, app.ChecklistActor{AccountID: id(2), SessionID: id(3)}); got != "EnterpriseChecklist(redacted) EnterpriseChecklist(redacted)" {
		t.Fatal("redaction", got)
	}
}
