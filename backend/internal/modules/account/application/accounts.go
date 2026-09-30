package application

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// Store is the persistence port behind every account use case. Atomicity
// lives here: TryConsume checks and marks a code in one step, Rotate swaps
// a family's tokens in one step, so concurrent retries cannot double-spend
// a code or fork a refresh chain. The memory implementation guards with a
// mutex; the Postgres one uses single-statement conditional updates.
// Provider links (P13-T03B) share the same contract: LinkProvider refuses
// cross-account binds, UnlinkProvider refuses the last login method.
type Store interface {
	SaveCode(ctx context.Context, rec domain.EmailCode) error
	CountCodesSince(ctx context.Context, addressHash string, sinceUnix int64) (int, error)
	LatestCode(ctx context.Context, addressHash string) (domain.EmailCode, bool, error)
	TryConsume(ctx context.Context, addressHash, code string, hasher domain.CodeHasher, nowUnix int64) (domain.EmailCode, error)
	FindAccount(ctx context.Context, addressHash string) (domain.Account, bool, error)
	CreateAccount(ctx context.Context, acc domain.Account, addressHash string) error
	CreateFamily(ctx context.Context, fam domain.SessionFamily) error
	Family(ctx context.Context, familyID string) (domain.SessionFamily, bool, error)
	Rotate(ctx context.Context, familyID, expectedRefreshHash, refreshSalt, refreshHash, accessSalt, accessHash string, accessExpires, nowUnix int64) error
	RevokeFamily(ctx context.Context, familyID string, nowUnix int64) error
	RevokeAccount(ctx context.Context, accountID string, nowUnix int64) error
	GetAccount(ctx context.Context, accountID string) (domain.Account, bool, error)
	SuspendAccount(ctx context.Context, accountID string, nowUnix int64) error
	ReactivateAccount(ctx context.Context, accountID string) error
	DeleteAccount(ctx context.Context, accountID string, nowUnix int64) error
	LinkProvider(ctx context.Context, link domain.ProviderLink) error
	UnlinkProvider(ctx context.Context, accountID, provider string) error
	ListProviders(ctx context.Context, accountID string) ([]domain.ProviderLink, error)
	FindProviderOwner(ctx context.Context, provider, subject string) (string, bool, error)
	CountAddresses(ctx context.Context, accountID string) (int, error)
	TryConsumeNonce(ctx context.Context, nonce string, nowUnix int64) (bool, error)
}

// Session is one issued login: opaque bearer tokens plus their lifetimes.
// Tokens are shown once here; only their salted hashes persist.
type Session struct {
	FamilyID        string
	AccountID       string
	AccessToken     string
	RefreshToken    string
	AccessExpires   int64
	AbsoluteExpires int64
}

// AuthResult is the outcome of a successful code consume.
type AuthResult struct {
	Account domain.Account
	Session Session
	Created bool
}

// Service wires frozen policy to the Store and MailSender ports.
type Service struct {
	Clock    domain.Clock
	Hasher   domain.CodeHasher
	Mail     domain.MailSender
	Store    Store
	Verifier domain.ProviderVerifier
	CodeGen  func() (string, error)
	TokenGen func() (string, error)
	AliasGen func() (string, error)
	IDGen    func() (string, error)
}

// RequestCode issues a code when quota and cooldown allow. Unknown
// addresses, cooldown hits and quota exhaustion all answer nil: the caller
// cannot distinguish them, so no oracle leaks registration or limits.
func (s *Service) RequestCode(ctx context.Context, address string) error {
	now := s.Clock.NowUnix()
	hash := domain.AddressHash(address)
	// Suspended and deleted accounts (deleted rows are unreachable by
	// address, but the guard is explicit) receive no code and no mail.
	// The answer stays nil, identical to quota/cooldown/unknown, so no
	// oracle leaks status.
	if acc, found, err := s.Store.FindAccount(ctx, hash); err != nil {
		return err
	} else if found {
		if err := accountUsable(acc); err != nil {
			return nil
		}
	}
	count, err := s.Store.CountCodesSince(ctx, hash, now-3600)
	if err != nil {
		return err
	}
	if count >= domain.MaxCodesPerAddressPerHour {
		return nil
	}
	if latest, found, err := s.Store.LatestCode(ctx, hash); err != nil {
		return err
	} else if found && latest.ConsumedAt == 0 && !domain.CodeExpired(latest.IssuedAt, now) &&
		!domain.ResendAllowed(latest.IssuedAt, now) {
		return nil
	}
	code, err := s.CodeGen()
	if err != nil {
		return err
	}
	salt, err := s.Hasher.NewSalt()
	if err != nil {
		return err
	}
	id, err := s.IDGen()
	if err != nil {
		return err
	}
	rec := domain.EmailCode{
		ID:          id,
		AddressHash: hash,
		Salt:        salt,
		Hash:        s.Hasher.Hash(salt, code),
		IssuedAt:    now,
	}
	if err := s.Store.SaveCode(ctx, rec); err != nil {
		return err
	}
	if err := s.Mail.SendCode(ctx, address, code); err != nil {
		return err
	}
	return nil
}

// ConsumeCode redeems one code for a session, creating the FREE account on
// first login. The store matches by hash across the address rows, so same-
// second issuance never misattributes a guess; wrong codes and unknown
// addresses share ErrCodeUnknown.
func (s *Service) ConsumeCode(ctx context.Context, address, code string) (AuthResult, error) {
	now := s.Clock.NowUnix()
	hash := domain.AddressHash(address)
	if acc, found, err := s.Store.FindAccount(ctx, hash); err != nil {
		return AuthResult{}, err
	} else if found {
		// Pre-check refuses suspended accounts without burning the
		// presented code. Deleted addresses are already unlinked, so a
		// later signup with the same address mints a new account.
		if err := accountUsable(acc); err != nil {
			return AuthResult{}, err
		}
	}
	if _, err := s.Store.TryConsume(ctx, hash, code, s.Hasher, now); err != nil {
		return AuthResult{}, err
	}
	acc, found, err := s.Store.FindAccount(ctx, hash)
	if err != nil {
		return AuthResult{}, err
	}
	created := false
	if !found {
		alias, err := s.AliasGen()
		if err != nil {
			return AuthResult{}, err
		}
		id, err := s.IDGen()
		if err != nil {
			return AuthResult{}, err
		}
		acc = domain.Account{ID: id, Alias: alias, Status: domain.StatusActive, CreatedAt: now}
		if err := s.Store.CreateAccount(ctx, acc, hash); err != nil {
			if errors.Is(err, domain.ErrAddressLinked) {
				// Lost race with a parallel signup: log into the winner.
				acc, found, err = s.Store.FindAccount(ctx, hash)
				if err != nil || !found {
					return AuthResult{}, domain.ErrAddressLinked
				}
			} else {
				return AuthResult{}, err
			}
		} else {
			created = true
		}
	}
	sess, err := s.newSession(ctx, acc.ID, now)
	if err != nil {
		return AuthResult{}, err
	}
	// Race guard: a suspension or deletion interleaved with issuance must
	// not leave a live session behind. The fresh family is revoked before
	// refusing, so the outcome is identical to a pre-issue suspension.
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
	return AuthResult{Account: acc, Session: sess, Created: created}, nil
}

// Refresh rotates a family's tokens. A superseded refresh token revokes the
// whole family before failing: token theft cannot fork a parallel chain.
func (s *Service) Refresh(ctx context.Context, familyID, refreshToken string) (Session, error) {
	now := s.Clock.NowUnix()
	fam, found, err := s.Store.Family(ctx, familyID)
	if err != nil {
		return Session{}, err
	}
	if !found {
		return Session{}, domain.ErrCodeUnknown
	}
	// Status leads: suspended and deleted accounts refuse before token
	// checks, so moderation/erasure takes effect with stable verdicts.
	acc, found, err := s.Store.GetAccount(ctx, fam.AccountID)
	if err != nil {
		return Session{}, err
	}
	if !found {
		return Session{}, domain.ErrAccountNotFound
	}
	if err := accountUsable(acc); err != nil {
		return Session{}, err
	}
	if fam.RevokedAt != 0 {
		return Session{}, domain.ErrSessionRevoked
	}
	if domain.RefreshExpired(fam.IssuedAt, now) {
		return Session{}, domain.ErrSessionExpired
	}
	if !s.Hasher.Equal(fam.RefreshHash, fam.RefreshSalt, refreshToken) {
		if err := s.Store.RevokeFamily(ctx, familyID, now); err != nil {
			return Session{}, err
		}
		return Session{}, domain.ErrSessionReuse
	}
	sess, err := s.rotate(ctx, fam, now)
	if err != nil {
		return Session{}, err
	}
	// Race guard: a suspension or deletion interleaved with rotation must
	// not leave fresh tokens behind. The rotated family is revoked
	// before refusing.
	if live, found, err := s.Store.GetAccount(ctx, fam.AccountID); err != nil {
		_ = s.Store.RevokeFamily(ctx, fam.ID, now)
		return Session{}, err
	} else if !found {
		_ = s.Store.RevokeFamily(ctx, fam.ID, now)
		return Session{}, domain.ErrAccountNotFound
	} else if err := accountUsable(live); err != nil {
		_ = s.Store.RevokeFamily(ctx, fam.ID, now)
		return Session{}, err
	}
	return sess, nil
}

// ValidateAccess checks one access token for transport guards.
func (s *Service) ValidateAccess(ctx context.Context, familyID, accessToken string) (string, error) {
	now := s.Clock.NowUnix()
	fam, found, err := s.Store.Family(ctx, familyID)
	if err != nil {
		return "", err
	}
	if !found {
		return "", domain.ErrCodeUnknown
	}
	// Status leads here as well, covering every session-authenticated
	// transport (revoke, link, unlink, list, delete) in one place.
	acc, found, err := s.Store.GetAccount(ctx, fam.AccountID)
	if err != nil {
		return "", err
	}
	if !found {
		return "", domain.ErrAccountNotFound
	}
	if err := accountUsable(acc); err != nil {
		return "", err
	}
	if fam.RevokedAt != 0 {
		return "", domain.ErrSessionRevoked
	}
	if now > fam.AccessExpires {
		return "", domain.ErrSessionExpired
	}
	if !s.Hasher.Equal(fam.AccessHash, fam.AccessSalt, accessToken) {
		return "", domain.ErrCodeUnknown
	}
	return fam.AccountID, nil
}

// RevokeAll revokes every session family of an account (logout everywhere,
// device loss, provider change, deletion).
func (s *Service) RevokeAll(ctx context.Context, accountID string) error {
	return s.Store.RevokeAccount(ctx, accountID, s.Clock.NowUnix())
}

func (s *Service) newSession(ctx context.Context, accountID string, now int64) (Session, error) {
	familyID, err := s.IDGen()
	if err != nil {
		return Session{}, err
	}
	return s.storeSession(ctx, accountID, familyID, now)
}

func (s *Service) rotate(ctx context.Context, fam domain.SessionFamily, now int64) (Session, error) {
	access, refresh, accessSalt, refreshSalt, err := s.mintTokens()
	if err != nil {
		return Session{}, err
	}
	accessExpires := now + domain.SessionAccessTTLSeconds
	if err := s.Store.Rotate(ctx, fam.ID, fam.RefreshHash, refreshSalt, s.Hasher.Hash(refreshSalt, refresh),
		accessSalt, s.Hasher.Hash(accessSalt, access), accessExpires, now); err != nil {
		if errors.Is(err, domain.ErrSessionReuse) {
			// Lost a rotation race after the check: fail closed.
			if rerr := s.Store.RevokeFamily(ctx, fam.ID, now); rerr != nil {
				return Session{}, rerr
			}
		}
		return Session{}, err
	}
	return Session{
		FamilyID:        fam.ID,
		AccountID:       fam.AccountID,
		AccessToken:     access,
		RefreshToken:    refresh,
		AccessExpires:   accessExpires,
		AbsoluteExpires: fam.IssuedAt + int64(domain.RefreshAbsoluteLifetimeDays)*24*3600,
	}, nil
}

func (s *Service) storeSession(ctx context.Context, accountID, familyID string, now int64) (Session, error) {
	access, refresh, accessSalt, refreshSalt, err := s.mintTokens()
	if err != nil {
		return Session{}, err
	}
	accessExpires := now + domain.SessionAccessTTLSeconds
	fam := domain.SessionFamily{
		ID:            familyID,
		AccountID:     accountID,
		RefreshSalt:   refreshSalt,
		RefreshHash:   s.Hasher.Hash(refreshSalt, refresh),
		AccessSalt:    accessSalt,
		AccessHash:    s.Hasher.Hash(accessSalt, access),
		AccessExpires: accessExpires,
		IssuedAt:      now,
	}
	if err := s.Store.CreateFamily(ctx, fam); err != nil {
		return Session{}, err
	}
	return Session{
		FamilyID:        familyID,
		AccountID:       accountID,
		AccessToken:     access,
		RefreshToken:    refresh,
		AccessExpires:   accessExpires,
		AbsoluteExpires: now + int64(domain.RefreshAbsoluteLifetimeDays)*24*3600,
	}, nil
}

func (s *Service) mintTokens() (access, refresh, accessSalt, refreshSalt string, err error) {
	if access, err = s.TokenGen(); err != nil {
		return "", "", "", "", err
	}
	if refresh, err = s.TokenGen(); err != nil {
		return "", "", "", "", err
	}
	if accessSalt, err = s.Hasher.NewSalt(); err != nil {
		return "", "", "", "", err
	}
	if refreshSalt, err = s.Hasher.NewSalt(); err != nil {
		return "", "", "", "", err
	}
	return access, refresh, accessSalt, refreshSalt, nil
}
