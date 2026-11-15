package http

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type unavailableReservations struct{}

func (*unavailableReservations) RequestReservation(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.ReservationRequest) (app.ReservationResult, bool, error) {
	return app.ReservationResult{}, false, app.ErrUnavailable
}
func (*unavailableReservations) ActOnReservation(context.Context, kernel.OrgScope, app.FleetActor, string, app.ReservationAct) (app.ReservationResult, bool, error) {
	return app.ReservationResult{}, false, app.ErrUnavailable
}
func (*unavailableReservations) CreateBlock(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.BlockCreate) (app.BlockResult, bool, error) {
	return app.BlockResult{}, false, app.ErrUnavailable
}
func (*unavailableReservations) RemoveBlock(context.Context, kernel.OrgScope, app.FleetActor, string, app.BlockRemove) (app.BlockResult, bool, error) {
	return app.BlockResult{}, false, app.ErrUnavailable
}
func (*unavailableReservations) ListReservations(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.PeriodFilter) (app.ReservationPage, error) {
	return app.ReservationPage{}, app.ErrUnavailable
}
func (*unavailableReservations) ListBlocks(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.PeriodFilter) (app.BlockPage, error) {
	return app.BlockPage{}, app.ErrUnavailable
}
func (*unavailableReservations) ReadAvailability(context.Context, kernel.OrgScope, app.FleetActor, string, string) (app.AvailabilityView, error) {
	return app.AvailabilityView{}, app.ErrUnavailable
}

type reservationAdmission struct{}

func (reservationAdmission) Validate(rule orgapi.Rule) error {
	if rule.Permission != "fleet.reservation.manage" || !rule.Write || !rule.Fresh || !rule.CompanyOnly {
		return orgapi.Unavailable
	}
	return nil
}
func (reservationAdmission) Check(context.Context, orgapi.Principal, orgapi.Rule) error {
	return orgapi.Unavailable
}

func TestReservationRegistrationPinsEightCompanyPolicies(t *testing.T) {
	_, cookies, gateway, auth := registrationFixture(t)
	gateway.WriteAdmission = reservationAdmission{}
	service := app.ReservationService{Enabled: true, Repository: &unavailableReservations{}, NewID: func() (string, error) { return "", app.ErrUnavailable }}
	router := chi.NewRouter()
	if err := RegisterAssetReservations(router, service, cookies, gateway, auth); err != nil {
		t.Fatal("complete reservation registration refused", err)
	}
	read := orgapi.Rule{Permission: "fleet.asset.read", CompanyOnly: true, Success: 200}
	write := func(status int) orgapi.Rule {
		return orgapi.Rule{Permission: "fleet.reservation.manage", Write: true, Fresh: true, CompanyOnly: true, Success: status}
	}
	want := map[string]orgapi.Rule{
		ReservationRequestPath: write(201), ReservationConfirmPath: write(200), ReservationCancelPath: write(200),
		BlockCreatePath: write(201), BlockRemovePath: write(200),
		ReservationListPath: read, BlockListPath: read, AvailabilityReadPath: read,
	}
	seen := 0
	if err := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		descriptor, ok := handler.(orgapi.Descriptor)
		if method != http.MethodPost || !ok || descriptor.OrganizationRule() != want[path] {
			t.Fatal("reservation source policy drift", path)
		}
		if completion, ok := handler.(orgapi.AuthorityCompletion); !ok || !completion.CompletesAuthority() {
			t.Fatal("reservation route lacks complete current authority", path)
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatal("reservation route count changed", seen)
	}
	if err := RegisterAssetReservations(chi.NewRouter(), app.ReservationService{Enabled: true, Repository: &unavailableReservations{}}, cookies, gateway, auth); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("registration without an identifier source", err)
	}
	if err := RegisterAssetReservations(chi.NewRouter(), app.ReservationService{}, cookies, gateway, auth); err != nil {
		t.Fatal("disabled surface registers nothing", err)
	}
}

// B-BR-EMP-295/297/298: refusals map to their registered codes; a conflict
// keeps only its window.
func TestReservationRefusalsMapToRegisteredCodes(t *testing.T) {
	from, to := time.Date(2026, 12, 1, 11, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)
	conflict := reservationMapped(app.NewReservationConflict(from, &to))
	if !errors.Is(conflict, orgapi.ReservationConflict) {
		t.Fatal("window conflict code", conflict)
	}
	if code, _ := orgapi.Registered(reservationMapped(app.ErrReservationConflict)); code != orgapi.ReservationConflict {
		t.Fatal("bare conflict code", code)
	}
	for _, err := range []error{d.ErrReservationEnded, d.ErrBlockEnded} {
		if code, _ := orgapi.Registered(reservationMapped(err)); code != orgapi.Conflict {
			t.Fatal("ended refusal", err, code)
		}
	}
	if code, _ := orgapi.Registered(reservationMapped(d.ErrInvalid)); code != orgapi.ValidationFailed {
		t.Fatal("validation refusal", code)
	}
	if _, err := (blockCreateRequest{SchemaVersion: 1, OperationID: "7b0e2a54-0000-4000-8000-000000009104", ReasonKind: "OTHER"}).command("7b0e2a54-0000-4000-8000-000000008201"); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("unknown block reason", err)
	}
	empty := ""
	if _, err := (reservationRequest{SchemaVersion: 1, OperationID: "7b0e2a54-0000-4000-8000-000000009101", StartsAt: "2026-12-01T11:00:00Z", EndsAt: "2026-12-01T15:00:00Z", EmployeeID: &empty, Purpose: "Synthetic"}).command("7b0e2a54-0000-4000-8000-000000008201"); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("empty employee selector", err)
	}
	if _, err := (reservationRequest{SchemaVersion: 1, OperationID: "7b0e2a54-0000-4000-8000-000000009101", StartsAt: "tomorrow", EndsAt: "2026-12-01T15:00:00Z", Purpose: "Synthetic"}).command("7b0e2a54-0000-4000-8000-000000008201"); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("non-RFC 3339 start", err)
	}
}
