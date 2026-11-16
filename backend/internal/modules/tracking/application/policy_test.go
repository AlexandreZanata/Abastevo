package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
)

func policyID(n int) string { return fmt.Sprintf("a7400000-0000-4000-8000-%012d", n) }

func policyScope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(policyID(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type policyAuthority struct{ value access.Authority }

func (s policyAuthority) LoadAuthority(context.Context, kernel.OrgScope, string) (access.Authority, error) {
	return s.value, nil
}

type policyTarget struct{}

func (policyTarget) Resolve(_ context.Context, s kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{OrgID: s.OrgID(), ID: s.OrgID().String()}, nil
}

type policyProof struct{}

func (policyProof) Confirm(context.Context, access.Subject, access.Policy) error { return nil }

type policyCapability struct{}

func (policyCapability) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return nil
}

type policyRecords struct{}

func (policyRecords) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return nil
}

// policyDecision completes a real OWNER decision for one action.
func policyDecision(t *testing.T, action app.PolicyAction) (context.Context, kernel.OrgScope, app.PolicyActor, access.Decision) {
	t.Helper()
	s := policyScope(t, 1)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	role, err := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: policyID(6), MembershipID: policyID(4), AssignedBy: policyID(4), Role: permissions.Owner, Scope: permissions.OrgScope, TargetID: s.OrgID().String(), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic owner fixture", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := access.Authorization{Source: policyAuthority{access.Authority{OrgID: s.OrgID(), AccountID: policyID(2), MembershipID: policyID(4), EmployeeID: policyID(5), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: policyCapability{}, Records: policyRecords{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	t.Cleanup(finish)
	prepared, err := a.Prepare(ctx, s, access.Actor{AccountID: policyID(2), Channel: "web"}, app.PolicySettingsPolicy(action), policyTarget{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Complete(ctx, prepared, policyProof{})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, s, app.PolicyActor{AccountID: policyID(2), SessionID: policyID(3)}, d
}

type policyRepository struct {
	ack      app.PolicyAck
	view     app.PolicyView
	inForce  app.PolicyInForce
	err      error
	calls    int
	member   string
	replayed bool
}

func (r *policyRepository) ChangePolicy(_ context.Context, _ kernel.OrgScope, _ app.PolicyActor, member string, _ app.PolicyCommand) (app.PolicyAck, bool, error) {
	r.calls++
	r.member = member
	return r.ack, r.replayed, r.err
}

func (r *policyRepository) ReadPolicy(_ context.Context, _ kernel.OrgScope, _ app.PolicyActor, member string) (app.PolicyView, error) {
	r.calls++
	r.member = member
	return r.view, r.err
}

func (r *policyRepository) ReadPolicyInForce(_ context.Context, _ kernel.OrgScope, _ app.PolicyActor, member string) (app.PolicyInForce, error) {
	r.calls++
	r.member = member
	return r.inForce, r.err
}

func text(s string) *string { return &s }

var noticeAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func enabled(policy string) app.PolicyCommandInput {
	at := noticeAt
	return app.PolicyCommandInput{OperationID: policyID(30), ExpectedRevision: 0, Policy: policy, NoticeVersion: text("notice-2026.11"), NoticeInForceAt: &at}
}

func fieldOf(t *testing.T, err error) (string, string) {
	t.Helper()
	var field *app.PolicyField
	if !errors.As(err, &field) {
		t.Fatal("expected a policy field refusal", err)
	}
	return field.Field, field.Code
}

// B-BR-EMP-467/469: the read is org.settings.read, the change a fresh
// configuration write under org.settings.manage, exactly the ORG-008
// settings policies; no tracking key is invented.
func TestTrackingPolicyPoliciesAreTheSettingsPolicies(t *testing.T) {
	if app.PolicySettingsPolicy(app.PolicyRead) != orgapp.SettingsPolicy(false) || app.PolicySettingsPolicy(app.PolicyManage) != orgapp.SettingsPolicy(true) {
		t.Fatal("tracking policy authority drifted from the settings policies")
	}
	if p := app.PolicySettingsPolicy(app.PolicyManage); !p.Fresh || p.Effect != access.ConfigurationWrite || p.Permission != "org.settings.manage" || p.Validate() != nil {
		t.Fatal("change policy", p)
	}
	if p := app.PolicySettingsPolicy(app.PolicyRead); p.Fresh || p.Effect != access.Read || p.Permission != "org.settings.read" || p.Validate() != nil {
		t.Fatal("read policy", p)
	}
	if p := app.PolicySettingsPolicy(0); p != (access.Policy{}) {
		t.Fatal("an unknown action has a policy")
	}
}

// B-BR-EMP-467/468: the command shape, the notice rule and the digest.
func TestTrackingPolicyCommandShapeNoticeAndDigest(t *testing.T) {
	off := app.PolicyCommandInput{OperationID: policyID(31), ExpectedRevision: 2, Policy: "OFF"}
	far := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, in := range []app.PolicyCommandInput{off, enabled("OPTIONAL"), enabled("EXPECTED")} {
		c, err := app.NewPolicyCommand(in)
		if err != nil {
			t.Fatal("a valid change is refused", in.Policy, err)
		}
		v, err := c.Snapshot()
		if err != nil || v.Policy != in.Policy || v.ExpectedRevision != in.ExpectedRevision || c.Enabled() != (in.Policy != "OFF") {
			t.Fatal("snapshot drift", err)
		}
	}
	// The snapshot is independent of the caller's pointers.
	in := enabled("EXPECTED")
	c, _ := app.NewPolicyCommand(in)
	*in.NoticeVersion, *in.NoticeInForceAt = "changed", noticeAt.Add(time.Hour)
	if v, _ := c.Snapshot(); *v.NoticeVersion != "notice-2026.11" || !v.NoticeInForceAt.Equal(noticeAt) {
		t.Fatal("a command shares the caller's notice")
	}
	if _, err := (app.PolicyCommand{}).Snapshot(); err == nil {
		t.Fatal("a zero command has a snapshot")
	}
	malformed := map[string]app.PolicyCommandInput{
		"operation":        {OperationID: "x", Policy: "OFF"},
		"upper operation":  {OperationID: strings.ToUpper(policyID(30)), Policy: "OFF"},
		"negative":         {OperationID: policyID(30), ExpectedRevision: -1, Policy: "OFF"},
		"revision too big": {OperationID: policyID(30), ExpectedRevision: app.MaxPolicyRevision + 1, Policy: "OFF"},
		"far instant":      {OperationID: policyID(30), Policy: "EXPECTED", NoticeVersion: text("notice-1"), NoticeInForceAt: &far},
		"zero instant":     {OperationID: policyID(30), Policy: "EXPECTED", NoticeVersion: text("notice-1"), NoticeInForceAt: &time.Time{}},
	}
	for name, in := range malformed {
		if _, err := app.NewPolicyCommand(in); !errors.Is(err, app.ErrMalformed) {
			t.Fatal("malformed change admitted", name, err)
		}
	}
	at := noticeAt
	refused := map[string]struct {
		in          app.PolicyCommandInput
		field, code string
	}{
		"unknown policy":   {app.PolicyCommandInput{OperationID: policyID(30), Policy: "ON"}, "policy", app.CodePolicyValue},
		"lower policy":     {app.PolicyCommandInput{OperationID: policyID(30), Policy: "expected"}, "policy", app.CodePolicyValue},
		"off with version": {app.PolicyCommandInput{OperationID: policyID(30), Policy: "OFF", NoticeVersion: text("notice-1")}, "notice_version", app.CodeAbsentWhenOff},
		"off with instant": {app.PolicyCommandInput{OperationID: policyID(30), Policy: "OFF", NoticeInForceAt: &at}, "notice_in_force_at", app.CodeAbsentWhenOff},
		"no version":       {app.PolicyCommandInput{OperationID: policyID(30), Policy: "EXPECTED", NoticeInForceAt: &at}, "notice_version", app.CodeRequiredWhenEnabled},
		"no instant":       {app.PolicyCommandInput{OperationID: policyID(30), Policy: "OPTIONAL", NoticeVersion: text("notice-1")}, "notice_in_force_at", app.CodeRequiredWhenEnabled},
		"upper version":    {app.PolicyCommandInput{OperationID: policyID(30), Policy: "EXPECTED", NoticeVersion: text("Notice-1"), NoticeInForceAt: &at}, "notice_version", app.CodeNoticeVersionFormat},
		"blank version":    {app.PolicyCommandInput{OperationID: policyID(30), Policy: "EXPECTED", NoticeVersion: text(""), NoticeInForceAt: &at}, "notice_version", app.CodeNoticeVersionFormat},
		"long version":     {app.PolicyCommandInput{OperationID: policyID(30), Policy: "EXPECTED", NoticeVersion: text(strings.Repeat("a", 65)), NoticeInForceAt: &at}, "notice_version", app.CodeNoticeVersionFormat},
		"spaced version":   {app.PolicyCommandInput{OperationID: policyID(30), Policy: "EXPECTED", NoticeVersion: text("notice 1"), NoticeInForceAt: &at}, "notice_version", app.CodeNoticeVersionFormat},
	}
	for name, c := range refused {
		_, err := app.NewPolicyCommand(c.in)
		if field, code := fieldOf(t, err); field != c.field || code != c.code {
			t.Fatal("wrong refusal", name, field, code)
		}
	}
	// The digest is 64 hex, bound to the organization and to every field.
	digest := func(in app.PolicyCommandInput, s kernel.OrgScope) string {
		c, err := app.NewPolicyCommand(in)
		if err != nil {
			t.Fatal(err)
		}
		return c.Digest(s)
	}
	base := digest(enabled("EXPECTED"), policyScope(t, 1))
	if len(base) != 64 || strings.Trim(base, "0123456789abcdef") != "" || base != digest(enabled("EXPECTED"), policyScope(t, 1)) {
		t.Fatal("digest is not stable 64 hex", base)
	}
	other := enabled("EXPECTED")
	other.ExpectedRevision = 1
	later := enabled("EXPECTED")
	moved := noticeAt.Add(time.Second)
	later.NoticeInForceAt = &moved
	version := enabled("EXPECTED")
	version.NoticeVersion = text("notice-2026.12")
	for name, d := range map[string]string{"organization": digest(enabled("EXPECTED"), policyScope(t, 9)), "policy": digest(enabled("OPTIONAL"), policyScope(t, 1)),
		"revision": digest(other, policyScope(t, 1)), "instant": digest(later, policyScope(t, 1)), "version": digest(version, policyScope(t, 1))} {
		if d == base {
			t.Fatal("digest ignores", name)
		}
	}
	if (app.PolicyCommand{}).Digest(policyScope(t, 1)) != "" {
		t.Fatal("a zero command has a digest")
	}
}

// B-BR-EMP-467/469: the service binds the completed decision before and
// after the repository and accepts only coherent results.
func TestTrackingPolicyServiceBindsDecisionsAndResults(t *testing.T) {
	ctx, s, actor, manage := policyDecision(t, app.PolicyManage)
	command, err := app.NewPolicyCommand(enabled("EXPECTED"))
	if err != nil {
		t.Fatal(err)
	}
	effective := noticeAt.Add(time.Hour)
	good := app.PolicyAck{Revision: 1, Policy: domain.PolicyExpected, NoticeVersion: "notice-2026.11", NoticeInForce: &noticeAt, EffectiveAt: effective}
	repo := &policyRepository{ack: good}
	if _, _, err := (app.PolicyService{Repository: repo}).Change(ctx, s, actor, manage, command); !errors.Is(err, app.ErrPolicyDisabled) || repo.calls != 0 {
		t.Fatal("a disabled service reached the repository", err)
	}
	service := app.PolicyService{Enabled: true, Repository: repo}
	ack, replayed, err := service.Change(ctx, s, actor, manage, command)
	if err != nil || ack.Revision != 1 || replayed || repo.calls != 1 || repo.member != policyID(4) {
		t.Fatal("valid change", err, ack.Revision, repo.calls, repo.member)
	}
	repo.replayed = true
	if _, replayed, err := service.Change(ctx, s, actor, manage, command); err != nil || !replayed {
		t.Fatal("replay flag lost", err)
	}
	readCtx, _, _, read := policyDecision(t, app.PolicyRead)
	repo.calls = 0
	if _, _, err := service.Change(readCtx, s, actor, read, command); !errors.Is(err, app.ErrPolicyUnauthorized) || repo.calls != 0 {
		t.Fatal("a read decision changed the policy", err)
	}
	if _, _, err := service.Change(ctx, policyScope(t, 9), actor, manage, command); !errors.Is(err, app.ErrPolicyUnauthorized) || repo.calls != 0 {
		t.Fatal("another organization's scope used the decision", err)
	}
	if _, _, err := service.Change(ctx, s, app.PolicyActor{AccountID: policyID(2)}, manage, command); !errors.Is(err, app.ErrPolicyUnauthorized) || repo.calls != 0 {
		t.Fatal("an actor without proof changed the policy", err)
	}
	if _, _, err := service.Change(ctx, s, actor, manage, app.PolicyCommand{}); err == nil || repo.calls != 0 {
		t.Fatal("an unvalidated command reached the repository")
	}
	for name, bad := range map[string]app.PolicyAck{
		"skipped revision": {Revision: 2, Policy: domain.PolicyExpected, NoticeVersion: "notice-2026.11", NoticeInForce: &noticeAt, EffectiveAt: effective},
		"other policy":     {Revision: 1, Policy: domain.PolicyOptional, NoticeVersion: "notice-2026.11", NoticeInForce: &noticeAt, EffectiveAt: effective},
		"other notice":     {Revision: 1, Policy: domain.PolicyExpected, NoticeVersion: "notice-x", NoticeInForce: &noticeAt, EffectiveAt: effective},
		"notice later":     {Revision: 1, Policy: domain.PolicyExpected, NoticeVersion: "notice-2026.11", NoticeInForce: &noticeAt, EffectiveAt: noticeAt.Add(-time.Second)},
		"no effective":     {Revision: 1, Policy: domain.PolicyExpected, NoticeVersion: "notice-2026.11", NoticeInForce: &noticeAt},
	} {
		repo.ack, repo.replayed = bad, false
		if _, _, err := service.Change(ctx, s, actor, manage, command); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("an incoherent result was accepted", name, err)
		}
	}
	repo.ack, repo.err = good, app.ErrPolicyConflict
	if _, _, err := service.Change(ctx, s, actor, manage, command); !errors.Is(err, app.ErrPolicyConflict) {
		t.Fatal("conflict lost", err)
	}

	// Reads.
	repo.err, repo.calls = nil, 0
	repo.view = app.PolicyView{Policy: domain.PolicyOff, InForce: domain.PolicyOff}
	if v, err := service.Read(readCtx, s, actor, read); err != nil || v.Revision != 0 || v.Policy != domain.PolicyOff || repo.calls != 1 {
		t.Fatal("empty read", err)
	}
	repo.view = app.PolicyView{Revision: 3, Policy: domain.PolicyExpected, NoticeVersion: "notice-2026.11", NoticeInForce: &noticeAt, EffectiveAt: &effective, InForce: domain.PolicyExpected, InForceRevision: 3}
	if v, err := service.Read(readCtx, s, actor, read); err != nil || v.Revision != 3 || v.InForce != domain.PolicyExpected {
		t.Fatal("stored read", err)
	}
	if _, err := service.Read(ctx, s, actor, manage); !errors.Is(err, app.ErrPolicyUnauthorized) {
		t.Fatal("a change decision read the policy", err)
	}
	for name, bad := range map[string]app.PolicyView{
		"empty with notice":      {Policy: domain.PolicyOff, NoticeVersion: "notice-1", InForce: domain.PolicyOff},
		"empty enabled":          {Policy: domain.PolicyExpected, InForce: domain.PolicyOff},
		"in force without row":   {Policy: domain.PolicyOff, InForce: domain.PolicyExpected, InForceRevision: 1},
		"stored without time":    {Revision: 1, Policy: domain.PolicyOff, InForce: domain.PolicyOff, InForceRevision: 1},
		"enabled without notice": {Revision: 1, Policy: domain.PolicyExpected, EffectiveAt: &effective, InForce: domain.PolicyExpected, InForceRevision: 1},
		"in force future":        {Revision: 1, Policy: domain.PolicyOff, EffectiveAt: &effective, InForce: domain.PolicyOff, InForceRevision: 2},
		"off in force enabled":   {Revision: 1, Policy: domain.PolicyOff, EffectiveAt: &effective, InForce: domain.PolicyExpected, InForceRevision: 1},
	} {
		repo.view = bad
		if _, err := service.Read(readCtx, s, actor, read); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("an incoherent view was accepted", name, err)
		}
	}
}

// Every private policy value prints redacted.
func TestTrackingPolicyValuesAreRedacted(t *testing.T) {
	c, err := app.NewPolicyCommand(enabled("EXPECTED"))
	if err != nil {
		t.Fatal(err)
	}
	in := enabled("EXPECTED")
	actor := app.PolicyActor{AccountID: policyID(2), SessionID: policyID(3)}
	view := app.PolicyView{Revision: 1, Policy: domain.PolicyExpected, NoticeVersion: "notice-2026.11"}
	for _, v := range []any{c, in, actor, view, app.PolicyAck{NoticeVersion: "notice-2026.11"}} {
		var buf bytes.Buffer
		slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "v", v)
		raw, _ := json.Marshal(v)
		for _, s := range []string{fmt.Sprint(v), fmt.Sprintf("%+v", v), string(raw), buf.String()} {
			if strings.Contains(s, "notice-2026.11") || strings.Contains(s, policyID(3)) || strings.Contains(s, policyID(30)) {
				t.Fatal("a private policy value printed", s)
			}
		}
	}
}

type policyOwnTarget struct{}

func (policyOwnTarget) Resolve(_ context.Context, s kernel.OrgScope, subject access.Subject) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{OrgID: s.OrgID(), ID: subject.MembershipID(), AssignedTo: subject.MembershipID()}, nil
}

// policyOwnDecision completes a real SELF-scoped DRIVER decision for the
// member's own read.
func policyOwnDecision(t *testing.T) (context.Context, access.Decision) {
	t.Helper()
	s := policyScope(t, 1)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	role, err := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: policyID(7), MembershipID: policyID(4), AssignedBy: policyID(4), Role: permissions.Driver, Scope: permissions.SelfScope, TargetID: policyID(4), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic driver fixture", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := access.Authorization{Source: policyAuthority{access.Authority{OrgID: s.OrgID(), AccountID: policyID(2), MembershipID: policyID(4), EmployeeID: policyID(5), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: policyCapability{}, Records: policyRecords{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	t.Cleanup(finish)
	prepared, err := a.Prepare(ctx, s, access.Actor{AccountID: policyID(2), Channel: "web"}, app.PolicySettingsPolicy(app.PolicyOwn), policyOwnTarget{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Complete(ctx, prepared, policyProof{})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, d
}

// EMP-GPS-002: the member's own read of the policy in force is a read under
// trip.trip.start (the catalog key of the driver's own trip, no invented
// key), never org.settings.read, and is bound to the member's own record.
func TestTrackingPolicyOwnReadIsTripStartOverTheOwnRecord(t *testing.T) {
	own := app.PolicySettingsPolicy(app.PolicyOwn)
	if own != (access.Policy{Permission: "trip.trip.start", Capability: "organizations", Effect: access.Read}) || app.PolicyOwnPermission != "trip.trip.start" || own.Validate() != nil {
		t.Fatal("own policy read", own)
	}
	if own == app.PolicySettingsPolicy(app.PolicyRead) || own == app.PolicySettingsPolicy(app.PolicyManage) {
		t.Fatal("the own read reuses a settings policy")
	}
	ctx, d := policyOwnDecision(t)
	s := policyScope(t, 1)
	actor := app.PolicyActor{AccountID: policyID(2), SessionID: policyID(3)}
	at := noticeAt
	repo := &policyRepository{inForce: app.PolicyInForce{Revision: 2, Policy: domain.PolicyExpected, NoticeVersion: "notice-2026.11", NoticeInForce: &at}}
	if _, err := (app.PolicyService{Repository: repo}).Mine(ctx, s, actor, d); !errors.Is(err, app.ErrPolicyDisabled) || repo.calls != 0 {
		t.Fatal("a disabled service reached the repository", err)
	}
	service := app.PolicyService{Enabled: true, Repository: repo}
	v, err := service.Mine(ctx, s, actor, d)
	if err != nil || v.Revision != 2 || v.Policy != domain.PolicyExpected || v.NoticeVersion != "notice-2026.11" || !v.NoticeInForce.Equal(noticeAt) || repo.calls != 1 || repo.member != policyID(4) {
		t.Fatal("own read", err, v.Revision, repo.calls, repo.member)
	}
	repo.inForce = app.PolicyInForce{Policy: domain.PolicyOff}
	if v, err := service.Mine(ctx, s, actor, d); err != nil || v.Revision != 0 || v.Policy != domain.PolicyOff || v.NoticeVersion != "" || v.NoticeInForce != nil {
		t.Fatal("nothing recorded", err, v.Revision)
	}
	repo.inForce = app.PolicyInForce{Revision: 3, Policy: domain.PolicyOff}
	if v, err := service.Mine(ctx, s, actor, d); err != nil || v.Revision != 3 || v.Policy != domain.PolicyOff {
		t.Fatal("turned off", err)
	}
	// Another decision, another organization or an actor without proof read nothing.
	readCtx, _, _, read := policyDecision(t, app.PolicyRead)
	repo.calls = 0
	if _, err := service.Mine(readCtx, s, actor, read); !errors.Is(err, app.ErrPolicyUnauthorized) || repo.calls != 0 {
		t.Fatal("a settings read decision read the member's policy", err)
	}
	if _, err := service.Read(ctx, s, actor, d); !errors.Is(err, app.ErrPolicyUnauthorized) || repo.calls != 0 {
		t.Fatal("the member's decision read the stored policy", err)
	}
	if _, err := service.Mine(ctx, policyScope(t, 9), actor, d); !errors.Is(err, app.ErrPolicyUnauthorized) || repo.calls != 0 {
		t.Fatal("another organization used the decision", err)
	}
	if _, err := service.Mine(ctx, s, app.PolicyActor{AccountID: policyID(2)}, d); !errors.Is(err, app.ErrPolicyUnauthorized) || repo.calls != 0 {
		t.Fatal("an actor without proof read the policy", err)
	}
	// An incoherent stored answer is never served.
	for name, bad := range map[string]app.PolicyInForce{
		"unknown policy":        {Revision: 1, Policy: "ON"},
		"enabled at revision 0": {Policy: domain.PolicyOptional, NoticeVersion: "notice-1", NoticeInForce: &at},
		"enabled no notice":     {Revision: 1, Policy: domain.PolicyExpected},
		"enabled no instant":    {Revision: 1, Policy: domain.PolicyExpected, NoticeVersion: "notice-1"},
		"off with notice":       {Revision: 1, Policy: domain.PolicyOff, NoticeVersion: "notice-1", NoticeInForce: &at},
		"negative revision":     {Revision: -1, Policy: domain.PolicyOff},
		"bad notice version":    {Revision: 1, Policy: domain.PolicyExpected, NoticeVersion: "Aviso 1", NoticeInForce: &at},
	} {
		repo.inForce = bad
		if _, err := service.Mine(ctx, s, actor, d); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("an incoherent policy in force was served", name, err)
		}
	}
	repo.inForce, repo.err = app.PolicyInForce{Policy: domain.PolicyOff}, app.ErrNotFound
	if _, err := service.Mine(ctx, s, actor, d); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("a refusal was lost", err)
	}
	if text := fmt.Sprintf("%v %+v", app.PolicyInForce{Revision: 1, Policy: domain.PolicyExpected, NoticeVersion: "notice-2026.11"}, app.PolicyInForce{NoticeVersion: "notice-x"}); strings.Contains(text, "notice") {
		t.Fatal("the policy in force printed its notice", text)
	}
}
