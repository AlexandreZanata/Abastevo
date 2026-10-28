package domain

import (
	"encoding/hex"
	"strings"
)

// ValidUUID is the canonical lowercase UUID check for path and operation IDs.
func ValidUUID(raw string) bool {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' || strings.ToLower(raw) != raw {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(raw, "-", ""))
	return err == nil
}
