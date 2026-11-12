package domain

import (
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// structureText is the required-name rule for names and codes that must be
// unique inside an organization (BR-ORG-007, compared trimmed and
// case-insensitively). It also refuses invisible format characters and text
// not in NFC, so a look-alike cannot become a second, "different" value.
func structureText(raw string) (string, error) {
	value, err := legalName(raw, true)
	if err != nil || !norm.NFC.IsNormalString(value) {
		return "", ErrInvalid
	}
	for _, r := range value {
		if unicode.Is(unicode.Cf, r) {
			return "", ErrInvalid
		}
	}
	return value, nil
}

// Counts are resolved from source-owned, current transactional guards. A pure
// value cannot prove that an adapter observed actual usage or held its locks.
type StructureUsage struct{ Assets, Employees, Budgets, Operations int64 }

func (u StructureUsage) Validate() error {
	if u.Assets < 0 || u.Employees < 0 || u.Budgets < 0 || u.Operations < 0 {
		return ErrConditions
	}
	return nil
}
func (u StructureUsage) InUse() bool {
	return u.Assets > 0 || u.Employees > 0 || u.Budgets > 0 || u.Operations > 0
}
