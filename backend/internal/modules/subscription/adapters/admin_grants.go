package adapters

import (
	"context"
	"errors"
	"time"

	qsub "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/subscription"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// adminGrants reads active, unexpired admin grants inside the caller
// transaction. Capabilities come from the code catalog, never the row.
func adminGrants(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) ([]string, time.Time, []domain.TierStatus, error) {
	rows, err := qsub.New(tx).AdminGrants(ctx, qsub.AdminGrantsParams{AccountID: uid(accountID), Now: stamp(now)})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			return nil, time.Time{}, nil, nil
		}
		return nil, time.Time{}, nil, err
	}
	seen := map[string]bool{}
	var capabilities []string
	var latest time.Time
	var tiers []domain.TierStatus
	for _, row := range rows {
		values, ok := entdomain.Capabilities(entdomain.Tier(row.Tier), entdomain.PolicyVersion)
		if !ok {
			continue
		}
		for _, value := range values {
			if !seen[value] {
				seen[value] = true
				capabilities = append(capabilities, value)
			}
		}
		status := domain.TierStatus{Tier: row.Tier, Source: "admin"}
		if row.EndAt.Valid {
			until := row.EndAt.Time.UTC()
			status.ValidUntil = &until
			if row.EndAt.Time.After(latest) {
				latest = row.EndAt.Time
			}
		}
		tiers = append(tiers, status)
	}
	return capabilities, latest, tiers, nil
}
