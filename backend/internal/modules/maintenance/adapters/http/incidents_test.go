package http

import (
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// EDR-241: the exact rule of each incident route.
func TestIncidentRulesAreExact(t *testing.T) {
	for action, want := range map[app.IncidentAction]orgapi.Rule{
		app.IncidentQueue:  {Permission: "check.incident.manage", CompanyOnly: true, Success: 200},
		app.IncidentRead:   {Permission: "check.incident.manage", CompanyOnly: true, Success: 200},
		app.IncidentOwn:    {Permission: "check.incident.report", CompanyOnly: true, Success: 200},
		app.IncidentDecide: {Permission: "check.incident.manage", Write: true, Fresh: true, RequireRevision: true, CompanyOnly: true, Success: 200},
	} {
		if IncidentRule(action) != want {
			t.Fatal("rule drift", action, IncidentRule(action))
		}
	}
}

// Refusals map only to registered codes; private causes stay unavailable.
func TestIncidentRefusalsMapToRegisteredCodes(t *testing.T) {
	for err, want := range map[error]orgapi.Code{
		app.ErrSelfApproval:                       orgapi.SelfApproval,
		incident.ErrInvalid:                       orgapi.ValidationFailed,
		app.ErrConflict:                           orgapi.Conflict,
		app.ErrNotFound:                           orgapi.NotFound,
		app.ErrOperationReused:                    orgapi.OperationReused,
		app.ErrSurfaceDisabled:                    orgapi.NotFound,
		access.ErrFreshAuthRequired:               orgapi.FreshAuthRequired,
		errors.New("PRIVATE-PROVIDER-ERROR-7731"): orgapi.Unavailable,
	} {
		if got := incidentMapped(err); !errors.Is(got, want) {
			t.Fatal("refusal mapping", err, got)
		}
	}
	var denied *permissions.PermissionDenial
	if !errors.As(permissions.DenyPermission("check.incident.manage"), &denied) {
		t.Fatal("denial fixture")
	}
	if code, ok := orgapi.Registered(incidentMapped(denied)); !ok || code != orgapi.Forbidden {
		t.Fatal("permission denial is not FORBIDDEN", code)
	}
}
