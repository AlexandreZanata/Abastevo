// Package application exchanges current account proof for an independent
// company cookie. Organization authority is always resolved separately.
package application

import (
	"context"
	"errors"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"log/slog"
	"time"
)

type Proof struct {
	AccountID, FamilyID, AccessHash                   string
	AuthenticatedAt, AccessExpiresAt, FamilyExpiresAt time.Time
}
type Row struct {
	ID, AccountID, FamilyID, Lookup, TokenSalt, TokenHash, CSRFSalt, CSRFHash string
	AuthenticatedAt, CreatedAt, LastSeenAt, ExpiresAt                         time.Time
}

func (Row) String() string         { return "companysession: verifier record redacted" }
func (Row) GoString() string       { return "companysession: verifier record redacted" }
func (Row) LogValue() slog.Value   { return slog.StringValue("companysession: verifier record redacted") }
func (Proof) String() string       { return "companysession: account proof redacted" }
func (Proof) GoString() string     { return "companysession: account proof redacted" }
func (Proof) LogValue() slog.Value { return slog.StringValue("companysession: account proof redacted") }

type Result struct {
	Token, CSRF string
	Row         Row
}

func (Result) String() string       { return "companysession: credentials redacted" }
func (Result) GoString() string     { return "companysession: credentials redacted" }
func (Result) LogValue() slog.Value { return slog.StringValue("companysession: credentials redacted") }

type Store interface {
	Create(context.Context, Row, Proof, string, time.Time) error
	Authenticate(context.Context, string, time.Time, bool, func(Row) error) (Row, error)
	Revoke(context.Context, string, string, time.Time) error
}
type Service struct {
	Enabled   bool
	Store     Store
	Hasher    account.CodeHasher
	Prove     func(context.Context, string, string) (Proof, error)
	Now       func() time.Time
	NewID     func() (string, error)
	NewSecret func() (string, error)
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) lookup(raw string) string { return s.Hasher.Hash("company.session.lookup.v1", raw) }
func (s *Service) ready() error {
	if s == nil || !s.Enabled {
		return domain.ErrDisabled
	}
	if s.Store == nil || s.Hasher == nil {
		return domain.ErrUnavailable
	}
	return nil
}
func normalized(err error) error {
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		return domain.ErrUnauthorized
	case errors.Is(err, domain.ErrCSRF):
		return domain.ErrCSRF
	case errors.Is(err, domain.ErrFresh):
		return domain.ErrFresh
	case errors.Is(err, domain.ErrRateLimited):
		return domain.ErrRateLimited
	default:
		return domain.ErrUnavailable
	}
}
func (s *Service) Login(ctx context.Context, family, access, oldCookie string) (Result, error) {
	if err := s.ready(); err != nil {
		return Result{}, err
	}
	if s.Prove == nil || s.NewID == nil || s.NewSecret == nil {
		return Result{}, domain.ErrUnavailable
	}
	if !domain.ValidID(family) || len(access) == 0 || len(access) > 1024 {
		return Result{}, domain.ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	proof, err := s.Prove(ctx, family, access)
	if err != nil {
		return Result{}, normalized(err)
	}
	now := s.now()
	if !domain.ValidID(proof.AccountID) || proof.FamilyID != family || !domain.ValidHash(proof.AccessHash) ||
		!proof.AccessExpiresAt.Equal(proof.AuthenticatedAt.Add(domain.FreshWindow)) {
		return Result{}, domain.ErrUnavailable
	}
	if !domain.Fresh(proof.AuthenticatedAt, now) || !now.Before(proof.FamilyExpiresAt) {
		return Result{}, domain.ErrFresh
	}
	id, err := s.NewID()
	if err != nil || !domain.ValidID(id) {
		return Result{}, domain.ErrUnavailable
	}
	token, err := s.NewSecret()
	if err != nil || !domain.ValidHash(token) {
		return Result{}, domain.ErrUnavailable
	}
	csrf, err := s.NewSecret()
	if err != nil || !domain.ValidHash(csrf) {
		return Result{}, domain.ErrUnavailable
	}
	token = domain.TokenPrefix + token
	csrf = domain.CSRFPrefix + csrf
	tokenSalt, err := s.Hasher.NewSalt()
	if err != nil {
		return Result{}, domain.ErrUnavailable
	}
	csrfSalt, err := s.Hasher.NewSalt()
	if err != nil {
		return Result{}, domain.ErrUnavailable
	}
	expires := now.Add(domain.AbsoluteLifetime)
	if proof.FamilyExpiresAt.Before(expires) {
		expires = proof.FamilyExpiresAt
	}
	row := Row{ID: id, AccountID: proof.AccountID, FamilyID: family, Lookup: s.lookup(token),
		TokenSalt: tokenSalt, TokenHash: s.Hasher.Hash(tokenSalt, "company.cookie.v1\x00"+token),
		CSRFSalt: csrfSalt, CSRFHash: s.Hasher.Hash(csrfSalt, "company.csrf.v1\x00"+csrf),
		AuthenticatedAt: proof.AuthenticatedAt, CreatedAt: now, LastSeenAt: now, ExpiresAt: expires}
	oldLookup := ""
	if domain.ValidSecret(oldCookie, domain.TokenPrefix) {
		oldLookup = s.lookup(oldCookie)
	}
	if err := s.Store.Create(ctx, row, proof, oldLookup, now); err != nil {
		return Result{}, normalized(err)
	}
	if ctx.Err() != nil {
		return Result{}, domain.ErrUnavailable
	}
	return Result{Token: token, CSRF: csrf, Row: row}, nil
}
func (s *Service) Authenticate(ctx context.Context, token, csrf string, fresh bool) (Row, error) {
	return s.authenticate(ctx, token, csrf, fresh, true)
}

// Peek verifies credentials without extending idle time. Organization entry
// confirms only after current membership/permission resolution.
func (s *Service) Peek(ctx context.Context, token, csrf string) (Row, error) {
	return s.authenticate(ctx, token, csrf, false, false)
}
func (s *Service) authenticate(ctx context.Context, token, csrf string, fresh, touch bool) (Row, error) {
	if err := s.ready(); err != nil {
		return Row{}, err
	}
	if !domain.ValidSecret(token, domain.TokenPrefix) {
		return Row{}, domain.ErrUnauthorized
	}
	if !domain.ValidSecret(csrf, domain.CSRFPrefix) {
		return Row{}, domain.ErrCSRF
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	now := s.now()
	row, err := s.Store.Authenticate(ctx, s.lookup(token), now, touch, func(row Row) error {
		if !domain.Live(row.CreatedAt, row.LastSeenAt, row.ExpiresAt, now) {
			return domain.ErrUnauthorized
		}
		if !s.Hasher.Equal(row.TokenHash, row.TokenSalt, "company.cookie.v1\x00"+token) {
			return domain.ErrUnauthorized
		}
		if !s.Hasher.Equal(row.CSRFHash, row.CSRFSalt, "company.csrf.v1\x00"+csrf) {
			return domain.ErrCSRF
		}
		if fresh && !domain.Fresh(row.AuthenticatedAt, now) {
			return domain.ErrFresh
		}
		return nil
	})
	if err != nil {
		return Row{}, normalized(err)
	}
	if ctx.Err() != nil {
		return Row{}, domain.ErrUnavailable
	}
	return row, nil
}
func (s *Service) Logout(ctx context.Context, token, csrf string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	row, err := s.Authenticate(ctx, token, csrf, false)
	if err != nil {
		return err
	}
	if err := s.Store.Revoke(ctx, row.ID, row.Lookup, s.now()); err != nil {
		return normalized(err)
	}
	return nil
}
