package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	companyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	authority "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type unavailableChecklists struct{}

func (*unavailableChecklists) CommandTemplate(context.Context, kernel.OrgScope, app.ChecklistActor, string, string, app.TemplateCommand) (app.TemplateAck, bool, error) {
	return app.TemplateAck{}, false, app.ErrUnavailable
}
func (*unavailableChecklists) ListTemplates(context.Context, kernel.OrgScope, app.ChecklistActor, string, app.TemplateFilter) (app.TemplatePage, error) {
	return app.TemplatePage{}, app.ErrUnavailable
}
func (*unavailableChecklists) ReadTemplate(context.Context, kernel.OrgScope, app.ChecklistActor, string, string, int64) (app.TemplateView, error) {
	return app.TemplateView{}, app.ErrUnavailable
}
func (*unavailableChecklists) ReadApplicable(context.Context, kernel.OrgScope, app.ChecklistActor, string, string, checklist.Context) (app.ApplicableView, error) {
	return app.ApplicableView{}, app.ErrUnavailable
}

type unavailableGate struct{}

func (*unavailableGate) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return access.ErrUnavailable
}

type unavailableStructure struct{}

func (*unavailableStructure) ResolveStructure(context.Context, kernel.OrgScope, access.Policy, permissions.ResourceFacts, time.Time) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{}, access.ErrUnavailable
}

type unavailableAssignee struct{}

func (*unavailableAssignee) ResolveAssignee(context.Context, kernel.OrgScope, access.Policy, permissions.ResourceFacts, string, time.Time) (bool, error) {
	return false, access.ErrUnavailable
}

type templateAdmission struct{}

func (templateAdmission) Validate(rule orgapi.Rule) error {
	if rule.Permission != "check.template.manage" || !rule.Write || !rule.Fresh || !rule.CompanyOnly {
		return orgapi.Unavailable
	}
	return nil
}
func (templateAdmission) Check(context.Context, orgapi.Principal, orgapi.Rule) error {
	return orgapi.Unavailable
}

func fixture(t *testing.T) (company.Handler, orgapi.Gateway, access.Authorization, access.Authorization) {
	t.Helper()
	const origin = "https://company.example.invalid"
	cookies, err := company.New(&companyapp.Service{Enabled: true}, origin)
	if err != nil {
		t.Fatal(err)
	}
	gateway := orgapi.Gateway{Enabled: true, Origin: origin, CursorSecret: []byte(strings.Repeat("s", 32)), Now: time.Now, WriteAdmission: templateAdmission{}}
	templates := access.Authorization{Source: authority.NewStore(nil), Capabilities: &unavailableGate{}, Records: ChecklistRecords{}, Now: time.Now}
	assets := templates
	assets.Structure, assets.Assignee = &unavailableStructure{}, &unavailableAssignee{}
	return cookies, gateway, templates, assets
}

// EDR-215: seven routes with their exact source policies; incomplete
// composition is refused; a disabled surface mounts nothing.
func TestChecklistRegistrationPinsSevenCompanyPolicies(t *testing.T) {
	cookies, gateway, templates, assets := fixture(t)
	service := app.ChecklistService{Enabled: true, Repository: &unavailableChecklists{}, NewID: func() (string, error) { return "", app.ErrUnavailable }}
	router := chi.NewRouter()
	if err := RegisterChecklists(router, service, cookies, gateway, templates, assets); err != nil {
		t.Fatal("complete checklist registration refused", err)
	}
	write := func(status int, revision bool) orgapi.Rule {
		return orgapi.Rule{Permission: "check.template.manage", Write: true, Fresh: true, RequireRevision: revision, CompanyOnly: true, Success: status}
	}
	read := orgapi.Rule{Permission: "check.template.manage", CompanyOnly: true, Success: 200}
	want := map[string]orgapi.Rule{
		TemplateCreatePath: write(201, false), TemplatePublishPath: write(200, true), TemplatePolicyPath: write(200, true), TemplateArchivePath: write(200, true),
		TemplateListPath: read, TemplateReadPath: read, ApplicablePath: {Permission: "fleet.asset.read", CompanyOnly: true, Success: 200},
	}
	seen := 0
	if err := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		descriptor, ok := handler.(orgapi.Descriptor)
		if method != http.MethodPost || !ok || descriptor.OrganizationRule() != want[path] {
			t.Fatal("checklist source policy drift", path)
		}
		if completion, ok := handler.(orgapi.AuthorityCompletion); !ok || !completion.CompletesAuthority() {
			t.Fatal("checklist route lacks complete current authority", path)
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatal("checklist route count changed", seen)
	}
	noStructure := assets
	noStructure.Structure = nil
	for name, register := range map[string]func() error{
		"no identifier source": func() error {
			return RegisterChecklists(chi.NewRouter(), app.ChecklistService{Enabled: true, Repository: &unavailableChecklists{}}, cookies, gateway, templates, assets)
		},
		"no repository": func() error {
			return RegisterChecklists(chi.NewRouter(), app.ChecklistService{Enabled: true, NewID: service.NewID}, cookies, gateway, templates, assets)
		},
		"no asset structure": func() error {
			return RegisterChecklists(chi.NewRouter(), service, cookies, gateway, templates, noStructure)
		},
	} {
		if err := register(); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("incomplete composition registered", name, err)
		}
	}
	if err := RegisterChecklists(chi.NewRouter(), app.ChecklistService{}, cookies, gateway, templates, assets); err != nil {
		t.Fatal("disabled surface registers nothing", err)
	}
}

// Records accept only the fixed policies on their own target kind.
func TestChecklistRecordsAcceptOnlyTheirPoliciesAndTargets(t *testing.T) {
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	root := permissions.ResourceFacts{OrgID: org, ID: org.String()}
	asset := permissions.ResourceFacts{OrgID: org, ID: "7b0e2a54-0000-4000-8000-000000008201"}
	rules := []permissions.Rule{{}}
	for _, tc := range []struct {
		policy access.Policy
		facts  permissions.ResourceFacts
	}{
		{app.Policy(app.TemplateRead), asset}, {app.Policy(app.TemplateManage), asset}, {app.Policy(app.ApplicableRead), root},
		{access.Policy{Permission: "check.checklist.execute", Capability: "organizations", Effect: access.Read}, root},
	} {
		if (ChecklistRecords{}).Check(t.Context(), s, access.Subject{}, tc.policy, tc.facts, rules) == nil {
			t.Fatal("record accepted a foreign policy or target", tc.policy.Permission)
		}
	}
}

// Refusals map to their registered codes.
func TestChecklistRefusalsMapToRegisteredCodes(t *testing.T) {
	for err, want := range map[error]orgapi.Code{
		app.ErrConflict: orgapi.Conflict, app.ErrNotFound: orgapi.NotFound, app.ErrSurfaceDisabled: orgapi.NotFound, app.ErrOperationReused: orgapi.OperationReused,
		app.ErrUnauthenticated: orgapi.Unauthenticated, checklist.ErrInvalid: orgapi.ValidationFailed, access.ErrSetupIncomplete: orgapi.OrgSetupIncomplete,
		access.ErrOrgSuspended: orgapi.OrgSuspended, access.ErrFreshAuthRequired: orgapi.FreshAuthRequired, errors.New("provider detail"): orgapi.Unavailable,
	} {
		if code, _ := orgapi.Registered(mapped(err)); code != want {
			t.Fatal("refusal mapping", err, code)
		}
	}
	if _, _, _, err := contentInput("x", "ORGANIZATION", nil, nil); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("missing items", err)
	}
	ids := []string{"b", "a"}
	_, target, _, err := contentInput("x", "ASSETS", &ids, []itemRequest{{Label: "x"}})
	if err != nil || target.IDs[0] != "b" {
		t.Fatal("wire content copied verbatim for domain validation", err)
	}
	ids[0] = "mutated"
	if target.IDs[0] != "b" {
		t.Fatal("wire content aliases the request")
	}
}
