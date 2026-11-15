package http

import (
	"context"
	"errors"
	"net/http"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type unavailableAttachments struct{}

func (*unavailableAttachments) AttachImplement(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.AttachmentCreate) (app.AttachmentResult, bool, error) {
	return app.AttachmentResult{}, false, app.ErrUnavailable
}
func (*unavailableAttachments) DetachImplement(context.Context, kernel.OrgScope, app.FleetActor, string, app.AttachmentDetach) (app.AttachmentResult, bool, error) {
	return app.AttachmentResult{}, false, app.ErrUnavailable
}
func (*unavailableAttachments) ListAttachments(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.PeriodFilter) (app.AttachmentPage, error) {
	return app.AttachmentPage{}, app.ErrUnavailable
}

type manageAdmission struct{}

func (manageAdmission) Validate(rule orgapi.Rule) error {
	if rule.Permission != "fleet.asset.manage" || !rule.Write || !rule.Fresh || !rule.CompanyOnly {
		return orgapi.Unavailable
	}
	return nil
}
func (manageAdmission) Check(context.Context, orgapi.Principal, orgapi.Rule) error {
	return orgapi.Unavailable
}

func TestAttachmentRegistrationPinsThreeCompanyPolicies(t *testing.T) {
	_, cookies, gateway, auth := registrationFixture(t)
	gateway.WriteAdmission = manageAdmission{}
	router := chi.NewRouter()
	if err := RegisterAssetAttachments(router, app.AttachmentService{Enabled: true, Repository: &unavailableAttachments{}, NewID: func() (string, error) { return "", app.ErrUnavailable }}, cookies, gateway, auth); err != nil {
		t.Fatal("complete attachment registration refused", err)
	}
	want := map[string]orgapi.Rule{
		AttachmentAttachPath: {Permission: "fleet.asset.manage", Write: true, Fresh: true, CompanyOnly: true, Success: 201},
		AttachmentDetachPath: {Permission: "fleet.asset.manage", Write: true, Fresh: true, CompanyOnly: true, Success: 200},
		AttachmentListPath:   {Permission: "fleet.asset.read", CompanyOnly: true, Success: 200},
	}
	seen := 0
	if err := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		descriptor, ok := handler.(orgapi.Descriptor)
		if method != http.MethodPost || !ok || descriptor.OrganizationRule() != want[path] {
			t.Fatal("attachment source policy drift", path)
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatal("attachment route count changed", seen)
	}
	for _, err := range []error{app.ErrAttachmentOverlap, d.ErrAttachmentEnded} {
		if code, _ := orgapi.Registered(attachmentMapped(err)); code != orgapi.Conflict {
			t.Fatal("attachment refusal", err, code)
		}
	}
	if code, _ := orgapi.Registered(attachmentMapped(d.ErrInvalid)); code != orgapi.ValidationFailed {
		t.Fatal("kind refusal", code)
	}
	if err := RegisterAssetAttachments(chi.NewRouter(), app.AttachmentService{Enabled: true, Repository: &unavailableAttachments{}}, cookies, gateway, auth); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("registration without identifiers", err)
	}
}
