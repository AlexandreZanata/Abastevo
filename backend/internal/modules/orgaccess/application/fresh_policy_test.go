package application_test

import (
	"errors"
	"testing"

	a "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// BR-AUTH-010 and organizations.md: full exports, archive/closure and grant
// acceptance join role, ownership, credential and key changes in requiring a
// freshly authenticated session; a policy cannot register them without it.
func TestSensitivePoliciesRequireFreshAuthentication(t *testing.T) {
	for _, key := range []d.Permission{"auth.role.assign", "auth.delegation.manage", "org.ownership.transfer", "idn.credential.manage", "data.key.manage", "rpt.export.run", "org.lifecycle.manage", "org.group.manage"} {
		stale := a.Policy{Permission: key, Capability: "organizations", Effect: a.ConfigurationWrite}
		if err := stale.Validate(); !errors.Is(err, a.ErrUnavailable) {
			t.Errorf("%s registered without fresh authentication: %v", key, err)
		}
		stale.Fresh = true
		if err := stale.Validate(); err != nil {
			t.Errorf("%s refused with fresh authentication: %v", key, err)
		}
	}
	if err := (a.Policy{Permission: "org.settings.read", Capability: "organizations", Effect: a.Read}).Validate(); err != nil {
		t.Fatal("ordinary read now requires fresh authentication", err)
	}
}
