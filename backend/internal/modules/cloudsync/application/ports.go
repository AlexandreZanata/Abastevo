package application

import (
	"context"
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"time"
)

type Principal struct{ AccountID, FamilyID, ContributorID, KeyID, DeviceID string }
type Record struct {
	Dataset  string          `json:"dataset"`
	ID       string          `json:"id"`
	Revision int64           `json:"revision"`
	Deleted  bool            `json:"deleted"`
	Payload  json.RawMessage `json:"payload"`
}
type VehicleStore interface {
	PutVehicle(context.Context, Principal, domain.Vehicle, int64) (Record, error)
}

type PortableStore interface {
	PutFavorite(context.Context, Principal, string, int64) (Record, error)
	PutPreferences(context.Context, Principal, domain.Preferences, int64) (Record, error)
}

type Device struct {
	ID          string     `json:"id"`
	Origin      string     `json:"origin"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSuccess *time.Time `json:"last_success_at"`
	Revoked     bool       `json:"revoked"`
}
type DeviceStore interface {
	RegisterDevice(context.Context, Principal) (Device, error)
	RevokeDevice(context.Context, Principal, string) error
	CheckDevice(context.Context, Principal) error
}
