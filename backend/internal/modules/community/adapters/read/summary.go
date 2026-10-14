package read

import (
	"context"
	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
)

// CitySummaryItem is a full-population standard-price statistic, not a price offer.
type CitySummaryItem struct {
	FuelProduct     string    `json:"fuel_product"`
	Unit            string    `json:"unit"`
	Currency        string    `json:"currency"`
	Source          string    `json:"source"`
	Condition       string    `json:"condition"`
	AverageMilliBrl int64     `json:"average_milli_brl"`
	MinimumMilliBrl int64     `json:"minimum_milli_brl"`
	MaximumMilliBrl int64     `json:"maximum_milli_brl"`
	StationCount    int64     `json:"station_count"`
	UpdatedAt       time.Time `json:"updated_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// CitySummary uses one SQL snapshot over current city projections, never paginated observations.
func (r *Reader) CitySummary(ctx context.Context, f application.FeedFilter) ([]CitySummaryItem, error) {
	rows, err := community.New(r.pool).CitySummary(ctx, community.CitySummaryParams{State: pgtype.Text{String: f.State, Valid: true}, MunicipalityCode: pgtype.Text{String: f.Municipality, Valid: true}, Now: pgTime(r.clock())})
	if err != nil {
		return nil, err
	}
	out := make([]CitySummaryItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, CitySummaryItem{FuelProduct: row.FuelProduct, Unit: row.Unit, Currency: "BRL", Source: "COMMUNITY", Condition: "STANDARD", AverageMilliBrl: row.AverageMilliBrl, MinimumMilliBrl: row.MinimumMilliBrl, MaximumMilliBrl: row.MaximumMilliBrl, StationCount: row.StationCount, UpdatedAt: row.UpdatedAt.Time, ExpiresAt: row.ExpiresAt.Time})
	}
	return out, nil
}
