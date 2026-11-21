package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgconnector"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type capabilityGuard interface {
	GuardWriteDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) error
}

type Access struct {
	Pool  *pgxpool.Pool
	Flags capabilityGuard
}

// WithActive proves the configured connection through the scoped operation or
// durable claim. The callback completes SQL work only and retains no authority
// for a later connection request.
func (a Access) WithActive(ctx context.Context, scope kernel.OrgScope, kind domain.Kind, connectionID, resourceID string,
	work func(context.Context, pgx.Tx, domain.Approval) error,
) error {
	if a.Pool == nil || a.Flags == nil || work == nil {
		return app.ErrUnavailable
	}
	if scope.Validate() != nil || !kind.Valid() || !audit.ValidID(connectionID) ||
		(resourceID != "" && !audit.ValidID(resourceID)) {
		return domain.ErrInvalid
	}
	return database.WithOrgTx(ctx, a.Pool, scope, func(tx pgx.Tx) error {
		if err := a.Flags.GuardWriteDBTX(ctx, tx, scope, "integrations"); err != nil {
			if errors.Is(err, capability.ErrEntitlementRequired) {
				return domain.ErrDenied
			}
			return err
		}
		org := scope.OrgID().String()
		q := db.New(tx)
		state, err := q.LockConnectorOrganization(ctx, org)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && state != string(organization.Active) {
			return domain.ErrDenied
		}
		if err != nil {
			return err
		}
		row, err := q.GetConnection(ctx, db.GetConnectionParams{OrgID: org, ID: connectionID})
		if errors.Is(err, pgx.ErrNoRows) || err == nil && domain.State(row.State) != domain.Active {
			return domain.ErrDenied
		}
		if err != nil {
			return err
		}
		review, err := currentApproval(ctx, q, org, row.ApprovalID)
		if err != nil {
			return err
		}
		now, err := q.ReadConnectionClock(ctx)
		if err != nil || !now.Valid {
			return app.ErrUnavailable
		}
		if review.Revision != row.ApprovalRevision || review.Check(kind, resourceID, now.Time, true) != nil {
			return domain.ErrDenied
		}
		return work(ctx, tx, review)
	})
}
