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
	// Energy propulsion and storage (EE-10, optional for legacy ICE).
	// Pointers distinguish absent (preserve on partial update) from set.
	// BEV carries battery without tank (CapacityMilli 0); PHEV carries
	// one battery and one tank; HEV carries tank without external
	// charge; ICE carries tank without battery. Unknown propulsion is
	// refused; battery is never backfilled.
	Propulsion *string `json:"propulsion,omitempty"`
	BatteryWh  *int64  `json:"battery_wh,omitempty"`
}

// Propulsion kinds. Empty/absent means legacy ICE (tank only).
const (
	PropulsionICE  = "ICE"
	PropulsionBEV  = "BEV"
	PropulsionPHEV = "PHEV"
	PropulsionHEV  = "HEV"
)

func NormalizeVehicle(v Vehicle) (Vehicle, error) {
	v.Name = strings.TrimSpace(v.Name)
	if !ValidID(v.ID) || !utf8.ValidString(v.Name) || utf8.RuneCountInString(v.Name) < 1 || utf8.RuneCountInString(v.Name) > 100 || v.SortOrder < 0 || v.SortOrder > 100000 {
		return Vehicle{}, ErrInvalid
	}
	if e := normalizeEnergy(&v); e != nil {
		return Vehicle{}, e
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

// normalizeEnergy validates propulsion, battery and tank as one unit.
// Absent propulsion means legacy ICE (tank required, no battery). Battery
// is never defaulted: nil stays nil unless explicitly set.
func normalizeEnergy(v *Vehicle) error {
	propulsion := PropulsionICE
	if v.Propulsion != nil {
		propulsion = *v.Propulsion
	}
	switch propulsion {
	case PropulsionICE:
		if v.BatteryWh != nil {
			return ErrInvalid
		}
		if v.CapacityMilli < 10 || v.CapacityMilli > 200000 || v.CapacityMilli%10 != 0 {
			return ErrInvalid
		}
	case PropulsionBEV:
		if v.BatteryWh == nil || *v.BatteryWh < 1 || *v.BatteryWh > 1000000000 {
			return ErrInvalid
		}
		if v.CapacityMilli != 0 {
			return ErrInvalid
		}
	case PropulsionPHEV:
		if v.BatteryWh == nil || *v.BatteryWh < 1 || *v.BatteryWh > 1000000000 {
			return ErrInvalid
		}
		if v.CapacityMilli < 10 || v.CapacityMilli > 200000 || v.CapacityMilli%10 != 0 {
			return ErrInvalid
		}
	case PropulsionHEV:
		if v.BatteryWh != nil {
			return ErrInvalid
		}
		if v.CapacityMilli < 10 || v.CapacityMilli > 200000 || v.CapacityMilli%10 != 0 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// MergeVehicleEnergy preserves stored energy fields when the incoming
// update omits them (nil). Older clients without propulsion/battery send
// nil; their stored values survive instead of erasing. Explicit values
// (including a switch to ICE with nil battery) always win. No backfill:
// nil stays nil unless the update sets it.
func MergeVehicleEnergy(stored, incoming Vehicle) Vehicle {
	if incoming.Propulsion == nil && stored.Propulsion != nil {
		propulsion := *stored.Propulsion
		incoming.Propulsion = &propulsion
	}
	if incoming.BatteryWh == nil && stored.BatteryWh != nil {
		battery := *stored.BatteryWh
		incoming.BatteryWh = &battery
	}
	return incoming
}

// RequireVehicleRegistration counts retained/archived IDs as registrations.
// Existing-ID edits/replays never invoke this admission rule.
func RequireVehicleRegistration(current int64, multiple bool) error {
	if current < 0 {
		return ErrInvalid
	}
	if current >= 3 {
		return ErrQuota
	}
	if current >= 1 && !multiple {
		return ErrDenied
	}
	return nil
}
