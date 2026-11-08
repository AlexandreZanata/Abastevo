package domain

import (
	"strconv"
	"strings"
)

// ParseFeedUF normalizes and validates an optional Brazilian state filter.
func ParseFeedUF(raw string) (string, error) {
	uf := strings.ToUpper(strings.TrimSpace(raw))
	if uf == "" {
		return "", nil
	}
	if _, ok := validUFs[uf]; !ok {
		return "", ErrInvalidRegion
	}
	return uf, nil
}

// ParseFeedMunicipalityIBGE parses an optional seven-digit IBGE municipality filter.
func ParseFeedMunicipalityIBGE(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	if len(trimmed) != 7 {
		return 0, ErrInvalidRegion
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return 0, ErrInvalidRegion
		}
	}
	code, err := strconv.Atoi(trimmed)
	if err != nil || code < 1000000 || code > 9999999 {
		return 0, ErrInvalidRegion
	}
	return code, nil
}

// ValidateFeedRegion checks optional UF/IBGE consistency for discovery filters.
func ValidateFeedRegion(uf string, municipalityIBGE *int) error {
	return ValidateTopicRegion(uf, municipalityIBGE)
}

// ParseFeedFuelProduct normalizes and validates an optional fuel filter.
func ParseFeedFuelProduct(raw string) (string, error) {
	fuel := strings.TrimSpace(raw)
	if fuel == "" {
		return "", nil
	}
	if !validFuelProducts[fuel] {
		return "", ErrInvalidFuelProduct
	}
	return fuel, nil
}

// ParseFeedStationID normalizes and validates an optional station filter.
func ParseFeedStationID(raw string) (string, error) {
	station := strings.ToLower(strings.TrimSpace(raw))
	if station == "" {
		return "", nil
	}
	if !uuidRegex.MatchString(station) {
		return "", ErrInvalidStationID
	}
	return station, nil
}
