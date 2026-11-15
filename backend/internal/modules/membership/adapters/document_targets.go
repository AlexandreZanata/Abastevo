package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	docapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	docdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// EmployeeDocumentTargets is the employee register's document link resolver
// (EMP-DOC-003). It reads the identity in the caller's scoped transaction and
// grants nothing by itself; employee history is never deleted.
type EmployeeDocumentTargets struct{}

var _ docapp.TargetResolver = EmployeeDocumentTargets{}

// DocumentTargetPermission keeps employee documents with the register's own
// management permission.
func (EmployeeDocumentTargets) DocumentTargetPermission(kind docdomain.TargetKind) (permissions.Permission, error) {
	if kind != docdomain.Employee {
		return "", docapp.ErrInvalid
	}
	return "idn.employee.manage", nil
}

func (EmployeeDocumentTargets) ResolveDocumentTargetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, kind docdomain.TargetKind, id string) (docapp.ResolvedTarget, error) {
	if kind != docdomain.Employee {
		return docapp.ResolvedTarget{}, docapp.ErrInvalid
	}
	if tx == nil || scope.Validate() != nil {
		return docapp.ResolvedTarget{}, docapp.ErrUnavailable
	}
	if parsed, err := kernel.ParseOrgID(id); err != nil || parsed.String() != id {
		return docapp.ResolvedTarget{}, docapp.ErrNotFound
	}
	if err := database.ValidateOrgTx(ctx, tx, scope); err != nil {
		return docapp.ResolvedTarget{}, docapp.ErrUnavailable
	}
	var found string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM org_employee_identities WHERE org_id=$1 AND id=$2`, scope.OrgID().String(), id).Scan(&found); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return docapp.ResolvedTarget{}, docapp.ErrNotFound
		}
		return docapp.ResolvedTarget{}, docapp.ErrUnavailable
	}
	if found != id {
		return docapp.ResolvedTarget{}, docapp.ErrUnavailable
	}
	return docapp.ResolvedTarget{Binding: docapp.BindEmployee}, nil
}
