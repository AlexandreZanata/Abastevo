package enterprisesuite

import (
	"time"

	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	connectorpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/adapters"
	connector "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func FleetWebhookHandler(o Options, sender connectorpg.WebhookSender) (jobs.Handler, error) {
	if !o.Enabled {
		return nil, nil
	}
	if o.Pool == nil {
		return nil, connector.ErrUnavailable
	}
	caps, err := capability.NewCatalog(CapabilityCodes())
	if err != nil {
		return nil, err
	}
	flags := cappg.NewStore(o.Pool, caps, nil)
	if o.BillingEnabled {
		flags.PlanWrites = billpg.EntitlementStore{Pool: o.Pool, Capabilities: caps, Now: time.Now}
	} else {
		sender = nil
	}
	return connectorpg.FleetWebhookWorker{Access: connectorpg.Access{Pool: o.Pool, Flags: flags}, Sender: sender}, nil
}
