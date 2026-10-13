package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"slices"
	"strings"
)

const PreferencesID = "00000000-0000-4000-8000-000000000001"

type Preferences struct {
	Version            int            `json:"version"`
	State              string         `json:"preferred_state,omitempty"`
	Municipality       string         `json:"preferred_municipality,omitempty"`
	Fuel               kernel.Product `json:"preferred_fuel_product,omitempty"`
	Locale             string         `json:"locale_tag"`
	LocaleUserSelected bool           `json:"locale_user_selected"`
	ShowPriceHistory   bool           `json:"show_price_history"`
	RadiusKm           int            `json:"nearest_station_radius_km"`
}

func NormalizePreferences(p Preferences) (Preferences, error) {
	if (p.LocaleUserSelected && p.Locale == "") || p.Version != 1 || !slices.Contains([]string{"", "pt-BR", "en", "es", "fr", "zh-CN", "ru", "de", "ja"}, p.Locale) || p.RadiusKm < 1 || p.RadiusKm > 100 {
		return Preferences{}, ErrInvalid
	}
	if p.State != "" && (len(p.State) != 2 || !strings.Contains("|AC|AL|AP|AM|BA|CE|DF|ES|GO|MA|MT|MS|MG|PA|PB|PR|PE|PI|RJ|RN|RS|RO|RR|SC|SP|SE|TO|", "|"+p.State+"|")) {
		return Preferences{}, ErrInvalid
	}
	if p.Municipality != "" {
		if p.State == "" || len(p.Municipality) != 7 {
			return Preferences{}, ErrInvalid
		}
		for _, c := range p.Municipality {
			if c < '0' || c > '9' {
				return Preferences{}, ErrInvalid
			}
		}
	}
	if p.Fuel == "GASOLINE_PREMIUM" {
		p.Fuel = kernel.GasolineAdditived
	}
	if p.Fuel != "" {
		if _, e := p.Fuel.Unit(); e != nil {
			return Preferences{}, ErrInvalid
		}
	}
	return p, nil
}

type Favorite struct {
	StationID string `json:"station_id"`
}
