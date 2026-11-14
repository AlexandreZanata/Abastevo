package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// Synthetic structure identifiers (EMP-AUTH-006). Norte and Sul are units,
// the centres and teams are distinct, V is the vehicle of example 3.
const (
	structuralMember = "7b0e2a54-0000-4000-8000-000000000601"
	structuralOther  = "7b0e2a54-0000-4000-8000-000000000602"
	structuralRecord = "7b0e2a54-0000-4000-8000-000000000901"
	unitNorte        = "7b0e2a54-0000-4000-8000-000000001001"
	unitSul          = "7b0e2a54-0000-4000-8000-000000001002"
	centreNorte      = "7b0e2a54-0000-4000-8000-000000001101"
	centreSul        = "7b0e2a54-0000-4000-8000-000000001102"
	teamA            = "7b0e2a54-0000-4000-8000-000000001201"
	teamB            = "7b0e2a54-0000-4000-8000-000000001202"
	vehicleV         = "7b0e2a54-0000-4000-8000-000000001301"
	vehicleW         = "7b0e2a54-0000-4000-8000-000000001302"
)

var structuralNow = time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)

func structuralAssignment(t *testing.T, role d.Role, kind d.ScopeType, target string) d.Assignment {
	t.Helper()
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	if target == "" {
		target = s.OrgID().String()
		if kind == d.SelfScope {
			target = structuralMember
		}
	}
	v, err := d.RehydrateAssignment(s, d.AssignmentSnapshot{OrgID: s.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000000701", MembershipID: structuralMember, AssignedBy: structuralOther, Role: role, Scope: kind, TargetID: target, ValidFrom: structuralNow.Add(-time.Hour), Reason: "synthetic structural scope", Revision: 1})
	if err != nil {
		t.Fatal(role, kind, err)
	}
	return v
}

func TestStructuralScopesRehydrateOnlyWithAStructureTarget(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	for _, kind := range []d.ScopeType{d.UnitScope, d.CostCentreScope, d.TeamScope, d.AssetScope} {
		if !kind.Structural() {
			t.Fatal("structural scope not recognised", kind)
		}
		a := structuralAssignment(t, d.Supervisor, kind, unitNorte)
		v, err := a.Snapshot(s)
		if err != nil || v.Scope != kind || v.TargetID != unitNorte {
			t.Fatal("structural snapshot changed", kind, err)
		}
	}
	for _, kind := range []d.ScopeType{d.OrgScope, d.SelfScope, "", "STATION", "unit", "UNIT "} {
		if kind.Structural() {
			t.Fatal("non-structural scope reported structural", kind)
		}
	}
	base := d.AssignmentSnapshot{OrgID: s.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000000701", MembershipID: structuralMember, AssignedBy: structuralOther, Role: d.Supervisor, Scope: d.UnitScope, TargetID: unitNorte, ValidFrom: structuralNow, Reason: "synthetic structural scope", Revision: 1}
	for name, change := range map[string]func(*d.AssignmentSnapshot){
		"owner is organization only":   func(v *d.AssignmentSnapshot) { v.Role = d.Owner },
		"station role unavailable":     func(v *d.AssignmentSnapshot) { v.Role = d.StationManager },
		"organization as unit":         func(v *d.AssignmentSnapshot) { v.TargetID = s.OrgID().String() },
		"own membership as team":       func(v *d.AssignmentSnapshot) { v.Scope = d.TeamScope; v.TargetID = structuralMember },
		"non-canonical target":         func(v *d.AssignmentSnapshot) { v.TargetID = "7B0E2A54-0000-4000-8000-000000001001" },
		"unknown station scope":        func(v *d.AssignmentSnapshot) { v.Scope = "STATION" },
		"lower-case alias":             func(v *d.AssignmentSnapshot) { v.Scope = "unit" },
		"asset scope without a target": func(v *d.AssignmentSnapshot) { v.Scope = d.AssetScope; v.TargetID = "" },
	} {
		v := base
		change(&v)
		if _, err := d.RehydrateAssignment(s, v); err == nil {
			t.Fatal("invalid structural assignment accepted:", name)
		}
	}
	for _, role := range []d.Role{d.Admin, d.FleetManager, d.Supervisor, d.Finance, d.Approver, d.Driver, d.OperationsStaff, d.Auditor, d.DataManager} {
		v := base
		v.Role = role
		if _, err := d.RehydrateAssignment(s, v); err != nil {
			t.Fatal("ordinary role refused at a unit", role, err)
		}
	}
}

func TestStructuralCoverageMatchesOnlyTheResolvedSnapshot(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	stored := d.ResourceFacts{OrgID: s.OrgID(), ID: structuralRecord, AuthoredBy: structuralOther, UnitID: unitNorte, CostCentreID: centreNorte, AssetID: vehicleV, Teams: []string{teamA}}
	for _, tc := range []struct {
		name       string
		role       d.Role
		scope      d.ScopeType
		target     string
		permission d.Permission
		change     func(*d.ResourceFacts)
		allow      bool
	}{
		{"unit match", d.Finance, d.UnitScope, unitNorte, "fuel.fueling.read", nil, true},
		{"unit mismatch", d.Finance, d.UnitScope, unitSul, "fuel.fueling.read", nil, false},
		{"record without unit", d.Finance, d.UnitScope, unitNorte, "fuel.fueling.read", func(v *d.ResourceFacts) { v.UnitID = "" }, false},
		{"cost centre match", d.Finance, d.CostCentreScope, centreNorte, "fuel.fueling.read", nil, true},
		{"wrong cost centre", d.Finance, d.CostCentreScope, centreSul, "fuel.fueling.read", nil, false},
		{"no stored cost centre is never derived from the unit", d.Finance, d.CostCentreScope, unitNorte, "fuel.fueling.read", func(v *d.ResourceFacts) { v.CostCentreID = "" }, false},
		{"unit identifier is not a cost centre", d.Finance, d.CostCentreScope, unitNorte, "fuel.fueling.read", nil, false},
		{"asset match", d.Supervisor, d.AssetScope, vehicleV, "meter.reading.correct", nil, true},
		{"other asset", d.Supervisor, d.AssetScope, vehicleW, "meter.reading.correct", nil, false},
		{"team of the author at the record instant", d.Supervisor, d.TeamScope, teamA, "meter.reading.correct", nil, true},
		{"other team", d.Supervisor, d.TeamScope, teamB, "meter.reading.correct", nil, false},
		{"no team facts", d.Supervisor, d.TeamScope, teamA, "meter.reading.correct", func(v *d.ResourceFacts) { v.Teams = nil }, false},
		{"unit is not a team", d.Supervisor, d.TeamScope, unitNorte, "meter.reading.correct", nil, false},
		{"organization-level resource carries no structure", d.Supervisor, d.UnitScope, unitNorte, "meter.reading.correct", func(v *d.ResourceFacts) { *v = d.ResourceFacts{OrgID: v.OrgID, ID: s.OrgID().String()} }, false},
		{"denied cell stays denied in scope", d.Driver, d.UnitScope, unitNorte, "fin.period.close", nil, false},
		{"own-records cell needs own record", d.Driver, d.UnitScope, unitNorte, "trip.trip.read", nil, false},
		{"own-records cell needs the scope too", d.Driver, d.UnitScope, unitSul, "trip.trip.read", func(v *d.ResourceFacts) { v.AuthoredBy = structuralMember }, false},
		{"own record inside the unit", d.Driver, d.UnitScope, unitNorte, "trip.trip.read", func(v *d.ResourceFacts) { v.AuthoredBy = structuralMember }, true},
		{"restriction kept in scope", d.Auditor, d.UnitScope, unitNorte, "gps.track.read", nil, false},
		{"restriction satisfied in scope", d.Auditor, d.UnitScope, unitNorte, "gps.track.read", func(v *d.ResourceFacts) { v.RecordedTrack = true }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := stored
			v.Teams = slices.Clone(stored.Teams)
			if tc.change != nil {
				tc.change(&v)
			}
			rules, err := d.CoveringRules(s, structuralMember, []d.Assignment{structuralAssignment(t, tc.role, tc.scope, tc.target)}, tc.permission, v, structuralNow)
			if tc.allow {
				if err != nil || len(rules) != 1 {
					t.Fatal(len(rules), err)
				}
				return
			}
			var denied *d.PermissionDenial
			if !errors.As(err, &denied) || denied.Permission() != tc.permission || len(rules) != 0 {
				t.Fatal("missing bounded permission denial", err)
			}
		})
	}
}

// 07-permissions §10 example 3 as a pure decision: the March record stored
// Norte; after V moves to Sul the asset itself resolves to Sul.
func TestPermissionsExampleThreeUsesTheStoredUnitNotTheCurrentAllocation(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	march := d.ResourceFacts{OrgID: s.OrgID(), ID: structuralRecord, UnitID: unitNorte, AssetID: vehicleV}
	assetNow := d.ResourceFacts{OrgID: s.OrgID(), ID: vehicleV, UnitID: unitSul, AssetID: vehicleV}
	norte := structuralAssignment(t, d.Finance, d.UnitScope, unitNorte)
	sul := structuralAssignment(t, d.Finance, d.UnitScope, unitSul)
	if _, err := d.CoveringRules(s, structuralMember, []d.Assignment{norte}, "fuel.fueling.read", march, structuralNow); err != nil {
		t.Fatal("Norte finance lost the March record", err)
	}
	if _, err := d.CoveringRules(s, structuralMember, []d.Assignment{sul}, "fuel.fueling.read", march, structuralNow); !errors.Is(err, d.ErrPermissionDenied) {
		t.Fatal("reassigned asset widened history to Sul", err)
	}
	if _, err := d.CoveringRules(s, structuralMember, []d.Assignment{sul}, "fleet.asset.read", assetNow, structuralNow); err != nil {
		t.Fatal("Sul finance cannot read the asset now in Sul", err)
	}
	if _, err := d.CoveringRules(s, structuralMember, []d.Assignment{norte}, "fleet.asset.read", assetNow, structuralNow); !errors.Is(err, d.ErrPermissionDenied) {
		t.Fatal("Norte kept the moved asset", err)
	}
	// Union of roles: an organization assignment still covers everything.
	if _, err := d.CoveringRules(s, structuralMember, []d.Assignment{sul, structuralAssignment(t, d.Auditor, d.OrgScope, "")}, "fuel.fueling.read", march, structuralNow); err != nil {
		t.Fatal("ORG union lost", err)
	}
}

func TestResourceFactsStructureIsValidatedAndMergedWithoutAliasing(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	base := d.ResourceFacts{OrgID: s.OrgID(), ID: structuralRecord, AuthoredBy: structuralMember}
	for name, change := range map[string]func(*d.ResourceFacts){
		"unit":           func(v *d.ResourceFacts) { v.UnitID = "bad" },
		"cost centre":    func(v *d.ResourceFacts) { v.CostCentreID = "7B0E2A54-0000-4000-8000-000000001101" },
		"asset":          func(v *d.ResourceFacts) { v.AssetID = " " + vehicleV },
		"team":           func(v *d.ResourceFacts) { v.Teams = []string{teamA, ""} },
		"duplicate team": func(v *d.ResourceFacts) { v.Teams = []string{teamA, teamA} },
	} {
		v := base
		change(&v)
		if v.Validate(s) == nil {
			t.Fatal("invalid structural fact accepted:", name)
		}
	}
	resolved := base
	resolved.UnitID, resolved.CostCentreID, resolved.AssetID, resolved.Teams = unitNorte, centreNorte, vehicleV, []string{teamB, teamA}
	merged, err := base.WithStructure(s, resolved)
	if err != nil || merged.UnitID != unitNorte || merged.CostCentreID != centreNorte || merged.AssetID != vehicleV || !slices.Equal(merged.Teams, []string{teamB, teamA}) || merged.AuthoredBy != structuralMember || merged.ID != structuralRecord {
		t.Fatal("structure not merged exactly", err)
	}
	resolved.Teams[0] = teamA
	if merged.Teams[0] != teamB {
		t.Fatal("merged teams alias the source")
	}
	for name, change := range map[string]func(*d.ResourceFacts){
		"other record":     func(v *d.ResourceFacts) { v.ID = vehicleV },
		"author injected":  func(v *d.ResourceFacts) { v.AuthoredBy = structuralOther },
		"assignee added":   func(v *d.ResourceFacts) { v.AssignedTo = structuralMember },
		"trip driver":      func(v *d.ResourceFacts) { v.TripDriver = structuralMember },
		"recorded track":   func(v *d.ResourceFacts) { v.RecordedTrack = true },
		"station":          func(v *d.ResourceFacts) { v.StationAuthorityLive = true },
		"agreement side":   func(v *d.ResourceFacts) { v.AgreementSide = d.FleetParty },
		"requested role":   func(v *d.ResourceFacts) { v.RequestedRole = d.Driver },
		"other org":        func(v *d.ResourceFacts) { v.OrgID = assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000302").OrgID() },
		"invalid resolved": func(v *d.ResourceFacts) { v.UnitID = "bad" },
	} {
		v := resolved
		change(&v)
		if _, err := base.WithStructure(s, v); err == nil {
			t.Fatal("structure source changed record identity or authorship:", name)
		}
	}
	invalid := base
	invalid.AuthoredBy = "bad"
	if _, err := invalid.WithStructure(s, resolved); err == nil {
		t.Fatal("structure merged into invalid facts")
	}
}

func TestStructuralCandidateOnlyForACurrentPositiveStructuralGrant(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	unit := structuralAssignment(t, d.Finance, d.UnitScope, unitNorte)
	for _, tc := range []struct {
		name        string
		assignments []d.Assignment
		permission  d.Permission
		at          time.Time
		want        bool
	}{
		{"organization only", []d.Assignment{structuralAssignment(t, d.Finance, d.OrgScope, "")}, "fuel.fueling.read", structuralNow, false},
		{"self only", []d.Assignment{structuralAssignment(t, d.Driver, d.SelfScope, "")}, "trip.trip.read", structuralNow, false},
		{"positive unit grant", []d.Assignment{unit}, "fuel.fueling.read", structuralNow, true},
		{"unit grant without the permission", []d.Assignment{unit}, "fleet.asset.manage", structuralNow, false},
		{"not yet valid", []d.Assignment{unit}, "fuel.fueling.read", structuralNow.Add(-2 * time.Hour), false},
	} {
		got, err := d.StructuralCandidate(s, structuralMember, tc.assignments, tc.permission, tc.at)
		if err != nil || got != tc.want {
			t.Fatal(tc.name, got, err)
		}
	}
	if got, err := d.StructuralCandidate(s, structuralOther, []d.Assignment{unit}, "fuel.fueling.read", structuralNow); err != nil || got {
		t.Fatal("another member's structural grant considered", got, err)
	}
	if _, err := d.StructuralCandidate(s, structuralMember, []d.Assignment{unit}, "unknown.key.value", structuralNow); err == nil {
		t.Fatal("unknown permission accepted")
	}
	if _, err := d.StructuralCandidate(s, "bad", []d.Assignment{unit}, "fuel.fueling.read", structuralNow); err == nil {
		t.Fatal("invalid member accepted")
	}
}

func TestScopeWithinNeverWidens(t *testing.T) {
	org := "7b0e2a54-0000-4000-8000-000000000301"
	for _, tc := range []struct {
		outer, inner             d.ScopeType
		outerTarget, innerTarget string
		want                     bool
	}{
		{d.OrgScope, d.OrgScope, org, org, true},
		{d.OrgScope, d.UnitScope, org, unitNorte, true},
		{d.OrgScope, d.SelfScope, org, structuralMember, true},
		{d.UnitScope, d.UnitScope, unitNorte, unitNorte, true},
		{d.SelfScope, d.SelfScope, structuralMember, structuralMember, true},
		{d.UnitScope, d.OrgScope, unitNorte, org, false},
		{d.UnitScope, d.UnitScope, unitNorte, unitSul, false},
		{d.SelfScope, d.UnitScope, structuralMember, unitNorte, false},
		{d.UnitScope, d.SelfScope, unitNorte, structuralMember, false},
		{d.TeamScope, d.AssetScope, teamA, teamA, false},
		{d.SelfScope, d.OrgScope, structuralMember, org, false},
		{"STATION", "STATION", unitNorte, unitNorte, false},
		{d.OrgScope, "STATION", org, unitNorte, false},
	} {
		if got := d.ScopeWithin(tc.outer, tc.outerTarget, tc.inner, tc.innerTarget); got != tc.want {
			t.Fatal(tc.outer, tc.inner, got)
		}
	}
}

func TestStructuralGrantsNarrowButNeverWidenTheGrantor(t *testing.T) {
	c := d.SystemCatalog()
	for _, kind := range []d.ScopeType{d.UnitScope, d.CostCentreScope, d.TeamScope, d.AssetScope} {
		if !d.CanAssignScopedRole(c, []d.Role{d.Owner}, d.Admin, kind) {
			t.Fatal("owner cannot narrow ADMIN to a structure", kind)
		}
		if !d.CanAssignScopedRole(c, []d.Role{d.Admin}, d.Approver, kind) {
			t.Fatal("admin cannot narrow APPROVER to a structure", kind)
		}
		// ADMIN holds maint.request.create only as S; SUPERVISOR holds O. The
		// structure does not lift that gap (same answer as at ORG).
		if d.CanAssignScopedRole(c, []d.Role{d.Admin}, d.Supervisor, kind) != d.CanAssignScopedRole(c, []d.Role{d.Admin}, d.Supervisor, d.OrgScope) {
			t.Fatal("structural containment differs from organization containment", kind)
		}
		if d.CanAssignScopedRole(c, []d.Role{d.Owner}, d.Owner, kind) {
			t.Fatal("structural OWNER admitted", kind)
		}
		if d.CanAssignScopedRole(c, []d.Role{d.Admin}, d.Finance, kind) {
			t.Fatal("structure invented missing financial permission", kind)
		}
		// A structural scope applies organization coverage inside the target,
		// so it never turns the grantor's own-record descriptor into O.
		if d.CanAssignScopedRole(c, []d.Role{d.Owner}, d.FleetManager, kind) {
			t.Fatal("structure widened a frozen own-record descriptor", kind)
		}
		if d.CanAssignScopedRole(c, []d.Role{d.Supervisor}, d.Driver, kind) {
			t.Fatal("grantor without auth.role.assign granted a structure", kind)
		}
		if d.CanAssignScopedRole(c, []d.Role{d.Owner}, d.StationAdmin, kind) {
			t.Fatal("station role granted at a structure", kind)
		}
	}
	if d.CanAssignScopedRole(c, []d.Role{d.Owner}, d.Admin, "STATION") {
		t.Fatal("unknown scope admitted")
	}
}
