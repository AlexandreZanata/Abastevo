package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

type unavailableLifecycle struct{}

func (*unavailableLifecycle) TransitionAsset(context.Context, kernel.OrgScope, app.FleetActor, string, app.AssetLifecycle) (app.AssetResult, bool, error) {
	return app.AssetResult{}, false, app.ErrUnavailable
}
func (*unavailableLifecycle) ListAssetLifecycle(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.LifecycleFilter) (app.LifecyclePage, error) {
	return app.LifecyclePage{}, app.ErrUnavailable
}

func TestLifecycleRegistrationPinsSevenCompanyPolicies(t *testing.T) {
	_, cookies, gateway, auth := registrationFixture(t)
	service := app.AssetLifecycleService{Enabled: true, Repository: &unavailableLifecycle{}}
	router := chi.NewRouter()
	if err := RegisterAssetLifecycle(router, service, cookies, gateway, auth); err != nil {
		t.Fatal("complete lifecycle registration refused", err)
	}
	manage := orgapi.Rule{Permission: "fleet.asset.manage", Write: true, Fresh: true, RequireRevision: true, CompanyOnly: true, Success: 200}
	want := map[string]orgapi.Rule{
		AssetAcquisitionPath: manage, AssetDeactivatePath: manage, AssetReactivatePath: manage, AssetDisposePath: manage,
		AssetRevertDisposalPath: manage, AssetArchivePath: manage,
		AssetLifecycleListPath: {Permission: "fleet.asset.read", CompanyOnly: true, Success: 200},
	}
	seen := 0
	if err := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		descriptor, ok := handler.(orgapi.Descriptor)
		if method != http.MethodPost || !ok || descriptor.OrganizationRule() != want[path] {
			t.Fatal("lifecycle source policy drift", path)
		}
		if completion, ok := handler.(orgapi.AuthorityCompletion); !ok || !completion.CompletesAuthority() {
			t.Fatal("lifecycle route lacks complete current authority", path)
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatal("lifecycle route count changed", seen)
	}
	for name, change := range map[string]func(*app.AssetLifecycleService, *access.Authorization){
		"repository": func(s *app.AssetLifecycleService, _ *access.Authorization) { s.Repository = nil },
		"typed repository": func(s *app.AssetLifecycleService, _ *access.Authorization) {
			s.Repository = (*unavailableLifecycle)(nil)
		},
		"source":  func(_ *app.AssetLifecycleService, a *access.Authorization) { a.Source = nil },
		"records": func(_ *app.AssetLifecycleService, a *access.Authorization) { a.Records = (*unavailableRecords)(nil) },
	} {
		s, a := service, auth
		change(&s, &a)
		if RegisterAssetLifecycle(chi.NewRouter(), s, cookies, gateway, a) == nil {
			t.Fatal("incomplete lifecycle registration accepted", name)
		}
	}
	hidden := chi.NewRouter()
	if RegisterAssetLifecycle(hidden, app.AssetLifecycleService{}, company.Handler{}, orgapi.Gateway{}, access.Authorization{}) != nil || len(hidden.Routes()) != 0 {
		t.Fatal("disabled lifecycle exposed routes or required live sources")
	}
}

func TestLifecycleDTOsAreStrictAndExact(t *testing.T) {
	asset := "7b0e2a54-0000-4000-8000-000000008201"
	valid := map[d.LifecycleCommand]string{
		d.RecordAcquisition: `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"acquired_at":"2026-11-01T12:00:00.123456789Z"}`,
		d.Deactivate:        `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":"Synthetic idle season 🚚"}`,
		d.Reactivate:        `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2}`,
		d.Dispose:           `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"kind":"WRITE_OFF","disposed_at":"2026-11-10T09:00:00-03:00","reason":"Synthetic total loss"}`,
		d.RevertDisposal:    `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":"Synthetic disposal recorded by mistake"}`,
		d.Archive:           `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2}`,
	}
	for command, raw := range valid {
		input, err := decodeLifecycle(command, []byte(raw))
		if err != nil {
			t.Fatal("canonical lifecycle request refused", command, err)
		}
		c, err := input.command(command, asset)
		if err != nil {
			t.Fatal("canonical lifecycle command refused", command, err)
		}
		v, _ := c.Snapshot()
		if v.Command != command || v.AssetID != asset || v.ExpectedRevision != 2 || input.CommandFields().Revision == nil || input.Credentials().AccessToken != "" {
			t.Fatal("lifecycle command lost its exact source shape", command)
		}
		switch command {
		case d.RecordAcquisition:
			if !v.At.Equal(time.Date(2026, 11, 1, 12, 0, 0, 123456789, time.UTC)) {
				t.Fatal("acquisition nanoseconds lost")
			}
		case d.Dispose:
			if v.DisposalKind != d.WriteOff || !v.At.Equal(time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)) || v.Reason != "Synthetic total loss" {
				t.Fatal("disposal intent changed")
			}
		case d.Deactivate:
			if v.Reason != "Synthetic idle season 🚚" {
				t.Fatal("reason text changed")
			}
		}
	}
	for name, test := range map[string]struct {
		command d.LifecycleCommand
		raw     string
	}{
		"state authority":    {d.Deactivate, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":"R","state":"INACTIVE"}`},
		"account authority":  {d.Archive, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"account_id":"x"}`},
		"open work claim":    {d.Dispose, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"kind":"SALE","disposed_at":"2026-11-10T09:00:00Z","reason":"R","open_operations":[]}`},
		"reason elsewhere":   {d.Reactivate, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":"R"}`},
		"instant elsewhere":  {d.Deactivate, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":"R","disposed_at":"2026-11-10T09:00:00Z"}`},
		"kind elsewhere":     {d.RevertDisposal, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":"R","kind":"SALE"}`},
		"amount":             {d.Dispose, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"kind":"SALE","disposed_at":"2026-11-10T09:00:00Z","reason":"R","amount_cents":"100"}`},
		"null reason":        {d.Deactivate, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":null}`},
		"alias":              {d.Deactivate, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"Reason":"R"}`},
		"duplicate":          {d.Deactivate, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":"a","reason":"b"}`},
		"lone surrogate":     {d.Deactivate, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":"\ud800"}`},
		"numeric instant":    {d.RecordAcquisition, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"acquired_at":1731582000}`},
		"trailing":           {d.Archive, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2} {}`},
		"array":              {d.Archive, `[]`},
		"null":               {d.Archive, `null`},
		"disposal list form": {d.Dispose, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"disposal":{"kind":"SALE"}}`},
	} {
		if _, err := decodeLifecycle(test.command, []byte(test.raw)); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("ambiguous or authoritative lifecycle input accepted", name, err)
		}
	}
	for name, test := range map[string]struct {
		command d.LifecycleCommand
		raw     string
	}{
		"missing reason":      {d.Deactivate, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2}`},
		"empty reason":        {d.RevertDisposal, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"reason":""}`},
		"missing kind":        {d.Dispose, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"disposed_at":"2026-11-10T09:00:00Z","reason":"R"}`},
		"unknown kind":        {d.Dispose, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"kind":"THEFT","disposed_at":"2026-11-10T09:00:00Z","reason":"R"}`},
		"local instant":       {d.Dispose, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"kind":"SALE","disposed_at":"2026-11-10T09:00:00","reason":"R"}`},
		"missing instant":     {d.RecordAcquisition, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2}`},
		"ten fraction digits": {d.RecordAcquisition, `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":2,"acquired_at":"2026-11-01T12:00:00.1234567891Z"}`},
	} {
		input, err := decodeLifecycle(test.command, []byte(test.raw))
		if err == nil {
			_, err = input.command(test.command, asset)
		}
		if !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid lifecycle field was not a validation refusal", name, err)
		}
	}
}

func TestLifecycleRefusalsAndProjectionsStayBoundedAndExact(t *testing.T) {
	err := lifecycleMapped(app.NewOpenOperations([]app.OpenWorkKind{app.OpenTrip, app.OpenWorkOrder}))
	w := httptest.NewRecorder()
	orgapi.WriteError(w, httptest.NewRequest(http.MethodPost, "/", nil), err)
	var body struct {
		Error struct {
			Code    string                         `json:"code"`
			Details []struct{ Field, Code string } `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 409 || body.Error.Code != "ASSET_HAS_OPEN_OPERATIONS" || len(body.Error.Details) != 2 || body.Error.Details[0].Field != "open_operations" || body.Error.Details[0].Code != "TRIP" || body.Error.Details[1].Code != "WORK_ORDER" {
		t.Fatal("open work refusal lost its bounded kinds", w.Code, w.Body.String())
	}
	for source, want := range map[error]orgapi.Code{app.ErrConflict: orgapi.Conflict, d.ErrInvalid: orgapi.ValidationFailed, app.ErrOperationReused: orgapi.OperationReused, app.ErrNotFound: orgapi.NotFound, errors.New("Synthetic private SQL"): orgapi.Unavailable} {
		if code, _ := orgapi.Registered(lifecycleMapped(source)); code != want {
			t.Fatal("lifecycle refusal mapping changed", want)
		}
	}
	scope := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008001")
	disposedAt := time.Date(2026, 11, 10, 12, 0, 0, 7, time.UTC)
	entries := []app.LifecycleEntry{
		{Revision: 3, Command: d.Dispose, From: d.Active, To: d.Disposed, Reason: "Synthetic sale", DisposalKind: d.Sale, DisposedAt: &disposedAt, RecordedAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)},
		{Revision: 4, Command: d.RevertDisposal, From: d.Disposed, To: d.Inactive, Reason: "Synthetic mistake", RecordedAt: time.Date(2026, 11, 14, 12, 1, 0, 0, time.UTC)},
	}
	items, err := lifecycleItems(scope, entries)
	if err != nil || len(items) != 2 {
		t.Fatal("valid lifecycle history refused", err)
	}
	raw, _ := json.Marshal(items)
	if !strings.Contains(string(raw), `"disposed_at":"2026-11-10T12:00:00.000000007Z"`) || !strings.Contains(string(raw), `"command":"REVERT_DISPOSAL"`) || strings.Contains(string(raw), `"disposal_kind":""`) {
		t.Fatal("lifecycle projection changed", string(raw))
	}
}
