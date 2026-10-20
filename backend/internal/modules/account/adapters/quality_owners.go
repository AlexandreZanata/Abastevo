package adapters

import (
	"context"
	"errors"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/account"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ActiveQualityOwnersTx is a bounded account-owned port for private review.
func ActiveQualityOwnersTx(ctx context.Context, tx pgx.Tx, owners []string) (map[string]bool, error) {
	if len(owners) > 100 {
		return nil, errors.New("account: quality owner page exceeds bound")
	}
	ids := make([]pgtype.UUID, 0, len(owners))
	for _, owner := range owners {
		var id pgtype.UUID
		if e := id.Scan(owner); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	rows, e := account.New(tx).ActiveQualityOwners(ctx, ids)
	if e != nil {
		return nil, e
	}
	out := make(map[string]bool, len(rows))
	for _, id := range rows {
		out[id.String()] = true
	}
	return out, nil
}
