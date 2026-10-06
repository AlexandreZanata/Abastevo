package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// KeyAccount is the outcome of a key-account signup: the account plus the
// account key shown exactly once. Only the salted verifier persists.
type KeyAccount struct {
	Account domain.Account
	Key     string
}

// keyCreateAttempts bounds key-collision retries; 158-bit entropy makes
// even one retry near-impossible, three keeps failure explainable.
const keyCreateAttempts = 3

// CreateKeyAccount mints an anonymous account for username and returns the
// single-display account key. Usernames collide visibly (409) so the
// contributor can pick another; keys never collide observably.
func (s *Service) CreateKeyAccount(ctx context.Context, username string) (KeyAccount, error) {
	if !domain.ValidUsername(username) {
		return KeyAccount{}, domain.ErrUsernameInvalid
	}
	name := domain.NormalizeUsername(username)
	usernameHash := domain.UsernameHash(name)
	if _, found, err := s.Store.FindKeyCredentialByUsername(ctx, usernameHash); err != nil {
		return KeyAccount{}, err
	} else if found {
		return KeyAccount{}, domain.ErrUsernameTaken
	}
	now := s.Clock.NowUnix()
	id, err := s.IDGen()
	if err != nil {
		return KeyAccount{}, err
	}
	acc := domain.Account{ID: id, Alias: name, Status: domain.StatusActive, CreatedAt: now}
	for attempt := 0; attempt < keyCreateAttempts; attempt++ {
		raw, err := s.KeyGen()
		if err != nil {
			return KeyAccount{}, err
		}
		canonical, err := domain.CanonicalKey(raw)
		if err != nil {
			return KeyAccount{}, err
		}
		salt, err := s.Hasher.NewSalt()
		if err != nil {
			return KeyAccount{}, err
		}
		cred := domain.KeyCredential{
			AccountID:    id,
			UsernameHash: usernameHash,
			KeyLookup:    domain.KeyLookup(canonical),
			KeySalt:      salt,
			KeyHash:      s.Hasher.Hash(salt, canonical),
			CreatedAt:    now,
		}
		if err := s.Store.CreateKeyAccount(ctx, acc, cred); err != nil {
			if err == domain.ErrKeyCollision {
				continue
			}
			return KeyAccount{}, err
		}
		return KeyAccount{Account: acc, Key: canonical}, nil
	}
	return KeyAccount{}, domain.ErrKeyCollision
}

// LoginWithKey authenticates with the account key alone: the lookup finds
// the candidate row and the salted verifier confirms. Unknown keys, wrong
// keys and dead accounts share ErrKeyInvalid-shaped failures with no
// oracle; sessions reuse the standard rotating family machinery.
func (s *Service) LoginWithKey(ctx context.Context, rawKey string) (AuthResult, error) {
	canonical, err := domain.CanonicalKey(rawKey)
	if err != nil {
		return AuthResult{}, domain.ErrKeyInvalid
	}
	cred, found, err := s.Store.FindKeyCredentialByLookup(ctx, domain.KeyLookup(canonical))
	if err != nil {
		return AuthResult{}, err
	}
	if !found {
		return AuthResult{}, domain.ErrKeyInvalid
	}
	acc, found, err := s.Store.GetAccount(ctx, cred.AccountID)
	if err != nil {
		return AuthResult{}, err
	}
	if !found {
		return AuthResult{}, domain.ErrKeyInvalid
	}
	if err := accountUsable(acc); err != nil {
		return AuthResult{}, err
	}
	if !s.Hasher.Equal(cred.KeyHash, cred.KeySalt, canonical) {
		return AuthResult{}, domain.ErrKeyInvalid
	}
	now := s.Clock.NowUnix()
	sess, err := s.newSession(ctx, acc.ID, now)
	if err != nil {
		return AuthResult{}, err
	}
	// Race guard mirrors code consume: a suspension or deletion
	// interleaved with issuance must not leave a live session behind.
	if live, found, err := s.Store.GetAccount(ctx, acc.ID); err != nil {
		_ = s.Store.RevokeFamily(ctx, sess.FamilyID, now)
		return AuthResult{}, err
	} else if !found {
		_ = s.Store.RevokeFamily(ctx, sess.FamilyID, now)
		return AuthResult{}, domain.ErrAccountNotFound
	} else if err := accountUsable(live); err != nil {
		_ = s.Store.RevokeAccount(ctx, acc.ID, now)
		return AuthResult{}, err
	}
	return AuthResult{Account: acc, Session: sess}, nil
}
