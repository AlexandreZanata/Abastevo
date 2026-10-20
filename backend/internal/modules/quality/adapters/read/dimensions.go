package read

import (
	"context"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// SampleDimensions counts judged samples, independently of raw assay cells.
// An absent derived row means unavailable, never a fabricated zero denominator.
type SampleDimensions struct {
	Available   bool              `json:"available"`
	Methodology string            `json:"methodology"`
	Limitation  string            `json:"limitation"`
	Items       []SampleDimension `json:"items"`
}

type SampleDimension struct {
	Fuel             string `json:"fuel_product"`
	Policy           string `json:"policy"`
	InputDigest      string `json:"input_digest"`
	Samples          int64  `json:"samples"`
	Visits           int64  `json:"visits"`
	Conforming       int64  `json:"conforming"`
	Nonconforming    int64  `json:"nonconforming"`
	Unresolved       int64  `json:"unresolved"`
	OldestObservedOn string `json:"oldest_observed_on"`
	NewestObservedOn string `json:"newest_observed_on"`
	ComputedAt       string `json:"computed_at"`
	ScoreState       string `json:"score_state"`
	Score            *int   `json:"score"`
}

func (r *Reader) DimensionsAtLease(ctx context.Context, lease Lease, station, fuel string) (SampleDimensions, error) {
	out := SampleDimensions{Methodology: domain.MethodologyVersion, Limitation: domain.MethodologyLimitation, Items: []SampleDimension{}}
	if !lease.HasPublished {
		return out, nil
	}
	id, err := mustUUID(station)
	if err != nil {
		return out, err
	}
	revision, err := mustUUID(lease.RevisionID)
	if err != nil {
		return out, err
	}
	rows, err := quality.New(r.pool).ReadStationDimensions(ctx, quality.ReadStationDimensionsParams{StationID: id, RevisionID: revision, Methodology: domain.MethodologyVersion, Fuel: fuel})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Items = append(out.Items, SampleDimension{Fuel: row.Fuel, Policy: row.Policy, InputDigest: row.InputDigest, Samples: row.Samples, Visits: row.Visits, Conforming: row.Conforming, Nonconforming: row.Nonconforming, Unresolved: row.Unresolved, OldestObservedOn: dateString(row.OldestObservedOn), NewestObservedOn: dateString(row.NewestObservedOn), ComputedAt: row.ComputedAt.Time.UTC().Format(time.RFC3339), ScoreState: string(domain.ScoreUnscored)})
	}
	out.Available = len(out.Items) > 0
	return out, nil
}
