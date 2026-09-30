package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/account"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// PGStore implements application.Store on a pgx pool.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore wires the owned generated queries to a pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func mustUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(stripDashes(text))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("adapters: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func stamp(unix int64) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Unix(unix, 0).UTC(), Valid: true}
}

func unix(ts pgtype.Timestamptz) int64 {
	if !ts.Valid {
		return 0
	}
	return ts.Time.Unix()
}

func toCode(row account.AccountEmailCode) domain.EmailCode {
	return domain.EmailCode{
		ID:          uuidString(row.ID),
		AddressHash: row.AddressHash,
		Salt:        row.Salt,
		Hash:        row.Hash,
		IssuedAt:    unix(row.IssuedAt),
		Attempts:    int(row.Attempts),
		ConsumedAt:  unix(row.ConsumedAt),
	}
}

func toFamily(row account.AccountSessionFamily) domain.SessionFamily {
	return domain.SessionFamily{
		ID:            uuidString(row.ID),
		AccountID:     uuidString(row.AccountID),
		RefreshSalt:   row.RefreshSalt,
		RefreshHash:   row.RefreshHash,
		AccessSalt:    row.AccessSalt,
		AccessHash:    row.AccessHash,
		AccessExpires: unix(row.AccessExpires),
		IssuedAt:      unix(row.IssuedAt),
		RevokedAt:     unix(row.RevokedAt),
	}
}

func (s *PGStore) SaveCode(ctx context.Context, rec domain.EmailCode) error {
	id, err := mustUUID(rec.ID)
	if err != nil {
		return err
	}
	return account.New(s.pool).InsertCode(ctx, account.InsertCodeParams{
		ID:          id,
		AddressHash: rec.AddressHash,
		Salt:        rec.Salt,
		Hash:        rec.Hash,
		IssuedAt:    stamp(rec.IssuedAt),
	})
}

func (s *PGStore) CountCodesSince(ctx context.Context, addressHash string, sinceUnix int64) (int, error) {
	n, err := account.New(s.pool).CountCodesSince(ctx, account.CountCodesSinceParams{
		AddressHash: addressHash,
		Since:       stamp(sinceUnix),
	})
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (s *PGStore) LatestCode(ctx context.Context, addressHash string) (domain.EmailCode, bool, error) {
	row, err := account.New(s.pool).LatestCode(ctx, addressHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.EmailCode{}, false, nil
		}
		return domain.EmailCode{}, false, err
	}
	return toCode(row), true, nil
}

// TryConsume locks every code row of the address, matches the presented
// code by hash across rows and marks the outcome in the same transaction:
// concurrent consumers serialize on the row locks, so one code admits
// exactly one session, and same-second issuance never misattributes a
// guess to a sibling row.
func (s *PGStore) TryConsume(ctx context.Context, addressHash, code string, hasher domain.CodeHasher, nowUnix int64) (domain.EmailCode, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.EmailCode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	rows, err := q.LockAddressCodes(ctx, addressHash)
	if err != nil {
		return domain.EmailCode{}, err
	}
	if len(rows) == 0 {
		return domain.EmailCode{}, domain.ErrCodeUnknown
	}
	for _, row := range rows {
		if !hasher.Equal(row.Hash, row.Salt, code) {
			continue
		}
		rec := toCode(row)
		if rec.ConsumedAt != 0 {
			return domain.EmailCode{}, domain.ErrCodeConsumed
		}
		if domain.CodeExpired(rec.IssuedAt, nowUnix) {
			return domain.EmailCode{}, domain.ErrCodeExpired
		}
		if !domain.AttemptAllowed(rec.Attempts) {
			return domain.EmailCode{}, domain.ErrCodeAttemptsExhausted
		}
		if err := q.ConsumeCode(ctx, account.ConsumeCodeParams{Now: stamp(nowUnix), ID: row.ID}); err != nil {
			return domain.EmailCode{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.EmailCode{}, err
		}
		rec.ConsumedAt = nowUnix
		return rec, nil
	}
	// No match: wrong codes stay indistinguishable from unknown addresses.
	// States surface only on hash match (proof of possession). Attempts burn
	// on the newest live row when one exists.
	for _, row := range rows {
		rec := toCode(row)
		if rec.ConsumedAt != 0 || domain.CodeExpired(rec.IssuedAt, nowUnix) || !domain.AttemptAllowed(rec.Attempts) {
			continue
		}
		if err := q.BumpCodeAttempts(ctx, row.ID); err != nil {
			return domain.EmailCode{}, err
		}
		if !domain.AttemptAllowed(rec.Attempts + 1) {
			if err := tx.Commit(ctx); err != nil {
				return domain.EmailCode{}, err
			}
			return domain.EmailCode{}, domain.ErrCodeAttemptsExhausted
		}
		break
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.EmailCode{}, err
	}
	return domain.EmailCode{}, domain.ErrCodeUnknown
}

func (s *PGStore) FindAccount(ctx context.Context, addressHash string) (domain.Account, bool, error) {
	row, err := account.New(s.pool).FindAccountByAddress(ctx, addressHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Account{}, false, nil
		}
		return domain.Account{}, false, err
	}
	return domain.Account{
		ID:        uuidString(row.ID),
		Alias:     row.Alias,
		Status:    row.Status,
		CreatedAt: unix(row.CreatedAt),
	}, true, nil
}

// CreateAccount links an address idempotently: a parallel signup that won
// the race surfaces ErrAddressLinked so the caller logs into the winner.
func (s *PGStore) CreateAccount(ctx context.Context, acc domain.Account, addressHash string) error {
	id, err := mustUUID(acc.ID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	if err := q.InsertAccount(ctx, account.InsertAccountParams{
		ID:        id,
		Alias:     acc.Alias,
		Status:    acc.Status,
		CreatedAt: stamp(acc.CreatedAt),
	}); err != nil {
		return err
	}
	if err := q.InsertAddress(ctx, account.InsertAddressParams{
		AddressHash: addressHash,
		AccountID:   id,
		LinkedAt:    stamp(acc.CreatedAt),
	}); err != nil {
		return err
	}
	row, err := q.FindAccountByAddress(ctx, addressHash)
	if err != nil {
		return err
	}
	if uuidString(row.ID) != acc.ID {
		return domain.ErrAddressLinked
	}
	return tx.Commit(ctx)
}

func (s *PGStore) CreateFamily(ctx context.Context, fam domain.SessionFamily) error {
	id, err := mustUUID(fam.ID)
	if err != nil {
		return err
	}
	accountID, err := mustUUID(fam.AccountID)
	if err != nil {
		return err
	}
	return account.New(s.pool).InsertFamily(ctx, account.InsertFamilyParams{
		ID:            id,
		AccountID:     accountID,
		RefreshSalt:   fam.RefreshSalt,
		RefreshHash:   fam.RefreshHash,
		AccessSalt:    fam.AccessSalt,
		AccessHash:    fam.AccessHash,
		AccessExpires: stamp(fam.AccessExpires),
		IssuedAt:      stamp(fam.IssuedAt),
	})
}

func (s *PGStore) Family(ctx context.Context, familyID string) (domain.SessionFamily, bool, error) {
	id, err := mustUUID(familyID)
	if err != nil {
		return domain.SessionFamily{}, false, nil
	}
	row, err := account.New(s.pool).GetFamily(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.SessionFamily{}, false, nil
		}
		return domain.SessionFamily{}, false, err
	}
	return toFamily(row), true, nil
}

// Rotate swaps a family's tokens only when the stored refresh hash still
// matches the winner the caller verified: a lost rotation race fails with
// ErrSessionReuse instead of forking a parallel chain.
func (s *PGStore) Rotate(ctx context.Context, familyID, expectedRefreshHash, refreshSalt, refreshHash, accessSalt, accessHash string, accessExpires, _ int64) error {
	id, err := mustUUID(familyID)
	if err != nil {
		return domain.ErrCodeUnknown
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	row, err := q.LockFamily(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrCodeUnknown
		}
		return err
	}
	if row.RevokedAt.Valid {
		return domain.ErrSessionRevoked
	}
	if !domain.EqualHash(row.RefreshHash, expectedRefreshHash) {
		return domain.ErrSessionReuse
	}
	if err := q.RotateFamily(ctx, account.RotateFamilyParams{
		ID:            id,
		RefreshSalt:   refreshSalt,
		RefreshHash:   refreshHash,
		AccessSalt:    accessSalt,
		AccessHash:    accessHash,
		AccessExpires: stamp(accessExpires),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PGStore) RevokeFamily(ctx context.Context, familyID string, nowUnix int64) error {
	id, err := mustUUID(familyID)
	if err != nil {
		return nil
	}
	return account.New(s.pool).RevokeFamily(ctx, account.RevokeFamilyParams{
		Now: stamp(nowUnix),
		ID:  id,
	})
}

func (s *PGStore) RevokeAccount(ctx context.Context, accountID string, nowUnix int64) error {
	id, err := mustUUID(accountID)
	if err != nil {
		return err
	}
	return account.New(s.pool).RevokeAccountFamilies(ctx, account.RevokeAccountFamiliesParams{
		Now:       stamp(nowUnix),
		AccountID: id,
	})
}
