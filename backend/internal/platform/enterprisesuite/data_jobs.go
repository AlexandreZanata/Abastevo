package enterprisesuite

import (
	"time"

	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	data "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	notifypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/adapters"
	notify "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func DataHandlers(o Options) ([]jobs.Handler, error) {
	if !o.Enabled || !o.BillingEnabled {
		return nil, nil
	}
	if o.Pool == nil {
		return nil, app.ErrUnavailable
	}
	caps, err := capability.NewCatalog(CapabilityCodes())
	if err != nil {
		return nil, err
	}
	definitions, err := audit.NewCatalog(data.WatchAuditDefinitions())
	if err != nil {
		return nil, err
	}
	notices, err := notify.NewCatalog([]notify.Kind{data.WatchNoticeKind()})
	if err != nil {
		return nil, err
	}
	audits := auditpg.NewStore(o.Pool, definitions)
	plans := billpg.EntitlementStore{Pool: o.Pool, Capabilities: caps, Now: time.Now}
	flags := cappg.NewStore(o.Pool, caps, audits)
	flags.PlanWrites = plans
	store := data.WatchStore{Pool: o.Pool, Plans: plans, Audit: audits, Now: time.Now, Enabled: true}
	inbox := &notifypg.Store{Pool: o.Pool, Catalog: notices, Authority: accesspg.NewStore(o.Pool), NewID: jobs.NewUUIDv4}
	return []jobs.Handler{data.WatchWorker{Store: store, Flags: flags, Notices: inbox, NewID: jobs.NewUUIDv4}}, nil
}
