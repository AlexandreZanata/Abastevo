package adapters

import (
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

func NormalizePayload(dataset, id string, raw []byte) ([]byte, error) {
	if !domain.ValidID(id) || len(raw) > 8192 {
		return nil, domain.ErrInvalid
	}
	switch dataset {
	case "vehicles":
		var v domain.Vehicle
		if httpapi.DecodeJSON(raw, &v) != nil || v.ID != id {
			return nil, domain.ErrInvalid
		}
		v, e := domain.NormalizeVehicle(v)
		if e != nil {
			return nil, e
		}
		return json.Marshal(v)
	case "favorites":
		var v domain.Favorite
		if httpapi.DecodeJSON(raw, &v) != nil || v.StationID != id {
			return nil, domain.ErrInvalid
		}
		return json.Marshal(v)
	case "preferences":
		var v domain.Preferences
		if id != domain.PreferencesID || httpapi.DecodeJSON(raw, &v) != nil {
			return nil, domain.ErrInvalid
		}
		v, e := domain.NormalizePreferences(v)
		if e != nil {
			return nil, e
		}
		return json.Marshal(v)
	default:
		return nil, domain.ErrInvalid
	}
}
