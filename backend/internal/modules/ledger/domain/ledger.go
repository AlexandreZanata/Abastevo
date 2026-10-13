// Package domain holds pure private refuel and expense values (PRM05-T01).
// Exact integer math only: BRL totals in centavos, quantities in
// milli-units, odometers in metres. No floats, no silent repair.
package domain

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const (
	MaxTotalCentavos  = 1000000000000
	MaxQuantityMilli  = 1000000000
	MaxOdometerMetres = 1000000000000
	MaxUnitPriceMilli = 1000000
	MaxNoteRunes      = 280
)

var (
	ErrInvalidID       = errors.New("ledger: invalid id")
	ErrInvalidVehicle  = errors.New("ledger: invalid vehicle")
	ErrInvalidTime     = errors.New("ledger: invalid event time")
	ErrInvalidTimezone = errors.New("ledger: invalid timezone")
	ErrInvalidFuel     = errors.New("ledger: invalid fuel")
	ErrInvalidUnit     = errors.New("ledger: invalid unit")
	ErrInvalidQuantity = errors.New("ledger: invalid quantity")
	ErrInvalidTotal    = errors.New("ledger: invalid total")
	ErrInvalidPrice    = errors.New("ledger: invalid unit price")
	ErrInvalidOdometer = errors.New("ledger: invalid odometer")
	ErrInvalidFill     = errors.New("ledger: invalid fill marker")
	ErrInvalidCategory = errors.New("ledger: invalid category")
	ErrInvalidNote     = errors.New("ledger: invalid note")
	ErrInvalidStation  = errors.New("ledger: invalid station")
)

type Fill string

const (
	Full    Fill = "full"
	Partial Fill = "partial"
	Unknown Fill = "unknown"
)

type Category string

const (
	Maintenance Category = "maintenance"
	Insurance   Category = "insurance"
	Taxes       Category = "taxes"
	Parking     Category = "parking"
	Tolls       Category = "tolls"
	Cleaning    Category = "cleaning"
	Other       Category = "other"
)

type Refuel struct {
	ID             string         `json:"id"`
	VehicleID      string         `json:"vehicle_id"`
	OccurredAt     time.Time      `json:"occurred_at"`
	Timezone       string         `json:"timezone"`
	Fuel           kernel.Product `json:"fuel"`
	Unit           kernel.Unit    `json:"unit"`
	QuantityMilli  int64          `json:"quantity_milli"`
	TotalCentavos  int64          `json:"total_centavos"`
	UnitPriceMilli int64          `json:"unit_price_milli,omitempty"`
	OdometerMetres *int64         `json:"odometer_metres"`
	Fill           Fill           `json:"fill"`
	StationID      string         `json:"station_id,omitempty"`
	Note           string         `json:"note,omitempty"`
}

type Expense struct {
	ID             string    `json:"id"`
	VehicleID      string    `json:"vehicle_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	Timezone       string    `json:"timezone"`
	AmountCentavos int64     `json:"amount_centavos"`
	Category       Category  `json:"category"`
	OdometerMetres *int64    `json:"odometer_metres"`
	Note           string    `json:"note,omitempty"`
}

func validID(s string) bool {
	if s == "00000000-0000-0000-0000-000000000000" || len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil
}

func validNote(s string) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > MaxNoteRunes {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func checkIdentity(id, vehicleID string) error {
	if !validID(id) {
		return ErrInvalidID
	}
	if !validID(vehicleID) {
		return ErrInvalidVehicle
	}
	return nil
}

func checkTime(occurredAt time.Time, timezone string) error {
	if occurredAt.IsZero() {
		return ErrInvalidTime
	}
	if len(timezone) == 0 || len(timezone) > 64 {
		return ErrInvalidTimezone
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return ErrInvalidTimezone
	}
	return nil
}

func checkOdometer(odometer *int64) error {
	if odometer == nil {
		return nil
	}
	if *odometer < 0 || *odometer > MaxOdometerMetres {
		return ErrInvalidOdometer
	}
	return nil
}

func NormalizeRefuel(r Refuel) (Refuel, error) {
	if err := checkIdentity(r.ID, r.VehicleID); err != nil {
		return Refuel{}, err
	}
	if err := checkTime(r.OccurredAt, r.Timezone); err != nil {
		return Refuel{}, err
	}
	switch r.Fuel {
	case kernel.Ethanol, kernel.GasolineRegular, kernel.GasolineAdditived, kernel.GasolinePremiumGrade, kernel.DieselS500, kernel.DieselS10, kernel.CNG:
	default:
		return Refuel{}, ErrInvalidFuel
	}
	wantUnit, err := r.Fuel.Unit()
	if err != nil {
		return Refuel{}, ErrInvalidFuel
	}
	if r.Unit != wantUnit || (r.Unit != kernel.Liter && r.Unit != kernel.CubicMetre) {
		return Refuel{}, ErrInvalidUnit
	}
	if r.QuantityMilli < 1 || r.QuantityMilli > MaxQuantityMilli {
		return Refuel{}, ErrInvalidQuantity
	}
	if r.TotalCentavos < 0 || r.TotalCentavos > MaxTotalCentavos {
		return Refuel{}, ErrInvalidTotal
	}
	if r.UnitPriceMilli < 0 || r.UnitPriceMilli > MaxUnitPriceMilli {
		return Refuel{}, ErrInvalidPrice
	}
	if err := checkOdometer(r.OdometerMetres); err != nil {
		return Refuel{}, err
	}
	switch r.Fill {
	case Full, Partial, Unknown:
	default:
		return Refuel{}, ErrInvalidFill
	}
	if r.StationID != "" && !validID(r.StationID) {
		return Refuel{}, ErrInvalidStation
	}
	if !validNote(r.Note) {
		return Refuel{}, ErrInvalidNote
	}
	return r, nil
}

func NormalizeExpense(e Expense) (Expense, error) {
	if err := checkIdentity(e.ID, e.VehicleID); err != nil {
		return Expense{}, err
	}
	if err := checkTime(e.OccurredAt, e.Timezone); err != nil {
		return Expense{}, err
	}
	if e.AmountCentavos < 0 || e.AmountCentavos > MaxTotalCentavos {
		return Expense{}, ErrInvalidTotal
	}
	switch e.Category {
	case Maintenance, Insurance, Taxes, Parking, Tolls, Cleaning, Other:
	default:
		return Expense{}, ErrInvalidCategory
	}
	if err := checkOdometer(e.OdometerMetres); err != nil {
		return Expense{}, err
	}
	if !validNote(e.Note) {
		return Expense{}, ErrInvalidNote
	}
	return e, nil
}

// HasPriceDiscrepancy reports whether the stated unit price times quantity
// differs from the authoritative actual total. Exact integer comparison:
// unit_price_milli * quantity_milli versus total_centavos * 10000.
// Absent unit prices never mismatch.
func (r Refuel) HasPriceDiscrepancy() bool {
	if r.UnitPriceMilli == 0 {
		return false
	}
	return r.UnitPriceMilli*r.QuantityMilli != r.TotalCentavos*10000
}
