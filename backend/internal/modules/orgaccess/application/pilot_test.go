package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	a "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

type states struct {
	state cap.State
	err   error
	calls int
	scope kernel.OrgScope
}

func (s *states) State(_ context.Context, scope kernel.OrgScope, _ string) (cap.State, error) {
	s.calls++
	s.scope = scope
	return s.state, s.err
}
func TestPilotCapabilitiesStayIndependentFromRolesAndPreserveReads(t *testing.T) {
	for _, tc := range []struct {
		name   string
		global bool
		state  cap.State
		effect a.Effect
		want   error
	}{
		{"zero activation", false, cap.State{Enabled: true, Revision: 1}, a.ConfigurationWrite, a.ErrEntitlementRequired},
		{"absent", true, cap.State{}, a.ConfigurationWrite, a.ErrEntitlementRequired},
		{"off", true, cap.State{Revision: 2}, a.ConfigurationWrite, a.ErrEntitlementRequired},
		{"enabled", true, cap.State{Enabled: true, Revision: 3}, a.ConfigurationWrite, nil},
		{"retained read", true, cap.State{Revision: 2}, a.Read, nil},
		{"reduce access", true, cap.State{}, a.ReduceAccess, nil},
		{"regularize", true, cap.State{}, a.RegularizePlan, nil},
		{"restore", true, cap.State{}, a.RestoreOrganization, nil},
		{"corrupt enabled", true, cap.State{Enabled: true}, a.ConfigurationWrite, a.ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			catalog, err := cap.NewCatalog([]string{"identity"})
			if err != nil {
				t.Fatal(err)
			}
			state := &states{state: tc.state}
			f.service.Capabilities = adapters.PilotEntitlements{Enabled: tc.global, Catalog: catalog, States: state}
			f.policy.Effect = tc.effect
			ctx, finish := a.BeginRequest(t.Context())
			defer finish()
			pending, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.service.Complete(ctx, pending, f.proof)
			if !errors.Is(err, tc.want) {
				t.Fatal(err, tc.want)
			}
			if tc.global && state.scope != f.scope {
				t.Fatal("wrong capability scope")
			}
		})
	}
	f := setup(t)
	catalog, _ := cap.NewCatalog([]string{"identity"})
	state := &states{state: cap.State{Enabled: true, Revision: 1}}
	f.service.Capabilities = adapters.PilotEntitlements{Enabled: true, Catalog: catalog, States: state}
	f.source.value.Assignments = nil
	ctx, finish := a.BeginRequest(t.Context())
	defer finish()
	if _, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource); err == nil || state.calls != 0 {
		t.Fatal("flag granted a role", err)
	}
}

func TestPilotMissingUnknownCorruptAndForeignStatesFailClosed(t *testing.T) {
	f := setup(t)
	catalog, err := cap.NewCatalog([]string{"identity"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, finish := a.BeginRequest(t.Context())
	defer finish()
	pending, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		state   *states
		catalog cap.Catalog
	}{
		{"missing", nil, catalog}, {"unknown", &states{state: cap.State{Enabled: true, Revision: 1}}, cap.Catalog{}},
		{"private error", &states{err: errors.New("synthetic private database detail")}, catalog},
		{"negative revision", &states{state: cap.State{Revision: -1}}, catalog},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gate := adapters.PilotEntitlements{Enabled: true, Catalog: tc.catalog}
			if tc.state != nil {
				gate.States = tc.state
			}
			if err := gate.Check(ctx, f.scope, pending.Subject(), f.policy); err != a.ErrUnavailable {
				t.Fatal(err)
			}
		})
	}
	state := &states{state: cap.State{Enabled: true, Revision: 1}}
	gate := adapters.PilotEntitlements{Enabled: true, Catalog: catalog, States: state}
	id, err := kernel.ParseOrgID(member)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.Check(ctx, foreign, pending.Subject(), f.policy); err != a.ErrUnavailable || state.calls != 0 {
		t.Fatal("foreign scope inspected flag", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := gate.Check(cancelled, f.scope, pending.Subject(), f.policy); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPilotTypedNilProductionReaderFailsClosed(t *testing.T) {
	f := setup(t)
	catalog, err := cap.NewCatalog([]string{"identity"})
	if err != nil {
		t.Fatal(err)
	}
	var missing *cappg.Store
	f.service.Capabilities = adapters.PilotEntitlements{Enabled: true, Catalog: catalog, States: missing}
	ctx, finish := a.BeginRequest(t.Context())
	defer finish()
	pending, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Complete(ctx, pending, f.proof); err != a.ErrUnavailable {
		t.Fatal(err)
	}
}

type planCapabilitySource func(context.Context, kernel.OrgScope, string) (bool, bool, error)

func (f planCapabilitySource) PlanCapability(ctx context.Context, scope kernel.OrgScope, capability string) (bool, bool, error) {
	return f(ctx, scope, capability)
}

func TestPlanCapabilitiesSupplementTheOrganizationRoleGate(t *testing.T) {
	for _, tc := range []struct {
		name              string
		pilot             cap.State
		governed, allowed bool
		role              bool
		effect            a.Effect
		failure           error
		want              error
		wantCalls         int
	}{
		{name: "pilot stays authoritative without billing", pilot: cap.State{Enabled: true, Revision: 1},
			role: true, effect: a.OperationalWrite, wantCalls: 1},
		{name: "active plan permits covered write", governed: true, allowed: true,
			role: true, effect: a.OperationalWrite, wantCalls: 1},
		{name: "explicit operator denial still applies", pilot: cap.State{Revision: 2}, governed: true, allowed: true,
			role: true, effect: a.OperationalWrite, want: a.ErrEntitlementRequired, wantCalls: 1},
		{name: "ended subscription cannot fall back to pilot flag", pilot: cap.State{Enabled: true, Revision: 1}, governed: true,
			role: true, effect: a.OperationalWrite, want: a.ErrEntitlementRequired, wantCalls: 1},
		{name: "permitted reads do not depend on billing", governed: true, failure: errors.New("synthetic unavailable"),
			role: true, effect: a.Read},
		{name: "unavailable plan facts block a write", pilot: cap.State{Enabled: true, Revision: 1}, role: true,
			failure: errors.New("synthetic unavailable"), effect: a.OperationalWrite, want: a.ErrUnavailable, wantCalls: 1},
		{name: "payment never creates a role", governed: true, allowed: true, effect: a.OperationalWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			f.policy.Effect = tc.effect
			if !tc.role {
				f.source.value.Assignments = nil
			}
			catalog, err := cap.NewCatalog([]string{"identity"})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			f.service.Capabilities = adapters.PilotEntitlements{Enabled: true, Catalog: catalog, States: &states{state: tc.pilot},
				Plans: planCapabilitySource(func(_ context.Context, org kernel.OrgScope, code string) (bool, bool, error) {
					calls++
					if org != f.scope || code != f.policy.Capability {
						t.Fatal("wrong scope for plan evaluation")
					}
					return tc.governed, tc.allowed, tc.failure
				})}
			ctx, finish := a.BeginRequest(t.Context())
			defer finish()
			pending, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource)
			if tc.role && err == nil {
				_, err = f.service.Complete(ctx, pending, f.proof)
			}
			if tc.role && !errors.Is(err, tc.want) || !tc.role && err == nil {
				t.Fatalf("gate result=%v want=%v role=%t", err, tc.want, tc.role)
			}
			if calls != tc.wantCalls {
				t.Fatalf("billing source calls=%d want=%d", calls, tc.wantCalls)
			}
		})
	}
}
