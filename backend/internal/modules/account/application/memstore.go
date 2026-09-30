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
	mu         sync.Mutex
	codes      []domain.EmailCode
	byAddr     map[string][]int
	accounts   map[string]domain.Account
	accountIDs map[string]string
	families   map[string]*domain.SessionFamily
}

// NewMemStore returns an empty memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		byAddr:     map[string][]int{},
		accounts:   map[string]domain.Account{},
		accountIDs: map[string]string{},
		families:   map[string]*domain.SessionFamily{},
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
	// No match: the newest row decides the failure mode.
	rec := &m.codes[idx[len(idx)-1]]
	if rec.ConsumedAt != 0 {
		return domain.EmailCode{}, domain.ErrCodeConsumed
	}
	if domain.CodeExpired(rec.IssuedAt, nowUnix) {
		return domain.EmailCode{}, domain.ErrCodeExpired
	}
	if !domain.AttemptAllowed(rec.Attempts) {
		return domain.EmailCode{}, domain.ErrCodeAttemptsExhausted
	}
	rec.Attempts++
	if !domain.AttemptAllowed(rec.Attempts) {
		return domain.EmailCode{}, domain.ErrCodeAttemptsExhausted
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
