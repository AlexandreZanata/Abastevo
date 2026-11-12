// Package application defines scoped membership storage, not actor authority.
package application

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound    = errors.New("membership: not found")
	ErrUnavailable = errors.New("membership: storage unavailable")
)

// Changed=false with nil means an exact retry. No method provisions an account,
// grants a role or discovers data from another organization's display fields.
type Repository interface {
	GetEmployee(context.Context, kernel.OrgScope, string) (d.Employee, error)
	CreateEmployee(context.Context, kernel.OrgScope, d.Employee) (bool, error)
	Get(context.Context, kernel.OrgScope, string) (d.Membership, error)
	ActiveByAccount(context.Context, kernel.OrgScope, string) (d.Membership, error)
	Create(context.Context, kernel.OrgScope, d.Membership) (bool, error)
	Save(context.Context, kernel.OrgScope, int64, d.Membership) (bool, error)
}

// Explicit-Tx composition validates the installed scope/ordinary role and
// leaves commit/rollback to the owning audit/admission command.
// Composers lock all relevant accounts in UUID order before any organization
// or employee writes; subsequent root locks use organization UUID order.
type TransactionRepository interface {
	GetEmployeeTx(context.Context, pgx.Tx, kernel.OrgScope, string) (d.Employee, error)
	CreateEmployeeTx(context.Context, pgx.Tx, kernel.OrgScope, d.Employee) (bool, error)
	GetTx(context.Context, pgx.Tx, kernel.OrgScope, string) (d.Membership, error)
	ActiveByAccountTx(context.Context, pgx.Tx, kernel.OrgScope, string) (d.Membership, error)
	CreateTx(context.Context, pgx.Tx, kernel.OrgScope, d.Membership) (bool, error)
	SaveTx(context.Context, pgx.Tx, kernel.OrgScope, int64, d.Membership) (bool, error)
}
