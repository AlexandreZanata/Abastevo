package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// Store is the persistence port behind every account use case. Atomicity
// lives here: TryConsume checks and marks a code in one step, Rotate swaps
// a family's tokens in one step, so concurrent retries cannot double-spend
// a code or fork a refresh chain. The memory implementation guards with a
// mutex; the Postgres one uses single-statement conditional updates.
type Store interface {
	SaveCode(ctx context.Context, rec domain.EmailCode) error
	CountCodesSince(ctx context.Context, addressHash string, sinceUnix int64) (int, error)
	LatestCode(ctx context.Context, addressHash string) (domain.EmailCode, bool, error)
	TryConsume(ctx context.Context, addressHash, candidateHash string, nowUnix int64) (domain.EmailCode, error)
	FindAccount(ctx context.Context, addressHash string) (domain.Account, bool, error)
	CreateAccount(ctx context.Context, acc domain.Account, addressHash string) error
	CreateFamily(ctx context.Context, fam domain.SessionFamily) error
	Family(ctx context.Context, familyID string) (domain.SessionFamily, bool, error)
	Rotate(ctx context.Context, familyID, refreshSalt, refreshHash, accessSalt, accessHash string, accessExpires, nowUnix int64) error
	RevokeFamily(ctx context.Context, familyID string, nowUnix int64) error
	RevokeAccount(ctx context.Context, accountID string, nowUnix int64) error
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
// first login. Wrong codes and unknown addresses share ErrCodeUnknown.
func (s *Service) ConsumeCode(ctx context.Context, address, code string) (AuthResult, error) {
	now := s.Clock.NowUnix()
	hash := domain.AddressHash(address)
	latest, found, err := s.Store.LatestCode(ctx, hash)
	if err != nil {
		return AuthResult{}, err
	}
	var candidateHash string
	if found {
		candidateHash = s.Hasher.Hash(latest.Salt, code)
	}
	if _, err := s.Store.TryConsume(ctx, hash, candidateHash, now); err != nil {
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
			return AuthResult{}, err
		}
		created = true
	}
	sess, err := s.newSession(ctx, acc.ID, now)
	if err != nil {
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
	return s.rotate(ctx, fam, now)
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
	if err := s.Store.Rotate(ctx, fam.ID, refreshSalt, s.Hasher.Hash(refreshSalt, refresh),
		accessSalt, s.Hasher.Hash(accessSalt, access), accessExpires, now); err != nil {
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
