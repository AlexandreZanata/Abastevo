package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"time"
)

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) Configured() bool {
	return s != nil && s.Provider != nil && s.Tokens != nil && s.AccountBinding != nil && (s.Environment == "test" || s.Environment == "production") && s.PackageName != ""
}
func (s *Service) Status(ctx context.Context, p Principal) (domain.Status, error) {
	if s == nil || s.Store == nil {
		return domain.Status{}, domain.ErrUnavailable
	}
	snapshot, err := s.Store.Status(ctx, p)
	if err != nil {
		return domain.Status{}, err
	}
	now := s.now()
	result := domain.Evaluate(snapshot, now, true)
	webActive := snapshot != nil && len(snapshot.WebCapabilities) > 0 &&
		!snapshot.WebValidUntil.IsZero() && now.Before(snapshot.WebValidUntil)
	adminActive := snapshot != nil && len(snapshot.AdminCapabilities) > 0 &&
		(snapshot.AdminValidUntil.IsZero() || now.Before(snapshot.AdminValidUntil))
	result.PaymentsConfigured = s.Configured() || webActive
	if !result.PaymentsConfigured && !adminActive {
		result.Capabilities = []string{}
		result.Reason = "payments-disabled"
	}
	return result, nil
}
func (s *Service) Admit(ctx context.Context, p Principal, c Command) (Receipt, error) {
	if !domain.ValidID(p.AccountID) || !domain.ValidID(p.FamilyID) || !domain.ValidID(p.ContributorID) || !domain.ValidID(p.KeyID) {
		return Receipt{}, domain.ErrUnauthorized
	}
	if !domain.ValidID(c.OperationID) || !domain.ValidToken(c.Token) || (c.Kind != "verify" && c.Kind != "restore") {
		return Receipt{}, domain.ErrInvalid
	}
	if !s.Configured() || s.Store == nil {
		return Receipt{}, domain.ErrUnavailable
	}
	digest := domain.TokenDigest(s.Environment, s.PackageName, c.Token)
	aad := s.Environment + "\x00" + s.PackageName + "\x00" + p.AccountID + "\x00" + digest
	cipher, err := s.Tokens.Seal(c.Token, aad)
	if err != nil {
		return Receipt{}, domain.ErrUnavailable
	}
	hash := sha256.Sum256([]byte(c.Kind + "\x00" + digest))
	a, err := s.Store.Begin(ctx, Attempt{Principal: p, OperationID: c.OperationID, Digest: digest, PayloadHash: hex.EncodeToString(hash[:]), Environment: s.Environment, PackageName: s.PackageName, CipherToken: cipher, Kind: c.Kind})
	if err != nil {
		return Receipt{}, err
	}
	if a.Replay {
		if a.Result == "denied" {
			return Receipt{}, domain.ErrDenied
		}
		status, err := s.Status(ctx, p)
		return Receipt{OperationID: c.OperationID, Status: status, Replayed: true}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, domain.ProviderTimeout)
	purchase, err := s.Provider.Verify(reqCtx, ProviderRequest{Token: c.Token, ExpectedAccount: s.AccountBinding(p.AccountID), Environment: s.Environment, PackageName: s.PackageName})
	cancel()
	if err != nil {
		if errors.Is(err, domain.ErrDenied) || errors.Is(err, domain.ErrNotFound) {
			if e := s.Store.Reject(ctx, a); e != nil {
				return Receipt{}, e
			}
			return Receipt{}, domain.ErrDenied
		}
		return Receipt{}, domain.ErrUnavailable
	}
	// Verification time is our completed authoritative lookup, never a client timestamp.
	purchase.Snapshot.VerifiedAt = s.now()
	linked := ""
	if purchase.LinkedToken != "" {
		linked = domain.TokenDigest(s.Environment, s.PackageName, purchase.LinkedToken)
	}
	if err = s.Store.Commit(ctx, a, VerifiedPurchase{Snapshot: purchase.Snapshot, LinkedDigest: linked}); err != nil {
		return Receipt{}, err
	}
	status, err := s.Status(ctx, p)
	return Receipt{OperationID: c.OperationID, Status: status}, err
}

func (s *Service) Export(ctx context.Context, p Principal, limit int, after time.Time, key string) ([]ExportRow, error) {
	if s == nil || s.Store == nil {
		return nil, domain.ErrUnavailable
	}
	store, ok := s.Store.(ExportStore)
	if !ok {
		return nil, domain.ErrUnavailable
	}
	// Deliberately independent of Provider/Tokens/AccountBinding and paid state.
	return store.ExportRows(ctx, p, limit, after, key)
}
