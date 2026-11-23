package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// ProviderLogin is one native sign-in attempt after transport decoding.
// Nonce is the RAW nonce; the identity token carries its
// domain.NativeNonceClaim. Neither the token nor the nonce is stored or
// logged.
type ProviderLogin struct {
	Provider string
	IDToken  string
	Nonce    string
}

// LoginWithProvider opens a FREE account session from a verified native
// provider identity token (P13-T05, B-BR-A01/A03). Order is fail-closed:
// availability (configured audience) before shape, shape before any
// key fetch, full verification (signature, issuer, the native audience,
// expiry, hashed nonce) before the single-use nonce is consumed, and
// account resolution only after the proof holds. The issuer plus
// subject is the only lookup key: an existing link signs in to its
// account (suspended and deleted accounts refuse with their verdicts);
// an unknown subject creates one FREE account bound to it. Equal email
// strings never select or merge an account.
func (s *Service) LoginWithProvider(ctx context.Context, req ProviderLogin) (AuthResult, error) {
	if !domain.ValidProvider(req.Provider) {
		return AuthResult{}, domain.ErrOIDCUnknownIssuer
	}
	audience := s.NativeAudiences[req.Provider]
	if !domain.NativeLoginProvider(req.Provider) || audience == "" || s.Verifier == nil {
		return AuthResult{}, domain.ErrProviderLoginUnavailable
	}
	if !domain.ValidProviderToken(req.IDToken) || !domain.ValidNativeNonce(req.Nonce) {
		return AuthResult{}, domain.ErrProviderProofMalformed
	}
	subject, err := s.Verifier.Verify(ctx, req.Provider, req.IDToken, audience, domain.NativeNonceClaim(req.Nonce))
	if err != nil {
		return AuthResult{}, err
	}
	if subject.Issuer == "" || subject.Subject == "" {
		return AuthResult{}, domain.ErrOIDCWrongAudience
	}
	now := s.Clock.NowUnix()
	id, err := s.IDGen()
	if err != nil {
		return AuthResult{}, err
	}
	alias, err := s.AliasGen()
	if err != nil {
		return AuthResult{}, err
	}
	candidate := domain.Account{ID: id, Alias: alias, Status: domain.StatusActive, CreatedAt: now}
	link := domain.ProviderLink{
		AccountID: id,
		Provider:  req.Provider,
		Issuer:    subject.Issuer,
		Subject:   subject.Subject,
		Email:     subject.Email,
		LinkedAt:  now,
	}
	acc, created, err := s.Store.ResolveProviderAccount(ctx, link, candidate)
	if err != nil {
		return AuthResult{}, err
	}
	if err := accountUsable(acc); err != nil {
		return AuthResult{}, err
	}
	sess, err := s.newSession(ctx, acc.ID, now)
	if err != nil {
		return AuthResult{}, err
	}
	// Race guard, identical to ConsumeCode: a suspension or deletion
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
	return AuthResult{Account: acc, Session: sess, Created: created}, nil
}
