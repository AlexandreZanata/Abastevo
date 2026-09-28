package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Wire fuel vocabulary (mirrors the OpenAPI FuelProduct enum).
var wireProducts = map[string]bool{
	"ETHANOL": true, "GASOLINE_REGULAR": true, "GASOLINE_ADDITIVED": true,
	"DIESEL_S500": true, "DIESEL_S10": true, "CNG": true, "LPG_P13": true,
}

var (
	ErrInvalidFilter  = errors.New("official: invalid filter")
	ErrUnknownStation = errors.New("official: unknown station")
	ErrBadUUID        = errors.New("official: malformed id")
)

// Condition is the fixed STANDARD group condition until community
// conditions arrive in P04.
type Condition struct {
	Kind        string  `json:"kind"`
	QualifierID *string `json:"qualifier_id"`
}

// OfficialSection is one published ANP value with provenance.
type OfficialSection struct {
	Source      string `json:"source"`
	AmountMilli int64  `json:"amount_milli_brl"`
	Currency    string `json:"currency"`
	CollectedOn string `json:"collected_on"`
	SurveyStart string `json:"survey_week_start"`
	SurveyEnd   string `json:"survey_week_end"`
	RevisionID  string `json:"revision_id"`
	SourceURL   string `json:"source_url"`
	SourceSha   string `json:"source_checksum"`
	RawText     string `json:"raw_price_text"`
}

// PriceGroup couples one product/unit/condition with its official section;
// Community stays null until P04 (UNKNOWN, never substituted).
type PriceGroup struct {
	StationID string           `json:"station_id"`
	Product   string           `json:"fuel_product"`
	Unit      string           `json:"unit"`
	Condition Condition        `json:"condition"`
	Official  *OfficialSection `json:"official"`
	Community *string          `json:"community"`
}

// HistoryEntry is one published official price with provenance.
type HistoryEntry struct {
	RevisionID  string `json:"revision_id"`
	WeekStart   string `json:"survey_week_start"`
	WeekEnd     string `json:"survey_week_end"`
	CollectedOn string `json:"collected_on"`
	Product     string `json:"fuel_product"`
	Unit        string `json:"unit"`
	AmountMilli int64  `json:"amount_milli_brl"`
	Currency    string `json:"currency"`
	SourceURL   string `json:"source_url"`
	SourceSha   string `json:"source_checksum"`
	RawText     string `json:"raw_price_text"`
}

// PriceReader is the owned read port adapters implement.
type PriceReader interface {
	Groups(ctx context.Context, stationID, fuel string) ([]PriceGroup, error)
	History(ctx context.Context, f HistoryFilter) ([]HistoryEntry, string, error)
}

// HistoryFilter carries validated history input. AfterDate/AfterID form the
// opaque sort position ("" on first page); RevisionID scopes one revision.
type HistoryFilter struct {
	StationID  string
	Fuel       string
	RevisionID string
	Limit      int
	AfterDate  string
	AfterID    string
	HasCursor  bool
}

// ValidateHistory enforces station shape, wire fuel vocabulary, optional
// revision scope and the opaque cursor position.
func ValidateHistory(stationID, fuel, revisionID string, limit int, lastKey string) (HistoryFilter, error) {
	if !isUUID(stationID) {
		return HistoryFilter{}, ErrBadUUID
	}
	f := HistoryFilter{StationID: strings.ToLower(stationID), Limit: limit}
	if fuel != "" {
		if !wireProducts[fuel] {
			return HistoryFilter{}, fmt.Errorf("%w: fuel_product", ErrInvalidFilter)
		}
		f.Fuel = fuel
	}
	if revisionID != "" {
		if !isUUID(revisionID) {
			return HistoryFilter{}, fmt.Errorf("%w: revision_id", ErrInvalidFilter)
		}
		f.RevisionID = strings.ToLower(revisionID)
	}
	if limit < 1 || limit > 100 {
		return HistoryFilter{}, fmt.Errorf("%w: limit", ErrInvalidFilter)
	}
	if lastKey != "" {
		date, id, ok := strings.Cut(lastKey, ":")
		if !ok || !isUUID(id) {
			return HistoryFilter{}, fmt.Errorf("%w: cursor", ErrInvalidFilter)
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return HistoryFilter{}, fmt.Errorf("%w: cursor", ErrInvalidFilter)
		}
		f.AfterDate, f.AfterID, f.HasCursor = date, id, true
	}
	return f, nil
}

// ValidateStationID accepts canonical UUID text only.
func ValidateStationID(id string) (string, error) {
	if !isUUID(id) {
		return "", ErrBadUUID
	}
	return strings.ToLower(id), nil
}

// ValidateFuel accepts wire vocabulary or empty (no filter).
func ValidateFuel(fuel string) (string, error) {
	if fuel == "" {
		return "", nil
	}
	if !wireProducts[fuel] {
		return "", fmt.Errorf("%w: fuel_product", ErrInvalidFilter)
	}
	return fuel, nil
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if r != '-' {
				return false
			}
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
