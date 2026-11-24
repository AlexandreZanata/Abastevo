package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/account"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func toNamedAccount(row account.Account) domain.Account {
	return domain.Account{
		ID: uuidString(row.ID), Alias: row.Alias, Status: row.Status,
		CreatedAt: unix(row.CreatedAt), DisplayName: row.DisplayName.String,
	}
}

func lockRecoveryAddress(ctx context.Context, tx pgx.Tx, addressHash string) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('recovery-email:' || $1, 0))", addressHash)
	return err
}

func admitKeyAccount(ctx context.Context, tx pgx.Tx, q *account.Queries, now int64) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(810002)"); err != nil {
		return err
	}
	if _, err := q.PruneKeyAccountAdmissions(ctx, now-3600); err != nil {
		return err
	}
	n, err := q.CountKeyAccountsSince(ctx, now-3600)
	if err != nil {
		return err
	}
	if n >= domain.MaxKeyAccountsPerHour {
		return domain.ErrAggregateExhausted
	}
	return nil
}

func lockFamilyAdmission(ctx context.Context, tx pgx.Tx, accountID string) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('families:' || $1))", accountID)
	return err
}

func checkFamilyBudget(ctx context.Context, q *account.Queries, accountID pgtype.UUID, now int64) error {
	n, err := q.CountLiveFamiliesByAccount(ctx, account.CountLiveFamiliesByAccountParams{
		AccountID: accountID, LiveSince: stamp(now - int64(domain.RefreshAbsoluteLifetimeDays)*24*3600),
	})
	if err != nil {
		return err
	}
	if n >= domain.MaxLiveSessionsPerAccount {
		return domain.ErrSessionBudgetExhausted
	}
	return nil
}

func insertSessionFamily(ctx context.Context, q *account.Queries, id, accountID pgtype.UUID, fam domain.SessionFamily) error {
	return q.InsertFamily(ctx, account.InsertFamilyParams{
		ID: id, AccountID: accountID, RefreshSalt: fam.RefreshSalt, RefreshHash: fam.RefreshHash,
		AccessSalt: fam.AccessSalt, AccessHash: fam.AccessHash,
		AccessExpires: stamp(fam.AccessExpires), IssuedAt: stamp(fam.IssuedAt),
	})
}

// commitCodeVerdict persists failed guesses and verified business refusals.
// Storage failures and unrelated use-case refusals roll back all writes.
func commitCodeVerdict(ctx context.Context, tx pgx.Tx, verdict error) error {
	switch verdict {
	case nil, domain.ErrCodeUnknown, domain.ErrCodeAttemptsExhausted, domain.ErrCodeConsumed, domain.ErrCodeExpired,
		domain.ErrAddressLinked, domain.ErrRecoveryNoAccount:
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return verdict
}

func (s *PGStore) SignUpWithCode(ctx context.Context, proof application.EmailProof, hasher domain.CodeHasher, acc domain.Account, cred domain.KeyCredential, fam domain.SessionFamily) error {
	id, err := mustUUID(acc.ID)
	if err != nil {
		return err
	}
	familyID, err := mustUUID(fam.ID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	if err := lockRecoveryAddress(ctx, tx, proof.Address.Hash); err != nil {
		return err
	}
	if _, err := consumeAddressCode(ctx, q, proof.Address.Hash, proof.Code, hasher, proof.Now, false); err != nil {
		return commitCodeVerdict(ctx, tx, err)
	}
	if _, err := q.FindAccountByAddress(ctx, proof.Address.Hash); err == nil {
		return commitCodeVerdict(ctx, tx, domain.ErrAddressLinked)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := admitKeyAccount(ctx, tx, q, proof.Now); err != nil {
		return err
	}
	if err := q.InsertNamedAccount(ctx, account.InsertNamedAccountParams{
		ID: id, Alias: acc.Alias, Status: acc.Status, CreatedAt: stamp(acc.CreatedAt), DisplayName: pgtext(acc.DisplayName),
	}); err != nil {
		return err
	}
	if err := insertRecoveryAddress(ctx, q, id, proof.Address, proof.Now); err != nil {
		return err
	}
	if err := q.InsertKeyCredential(ctx, account.InsertKeyCredentialParams{
		AccountID: id, UsernameHash: cred.UsernameHash, KeyLookup: cred.KeyLookup,
		KeySalt: cred.KeySalt, KeyHash: cred.KeyHash, CreatedAt: cred.CreatedAt,
	}); err != nil {
		return keyCredentialConflict(err)
	}
	if err := q.RecordKeyAccountAdmission(ctx, proof.Now); err != nil {
		return err
	}
	if err := insertSessionFamily(ctx, q, familyID, id, fam); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PGStore) RecoverWithCode(ctx context.Context, proof application.EmailProof, hasher domain.CodeHasher, cred domain.KeyCredential, fam domain.SessionFamily, auditID string) (domain.Account, error) {
	familyID, err := mustUUID(fam.ID)
	if err != nil {
		return domain.Account{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Account{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	if err := lockRecoveryAddress(ctx, tx, proof.Address.Hash); err != nil {
		return domain.Account{}, err
	}
	row, err := lockRecoveryOwner(ctx, q, proof.Address.Hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, err
	}
	if err == nil && row.Status != domain.StatusActive {
		// Refuse a proven suspended account without spending its code.
		// An incorrect guess cannot reveal that the address is suspended.
		if _, err := verifyAddressCode(ctx, q, proof.Address.Hash, proof.Code, hasher, proof.Now, addressCodeCheck{}); err != nil {
			return domain.Account{}, err
		}
		return domain.Account{}, checkAccountStatus(row.Status)
	}
	if _, checkErr := consumeAddressCode(ctx, q, proof.Address.Hash, proof.Code, hasher, proof.Now, false); checkErr != nil {
		return domain.Account{}, commitCodeVerdict(ctx, tx, checkErr)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, commitCodeVerdict(ctx, tx, domain.ErrRecoveryNoAccount)
	}

	acc := toNamedAccount(row)
	old, err := q.FindKeyCredentialByAccount(ctx, row.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, err
	}
	if old.KeyLookup == cred.KeyLookup {
		return domain.Account{}, domain.ErrKeyCollision
	}
	if err := q.ReplaceKeyCredential(ctx, account.ReplaceKeyCredentialParams{
		AccountID: row.ID, UsernameHash: domain.UsernameHash("account:" + acc.ID),
		KeyLookup: cred.KeyLookup, KeySalt: cred.KeySalt, KeyHash: cred.KeyHash, CreatedAt: cred.CreatedAt,
	}); err != nil {
		return domain.Account{}, keyCredentialConflict(err)
	}
	if err := q.RevokeAccountFamilies(ctx, account.RevokeAccountFamiliesParams{Now: stamp(proof.Now), AccountID: row.ID}); err != nil {
		return domain.Account{}, err
	}
	if err := writeRecoveryAudit(ctx, q, row.ID, auditID, domain.RecoveryAction, proof.Now); err != nil {
		return domain.Account{}, err
	}
	if err := insertSessionFamily(ctx, q, familyID, row.ID, fam); err != nil {
		return domain.Account{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Account{}, err
	}
	return acc, nil
}

func lockRecoveryOwner(ctx context.Context, q *account.Queries, addressHash string) (account.Account, error) {
	owner, err := q.FindAccountByAddress(ctx, addressHash)
	if err != nil {
		return account.Account{}, err
	}
	row, err := q.LockNamedAccount(ctx, owner.ID)
	if err != nil {
		return account.Account{}, err
	}
	// A deletion or replacement of the old address might have released it
	// while the account row lock was pending. Check ownership again.
	stillOwner, err := q.FindAccountByAddress(ctx, addressHash)
	if err != nil {
		return account.Account{}, err
	}
	if stillOwner.ID != row.ID {
		return account.Account{}, pgx.ErrNoRows
	}
	return row, nil
}

func checkAccountStatus(status string) error {
	switch status {
	case domain.StatusActive:
		return nil
	case domain.StatusSuspended:
		return domain.ErrAccountSuspended
	case domain.StatusDeleted:
		return domain.ErrAccountDeleted
	default:
		return domain.ErrAccountNotFound
	}
}

func (s *PGStore) TryCreateKeyFamily(ctx context.Context, fam domain.SessionFamily, verified domain.KeyCredential) (domain.Account, error) {
	id, err := mustUUID(fam.AccountID)
	if err != nil {
		return domain.Account{}, err
	}
	familyID, err := mustUUID(fam.ID)
	if err != nil {
		return domain.Account{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Account{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	// The normal session admission lock precedes the account row lock.
	// Recovery only locks the row. It may therefore revoke this family
	// after insertion, or make the credential recheck fail before insertion.
	if err := lockFamilyAdmission(ctx, tx, fam.AccountID); err != nil {
		return domain.Account{}, err
	}
	row, err := q.LockNamedAccount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.ErrKeyInvalid
	}
	if err != nil {
		return domain.Account{}, err
	}
	current, err := q.FindKeyCredentialByAccount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.ErrKeyInvalid
	}
	if err != nil {
		return domain.Account{}, err
	}
	if toKeyCredential(current) != verified {
		return domain.Account{}, domain.ErrKeyInvalid
	}
	if err := checkAccountStatus(row.Status); err != nil {
		return domain.Account{}, err
	}
	if err := checkFamilyBudget(ctx, q, id, fam.IssuedAt); err != nil {
		return domain.Account{}, err
	}
	if err := insertSessionFamily(ctx, q, familyID, id, fam); err != nil {
		return domain.Account{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Account{}, err
	}
	return toNamedAccount(row), nil
}

func lockProfileAccess(ctx context.Context, q *account.Queries, proof domain.SessionProof, fresh bool) (account.Account, error) {
	id, err := mustUUID(proof.AccountID)
	if err != nil {
		return account.Account{}, domain.ErrAccountNotFound
	}
	row, err := q.LockNamedAccount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return account.Account{}, domain.ErrAccountNotFound
	}
	if err != nil {
		return account.Account{}, err
	}
	if err := checkAccountStatus(row.Status); err != nil {
		return account.Account{}, err
	}
	familyID, err := mustUUID(proof.FamilyID)
	if err != nil {
		return account.Account{}, domain.ErrCodeUnknown
	}
	family, err := q.LockFamily(ctx, familyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return account.Account{}, domain.ErrCodeUnknown
	}
	if err != nil {
		return account.Account{}, err
	}
	return row, proof.Verify(toFamily(family), fresh)
}

func (s *PGStore) ReadProfile(ctx context.Context, proof domain.SessionProof) (domain.AccountProfile, error) {
	return s.profileWithName(ctx, proof, "")
}

func (s *PGStore) UpdateProfileName(ctx context.Context, proof domain.SessionProof, name string) (domain.AccountProfile, error) {
	return s.profileWithName(ctx, proof, name)
}

func (s *PGStore) profileWithName(ctx context.Context, proof domain.SessionProof, name string) (domain.AccountProfile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	row, err := lockProfileAccess(ctx, q, proof, false)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	if name != "" {
		row.DisplayName = pgtext(name)
		if err := q.UpdateDisplayName(ctx, account.UpdateDisplayNameParams{ID: row.ID, DisplayName: row.DisplayName}); err != nil {
			return domain.AccountProfile{}, err
		}
	}
	return finishProfile(ctx, tx, q, row)
}

func (s *PGStore) AttachRecoveryEmail(ctx context.Context, proof domain.SessionProof, email application.EmailProof, hasher domain.CodeHasher, auditID string) (domain.AccountProfile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	if err := lockRecoveryAddress(ctx, tx, email.Address.Hash); err != nil {
		return domain.AccountProfile{}, err
	}
	row, err := lockProfileAccess(ctx, q, proof, true)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	if _, err := consumeAddressCode(ctx, q, email.Address.Hash, email.Code, hasher, email.Now, false); err != nil {
		return domain.AccountProfile{}, commitCodeVerdict(ctx, tx, err)
	}
	if owner, err := q.FindAccountByAddress(ctx, email.Address.Hash); err == nil {
		if owner.ID != row.ID {
			return domain.AccountProfile{}, commitCodeVerdict(ctx, tx, domain.ErrAddressLinked)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.AccountProfile{}, err
	}

	action := domain.RecoveryEmailAdd
	if _, err := q.ReadRecoveryAddress(ctx, row.ID); err == nil {
		action = domain.RecoveryEmailChange
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.AccountProfile{}, err
	}
	if err := q.DeleteAccountAddresses(ctx, row.ID); err != nil {
		return domain.AccountProfile{}, err
	}
	if err := insertRecoveryAddress(ctx, q, row.ID, email.Address, proof.Now); err != nil {
		return domain.AccountProfile{}, err
	}
	if err := writeRecoveryAudit(ctx, q, row.ID, auditID, action, proof.Now); err != nil {
		return domain.AccountProfile{}, err
	}
	return finishProfile(ctx, tx, q, row)
}

func pgtext(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func insertRecoveryAddress(ctx context.Context, q *account.Queries, id pgtype.UUID, addr domain.RecoveryAddress, now int64) error {
	n, err := q.InsertRecoveryAddress(ctx, account.InsertRecoveryAddressParams{
		AddressHash: addr.Hash, AccountID: id, EmailHint: pgtext(addr.Hint), LinkedAt: stamp(now),
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrAddressLinked
	}
	return nil
}

func writeRecoveryAudit(ctx context.Context, q *account.Queries, accountID pgtype.UUID, auditID, action string, now int64) error {
	id, err := mustUUID(auditID)
	if err != nil {
		return err
	}
	return q.InsertRecoveryAudit(ctx, account.InsertRecoveryAuditParams{ID: id, AccountID: accountID, Action: action, OccurredAt: stamp(now)})
}

func finishProfile(ctx context.Context, tx pgx.Tx, q *account.Queries, row account.Account) (domain.AccountProfile, error) {
	addr, err := q.ReadRecoveryAddress(ctx, row.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.AccountProfile{}, err
	}
	hasKey, err := q.HasAccountKey(ctx, row.ID)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	links, err := q.ListProviderLinks(ctx, row.ID)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	providers := make([]string, 0, len(links))
	for _, link := range links {
		providers = append(providers, link.Provider)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AccountProfile{}, err
	}
	return domain.AccountProfile{
		Account: toNamedAccount(row), RecoveryEmailHint: addr.EmailHint.String,
		HasAccountKey: hasKey, Providers: providers,
	}, nil
}

// PurgeRecoveryAudit deletes a bounded batch and reports the oldest
// remaining overdue event for the worker's existing retention sweep.
func (s *PGStore) PurgeRecoveryAudit(ctx context.Context, now time.Time, maxRows int32) (int64, time.Time, error) {
	q := account.New(s.pool)
	cutoff := stamp(now.Unix() - domain.RecoveryAuditTTLSeconds)
	n, err := q.PurgeRecoveryAudit(ctx, account.PurgeRecoveryAuditParams{Cutoff: cutoff, BatchSize: maxRows})
	if err != nil {
		return 0, time.Time{}, err
	}
	oldest, err := q.OldestRecoveryAudit(ctx, cutoff)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, time.Time{}, nil
	}
	return n, oldest.Time, err
}
