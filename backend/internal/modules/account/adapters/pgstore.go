package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// CountAllCodesSince counts issued codes across all addresses (SEC01-C
// aggregate mail accounting).
func (s *PGStore) CountAllCodesSince(ctx context.Context, sinceUnix int64) (int, error) {
	n, err := account.New(s.pool).CountAllCodesSince(ctx, stamp(sinceUnix))
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// CountKeyAccountsSince counts key-account creations in the window
// (SEC01-C aggregate username accounting; created_at is unix seconds).
func (s *PGStore) CountKeyAccountsSince(ctx context.Context, sinceUnix int64) (int, error) {
	n, err := account.New(s.pool).CountKeyAccountsSince(ctx, sinceUnix)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// CountLiveFamiliesByAccount counts live session families (SEC01-C
// per-account session accounting): non-revoked and inside the
// absolute refresh lifetime at nowUnix.
func (s *PGStore) CountLiveFamiliesByAccount(ctx context.Context, accountID string, nowUnix int64) (int, error) {
	id, err := mustUUID(accountID)
	if err != nil {
		return 0, domain.ErrAccountNotFound
	}
	liveSince := nowUnix - int64(domain.RefreshAbsoluteLifetimeDays)*24*3600
	n, err := account.New(s.pool).CountLiveFamiliesByAccount(ctx, account.CountLiveFamiliesByAccountParams{
		AccountID: id,
		LiveSince: stamp(liveSince),
	})
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// PurgeExpiredCodes deletes code verifiers older than beforeUnix
// (SEC01-C bounded retention).
func (s *PGStore) PurgeExpiredCodes(ctx context.Context, beforeUnix int64) (int, error) {
	n, err := account.New(s.pool).PurgeExpiredCodes(ctx, stamp(beforeUnix))
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// PurgeExpiredFamilies deletes absolutely-expired families older than
// beforeUnix (SEC01-C bounded retention).
func (s *PGStore) PurgeExpiredFamilies(ctx context.Context, beforeUnix int64) (int, error) {
	n, err := account.New(s.pool).PurgeExpiredFamilies(ctx, stamp(beforeUnix))
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// DeleteCode compensates one issued code by ID (SEC01-D quota
// rollback). Unknown IDs converge no-op.
func (s *PGStore) DeleteCode(ctx context.Context, id string) error {
	uid, err := mustUUID(id)
	if err != nil {
		return nil
	}
	_, err = account.New(s.pool).DeleteCodeByID(ctx, uid)
	return err
}

// TryIssueCode atomically admits one code issuance (SEC01-C): global
// advisory lock serializes concurrent issuers, then per-address quota,
// aggregate ceiling, account usability and cooldown admit inside one
// transaction. Refusals return admitted=false with no rows and no
// oracle.
func (s *PGStore) TryIssueCode(ctx context.Context, rec domain.EmailCode, nowUnix int64) (bool, error) {
	id, err := mustUUID(rec.ID)
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(810001)`); err != nil {
		return false, err
	}
	q := account.New(tx)
	if row, err := q.FindAccountByAddress(ctx, rec.AddressHash); err == nil {
		if row.Status != domain.StatusActive {
			return false, nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if n, err := q.CountCodesSince(ctx, account.CountCodesSinceParams{
		AddressHash: rec.AddressHash, Since: stamp(nowUnix - 3600),
	}); err != nil {
		return false, err
	} else if int(n) >= domain.MaxCodesPerAddressPerHour {
		return false, nil
	}
	if n, err := q.CountAllCodesSince(ctx, stamp(nowUnix-3600)); err != nil {
		return false, err
	} else if int(n) >= domain.MaxAggregateCodesPerHour {
		return false, nil
	}
	if latest, err := q.LatestCode(ctx, rec.AddressHash); err == nil {
		lr := toCode(latest)
		if lr.ConsumedAt == 0 && !domain.CodeExpired(lr.IssuedAt, nowUnix) &&
			!domain.ResendAllowed(lr.IssuedAt, nowUnix) {
			return false, nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if err := q.InsertCode(ctx, account.InsertCodeParams{
		ID: id, AddressHash: rec.AddressHash, Salt: rec.Salt, Hash: rec.Hash, IssuedAt: stamp(rec.IssuedAt),
	}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// TryCreateKeyAccount atomically admits one key-account creation
// (SEC01-C): aggregate hourly ceiling plus username/key uniqueness in
// one transaction under a global advisory lock.
func (s *PGStore) TryCreateKeyAccount(ctx context.Context, acc domain.Account, cred domain.KeyCredential, nowUnix int64) error {
	id, err := mustUUID(acc.ID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(810002)`); err != nil {
		return err
	}
	q := account.New(tx)
	if _, err := q.PruneKeyAccountAdmissions(ctx, nowUnix-3600); err != nil {
		return err
	}
	if n, err := q.CountKeyAccountsSince(ctx, nowUnix-3600); err != nil {
		return err
	} else if int(n) >= domain.MaxKeyAccountsPerHour {
		return domain.ErrAggregateExhausted
	}
	if err := q.InsertAccount(ctx, account.InsertAccountParams{
		ID: id, Alias: acc.Alias, Status: acc.Status, CreatedAt: stamp(acc.CreatedAt),
	}); err != nil {
		return err
	}
	if err := q.InsertKeyCredential(ctx, account.InsertKeyCredentialParams{
		AccountID: id, UsernameHash: cred.UsernameHash, KeyLookup: cred.KeyLookup,
		KeySalt: cred.KeySalt, KeyHash: cred.KeyHash, CreatedAt: cred.CreatedAt,
	}); err != nil {
		return keyCredentialConflict(err)
	}
	if err := q.RecordKeyAccountAdmission(ctx, nowUnix); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// TryCreateFamily atomically admits one session family (SEC01-C):
// per-account live budget plus insert in one transaction under a
// per-account advisory lock.
func (s *PGStore) TryCreateFamily(ctx context.Context, fam domain.SessionFamily, nowUnix int64) error {
	id, err := mustUUID(fam.ID)
	if err != nil {
		return err
	}
	accountID, err := mustUUID(fam.AccountID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('families:' || $1))`, fam.AccountID); err != nil {
		return err
	}
	q := account.New(tx)
	liveSince := nowUnix - int64(domain.RefreshAbsoluteLifetimeDays)*24*3600
	if n, err := q.CountLiveFamiliesByAccount(ctx, account.CountLiveFamiliesByAccountParams{
		AccountID: accountID, LiveSince: stamp(liveSince),
	}); err != nil {
		return err
	} else if int(n) >= domain.MaxLiveSessionsPerAccount {
		return domain.ErrSessionBudgetExhausted
	}
	if err := q.InsertFamily(ctx, account.InsertFamilyParams{
		ID: id, AccountID: accountID, RefreshSalt: fam.RefreshSalt, RefreshHash: fam.RefreshHash,
		AccessSalt: fam.AccessSalt, AccessHash: fam.AccessHash,
		AccessExpires: stamp(fam.AccessExpires), IssuedAt: stamp(fam.IssuedAt),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
// ErrSessionReuse instead of forking a parallel chain. The superseded
// verifier archives into the bounded hashed rotation history (SEC07-A)
// in the same transaction, pruned to domain.MaxRefreshHistory.
func (s *PGStore) Rotate(ctx context.Context, familyID, expectedRefreshHash, refreshSalt, refreshHash, accessSalt, accessHash string, accessExpires, nowUnix int64) error {
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
	if err := q.InsertRefreshHistory(ctx, account.InsertRefreshHistoryParams{
		FamilyID:    id,
		RefreshSalt: row.RefreshSalt,
		RefreshHash: row.RefreshHash,
		RotatedAt:   stamp(nowUnix),
	}); err != nil {
		return err
	}
	if _, err := q.PruneRefreshHistory(ctx, account.PruneRefreshHistoryParams{
		FamilyID: id,
		Keep:     int32(domain.MaxRefreshHistory),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RefreshHistory returns the bounded hashed rotation history, newest
// first (SEC07-A). Salted hashes only; callers compare with the token
// hasher and never log the values.
func (s *PGStore) RefreshHistory(ctx context.Context, familyID string) ([]domain.RotatedVerifier, error) {
	id, err := mustUUID(familyID)
	if err != nil {
		return nil, nil
	}
	rows, err := account.New(s.pool).ListRefreshHistory(ctx, account.ListRefreshHistoryParams{
		FamilyID: id,
		Cap:      int32(domain.MaxRefreshHistory),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.RotatedVerifier, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.RotatedVerifier{Salt: r.RefreshSalt, Hash: r.RefreshHash})
	}
	return out, nil
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

func toLink(row account.AccountProviderLink) domain.ProviderLink {
	return domain.ProviderLink{
		AccountID: uuidString(row.AccountID),
		Provider:  row.Provider,
		Issuer:    row.Issuer,
		Subject:   row.Subject,
		Email:     row.Email,
		LinkedAt:  unix(row.LinkedAt),
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// LinkProvider binds a verified subject, refusing cross-account binds.
// Same-account relinks converge; provider rotation within one account
// updates the stored subject. A lost unique race reports cross-account.
func (s *PGStore) LinkProvider(ctx context.Context, link domain.ProviderLink) error {
	if !domain.ValidProvider(link.Provider) || link.Issuer == "" || link.Subject == "" {
		return domain.ErrOIDCWrongAudience
	}
	accountID, err := mustUUID(link.AccountID)
	if err != nil {
		return domain.ErrAccountNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	row, err := q.GetAccountByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAccountNotFound
		}
		return err
	}
	if row.Status == domain.StatusDeleted {
		return domain.ErrAccountDeleted
	}
	if row.Status != domain.StatusActive {
		return domain.ErrAccountSuspended
	}
	if owner, err := q.FindProviderOwner(ctx, account.FindProviderOwnerParams{
		Provider: link.Provider,
		Subject:  link.Subject,
	}); err == nil {
		if uuidString(owner.AccountID) != link.AccountID {
			return domain.ErrLinkCrossAccount
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if existing, err := q.GetProviderLink(ctx, account.GetProviderLinkParams{
		AccountID: accountID,
		Provider:  link.Provider,
	}); err == nil {
		if existing.Subject == link.Subject && existing.Issuer == link.Issuer {
			if err := q.UpdateProviderLink(ctx, account.UpdateProviderLinkParams{
				AccountID: accountID,
				Provider:  link.Provider,
				Issuer:    link.Issuer,
				Subject:   link.Subject,
				Email:     link.Email,
				LinkedAt:  stamp(link.LinkedAt),
			}); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		if err := q.UpdateProviderLink(ctx, account.UpdateProviderLinkParams{
			AccountID: accountID,
			Provider:  link.Provider,
			Issuer:    link.Issuer,
			Subject:   link.Subject,
			Email:     link.Email,
			LinkedAt:  stamp(link.LinkedAt),
		}); err != nil {
			if isUniqueViolation(err) {
				return domain.ErrLinkCrossAccount
			}
			return err
		}
		return tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := q.InsertProviderLink(ctx, account.InsertProviderLinkParams{
		AccountID: accountID,
		Provider:  link.Provider,
		Issuer:    link.Issuer,
		Subject:   link.Subject,
		Email:     link.Email,
		LinkedAt:  stamp(link.LinkedAt),
	}); err != nil {
		if isUniqueViolation(err) {
			return domain.ErrLinkCrossAccount
		}
		return err
	}
	return tx.Commit(ctx)
}

// UnlinkProvider removes one binding, refusing the last login method.
func (s *PGStore) UnlinkProvider(ctx context.Context, accountID, provider string) error {
	if !domain.ValidProvider(provider) {
		return domain.ErrOIDCUnknownIssuer
	}
	id, err := mustUUID(accountID)
	if err != nil {
		return domain.ErrAccountNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	row, err := q.GetAccountByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAccountNotFound
		}
		return err
	}
	if row.Status == domain.StatusDeleted {
		return domain.ErrAccountDeleted
	}
	if row.Status != domain.StatusActive {
		return domain.ErrAccountSuspended
	}
	if _, err := q.GetProviderLink(ctx, account.GetProviderLinkParams{
		AccountID: id,
		Provider:  provider,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrProviderNotLinked
		}
		return err
	}
	links, err := q.ListProviderLinks(ctx, id)
	if err != nil {
		return err
	}
	nAddr, err := q.CountAddressesByAccount(ctx, id)
	if err != nil {
		return err
	}
	if int64(len(links))+nAddr <= 1 {
		return domain.ErrLastLoginMethod
	}
	if err := q.DeleteProviderLink(ctx, account.DeleteProviderLinkParams{
		AccountID: id,
		Provider:  provider,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListProviders returns the provider bindings of one account.
func (s *PGStore) ListProviders(ctx context.Context, accountID string) ([]domain.ProviderLink, error) {
	id, err := mustUUID(accountID)
	if err != nil {
		return nil, domain.ErrAccountNotFound
	}
	rows, err := account.New(s.pool).ListProviderLinks(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProviderLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLink(r))
	}
	return out, nil
}

// FindProviderOwner resolves the account owning a provider subject.
func (s *PGStore) FindProviderOwner(ctx context.Context, provider, subject string) (string, bool, error) {
	row, err := account.New(s.pool).FindProviderOwner(ctx, account.FindProviderOwnerParams{
		Provider: provider,
		Subject:  subject,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return uuidString(row.AccountID), true, nil
}

// CountAddresses counts email bindings of one account.
func (s *PGStore) CountAddresses(ctx context.Context, accountID string) (int, error) {
	id, err := mustUUID(accountID)
	if err != nil {
		return 0, domain.ErrAccountNotFound
	}
	n, err := account.New(s.pool).CountAddressesByAccount(ctx, id)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// TryConsumeNonce reserves a login nonce exactly once across processes.
func (s *PGStore) TryConsumeNonce(ctx context.Context, nonce string, nowUnix int64) (bool, error) {
	if nonce == "" {
		return false, nil
	}
	_, err := account.New(s.pool).InsertNonce(ctx, account.InsertNonceParams{
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

// GetAccount resolves an account by ID, including suspended/deleted rows.
func (s *PGStore) GetAccount(ctx context.Context, accountID string) (domain.Account, bool, error) {
	id, err := mustUUID(accountID)
	if err != nil {
		return domain.Account{}, false, nil
	}
	row, err := account.New(s.pool).GetAccountByID(ctx, id)
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

// SuspendAccount marks the account suspended and revokes every session
// family in one transaction, so no live session survives the status
// change.
func (s *PGStore) SuspendAccount(ctx context.Context, accountID string, nowUnix int64) error {
	id, err := mustUUID(accountID)
	if err != nil {
		return domain.ErrAccountNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	if _, err := q.GetAccountByID(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAccountNotFound
		}
		return err
	}
	if err := q.SetAccountStatus(ctx, account.SetAccountStatusParams{
		Status: domain.StatusSuspended,
		ID:     id,
	}); err != nil {
		return err
	}
	if err := q.RevokeAccountFamilies(ctx, account.RevokeAccountFamiliesParams{
		Now:       stamp(nowUnix),
		AccountID: id,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReactivateAccount returns a suspended account to active. Revoked
// sessions stay revoked; the owner logs in again. Reactivating a
// deleted account refuses: deletion is terminal, a new signup mints a
// new account instead.
func (s *PGStore) ReactivateAccount(ctx context.Context, accountID string) error {
	id, err := mustUUID(accountID)
	if err != nil {
		return domain.ErrAccountNotFound
	}
	row, err := account.New(s.pool).GetAccountByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAccountNotFound
		}
		return err
	}
	if row.Status == domain.StatusDeleted {
		return domain.ErrAccountDeleted
	}
	return account.New(s.pool).SetAccountStatus(ctx, account.SetAccountStatusParams{
		Status: domain.StatusActive,
		ID:     id,
	})
}

// DeleteAccount marks the account deleted, revokes every session family,
// captures contributor bindings into durable erasure intents BEFORE
// dropping them, and drops address and provider bindings — all in one
// transaction, so revocation and intent are atomic (SEC04-B). The
// account row stays as an audit record; later signups mint new
// accounts. It returns the captured contributor IDs for scope cleanup.
func (s *PGStore) DeleteAccount(ctx context.Context, accountID string, nowUnix int64) ([]string, error) {
	id, err := mustUUID(accountID)
	if err != nil {
		return nil, domain.ErrAccountNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	if _, err := q.GetAccountByID(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, err
	}
	if err := q.SetAccountStatus(ctx, account.SetAccountStatusParams{
		Status: domain.StatusDeleted,
		ID:     id,
	}); err != nil {
		return nil, err
	}
	if err := q.RevokeAccountFamilies(ctx, account.RevokeAccountFamiliesParams{
		Now:       stamp(nowUnix),
		AccountID: id,
	}); err != nil {
		return nil, err
	}
	var contributors []string
	if rows, err := q.ListBindings(ctx, id); err != nil {
		return nil, err
	} else {
		for _, row := range rows {
			contributors = append(contributors, row.ContributorID)
		}
	}
	sort.Strings(contributors)
	for _, contributor := range contributors {
		if _, err := q.InsertErasureIntent(ctx, account.InsertErasureIntentParams{
			AccountID: id, ContributorID: contributor, CreatedAt: stamp(nowUnix),
		}); err != nil {
			return nil, err
		}
	}
	if err := q.DeleteAccountAddresses(ctx, id); err != nil {
		return nil, err
	}
	if err := q.DeleteAccountProviders(ctx, id); err != nil {
		return nil, err
	}
	// SEC04-C: key verifiers never survive the owner.
	if err := q.DeleteAccountKeys(ctx, id); err != nil {
		return nil, err
	}
	if err := q.DeleteAccountBindings(ctx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return contributors, nil
}

// ListErasureIntents returns the durable contributor intents of one
// account for enqueue replay after a crash between revocation and
// scope cleanup (SEC04-B/D).
func (s *PGStore) ListErasureIntents(ctx context.Context, accountID string) ([]string, error) {
	id, err := mustUUID(accountID)
	if err != nil {
		return nil, domain.ErrAccountNotFound
	}
	rows, err := account.New(s.pool).ListErasureIntentsByAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ContributorID)
	}
	return out, nil
}

func toBinding(row account.AccountContributorBinding) domain.ContributorBinding {
	return domain.ContributorBinding{
		AccountID:      uuidString(row.AccountID),
		ContributorID:  row.ContributorID,
		KeyFingerprint: row.KeyFingerprint,
		BoundAt:        unix(row.BoundAt),
	}
}

func toBindingAudit(row account.AccountBindingAudit) domain.BindingAudit {
	return domain.BindingAudit{
		ID:            uuidString(row.ID),
		AccountID:     uuidString(row.AccountID),
		ContributorID: row.ContributorID,
		Action:        row.Action,
		OccurredAt:    unix(row.OccurredAt),
	}
}

// BindContributor records a verified contributor binding, refusing
// stolen-ID binds. Same-account relinks converge; a lost unique race
// reports cross-account, like the provider-link lane.
func (s *PGStore) BindContributor(ctx context.Context, binding domain.ContributorBinding, auditID string) error {
	if binding.AccountID == "" || binding.ContributorID == "" || binding.KeyFingerprint == "" {
		return domain.ErrBindingInvalid
	}
	accountID, err := mustUUID(binding.AccountID)
	if err != nil {
		return domain.ErrAccountNotFound
	}
	auditUUID, err := mustUUID(auditID)
	if err != nil {
		return domain.ErrBindingInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	row, err := q.GetAccountByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAccountNotFound
		}
		return err
	}
	if row.Status == domain.StatusDeleted {
		return domain.ErrAccountDeleted
	}
	if row.Status != domain.StatusActive {
		return domain.ErrAccountSuspended
	}
	if owner, err := q.FindBindingOwner(ctx, binding.ContributorID); err == nil {
		if uuidString(owner.AccountID) != binding.AccountID {
			return domain.ErrBindingCrossAccount
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err := q.GetBinding(ctx, account.GetBindingParams{
		AccountID:     accountID,
		ContributorID: binding.ContributorID,
	}); err == nil {
		if err := q.UpdateBinding(ctx, account.UpdateBindingParams{
			AccountID:      accountID,
			ContributorID:  binding.ContributorID,
			KeyFingerprint: binding.KeyFingerprint,
			BoundAt:        stamp(binding.BoundAt),
		}); err != nil {
			if isUniqueViolation(err) {
				return domain.ErrBindingCrossAccount
			}
			return err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	} else if err := q.InsertBinding(ctx, account.InsertBindingParams{
		AccountID:      accountID,
		ContributorID:  binding.ContributorID,
		KeyFingerprint: binding.KeyFingerprint,
		BoundAt:        stamp(binding.BoundAt),
	}); err != nil {
		if isUniqueViolation(err) {
			return domain.ErrBindingCrossAccount
		}
		return err
	}
	if err := q.InsertBindingAudit(ctx, account.InsertBindingAuditParams{
		ID:            auditUUID,
		AccountID:     accountID,
		ContributorID: binding.ContributorID,
		Action:        domain.BindingActionBind,
		OccurredAt:    stamp(binding.BoundAt),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UnbindContributor removes one binding and audits the removal.
func (s *PGStore) UnbindContributor(ctx context.Context, accountID, contributorID, auditID string, nowUnix int64) error {
	if accountID == "" || contributorID == "" {
		return domain.ErrBindingInvalid
	}
	id, err := mustUUID(accountID)
	if err != nil {
		return domain.ErrAccountNotFound
	}
	auditUUID, err := mustUUID(auditID)
	if err != nil {
		return domain.ErrBindingInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	row, err := q.GetAccountByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAccountNotFound
		}
		return err
	}
	if row.Status == domain.StatusDeleted {
		return domain.ErrAccountDeleted
	}
	if row.Status != domain.StatusActive {
		return domain.ErrAccountSuspended
	}
	if _, err := q.GetBinding(ctx, account.GetBindingParams{
		AccountID:     id,
		ContributorID: contributorID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrBindingNotFound
		}
		return err
	}
	if err := q.DeleteBinding(ctx, account.DeleteBindingParams{
		AccountID:     id,
		ContributorID: contributorID,
	}); err != nil {
		return err
	}
	if err := q.InsertBindingAudit(ctx, account.InsertBindingAuditParams{
		ID:            auditUUID,
		AccountID:     id,
		ContributorID: contributorID,
		Action:        domain.BindingActionUnbind,
		OccurredAt:    stamp(nowUnix),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListBindings returns the contributor bindings of one account.
func (s *PGStore) ListBindings(ctx context.Context, accountID string) ([]domain.ContributorBinding, error) {
	id, err := mustUUID(accountID)
	if err != nil {
		return nil, domain.ErrAccountNotFound
	}
	rows, err := account.New(s.pool).ListBindings(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ContributorBinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, toBinding(r))
	}
	return out, nil
}

// FindBindingOwner resolves the account owning a contributor binding.
func (s *PGStore) FindBindingOwner(ctx context.Context, contributorID string) (string, bool, error) {
	row, err := account.New(s.pool).FindBindingOwner(ctx, contributorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return uuidString(row.AccountID), true, nil
}

// ListBindingAudit returns the recovery audit trail of one account.
func (s *PGStore) ListBindingAudit(ctx context.Context, accountID string) ([]domain.BindingAudit, error) {
	id, err := mustUUID(accountID)
	if err != nil {
		return nil, domain.ErrAccountNotFound
	}
	rows, err := account.New(s.pool).ListBindingAudit(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]domain.BindingAudit, 0, len(rows))
	for _, r := range rows {
		out = append(out, toBindingAudit(r))
	}
	return out, nil
}

// toKeyCredential maps a key-credential row to the domain shape.
func toKeyCredential(row account.AccountKeyCredential) domain.KeyCredential {
	return domain.KeyCredential{
		AccountID:    uuidString(row.AccountID),
		UsernameHash: row.UsernameHash,
		KeyLookup:    row.KeyLookup,
		KeySalt:      row.KeySalt,
		KeyHash:      row.KeyHash,
		CreatedAt:    row.CreatedAt,
	}
}

// CreateKeyAccount inserts the account and its key credential atomically.
// A lost username race surfaces ErrUsernameTaken; a lost key race surfaces
// ErrKeyCollision so the caller mints a fresh key and retries.
func (s *PGStore) CreateKeyAccount(ctx context.Context, acc domain.Account, cred domain.KeyCredential) error {
	return s.TryCreateKeyAccount(ctx, acc, cred, cred.CreatedAt)
}

// keyCredentialConflict preserves username/key race verdicts for admission.
func keyCredentialConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if strings.Contains(pgErr.ConstraintName, "key_lookup") {
			return domain.ErrKeyCollision
		}
		return domain.ErrUsernameTaken
	}
	return err
}

// FindKeyCredentialByUsername returns the credential for a username hash,
// or false when no account uses it.
func (s *PGStore) FindKeyCredentialByUsername(ctx context.Context, usernameHash string) (domain.KeyCredential, bool, error) {
	row, err := account.New(s.pool).FindKeyCredentialByUsername(ctx, usernameHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.KeyCredential{}, false, nil
		}
		return domain.KeyCredential{}, false, err
	}
	return toKeyCredential(row), true, nil
}

// FindKeyCredentialByLookup returns the credential for a key lookup, or
// false when no account holds it.
func (s *PGStore) FindKeyCredentialByLookup(ctx context.Context, lookup string) (domain.KeyCredential, bool, error) {
	row, err := account.New(s.pool).FindKeyCredentialByLookup(ctx, lookup)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.KeyCredential{}, false, nil
		}
		return domain.KeyCredential{}, false, err
	}
	return toKeyCredential(row), true, nil
}
