package adapters

import (
	"context"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	caps "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgcapability"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

type documentCapability interface {
	GuardWriteDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) error
}

type DocumentQuota struct {
	Entitlements EntitlementStore
	Flags        documentCapability
}

// LockStorageReservation serializes new reservations with each other and with
// plan and operator changes. The document transaction takes it before audit.
func (q DocumentQuota) LockStorageReservation(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) error {
	if q.Flags == nil || database.ValidateOrgTx(ctx, tx, scope) != nil {
		return capability.ErrUnavailable
	}
	if err := caps.New(tx).LockEntitlementWrite(ctx, scope.OrgID().String()); err != nil {
		return err
	}
	err := q.Flags.GuardWriteDBTX(ctx, tx, scope, "organizations")
	if errors.Is(err, capability.ErrEntitlementRequired) {
		return access.ErrEntitlementRequired
	}
	return err
}

func (q DocumentQuota) CheckStorageReservation(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, addition int64) error {
	governed, plans, err := q.Entitlements.entitledPlans(ctx, tx, scope, "organizations", time.Time{})
	if err != nil || !governed {
		return err
	}
	used, err := db.New(tx).CountDocumentStorageBytes(ctx, scope.OrgID().String())
	if err != nil {
		return err
	}
	return checkPlanIncrease(plans, domain.StorageBytes, used, addition)
}
