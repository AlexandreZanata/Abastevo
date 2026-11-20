package adapters

import (
	"context"
	"errors"
	"time"

	dbstationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CommerceStore persists the station's announced prices and labelled
// promotions through the reviewed sqlc queries (000260).
type CommerceStore struct {
	Q      *dbstationprofile.Queries
	Prices *dbstationprofile.Queries
}

var _ application.CommerceStore = (*CommerceStore)(nil)

func uuidOrNull(raw string) pgtype.UUID {
	var out pgtype.UUID
	if raw == "" {
		return out
	}
	if err := out.Scan(raw); err != nil {
		return pgtype.UUID{}
	}
	return out
}

func stampTime(v time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: v.UTC(), Valid: !v.IsZero()}
}

func (s CommerceStore) queries() *dbstationprofile.Queries {
	if s.Prices != nil {
		return s.Prices
	}
	return s.Q
}

func announcedOf(row dbstationprofile.StationAnnouncedPrice) application.StoredAnnouncedPrice {
	return application.StoredAnnouncedPrice{
		ID:            row.ID.String(),
		Product:       row.FuelProduct,
		Unit:          row.Unit,
		PriceMilliBRL: row.PriceMilliBrl,
		Conditions:    row.Conditions,
		ValidFrom:     row.ValidFrom.Time.UTC(),
		ValidUntil:    row.ValidUntil.Time.UTC(),
		State:         row.State,
	}
}

func (s CommerceStore) InsertAnnouncedPrice(ctx context.Context, stationID, id string, in application.StoredAnnouncedPrice) (application.StoredAnnouncedPrice, error) {
	row, err := s.queries().InsertAnnouncedPrice(ctx, dbstationprofile.InsertAnnouncedPriceParams{
		ID: uuidOrNull(id), StationID: uuidOrNull(stationID), FuelProduct: in.Product, Unit: in.Unit,
		PriceMilliBrl: in.PriceMilliBRL, Conditions: in.Conditions, ValidFrom: stampTime(in.ValidFrom), ValidUntil: stampTime(in.ValidUntil),
		AuthorAccountID: uuidOrNull(in.AuthorAccountID),
	})
	if err != nil {
		return application.StoredAnnouncedPrice{}, err
	}
	return announcedOf(row), nil
}

func (s CommerceStore) WithdrawAnnouncedPrice(ctx context.Context, stationID, priceID string) (application.StoredAnnouncedPrice, error) {
	row, err := s.queries().WithdrawAnnouncedPrice(ctx, dbstationprofile.WithdrawAnnouncedPriceParams{
		ID: uuidOrNull(priceID), StationID: uuidOrNull(stationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.StoredAnnouncedPrice{}, application.ErrCommerceNotFound
	}
	if err != nil {
		return application.StoredAnnouncedPrice{}, err
	}
	return announcedOf(row), nil
}

func (s CommerceStore) ListAnnouncedPrices(ctx context.Context, stationID string) ([]application.StoredAnnouncedPrice, error) {
	rows, err := s.queries().ListAnnouncedPrices(ctx, uuidOrNull(stationID))
	if err != nil {
		return nil, err
	}
	out := make([]application.StoredAnnouncedPrice, 0, len(rows))
	for _, row := range rows {
		out = append(out, announcedOf(row))
	}
	return out, nil
}

func promotionOf(row dbstationprofile.StationPromotion) application.StoredPromotion {
	product := ""
	if row.FuelProduct.Valid {
		product = row.FuelProduct.String
	}
	return application.StoredPromotion{
		ID:          row.ID.String(),
		Label:       row.Label,
		Description: row.Description,
		Product:     product,
		ValidFrom:   row.ValidFrom.Time.UTC(),
		ValidUntil:  row.ValidUntil.Time.UTC(),
		State:       row.State,
	}
}

func (s CommerceStore) InsertPromotion(ctx context.Context, stationID, id string, in application.StoredPromotion) (application.StoredPromotion, error) {
	product := pgtype.Text{}
	if in.Product != "" {
		product = pgtype.Text{String: in.Product, Valid: true}
	}
	row, err := s.queries().InsertPromotion(ctx, dbstationprofile.InsertPromotionParams{
		ID: uuidOrNull(id), StationID: uuidOrNull(stationID), Label: in.Label, Description: in.Description,
		FuelProduct: product, ValidFrom: stampTime(in.ValidFrom), ValidUntil: stampTime(in.ValidUntil),
		AuthorAccountID: uuidOrNull(in.AuthorAccountID),
	})
	if err != nil {
		return application.StoredPromotion{}, err
	}
	return promotionOf(row), nil
}

func (s CommerceStore) WithdrawPromotion(ctx context.Context, stationID, promotionID string) (application.StoredPromotion, error) {
	row, err := s.queries().WithdrawPromotion(ctx, dbstationprofile.WithdrawPromotionParams{
		ID: uuidOrNull(promotionID), StationID: uuidOrNull(stationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.StoredPromotion{}, application.ErrCommerceNotFound
	}
	if err != nil {
		return application.StoredPromotion{}, err
	}
	return promotionOf(row), nil
}

func (s CommerceStore) ListPromotions(ctx context.Context, stationID string) ([]application.StoredPromotion, error) {
	rows, err := s.queries().ListPromotions(ctx, uuidOrNull(stationID))
	if err != nil {
		return nil, err
	}
	out := make([]application.StoredPromotion, 0, len(rows))
	for _, row := range rows {
		out = append(out, promotionOf(row))
	}
	return out, nil
}

// AnnouncedDisplayState is the read-time projection of one stored row.
func AnnouncedDisplayState(state string, now, from, until time.Time) string {
	return string(domain.AnnouncedDisplayState(domain.AnnouncedPriceState(state), now, from, until))
}

// PromotionDisplayState is the read-time projection of one stored row.
func PromotionDisplayState(state string, now, from, until time.Time) string {
	return string(domain.PromotionDisplayState(domain.PromotionState(state), now, from, until))
}
