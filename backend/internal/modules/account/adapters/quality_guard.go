package adapters

import (
	"context"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/account"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/jackc/pgx/v5"
)

// RequireQualitySessionTx revalidates and locks account, family and binding in
// the command transaction. The owning module retains all credential checks.
func (s *PGStore) RequireQualitySessionTx(ctx context.Context, tx pgx.Tx, owner, family, token, contributor string, hasher domain.CodeHasher, now int64) error {
	oid, err := mustUUID(owner)
	if err != nil {
		return domain.ErrCodeUnknown
	}
	fid, err := mustUUID(family)
	if err != nil {
		return domain.ErrCodeUnknown
	}
	_, err = mustUUID(contributor)
	if err != nil {
		return domain.ErrCodeUnknown
	}
	q := account.New(tx)
	a, err := q.LockQualityAccount(ctx, oid)
	if err != nil {
		return err
	}
	if a.Status != domain.StatusActive {
		return domain.ErrCodeUnknown
	}
	f, err := q.LockFamily(ctx, fid)
	if err != nil {
		return err
	}
	if f.AccountID != oid || f.RevokedAt.Valid || now > f.AccessExpires.Time.Unix() || !hasher.Equal(f.AccessHash, f.AccessSalt, token) {
		return domain.ErrCodeUnknown
	}
	_, err = q.LockQualityBinding(ctx, account.LockQualityBindingParams{AccountID: oid, ContributorID: contributor})
	return err
}
