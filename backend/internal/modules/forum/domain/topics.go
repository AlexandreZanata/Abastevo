package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Quota policy constants for forum user topics per B-BR-FOR-11.
const (
	MaxTopicTitleRunes      = 120
	MaxTopicBodyRunes       = 4000
	MaxTopicHourlyQuota     = 3
	MaxTopicDailyQuota      = 10
	MaxLiveTopicsPerAccount = 20
)

var validUFs = map[string]int{
	"RO": 11, "AC": 12, "AM": 13, "RR": 14, "PA": 15, "AP": 16, "TO": 17,
	"MA": 21, "PI": 22, "CE": 23, "RN": 24, "PB": 25, "PE": 26, "AL": 27, "SE": 28, "BA": 29,
	"MG": 31, "ES": 32, "RJ": 33, "SP": 35,
	"PR": 41, "SC": 42, "RS": 43,
	"MS": 50, "MT": 51, "GO": 52, "DF": 53,
}

var validFuelProducts = map[string]bool{
	"GASOLINE_REGULAR": true,
	"GASOLINE_PREMIUM": true,
	"ETHANOL":          true,
	"DIESEL_S10":       true,
	"DIESEL_S500":      true,
	"GLP":              true,
	"GNV":              true,
}

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidateTopicTitle validates plain UTF-8 text, rejects NUL bytes,
// and enforces 1..120 Unicode code points (scalar values) after trimming.
func ValidateTopicTitle(title string) (string, error) {
	if !utf8.ValidString(title) {
		return "", ErrInvalidUTF8
	}
	if strings.IndexByte(title, 0) >= 0 {
		return "", ErrInvalidUTF8
	}
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "", ErrTopicTitleEmpty
	}
	count := utf8.RuneCountInString(trimmed)
	if count < 1 {
		return "", ErrTopicTitleEmpty
	}
	if count > MaxTopicTitleRunes {
		return "", ErrTopicTitleTooLong
	}
	return trimmed, nil
}

// ValidateTopicBody validates plain UTF-8 text, rejects NUL bytes,
// and enforces 1..4000 Unicode code points (scalar values) after trimming.
func ValidateTopicBody(body string) (string, error) {
	if !utf8.ValidString(body) {
		return "", ErrInvalidUTF8
	}
	if strings.IndexByte(body, 0) >= 0 {
		return "", ErrInvalidUTF8
	}
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", ErrTopicBodyEmpty
	}
	count := utf8.RuneCountInString(trimmed)
	if count < 1 {
		return "", ErrTopicBodyEmpty
	}
	if count > MaxTopicBodyRunes {
		return "", ErrTopicBodyTooLong
	}
	return trimmed, nil
}

// ValidateTopicRegion validates Brazilian UF code (2 uppercase letters)
// and optional 7-digit IBGE municipality code ensuring UF prefix consistency.
func ValidateTopicRegion(uf string, municipalityIBGE *int) error {
	trimmedUF := strings.TrimSpace(uf)
	if trimmedUF == "" && municipalityIBGE == nil {
		return nil
	}

	expectedPrefix := 0
	if trimmedUF != "" {
		prefix, ok := validUFs[trimmedUF]
		if !ok {
			return ErrInvalidRegion
		}
		expectedPrefix = prefix
	}

	if municipalityIBGE != nil {
		code := *municipalityIBGE
		if code < 1000000 || code > 9999999 {
			return ErrInvalidRegion
		}
		if expectedPrefix > 0 {
			actualPrefix := code / 100000
			if actualPrefix != expectedPrefix {
				return ErrInvalidRegion
			}
		}
	}

	return nil
}

// ValidateTopic validates complete topic metadata, category, title, body, and optional subject tags.
func ValidateTopic(
	category Category,
	title, body string,
	uf string,
	municipalityIBGE *int,
	fuelProduct *string,
	stationID *string,
) error {
	if _, err := ParseCategory(string(category)); err != nil {
		return err
	}
	if _, err := ValidateTopicTitle(title); err != nil {
		return err
	}
	if _, err := ValidateTopicBody(body); err != nil {
		return err
	}
	if err := ValidateTopicRegion(uf, municipalityIBGE); err != nil {
		return err
	}
	if fuelProduct != nil && *fuelProduct != "" {
		if !validFuelProducts[*fuelProduct] {
			return fmt.Errorf("%w: unknown fuel %s", ErrInvalidFuelProduct, *fuelProduct)
		}
	}
	if stationID != nil && *stationID != "" {
		if !uuidRegex.MatchString(*stationID) {
			return fmt.Errorf("%w: %s", ErrInvalidStationID, *stationID)
		}
	}
	return nil
}
