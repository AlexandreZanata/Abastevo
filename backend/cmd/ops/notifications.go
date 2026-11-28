package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	notificationsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/adapters"
	notificationsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
)

// contributionProducers resolves an observation's attribution token to its
// contributor (identity) and then to the bound account (account), inside
// the decision transaction (B-BR-NOTIF-02).
func contributionProducers(raw *pgxpool.Pool, identity *identityadapters.Registrar, accounts *accountadapters.PGStore) notificationsapp.Producers {
	return notificationsapp.Producers{
		Notifier: notificationsadapters.NewStore(raw),
		Clock:    time.Now,
		AccountOfContributor: func(ctx context.Context, tx pgx.Tx, ref string) (string, bool, error) {
			contributor, found, err := identity.ContributorForAttributionTx(ctx, tx, ref)
			if err != nil || !found {
				return "", false, err
			}
			return accounts.FindBindingOwnerTx(ctx, tx, contributor)
		},
	}
}

// contributionNotices turns terminal observation decisions into account
// notifications; contributors without an account get nothing.
func contributionNotices(producers notificationsapp.Producers) communityapp.DecisionNotifier {
	return func(ctx context.Context, tx pgx.Tx, n communityapp.DecisionNotice) error {
		return producers.ContributionDecided(ctx, tx, notificationsapp.ContributionFacts{
			ContributorRef: n.ContributorRef, ObservationID: n.ObservationID, StationID: n.StationID,
			StationName: n.StationName, Fuel: n.Product, Validated: n.Validated, Reason: n.Reason,
		})
	}
}
