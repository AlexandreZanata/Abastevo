package read

import (
	"context"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/jackc/pgx/v5"
)

// ActiveReference supplies the canonical directory port inside a caller transaction.
func (r *Reader) ActiveReference(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return false, err
	}
	return q.New(tx).ActiveReference(ctx, uid)
}
