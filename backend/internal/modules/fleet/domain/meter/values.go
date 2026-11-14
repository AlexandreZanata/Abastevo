// Package meter preserves pure organization-scoped meter evidence, never authority.
package meter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid           = errors.New("meter: invalid evidence")
	ErrScope             = errors.New("meter: organization scope denied")
	ErrAlreadySuperseded = errors.New("meter: reading already voided or superseded")
	ErrOperationReused   = errors.New("meter: operation identifier reused")
	ErrAlreadyInService  = errors.New("meter: kind already in service")
)

type redacted struct{}

func (redacted) String() string                 { return "EnterpriseMeterValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

type Kind string

const (
	Odometer  Kind = "ODOMETER"
	HourMeter Kind = "HOUR_METER"
)

type EntryForm string

const (
	BaseUnits    EntryForm = "BASE"
	Kilometres   EntryForm = "KM"
	DecimalHours EntryForm = "HOURS"
	HoursMinutes EntryForm = "HM"
)

// ParseEntry decodes a caller's explicit kind and entry form without normalization.
func ParseEntry(kind Kind, form EntryForm, text, minutes string, modulus *int64) (Value, error) {
	switch form {
	case BaseUnits:
		return ParseBase(kind, text, modulus)
	case Kilometres:
		if kind == Odometer {
			return ParseKilometres(text, modulus)
		}
	case DecimalHours:
		if kind == HourMeter {
			return ParseHours(text, modulus)
		}
	case HoursMinutes:
		if kind == HourMeter {
			return ParseHoursMinutes(text, minutes, modulus)
		}
	}
	return Value{}, ErrInvalid
}

func (k Kind) Unit() string {
	switch k {
	case Odometer:
		return "m"
	case HourMeter:
		return "s"
	}
	return ""
}
func bound(kind Kind) int64 {
	switch kind {
	case Odometer:
		return 1_000_000_000_000
	case HourMeter:
		return 3_600_000_000
	}
	return -1
}

type Value struct {
	redacted
	kind  Kind
	count int64
}

func NewValue(kind Kind, count int64, modulus *int64) (Value, error) {
	if count < 0 || count > bound(kind) || modulus != nil && (*modulus <= 0 || count >= *modulus) {
		return Value{}, ErrInvalid
	}
	return Value{kind: kind, count: count}, nil
}
func (v Value) Kind() Kind   { return v.kind }
func (v Value) Count() int64 { return v.count }
func integer(raw string, maximum int64) (int64, error) {
	if raw == "" || len(raw) > 19 || len(raw) > 1 && raw[0] == '0' {
		return 0, ErrInvalid
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, ErrInvalid
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n > maximum {
		return 0, ErrInvalid
	}
	return n, nil
}

// ParseBase accepts exact base-unit integer text without normalization.
func ParseBase(kind Kind, raw string, modulus *int64) (Value, error) {
	n, err := integer(raw, bound(kind))
	if err != nil {
		return Value{}, err
	}
	return NewValue(kind, n, modulus)
}
func decimal(kind Kind, raw string, digits int, scale int64, modulus *int64) (Value, error) {
	whole, fraction, has := strings.Cut(raw, ".")
	n, err := integer(whole, bound(kind)/scale)
	if err != nil {
		return Value{}, err
	}
	var fractional int64
	if has {
		if len(fraction) < 1 || len(fraction) > digits {
			return Value{}, ErrInvalid
		}
		divisor := int64(1)
		for i := 0; i < len(fraction); i++ {
			if fraction[i] < '0' || fraction[i] > '9' {
				return Value{}, ErrInvalid
			}
			fractional = fractional*10 + int64(fraction[i]-'0')
			divisor *= 10
		}
		// The accepted precision divides the base-unit scale exactly.
		fractional = fractional * (scale / divisor)
	}
	return NewValue(kind, n*scale+fractional, modulus)
}
func ParseKilometres(raw string, modulus *int64) (Value, error) {
	return decimal(Odometer, raw, 3, 1000, modulus)
}
func ParseHours(raw string, modulus *int64) (Value, error) {
	return decimal(HourMeter, raw, 2, 3600, modulus)
}
func ParseHoursMinutes(hours, minutes string, modulus *int64) (Value, error) {
	h, err := integer(hours, bound(HourMeter)/3600)
	if err != nil {
		return Value{}, err
	}
	m, err := integer(minutes, 59)
	if err != nil {
		return Value{}, err
	}
	return NewValue(HourMeter, h*3600+m*60, modulus)
}
func reason(raw string) bool {
	if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > 280 || strings.TrimSpace(raw) == "" {
		return false
	}
	for _, point := range raw {
		if unicode.IsControl(point) {
			return false
		}
	}
	return true
}

type Missing struct {
	redacted
	reason string
}

func NewMissing(raw string) (Missing, error) {
	if !reason(raw) {
		return Missing{}, ErrInvalid
	}
	return Missing{reason: raw}, nil
}
func (m Missing) Reason() string { return m.reason }

type State string

const (
	Valid   State = "VALID"
	Suspect State = "SUSPECT"
	Voided  State = "VOIDED"
)

type Origin string

const (
	TripStart    Origin = "TRIP_START"
	TripEnd      Origin = "TRIP_END"
	Handover     Origin = "HANDOVER"
	Fueling      Origin = "FUELING"
	Checklist    Origin = "CHECKLIST"
	Maintenance  Origin = "MAINTENANCE"
	Standalone   Origin = "STANDALONE"
	Import       Origin = "IMPORT"
	Installation Origin = "METER_INSTALLATION"
	Removal      Origin = "METER_REMOVAL"
)

func validOrigin(o Origin) bool {
	switch o {
	case TripStart, TripEnd, Handover, Fueling, Checklist, Maintenance, Standalone, Import, Installation, Removal:
		return true
	}
	return false
}

type Method string

const (
	Typed          Method = "TYPED"
	TypedWithPhoto Method = "TYPED_WITH_PHOTO"
	Imported       Method = "IMPORTED"
	Integration    Method = "INTEGRATION"
)

func validMethod(m Method) bool {
	return m == Typed || m == TypedWithPhoto || m == Imported || m == Integration
}
