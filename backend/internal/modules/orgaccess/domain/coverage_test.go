package domain_test

import (
	"errors"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func TestCoveringRulesPreserveSelfAndCatalogRestrictions(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	member, other, record := "7b0e2a54-0000-4000-8000-000000000601", "7b0e2a54-0000-4000-8000-000000000602", "7b0e2a54-0000-4000-8000-000000000901"
	target := d.ResourceFacts{OrgID: s.OrgID(), ID: record, AuthoredBy: member, TripDriver: member, AgreementSide: d.FleetParty, RecordedTrack: true, RequestedRole: d.Driver}
	makeRole := func(role d.Role, kind d.ScopeType) d.Assignment {
		t.Helper()
		id := s.OrgID().String()
		if kind == d.SelfScope {
			id = member
		}
		v, err := d.RehydrateAssignment(s, d.AssignmentSnapshot{OrgID: s.OrgID(), ID: record, MembershipID: member, AssignedBy: member, Role: role, Scope: kind, TargetID: id, ValidFrom: now.Add(-time.Hour), Reason: "synthetic policy", Revision: 1})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct {
		name       string
		role       d.Role
		scope      d.ScopeType
		permission d.Permission
		change     func(*d.ResourceFacts)
		allow      bool
	}{
		{"owner own trip", d.Owner, d.OrgScope, "trip.trip.start", nil, true},
		{"owner cannot start other's trip", d.Owner, d.OrgScope, "trip.trip.start", func(v *d.ResourceFacts) { v.AuthoredBy = other; v.TripDriver = other }, false},
		{"driver own trip read", d.Driver, d.SelfScope, "trip.trip.read", nil, true},
		{"driver assigned asset", d.Driver, d.SelfScope, "trip.trip.read", func(v *d.ResourceFacts) { v.AuthoredBy = other; v.AssignedTo = member }, true},
		{"driver foreign self", d.Driver, d.SelfScope, "trip.trip.read", func(v *d.ResourceFacts) { v.AuthoredBy = other; v.TripDriver = other }, false},
		{"driver GPS own trip", d.Driver, d.SelfScope, "gps.location.read_live", nil, true},
		{"driver GPS assigned non-own trip", d.Driver, d.SelfScope, "gps.location.read_live", func(v *d.ResourceFacts) { v.AuthoredBy = other; v.AssignedTo = member; v.TripDriver = other }, false},
		{"auditor recorded", d.Auditor, d.OrgScope, "gps.track.read", nil, true},
		{"auditor not live", d.Auditor, d.OrgScope, "gps.track.read", func(v *d.ResourceFacts) { v.RecordedTrack = false }, false},
		{"admin can assign driver", d.Admin, d.OrgScope, "auth.role.assign", nil, true},
		{"admin cannot assign owner", d.Admin, d.OrgScope, "auth.role.assign", func(v *d.ResourceFacts) { v.RequestedRole = d.Owner }, false},
		{"owner fleet agreement", d.Owner, d.OrgScope, "agree.agreement.manage", nil, true},
		{"owner not station party", d.Owner, d.OrgScope, "agree.agreement.manage", func(v *d.ResourceFacts) { v.AgreementSide = d.StationParty }, false},
		{"unassigned key denied", d.Owner, d.OrgScope, "auth.role.read", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := target
			if tc.change != nil {
				tc.change(&v)
			}
			rules, err := d.CoveringRules(s, member, []d.Assignment{makeRole(tc.role, tc.scope)}, tc.permission, v, now)
			if tc.allow {
				if err != nil || len(rules) != 1 {
					t.Fatal(len(rules), err)
				}
			} else {
				var denied *d.PermissionDenial
				if !errors.As(err, &denied) || denied.Permission() != tc.permission || len(rules) != 0 {
					t.Fatal("missing bounded permission denial", err)
				}
			}
		})
	}
}

func TestCoveringRulesExcludeForeignEndedExpiredAndOtherMembership(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	member := "7b0e2a54-0000-4000-8000-000000000601"
	target := d.ResourceFacts{OrgID: s.OrgID(), ID: member, AuthoredBy: member}
	base := d.AssignmentSnapshot{OrgID: s.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000000701", MembershipID: member, AssignedBy: member, Role: d.Driver, Scope: d.SelfScope, TargetID: member, ValidFrom: now.Add(-time.Hour), Revision: 1, Reason: "synthetic policy"}
	for _, change := range []func(*d.AssignmentSnapshot){func(v *d.AssignmentSnapshot) { v.ValidTo = now }, func(v *d.AssignmentSnapshot) { v.ValidFrom = now.Add(time.Second) }, func(v *d.AssignmentSnapshot) { v.EndedAt = now; v.Revision = 2 }, func(v *d.AssignmentSnapshot) {
		v.MembershipID = "7b0e2a54-0000-4000-8000-000000000602"
		v.TargetID = v.MembershipID
	}} {
		v := base
		change(&v)
		a, err := d.RehydrateAssignment(s, v)
		if err != nil {
			t.Fatal(err)
		}
		if rules, err := d.CoveringRules(s, member, []d.Assignment{a}, "trip.trip.read", target, now); len(rules) != 0 || !errors.Is(err, d.ErrPermissionDenied) {
			t.Fatal("invalid candidate granted", err)
		}
	}
	foreign := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000302")
	target.OrgID = foreign.OrgID()
	if _, err := d.CoveringRules(s, member, nil, "trip.trip.read", target, now); err != d.ErrResourceNotFound {
		t.Fatal(err)
	}
}
