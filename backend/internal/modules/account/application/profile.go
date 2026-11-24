package application

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// VerifiedKeyResult is returned only after the key and its session commit.
type VerifiedKeyResult struct {
	Account domain.Account
	Session Session
	Key     string
}

type EmailProof struct {
	Address domain.RecoveryAddress
	Code    string
	Now     int64
}

func emailProof(email, code string, now int64) (EmailProof, error) {
	if !domain.ValidEmail(email) {
		return EmailProof{}, domain.ErrEmailInvalid
	}
	return EmailProof{
		Address: domain.RecoveryAddress{Hash: domain.AddressHash(email), Hint: domain.EmailHint(email)},
		Code:    code,
		Now:     now,
	}, nil
}

// SignUp verifies address ownership and creates a key account with a public
// name. The OTP, account, credential and first session commit together.
func (s *Service) SignUp(ctx context.Context, name, email, code string) (VerifiedKeyResult, error) {
	if !domain.ValidPublicName(name) {
		return VerifiedKeyResult{}, domain.ErrNameInvalid
	}
	now := s.Clock.NowUnix()
	proof, err := emailProof(email, code, now)
	if err != nil {
		return VerifiedKeyResult{}, err
	}
	id, err := s.IDGen()
	if err != nil {
		return VerifiedKeyResult{}, err
	}
	alias, err := s.AliasGen()
	if err != nil {
		return VerifiedKeyResult{}, err
	}
	acc := domain.Account{
		ID: id, Alias: alias, DisplayName: domain.NormalizePublicName(name),
		Status: domain.StatusActive, CreatedAt: now,
	}
	return s.withVerifiedKey(now, func(cred domain.KeyCredential, fam domain.SessionFamily) (domain.Account, error) {
		cred.AccountID = id
		cred.UsernameHash = domain.UsernameHash("account:" + id)
		fam.AccountID = id
		return acc, s.Store.SignUpWithCode(ctx, proof, s.Hasher, acc, cred, fam)
	})
}

// RecoverKey verifies the recovery address, then replaces the key and all
// prior sessions in the same transaction that issues the new session.
func (s *Service) RecoverKey(ctx context.Context, email, code string) (VerifiedKeyResult, error) {
	now := s.Clock.NowUnix()
	proof, err := emailProof(email, code, now)
	if err != nil {
		return VerifiedKeyResult{}, err
	}
	auditID, err := s.IDGen()
	if err != nil {
		return VerifiedKeyResult{}, err
	}
	return s.withVerifiedKey(now, func(cred domain.KeyCredential, fam domain.SessionFamily) (domain.Account, error) {
		return s.Store.RecoverWithCode(ctx, proof, s.Hasher, cred, fam, auditID)
	})
}

func (s *Service) withVerifiedKey(now int64, persist func(domain.KeyCredential, domain.SessionFamily) (domain.Account, error)) (VerifiedKeyResult, error) {
	familyID, err := s.IDGen()
	if err != nil {
		return VerifiedKeyResult{}, err
	}
	fam, session, err := s.mintSession("", familyID, now)
	if err != nil {
		return VerifiedKeyResult{}, err
	}
	for range keyCreateAttempts {
		key, cred, err := s.mintKey(now)
		if err != nil {
			return VerifiedKeyResult{}, err
		}
		acc, err := persist(cred, fam)
		if errors.Is(err, domain.ErrKeyCollision) {
			continue
		}
		if err != nil {
			return VerifiedKeyResult{}, err
		}
		session.AccountID = acc.ID
		return VerifiedKeyResult{Account: acc, Session: session, Key: key}, nil
	}
	return VerifiedKeyResult{}, domain.ErrKeyCollision
}

func (s *Service) validateSessionProof(ctx context.Context, familyID, accessToken string, fresh bool) (domain.SessionProof, error) {
	now := s.Clock.NowUnix()
	fam, found, err := s.Store.Family(ctx, familyID)
	if err != nil {
		return domain.SessionProof{}, err
	}
	if !found {
		return domain.SessionProof{}, domain.ErrCodeUnknown
	}
	acc, found, err := s.Store.GetAccount(ctx, fam.AccountID)
	if err != nil {
		return domain.SessionProof{}, err
	}
	if !found {
		return domain.SessionProof{}, domain.ErrAccountNotFound
	}
	if err := accountUsable(acc); err != nil {
		return domain.SessionProof{}, err
	}
	proof := domain.SessionProof{
		FamilyID: familyID, AccountID: fam.AccountID, Now: now,
		AccessHash: s.Hasher.Hash(fam.AccessSalt, accessToken),
	}
	return proof, proof.Verify(fam, fresh)
}

func (s *Service) Profile(ctx context.Context, familyID, accessToken string) (domain.AccountProfile, error) {
	proof, err := s.validateSessionProof(ctx, familyID, accessToken, false)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	return s.Store.ReadProfile(ctx, proof)
}

func (s *Service) UpdateName(ctx context.Context, familyID, accessToken, name string) (domain.AccountProfile, error) {
	if !domain.ValidPublicName(name) {
		return domain.AccountProfile{}, domain.ErrNameInvalid
	}
	proof, err := s.validateSessionProof(ctx, familyID, accessToken, false)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	return s.Store.UpdateProfileName(ctx, proof, domain.NormalizePublicName(name))
}

func (s *Service) SetRecoveryEmail(ctx context.Context, familyID, accessToken, email, code string) (domain.AccountProfile, error) {
	emailCheck, err := emailProof(email, code, s.Clock.NowUnix())
	if err != nil {
		return domain.AccountProfile{}, err
	}
	proof, err := s.validateSessionProof(ctx, familyID, accessToken, true)
	if err != nil {
		return domain.AccountProfile{}, err
	}
	emailCheck.Now = proof.Now
	id, err := s.IDGen()
	if err != nil {
		return domain.AccountProfile{}, err
	}
	return s.Store.AttachRecoveryEmail(ctx, proof, emailCheck, s.Hasher, id)
}
