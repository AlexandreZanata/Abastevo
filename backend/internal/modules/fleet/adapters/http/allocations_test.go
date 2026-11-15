package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type unavailableAllocations struct{}

func (*unavailableAllocations) ListAssetAllocations(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.AllocationHistory, app.AllocationFilter) (app.AllocationPage, error) {
	return app.AllocationPage{}, app.ErrUnavailable
}

func TestAllocationRegistrationPinsTwoCurrentReadPolicies(t *testing.T) {
	_, cookies, gateway, auth := registrationFixture(t)
	service := app.AllocationHistoryService{Enabled: true, Repository: &unavailableAllocations{}}
	router := chi.NewRouter()
	if err := RegisterAssetAllocations(router, service, cookies, gateway, auth); err != nil {
		t.Fatal("complete allocation registration refused", err)
	}
	read := orgapi.Rule{Permission: "fleet.asset.read", CompanyOnly: true, Success: 200}
	want := map[string]orgapi.Rule{AssetUnitAllocationListPath: read, AssetCostCentreAllocationListPath: read}
	seen := 0
	if err := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		descriptor, ok := handler.(orgapi.Descriptor)
		if method != http.MethodPost || !ok || descriptor.OrganizationRule() != want[path] {
			t.Fatal("allocation source policy drift", path)
		}
		if completion, ok := handler.(orgapi.AuthorityCompletion); !ok || !completion.CompletesAuthority() {
			t.Fatal("allocation route lacks complete current authority", path)
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatal("allocation route count changed", seen)
	}
	for name, change := range map[string]func(*app.AllocationHistoryService, *access.Authorization){
		"repository": func(s *app.AllocationHistoryService, _ *access.Authorization) { s.Repository = nil },
		"typed repository": func(s *app.AllocationHistoryService, _ *access.Authorization) {
			s.Repository = (*unavailableAllocations)(nil)
		},
		"source":       func(_ *app.AllocationHistoryService, a *access.Authorization) { a.Source = nil },
		"records":      func(_ *app.AllocationHistoryService, a *access.Authorization) { a.Records = (*unavailableRecords)(nil) },
		"capabilities": func(_ *app.AllocationHistoryService, a *access.Authorization) { a.Capabilities = nil },
		"clock":        func(_ *app.AllocationHistoryService, a *access.Authorization) { a.Now = nil },
	} {
		s, a := service, auth
		change(&s, &a)
		if RegisterAssetAllocations(chi.NewRouter(), s, cookies, gateway, a) == nil {
			t.Fatal("incomplete allocation registration accepted", name)
		}
	}
	if RegisterAssetAllocations(nil, service, cookies, gateway, auth) == nil {
		t.Fatal("allocation registration without router accepted")
	}
	hidden := chi.NewRouter()
	if RegisterAssetAllocations(hidden, app.AllocationHistoryService{}, company.Handler{}, orgapi.Gateway{}, access.Authorization{}) != nil || len(hidden.Routes()) != 0 {
		t.Fatal("disabled allocation history exposed routes or required live sources")
	}
}

func TestAllocationListRequestIsStrict(t *testing.T) {
	var ok allocationListRequest
	if err := json.Unmarshal([]byte(`{"limit":5,"cursor":"opaque"}`), &ok); err != nil || ok.Limit != 5 || ok.Cursor != "opaque" || ok.Credentials().AccessToken != "" {
		t.Fatal("canonical allocation page refused", err)
	}
	if err := json.Unmarshal([]byte(`{}`), &ok); err != nil {
		t.Fatal("default allocation page refused", err)
	}
	for name, raw := range map[string]string{
		"unit selector":   `{"unit_id":"7b0e2a54-0000-4000-8000-000000008301"}`,
		"instant filter":  `{"at":"2026-03-15T00:00:00Z"}`,
		"history choice":  `{"history":"COST_CENTRE"}`,
		"alias":           `{"Limit":5}`,
		"duplicate":       `{"limit":5,"limit":6}`,
		"string limit":    `{"limit":"5"}`,
		"lone surrogate":  `{"cursor":"\ud800"}`,
		"trailing":        `{} {}`,
		"array":           `[]`,
		"null":            `null`,
		"account context": `{"account_id":"x"}`,
	} {
		var v allocationListRequest
		if err := v.UnmarshalJSON([]byte(raw)); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("ambiguous allocation page accepted", name, err)
		}
	}
}

func TestAllocationProjectionIsExactAndOmitsOpenEnds(t *testing.T) {
	scope := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008001")
	from := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	change := time.Date(2026, 4, 1, 3, 0, 0, 123456000, time.UTC)
	march, _ := d.NewAllocationPeriod(from, &change)
	april, _ := d.NewAllocationPeriod(change, nil)
	entries := []app.AllocationEntry{
		{TargetID: "7b0e2a54-0000-4000-8000-000000008301", Period: march, RecordedAt: from},
		{TargetID: "7b0e2a54-0000-4000-8000-000000008302", Period: april, RecordedAt: time.Date(2026, 3, 20, 10, 0, 0, 5000, time.UTC)},
	}
	for history, field := range map[app.AllocationHistory]string{app.UnitAllocationHistory: "unit_id", app.CostCentreAllocationHistory: "cost_centre_id"} {
		items, err := allocationItems(scope, history, entries)
		if err != nil || len(items) != 2 {
			t.Fatal("valid allocation history refused", history, err)
		}
		raw, _ := json.Marshal(items)
		want, _ := json.Marshal([]map[string]string{
			{field: "7b0e2a54-0000-4000-8000-000000008301", "recorded_at": "2026-03-01T12:00:00Z", "valid_from": "2026-03-01T12:00:00Z", "valid_to": "2026-04-01T03:00:00.123456Z"},
			{field: "7b0e2a54-0000-4000-8000-000000008302", "recorded_at": "2026-03-20T10:00:00.000005Z", "valid_from": "2026-04-01T03:00:00.123456Z"},
		})
		if string(raw) != string(want) {
			t.Fatal("allocation projection changed", string(raw))
		}
	}
	for name, bad := range map[string][]app.AllocationEntry{
		"bad target":      {{TargetID: "Norte", Period: march, RecordedAt: from}},
		"unset recording": {{TargetID: "7b0e2a54-0000-4000-8000-000000008301", Period: march}},
		"zero period":     {{TargetID: "7b0e2a54-0000-4000-8000-000000008301", RecordedAt: from}},
	} {
		if _, err := allocationItems(scope, app.UnitAllocationHistory, bad); !errors.Is(err, orgapi.Unavailable) {
			t.Fatal("invalid allocation projection accepted", name)
		}
	}
	if _, err := allocationItems(scope, "TEAM", entries); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("unknown allocation history projected")
	}
	if _, err := allocationItems(kernel.OrgScope{}, app.UnitAllocationHistory, entries); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("allocation projection without scope")
	}
	for source, want := range map[error]orgapi.Code{app.ErrNotFound: orgapi.NotFound, d.ErrInvalid: orgapi.ValidationFailed, app.ErrSurfaceDisabled: orgapi.NotFound, errors.New("Synthetic private SQL"): orgapi.Unavailable} {
		if code, _ := orgapi.Registered(assetMapped(source)); code != want {
			t.Fatal("allocation refusal mapping changed", want)
		}
	}
	if strings.Contains(AssetUnitAllocationListPath+AssetCostCentreAllocationListPath, "{unit_id}") {
		t.Fatal("allocation route selects a structure instead of an asset")
	}
}
