package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxDeviceLabelBytes = 80

// Labels identify only this organization's enrolment. They are exact private
// metadata, never installation identity, proof, or an account/device inventory.
func ValidateDeviceLabel(label string) error {
	if label == "" || label != strings.TrimSpace(label) || !utf8.ValidString(label) || len(label) > MaxDeviceLabelBytes {
		return ErrInvalid
	}
	for _, r := range label {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ErrInvalid
		}
	}
	return nil
}
