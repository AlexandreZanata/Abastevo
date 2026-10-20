package adapters

import (
	"context"
	dbidentity "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/identity"
	"github.com/jackc/pgx/v5"
)

// RequireQualitySigningKeyTx closes the verify-to-write revocation window.
func RequireQualitySigningKeyTx(ctx context.Context, tx pgx.Tx, contributor, key string) error {
	cid, err := mustUUID(contributor)
	if err != nil {
		return err
	}
	kid, err := mustUUID(key)
	if err != nil {
		return err
	}
	_, err = dbidentity.New(tx).LockQualitySigningKey(ctx, dbidentity.LockQualitySigningKeyParams{ContributorID: cid, KeyID: kid})
	return err
}
