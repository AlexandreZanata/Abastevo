package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

func TestReservePolicyIsFreshReservationManageConfigurationWrite(t *testing.T) {
	p := app.ReservePolicy()
	if p != (access.Policy{Permission: "fleet.reservation.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}) || p.Validate() != nil {
		t.Fatal("reservation policy drift", p)
	}
	for _, other := range []app.FleetAction{app.FleetRead, app.FleetManage, app.FleetAssign, app.FleetReadOwnAssignments} {
		if app.FleetReserve == other {
			t.Fatal("reservation action collides", other)
		}
	}
}

func requestInput() app.ReservationRequestInput {
	return app.ReservationRequestInput{OperationID: id(10), AssetID: id(20), Start: assignmentNow.Add(time.Hour), End: assignmentNow.Add(3 * time.Hour), EmployeeID: id(30), Purpose: "Synthetic delivery round"}
}

// B-BR-EMP-294/298: exact command shapes; each digest binds the whole intent.
func TestReservationAndBlockCommandShapesAndDigests(t *testing.T) {
	s := scope(t)
	base, err := app.NewReservationRequest(requestInput())
	if err != nil {
		t.Fatal(err)
	}
	digests := map[string]bool{base.Digest(s): true}
	for name, mutate := range map[string]func(*app.ReservationRequestInput){
		"confirm":     func(v *app.ReservationRequestInput) { v.Confirm = true },
		"no employee": func(v *app.ReservationRequestInput) { v.EmployeeID = "" },
		"purpose":     func(v *app.ReservationRequestInput) { v.Purpose = "Synthetic other purpose" },
		"end":         func(v *app.ReservationRequestInput) { v.End = v.End.Add(time.Minute) },
	} {
		in := requestInput()
		mutate(&in)
		c, err := app.NewReservationRequest(in)
		if err != nil || len(c.Digest(s)) != 64 || digests[c.Digest(s)] {
			t.Fatal("request digest does not bind", name, err)
		}
		digests[c.Digest(s)] = true
	}
	for name, mutate := range map[string]func(*app.ReservationRequestInput){
		"empty purpose":     func(v *app.ReservationRequestInput) { v.Purpose = " " },
		"long purpose":      func(v *app.ReservationRequestInput) { v.Purpose = string(make([]rune, 281)) },
		"bad employee":      func(v *app.ReservationRequestInput) { v.EmployeeID = "not-an-id" },
		"bad asset":         func(v *app.ReservationRequestInput) { v.AssetID = "" },
		"empty window":      func(v *app.ReservationRequestInput) { v.End = v.Start },
		"reversed window":   func(v *app.ReservationRequestInput) { v.End = v.Start.Add(-time.Minute) },
		"inexact start":     func(v *app.ReservationRequestInput) { v.Start = v.Start.Add(time.Nanosecond) },
		"missing operation": func(v *app.ReservationRequestInput) { v.OperationID = "" },
	} {
		in := requestInput()
		mutate(&in)
		if _, err := app.NewReservationRequest(in); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid request admitted", name, err)
		}
	}
	act := app.ReservationActInput{OperationID: id(11), AssetID: id(20), ReservationID: id(40)}
	confirm, err := app.NewReservationConfirm(act)
	if err != nil || confirm.Command() != fleet.ConfirmReservation {
		t.Fatal(err)
	}
	cancel, err := app.NewReservationCancel(act)
	if err != nil || cancel.Command() != fleet.CancelReservation || cancel.Digest(s) == confirm.Digest(s) {
		t.Fatal("confirm and cancel share an intent", err)
	}
	act.Reason = "Synthetic reason"
	if _, err := app.NewReservationConfirm(act); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("confirmation with a reason")
	}
	if reasoned, err := app.NewReservationCancel(act); err != nil || reasoned.Digest(s) == cancel.Digest(s) {
		t.Fatal("cancel reason not bound", err)
	}
	var zero app.ReservationAct
	if _, err := zero.Snapshot(); !errors.Is(err, fleet.ErrInvalid) || zero.Digest(s) != "" {
		t.Fatal("zero act admitted")
	}
	start, end := assignmentNow.Add(time.Hour), assignmentNow.Add(2*time.Hour)
	block, err := app.NewBlockCreate(app.BlockCreateInput{OperationID: id(12), AssetID: id(20), Reason: fleet.LentOutBlock, Note: "Synthetic loan", Start: &start, End: &end})
	if err != nil || len(block.Digest(s)) != 64 {
		t.Fatal(err)
	}
	for name, in := range map[string]app.BlockCreateInput{
		"unknown reason": {OperationID: id(12), AssetID: id(20), Reason: "OTHER"},
		"reversed":       {OperationID: id(12), AssetID: id(20), Reason: fleet.SafetyBlock, Start: &end, End: &start},
		"control note":   {OperationID: id(12), AssetID: id(20), Reason: fleet.SafetyBlock, Note: "a\x00b"},
	} {
		if _, err := app.NewBlockCreate(in); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid block admitted", name, err)
		}
	}
	// A block removed without a reason is refused (negative case).
	for _, reason := range []string{"", "  "} {
		if _, err := app.NewBlockRemove(app.BlockRemoveInput{OperationID: id(13), AssetID: id(20), BlockID: id(50), Reason: reason}); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("block removal without a reason admitted", reason)
		}
	}
	if remove, err := app.NewBlockRemove(app.BlockRemoveInput{OperationID: id(13), AssetID: id(20), BlockID: id(50), Reason: "Synthetic return"}); err != nil || len(remove.Digest(s)) != 64 {
		t.Fatal(err)
	}
}

func TestReservationConflictRevealsOnlyTheWindow(t *testing.T) {
	from, to := assignmentNow.Add(time.Hour), assignmentNow.Add(2*time.Hour)
	err := app.NewReservationConflict(from, &to)
	var conflict app.ReservationConflict
	if !errors.Is(err, app.ErrReservationConflict) || !errors.As(err, &conflict) || !conflict.From.Equal(from) || conflict.To == nil || !conflict.To.Equal(to) {
		t.Fatal("conflict window", err)
	}
	if open := app.NewReservationConflict(from, nil); !errors.Is(open, app.ErrReservationConflict) {
		t.Fatal("open-ended block window refused")
	}
	if bad := app.NewReservationConflict(to, &from); !errors.Is(bad, app.ErrUnavailable) {
		t.Fatal("reversed conflict window admitted")
	}
}

type reservationRepository struct {
	calls   int
	members []string
	result  app.ReservationResult
	block   app.BlockResult
	page    app.ReservationPage
	blocks  app.BlockPage
	view    app.AvailabilityView
	err     error
}

func (r *reservationRepository) RequestReservation(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member, id string, _ app.ReservationRequest) (app.ReservationResult, bool, error) {
	r.calls++
	r.members = append(r.members, member)
	v := r.result
	if v.ReservationID == "" {
		v.ReservationID = id
	}
	return v, false, r.err
}
func (r *reservationRepository) ActOnReservation(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member string, _ app.ReservationAct) (app.ReservationResult, bool, error) {
	r.calls++
	r.members = append(r.members, member)
	return r.result, true, r.err
}
func (r *reservationRepository) CreateBlock(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member, id string, _ app.BlockCreate) (app.BlockResult, bool, error) {
	r.calls++
	v := r.block
	if v.BlockID == "" {
		v.BlockID = id
	}
	return v, false, r.err
}
func (r *reservationRepository) RemoveBlock(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, _ string, _ app.BlockRemove) (app.BlockResult, bool, error) {
	r.calls++
	return r.block, false, r.err
}
func (r *reservationRepository) ListReservations(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, _, _ string, _ app.PeriodFilter) (app.ReservationPage, error) {
	r.calls++
	return r.page, r.err
}
func (r *reservationRepository) ListBlocks(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, _, _ string, _ app.PeriodFilter) (app.BlockPage, error) {
	r.calls++
	return r.blocks, r.err
}
func (r *reservationRepository) ReadAvailability(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, _, _ string) (app.AvailabilityView, error) {
	r.calls++
	return r.view, r.err
}

func TestReservationServiceUsesFreshReserveAndValidatesResults(t *testing.T) {
	asset := id(20)
	ctx, finish, s, actor, d := policyDecision(t, asset, app.ReservePolicy())
	defer finish()
	repo := &reservationRepository{result: app.ReservationResult{AssetID: asset, State: fleet.ReservationRequested}}
	service := app.ReservationService{Enabled: true, Repository: repo, NewID: sequence(100)}
	command, _ := app.NewReservationRequest(requestInput())
	v, _, err := service.Request(ctx, s, actor, d, command)
	if err != nil || v.ReservationID != id(101) || v.State != fleet.ReservationRequested || repo.members[0] != id(4) {
		t.Fatal("request", v, err)
	}
	// A result in another state than the intent is never returned.
	in := requestInput()
	in.Confirm = true
	confirmed, _ := app.NewReservationRequest(in)
	if _, _, err := service.Request(ctx, s, actor, d, confirmed); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("request answered REQUESTED for a confirming intent", err)
	}
	repo.result = app.ReservationResult{AssetID: asset, ReservationID: id(40), State: fleet.ReservationConfirmed}
	act, _ := app.NewReservationConfirm(app.ReservationActInput{OperationID: id(11), AssetID: asset, ReservationID: id(40)})
	if v, _, err := service.Act(ctx, s, actor, d, act); err != nil || v.State != fleet.ReservationConfirmed {
		t.Fatal("confirm", err)
	}
	cancel, _ := app.NewReservationCancel(app.ReservationActInput{OperationID: id(12), AssetID: asset, ReservationID: id(40)})
	if _, _, err := service.Act(ctx, s, actor, d, cancel); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("cancel answered CONFIRMED", err)
	}
	repo.err = app.NewReservationConflict(assignmentNow.Add(time.Hour), nil)
	if _, _, err := service.Act(ctx, s, actor, d, act); !errors.Is(err, app.ErrReservationConflict) {
		t.Fatal("repository refusal lost", err)
	}
	repo.err = nil
	repo.block = app.BlockResult{AssetID: asset}
	block, _ := app.NewBlockCreate(app.BlockCreateInput{OperationID: id(13), AssetID: asset, Reason: fleet.SafetyBlock})
	if v, _, err := service.Block(ctx, s, actor, d, block); err != nil || v.BlockID != id(103) {
		t.Fatal("block", v, err)
	}
	repo.block = app.BlockResult{AssetID: asset, BlockID: id(51)}
	remove, _ := app.NewBlockRemove(app.BlockRemoveInput{OperationID: id(14), AssetID: asset, BlockID: id(50), Reason: "Synthetic end"})
	if _, _, err := service.Unblock(ctx, s, actor, d, remove); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("another block acknowledged", err)
	}
	if (app.ReservationService{Repository: repo, NewID: sequence(1)}).Enabled {
		t.Fatal("disabled by default")
	}
	if _, _, err := (app.ReservationService{Repository: repo, NewID: sequence(1)}).Request(ctx, s, actor, d, command); !errors.Is(err, app.ErrSurfaceDisabled) {
		t.Fatal("disabled surface", err)
	}
	if _, _, err := (app.ReservationService{Enabled: true, Repository: repo}).Request(ctx, s, actor, d, command); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("missing identifier source", err)
	}
}

// A decision for another policy or asset never reaches the repository.
func TestReservationServiceRefusesAnotherPolicyOrTargetBeforeTheRepository(t *testing.T) {
	asset := id(20)
	command, _ := app.NewReservationRequest(requestInput())
	for name, c := range map[string]struct {
		target string
		policy access.Policy
	}{
		"assign policy":   {asset, app.AssignPolicy()},
		"manage policy":   {asset, app.Policy(true)},
		"read policy":     {asset, app.Policy(false)},
		"other asset":     {id(21), app.ReservePolicy()},
		"organization id": {scope(t).OrgID().String(), app.ReservePolicy()},
	} {
		ctx, finish, s, actor, d := policyDecision(t, c.target, c.policy)
		repo := &reservationRepository{}
		service := app.ReservationService{Enabled: true, Repository: repo, NewID: sequence(1)}
		if _, _, err := service.Request(ctx, s, actor, d, command); !errors.Is(err, app.ErrUnauthenticated) || repo.calls != 0 {
			t.Fatal(name, err, repo.calls)
		}
		finish()
	}
	// Reads use current fleet.asset.read, never the reservation write policy.
	ctx, finish, s, actor, d := policyDecision(t, asset, app.ReservePolicy())
	defer finish()
	repo := &reservationRepository{}
	service := app.ReservationService{Enabled: true, Repository: repo}
	if _, err := service.Availability(ctx, s, actor, d, asset); !errors.Is(err, app.ErrUnauthenticated) || repo.calls != 0 {
		t.Fatal("availability under the write policy", err)
	}
}

func TestReservationServiceReadsValidateEveryRow(t *testing.T) {
	asset := id(20)
	ctx, finish, s, actor, d := policyDecision(t, asset, app.Policy(false))
	defer finish()
	window, _ := fleet.NewReservationWindow(assignmentNow.Add(-2*time.Hour), assignmentNow.Add(time.Hour))
	requested := app.ReservationView{ID: id(40), AssetID: asset, Purpose: "Synthetic purpose", Window: window, Tolerance: time.Hour, State: fleet.ReservationRequested, RequestedAt: assignmentNow.Add(-3 * time.Hour)}
	expired := requested
	expired.State, expired.EndCause, expired.EndedAt = fleet.ReservationExpired, app.EndedByTime, &window.From
	repo := &reservationRepository{page: app.ReservationPage{Reservations: []app.ReservationView{expired}, At: assignmentNow}}
	service := app.ReservationService{Enabled: true, Repository: repo}
	page, err := service.List(ctx, s, actor, d, asset, app.PeriodFilter{Limit: 10})
	if err != nil || len(page.Reservations) != 1 || page.Reservations[0].State != fleet.ReservationExpired {
		t.Fatal("expired row", err)
	}
	for name, bad := range map[string]app.ReservationView{
		"requested without its outcome": requested,
		"expired at the wrong instant":  func() app.ReservationView { v := expired; at := window.To; v.EndedAt = &at; return v }(),
		"other asset":                   func() app.ReservationView { v := expired; v.AssetID = id(21); return v }(),
	} {
		repo.page = app.ReservationPage{Reservations: []app.ReservationView{bad}, At: assignmentNow}
		if name == "requested without its outcome" {
			repo.page.At = window.From.Add(-time.Minute)
			repo.page.Reservations[0].State = fleet.ReservationExpired
		}
		if _, err := service.List(ctx, s, actor, d, asset, app.PeriodFilter{Limit: 10}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid reservation row returned", name, err)
		}
	}
	period, _ := fleet.NewAssignmentPeriod(assignmentNow, nil)
	removedAt := assignmentNow.Add(time.Minute)
	removedEnd := removedAt
	removedPeriod, _ := fleet.NewAssignmentPeriod(assignmentNow, &removedEnd)
	repo.blocks = app.BlockPage{Blocks: []app.BlockView{{ID: id(50), AssetID: asset, Reason: fleet.SafetyBlock, Period: period, RecordedAt: assignmentNow}, {ID: id(51), AssetID: asset, Reason: fleet.SafetyBlock, Period: removedPeriod, RecordedAt: assignmentNow, RemovedAt: &removedAt, RemovalReason: "Synthetic end"}}, At: assignmentNow}
	if page, err := service.Blocks(ctx, s, actor, d, asset, app.PeriodFilter{Limit: 10}); err != nil || len(page.Blocks) != 2 {
		t.Fatal("blocks", err)
	}
	repo.blocks.Blocks[1].RemovalReason = ""
	if _, err := service.Blocks(ctx, s, actor, d, asset, app.PeriodFilter{Limit: 10}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("removed block without its reason returned", err)
	}
	repo.view = app.AvailabilityView{AssetID: asset, At: assignmentNow, Assessment: fleet.Assessment{Availability: fleet.Blocked, Causes: []fleet.Cause{{Availability: fleet.Blocked, Kind: fleet.BlockCause, ID: id(50), Block: fleet.SafetyBlock, From: assignmentNow}}}, MissingSources: []app.AvailabilitySource{app.ActiveUseSource, app.MaintenanceHoldSource}}
	if v, err := service.Availability(ctx, s, actor, d, asset); err != nil || v.Assessment.Availability != fleet.Blocked || len(v.MissingSources) != 2 {
		t.Fatal("availability", err)
	}
	repo.view.Assessment.Availability = fleet.Available
	if _, err := service.Availability(ctx, s, actor, d, asset); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("availability inconsistent with its causes", err)
	}
	repo.view.Assessment = fleet.Assessment{Availability: fleet.Available}
	repo.view.MissingSources = []app.AvailabilitySource{app.MaintenanceHoldSource, app.ActiveUseSource}
	if _, err := service.Availability(ctx, s, actor, d, asset); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unordered missing sources", err)
	}
}
