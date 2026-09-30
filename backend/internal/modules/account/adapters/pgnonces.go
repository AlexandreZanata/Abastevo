package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/account"
)

// PGNonces is the DB-backed single-use nonce ledger for OIDC logins
// (P13-T03B). One nonce authorizes one link across processes: the primary
// key admits exactly one consumer, every other consumer observes the
// conflict as already consumed. Errors fail closed as consumed.
type PGNonces struct {
	pool *pgxpool.Pool
}

// NewPGNonces wires the nonce ledger to a pool.
func NewPGNonces(pool *pgxpool.Pool) *PGNonces {
	return &PGNonces{pool: pool}
}

// TryConsume reserves nonce, reporting false when already consumed.
func (n *PGNonces) TryConsume(ctx context.Context, nonce string, nowUnix int64) (bool, error) {
	if nonce == "" {
		return false, nil
	}
	if n == nil || n.pool == nil {
		return false, errors.New("adapters: missing nonce pool")
	}
	_, err := account.New(n.pool).InsertNonce(ctx, account.InsertNonceParams{
		Nonce:      nonce,
		ConsumedAt: stamp(nowUnix),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Consume satisfies the OIDC verifier port. DB errors fail closed.
func (n *PGNonces) Consume(nonce string) bool {
	if nonce == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, err := n.TryConsume(ctx, nonce, time.Now().Unix())
	if err != nil {
		return false
	}
	return ok
}
