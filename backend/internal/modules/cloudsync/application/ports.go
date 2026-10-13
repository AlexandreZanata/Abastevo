package application

import (
	"context"
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
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
