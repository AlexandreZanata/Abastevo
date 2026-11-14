package adapters_test

import (
	"errors"
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
)

func TestFleetAuthorityCannotRunWithoutRetainedTransactionAndSource(t *testing.T) {
	// A current source check cannot be replaced by a constructor, an empty
	// transaction or syntactically valid client identifiers.
	var absent *adapters.FleetAuthority
	for _, guard := range []*adapters.FleetAuthority{absent, {}} {
		for _, action := range []fleet.FleetAction{fleet.FleetRead, fleet.FleetManage, 0, 255} {
			err := guard.CheckFleetTx(t.Context(), nil, kernel.OrgScope{}, fleet.FleetActor{}, "", "", action)
			if !errors.Is(err, fleet.ErrUnavailable) {
				t.Fatal("missing retained source admitted a fleet action", err)
			}
		}
	}
}
