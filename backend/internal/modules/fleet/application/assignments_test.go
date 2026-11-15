package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
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

var assignmentNow = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func policyDecision(t *testing.T, target string, policy access.Policy) (context.Context, func(), kernel.OrgScope, app.FleetActor, access.Decision) {
	t.Helper()
	s := scope(t)
	role, e := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: id(6), MembershipID: id(4), AssignedBy: id(4), Role: permissions.Owner, Scope: permissions.OrgScope, TargetID: s.OrgID().String(), ValidFrom: assignmentNow.Add(-time.Hour), Reason: "Synthetic assignment fixture", Revision: 1})
	if e != nil {
		t.Fatal(e)
	}
	a := access.Authorization{Source: currentAuthority{value: access.Authority{OrgID: s.OrgID(), AccountID: id(2), MembershipID: id(4), EmployeeID: id(5), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: syntheticCapability{}, Records: syntheticRecordCheck{}, Now: func() time.Time { return assignmentNow }}
	ctx, finish := access.BeginRequest(t.Context())
	prepared, e := a.Prepare(ctx, s, access.Actor{AccountID: id(2), Channel: "web"}, policy, resource{id: target})
	if e != nil {
		t.Fatal(e)
	}
	d, e := a.Complete(ctx, prepared, syntheticProof{})
	if e != nil {
		t.Fatal(e)
	}
	return ctx, finish, s, app.FleetActor{AccountID: id(2), SessionID: id(3)}, d
}

func createInput() app.AssignmentCreateInput {
	return app.AssignmentCreateInput{OperationID: id(10), AssetID: id(20), Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{id(30), id(31)}}
}

func TestAssignPolicyIsFreshAssignConfigurationWrite(t *testing.T) {
	p := app.AssignPolicy()
	if p != (access.Policy{Permission: "fleet.asset.assign", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}) || p.Validate() != nil {
		t.Fatal("assignment policy drift", p)
	}
	if app.FleetAssign == app.FleetRead || app.FleetAssign == app.FleetManage || app.FleetReadOwnAssignments == app.FleetAssign {
		t.Fatal("assignment actions collide with existing fleet actions")
	}
}

// B-BR-EMP-243/244: exact command shape; the digest binds the whole intent.
func TestAssignmentCreateCommandShapeAndDigest(t *testing.T) {
	s := scope(t)
	start, end := assignmentNow.Add(time.Hour), assignmentNow.Add(49*time.Hour)
	valid := []app.AssignmentCreateInput{
		createInput(),
		{OperationID: id(10), AssetID: id(20), Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{id(30)}, ReplacePrimary: true},
		{OperationID: id(10), AssetID: id(20), Kind: fleet.TemporaryAssignment, EmployeeIDs: []string{id(30)}, Start: &start, End: &end, Reason: "Synthetic holiday cover"},
		{OperationID: id(10), AssetID: id(20), Kind: fleet.TemporaryAssignment, EmployeeIDs: []string{id(30)}, End: &end},
	}
	for i, in := range valid {
		c, err := app.NewAssignmentCreate(in)
		if err != nil {
			t.Fatal("valid create refused", i, err)
		}
		if c.Digest(s) == "" || len(c.Digest(s)) != 64 {
			t.Fatal("create digest missing", i)
		}
	}
	tooMany := make([]string, fleet.MaxAssignees+1)
	for i := range tooMany {
		tooMany[i] = id(1000 + i)
	}
	early := start.Add(-2 * time.Hour)
	inexact := start.Add(time.Nanosecond)
	for name, edit := range map[string]func(*app.AssignmentCreateInput){
		"operation":           func(v *app.AssignmentCreateInput) { v.OperationID = "x" },
		"asset":               func(v *app.AssignmentCreateInput) { v.AssetID = strings.ToUpper(id(20)) },
		"kind":                func(v *app.AssignmentCreateInput) { v.Kind = "POOL" },
		"no employee":         func(v *app.AssignmentCreateInput) { v.EmployeeIDs = nil },
		"employee syntax":     func(v *app.AssignmentCreateInput) { v.EmployeeIDs = []string{"employee-1"} },
		"duplicate employee":  func(v *app.AssignmentCreateInput) { v.EmployeeIDs = []string{id(30), id(30)} },
		"too many":            func(v *app.AssignmentCreateInput) { v.EmployeeIDs = tooMany },
		"two primaries":       func(v *app.AssignmentCreateInput) { v.Kind = fleet.PrimaryAssignment },
		"temporary no end":    func(v *app.AssignmentCreateInput) { v.Kind = fleet.TemporaryAssignment },
		"end before start":    func(v *app.AssignmentCreateInput) { v.Start, v.End = &start, &early },
		"empty period":        func(v *app.AssignmentCreateInput) { v.Start, v.End = &start, &start },
		"inexact start":       func(v *app.AssignmentCreateInput) { v.Start = &inexact },
		"zero start":          func(v *app.AssignmentCreateInput) { v.Start = &time.Time{} },
		"replace additional":  func(v *app.AssignmentCreateInput) { v.ReplacePrimary = true },
		"reason control":      func(v *app.AssignmentCreateInput) { v.Reason = "bad\x00reason" },
		"reason too long":     func(v *app.AssignmentCreateInput) { v.Reason = strings.Repeat("a", 281) },
		"reason invalid utf8": func(v *app.AssignmentCreateInput) { v.Reason = string([]byte{0xff}) },
	} {
		in := createInput()
		edit(&in)
		if _, err := app.NewAssignmentCreate(in); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid create accepted", name, err)
		}
	}
	// Non-aliasing snapshot; the same instant from another zone is the same intent.
	in := valid[2]
	in.EmployeeIDs = []string{id(30)}
	zoned := start.In(time.FixedZone("BRT", -3*3600))
	ownEnd := end
	in.Start, in.End = &zoned, &ownEnd
	c, err := app.NewAssignmentCreate(in)
	if err != nil {
		t.Fatal(err)
	}
	in.EmployeeIDs[0] = id(99)
	*in.End = end.Add(time.Hour)
	zoned = zoned.Add(time.Hour)
	got, err := c.Snapshot()
	if err != nil || got.EmployeeIDs[0] != id(30) || !got.End.Equal(end) || !got.Start.Equal(start) || got.Start.Location() != time.UTC {
		t.Fatal("create snapshot aliases caller data", got, err)
	}
	same, _ := app.NewAssignmentCreate(valid[2])
	if same.Digest(s) != c.Digest(s) {
		t.Fatal("zone changed the intent digest")
	}
	base, _ := app.NewAssignmentCreate(createInput())
	for name, edit := range map[string]func(*app.AssignmentCreateInput){
		"order":  func(v *app.AssignmentCreateInput) { v.EmployeeIDs = []string{id(31), id(30)} },
		"kind":   func(v *app.AssignmentCreateInput) { v.Kind = fleet.TemporaryAssignment; v.End = &end },
		"reason": func(v *app.AssignmentCreateInput) { v.Reason = "Synthetic" },
		"start":  func(v *app.AssignmentCreateInput) { v.Start = &start },
		"asset":  func(v *app.AssignmentCreateInput) { v.AssetID = id(21) },
	} {
		in := createInput()
		edit(&in)
		other, err := app.NewAssignmentCreate(in)
		if err != nil || other.Digest(s) == base.Digest(s) {
			t.Fatal("digest ignores intent field", name, err)
		}
	}
	org, _ := kernel.ParseOrgID(id(98))
	otherScope, _ := kernel.NewOrgScope(org)
	if base.Digest(otherScope) == base.Digest(s) || (app.AssignmentCreate{}).Digest(s) != "" {
		t.Fatal("digest not scoped or zero command digested")
	}
}

func TestAssignmentEndCommandShapeAndDigest(t *testing.T) {
	s := scope(t)
	c, err := app.NewAssignmentEnd(app.AssignmentEndInput{OperationID: id(11), AssetID: id(20), AssignmentID: id(40)})
	if err != nil || len(c.Digest(s)) != 64 {
		t.Fatal("valid end refused", err)
	}
	withReason, err := app.NewAssignmentEnd(app.AssignmentEndInput{OperationID: id(11), AssetID: id(20), AssignmentID: id(40), Reason: "Synthetic return"})
	if err != nil || withReason.Digest(s) == c.Digest(s) {
		t.Fatal("end reason not bound", err)
	}
	for name, in := range map[string]app.AssignmentEndInput{
		"operation":  {OperationID: "", AssetID: id(20), AssignmentID: id(40)},
		"asset":      {OperationID: id(11), AssetID: "x", AssignmentID: id(40)},
		"assignment": {OperationID: id(11), AssetID: id(20), AssignmentID: "x"},
		"reason":     {OperationID: id(11), AssetID: id(20), AssignmentID: id(40), Reason: strings.Repeat("a", 281)},
	} {
		if _, err := app.NewAssignmentEnd(in); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid end accepted", name, err)
		}
	}
}

func TestAssignmentFiltersAreBounded(t *testing.T) {
	from := assignmentNow
	if (app.AssignmentFilter{Limit: 1}).Validate() != nil || (app.AssignmentFilter{Limit: 100, AfterFrom: &from, AfterID: id(40)}).Validate() != nil {
		t.Fatal("valid filter refused")
	}
	inexact := from.Add(time.Nanosecond)
	for name, f := range map[string]app.AssignmentFilter{"zero": {}, "large": {Limit: 101}, "half cursor": {Limit: 5, AfterFrom: &from}, "id only": {Limit: 5, AfterID: id(40)}, "bad id": {Limit: 5, AfterFrom: &from, AfterID: "x"}, "inexact": {Limit: 5, AfterFrom: &inexact, AfterID: id(40)}} {
		if !errors.Is(f.Validate(), fleet.ErrInvalid) {
			t.Fatal("invalid filter accepted", name)
		}
	}
	if (app.OwnAssignmentFilter{Limit: 100, AfterID: id(40)}).Validate() != nil {
		t.Fatal("valid own filter refused")
	}
	for _, f := range []app.OwnAssignmentFilter{{}, {Limit: 101}, {Limit: 3, AfterID: "x"}} {
		if !errors.Is(f.Validate(), fleet.ErrInvalid) {
			t.Fatal("invalid own filter accepted", f)
		}
	}
}

type assignmentRepository struct {
	result   app.AssignmentResult
	page     app.AssignmentPage
	err      error
	ids      []string
	member   string
	asset    string
	calls    int
	replayed bool
}

func (r *assignmentRepository) CreateAssignments(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member string, ids []string, _ app.AssignmentCreate) (app.AssignmentResult, bool, error) {
	r.calls++
	r.member, r.ids = member, slices.Clone(ids)
	return r.result, r.replayed, r.err
}
func (r *assignmentRepository) EndAssignment(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member string, _ app.AssignmentEnd) (app.AssignmentResult, bool, error) {
	r.calls++
	r.member = member
	return r.result, r.replayed, r.err
}
func (r *assignmentRepository) ListAssetAssignments(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member, asset string, _ app.AssignmentFilter) (app.AssignmentPage, error) {
	r.calls++
	r.member, r.asset = member, asset
	return r.page, r.err
}
func (r *assignmentRepository) ListOwnAssignments(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member string, _ app.OwnAssignmentFilter) (app.AssignmentPage, error) {
	r.calls++
	r.member = member
	return r.page, r.err
}

func sequence(start int) func() (string, error) {
	n := start
	return func() (string, error) { n++; return id(n), nil }
}

func TestAssignmentServiceCreateUsesFreshAssignAndValidatesResults(t *testing.T) {
	command, err := app.NewAssignmentCreate(createInput())
	if err != nil {
		t.Fatal(err)
	}
	ctx, finish, s, actor, d := policyDecision(t, id(20), app.AssignPolicy())
	defer finish()
	repo := &assignmentRepository{result: app.AssignmentResult{AssetID: id(20), AssignmentIDs: []string{id(501), id(502)}}}
	service := app.AssignmentService{Enabled: true, Repository: repo, NewID: sequence(500)}
	got, replayed, err := service.Create(ctx, s, actor, d, command)
	if err != nil || replayed || !slices.Equal(got.AssignmentIDs, []string{id(501), id(502)}) || !slices.Equal(repo.ids, []string{id(501), id(502)}) || repo.member != id(4) {
		t.Fatal("create through service", got, err, repo.ids)
	}
	got.AssignmentIDs[0] = id(9)
	for name, result := range map[string]app.AssignmentResult{
		"count":             {AssetID: id(20), AssignmentIDs: []string{id(501)}},
		"asset":             {AssetID: id(21), AssignmentIDs: []string{id(501), id(502)}},
		"duplicate":         {AssetID: id(20), AssignmentIDs: []string{id(501), id(501)}},
		"syntax":            {AssetID: id(20), AssignmentIDs: []string{id(501), "x"}},
		"replaced unwanted": {AssetID: id(20), AssignmentIDs: []string{id(501), id(502)}, ReplacedID: id(503)},
	} {
		repo := &assignmentRepository{result: result}
		if _, _, err := (app.AssignmentService{Enabled: true, Repository: repo, NewID: sequence(500)}).Create(ctx, s, actor, d, command); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid repository result published", name, err)
		}
	}
	for name, svc := range map[string]app.AssignmentService{
		"no repository": {Enabled: true, NewID: sequence(500)},
		"no ids":        {Enabled: true, Repository: &assignmentRepository{}},
		"bad ids":       {Enabled: true, Repository: &assignmentRepository{}, NewID: func() (string, error) { return "x", nil }},
		"failing ids":   {Enabled: true, Repository: &assignmentRepository{}, NewID: func() (string, error) { return "", errors.New("synthetic") }},
	} {
		if _, _, err := svc.Create(ctx, s, actor, d, command); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("unavailable source accepted", name, err)
		}
	}
	if _, _, err := (app.AssignmentService{Repository: repo}).Create(ctx, s, actor, d, command); !errors.Is(err, app.ErrSurfaceDisabled) {
		t.Fatal("disabled surface", err)
	}
	overlap := &assignmentRepository{err: app.ErrAssignmentOverlap}
	if _, _, err := (app.AssignmentService{Enabled: true, Repository: overlap, NewID: sequence(500)}).Create(ctx, s, actor, d, command); !errors.Is(err, app.ErrAssignmentOverlap) {
		t.Fatal("overlap lost", err)
	}
	replace, _ := app.NewAssignmentCreate(app.AssignmentCreateInput{OperationID: id(10), AssetID: id(20), Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{id(30)}, ReplacePrimary: true})
	replaced := &assignmentRepository{result: app.AssignmentResult{AssetID: id(20), AssignmentIDs: []string{id(501)}, ReplacedID: id(450)}, replayed: true}
	if v, replayed, err := (app.AssignmentService{Enabled: true, Repository: replaced, NewID: sequence(500)}).Create(ctx, s, actor, d, replace); err != nil || !replayed || v.ReplacedID != id(450) {
		t.Fatal("replace result", v, err)
	}
	if _, _, err := (app.AssignmentService{Enabled: true, Repository: &assignmentRepository{result: app.AssignmentResult{AssetID: id(20), AssignmentIDs: []string{id(501)}, ReplacedID: id(501)}}, NewID: sequence(500)}).Create(ctx, s, actor, d, replace); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("self-replacement published", err)
	}
}

func TestAssignmentServiceRefusesAnotherPolicyOrTargetBeforeTheRepository(t *testing.T) {
	command, _ := app.NewAssignmentCreate(createInput())
	end, _ := app.NewAssignmentEnd(app.AssignmentEndInput{OperationID: id(11), AssetID: id(20), AssignmentID: id(40)})
	for name, tc := range map[string]struct {
		target string
		policy access.Policy
	}{
		"manage decision": {id(20), app.Policy(true)},
		"read decision":   {id(20), app.Policy(false)},
		"other asset":     {id(21), app.AssignPolicy()},
	} {
		ctx, finish, s, actor, d := policyDecision(t, tc.target, tc.policy)
		repo := &assignmentRepository{}
		service := app.AssignmentService{Enabled: true, Repository: repo, NewID: sequence(500)}
		if _, _, err := service.Create(ctx, s, actor, d, command); !errors.Is(err, app.ErrUnauthenticated) {
			t.Fatal("create with foreign decision", name, err)
		}
		if _, _, err := service.End(ctx, s, actor, d, end); !errors.Is(err, app.ErrUnauthenticated) || repo.calls != 0 {
			t.Fatal("end with foreign decision", name, err)
		}
		finish()
	}
	ctx, finish, s, actor, d := policyDecision(t, id(20), app.Policy(false))
	defer finish()
	repo := &assignmentRepository{}
	if _, err := (app.AssignmentService{Enabled: true, Repository: repo}).Mine(ctx, s, actor, d, app.OwnAssignmentFilter{Limit: 5}); !errors.Is(err, app.ErrUnauthenticated) || repo.calls != 0 {
		t.Fatal("own list accepted an asset decision", err)
	}
}

func assignmentView(n int, asset string, kind fleet.AssignmentKind, from time.Time, to *time.Time, at time.Time) app.AssignmentView {
	p, err := fleet.NewAssignmentPeriod(from, to)
	if err != nil {
		panic(err)
	}
	v := app.AssignmentView{ID: id(n), AssetID: asset, EmployeeID: id(30), Kind: kind, Period: p, State: p.StateAt(at), RecordedAt: from}
	if p.Empty() {
		v.EndCause, v.EndedAt = app.EndedByCommand, &from
	}
	return v
}

func TestAssignmentServiceEndAndListsValidateEveryRow(t *testing.T) {
	end, _ := app.NewAssignmentEnd(app.AssignmentEndInput{OperationID: id(11), AssetID: id(20), AssignmentID: id(40)})
	ctx, finish, s, actor, d := policyDecision(t, id(20), app.AssignPolicy())
	repo := &assignmentRepository{result: app.AssignmentResult{AssetID: id(20), AssignmentIDs: []string{id(40)}}}
	if v, _, err := (app.AssignmentService{Enabled: true, Repository: repo}).End(ctx, s, actor, d, end); err != nil || v.AssignmentIDs[0] != id(40) {
		t.Fatal("end through service", err)
	}
	for _, result := range []app.AssignmentResult{{AssetID: id(20), AssignmentIDs: []string{id(41)}}, {AssetID: id(20), AssignmentIDs: []string{id(40)}, ReplacedID: id(42)}, {AssetID: id(20)}} {
		if _, _, err := (app.AssignmentService{Enabled: true, Repository: &assignmentRepository{result: result}}).End(ctx, s, actor, d, end); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid end result published", result, err)
		}
	}
	ended := &assignmentRepository{err: fleet.ErrAssignmentEnded}
	if _, _, err := (app.AssignmentService{Enabled: true, Repository: ended}).End(ctx, s, actor, d, end); !errors.Is(err, fleet.ErrAssignmentEnded) {
		t.Fatal("ended refusal lost", err)
	}
	finish()

	ctx, finish, s, actor, d = policyDecision(t, id(20), app.Policy(false))
	defer finish()
	at := assignmentNow
	later := at.Add(time.Hour)
	rows := []app.AssignmentView{
		assignmentView(41, id(20), fleet.PrimaryAssignment, at.Add(-time.Hour), &at, at),
		assignmentView(42, id(20), fleet.AdditionalAssignment, at.Add(-time.Hour), nil, at),
		assignmentView(43, id(20), fleet.TemporaryAssignment, later, func() *time.Time { v := later.Add(time.Hour); return &v }(), at),
	}
	repo = &assignmentRepository{page: app.AssignmentPage{Assignments: rows, At: at}}
	page, err := (app.AssignmentService{Enabled: true, Repository: repo}).List(ctx, s, actor, d, id(20), app.AssignmentFilter{Limit: 3})
	if err != nil || len(page.Assignments) != 3 || page.Assignments[0].State != fleet.AssignmentEnded || page.Assignments[1].State != fleet.AssignmentActive || page.Assignments[2].State != fleet.AssignmentScheduled || repo.asset != id(20) {
		t.Fatal("asset assignment page", err, page)
	}
	page.Assignments[0].Period.To = nil
	if again, _ := (app.AssignmentService{Enabled: true, Repository: repo}).List(ctx, s, actor, d, id(20), app.AssignmentFilter{Limit: 3}); again.Assignments[0].Period.To == nil {
		t.Fatal("page aliases repository rows")
	}
	bad := func(edit func(*app.AssignmentPage)) app.AssignmentPage {
		p := app.AssignmentPage{Assignments: []app.AssignmentView{assignmentView(41, id(20), fleet.PrimaryAssignment, at.Add(-time.Hour), &at, at), assignmentView(42, id(20), fleet.AdditionalAssignment, at.Add(-time.Hour), nil, at)}, At: at}
		edit(&p)
		return p
	}
	for name, p := range map[string]app.AssignmentPage{
		"state drift":     bad(func(p *app.AssignmentPage) { p.Assignments[0].State = fleet.AssignmentActive }),
		"other asset":     bad(func(p *app.AssignmentPage) { p.Assignments[0].AssetID = id(21) }),
		"order":           bad(func(p *app.AssignmentPage) { p.Assignments[0], p.Assignments[1] = p.Assignments[1], p.Assignments[0] }),
		"temporary open":  bad(func(p *app.AssignmentPage) { p.Assignments[1].Kind = fleet.TemporaryAssignment }),
		"cause no time":   bad(func(p *app.AssignmentPage) { p.Assignments[0].EndCause = app.EndedByDisposal }),
		"unknown cause":   bad(func(p *app.AssignmentPage) { p.Assignments[0].EndCause, p.Assignments[0].EndedAt = "LOST", &at }),
		"no instant":      bad(func(p *app.AssignmentPage) { p.At = time.Time{} }),
		"too many":        bad(func(p *app.AssignmentPage) { p.Assignments = append(p.Assignments, p.Assignments[1], p.Assignments[1]) }),
		"bad employee":    bad(func(p *app.AssignmentPage) { p.Assignments[1].EmployeeID = "x" }),
		"cursor mismatch": bad(func(p *app.AssignmentPage) { p.NextID = id(41) }),
	} {
		if _, err := (app.AssignmentService{Enabled: true, Repository: &assignmentRepository{page: p}}).List(ctx, s, actor, d, id(20), app.AssignmentFilter{Limit: 3}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid page published", name, err)
		}
	}
	full := bad(func(p *app.AssignmentPage) {
		from := p.Assignments[1].Period.From
		p.NextFrom, p.NextID = &from, id(42)
	})
	if v, err := (app.AssignmentService{Enabled: true, Repository: &assignmentRepository{page: full}}).List(ctx, s, actor, d, id(20), app.AssignmentFilter{Limit: 2}); err != nil || v.NextID != id(42) {
		t.Fatal("full page cursor", err)
	}
	if _, err := (app.AssignmentService{Enabled: true, Repository: &assignmentRepository{page: full}}).List(ctx, s, actor, d, id(21), app.AssignmentFilter{Limit: 2}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("list of another asset accepted", err)
	}
}

func TestAssignmentServiceOwnListTargetsTheRequestingMembership(t *testing.T) {
	ctx, finish, s, actor, d := policyDecision(t, id(4), app.Policy(false))
	defer finish()
	at := assignmentNow
	rows := []app.AssignmentView{
		assignmentView(41, id(20), fleet.PrimaryAssignment, at.Add(-time.Hour), nil, at),
		assignmentView(42, id(21), fleet.AdditionalAssignment, at, nil, at),
	}
	repo := &assignmentRepository{page: app.AssignmentPage{Assignments: rows, At: at}}
	page, err := (app.AssignmentService{Enabled: true, Repository: repo}).Mine(ctx, s, actor, d, app.OwnAssignmentFilter{Limit: 10})
	if err != nil || len(page.Assignments) != 2 || repo.member != id(4) {
		t.Fatal("own assignment page", err)
	}
	// Only ACTIVE assignments belong to the own list, in identifier order.
	scheduled := []app.AssignmentView{assignmentView(43, id(20), fleet.PrimaryAssignment, at.Add(time.Hour), nil, at)}
	unordered := []app.AssignmentView{rows[1], rows[0]}
	for name, p := range map[string]app.AssignmentPage{"scheduled": {Assignments: scheduled, At: at}, "order": {Assignments: unordered, At: at}, "from cursor": {Assignments: rows, At: at, NextFrom: &at, NextID: id(42)}} {
		if _, err := (app.AssignmentService{Enabled: true, Repository: &assignmentRepository{page: p}}).Mine(ctx, s, actor, d, app.OwnAssignmentFilter{Limit: 2}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid own page published", name, err)
		}
	}
}
