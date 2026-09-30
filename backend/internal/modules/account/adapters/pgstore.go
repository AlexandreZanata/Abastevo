package adapters

import (
	"context"
	"encoding/hex"
	"errors"
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

// DeleteAccount marks the account deleted, revokes every session family
// and drops address and provider bindings atomically. The account row
// stays as an audit record; later signups mint new accounts.
func (s *PGStore) DeleteAccount(ctx context.Context, accountID string, nowUnix int64) error {
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
		Status: domain.StatusDeleted,
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
	if err := q.DeleteAccountAddresses(ctx, id); err != nil {
		return err
	}
	if err := q.DeleteAccountProviders(ctx, id); err != nil {
		return err
	}
	if err := q.DeleteAccountBindings(ctx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
