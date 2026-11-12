// Package application declares private role-period storage, not actor grants.
package application

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound    = errors.New("orgaccess: assignment not found")
	ErrUnavailable = errors.New("orgaccess: assignment storage unavailable")
)

// Candidates are period/membership candidates only. A command must separately
// resolve actor proof, org/plan, permission, scope and record policy.
type Repository interface {
	Get(context.Context, kernel.OrgScope, string) (d.Assignment, error)
	Create(context.Context, kernel.OrgScope, d.Assignment) (bool, error)
	End(context.Context, kernel.OrgScope, string, int64) (d.Assignment, bool, error)
	Candidates(context.Context, kernel.OrgScope, string) ([]d.Assignment, error)
}

// Composition verifies an ordinary scoped transaction without committing it.
// Prelock all affected account UUIDs, then organization UUIDs, in sorted order
// before writes. This matches membership/admission/account-erasure ordering.
type TransactionRepository interface {
	GetTx(context.Context, pgx.Tx, kernel.OrgScope, string) (d.Assignment, error)
	CreateTx(context.Context, pgx.Tx, kernel.OrgScope, d.Assignment) (bool, error)
	EndTx(context.Context, pgx.Tx, kernel.OrgScope, string, int64) (d.Assignment, bool, error)
	CandidatesTx(context.Context, pgx.Tx, kernel.OrgScope, string) ([]d.Assignment, error)
}
