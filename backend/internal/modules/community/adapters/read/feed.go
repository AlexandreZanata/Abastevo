package read

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
)

// FeedItem is an explicit public allowlist; raw observations and media stay private.
type FeedItem struct {
	StationID      string    `json:"station_id"`
	StationName    string    `json:"station_name"`
	FuelProduct    string    `json:"fuel_product"`
	Unit           string    `json:"unit"`
	AmountMilliBrl int64     `json:"amount_milli_brl"`
	Currency       string    `json:"currency"`
	Source         string    `json:"source"`
	Condition      string    `json:"condition"`
	Confidence     string    `json:"confidence"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Supporters     int       `json:"supporters"`
	Confirmations  int       `json:"confirmations"`
	Version        int64     `json:"version"`
	// Community star average of the same station+fuel; nil when unrated.
	RatingsCount int      `json:"ratings_count"`
	RatingsAvg   *float64 `json:"ratings_avg,omitempty"`
}

func (r *Reader) Feed(ctx context.Context, f application.FeedFilter) ([]FeedItem, string, error) {
	now := r.clock()
	afterAvg := -1.0
	if f.HasAfterAvg {
		afterAvg = f.AfterAvg
	}
	rows, err := community.New(r.pool).CityFeed(ctx, community.CityFeedParams{
		State: pgtype.Text{String: f.State, Valid: true}, MunicipalityCode: pgtype.Text{String: f.Municipality, Valid: true}, FuelProduct: f.Product, Unit: f.Unit,
		Now: pgTime(now), HasCursor: f.HasCursor, SortOrder: f.Order, AfterTime: pgTime(f.AfterTime),
		AfterID: f.AfterID, AfterAmount: f.AfterAmount, AfterAvg: afterAvg, LimitPlusOne: int32(f.Limit + 1),
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		last := rows[len(rows)-1]
		var avg *float64
		if last.RatingsCount > 0 {
			v := float64(last.StarsSum) / float64(last.RatingsCount)
			avg = &v
		}
		next = application.FeedKeyAvg(last.AnchorReceivedAt.Time, uuidString(last.StationID), last.AmountMilliBrl.Int64, avg)
	}
	items := make([]FeedItem, 0, len(rows))
	for _, row := range rows {
		confidence := row.Confidence
		if confidence == "HIGH" && now.Sub(row.AnchorReceivedAt.Time) > 6*time.Hour {
			confidence = "MEDIUM"
		}
		var avg *float64
		if row.RatingsCount > 0 {
			v := float64(row.StarsSum) / float64(row.RatingsCount)
			avg = &v
		}
		items = append(items, FeedItem{StationID: uuidString(row.StationID), StationName: row.DisplayName,
			FuelProduct: row.FuelProduct, Unit: row.Unit, AmountMilliBrl: row.AmountMilliBrl.Int64,
			Currency: "BRL", Source: "COMMUNITY", Condition: "STANDARD", Confidence: confidence,
			UpdatedAt: row.AnchorReceivedAt.Time, ExpiresAt: row.ExpiresAt.Time,
			Supporters: int(row.IndependentSupporters), Confirmations: int(row.ConfirmationCount), Version: row.ProjectionVersion,
			RatingsCount: int(row.RatingsCount), RatingsAvg: avg})
	}
	return items, next, nil
}
