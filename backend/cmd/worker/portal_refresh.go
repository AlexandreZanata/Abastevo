package main

import (
	"context"
	"time"

	enterprisejobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/adapters/jobs"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/jackc/pgx/v5/pgxpool"
)

func portalRefreshDedupe(state enterprisejobs.State) string {
	key := "enterprise-portal-refresh:" + state.ContentSHA256
	if state.Official != nil {
		key += ":" + state.Official.RevisionID + ":" + state.Official.LeaseID
	}
	return key
}

// qualitySourceStatus is the read-only FQ gate for automatic portal refresh.
// LeaseBlock distinguishes a current published revision from a withdrawn or
// absent one without serving event facts.
func qualitySourceStatus(pool *pgxpool.Pool) func(context.Context) (enterprisejobs.SourceStatus, error) {
	return func(ctx context.Context) (enterprisejobs.SourceStatus, error) {
		lease, err := qualityread.NewReader(pool).LeaseBlock(ctx, time.Now(), "sanctions", "sanction-v1")
		if err != nil {
			return enterprisejobs.SourceStatus{}, err
		}
		status := enterprisejobs.SourceStatus{
			RevisionID:       lease.RevisionID,
			Edition:          lease.Edition,
			SourcePeriod:     lease.SourcePeriod,
			SourceClockValue: lease.SourceClockValue,
			Status:           lease.Status,
			HasPublished:     lease.HasPublished,
		}
		if !lease.PublishedAt.IsZero() {
			status.PublishedAt = lease.PublishedAt.UTC().Format(time.RFC3339)
		}
		return status, nil
	}
}
