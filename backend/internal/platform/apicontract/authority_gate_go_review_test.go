package apicontract

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// A guarded organization route must complete current AUTH authority (the
// gateway's ConfirmAuthority step) after its resolver. A raw orgapi.Register
// route with only a permissive resolver skips capability/record completion and
// must not pass the full-router gate even with matching contract metadata.
func TestOrganizationRouteGateRequiresAuthorityCompletion(t *testing.T) {
	document, router := conventionRoute(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal("authority-completing route refused", err)
	}
	raw, rawRouter := conventionRouteWith(t, nil)
	err := ValidateOrganizationRoutes(rawRouter, raw)
	if err == nil || !strings.Contains(err.Error(), "authority") {
		t.Fatal("raw organization route passed the gate", err)
	}
}

var completingAuthority = func(context.Context, orgapi.Principal, orgapi.Rule) error { return nil }
