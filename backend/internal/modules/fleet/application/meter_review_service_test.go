package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

type reviewRepository struct {
	calls      int
	after      func()
	view       app.MeterReviewView
	page       app.MeterReviewPage
	resolution protocol.ReviewResolution
	err        error
	member     string
}

func TestMeterReviewViewRefusesUnsetServerReceipt(t *testing.T) {
	view := reviewView(t, 30)
	view.ReceivedAt = time.Time{}
	if view.Validate() == nil {
		t.Fatal("unset server receipt became a private review")
	}
}

func TestMeterReviewViewRetainsOnlyDeliveredOwnerForms(t *testing.T) {
	for name, edit := range map[string]func(*app.MeterReviewView){"other-origin": func(v *app.MeterReviewView) { v.Fact.Origin = meter.Fueling }, "future-method": func(v *app.MeterReviewView) { v.Fact.Method = meter.TypedWithPhoto }} {
		t.Run(name, func(t *testing.T) {
			view := reviewView(t, 30)
			edit(&view)
			if view.Validate() == nil {
				t.Fatal("undelivered held form admitted")
			}
		})
	}
}

func (r *reviewRepository) called(member string) {
	r.calls++
	r.member = member
	if r.after != nil {
		r.after()
	}
}
func (r *reviewRepository) ListMeterReviews(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member string, _ app.MeterReviewFilter) (app.MeterReviewPage, error) {
	r.called(member)
	return r.page, r.err
}
func (r *reviewRepository) ReadMeterReview(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member, _ string) (app.MeterReviewView, error) {
	r.called(member)
	return r.view, r.err
}
func (r *reviewRepository) ResolveMeterReview(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member string, _ app.MeterReviewDecision) (protocol.ReviewResolution, bool, error) {
	r.called(member)
	return r.resolution, true, r.err
}

func reviewDecision(t *testing.T, target string, write bool) (context.Context, func(), kernel.OrgScope, app.FleetActor, access.Decision) {
	t.Helper()
	s := scope(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	role, e := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: id(6), MembershipID: id(4), AssignedBy: id(4), Role: permissions.Owner, Scope: permissions.OrgScope, TargetID: s.OrgID().String(), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic review authority", Revision: 1})
	if e != nil {
		t.Fatal(e)
	}
	a := access.Authorization{Source: currentAuthority{value: access.Authority{OrgID: s.OrgID(), AccountID: id(2), MembershipID: id(4), EmployeeID: id(5), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: syntheticCapability{}, Records: syntheticRecordCheck{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	prepared, e := a.Prepare(ctx, s, access.Actor{AccountID: id(2), Channel: "web"}, app.MeterReviewPolicy(write), resource{id: target})
	if e != nil {
		t.Fatal(e)
	}
	d, e := a.Complete(ctx, prepared, syntheticProof{})
	if e != nil {
		t.Fatal(e)
	}
	return ctx, finish, s, app.FleetActor{AccountID: id(2), SessionID: id(3)}, d
}
func reviewView(t *testing.T, n int) app.MeterReviewView {
	t.Helper()
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	v, e := meter.NewValue(meter.Odometer, 123, nil)
	if e != nil {
		t.Fatal(e)
	}
	return app.MeterReviewView{ReviewItemID: id(n), OperationID: id(n + 100), PayloadDigest: "sha256:" + strings.Repeat("a", 64), Code: "PERMISSION_CHANGED", State: app.MeterReviewOpen, Revision: 1, AuthorAccountID: id(42), AuthorMembershipID: id(44), AuthorEmployeeID: id(45), EnrollmentID: id(46), ReceivedAt: now, ExpiresAt: now.Add(60 * 24 * time.Hour), Fact: app.MeterReadingInput{OperationID: id(n + 100), AssetID: id(20), MeterID: id(21), SegmentID: id(22), ReadingID: id(n + 200), Value: v, Declared: orgtime.Declaration{OccurredAt: now.Add(-time.Hour), EntryZone: "UTC"}, Origin: meter.Standalone, Method: meter.Typed}}
}
func reviewResolution(t *testing.T, v app.MeterReviewView, commandID string) protocol.ReviewResolution {
	t.Helper()
	revision := int64(1)
	r, e := protocol.NewReviewResolution(protocol.ReviewResolutionSnapshot{OperationID: v.OperationID, PayloadDigest: v.PayloadDigest, ReviewItemID: v.ReviewItemID, ResolutionID: commandID, ReviewRevision: 2, Outcome: protocol.ReviewAccepted, Reason: "", ResolvedAt: orgtime.FormatInstant(v.ReceivedAt.Add(time.Hour)), EntityRevision: &revision})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestMeterReviewServicePublishesOnlyWholeValidPrivatePages(t *testing.T) {
	s := scope(t)
	ctx, finish, _, actor, d := reviewDecision(t, s.OrgID().String(), false)
	defer finish()
	r := &reviewRepository{page: app.MeterReviewPage{Items: []app.MeterReviewView{reviewView(t, 30), reviewView(t, 31)}, NextID: id(31)}}
	service := app.MeterReviewService{Enabled: true, Repository: r}
	page, e := service.List(ctx, s, actor, d, app.MeterReviewFilter{Limit: 2})
	if e != nil || len(page.Items) != 2 || r.member != id(4) {
		t.Fatal("valid current private page", e)
	}
	page.Items[0].Code = "aliased"
	if r.page.Items[0].Code != "PERMISSION_CHANGED" {
		t.Fatal("private page aliases repository")
	}
	base := r.page
	for name, edit := range map[string]func(*app.MeterReviewPage){"later-invalid": func(p *app.MeterReviewPage) { p.Items[1].Fact.MeterID = "" }, "unsorted": func(p *app.MeterReviewPage) { p.Items[1] = p.Items[0] }, "cursor": func(p *app.MeterReviewPage) { p.NextID = id(32) }, "unproven-resolution": func(p *app.MeterReviewPage) { p.Items[1].State = app.MeterReviewAccepted; p.Items[1].Revision = 2 }, "wrong-state-filter": func(p *app.MeterReviewPage) { p.Items[1].State = app.MeterReviewExpired }, "receipt": func(p *app.MeterReviewPage) { p.Items[1].ExpiresAt = p.Items[1].ReceivedAt }} {
		t.Run(name, func(t *testing.T) {
			p := base
			p.Items = append([]app.MeterReviewView(nil), base.Items...)
			edit(&p)
			r.page = p
			got, e := service.List(ctx, s, actor, d, app.MeterReviewFilter{Limit: 2, State: app.MeterReviewOpen})
			if !errors.Is(e, app.ErrUnavailable) || len(got.Items) != 0 {
				t.Fatal("invalid suffix leaked private page", e)
			}
		})
	}
}
func TestMeterReviewServiceRetainsExactResolutionAndFencesCurrentProof(t *testing.T) {
	ctx, finish, s, actor, d := reviewDecision(t, id(30), true)
	defer finish()
	command, e := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: id(50), ReviewItemID: id(30), ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if e != nil {
		t.Fatal(e)
	}
	r := &reviewRepository{view: reviewView(t, 30)}
	r.resolution = reviewResolution(t, r.view, id(50))
	service := app.MeterReviewService{Enabled: true, Repository: r}
	value, replay, e := service.Resolve(ctx, s, actor, d, command)
	if e != nil || !replay || string(value.CanonicalBytes()) != string(r.resolution.CanonicalBytes()) {
		t.Fatal("exact resolution replay lost", e)
	}
	for name, edit := range map[string]func(*protocol.ReviewResolutionSnapshot){"wrong-item": func(v *protocol.ReviewResolutionSnapshot) { v.ReviewItemID = id(99) }, "wrong-command": func(v *protocol.ReviewResolutionSnapshot) { v.ResolutionID = id(99) }, "changed-reason": func(v *protocol.ReviewResolutionSnapshot) { v.Reason = "changed" }, "opposite": func(v *protocol.ReviewResolutionSnapshot) {
		v.Outcome = protocol.ReviewRefused
		v.EntityRevision = nil
		v.Reason = "different"
	}} {
		t.Run(name, func(t *testing.T) {
			v := reviewResolution(t, r.view, id(50)).Snapshot()
			edit(&v)
			r.resolution, e = protocol.NewReviewResolution(v)
			if e != nil {
				t.Fatal(e)
			}
			got, _, e := service.Resolve(ctx, s, actor, d, command)
			if !errors.Is(e, app.ErrUnavailable) || len(got.CanonicalBytes()) != 0 {
				t.Fatal("different resolution published", e)
			}
		})
	}
	r.resolution = reviewResolution(t, r.view, id(50))
	r.after = finish
	if _, _, e := service.Resolve(ctx, s, actor, d, command); !errors.Is(e, app.ErrUnauthenticated) {
		t.Fatal("ended request proof leaked resolution", e)
	}
}
func TestMeterReviewServiceRequiresOwnerPolicyBeforePrivateRead(t *testing.T) {
	ctx, finish, s, actor, d := decision(t, id(30), false)
	defer finish()
	r := &reviewRepository{view: reviewView(t, 30)}
	service := app.MeterReviewService{Enabled: true, Repository: r}
	if _, e := service.Read(ctx, s, actor, d, id(30)); !errors.Is(e, app.ErrUnauthenticated) || r.calls != 0 {
		t.Fatal("fleet.read proof reached meter review source", e)
	}
	ctx2, finish2, _, _, rd := reviewDecision(t, id(30), false)
	defer finish2()
	if _, e := service.Read(ctx2, s, actor, rd, id(30)); e != nil {
		t.Fatal("real owner read refused", e)
	}
	r.view.ReviewItemID = id(31)
	if _, e := service.Read(ctx2, s, actor, rd, id(30)); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("rebound private item published", e)
	}
	service.Enabled = false
	if _, e := service.Read(ctx2, s, actor, rd, id(30)); !errors.Is(e, app.ErrSurfaceDisabled) {
		t.Fatal("closed surface reached source", e)
	}
}
