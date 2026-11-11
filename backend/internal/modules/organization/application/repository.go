// Package application declares scoped storage ports, not membership authority.
package application

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound      = errors.New("organization: not found")
	ErrUninitialized = errors.New("organization: metadata pending")
	ErrUnavailable   = errors.New("organization: storage unavailable")
)

// A true result means inserted/changed; false with nil means identical replay.
// Implementations never discover roots by display name or grant actor authority.
type Repository interface {
	Get(context.Context, kernel.OrgScope) (domain.Organization, error)
	Create(context.Context, kernel.OrgScope, domain.Organization) (bool, error)
	Save(context.Context, kernel.OrgScope, int64, domain.Organization) (bool, error)
}

// TransactionRepository joins an owning audit/onboarding transaction. It neither
// starts nor commits that transaction, and verifies its role and installed scope.
type TransactionRepository interface {
	GetTx(context.Context, pgx.Tx, kernel.OrgScope) (domain.Organization, error)
	CreateTx(context.Context, pgx.Tx, kernel.OrgScope, domain.Organization) (bool, error)
	SaveTx(context.Context, pgx.Tx, kernel.OrgScope, int64, domain.Organization) (bool, error)
}
