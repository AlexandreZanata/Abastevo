package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

const (
	linkOp     = "7b0e2a54-0000-4000-8000-000000020911"
	readingID  = "7b0e2a54-0000-4000-8000-000000020912"
	assetID    = "7b0e2a54-0000-4000-8000-000000020913"
	employeeID = "7b0e2a54-0000-4000-8000-000000020914"
)

type fakeResolver struct {
	permission permissions.Permission
	err        error
}

func (r fakeResolver) DocumentTargetPermission(kind domain.TargetKind) (permissions.Permission, error) {
	return r.permission, r.err
}
func (fakeResolver) ResolveDocumentTargetTx(context.Context, pgx.Tx, kernel.OrgScope, domain.TargetKind, string) (app.ResolvedTarget, error) {
	return app.ResolvedTarget{}, app.ErrUnavailable
}

func registry() app.Targets {
	return app.Targets{domain.Asset: fakeResolver{permission: "fleet.asset.manage"}, domain.MeterReading: fakeResolver{permission: "meter.reading.record"},
		domain.Employee: fakeResolver{permission: "idn.employee.manage"}}
}

func link() app.Link {
	return app.Link{OperationID: linkOp, DocumentID: docID, Target: app.TargetRef{Kind: domain.MeterReading, ID: readingID}}
}

type fakeLinks struct {
	view                     app.LinkView
	replayed                 bool
	err                      error
	links, documents, target int
	documentFilter           app.DocumentLinkFilter
	targetFilter             app.TargetLinkFilter
	page                     app.LinkPage
}

func (r *fakeLinks) LinkTx(context.Context, pgx.Tx, kernel.OrgScope, app.Actor, app.Link) (app.LinkView, bool, error) {
	return app.LinkView{}, false, app.ErrUnavailable
}
func (r *fakeLinks) Link(context.Context, kernel.OrgScope, app.Actor, app.Link) (app.LinkView, bool, error) {
	r.links++
	return r.view, r.replayed, r.err
}
func (r *fakeLinks) DocumentLinks(_ context.Context, _ kernel.OrgScope, _ app.Actor, _ string, filter app.DocumentLinkFilter) (app.LinkPage, error) {
	r.documents++
	r.documentFilter = filter
	return r.page, r.err
}
func (r *fakeLinks) TargetLinks(_ context.Context, _ kernel.OrgScope, _ app.Actor, _ app.TargetRef, filter app.TargetLinkFilter) (app.LinkPage, error) {
	r.target++
	r.targetFilter = filter
	return r.page, r.err
}

func TestBindingsBelongToExactlyOneRegisteredKind(t *testing.T) {
	for binding, want := range map[app.Binding]domain.TargetKind{app.BindAsset: domain.Asset, app.BindEmployee: domain.Employee,
		app.BindMeterReading: domain.MeterReading, app.BindHeldMeterReading: domain.MeterReading} {
		if kind, ok := binding.Kind(); !ok || kind != want {
			t.Fatal("binding kind", binding, kind)
		}
	}
	for _, binding := range []app.Binding{"", "FUELING", "asset", "HELD_READING", "EXPENSE"} {
		if _, ok := binding.Kind(); ok {
			t.Fatal("unknown binding accepted", binding)
		}
	}
}

func TestTargetRegistryIsClosedAndBuildsOwnerPolicies(t *testing.T) {
	targets := registry()
	for kind, permission := range map[domain.TargetKind]permissions.Permission{domain.Asset: "fleet.asset.manage", domain.MeterReading: "meter.reading.record", domain.Employee: "idn.employee.manage"} {
		for _, effect := range []access.Effect{access.OperationalWrite, access.Read} {
			policy, resolver, err := targets.Policy(kind, effect)
			if err != nil || resolver == nil || policy != (access.Policy{Permission: permission, Capability: "organizations", Effect: effect}) {
				t.Fatal("owner policy", kind, effect, policy, err)
			}
		}
		if _, err := targets.Resolver(kind); err != nil {
			t.Fatal("registered kind refused", kind, err)
		}
	}
	// Kinds without a registered owner resolver are refused, never stored.
	for _, kind := range []domain.TargetKind{domain.Fueling, domain.Expense, domain.WorkOrder, domain.Trip, "BOGUS", "asset", ""} {
		if _, err := targets.Resolver(kind); !errors.Is(err, app.ErrInvalid) {
			t.Fatal("unregistered kind admitted", kind, err)
		}
		if _, _, err := targets.Policy(kind, access.OperationalWrite); !errors.Is(err, app.ErrInvalid) {
			t.Fatal("unregistered kind policy", kind, err)
		}
	}
	if _, err := (app.Targets)(nil).Resolver(domain.Asset); !errors.Is(err, app.ErrInvalid) {
		t.Fatal("empty registry admitted a kind", err)
	}
	// Only Read and OperationalWrite target effects; never a document
	// permission, an unknown permission or a resolver refusing its own kind.
	for name, change := range map[string]func() (app.Targets, access.Effect){
		"configuration": func() (app.Targets, access.Effect) { return registry(), access.ConfigurationWrite },
		"zero-effect":   func() (app.Targets, access.Effect) { return registry(), 0 },
		"document-permission": func() (app.Targets, access.Effect) {
			return app.Targets{domain.Asset: fakeResolver{permission: "doc.document.view_content"}}, access.Read
		},
		"unknown-permission": func() (app.Targets, access.Effect) {
			return app.Targets{domain.Asset: fakeResolver{permission: "fleet.asset.everything"}}, access.Read
		},
		"resolver-refuses": func() (app.Targets, access.Effect) {
			return app.Targets{domain.Asset: fakeResolver{err: app.ErrInvalid}}, access.Read
		},
		"nil-resolver": func() (app.Targets, access.Effect) { return app.Targets{domain.Asset: nil}, access.Read },
	} {
		targets, effect := change()
		if _, _, err := targets.Policy(domain.Asset, effect); err == nil {
			t.Fatal("unsafe target policy accepted", name)
		}
	}
}

func TestLinkCommandValidatesScopeIdentifiersAndClosedKinds(t *testing.T) {
	s := scope(t)
	if err := link().Validate(s); err != nil {
		t.Fatal("valid link refused", err)
	}
	if err := link().Validate(kernel.OrgScope{}); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("unscoped link", err)
	}
	for name, change := range map[string]func(*app.Link){
		"operation-missing":   func(l *app.Link) { l.OperationID = "" },
		"operation-upper":     func(l *app.Link) { l.OperationID = strings.ToUpper(linkOp) },
		"operation-nil":       func(l *app.Link) { l.OperationID = "00000000-0000-0000-0000-000000000000" },
		"target-kind-unknown": func(l *app.Link) { l.Target.Kind = "BOGUS" },
		"target-kind-lower":   func(l *app.Link) { l.Target.Kind = "meter_reading" },
		"target-id-padded":    func(l *app.Link) { l.Target.ID = " " + readingID },
		"target-id-nil":       func(l *app.Link) { l.Target.ID = "00000000-0000-0000-0000-000000000000" },
	} {
		l := link()
		change(&l)
		if err := l.Validate(s); !errors.Is(err, app.ErrInvalid) {
			t.Fatal("invalid link accepted", name, err)
		}
	}
	for _, document := range []string{"", "not-a-uuid", strings.ToUpper(docID)} {
		l := link()
		l.DocumentID = document
		if err := l.Validate(s); !errors.Is(err, app.ErrNotFound) {
			t.Fatal("invalid document selector", document, err)
		}
	}
	// The pure EMP-DOC-001 kinds are syntactically valid targets; the closed
	// registry, not the syntax, refuses kinds that have no owner resolver.
	for _, kind := range []domain.TargetKind{domain.Fueling, domain.Expense, domain.WorkOrder, domain.Trip} {
		if err := (app.TargetRef{Kind: kind, ID: readingID}).Validate(s); err != nil {
			t.Fatal("domain kind syntax refused", kind, err)
		}
	}
}

func TestLinkDigestPinsActorDocumentAndTarget(t *testing.T) {
	s := scope(t)
	base := link().Digest(s, actor())
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(base) || base != link().Digest(s, actor()) {
		t.Fatal("digest is not a stable SHA-256", base)
	}
	other := actor()
	other.MembershipID = assetID
	changes := []app.Link{link(), link(), link(), link()}
	changes[0].OperationID = assetID
	changes[1].DocumentID = assetID
	changes[2].Target = app.TargetRef{Kind: domain.Asset, ID: readingID}
	changes[3].Target.ID = employeeID
	seen := map[string]bool{base: true}
	for _, c := range changes {
		seen[c.Digest(s, actor())] = true
	}
	seen[link().Digest(s, other)] = true
	if len(seen) != 6 {
		t.Fatal("digest ignores a pinned field", len(seen))
	}
}

func TestLinkServiceValidatesBeforeTheRepositoryAndIsDefaultOff(t *testing.T) {
	s := scope(t)
	created := app.LinkView{DocumentID: docID, Target: link().Target, DocumentStorage: domain.Uploading, OperationID: linkOp, CreatedBy: memberID, CreatedAt: time.Unix(1_800_000_000, 0).UTC()}
	repository := &fakeLinks{view: created}
	service := app.Service{Enabled: true, Repository: &fakeRepository{}, Links: repository}
	got, err := service.Link(t.Context(), s, actor(), link())
	if err != nil || got.View != created || got.Replayed || repository.links != 1 {
		t.Fatal("link not delegated", err)
	}
	repository.replayed = true
	if got, err := service.Link(t.Context(), s, actor(), link()); err != nil || !got.Replayed {
		t.Fatal("replay flag lost", err)
	}
	bad := link()
	bad.Target.Kind = "BOGUS"
	if _, err := service.Link(t.Context(), s, actor(), bad); !errors.Is(err, app.ErrInvalid) || repository.links != 2 {
		t.Fatal("invalid link reached the repository", err)
	}
	repository.err = app.ErrConflict
	if _, err := service.Link(t.Context(), s, actor(), link()); !errors.Is(err, app.ErrConflict) {
		t.Fatal("repository refusal lost", err)
	}
	repository.err = nil
	for name, svc := range map[string]app.Service{
		"disabled": {Links: repository},
		"no-links": {Enabled: true, Repository: &fakeRepository{}},
	} {
		want := app.ErrDisabled
		if name == "no-links" {
			want = app.ErrUnavailable
		}
		if _, err := svc.Link(t.Context(), s, actor(), link()); !errors.Is(err, want) {
			t.Fatal("link surface", name, err)
		}
		if _, err := svc.DocumentLinks(t.Context(), s, actor(), docID, app.DocumentLinkFilter{Limit: 20}); !errors.Is(err, want) {
			t.Fatal("document list surface", name, err)
		}
		if _, err := svc.TargetLinks(t.Context(), s, actor(), link().Target, app.TargetLinkFilter{Limit: 20}); !errors.Is(err, want) {
			t.Fatal("target list surface", name, err)
		}
	}
	invalid := actor()
	invalid.Channel = "kiosk"
	if _, err := service.Link(t.Context(), s, invalid, link()); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("invalid actor", err)
	}
	if repository.links != 3 {
		t.Fatal("refused requests reached the repository", repository.links)
	}
}

func TestLinkListsBoundPagesAndCursors(t *testing.T) {
	s := scope(t)
	repository := &fakeLinks{page: app.LinkPage{Items: []app.LinkView{{DocumentID: docID, Target: link().Target, DocumentStorage: domain.Stored}}, More: true}}
	service := app.Service{Enabled: true, Repository: &fakeRepository{}, Links: repository}
	after := link().Target
	page, err := service.DocumentLinks(t.Context(), s, actor(), docID, app.DocumentLinkFilter{Limit: 2, After: &after})
	if err != nil || len(page.Items) != 1 || !page.More || repository.documentFilter.Limit != 2 || *repository.documentFilter.After != after {
		t.Fatal("document page", err)
	}
	if _, err := service.TargetLinks(t.Context(), s, actor(), link().Target, app.TargetLinkFilter{Limit: app.MaxLinkPage, AfterDocument: docID}); err != nil || repository.targetFilter.AfterDocument != docID {
		t.Fatal("target page", err)
	}
	unknown := app.TargetRef{Kind: "BOGUS", ID: readingID}
	for name, call := range map[string]func() error{
		"zero-limit": func() error {
			_, err := service.DocumentLinks(t.Context(), s, actor(), docID, app.DocumentLinkFilter{})
			return err
		},
		"over-limit": func() error {
			_, err := service.TargetLinks(t.Context(), s, actor(), link().Target, app.TargetLinkFilter{Limit: app.MaxLinkPage + 1})
			return err
		},
		"bad-after-target": func() error {
			_, err := service.DocumentLinks(t.Context(), s, actor(), docID, app.DocumentLinkFilter{Limit: 2, After: &unknown})
			return err
		},
		"bad-after-document": func() error {
			_, err := service.TargetLinks(t.Context(), s, actor(), link().Target, app.TargetLinkFilter{Limit: 2, AfterDocument: "x"})
			return err
		},
		"bad-target": func() error {
			_, err := service.TargetLinks(t.Context(), s, actor(), unknown, app.TargetLinkFilter{Limit: 2})
			return err
		},
	} {
		if err := call(); !errors.Is(err, app.ErrInvalid) {
			t.Fatal("invalid page accepted", name, err)
		}
	}
	if _, err := service.DocumentLinks(t.Context(), s, actor(), "x", app.DocumentLinkFilter{Limit: 2}); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("invalid document selector", err)
	}
	if repository.documents != 1 || repository.target != 1 {
		t.Fatal("refused pages reached the repository", repository.documents, repository.target)
	}
}

func TestLinkValuesNeverFormatIdentifiers(t *testing.T) {
	view := app.LinkView{DocumentID: docID, Target: link().Target, DocumentStorage: domain.Uploading, OperationID: linkOp, CreatedBy: memberID}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, untimedLog))
	for _, value := range []any{link(), view, app.Linked{View: view}, app.LinkPage{Items: []app.LinkView{view}}} {
		raw, _ := json.Marshal(value)
		logger.Info("synthetic", "value", value)
		for _, out := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(raw), logs.String()} {
			for _, secret := range []string{docID, readingID, linkOp, memberID} {
				if strings.Contains(out, secret) {
					t.Fatal("private link value formatted", out)
				}
			}
		}
	}
}

// untimedLog drops the record time so that a numeric private marker cannot
// match digits of the timestamp by chance (a 4-digit marker did, about once in
// 200 runs).
var untimedLog = &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}}
