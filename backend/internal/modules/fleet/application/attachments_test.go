package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// B-BR-EMP-325/326: exact command shapes; each digest binds the whole intent.
func TestAttachmentCommandShapesAndDigests(t *testing.T) {
	s := scope(t)
	start, end := assignmentNow.Add(time.Hour), assignmentNow.Add(2*time.Hour)
	base := app.AttachmentCreateInput{OperationID: id(10), ImplementID: id(20), PoweredAssetID: id(21), Start: &start, End: &end}
	c, err := app.NewAttachmentCreate(base)
	if err != nil || len(c.Digest(s)) != 64 {
		t.Fatal("attach", err)
	}
	open := base
	open.End = nil
	if o, _ := app.NewAttachmentCreate(open); o.Digest(s) == c.Digest(s) {
		t.Fatal("attach digest ignores the end")
	}
	for name, mutate := range map[string]func(*app.AttachmentCreateInput){
		"same asset":    func(v *app.AttachmentCreateInput) { v.PoweredAssetID = v.ImplementID },
		"bad powered":   func(v *app.AttachmentCreateInput) { v.PoweredAssetID = "tractor" },
		"reversed":      func(v *app.AttachmentCreateInput) { v.End = &start; v.Start = &end },
		"inexact start": func(v *app.AttachmentCreateInput) { at := start.Add(time.Nanosecond); v.Start = &at },
		"no operation":  func(v *app.AttachmentCreateInput) { v.OperationID = "" },
	} {
		in := base
		mutate(&in)
		if _, err := app.NewAttachmentCreate(in); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid attach admitted", name, err)
		}
	}
	detach, err := app.NewAttachmentDetach(app.AttachmentDetachInput{OperationID: id(11), AssetID: id(20), AttachmentID: id(40), End: &end, Reason: "Synthetic harrow returned"})
	if err != nil || len(detach.Digest(s)) != 64 {
		t.Fatal("detach", err)
	}
	if _, err := app.NewAttachmentDetach(app.AttachmentDetachInput{OperationID: id(11), AssetID: id(20), AttachmentID: id(40), Reason: " "}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("blank detach reason admitted", err)
	}
}

type attachmentRepository struct {
	calls  int
	result app.AttachmentResult
	page   app.AttachmentPage
	err    error
}

func (r *attachmentRepository) AttachImplement(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, _, id string, _ app.AttachmentCreate) (app.AttachmentResult, bool, error) {
	r.calls++
	v := r.result
	if v.AttachmentID == "" {
		v.AttachmentID = id
	}
	return v, false, r.err
}
func (r *attachmentRepository) DetachImplement(context.Context, kernel.OrgScope, app.FleetActor, string, app.AttachmentDetach) (app.AttachmentResult, bool, error) {
	r.calls++
	return r.result, true, r.err
}
func (r *attachmentRepository) ListAttachments(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.PeriodFilter) (app.AttachmentPage, error) {
	r.calls++
	return r.page, r.err
}

// Commands need fresh fleet.asset.manage on the path asset (UC-FLEET-10);
// another policy or asset never reaches the repository; results are checked.
func TestAttachmentServiceUsesFreshManageAndValidatesResults(t *testing.T) {
	implement := id(20)
	command, _ := app.NewAttachmentCreate(app.AttachmentCreateInput{OperationID: id(10), ImplementID: implement, PoweredAssetID: id(21)})
	for name, c := range map[string]struct {
		target string
		write  bool
	}{"read policy": {implement, false}, "powered asset target": {id(21), true}} {
		ctx, finish, s, actor, d := policyDecision(t, c.target, app.Policy(c.write))
		repo := &attachmentRepository{}
		if _, _, err := (app.AttachmentService{Enabled: true, Repository: repo, NewID: sequence(1)}).Attach(ctx, s, actor, d, command); !errors.Is(err, app.ErrUnauthenticated) || repo.calls != 0 {
			t.Fatal(name, err)
		}
		finish()
	}
	ctx, finish, s, actor, d := policyDecision(t, implement, app.Policy(true))
	defer finish()
	repo := &attachmentRepository{result: app.AttachmentResult{AssetID: implement}}
	service := app.AttachmentService{Enabled: true, Repository: repo, NewID: sequence(100)}
	if v, _, err := service.Attach(ctx, s, actor, d, command); err != nil || v.AttachmentID != id(101) {
		t.Fatal("attach", v, err)
	}
	repo.err = app.ErrAttachmentOverlap
	if _, _, err := service.Attach(ctx, s, actor, d, command); !errors.Is(err, app.ErrAttachmentOverlap) {
		t.Fatal("overlap refusal lost", err)
	}
	repo.err, repo.result = nil, app.AttachmentResult{AssetID: implement, AttachmentID: id(41)}
	detach, _ := app.NewAttachmentDetach(app.AttachmentDetachInput{OperationID: id(11), AssetID: implement, AttachmentID: id(40)})
	if _, _, err := service.Detach(ctx, s, actor, d, detach); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("another attachment acknowledged", err)
	}
	if _, _, err := (app.AttachmentService{Repository: repo}).Attach(ctx, s, actor, d, command); !errors.Is(err, app.ErrSurfaceDisabled) {
		t.Fatal("disabled surface", err)
	}
}

func TestAttachmentServiceListValidatesEveryRow(t *testing.T) {
	asset := id(20)
	ctx, finish, s, actor, d := policyDecision(t, asset, app.Policy(false))
	defer finish()
	period, _ := fleet.NewAssignmentPeriod(assignmentNow.Add(-time.Hour), nil)
	row := app.AttachmentView{ID: id(40), ImplementID: asset, PoweredAssetID: id(21), Period: period, State: fleet.AssignmentActive, RecordedAt: assignmentNow.Add(-time.Hour)}
	repo := &attachmentRepository{page: app.AttachmentPage{Attachments: []app.AttachmentView{row}, At: assignmentNow}}
	service := app.AttachmentService{Enabled: true, Repository: repo}
	if page, err := service.List(ctx, s, actor, d, asset, app.PeriodFilter{Limit: 10}); err != nil || len(page.Attachments) != 1 {
		t.Fatal("list", err)
	}
	for name, bad := range map[string]app.AttachmentView{
		"unrelated asset": func() app.AttachmentView { v := row; v.ImplementID = id(22); return v }(),
		"wrong state":     func() app.AttachmentView { v := row; v.State = fleet.AssignmentScheduled; return v }(),
		"detached open":   func() app.AttachmentView { v := row; at := assignmentNow; v.DetachedAt = &at; return v }(),
	} {
		repo.page = app.AttachmentPage{Attachments: []app.AttachmentView{bad}, At: assignmentNow}
		if _, err := service.List(ctx, s, actor, d, asset, app.PeriodFilter{Limit: 10}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid attachment row returned", name, err)
		}
	}
}
