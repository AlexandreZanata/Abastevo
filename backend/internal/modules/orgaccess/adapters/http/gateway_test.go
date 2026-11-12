package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type capability struct{}

func (capability) Check(context.Context, kernel.OrgScope, app.Subject, app.Policy) error { return nil }

type records struct{}

func (records) Check(context.Context, kernel.OrgScope, app.Subject, app.Policy, d.ResourceFacts, []d.Rule) error {
	return nil
}

type resource struct{}

func (resource) Resolve(context.Context, kernel.OrgScope, app.Subject) (d.ResourceFacts, error) {
	return d.ResourceFacts{}, app.ErrNotFound
}
func TestUnavailableAccountProofCannotAuthenticate(t *testing.T) {
	sessions := guarded.AccountSessions{}
	for _, fresh := range []bool{false, true} {
		if account, err := sessions.Confirm(t.Context(), "synthetic", "synthetic", fresh); account != "" || err != orgapi.Unavailable {
			t.Fatal(account, err)
		}
	}
	if account, err := sessions.Validate(t.Context(), "synthetic", "synthetic"); account != "" || err != orgapi.Unavailable {
		t.Fatal(account, err)
	}
}
func TestGuardedRegistrationRefusesMissingOrConflictingGates(t *testing.T) {
	policy := app.Policy{Permission: "idn.employee.read", Capability: "identity", Effect: app.Read}
	rule := orgapi.Rule{Permission: "idn.employee.read"}
	service := app.Authorization{Source: adapters.NewStore(nil), Capabilities: capability{}, Records: records{}, Now: time.Now}
	gateway := orgapi.Gateway{Origin: "https://api.example.invalid", CursorSecret: []byte(strings.Repeat("s", 32)), Sessions: func(context.Context, string, string) (string, error) { return "", orgapi.Unauthenticated }, SessionConfirm: func(context.Context, string, string, bool) (string, error) { return "", orgapi.Unauthenticated }, SignedOwner: func(*http.Request, string) (string, string, error) { return "", "", orgapi.Unauthenticated }}
	handler := func(context.Context, orgapi.Principal, app.Decision, orgapi.Session) (any, error) {
		t.Fatal("invalid configuration dispatched")
		return nil, nil
	}
	for _, change := range []func(*orgapi.Gateway, *app.Authorization, *orgapi.Rule){func(g *orgapi.Gateway, _ *app.Authorization, _ *orgapi.Rule) { g.SessionConfirm = nil }, func(_ *orgapi.Gateway, s *app.Authorization, _ *orgapi.Rule) { s.Source = nil }, func(_ *orgapi.Gateway, s *app.Authorization, _ *orgapi.Rule) { s.Records = nil }, func(_ *orgapi.Gateway, s *app.Authorization, _ *orgapi.Rule) { s.Capabilities = nil }, func(_ *orgapi.Gateway, _ *app.Authorization, r *orgapi.Rule) { r.Permission = "trip.trip.read" }, func(g *orgapi.Gateway, _ *app.Authorization, _ *orgapi.Rule) {
		g.Resolve = func(context.Context, orgapi.Actor, kernel.OrgScope, orgapi.Rule) (orgapi.Membership, error) {
			return orgapi.Membership{}, nil
		}
	}, func(g *orgapi.Gateway, _ *app.Authorization, _ *orgapi.Rule) {
		g.ConfirmAuthority = func(context.Context, orgapi.Principal, orgapi.Rule) error { return nil }
	}} {
		g, s, r := gateway, service, rule
		change(&g, &s, &r)
		if err := guarded.Register(chi.NewRouter(), "/v1/orgs/{org_id}/synthetic/read", g, r, s, policy, resource{}, handler); err == nil {
			t.Fatal("incomplete/conflicting gate accepted")
		}
	}
}
