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

type assigneeSource struct {
	events   *[]string
	assigned bool
	err      error
	policy   a.Policy
	member   string
	at       time.Time
	received d.ResourceFacts
}

func (s *assigneeSource) ResolveAssignee(_ context.Context, _ kernel.OrgScope, policy a.Policy, facts d.ResourceFacts, member string, at time.Time) (bool, error) {
	*s.events = append(*s.events, "assignee")
	s.policy, s.member, s.at, s.received = policy, member, at, facts
	return s.assigned, s.err
}

func assigneeFixture(t *testing.T, assignments ...d.Assignment) (fixture, *assigneeSource, *capturedRecords) {
	t.Helper()
	f := setup(t)
	f.source.value.Assignments = assignments
	f.policy = a.Policy{Permission: "fleet.asset.read", Capability: "organizations", Effect: a.Read}
	source := &assigneeSource{events: f.events, assigned: true}
	records := &capturedRecords{events: f.events}
	f.service.Assignee = source
	f.service.Records = records
	return f, source, records
}

// EMP-FLEET-006 (EDR-165): the assignee lookup mirrors the EDR-161 structure
// lookup: only for a current positive SELF grant, setting only AssignedTo.
func TestPrepareResolvesTheAssigneeOnlyForACurrentSelfGrant(t *testing.T) {
	base := setup(t)
	self := structureRole(t, base, d.Driver, d.SelfScope, member)
	t.Run("organization grant never consults the source", func(t *testing.T) {
		f, _, records := assigneeFixture(t, structureRole(t, base, d.FleetManager, d.OrgScope, ""))
		if err := structurePrepare(t, f); err != nil {
			t.Fatal(err)
		}
		if want := []string{"authority", "resource", "proof", "capability", "record"}; !reflect.DeepEqual(*f.events, want) || records.facts.AssignedTo != "" {
			t.Fatal("ORG request changed its steps or facts", *f.events)
		}
	})
	t.Run("assigned record", func(t *testing.T) {
		f, source, records := assigneeFixture(t, self)
		if err := structurePrepare(t, f); err != nil {
			t.Fatal(err)
		}
		if want := []string{"authority", "resource", "assignee", "proof", "capability", "record"}; !reflect.DeepEqual(*f.events, want) {
			t.Fatal(*f.events)
		}
		if source.policy != f.policy || source.member != member || !source.at.Equal(time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)) || source.received.ID != target || source.received.AssignedTo != "" {
			t.Fatal("source did not receive the policy, member, Prepare instant and unresolved facts")
		}
		if records.facts.AssignedTo != member || records.facts.AuthoredBy != "" || records.facts.UnitID != "" {
			t.Fatal("record gate did not receive exactly the assignee fact", records.facts)
		}
	})
	t.Run("own record from the resolver needs no lookup", func(t *testing.T) {
		f, _, _ := assigneeFixture(t, self)
		f.resource.value.AssignedTo = member
		if err := structurePrepare(t, f); err != nil || slices.Contains(*f.events, "assignee") {
			t.Fatal("resolver-own record consulted the source", *f.events, err)
		}
	})
	for _, tc := range []struct {
		name   string
		roles  []d.Assignment
		change func(*fixture, *assigneeSource)
		want   error
	}{
		{"not assigned", []d.Assignment{self}, func(_ *fixture, s *assigneeSource) { s.assigned = false }, d.ErrPermissionDenied},
		{"no source configured fails closed", []d.Assignment{self}, func(f *fixture, _ *assigneeSource) { f.service.Assignee = nil }, d.ErrPermissionDenied},
		{"private source failure", []d.Assignment{self}, func(_ *fixture, s *assigneeSource) { s.err = errors.New("synthetic private SQL detail") }, a.ErrUnavailable},
		{"self grant without the permission", []d.Assignment{self}, func(f *fixture, _ *assigneeSource) {
			f.policy = a.Policy{Permission: "fleet.asset.manage", Capability: "organizations", Effect: a.ConfigurationWrite, Fresh: true}
		}, d.ErrPermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, source, _ := assigneeFixture(t, tc.roles...)
			if tc.change != nil {
				tc.change(&f, source)
			}
			err := structurePrepare(t, f)
			if !errors.Is(err, tc.want) {
				t.Fatal(err, tc.want)
			}
			if slices.Contains(*f.events, "proof") {
				t.Fatal("refused assignee reached later gates", *f.events)
			}
		})
	}
}
