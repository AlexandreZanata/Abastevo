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
	// AuthorizationCode is the optional single-use code of the same
	// native authorization. When grant capture is configured it is
	// redeemed for a revocable refresh token; otherwise it is ignored.
	AuthorizationCode string
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
// strings never select or merge an account. An accompanying
// authorization code is redeemed best-effort into a sealed, revocable
// provider grant once the account is known to be usable.
func (s *Service) LoginWithProvider(ctx context.Context, req ProviderLogin) (AuthResult, error) {
	subject, err := s.verifyNativeProvider(ctx, req)
	if err != nil {
		return AuthResult{}, err
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
	var acc domain.Account
	var created bool
	if req.Provider == "google" {
		// Recovery uses the hashed address and masked hint. Keep the
		// legacy provider email column empty for the native Google flow.
		link.Email = ""
		acc, created, err = s.Store.ResolveGoogleAccount(ctx, link, candidate, domain.GoogleRecoveryAddress(subject))
	} else {
		acc, created, err = s.Store.ResolveProviderAccount(ctx, link, candidate)
	}
	if err != nil {
		return AuthResult{}, err
	}
	if err := accountUsable(acc); err != nil {
		return AuthResult{}, err
	}
	if req.Provider == "apple" {
		s.captureGrant(ctx, acc.ID, req.Provider, subject.Subject, req.AuthorizationCode, now)
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

func (s *Service) verifyNativeProvider(ctx context.Context, req ProviderLogin) (domain.ProviderSubject, error) {
	if !domain.ValidProvider(req.Provider) {
		return domain.ProviderSubject{}, domain.ErrOIDCUnknownIssuer
	}
	audience := s.NativeAudiences[req.Provider]
	if !domain.NativeLoginProvider(req.Provider) || audience == "" || s.Verifier == nil {
		return domain.ProviderSubject{}, domain.ErrProviderLoginUnavailable
	}
	if !domain.ValidProviderToken(req.IDToken) || !domain.ValidNativeNonce(req.Nonce) ||
		(req.AuthorizationCode != "" && !domain.ValidAuthorizationCode(req.AuthorizationCode)) {
		return domain.ProviderSubject{}, domain.ErrProviderProofMalformed
	}
	subject, err := s.Verifier.Verify(ctx, req.Provider, req.IDToken, audience, domain.NativeNonceClaim(req.Nonce))
	if err != nil {
		return domain.ProviderSubject{}, err
	}
	if subject.Issuer == "" || subject.Subject == "" {
		return domain.ProviderSubject{}, domain.ErrOIDCWrongAudience
	}
	return subject, nil
}

// LinkGoogleNative binds a Google proof to an account with a fresh session.
func (s *Service) LinkGoogleNative(ctx context.Context, familyID, accessToken string, req ProviderLogin) (domain.ProviderLink, error) {
	if req.Provider != "google" || req.AuthorizationCode != "" {
		return domain.ProviderLink{}, domain.ErrProviderProofMalformed
	}
	proof, err := s.validateSessionProof(ctx, familyID, accessToken, true)
	if err != nil {
		return domain.ProviderLink{}, err
	}
	subject, err := s.verifyNativeProvider(ctx, req)
	if err != nil {
		return domain.ProviderLink{}, err
	}
	// Verification can take time. The store checks the session again
	// under the account and family locks at the time of the write.
	proof.Now = s.Clock.NowUnix()
	return s.Store.LinkGoogleWithProof(ctx, proof, domain.ProviderLink{
		AccountID: proof.AccountID, Provider: req.Provider, Issuer: subject.Issuer,
		Subject: subject.Subject, LinkedAt: proof.Now,
	})
}
