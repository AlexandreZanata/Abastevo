package http

import (
	"context"
	"errors"
	"time"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	companydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// AccountSessions reuses the existing live verifier and its exact metadata
// check. It creates no family or alternate login, and retains no credential.
type AccountSessions struct{ Service *account.Service }

func (a AccountSessions) Validate(ctx context.Context, family, token string) (string, error) {
	return a.Confirm(ctx, family, token, false)
}
func (a AccountSessions) Confirm(ctx context.Context, family, token string, fresh bool) (string, error) {
	proof, err := company.AccountProof(a.Service)(ctx, family, token)
	if err != nil {
		if errors.Is(err, companydomain.ErrUnauthorized) {
			return "", orgapi.Unauthenticated
		}
		return "", orgapi.Unavailable
	}
	if fresh && !companydomain.Fresh(proof.AuthenticatedAt, time.Unix(a.Service.Clock.NowUnix(), 0).UTC()) {
		return "", orgapi.FreshAuthRequired
	}
	return proof.AccountID, nil
}
