package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	m "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	a "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	o "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestConcurrentCompletionHasOneProofCapabilityAndRecordCheck(t *testing.T) {
	f := setup(t)
	ctx, finish := a.BeginRequest(t.Context())
	defer finish()
	actor := a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}
	pending, err := f.service.Prepare(ctx, f.scope, actor, f.policy, f.resource)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.service.Complete(ctx, pending, f.proof); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if err != a.ErrInvalidContext {
			t.Fatal(err)
		}
	}
	if success != 1 || !reflect.DeepEqual(*f.events, []string{"authority", "resource", "proof", "capability", "record"}) {
		t.Fatal(success, *f.events)
	}
}

func TestAuthorizationRejectsMalformedProofMetadataAndMissingGates(t *testing.T) {
	for _, actor := range []a.Actor{{}, {AccountID: account, Channel: "android"}, {AccountID: account, Channel: "web", DeviceKeyID: employee}, {AccountID: account, Channel: "unknown"}} {
		f := setup(t)
		ctx, finish := a.BeginRequest(t.Context())
		if _, err := f.service.Prepare(ctx, f.scope, actor, f.policy, f.resource); err != a.ErrUnauthenticated {
			t.Fatal(err)
		}
		finish()
	}
	for _, change := range []func(*fixture){func(f *fixture) { f.service.Now = nil }, func(f *fixture) { f.service.Now = func() time.Time { return time.Time{} } }, func(f *fixture) { f.policy.Permission = "unknown.key" }, func(f *fixture) { f.policy.Capability = "" }, func(f *fixture) { f.policy.Effect = 0 }, func(f *fixture) { f.policy.Permission = "auth.role.assign"; f.policy.Fresh = false }, func(f *fixture) { f.source.value.Model = "invalid" }, func(f *fixture) { f.source.value.MembershipID = "bad" }, func(f *fixture) { f.source.value.MembershipState = "unknown" }, func(f *fixture) { f.source.value.OrganizationState = "unknown" }, func(f *fixture) { f.source.value.Assignments = []d.Assignment{{}} }} {
		f := setup(t)
		change(&f)
		ctx, finish := a.BeginRequest(t.Context())
		if _, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource); err != a.ErrUnavailable {
			t.Fatal(err)
		}
		finish()
	}
	for _, missing := range []string{"proof", "record"} {
		f := setup(t)
		ctx, finish := a.BeginRequest(t.Context())
		p, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource)
		if err != nil {
			t.Fatal(err)
		}
		var proofGate a.ProofGate = f.proof
		if missing == "proof" {
			proofGate = nil
		} else {
			f.service.Records = nil
		}
		if _, err := f.service.Complete(ctx, p, proofGate); err != a.ErrUnavailable {
			t.Fatal(err)
		}
		finish()
	}
	f := setup(t)
	ctx, finish := a.BeginRequest(t.Context())
	f.source.value.Model = m.ManagedAccess
	if _, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "web"}, f.policy, f.resource); err != a.ErrNoMembership {
		t.Fatal("managed web access accepted", err)
	}
	finish()
}

func TestAuthorizationFormattingAndCancelledRequestRetainNoAuthority(t *testing.T) {
	f := setup(t)
	ctx, finish := a.BeginRequest(t.Context())
	defer finish()
	actor := a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}
	pending, err := f.service.Prepare(ctx, f.scope, actor, f.policy, f.resource)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := f.service.Complete(ctx, pending, f.proof)
	if err != nil {
		t.Fatal(err)
	}
	if s := decision.Subject(); s.AccountID() != account || s.EmployeeID() != employee || s.ContributorID() != member || s.DeviceKeyID() != employee || s.Model() != m.ExistingAccount || pending.Policy() != f.policy {
		t.Fatal("attribution lost")
	}
	for _, v := range []any{actor, f.source.value, pending, decision, decision.Subject()} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			text := fmt.Sprintf(verb, v)
			if strings.Contains(text, account) || strings.Contains(text, member) {
				t.Fatal("format leaked authority")
			}
		}
		encoded, err := json.Marshal(v)
		if err != nil || strings.Contains(string(encoded), account) {
			t.Fatal("JSON leaked authority")
		}
		if logger, ok := v.(slog.LogValuer); !ok || strings.Contains(logger.LogValue().String(), member) {
			t.Fatal("slog leaked authority")
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := decision.Check(cancelled, f.scope, account, f.policy.Permission, target); err != a.ErrInvalidContext {
		t.Fatal(err)
	}
	if _, err := f.service.Prepare(t.Context(), f.scope, actor, f.policy, f.resource); err != a.ErrInvalidContext {
		t.Fatal("missing request lease accepted", err)
	}
	if err := (a.Decision{}).Check(ctx, f.scope, account, f.policy.Permission, target); err != a.ErrInvalidContext {
		t.Fatal(err)
	}
}

const (
	account  = "7b0e2a54-0000-4000-8000-000000000701"
	member   = "7b0e2a54-0000-4000-8000-000000000601"
	employee = "7b0e2a54-0000-4000-8000-000000000801"
	target   = "7b0e2a54-0000-4000-8000-000000000901"
)

type source struct {
	value  a.Authority
	err    error
	events *[]string
}

func (s *source) LoadAuthority(context.Context, kernel.OrgScope, string) (a.Authority, error) {
	*s.events = append(*s.events, "authority")
	return s.value, s.err
}

type resource struct {
	value  d.ResourceFacts
	err    error
	events *[]string
}

func (r *resource) Resolve(context.Context, kernel.OrgScope, a.Subject) (d.ResourceFacts, error) {
	*r.events = append(*r.events, "resource")
	return r.value, r.err
}

type proof struct {
	err    error
	events *[]string
	fresh  bool
}

func (p *proof) Confirm(_ context.Context, _ a.Subject, policy a.Policy) error {
	*p.events = append(*p.events, "proof")
	p.fresh = policy.Fresh
	return p.err
}

type capability struct {
	err    error
	events *[]string
}

func (c *capability) Check(context.Context, kernel.OrgScope, a.Subject, a.Policy) error {
	*c.events = append(*c.events, "capability")
	return c.err
}

type records struct {
	err    error
	events *[]string
}

func (r *records) Check(context.Context, kernel.OrgScope, a.Subject, a.Policy, d.ResourceFacts, []d.Rule) error {
	*r.events = append(*r.events, "record")
	return r.err
}

type fixture struct {
	service    a.Authorization
	source     *source
	resource   *resource
	proof      *proof
	capability *capability
	records    *records
	scope      kernel.OrgScope
	events     *[]string
	policy     a.Policy
}

func setup(t *testing.T) fixture {
	t.Helper()
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	role, err := d.RehydrateAssignment(scope, d.AssignmentSnapshot{OrgID: id, ID: target, MembershipID: member, AssignedBy: member, Role: d.Owner, Scope: d.OrgScope, TargetID: id.String(), ValidFrom: now.Add(-time.Hour), Reason: "synthetic authority", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	events := []string{}
	s := &source{value: a.Authority{OrgID: id, AccountID: account, MembershipID: member, EmployeeID: employee, MembershipState: m.MembershipActive, EmployeeState: m.EmployeeActive, Model: m.ExistingAccount, OrganizationState: o.Active, Assignments: []d.Assignment{role}}, events: &events}
	r := &resource{value: d.ResourceFacts{OrgID: id, ID: target}, events: &events}
	p := &proof{events: &events}
	c := &capability{events: &events}
	record := &records{events: &events}
	return fixture{a.Authorization{Source: s, Capabilities: c, Records: record, Now: func() time.Time { return now }}, s, r, p, c, record, scope, &events, a.Policy{Permission: "idn.employee.read", Capability: "identity", Effect: a.Read}}
}
func TestAuthorizationOrdersGatesAndBindsDecisionToLiveRequest(t *testing.T) {
	f := setup(t)
	ctx, finish := a.BeginRequest(t.Context())
	defer finish()
	prepared, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := f.service.Complete(ctx, prepared, f.proof)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"authority", "resource", "proof", "capability", "record"}; !reflect.DeepEqual(*f.events, want) {
		t.Fatal(*f.events)
	}
	if err := decision.Check(ctx, f.scope, account, f.policy.Permission, target); err != nil {
		t.Fatal(err)
	}
	otherCtx, otherFinish := a.BeginRequest(ctx)
	defer otherFinish()
	if err := decision.Check(otherCtx, f.scope, account, f.policy.Permission, target); err != a.ErrInvalidContext {
		t.Fatal("decision reused in another request", err)
	}
	if err := decision.Check(ctx, f.scope, member, f.policy.Permission, target); err != a.ErrInvalidContext {
		t.Fatal(err)
	}
	if err := decision.Check(ctx, f.scope, account, "trip.trip.read", target); err != a.ErrInvalidContext {
		t.Fatal(err)
	}
	if err := decision.Check(ctx, f.scope, account, f.policy.Permission, member); err != a.ErrInvalidContext {
		t.Fatal(err)
	}
	if _, err := f.service.Complete(ctx, prepared, f.proof); err != a.ErrInvalidContext {
		t.Fatal("preparation reused", err)
	}
	finish()
	if err := decision.Check(ctx, f.scope, account, f.policy.Permission, target); err != a.ErrInvalidContext {
		t.Fatal("finished request retained authority", err)
	}
}
func TestAuthorizationFailClosedAtEveryGate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fixture)
		want   error
		events []string
	}{
		{"missing source", func(f *fixture) { f.service.Source = nil }, a.ErrUnavailable, nil},
		{"missing membership", func(f *fixture) { f.source.err = a.ErrNotFound }, a.ErrNoMembership, []string{"authority"}},
		{"wrong account", func(f *fixture) { f.source.value.AccountID = member }, a.ErrUnavailable, []string{"authority"}},
		{"suspended", func(f *fixture) { f.source.value.MembershipState = m.MembershipSuspended }, a.ErrMembershipSuspended, []string{"authority"}},
		{"ended", func(f *fixture) { f.source.value.MembershipState = m.MembershipEnded }, a.ErrNoMembership, []string{"authority"}},
		{"org suspended write", func(f *fixture) { f.source.value.OrganizationState = o.Suspended; f.policy.Effect = a.OperationalWrite }, a.ErrOrgSuspended, []string{"authority"}},
		{"org archived write", func(f *fixture) { f.source.value.OrganizationState = o.Archived; f.policy.Effect = a.OperationalWrite }, a.ErrOrgArchived, []string{"authority"}},
		{"pending operational", func(f *fixture) {
			f.source.value.OrganizationState = o.PendingSetup
			f.policy.Effect = a.OperationalWrite
		}, a.ErrSetupIncomplete, []string{"authority"}},
		{"closed", func(f *fixture) { f.source.value.OrganizationState = o.Closed }, a.ErrNoMembership, []string{"authority"}},
		{"role revoked", func(f *fixture) { f.source.value.Assignments = nil }, d.ErrPermissionDenied, []string{"authority"}},
		{"registered unassigned", func(f *fixture) { f.policy.Permission = "auth.role.read" }, d.ErrPermissionDenied, []string{"authority"}},
		{"unknown resource", func(f *fixture) { f.resource.err = a.ErrNotFound }, d.ErrResourceNotFound, []string{"authority", "resource"}},
		{"foreign resource", func(f *fixture) { id, _ := kernel.ParseOrgID(member); f.resource.value.OrgID = id }, d.ErrResourceNotFound, []string{"authority", "resource"}},
		{"proof revoked", func(f *fixture) { f.proof.err = a.ErrUnauthenticated }, a.ErrUnauthenticated, []string{"authority", "resource", "proof"}},
		{"fresh proof required", func(f *fixture) { f.proof.err = a.ErrFreshAuthRequired }, a.ErrFreshAuthRequired, []string{"authority", "resource", "proof"}},
		{"capability missing", func(f *fixture) { f.capability.err = a.ErrEntitlementRequired }, a.ErrEntitlementRequired, []string{"authority", "resource", "proof", "capability"}},
		{"limit", func(f *fixture) { f.capability.err = a.ErrLimitReached }, a.ErrLimitReached, []string{"authority", "resource", "proof", "capability"}},
		{"missing capability provider", func(f *fixture) { f.service.Capabilities = nil }, a.ErrUnavailable, []string{"authority", "resource", "proof"}},
		{"record self approval", func(f *fixture) { f.records.err = a.ErrSelfApproval }, a.ErrSelfApproval, []string{"authority", "resource", "proof", "capability", "record"}},
		{"record failure private detail", func(f *fixture) { f.records.err = errors.New("synthetic private SQL detail") }, a.ErrUnavailable, []string{"authority", "resource", "proof", "capability", "record"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			tc.change(&f)
			ctx, finish := a.BeginRequest(t.Context())
			defer finish()
			pending, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource)
			if err == nil {
				_, err = f.service.Complete(ctx, pending, f.proof)
			}
			if !errors.Is(err, tc.want) {
				t.Fatal(err, tc.want)
			}
			if !reflect.DeepEqual(*f.events, tc.events) && !(len(*f.events) == 0 && len(tc.events) == 0) {
				t.Fatal(*f.events, tc.events)
			}
		})
	}
}
func TestOrganizationReadAndReductionExceptionsKeepOtherGates(t *testing.T) {
	for _, state := range []o.State{o.Suspended, o.Archived} {
		for _, effect := range []a.Effect{a.Read, a.ReduceAccess, a.RegularizePlan} {
			f := setup(t)
			f.source.value.OrganizationState = state
			f.policy.Effect = effect
			ctx, finish := a.BeginRequest(t.Context())
			pending, err := f.service.Prepare(ctx, f.scope, a.Actor{AccountID: account, Channel: "android", ContributorID: member, DeviceKeyID: employee}, f.policy, f.resource)
			if err != nil {
				t.Fatal(state, effect, err)
			}
			f.capability.err = a.ErrEntitlementRequired
			if _, err := f.service.Complete(ctx, pending, f.proof); err != a.ErrEntitlementRequired {
				t.Fatal("state exception bypassed capability", err)
			}
			finish()
		}
	}
}
