package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
)

type WatchInput struct {
	OperationID string
	Dataset     string
	Fuel        kernel.Product
	Revision    int64
}

type Watch struct {
	ID, Dataset            string
	Fuel                   kernel.Product
	Baseline, LastObserved int64
	Active                 bool
}

type Watches struct {
	Items []Watch
	After string
}

type WatchCommands interface {
	CreateWatch(context.Context, kernel.OrgScope, organization.CompanyActor, string, WatchInput) (Watch, bool, error)
	DisableWatch(context.Context, kernel.OrgScope, organization.CompanyActor, string, string, string) (Watch, bool, error)
	ListWatches(context.Context, kernel.OrgScope, organization.CompanyActor, string, string, int) (Watches, error)
}
