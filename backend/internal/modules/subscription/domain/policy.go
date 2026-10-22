// Package domain owns Premium rights independently of community trust.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type State string

const (
	None              State = "NONE"
	Pending           State = "PENDING"
	Active            State = "ACTIVE"
	Canceled          State = "CANCELED"
	Grace             State = "GRACE"
	Hold              State = "HOLD"
	Paused            State = "PAUSED"
	Expired           State = "EXPIRED"
	Revoked           State = "REVOKED"
	Unknown           State = "UNKNOWN"
	ProofFreshness          = 15 * time.Minute
	VerificationLease       = 20 * time.Second
	ProviderTimeout         = 5 * time.Second
	MaxTokenBytes           = 4096
)

var (
	ErrInvalid      = errors.New("subscription: invalid input")
	ErrUnauthorized = errors.New("subscription: authentication required")
	ErrConflict     = errors.New("subscription: operation conflict")
	ErrBusy         = errors.New("subscription: verification busy")
	ErrUnavailable  = errors.New("subscription: provider unavailable")
	ErrDenied       = errors.New("subscription: proof denied")
	ErrStale        = errors.New("subscription: stale verification")
	ErrNotFound     = errors.New("subscription: purchase unavailable")
	ErrQuota        = errors.New("subscription: admission exhausted")
)

type Snapshot struct {
	State        State
	ValidUntil   time.Time
	VerifiedAt   time.Time
	ProductID    string
	BasePlanID   string
	Retired      bool
	Acknowledged bool
}
type Status struct {
	PolicyVersion      int        `json:"policy_version"`
	State              State      `json:"state"`
	Reason             string     `json:"reason"`
	Capabilities       []string   `json:"capabilities"`
	ValidUntil         *time.Time `json:"valid_until"`
	RetainedDataAccess bool       `json:"retained_data_access"`
	PaymentsConfigured bool       `json:"payments_configured"`
}

func Evaluate(s *Snapshot, now time.Time, live bool) Status {
	result := Status{PolicyVersion: 1, State: None, Reason: "no-purchase", Capabilities: []string{}, RetainedDataAccess: true}
	if !live {
		result.Reason = "account-inactive"
		return result
	}
	if s == nil {
		return result
	}
	result.State = s.State
	if !ValidState(s.State) {
		result.State = Unknown
	}
	if !s.ValidUntil.IsZero() {
		until := s.ValidUntil.UTC()
		result.ValidUntil = &until
	}
	if s.Retired {
		result.Reason = "purchase-retired"
		return result
	}
	if s.VerifiedAt.IsZero() || now.Before(s.VerifiedAt) || now.Sub(s.VerifiedAt) >= ProofFreshness {
		result.Reason = "proof-stale"
		return result
	}
	if !now.Before(s.ValidUntil) {
		result.Reason = "access-expired"
		return result
	}
	switch s.State {
	case Active, Canceled, Grace:
		result.Reason = "verified"
		result.Capabilities = []string{"cloud.sync", "cloud.backup", "cloud.restore", "alerts.advanced", "professional.write", "professional.report", "vehicles.multiple"}
	default:
		result.Reason = "state-ineligible"
	}
	return result
}

func ValidState(s State) bool {
	switch s {
	case None, Pending, Active, Canceled, Grace, Hold, Paused, Expired, Revoked, Unknown:
		return true
	}
	return false
}
func ValidID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil
}
func ValidToken(s string) bool {
	if len(s) == 0 || len(s) > MaxTokenBytes {
		return false
	}
	for _, c := range s {
		if c <= 32 || c > 126 {
			return false
		}
	}
	return true
}
func TokenDigest(environment, packageName, token string) string {
	sum := sha256.Sum256([]byte("play\x00" + environment + "\x00" + packageName + "\x00" + token))
	return hex.EncodeToString(sum[:])
}
