package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type keyCapability interface {
	GuardWriteDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) error
}

type KeyAccess struct {
	Pool         *pgxpool.Pool
	Entitlements keyCapability
	Verifier     domain.KeyVerifier
}

// WithAuthorizedKey retains current plan, organization and key-row locks
// through a scoped Data read. It provides credential metadata only.
func (a KeyAccess) WithAuthorizedKey(ctx context.Context, scope kernel.OrgScope, bearer domain.Bearer,
	required domain.Scope, read func(context.Context, pgx.Tx, domain.Key) error,
) error {
	if a.Pool == nil || a.Entitlements == nil || read == nil {
		return app.ErrUnavailable
	}
	if scope.Validate() != nil || bearer.Empty() || required != domain.ReadRegional {
		return domain.ErrKeyDenied
	}
	return database.WithOrgTx(ctx, a.Pool, scope, func(tx pgx.Tx) error {
		if err := a.Entitlements.GuardWriteDBTX(ctx, tx, scope, "data"); err != nil {
			if errors.Is(err, capability.ErrEntitlementRequired) {
				return domain.ErrKeyDenied
			}
			return app.ErrUnavailable
		}
		org := scope.OrgID().String()
		queries := db.New(tx)
		state, err := queries.LockDataOrganization(ctx, org)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && state != string(organization.Active) {
			return domain.ErrKeyDenied
		}
		if err != nil {
			return app.ErrUnavailable
		}
		row, err := queries.LockKeyProof(ctx, db.LockKeyProofParams{OrgID: org, ID: bearer.ID()})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrKeyDenied
		}
		if err != nil {
			return app.ErrUnavailable
		}
		key := keyView(db.ReadKeyMetadataRow{ID: row.ID, Label: row.Label, Scopes: row.Scopes, State: row.State,
			Revision: row.Revision, IssuedAt: row.IssuedAt, ExpiresAt: row.ExpiresAt, OverlapUntil: row.OverlapUntil})
		now, err := queries.KeyClock(ctx)
		if err != nil || !now.Valid {
			return app.ErrUnavailable
		}
		if !a.Verifier.Matches(scope, bearer, row.Verifier) || !key.LiveAt(now.Time, required) {
			return domain.ErrKeyDenied
		}
		return read(ctx, tx, key)
	})
}
