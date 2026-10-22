package adapters

import (
	"context"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/ledger"
	"github.com/jackc/pgx/v5"
)

// Owner-local lock order is shared by commands, projections, gaps and erasure.
func lockLedgerOwner(ctx context.Context, tx pgx.Tx, accountID string) error {
	query := q.New(tx)
	if err := query.EnsureLedgerConsent(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if _, err := query.LockLedgerOwner(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	return nil
}
