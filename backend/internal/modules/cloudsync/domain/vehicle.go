package domain

import (
	"encoding/hex"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid      = errors.New("cloud: invalid input")
	ErrUnauthorized = errors.New("cloud: authentication required")
	ErrDenied       = errors.New("cloud: capability or consent required")
	ErrConflict     = errors.New("cloud: revision conflict")
	ErrQuota        = errors.New("cloud: bounded quota exceeded")
	ErrNotFound     = errors.New("cloud: record unavailable")
	ErrUnavailable  = errors.New("cloud: service unavailable")
	ErrResnapshot   = errors.New("cloud: resnapshot required")
)

func ValidID(s string) bool {
	if s == "00000000-0000-0000-0000-000000000000" || len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || strings.ToLower(s) != s {
		return false
	}
	_, e := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return e == nil
}

type Vehicle struct {
	ID            string         `json:"id"`
	Name          string         `json:"display_name"`
	CapacityMilli int64          `json:"capacity_milli"`
	Fuel          kernel.Product `json:"fuel_product"`
	Unit          kernel.Unit    `json:"unit"`
	PriceSource   string         `json:"price_source"`
	SpecificCNPJ  string         `json:"specific_station_cnpj,omitempty"`
	BasicAlert    bool           `json:"price_drop_alert_enabled"`
	SortOrder     int            `json:"sort_order"`
	Archived      bool           `json:"archived"`
}

func NormalizeVehicle(v Vehicle) (Vehicle, error) {
	v.Name = strings.TrimSpace(v.Name)
	if !ValidID(v.ID) || !utf8.ValidString(v.Name) || utf8.RuneCountInString(v.Name) < 1 || utf8.RuneCountInString(v.Name) > 100 || v.CapacityMilli < 10 || v.CapacityMilli > 200000 || v.CapacityMilli%10 != 0 || v.SortOrder < 0 || v.SortOrder > 100000 {
		return Vehicle{}, ErrInvalid
	}
	for _, r := range v.Name {
		if unicode.IsControl(r) {
			return Vehicle{}, ErrInvalid
		}
	}
	if v.Fuel == "GASOLINE_PREMIUM" {
		v.Fuel = kernel.GasolineAdditived
	}
	unit, e := v.Fuel.Unit()
	if e != nil || unit != v.Unit || v.Fuel == kernel.LPGP13 {
		return Vehicle{}, ErrInvalid
	}
	switch v.PriceSource {
	case "CHEAPEST_STATION":
		if v.SpecificCNPJ != "" {
			return Vehicle{}, ErrInvalid
		}
	case "SPECIFIC_STATION":
		c, e := kernel.ParseCNPJ(v.SpecificCNPJ)
		if e != nil {
			return Vehicle{}, ErrInvalid
		}
		v.SpecificCNPJ = c.Normalized()
	default:
		return Vehicle{}, ErrInvalid
	}
	return v, nil
}
