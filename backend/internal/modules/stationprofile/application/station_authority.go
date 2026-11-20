package application

import (
	"context"

	agreeapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/application"
)

// StationAuthority proves the station surface's live grant scope for the
// agreement module (EMP-AGREE-002/005): the current grant, live account and
// operator currency are rechecked exactly like the profile manage path.
type StationAuthority struct {
	Grants      DecisionStore
	OperatorOf  func(ctx context.Context, stationID string) (cnpj string, found bool, err error)
	AccountLive func(ctx context.Context, accountID string) (bool, error)
}

var _ agreeapp.StationAuthority = (*StationAuthority)(nil)

// StationGrantTx proves one bounded scope at the station and answers the
// grant identity the agreement module records with its audit events.
func (a StationAuthority) StationGrantTx(ctx context.Context, stationID, accountID, scope string) (agreeapp.StationGrant, error) {
	if accountID == "" || stationID == "" {
		return agreeapp.StationGrant{}, agreeapp.ErrUnauthenticated
	}
	grant, found, err := a.Grants.ActiveGrant(ctx, accountID, stationID)
	if err != nil {
		return agreeapp.StationGrant{}, err
	}
	if !found {
		return agreeapp.StationGrant{}, agreeapp.ErrUnauthenticated
	}
	live, err := a.AccountLive(ctx, accountID)
	if err != nil {
		return agreeapp.StationGrant{}, err
	}
	if !live {
		return agreeapp.StationGrant{}, ErrAccountGone
	}
	cnpj, found, err := a.OperatorOf(ctx, stationID)
	if err != nil {
		return agreeapp.StationGrant{}, err
	}
	if !found || cnpj == "" || cnpj != grant.OperatorCNPJ {
		return agreeapp.StationGrant{}, ErrManageStale
	}
	for _, granted := range grant.Scopes {
		if granted == scope {
			return agreeapp.StationGrant{GrantID: grant.ID, OperatorCNPJ: grant.OperatorCNPJ, Scopes: grant.Scopes}, nil
		}
	}
	return agreeapp.StationGrant{}, agreeapp.ErrUnauthenticated
}
