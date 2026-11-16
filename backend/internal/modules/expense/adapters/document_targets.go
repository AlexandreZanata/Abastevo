package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/expense"
	documentapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	documentdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// ExpenseDocumentTargets resolves the EXPENSE link kind (EMP-EXP-004,
// EDR-338): a receipt documents one stored DIRECT expense under the owner's
// exp.expense.record, with the expense's author as the SELF subject (an S
// cell: only the author's own expense is covered). A missing expense, one of
// another organization and a FUELING-origin expense (documented through its
// fueling) are not found; it grants no document access and is never a cost.
type ExpenseDocumentTargets struct{}

var _ documentapp.TargetResolver = ExpenseDocumentTargets{}

func (ExpenseDocumentTargets) DocumentTargetPermission(kind documentdomain.TargetKind) (permissions.Permission, error) {
	if kind != documentdomain.Expense {
		return "", documentapp.ErrInvalid
	}
	return app.Policy(app.RecordAction).Permission, nil
}

func (ExpenseDocumentTargets) ResolveDocumentTargetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, kind documentdomain.TargetKind, id string) (documentapp.ResolvedTarget, error) {
	if tx == nil || kind != documentdomain.Expense || database.ValidateOrgTx(ctx, tx, scope) != nil {
		return documentapp.ResolvedTarget{}, documentapp.ErrUnavailable
	}
	if parsed, err := kernel.ParseOrgID(id); err != nil || parsed.String() != id {
		return documentapp.ResolvedTarget{}, documentapp.ErrNotFound
	}
	author, err := q.New(tx).ExpenseDocumentAuthor(ctx, q.ExpenseDocumentAuthorParams{OrgID: scope.OrgID().String(), ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return documentapp.ResolvedTarget{}, documentapp.ErrNotFound
	}
	if err != nil {
		return documentapp.ResolvedTarget{}, documentapp.ErrUnavailable
	}
	return documentapp.ResolvedTarget{Binding: documentapp.BindExpense, AuthoredBy: author}, nil
}
