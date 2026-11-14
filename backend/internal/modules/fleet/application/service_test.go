package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

type currentAuthority struct{ value access.Authority }

func (s currentAuthority) LoadAuthority(context.Context, kernel.OrgScope, string) (access.Authority, error) {
	return s.value, nil
}

type resource struct{ id string }

func (r resource) Resolve(_ context.Context, s kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{OrgID: s.OrgID(), ID: r.id}, nil
}

type syntheticProof struct{}

func (syntheticProof) Confirm(context.Context, access.Subject, access.Policy) error { return nil }

type syntheticCapability struct{}

func (syntheticCapability) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return nil
}

type syntheticRecordCheck struct{}

func (syntheticRecordCheck) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return nil
}
func decision(t *testing.T, target string, write bool, channels ...string) (context.Context, func(), kernel.OrgScope, app.FleetActor, access.Decision) {
	t.Helper()
	s := scope(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	role, e := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: id(6), MembershipID: id(4), AssignedBy: id(4), Role: permissions.Owner, Scope: permissions.OrgScope, TargetID: s.OrgID().String(), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic fleet unit fixture", Revision: 1})
	if e != nil {
		t.Fatal(e)
	}
	a := access.Authorization{Source: currentAuthority{value: access.Authority{OrgID: s.OrgID(), AccountID: id(2), MembershipID: id(4), EmployeeID: id(5), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: syntheticCapability{}, Records: syntheticRecordCheck{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	actor := access.Actor{AccountID: id(2), Channel: "web"}
	if len(channels) > 0 {
		actor.Channel = channels[0]
		if actor.Channel == "android" {
			actor.ContributorID = id(70)
			actor.DeviceKeyID = id(71)
		}
	}
	prepared, e := a.Prepare(ctx, s, actor, app.Policy(write), resource{id: target})
	if e != nil {
		t.Fatal(e)
	}
	d, e := a.Complete(ctx, prepared, syntheticProof{})
	if e != nil {
		t.Fatal(e)
	}
	return ctx, finish, s, app.FleetActor{AccountID: id(2), SessionID: id(3)}, d
}

func TestFleetServiceRefusesReboundAndCanceledAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, channel string
		alter         func(*app.FleetActor, *kernel.OrgScope)
		target        string
	}{
		{"account", "web", func(a *app.FleetActor, _ *kernel.OrgScope) { a.AccountID = id(99) }, id(20)},
		{"invalid-session", "web", func(a *app.FleetActor, _ *kernel.OrgScope) { a.SessionID = "bad" }, id(20)},
		{"organization", "web", func(_ *app.FleetActor, s *kernel.OrgScope) {
			org, _ := kernel.ParseOrgID(id(99))
			*s, _ = kernel.NewOrgScope(org)
		}, id(20)},
		{"target", "web", nil, id(21)},
		{"android-not-company", "android", nil, id(20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, finish, s, actor, d := decision(t, id(20), false, tc.channel)
			defer finish()
			repo := &repository{view: view(t, s, 20)}
			if tc.alter != nil {
				tc.alter(&actor, &s)
			}
			if _, e := (app.AssetService{Enabled: true, Repository: repo}).Read(ctx, s, actor, d, tc.target); !errors.Is(e, app.ErrUnauthenticated) || repo.reads != 0 {
				t.Fatal("rebound proof reached repository", e)
			}
		})
	}
	ctx, finish, s, actor, d := decision(t, id(20), false)
	defer finish()
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	repo := &repository{view: view(t, s, 20)}
	if _, e := (app.AssetService{Enabled: true, Repository: repo}).Read(ctx, s, actor, d, id(20)); !errors.Is(e, context.Canceled) || repo.reads != 0 {
		t.Fatal("cancellation lost", e)
	}
}

func TestFleetServiceRetainsOriginalDiscardAndRejectsPartialFailureOutput(t *testing.T) {
	ctx, finish, s, actor, d := decision(t, id(20), true)
	defer finish()
	command, e := app.NewAssetWrite(app.AssetWriteInput{Kind: app.DiscardAsset, OperationID: id(10), AssetID: id(20), ExpectedRevision: 3})
	if e != nil {
		t.Fatal(e)
	}
	repo := &repository{result: app.AssetResult{AssetID: id(20), Revision: 4, Discarded: true}}
	service := app.AssetService{Enabled: true, Repository: repo}
	out, _, e := service.Write(ctx, s, actor, d, command)
	if e != nil || out.State != "" || !out.Discarded || out.Revision != 4 || repo.newID != "" {
		t.Fatal("discard metadata invented state", e)
	}
	repo.err = app.ErrConflict
	out, replay, e := service.Write(ctx, s, actor, d, command)
	if !errors.Is(e, app.ErrConflict) || replay || out != (app.AssetResult{}) {
		t.Fatal("partial failure output escaped", e)
	}
	repo.err = nil
	repo.result.State = fleet.Draft
	if _, _, e = service.Write(ctx, s, actor, d, command); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("discard copied stale lifecycle", e)
	}
}

func TestFleetServiceListValidatesWholePageAndClonesSource(t *testing.T) {
	s := scope(t)
	ctx, finish, s, actor, d := decision(t, s.OrgID().String(), false)
	defer finish()
	v := view(t, s, 20)
	for name, page := range map[string]app.AssetPage{"duplicate": {Assets: []app.AssetView{v, v}}, "reverse": {Assets: []app.AssetView{view(t, s, 21), v}}, "bad-next": {Assets: []app.AssetView{v}, NextID: id(99)}, "too-many": {Assets: []app.AssetView{v, view(t, s, 21), view(t, s, 22)}}} {
		t.Run(name, func(t *testing.T) {
			repo := &repository{page: page}
			if _, e := (app.AssetService{Enabled: true, Repository: repo}).List(ctx, s, actor, d, app.AssetFilter{Limit: 2}); !errors.Is(e, app.ErrUnavailable) {
				t.Fatal("malformed page escaped", e)
			}
		})
	}
	repo := &repository{page: app.AssetPage{Assets: []app.AssetView{v}, NextID: id(20)}}
	service := app.AssetService{Enabled: true, Repository: repo}
	page, e := service.List(ctx, s, actor, d, app.AssetFilter{Limit: 1})
	if e != nil || page.NextID != id(20) {
		t.Fatal("valid keyset page refused", e)
	}
	page.Assets[0].Profile.Tanks[0] = fleet.Tank{}
	if repo.page.Assets[0].Profile.Tanks[0].Snapshot().Capacity != 275000 {
		t.Fatal("page profile slice escaped")
	}
	repo.page = app.AssetPage{}
	page, e = service.List(ctx, s, actor, d, app.AssetFilter{Limit: 1})
	if e != nil || page.Assets == nil || len(page.Assets) != 0 {
		t.Fatal("empty page shape invented data", e)
	}
}

func TestFleetServiceFailsClosedWhenIdentityGeneratorUnavailable(t *testing.T) {
	s := scope(t)
	ctx, finish, s, actor, d := decision(t, s.OrgID().String(), true)
	defer finish()
	command, e := app.NewAssetWrite(input(t))
	if e != nil {
		t.Fatal(e)
	}
	for _, generate := range []func() (string, error){nil, func() (string, error) { return "bad", nil }, func() (string, error) { return "", errors.New("synthetic source failure") }} {
		repo := &repository{}
		if _, _, e := (app.AssetService{Enabled: true, Repository: repo, NewID: generate}).Write(ctx, s, actor, d, command); !errors.Is(e, app.ErrUnavailable) || repo.writes != 0 {
			t.Fatal("unavailable identity generator reached storage", e)
		}
	}
}

func TestFleetServiceValidatesOriginalCommandResultMetadata(t *testing.T) {
	ctx, finish, s, actor, d := decision(t, id(20), true)
	defer finish()
	for _, kind := range []app.AssetWriteKind{app.ChangeAsset, app.ActivateAsset} {
		t.Run(string(kind), func(t *testing.T) {
			in := app.AssetWriteInput{Kind: kind, OperationID: id(10), AssetID: id(20), ExpectedRevision: 2}
			state := fleet.Active
			if kind == app.ChangeAsset {
				in = input(t)
				in.Kind = kind
				in.AssetID = id(20)
				in.ExpectedRevision = 2
				state = fleet.Draft
			}
			command, e := app.NewAssetWrite(in)
			if e != nil {
				t.Fatal(e)
			}
			repo := &repository{result: app.AssetResult{AssetID: id(20), State: state, Revision: 3}}
			service := app.AssetService{Enabled: true, Repository: repo}
			if got, _, e := service.Write(ctx, s, actor, d, command); e != nil || got.Revision != 3 {
				t.Fatal("valid original metadata refused", e)
			}
			for _, alter := range []func(*app.AssetResult){func(r *app.AssetResult) { r.AssetID = id(21) }, func(r *app.AssetResult) { r.Revision = 2 }, func(r *app.AssetResult) { r.Discarded = true }, func(r *app.AssetResult) { r.State = "UNKNOWN" }} {
				repo.result = app.AssetResult{AssetID: id(20), State: state, Revision: 3}
				alter(&repo.result)
				if got, replay, e := service.Write(ctx, s, actor, d, command); !errors.Is(e, app.ErrUnavailable) || replay || got != (app.AssetResult{}) {
					t.Fatal("uncorrelated result metadata escaped", e)
				}
			}
		})
	}
}

func TestFleetServiceUnavailableAndInvalidReadsReturnNoPrivateResult(t *testing.T) {
	ctx, finish, s, actor, d := decision(t, id(20), false)
	defer finish()
	var e error
	repo := &repository{view: view(t, s, 20), err: app.ErrNotFound}
	service := app.AssetService{Enabled: true, Repository: repo}
	if got, e := service.Read(ctx, s, actor, d, id(20)); !errors.Is(e, app.ErrNotFound) || got.Ref.ID().Value() != "" {
		t.Fatal("failed read exposed partial value", e)
	}
	if _, e = (app.AssetService{}).Read(ctx, s, actor, d, id(20)); !errors.Is(e, app.ErrSurfaceDisabled) {
		t.Fatal("disabled read enabled", e)
	}
	if _, e = service.Read(ctx, s, actor, d, "bad"); !errors.Is(e, fleet.ErrInvalid) {
		t.Fatal("invalid target reached storage", e)
	}
	if _, e = service.Read(context.Background(), s, actor, d, id(20)); !errors.Is(e, app.ErrUnauthenticated) {
		t.Fatal("missing request lease accepted", e)
	}
	if _, e = service.Read(ctx, kernel.OrgScope{}, actor, d, id(20)); !errors.Is(e, fleet.ErrScope) {
		t.Fatal("missing scope accepted", e)
	}
	service.Repository = nil
	if _, e = service.Read(ctx, s, actor, d, id(20)); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("missing read producer accepted", e)
	}
	ctxList, finishList, s, actor, dList := decision(t, s.OrgID().String(), false)
	defer finishList()
	repo.err = app.ErrUnavailable
	repo.page = app.AssetPage{Assets: []app.AssetView{repo.view}}
	service.Repository = repo
	if got, e := service.List(ctxList, s, actor, dList, app.AssetFilter{Limit: 1}); !errors.Is(e, app.ErrUnavailable) || len(got.Assets) != 0 {
		t.Fatal("failed list exposed partial page", e)
	}
	if _, e = (app.AssetService{}).List(ctxList, s, actor, dList, app.AssetFilter{Limit: 1}); !errors.Is(e, app.ErrSurfaceDisabled) {
		t.Fatal("disabled list enabled", e)
	}
	if _, e = service.List(ctxList, s, actor, dList, app.AssetFilter{}); !errors.Is(e, fleet.ErrInvalid) {
		t.Fatal("unbounded list accepted", e)
	}
}

type repository struct {
	writes, reads, lists int
	actor                app.FleetActor
	member, newID        string
	result               app.AssetResult
	view                 app.AssetView
	page                 app.AssetPage
	err                  error
	after                func()
}

func (r *repository) WriteAsset(_ context.Context, _ kernel.OrgScope, a app.FleetActor, member, newID string, _ app.AssetWrite) (app.AssetResult, bool, error) {
	r.writes++
	r.actor, r.member, r.newID = a, member, newID
	if r.after != nil {
		r.after()
	}
	return r.result, false, r.err
}
func (r *repository) ReadAsset(context.Context, kernel.OrgScope, app.FleetActor, string, string) (app.AssetView, error) {
	r.reads++
	if r.after != nil {
		r.after()
	}
	return r.view, r.err
}
func (r *repository) ListAssets(context.Context, kernel.OrgScope, app.FleetActor, string, app.AssetFilter) (app.AssetPage, error) {
	r.lists++
	if r.after != nil {
		r.after()
	}
	return r.page, r.err
}
func view(t *testing.T, s kernel.OrgScope, n int) app.AssetView {
	t.Helper()
	assetID, e := fleet.ParseAssetID(id(n))
	if e != nil {
		t.Fatal(e)
	}
	ref, e := fleet.NewAssetRef(s, assetID)
	if e != nil {
		t.Fatal(e)
	}
	p := input(t).Profile
	p.Category = id(30)
	return app.AssetView{Ref: ref, Profile: p, CategoryLabel: "Caminhão rígido", State: fleet.Draft, Revision: 1}
}

func TestFleetServiceDelegatesOnlyCurrentBoundRequests(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		s := scope(t)
		ctx, finish, s, actor, d := decision(t, s.OrgID().String(), true)
		defer finish()
		repo := &repository{result: app.AssetResult{AssetID: id(60), State: fleet.Draft, Revision: 1}}
		command, e := app.NewAssetWrite(input(t))
		if e != nil {
			t.Fatal(e)
		}
		got, replay, e := (app.AssetService{Enabled: true, Repository: repo, NewID: func() (string, error) { return id(60), nil }}).Write(ctx, s, actor, d, command)
		if e != nil || replay || got.AssetID != id(60) || repo.writes != 1 || repo.member != id(4) || repo.actor != actor || repo.newID != id(60) {
			t.Fatal("current binding not passed exactly", e)
		}
	})
	t.Run("read-and-late-expiry", func(t *testing.T) {
		ctx, finish, s, actor, d := decision(t, id(20), false)
		defer finish()
		repo := &repository{view: view(t, s, 20)}
		service := app.AssetService{Enabled: true, Repository: repo}
		got, e := service.Read(ctx, s, actor, d, id(20))
		if e != nil || got.Ref.ID().Value() != id(20) || repo.reads != 1 {
			t.Fatal("current read denied", e)
		}
		repo.after = finish
		if got, e = service.Read(ctx, s, actor, d, id(20)); !errors.Is(e, app.ErrUnauthenticated) || got.Ref.ID().Value() != "" {
			t.Fatal("ended request exposed late private data", e)
		}
	})
	t.Run("list-and-typed-nil", func(t *testing.T) {
		s := scope(t)
		ctx, finish, s, actor, d := decision(t, s.OrgID().String(), false)
		defer finish()
		repo := &repository{page: app.AssetPage{Assets: []app.AssetView{view(t, s, 20)}}}
		service := app.AssetService{Enabled: true, Repository: repo}
		page, e := service.List(ctx, s, actor, d, app.AssetFilter{Limit: 10})
		if e != nil || len(page.Assets) != 1 || repo.lists != 1 {
			t.Fatal("current list denied", e)
		}
		var absent *repository
		service.Repository = absent
		if _, e = service.List(ctx, s, actor, d, app.AssetFilter{Limit: 10}); !errors.Is(e, app.ErrUnavailable) {
			t.Fatal("typed nil repository not closed", e)
		}
	})
}
