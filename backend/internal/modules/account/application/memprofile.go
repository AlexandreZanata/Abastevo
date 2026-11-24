package application

import (
	"context"
	"sort"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func (m *MemStore) SignUpWithCode(_ context.Context, proof EmailProof, hasher domain.CodeHasher, acc domain.Account, cred domain.KeyCredential, fam domain.SessionFamily) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	code, err := m.consumeCodeLocked(proof.Address.Hash, proof.Code, hasher, proof.Now, false)
	if err != nil {
		return err
	}
	if _, exists := m.accounts[proof.Address.Hash]; exists {
		return domain.ErrAddressLinked
	}
	if err := m.createKeyAccountLocked(acc, cred, proof.Now); err != nil {
		m.restoreCodeLocked(code)
		return err
	}
	m.bindRecoveryAddressLocked(acc, proof.Address)
	m.families[fam.ID] = &fam
	return nil
}

func (m *MemStore) RecoverWithCode(_ context.Context, proof EmailProof, hasher domain.CodeHasher, cred domain.KeyCredential, fam domain.SessionFamily, _ string) (domain.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	acc, found := m.accounts[proof.Address.Hash]
	if found && !acc.Active() {
		if _, err := m.verifyCodeLocked(proof.Address.Hash, proof.Code, hasher, proof.Now, false); err != nil {
			return domain.Account{}, err
		}
		return domain.Account{}, accountUsable(acc)
	}
	code, err := m.consumeCodeLocked(proof.Address.Hash, proof.Code, hasher, proof.Now, false)
	if err != nil {
		return domain.Account{}, err
	}
	if !found {
		return domain.Account{}, domain.ErrRecoveryNoAccount
	}
	if _, exists := m.keyByLookup[cred.KeyLookup]; exists {
		m.restoreCodeLocked(code)
		return domain.Account{}, domain.ErrKeyCollision
	}
	cred.AccountID = acc.ID
	if old, ok := m.keyCreds[acc.ID]; ok {
		cred.UsernameHash = old.UsernameHash
		delete(m.keyByLookup, old.KeyLookup)
	} else {
		cred.UsernameHash = domain.UsernameHash("account:" + acc.ID)
	}
	m.keyCreds[acc.ID] = cred
	m.keyByLookup[cred.KeyLookup] = acc.ID
	m.keyByUsername[cred.UsernameHash] = acc.ID
	m.revokeAccountLocked(acc.ID, proof.Now)
	fam.AccountID = acc.ID
	m.families[fam.ID] = &fam
	return acc, nil
}

func (m *MemStore) TryCreateKeyFamily(_ context.Context, fam domain.SessionFamily, verified domain.KeyCredential) (domain.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cred, ok := m.keyCreds[fam.AccountID]; !ok || cred != verified {
		return domain.Account{}, domain.ErrKeyInvalid
	}
	acc, exists := m.accountsByID[fam.AccountID]
	if !exists {
		return domain.Account{}, domain.ErrKeyInvalid
	}
	if err := accountUsable(acc); err != nil {
		return domain.Account{}, err
	}
	if err := m.admitFamilyLocked(fam, fam.IssuedAt); err != nil {
		return domain.Account{}, err
	}
	return acc, nil
}

func (m *MemStore) restoreCodeLocked(code domain.EmailCode) {
	for _, i := range m.byAddr[code.AddressHash] {
		if m.codes[i].ID == code.ID {
			m.codes[i].ConsumedAt = 0
			return
		}
	}
}

func (m *MemStore) bindRecoveryAddressLocked(acc domain.Account, address domain.RecoveryAddress) {
	if old := m.accountIDs[acc.ID]; old != "" {
		delete(m.accounts, old)
	}
	m.accounts[address.Hash] = acc
	m.accountIDs[acc.ID] = address.Hash
	m.recoveryHints[acc.ID] = address.Hint
}

func (m *MemStore) verifyProfileAccessLocked(proof domain.SessionProof, fresh bool) (domain.Account, error) {
	acc, exists := m.accountsByID[proof.AccountID]
	if !exists {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	if err := accountUsable(acc); err != nil {
		return domain.Account{}, err
	}
	fam, exists := m.families[proof.FamilyID]
	if !exists {
		return domain.Account{}, domain.ErrCodeUnknown
	}
	return acc, proof.Verify(*fam, fresh)
}

func (m *MemStore) profileLocked(acc domain.Account) domain.AccountProfile {
	_, hasKey := m.keyCreds[acc.ID]
	providers := make([]string, 0, len(m.links[acc.ID]))
	for provider := range m.links[acc.ID] {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return domain.AccountProfile{Account: acc, RecoveryEmailHint: m.recoveryHints[acc.ID], HasAccountKey: hasKey, Providers: providers}
}

func (m *MemStore) ReadProfile(_ context.Context, proof domain.SessionProof) (domain.AccountProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, err := m.verifyProfileAccessLocked(proof, false)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	return m.profileLocked(acc), nil
}

func (m *MemStore) UpdateProfileName(_ context.Context, proof domain.SessionProof, name string) (domain.AccountProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, err := m.verifyProfileAccessLocked(proof, false)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	acc.DisplayName = name
	m.accountsByID[proof.AccountID] = acc
	if hash := m.accountIDs[acc.ID]; hash != "" {
		m.accounts[hash] = acc
	}
	return m.profileLocked(acc), nil
}

func (m *MemStore) AttachRecoveryEmail(_ context.Context, proof domain.SessionProof, email EmailProof, hasher domain.CodeHasher, _ string) (domain.AccountProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, err := m.verifyProfileAccessLocked(proof, true)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	if _, err := m.consumeCodeLocked(email.Address.Hash, email.Code, hasher, email.Now, false); err != nil {
		return domain.AccountProfile{}, err
	}
	if other, found := m.accounts[email.Address.Hash]; found && other.ID != acc.ID {
		return domain.AccountProfile{}, domain.ErrAddressLinked
	}
	m.bindRecoveryAddressLocked(acc, email.Address)
	return m.profileLocked(acc), nil
}
