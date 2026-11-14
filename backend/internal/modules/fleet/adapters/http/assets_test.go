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
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	authority "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type unavailableRepository struct{}

func (*unavailableRepository) WriteAsset(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.AssetWrite) (app.AssetResult, bool, error) {
	return app.AssetResult{}, false, app.ErrUnavailable
}
func (*unavailableRepository) ReadAsset(context.Context, kernel.OrgScope, app.FleetActor, string, string) (app.AssetView, error) {
	return app.AssetView{}, app.ErrUnavailable
}
func (*unavailableRepository) ListAssets(context.Context, kernel.OrgScope, app.FleetActor, string, app.AssetFilter) (app.AssetPage, error) {
	return app.AssetPage{}, app.ErrUnavailable
}

type unavailableGate struct{}

func (*unavailableGate) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return access.ErrUnavailable
}

type unavailableRecords struct{}

func (*unavailableRecords) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return access.ErrUnavailable
}

type unavailableAdmission struct{}

func (unavailableAdmission) Validate(rule orgapi.Rule) error {
	if rule.Permission != "fleet.asset.manage" || !rule.Write || !rule.Fresh || !rule.CompanyOnly {
		return orgapi.Unavailable
	}
	return nil
}
func (unavailableAdmission) Check(context.Context, orgapi.Principal, orgapi.Rule) error {
	return orgapi.Unavailable
}

func registrationFixture(t *testing.T) (app.AssetService, company.Handler, orgapi.Gateway, access.Authorization) {
	t.Helper()
	const origin = "https://company.example.invalid"
	cookies, err := company.New(&companyapp.Service{Enabled: true}, origin)
	if err != nil {
		t.Fatal(err)
	}
	service := app.AssetService{Enabled: true, Repository: &unavailableRepository{}, NewID: func() (string, error) { return "", app.ErrUnavailable }}
	gateway := orgapi.Gateway{Enabled: true, Origin: origin, CursorSecret: []byte(strings.Repeat("s", 32)), Now: time.Now, WriteAdmission: unavailableAdmission{}}
	auth := access.Authorization{Source: authority.NewStore(nil), Capabilities: &unavailableGate{}, Records: &unavailableRecords{}, Now: time.Now}
	return service, cookies, gateway, auth
}

func TestAssetRegistrationPinsSixCompanyPoliciesAndRefusesIncompleteComposition(t *testing.T) {
	service, cookies, gateway, auth := registrationFixture(t)
	router := chi.NewRouter()
	if err := RegisterAssets(router, service, cookies, gateway, auth); err != nil {
		t.Fatal("complete fleet registration refused", err)
	}
	want := map[string]orgapi.Rule{
		AssetCreatePath:   {Permission: "fleet.asset.manage", Write: true, Fresh: true, CompanyOnly: true, Success: 201},
		AssetChangePath:   {Permission: "fleet.asset.manage", Write: true, Fresh: true, RequireRevision: true, CompanyOnly: true, Success: 200},
		AssetDiscardPath:  {Permission: "fleet.asset.manage", Write: true, Fresh: true, RequireRevision: true, CompanyOnly: true, Success: 200},
		AssetActivatePath: {Permission: "fleet.asset.manage", Write: true, Fresh: true, RequireRevision: true, CompanyOnly: true, Success: 200},
		AssetReadPath:     {Permission: "fleet.asset.read", CompanyOnly: true, Success: 200},
		AssetListPath:     {Permission: "fleet.asset.read", CompanyOnly: true, Success: 200},
	}
	seen := 0
	if err := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method != http.MethodPost {
			t.Fatal("unexpected fleet method")
		}
		descriptor, ok := handler.(orgapi.Descriptor)
		if !ok || descriptor.OrganizationRule() != want[path] {
			t.Fatal("fleet source policy drift", path)
		}
		completion, ok := handler.(orgapi.AuthorityCompletion)
		if !ok || !completion.CompletesAuthority() {
			t.Fatal("fleet route lacks complete current authority")
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != 6 {
		t.Fatal("fleet route count changed")
	}
	for name, change := range map[string]func(*app.AssetService, *company.Handler, *orgapi.Gateway, *access.Authorization){
		"repository": func(s *app.AssetService, _ *company.Handler, _ *orgapi.Gateway, _ *access.Authorization) {
			s.Repository = nil
		},
		"typed nil repository": func(s *app.AssetService, _ *company.Handler, _ *orgapi.Gateway, _ *access.Authorization) {
			s.Repository = (*unavailableRepository)(nil)
		},
		"ID generator": func(s *app.AssetService, _ *company.Handler, _ *orgapi.Gateway, _ *access.Authorization) {
			s.NewID = nil
		},
		"company proof": func(_ *app.AssetService, c *company.Handler, _ *orgapi.Gateway, _ *access.Authorization) {
			c.Service = nil
		},
		"source": func(_ *app.AssetService, _ *company.Handler, _ *orgapi.Gateway, a *access.Authorization) {
			a.Source = nil
		},
		"capability": func(_ *app.AssetService, _ *company.Handler, _ *orgapi.Gateway, a *access.Authorization) {
			a.Capabilities = (*unavailableGate)(nil)
		},
		"record policy": func(_ *app.AssetService, _ *company.Handler, _ *orgapi.Gateway, a *access.Authorization) {
			a.Records = (*unavailableRecords)(nil)
		},
		"proof channel": func(_ *app.AssetService, _ *company.Handler, g *orgapi.Gateway, _ *access.Authorization) {
			g.Sessions = func(context.Context, string, string) (string, error) { return "", nil }
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, c, g, a := registrationFixture(t)
			change(&s, &c, &g, &a)
			if RegisterAssets(chi.NewRouter(), s, c, g, a) == nil {
				t.Fatal("incomplete fleet registration accepted")
			}
		})
	}
	service.Enabled = false
	hidden := chi.NewRouter()
	if RegisterAssets(hidden, service, company.Handler{}, orgapi.Gateway{}, access.Authorization{}) != nil {
		t.Fatal("disabled fleet required live sources")
	}
	if len(hidden.Routes()) != 0 {
		t.Fatal("disabled fleet exposed routes")
	}
}

func TestAssetProfileUsesOwnedProductsAndExactRepresentations(t *testing.T) {
	input := assetFields{Kind: d.Vehicle, Name: "Synthetic asset", UsageBasis: d.Distance, Plate: "ABC1D23", Category: categoryFields{NewLabel: "Synthetic category"}, Tanks: []tankFields{{Role: d.Main, CapacityMilli: "275000", Products: []string{"DIESEL_S10"}}}, ExpectedConsumption: []expectedFields{{Product: "DIESEL_S10", Basis: d.Distance, Value: "9007199254740993", Source: "Synthetic manual", Date: "2026-11-14"}}}
	profile, err := input.profile()
	if err != nil || profile.Category != "" || len(profile.Tanks) != 1 || profile.Tanks[0].Snapshot().Capacity != 275000 || profile.Expected[0].Snapshot().Value != 9007199254740993 {
		t.Fatal("owned product/exact profile conversion failed")
	}
	for _, raw := range []string{"0", "01", "1.0", "1000000001", "-1"} {
		copy := input
		copy.Tanks = []tankFields{{Role: d.Main, CapacityMilli: raw, Products: []string{"DIESEL_S10"}}}
		if _, err := copy.profile(); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid exact capacity accepted")
		}
	}
	for _, products := range [][]string{{"UNKNOWN"}, {"DIESEL_S10", "CNG"}} {
		copy := input
		copy.Tanks = []tankFields{{Role: d.Main, CapacityMilli: "100", Products: products}}
		if _, err := copy.profile(); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("fabricated or mixed-unit products accepted")
		}
	}
}

func TestAssetRefusalMappingDoesNotExposeStorageOrProviderDetails(t *testing.T) {
	for source, want := range map[error]orgapi.Code{app.ErrIdentifierTaken: orgapi.AssetIdentifierTaken, app.ErrConflict: orgapi.Conflict, app.ErrOperationReused: orgapi.OperationReused, app.ErrNotFound: orgapi.NotFound, app.ErrUnauthenticated: orgapi.Unauthenticated, app.ErrSurfaceDisabled: orgapi.NotFound, d.ErrInvalid: orgapi.ValidationFailed, d.ErrKindLocked: orgapi.Conflict, errors.New("Synthetic private SQL value"): orgapi.Unavailable} {
		if assetMapped(source) != want {
			t.Fatal("asset refusal mapping changed")
		}
	}
}
