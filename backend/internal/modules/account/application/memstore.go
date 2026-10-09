package application

import (
	"context"
	"sync"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// MemStore is the mutex-guarded memory Store for unit tests and local-only
// runs. It honors the exact atomicity contract Postgres implements in
// P13-T02B: TryConsume and Rotate complete under one lock so the same
// interleavings are covered here first.
type MemStore struct {
	mu            sync.Mutex
	codes         []domain.EmailCode
	byAddr        map[string][]int
	accounts      map[string]domain.Account
	accountIDs    map[string]string
	accountsByID  map[string]domain.Account
	families      map[string]*domain.SessionFamily
	links         map[string]map[string]domain.ProviderLink
	bySubject     map[string]string
	nonces        map[string]bool
	bindings      map[string]map[string]domain.ContributorBinding
	byContributor map[string]string
	audit         map[string][]domain.BindingAudit
	keyCreds      map[string]domain.KeyCredential
	keyByUsername map[string]string
	keyByLookup   map[string]string
}

// NewMemStore returns an empty memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		byAddr:        map[string][]int{},
		accounts:      map[string]domain.Account{},
		accountIDs:    map[string]string{},
		accountsByID:  map[string]domain.Account{},
		families:      map[string]*domain.SessionFamily{},
		links:         map[string]map[string]domain.ProviderLink{},
		bySubject:     map[string]string{},
		nonces:        map[string]bool{},
		bindings:      map[string]map[string]domain.ContributorBinding{},
		byContributor: map[string]string{},
		audit:         map[string][]domain.BindingAudit{},
		keyCreds:      map[string]domain.KeyCredential{},
		keyByUsername: map[string]string{},
		keyByLookup:   map[string]string{},
	}
}

func (m *MemStore) SaveCode(_ context.Context, rec domain.EmailCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codes = append(m.codes, rec)
	m.byAddr[rec.AddressHash] = append(m.byAddr[rec.AddressHash], len(m.codes)-1)
	return nil
}

func (m *MemStore) CountCodesSince(_ context.Context, addressHash string, sinceUnix int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, i := range m.byAddr[addressHash] {
		if m.codes[i].IssuedAt >= sinceUnix {
			n++
		}
	}
	return n, nil
}

func (m *MemStore) LatestCode(_ context.Context, addressHash string) (domain.EmailCode, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.byAddr[addressHash]
	if len(idx) == 0 {
		return domain.EmailCode{}, false, nil
	}
	return m.codes[idx[len(idx)-1]], true, nil
}

func (m *MemStore) TryConsume(_ context.Context, addressHash, code string, hasher domain.CodeHasher, nowUnix int64) (domain.EmailCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.byAddr[addressHash]
	if len(idx) == 0 {
		return domain.EmailCode{}, domain.ErrCodeUnknown
	}
	// Match-first: a presented code decides its own row, so same-second
	// issuance never burns attempts on a sibling row.
	for _, i := range idx {
		rec := &m.codes[i]
		if !hasher.Equal(rec.Hash, rec.Salt, code) {
			continue
		}
		if rec.ConsumedAt != 0 {
			return domain.EmailCode{}, domain.ErrCodeConsumed
		}
		if domain.CodeExpired(rec.IssuedAt, nowUnix) {
			return domain.EmailCode{}, domain.ErrCodeExpired
		}
		if !domain.AttemptAllowed(rec.Attempts) {
			return domain.EmailCode{}, domain.ErrCodeAttemptsExhausted
		}
		rec.ConsumedAt = nowUnix
		return *rec, nil
	}
	// No match: wrong codes stay indistinguishable from unknown addresses.
	// States surface only on hash match (proof of possession). Attempts burn
	// on the newest live row when one exists.
	for i := len(idx) - 1; i >= 0; i-- {
		rec := &m.codes[idx[i]]
		if rec.ConsumedAt == 0 && !domain.CodeExpired(rec.IssuedAt, nowUnix) && domain.AttemptAllowed(rec.Attempts) {
			rec.Attempts++
			if !domain.AttemptAllowed(rec.Attempts) {
				return domain.EmailCode{}, domain.ErrCodeAttemptsExhausted
			}
			break
		}
	}
	return domain.EmailCode{}, domain.ErrCodeUnknown
}

func (m *MemStore) FindAccount(_ context.Context, addressHash string) (domain.Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accounts[addressHash]
	return acc, ok, nil
}

func (m *MemStore) CreateAccount(_ context.Context, acc domain.Account, addressHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.accounts[addressHash]; ok && existing.ID != acc.ID {
		return domain.ErrAddressLinked
	}
	m.accounts[addressHash] = acc
	m.accountIDs[acc.ID] = addressHash
	m.accountsByID[acc.ID] = acc
	return nil
}

func (m *MemStore) CreateFamily(_ context.Context, fam domain.SessionFamily) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := fam
	m.families[fam.ID] = &cp
	return nil
}

func (m *MemStore) Family(_ context.Context, familyID string) (domain.SessionFamily, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fam, ok := m.families[familyID]
	if !ok {
		return domain.SessionFamily{}, false, nil
	}
	return *fam, true, nil
}

func (m *MemStore) Rotate(_ context.Context, familyID, expectedRefreshHash, refreshSalt, refreshHash, accessSalt, accessHash string, accessExpires, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	fam, ok := m.families[familyID]
	if !ok {
		return domain.ErrCodeUnknown
	}
	if !domain.EqualHash(fam.RefreshHash, expectedRefreshHash) {
		return domain.ErrSessionReuse
	}
	fam.RefreshSalt, fam.RefreshHash = refreshSalt, refreshHash
	fam.AccessSalt, fam.AccessHash = accessSalt, accessHash
	fam.AccessExpires = accessExpires
	return nil
}

func (m *MemStore) RevokeFamily(_ context.Context, familyID string, nowUnix int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fam, ok := m.families[familyID]; ok {
		fam.RevokedAt = nowUnix
	}
	return nil
}

func (m *MemStore) RevokeAccount(_ context.Context, accountID string, nowUnix int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, fam := range m.families {
		if fam.AccountID == accountID {
			fam.RevokedAt = nowUnix
		}
	}
	return nil
}

func subjectKey(provider, subject string) string {
	return provider + "\x00" + subject
}

// LinkProvider binds a verified subject, refusing cross-account binds.
// Same-account relinks converge; same-account provider rotation updates
// the stored subject.
func (m *MemStore) LinkProvider(_ context.Context, link domain.ProviderLink) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !domain.ValidProvider(link.Provider) || link.Issuer == "" || link.Subject == "" {
		return domain.ErrOIDCWrongAudience
	}
	acc, ok := m.accountsByID[link.AccountID]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if err := accountUsable(acc); err != nil {
		return err
	}
	key := subjectKey(link.Provider, link.Subject)
	if owner, ok := m.bySubject[key]; ok && owner != link.AccountID {
		return domain.ErrLinkCrossAccount
	}
	if m.links[link.AccountID] == nil {
		m.links[link.AccountID] = map[string]domain.ProviderLink{}
	}
	if existing, ok := m.links[link.AccountID][link.Provider]; ok {
		if existing.Subject != link.Subject {
			delete(m.bySubject, subjectKey(link.Provider, existing.Subject))
		}
	}
	m.links[link.AccountID][link.Provider] = link
	m.bySubject[key] = link.AccountID
	return nil
}

// UnlinkProvider removes one binding, refusing the last login method.
func (m *MemStore) UnlinkProvider(_ context.Context, accountID, provider string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !domain.ValidProvider(provider) {
		return domain.ErrOIDCUnknownIssuer
	}
	acc, ok := m.accountsByID[accountID]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if err := accountUsable(acc); err != nil {
		return err
	}
	set := m.links[accountID]
	existing, ok := set[provider]
	if !ok {
		return domain.ErrProviderNotLinked
	}
	addresses := 0
	for _, acc := range m.accounts {
		if acc.ID == accountID {
			addresses++
		}
	}
	if len(set)+addresses <= 1 {
		return domain.ErrLastLoginMethod
	}
	delete(set, provider)
	delete(m.bySubject, subjectKey(provider, existing.Subject))
	return nil
}

// ListProviders returns the provider bindings of one account.
func (m *MemStore) ListProviders(_ context.Context, accountID string) ([]domain.ProviderLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.links[accountID]
	out := make([]domain.ProviderLink, 0, len(set))
	for _, l := range set {
		out = append(out, l)
	}
	return out, nil
}

// FindProviderOwner resolves the account owning a provider subject.
func (m *MemStore) FindProviderOwner(_ context.Context, provider, subject string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	owner, ok := m.bySubject[subjectKey(provider, subject)]
	return owner, ok, nil
}

// CountAddresses counts email bindings of one account.
func (m *MemStore) CountAddresses(_ context.Context, accountID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, acc := range m.accounts {
		if acc.ID == accountID {
			n++
		}
	}
	return n, nil
}

// TryConsumeNonce reserves a login nonce exactly once across callers.
func (m *MemStore) TryConsumeNonce(_ context.Context, nonce string, _ int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if nonce == "" {
		return false, nil
	}
	if m.nonces[nonce] {
		return false, nil
	}
	m.nonces[nonce] = true
	return true, nil
}

// GetAccount resolves an account by ID, including suspended/deleted rows.
func (m *MemStore) GetAccount(_ context.Context, accountID string) (domain.Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accountsByID[accountID]
	return acc, ok, nil
}

func (m *MemStore) setStatusLocked(accountID, status string) error {
	acc, ok := m.accountsByID[accountID]
	if !ok {
		return domain.ErrAccountNotFound
	}
	acc.Status = status
	m.accountsByID[accountID] = acc
	for hash, a := range m.accounts {
		if a.ID == accountID {
			a.Status = status
			m.accounts[hash] = a
		}
	}
	return nil
}

func (m *MemStore) revokeAccountLocked(accountID string, nowUnix int64) {
	for _, fam := range m.families {
		if fam.AccountID == accountID {
			fam.RevokedAt = nowUnix
		}
	}
}

// SuspendAccount marks the account suspended and revokes every session
// family immediately. New and rotated sessions fail closed afterwards.
func (m *MemStore) SuspendAccount(_ context.Context, accountID string, nowUnix int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.setStatusLocked(accountID, domain.StatusSuspended); err != nil {
		return err
	}
	m.revokeAccountLocked(accountID, nowUnix)
	return nil
}

// ReactivateAccount returns a suspended account to active. Revoked
// sessions stay revoked; the owner logs in again for fresh families.
func (m *MemStore) ReactivateAccount(_ context.Context, accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, ok := m.accountsByID[accountID]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if acc.Status == domain.StatusDeleted {
		return domain.ErrAccountDeleted
	}
	return m.setStatusLocked(accountID, domain.StatusActive)
}

// DeleteAccount marks the account deleted, revokes every session family
// and drops address and provider bindings. The account row stays as an
// audit record; a later signup with the same address mints a new account
// without reputation carryover.
func (m *MemStore) DeleteAccount(_ context.Context, accountID string, nowUnix int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.setStatusLocked(accountID, domain.StatusDeleted); err != nil {
		return err
	}
	m.revokeAccountLocked(accountID, nowUnix)
	for hash, a := range m.accounts {
		if a.ID == accountID {
			delete(m.accounts, hash)
		}
	}
	delete(m.accountIDs, accountID)
	if set, ok := m.links[accountID]; ok {
		for provider, l := range set {
			delete(m.bySubject, subjectKey(provider, l.Subject))
		}
		delete(m.links, accountID)
	}
	if set, ok := m.bindings[accountID]; ok {
		for contributor := range set {
			delete(m.byContributor, contributor)
		}
		delete(m.bindings, accountID)
	}
	return nil
}

// BindContributor records a verified contributor binding, refusing
// stolen-ID binds. Same-account relinks converge idempotently; every
// accepted bind appends an audit row.
func (m *MemStore) BindContributor(_ context.Context, binding domain.ContributorBinding, auditID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if binding.AccountID == "" || binding.ContributorID == "" || binding.KeyFingerprint == "" {
		return domain.ErrBindingInvalid
	}
	acc, ok := m.accountsByID[binding.AccountID]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if err := accountUsable(acc); err != nil {
		return err
	}
	if owner, ok := m.byContributor[binding.ContributorID]; ok && owner != binding.AccountID {
		return domain.ErrBindingCrossAccount
	}
	if m.bindings[binding.AccountID] == nil {
		m.bindings[binding.AccountID] = map[string]domain.ContributorBinding{}
	}
	m.bindings[binding.AccountID][binding.ContributorID] = binding
	m.byContributor[binding.ContributorID] = binding.AccountID
	m.audit[binding.AccountID] = append(m.audit[binding.AccountID], domain.BindingAudit{
		ID:            auditID,
		AccountID:     binding.AccountID,
		ContributorID: binding.ContributorID,
		Action:        domain.BindingActionBind,
		OccurredAt:    binding.BoundAt,
	})
	return nil
}

// UnbindContributor removes one binding, auditing the removal.
// Unknown bindings refuse without touching the ledger.
func (m *MemStore) UnbindContributor(_ context.Context, accountID, contributorID, auditID string, nowUnix int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if accountID == "" || contributorID == "" {
		return domain.ErrBindingInvalid
	}
	acc, ok := m.accountsByID[accountID]
	if !ok {
		return domain.ErrAccountNotFound
	}
	if err := accountUsable(acc); err != nil {
		return err
	}
	set := m.bindings[accountID]
	if _, ok := set[contributorID]; !ok {
		return domain.ErrBindingNotFound
	}
	delete(set, contributorID)
	delete(m.byContributor, contributorID)
	m.audit[accountID] = append(m.audit[accountID], domain.BindingAudit{
		ID:            auditID,
		AccountID:     accountID,
		ContributorID: contributorID,
		Action:        domain.BindingActionUnbind,
		OccurredAt:    nowUnix,
	})
	return nil
}

// ListBindings returns the contributor bindings of one account.
func (m *MemStore) ListBindings(_ context.Context, accountID string) ([]domain.ContributorBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.bindings[accountID]
	out := make([]domain.ContributorBinding, 0, len(set))
	for _, b := range set {
		out = append(out, b)
	}
	return out, nil
}

// FindBindingOwner resolves the account owning a contributor binding.
func (m *MemStore) FindBindingOwner(_ context.Context, contributorID string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	owner, ok := m.byContributor[contributorID]
	return owner, ok, nil
}

// ListBindingAudit returns the recovery audit trail of one account.
func (m *MemStore) ListBindingAudit(_ context.Context, accountID string) ([]domain.BindingAudit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.BindingAudit(nil), m.audit[accountID]...), nil
}

// CreateKeyAccount stores the account and its key credential under one
// lock. Username races refuse with ErrUsernameTaken, key races with
// ErrKeyCollision so the caller mints a fresh key and retries.
func (m *MemStore) CreateKeyAccount(_ context.Context, acc domain.Account, cred domain.KeyCredential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if owner, ok := m.keyByUsername[cred.UsernameHash]; ok && owner != acc.ID {
		return domain.ErrUsernameTaken
	}
	if owner, ok := m.keyByLookup[cred.KeyLookup]; ok && owner != acc.ID {
		return domain.ErrKeyCollision
	}
	m.accountsByID[acc.ID] = acc
	m.keyCreds[acc.ID] = cred
	m.keyByUsername[cred.UsernameHash] = acc.ID
	m.keyByLookup[cred.KeyLookup] = acc.ID
	return nil
}

// FindKeyCredentialByUsername returns the credential for a username hash,
// or false when no account uses it.
func (m *MemStore) FindKeyCredentialByUsername(_ context.Context, usernameHash string) (domain.KeyCredential, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.keyByUsername[usernameHash]
	if !ok {
		return domain.KeyCredential{}, false, nil
	}
	return m.keyCreds[id], true, nil
}

// FindKeyCredentialByLookup returns the credential for a key lookup, or
// false when no account holds it.
func (m *MemStore) FindKeyCredentialByLookup(_ context.Context, lookup string) (domain.KeyCredential, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.keyByLookup[lookup]
	if !ok {
		return domain.KeyCredential{}, false, nil
	}
	return m.keyCreds[id], true, nil
}
