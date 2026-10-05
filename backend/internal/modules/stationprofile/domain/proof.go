package domain

import (
	"bytes"
	"errors"
	"strings"
	"time"
)

// Frozen proof-byte policy (P30-T01 contract freeze): PDF only,
// 5 MB cap, 24 h all-copy cap on embedded scans, multi-year audit
// retention on granted-case authorizations ([CALIBRATE: legal review]).
const (
	MaxProofBytes      = 5 << 20
	ScanRetention      = 24 * time.Hour
	AuthorizationYears = 2
)

// ExpiryForKind returns the retention for a proof kind, or 0 for
// unknown kinds (fail closed: intake refuses what it cannot retain).
func ExpiryForKind(kind string) time.Duration {
	switch kind {
	case "scan":
		return ScanRetention
	case "authorization", "mandate":
		return AuthorizationYears * 365 * 24 * time.Hour
	default:
		return 0
	}
}

var (
	ErrProofFormat = errors.New("stationprofile: unsupported proof format")
	ErrProofSize   = errors.New("stationprofile: proof size out of bounds")
	ErrProofActive = errors.New("stationprofile: active or encrypted content refused")
	ErrProofSecret = errors.New("stationprofile: key material refused")
)

// forbiddenMarkers refuse active content, encryption and embedded
// executables without parsing the document (no unbounded parser
// execution, no URL fetch).
var forbiddenMarkers = [][]byte{
	[]byte("/Encrypt"),
	[]byte("/JavaScript"),
	[]byte("/EmbeddedFiles"),
	[]byte("/Launch"),
	[]byte("/XFA"),
}

// secretMarkers refuse private keys, passwords and key bundles even
// inside a PDF container.
var secretMarkers = [][]byte{
	[]byte("private-key"),
	[]byte("private_key"),
	[]byte("BEGIN PRIVATE KEY"),
	[]byte("password"),
}

// ValidateProofBytes enforces the frozen intake policy on raw bytes.
// Magic mismatch, empty/oversize bodies, encrypted/active content and
// key material all fail here — never as an approval fallback.
func ValidateProofBytes(filename, contentType string, body []byte) error {
	lowerName := strings.ToLower(filename)
	if strings.HasSuffix(lowerName, ".pfx") || strings.HasSuffix(lowerName, ".p12") ||
		strings.HasSuffix(lowerName, ".pem") || strings.HasSuffix(lowerName, ".key") {
		return ErrProofSecret
	}
	if len(body) == 0 || len(body) > MaxProofBytes {
		return ErrProofSize
	}
	if !bytes.HasPrefix(body, []byte("%PDF-")) {
		return ErrProofFormat
	}
	lower := bytes.ToLower(body)
	for _, marker := range forbiddenMarkers {
		if bytes.Contains(lower, bytes.ToLower(marker)) {
			return ErrProofActive
		}
	}
	for _, marker := range secretMarkers {
		if bytes.Contains(lower, bytes.ToLower(marker)) {
			return ErrProofSecret
		}
	}
	_ = contentType
	return nil
}
