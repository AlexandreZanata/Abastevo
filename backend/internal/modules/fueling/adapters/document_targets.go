package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fueling"
	documentapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	documentdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// FuelingDocumentTargets resolves the FUELING link kind (EMP-FUEL-011,
// EDR-297): a receipt documents one stored fueling under the owner's
// fuel.fueling.record, with the fueling's author as the SELF subject (an S
// cell: only the author's own fueling is covered). A missing fueling or one
// of another organization is not found; it grants no document access.
type FuelingDocumentTargets struct{}

var _ documentapp.TargetResolver = FuelingDocumentTargets{}

func (FuelingDocumentTargets) DocumentTargetPermission(kind documentdomain.TargetKind) (permissions.Permission, error) {
	if kind != documentdomain.Fueling {
		return "", documentapp.ErrInvalid
	}
	return app.Policy(app.RecordAction).Permission, nil
}

func (FuelingDocumentTargets) ResolveDocumentTargetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, kind documentdomain.TargetKind, id string) (documentapp.ResolvedTarget, error) {
	if tx == nil || kind != documentdomain.Fueling || database.ValidateOrgTx(ctx, tx, scope) != nil {
		return documentapp.ResolvedTarget{}, documentapp.ErrUnavailable
	}
	if parsed, err := kernel.ParseOrgID(id); err != nil || parsed.String() != id {
		return documentapp.ResolvedTarget{}, documentapp.ErrNotFound
	}
	author, err := q.New(tx).FuelingDocumentAuthor(ctx, q.FuelingDocumentAuthorParams{OrgID: scope.OrgID().String(), ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return documentapp.ResolvedTarget{}, documentapp.ErrNotFound
	}
	if err != nil {
		return documentapp.ResolvedTarget{}, documentapp.ErrUnavailable
	}
	return documentapp.ResolvedTarget{Binding: documentapp.BindFueling, AuthoredBy: author}, nil
}
