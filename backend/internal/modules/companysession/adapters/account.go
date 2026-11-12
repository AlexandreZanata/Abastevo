package adapters

import (
	"context"
	"errors"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"time"
)

// AccountProof reuses live account validation. A second exact verifier check
// binds the returned metadata if refresh interleaves the first check. Create
// compares that source hash/expiry again while the family is locked.
func AccountProof(service *account.Service) func(context.Context, string, string) (app.Proof, error) {
	return func(ctx context.Context, family, token string) (app.Proof, error) {
		if service == nil || service.Store == nil || service.Hasher == nil || service.Clock == nil {
			return app.Proof{}, domain.ErrUnavailable
		}
		owner, err := service.ValidateAccess(ctx, family, token)
		if err != nil {
			for _, denied := range []error{accdomain.ErrCodeUnknown, accdomain.ErrAccountNotFound, accdomain.ErrAccountSuspended, accdomain.ErrAccountDeleted, accdomain.ErrSessionExpired, accdomain.ErrSessionRevoked, accdomain.ErrSessionInvalid} {
				if errors.Is(err, denied) {
					return app.Proof{}, domain.ErrUnauthorized
				}
			}
			return app.Proof{}, domain.ErrUnavailable
		}
		row, found, err := service.Store.Family(ctx, family)
		if err != nil {
			return app.Proof{}, domain.ErrUnavailable
		}
		now := service.Clock.NowUnix()
		if !found || row.AccountID != owner || row.RevokedAt != 0 || now < row.IssuedAt || accdomain.RefreshExpired(row.IssuedAt, now) ||
			now > row.AccessExpires || !service.Hasher.Equal(row.AccessHash, row.AccessSalt, token) {
			return app.Proof{}, domain.ErrUnauthorized
		}
		expiry := time.Unix(row.AccessExpires, 0).UTC()
		return app.Proof{AccountID: owner, FamilyID: family, AccessHash: row.AccessHash, AuthenticatedAt: expiry.Add(-time.Duration(accdomain.SessionAccessTTLSeconds) * time.Second),
			AccessExpiresAt: expiry, FamilyExpiresAt: time.Unix(row.IssuedAt, 0).UTC().Add(time.Duration(accdomain.RefreshAbsoluteLifetimeDays) * 24 * time.Hour)}, nil
	}
}
