package application

import (
	"context"
	"encoding/json"
)

type TripFieldRepository interface {
	PullTripField(context.Context, Source, string) (TripField, error)
}

// TripField is the allowlisted driver projection for one enrolled trip.
type TripField struct {
	TripID          string          `json:"trip_id"`
	Plan            json.RawMessage `json:"plan"`
	Expired         int             `json:"expired"`
	DueSoon         int             `json:"due_soon"`
	NearestDays     *int            `json:"nearest_days"`
	OpenMaintenance int             `json:"open_maintenance"`
	Voucher         *TripVoucher    `json:"voucher"`
}

type TripVoucher struct {
	ID               string `json:"id"`
	Product          string `json:"product"`
	Unit             string `json:"unit"`
	MaxQuantityMilli int64  `json:"max_quantity_milli"`
	MaxCentavos      int64  `json:"max_centavos"`
	ValidUntil       int64  `json:"valid_until"`
	State            string `json:"state"`
}
