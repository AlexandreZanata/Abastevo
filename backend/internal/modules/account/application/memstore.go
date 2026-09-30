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
	mu           sync.Mutex
	codes        []domain.EmailCode
	byAddr       map[string][]int
	accounts     map[string]domain.Account
	accountIDs   map[string]string
	accountsByID map[string]domain.Account
	families     map[string]*domain.SessionFamily
	links        map[string]map[string]domain.ProviderLink
	bySubject    map[string]string
	nonces       map[string]bool
}

// NewMemStore returns an empty memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		byAddr:       map[string][]int{},
		accounts:     map[string]domain.Account{},
		accountIDs:   map[string]string{},
		accountsByID: map[string]domain.Account{},
		families:     map[string]*domain.SessionFamily{},
		links:        map[string]map[string]domain.ProviderLink{},
		bySubject:    map[string]string{},
		nonces:       map[string]bool{},
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
	if _, ok := m.accountsByID[link.AccountID]; !ok {
		return domain.ErrAccountNotFound
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
	if _, ok := m.accountsByID[accountID]; !ok {
		return domain.ErrAccountNotFound
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
