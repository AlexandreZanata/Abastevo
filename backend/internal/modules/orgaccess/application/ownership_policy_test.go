package application_test

import (
	"errors"
	"testing"

	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func TestOwnershipConsentAlwaysRequiresFreshSourcePolicy(t *testing.T) {
	for _, permission := range []permissions.Permission{"org.ownership.transfer", "org.ownership.respond"} {
		for _, effect := range []access.Effect{access.Read, access.ConfigurationWrite, access.ReduceAccess} {
			policy := access.Policy{Permission: permission, Capability: "organizations", Effect: effect}
			if !errors.Is(policy.Validate(), access.ErrUnavailable) {
				t.Fatal("ownership source admitted without fresh proof", permission, effect)
			}
			policy.Fresh = true
			if err := policy.Validate(); err != nil {
				t.Fatal("fresh ownership source rejected", permission, effect, err)
			}
		}
	}
}
