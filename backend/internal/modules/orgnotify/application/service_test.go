package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
)

const (
	orgID   = "7b0e2a54-0000-4000-8000-000000002201"
	account = "7b0e2a54-0000-4000-8000-000000002202"
	member  = "7b0e2a54-0000-4000-8000-000000002203"
	notice  = "7b0e2a54-0000-4000-8000-000000002204"
	opID    = "7b0e2a54-0000-4000-8000-000000002205"
)

type fakeRepo struct {
	calls     int
	query     app.InboxQuery
	read      app.ReadCommand
	pref      app.PreferenceCommand
	stored    []app.Preference
	err       error
	listed    app.InboxPage
	readOut   app.ReadResult
	prefOut   app.PreferenceResult
	selfCalls []app.Self
}

func (f *fakeRepo) ListInbox(_ context.Context, _ kernel.OrgScope, s app.Self, q app.InboxQuery) (app.InboxPage, error) {
	f.calls++
	f.selfCalls = append(f.selfCalls, s)
	f.query = q
	return f.listed, f.err
}
func (f *fakeRepo) MarkRead(_ context.Context, _ kernel.OrgScope, s app.Self, c app.ReadCommand) (app.ReadResult, error) {
	f.calls++
	f.selfCalls = append(f.selfCalls, s)
	f.read = c
	return f.readOut, f.err
}
func (f *fakeRepo) Preferences(_ context.Context, _ kernel.OrgScope, s app.Self) ([]app.Preference, error) {
	f.calls++
	f.selfCalls = append(f.selfCalls, s)
	return f.stored, f.err
}
func (f *fakeRepo) SetPreference(_ context.Context, _ kernel.OrgScope, s app.Self, c app.PreferenceCommand) (app.PreferenceResult, error) {
	f.calls++
	f.selfCalls = append(f.selfCalls, s)
	f.pref = c
	return f.prefOut, f.err
}

func scope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(orgID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func service(t *testing.T, repo *fakeRepo) app.Service {
	t.Helper()
	c, err := domain.NewCatalog([]domain.Kind{
		{Name: "test.report.ready", Templates: []string{"test.report.ready.v1"}},
		{Name: "test.access.changed", Mandatory: true, Templates: []string{"test.access.changed.v1"}},
		{Name: "test.budget.alert", Templates: []string{"test.budget.alert.v1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return app.Service{Enabled: true, Catalog: c, Repository: repo}
}

var self = app.Self{AccountID: account, MembershipID: member}

func TestServiceRefusesDisabledOrIncompleteComposition(t *testing.T) {
	ctx, s := t.Context(), scope(t)
	repo := &fakeRepo{}
	for name, svc := range map[string]app.Service{
		"disabled":      {Catalog: service(t, repo).Catalog, Repository: repo},
		"no repository": {Enabled: true, Catalog: service(t, repo).Catalog},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.List(ctx, s, self, app.InboxQuery{}); !errors.Is(err, domain.ErrUnavailable) {
				t.Fatal(err)
			}
			if _, err := svc.MarkRead(ctx, s, self, app.ReadCommand{OperationID: opID, NoticeID: notice}); !errors.Is(err, domain.ErrUnavailable) {
				t.Fatal(err)
			}
			if _, err := svc.Preferences(ctx, s, self); !errors.Is(err, domain.ErrUnavailable) {
				t.Fatal(err)
			}
			if _, err := svc.SetPreference(ctx, s, self, app.PreferenceCommand{OperationID: opID, Kind: "test.report.ready"}); !errors.Is(err, domain.ErrUnavailable) {
				t.Fatal(err)
			}
		})
	}
	if repo.calls != 0 {
		t.Fatal("repository reached without composition")
	}
}

func TestServiceValidatesSelectorsBeforeStorage(t *testing.T) {
	ctx, s := t.Context(), scope(t)
	repo := &fakeRepo{}
	svc := service(t, repo)
	for name, v := range map[string]app.Self{
		"empty account": {MembershipID: member}, "empty member": {AccountID: account},
		"uppercase member": {AccountID: account, MembershipID: "7B0E2A54-0000-4000-8000-000000002203"},
		"nil member":       {AccountID: account, MembershipID: "00000000-0000-0000-0000-000000000000"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.List(ctx, s, v, app.InboxQuery{}); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("invalid self selector", err)
			}
			if _, err := svc.Preferences(ctx, s, v); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("invalid self selector", err)
			}
		})
	}
	if _, err := svc.List(ctx, kernel.OrgScope{}, self, app.InboxQuery{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("invalid scope", err)
	}
	for _, q := range []app.InboxQuery{{Limit: -1}, {Limit: app.MaxLimit + 1}, {After: -1}} {
		if _, err := svc.List(ctx, s, self, q); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid page accepted", q, err)
		}
	}
	for _, c := range []app.ReadCommand{{OperationID: "x", NoticeID: notice}, {OperationID: opID, NoticeID: "x"}, {OperationID: opID}} {
		if _, err := svc.MarkRead(ctx, s, self, c); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid read command accepted", err)
		}
	}
	for _, c := range []app.PreferenceCommand{{OperationID: "x", Kind: "test.report.ready"}, {OperationID: opID, Kind: "test.report.ready", Revision: -1}} {
		if _, err := svc.SetPreference(ctx, s, self, c); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid preference command accepted", err)
		}
	}
	for _, kind := range []string{"test.access.changed", "test.unknown.kind"} {
		if _, err := svc.SetPreference(ctx, s, self, app.PreferenceCommand{OperationID: opID, Kind: kind, Muted: true}); !errors.Is(err, domain.ErrValidation) {
			t.Fatal("mandatory or unknown kind muted", kind, err)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid input reached storage", repo.calls)
	}
}

func TestServiceDelegatesValidCommands(t *testing.T) {
	ctx, s := t.Context(), scope(t)
	repo := &fakeRepo{listed: app.InboxPage{Notices: []app.Notice{{ID: notice}}}, readOut: app.ReadResult{Replayed: true}, prefOut: app.PreferenceResult{Preference: app.Preference{Kind: "test.report.ready", Muted: true, Revision: 1}}}
	svc := service(t, repo)
	page, err := svc.List(ctx, s, self, app.InboxQuery{UnreadOnly: true, After: 9})
	if err != nil || len(page.Notices) != 1 || repo.query.Limit != app.DefaultLimit || !repo.query.UnreadOnly || repo.query.After != 9 {
		t.Fatal("list not delegated with default limit", err, repo.query)
	}
	if out, err := svc.MarkRead(ctx, s, self, app.ReadCommand{OperationID: opID, NoticeID: notice}); err != nil || !out.Replayed || repo.read.NoticeID != notice {
		t.Fatal("read not delegated", err)
	}
	out, err := svc.SetPreference(ctx, s, self, app.PreferenceCommand{OperationID: opID, Kind: "test.report.ready", Muted: true})
	if err != nil || out.Preference.Mandatory || !out.Preference.Muted || repo.pref.Kind != "test.report.ready" {
		t.Fatal("preference not delegated", err)
	}
	for _, v := range repo.selfCalls {
		if v != self {
			t.Fatal("self selector changed")
		}
	}
	repo.err = domain.ErrNotFound
	if _, err := svc.List(ctx, s, self, app.InboxQuery{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("repository refusal lost", err)
	}
}

func TestPreferencesListEveryRegisteredKindWithDefaults(t *testing.T) {
	ctx, s := t.Context(), scope(t)
	repo := &fakeRepo{stored: []app.Preference{
		{Kind: "test.report.ready", Muted: true, Revision: 3},
		{Kind: "test.retired.kind", Muted: true, Revision: 1},
	}}
	got, err := service(t, repo).Preferences(ctx, s, self)
	if err != nil {
		t.Fatal(err)
	}
	want := []app.Preference{
		{Kind: "test.access.changed", Mandatory: true},
		{Kind: "test.budget.alert"},
		{Kind: "test.report.ready", Muted: true, Revision: 3},
	}
	if len(got) != len(want) {
		t.Fatal("preference inventory drift", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal("preference", i, got[i], want[i])
		}
	}
	// A mandatory kind is never reported muted even if storage were inconsistent.
	repo.stored = []app.Preference{{Kind: "test.access.changed", Muted: true, Revision: 1}}
	got, err = service(t, repo).Preferences(ctx, s, self)
	if err != nil || got[0].Muted || !got[0].Mandatory {
		t.Fatal("mandatory kind reported muted", got, err)
	}
}
