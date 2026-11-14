package application_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	a "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

const (
	structureUnitNorte = "7b0e2a54-0000-4000-8000-000000001001"
	structureUnitSul   = "7b0e2a54-0000-4000-8000-000000001002"
	structureTeam      = "7b0e2a54-0000-4000-8000-000000001201"
	structureAsset     = "7b0e2a54-0000-4000-8000-000000001301"
)

type structureSource struct {
	events   *[]string
	value    d.ResourceFacts
	err      error
	edit     func(*d.ResourceFacts)
	policy   a.Policy
	at       time.Time
	received d.ResourceFacts
}

func (s *structureSource) ResolveStructure(_ context.Context, scope kernel.OrgScope, policy a.Policy, facts d.ResourceFacts, at time.Time) (d.ResourceFacts, error) {
	*s.events = append(*s.events, "structure")
	s.policy, s.at, s.received = policy, at, facts
	if s.err != nil {
		return d.ResourceFacts{}, s.err
	}
	out := facts
	out.UnitID, out.CostCentreID, out.AssetID, out.Teams = s.value.UnitID, s.value.CostCentreID, s.value.AssetID, slices.Clone(s.value.Teams)
	if s.edit != nil {
		s.edit(&out)
	}
	return out, nil
}

type capturedRecords struct {
	events *[]string
	facts  d.ResourceFacts
}

func (r *capturedRecords) Check(_ context.Context, _ kernel.OrgScope, _ a.Subject, _ a.Policy, facts d.ResourceFacts, _ []d.Rule) error {
	*r.events = append(*r.events, "record")
	r.facts = facts
	return nil
}

func structureFixture(t *testing.T, assignments ...d.Assignment) (fixture, *structureSource, *capturedRecords) {
	t.Helper()
	f := setup(t)
	f.source.value.Assignments = assignments
	f.policy = a.Policy{Permission: "meter.reading.correct", Capability: "organizations", Effect: a.Read}
	source := &structureSource{events: f.events, value: d.ResourceFacts{UnitID: structureUnitNorte, AssetID: structureAsset, Teams: []string{structureTeam}}}
	records := &capturedRecords{events: f.events}
	f.service.Structure = source
	f.service.Records = records
	return f, source, records
}

func structureRole(t *testing.T, f fixture, role d.Role, kind d.ScopeType, target string) d.Assignment {
	t.Helper()
	if target == "" {
		target = f.scope.OrgID().String()
	}
	v, err := d.RehydrateAssignment(f.scope, d.AssignmentSnapshot{OrgID: f.scope.OrgID(), ID: target, MembershipID: member, AssignedBy: member, Role: role, Scope: kind, TargetID: target, ValidFrom: time.Date(2026, 11, 11, 11, 0, 0, 0, time.UTC), Reason: "synthetic structural authority", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func structurePrepare(t *testing.T, f fixture) error {
	t.Helper()
	ctx, finish := a.BeginRequest(t.Context())
	defer finish()
	pending, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "web"}, f.policy, f.resource)
	if err != nil {
		return err
	}
	_, err = f.service.Complete(ctx, pending, f.proof)
	return err
}

func TestPrepareResolvesStructureOnlyForACurrentStructuralGrant(t *testing.T) {
	base := setup(t)
	unitNorte := structureRole(t, base, d.Supervisor, d.UnitScope, structureUnitNorte)
	t.Run("organization grant never consults the source", func(t *testing.T) {
		f, _, records := structureFixture(t)
		f.source.value.Assignments = []d.Assignment{structureRole(t, f, d.Supervisor, d.OrgScope, "")}
		if err := structurePrepare(t, f); err != nil {
			t.Fatal(err)
		}
		if want := []string{"authority", "resource", "proof", "capability", "record"}; !reflect.DeepEqual(*f.events, want) || records.facts.UnitID != "" {
			t.Fatal("ORG request changed its steps or facts", *f.events)
		}
	})
	t.Run("structural grant without the permission never consults the source", func(t *testing.T) {
		f, _, _ := structureFixture(t, structureRole(t, base, d.Driver, d.UnitScope, structureUnitNorte), structureRole(t, base, d.Supervisor, d.OrgScope, ""))
		if err := structurePrepare(t, f); err != nil || slices.Contains(*f.events, "structure") {
			t.Fatal("non-positive structural grant consulted the source", *f.events, err)
		}
	})
	t.Run("matching stored unit", func(t *testing.T) {
		f, source, records := structureFixture(t, unitNorte)
		if err := structurePrepare(t, f); err != nil {
			t.Fatal(err)
		}
		if want := []string{"authority", "resource", "structure", "proof", "capability", "record"}; !reflect.DeepEqual(*f.events, want) {
			t.Fatal(*f.events)
		}
		if source.policy != f.policy || !source.at.Equal(time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)) || source.received.ID != target || source.received.UnitID != "" {
			t.Fatal("source did not receive the policy, Prepare instant and unresolved facts")
		}
		if records.facts.UnitID != structureUnitNorte || records.facts.AssetID != structureAsset || !slices.Equal(records.facts.Teams, []string{structureTeam}) {
			t.Fatal("record gate did not receive the resolved structure")
		}
	})
	for _, tc := range []struct {
		name   string
		roles  []d.Assignment
		change func(*fixture, *structureSource)
		want   error
	}{
		{"other unit", []d.Assignment{structureRole(t, base, d.Supervisor, d.UnitScope, structureUnitSul)}, nil, d.ErrPermissionDenied},
		{"no source configured fails closed", []d.Assignment{unitNorte}, func(f *fixture, _ *structureSource) { f.service.Structure = nil }, d.ErrPermissionDenied},
		{"record without stored unit", []d.Assignment{unitNorte}, func(_ *fixture, s *structureSource) { s.value.UnitID = "" }, d.ErrPermissionDenied},
		{"missing record", []d.Assignment{unitNorte}, func(_ *fixture, s *structureSource) { s.err = a.ErrNotFound }, d.ErrResourceNotFound},
		{"private source failure", []d.Assignment{unitNorte}, func(_ *fixture, s *structureSource) { s.err = errors.New("synthetic private SQL detail") }, a.ErrUnavailable},
		{"source injected authorship", []d.Assignment{unitNorte}, func(_ *fixture, s *structureSource) {
			s.edit = func(v *d.ResourceFacts) { v.AuthoredBy = member }
		}, a.ErrUnavailable},
		{"source replaced the record", []d.Assignment{unitNorte}, func(_ *fixture, s *structureSource) {
			s.edit = func(v *d.ResourceFacts) { v.ID = structureAsset }
		}, a.ErrUnavailable},
		{"invalid resolved team", []d.Assignment{structureRole(t, base, d.Supervisor, d.TeamScope, structureTeam)}, func(_ *fixture, s *structureSource) { s.value.Teams = []string{structureTeam, structureTeam} }, a.ErrUnavailable},
		{"team grant without team facts", []d.Assignment{structureRole(t, base, d.Supervisor, d.TeamScope, structureTeam)}, func(_ *fixture, s *structureSource) { s.value.Teams = nil }, d.ErrPermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, source, _ := structureFixture(t, tc.roles...)
			if tc.change != nil {
				tc.change(&f, source)
			}
			err := structurePrepare(t, f)
			if !errors.Is(err, tc.want) {
				t.Fatal(err, tc.want)
			}
			var denied *d.PermissionDenial
			if errors.Is(tc.want, d.ErrPermissionDenied) && (!errors.As(err, &denied) || denied.Permission() != f.policy.Permission) {
				t.Fatal("structural refusal must name exactly the permission", err)
			}
			if slices.Contains(*f.events, "proof") {
				t.Fatal("refused structure reached later gates", *f.events)
			}
		})
	}
	t.Run("asset and team grants match their facts", func(t *testing.T) {
		for _, role := range []d.Assignment{structureRole(t, base, d.Supervisor, d.AssetScope, structureAsset), structureRole(t, base, d.Supervisor, d.TeamScope, structureTeam)} {
			f, _, _ := structureFixture(t, role)
			if err := structurePrepare(t, f); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestRoleInputsAdmitStructuralScopesWithoutWidening(t *testing.T) {
	scope := testScope(t)
	assign := a.RoleAssignInput{OperationID: "7b0e2a54-0000-4000-8000-000000000010", MembershipID: "7b0e2a54-0000-4000-8000-000000000020", Role: d.Supervisor, Scope: d.UnitScope, TargetID: structureUnitNorte, Reason: "Synthetic unit supervisor"}
	change := a.RoleChangeInput{OperationID: "7b0e2a54-0000-4000-8000-000000000011", AssignmentID: "7b0e2a54-0000-4000-8000-000000000030", ExpectedRevision: 1, Role: d.Supervisor, Scope: d.TeamScope, TargetID: structureTeam, Reason: "Synthetic team supervisor"}
	for _, kind := range []d.ScopeType{d.UnitScope, d.CostCentreScope, d.TeamScope, d.AssetScope} {
		v, c := assign, change
		v.Scope, c.Scope = kind, kind
		if err := v.Validate(scope); err != nil {
			t.Fatal("structural assign refused", kind, err)
		}
		if err := c.Validate(scope); err != nil {
			t.Fatal("structural change refused", kind, err)
		}
	}
	other := assign
	other.Scope = d.TeamScope
	if assign.Digest(scope) == other.Digest(scope) {
		t.Fatal("scope type is not bound into the command digest")
	}
	for name, edit := range map[string]func(*a.RoleAssignInput){
		"owner at a unit":     func(v *a.RoleAssignInput) { v.Role = d.Owner },
		"station role":        func(v *a.RoleAssignInput) { v.Role = d.StationAttendant },
		"organization target": func(v *a.RoleAssignInput) { v.TargetID = scope.OrgID().String() },
		"membership as team":  func(v *a.RoleAssignInput) { v.Scope = d.TeamScope; v.TargetID = v.MembershipID },
		"station scope":       func(v *a.RoleAssignInput) { v.Scope = "STATION" },
	} {
		v := assign
		edit(&v)
		if err := v.Validate(scope); !errors.Is(err, d.ErrAssignmentScope) {
			t.Fatal("structural widening accepted:", name, err)
		}
	}
	for name, edit := range map[string]func(*a.RoleChangeInput){
		"owner at a team":     func(v *a.RoleChangeInput) { v.Role = d.Owner },
		"organization target": func(v *a.RoleChangeInput) { v.TargetID = scope.OrgID().String() },
		"station scope":       func(v *a.RoleChangeInput) { v.Scope = "STATION" },
	} {
		v := change
		edit(&v)
		if err := v.Validate(scope); !errors.Is(err, d.ErrAssignmentScope) {
			t.Fatal("structural change widening accepted:", name, err)
		}
	}
}
