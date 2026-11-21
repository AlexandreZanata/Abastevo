package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// AccountingFile proves the current rights of the same requester for a live,
// versioned accounting artifact. The connector separately verifies the exact
// external consumer and the approved limits for this artifact.
func (s *Store) AccountingFile(ctx context.Context, scope kernel.OrgScope, id string) (domain.Object, error) {
	if s == nil || s.Pool == nil || s.Authority == nil || scope.Validate() != nil || !domain.ValidID(id) {
		return domain.Object{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var object domain.Object
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		row, err := db.New(tx).AccountingConnectorFile(ctx, db.AccountingConnectorFileParams{OrgID: org, ID: id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrNotFound
			}
			return err
		}
		requester := app.Requester{AccountID: row.RequesterAccountID, MembershipID: row.RequesterMembershipID}
		if err := s.Authority.CheckRequesterTx(ctx, tx, scope, requester, row.Kind, access.Permission("exp.expense.read")); err != nil {
			return err
		}
		object = domain.Object{OrgID: org, ExportID: id, SHA256: row.Sha256, MediaType: row.MediaType, Bytes: row.Bytes, Rows: row.Rows}
		return object.Validate(scope)
	})
	return object, storeError(ctx, err)
}
