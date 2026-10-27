// Package domain owns the frozen granted-tier catalog. Capabilities are
// derived at evaluation time; no request or database row carries a capability
// array. The package performs no I/O.
package domain

import (
	"errors"
	"time"
)

const (
	CatalogVersion = "tier-catalog-v1"
	PolicyVersion  = 1
)

var (
	ErrInvalid        = errors.New("entitlement: invalid input")
	ErrIneligible     = errors.New("entitlement: account-ineligible")
	ErrExists         = errors.New("entitlement: exists")
	ErrNotFound       = errors.New("entitlement: not-found")
	ErrAlreadyRevoked = errors.New("entitlement: already-revoked")
	ErrUnavailable    = errors.New("entitlement: unavailable")
)

type Tier string

const (
	TierPRO         Tier = "PRO"
	TierContributor Tier = "CONTRIBUTOR"
	TierEnterprise  Tier = "ENTERPRISE"
)

type State string

const (
	StateActive  State = "ACTIVE"
	StateRevoked State = "REVOKED"
)

// PremiumCapabilities is the frozen PRO bundle. It is the same bundle paid
// Premium exposes through the subscription projection.
var PremiumCapabilities = []string{
	"cloud.sync",
	"cloud.backup",
	"cloud.restore",
	"alerts.advanced",
	"professional.write",
	"professional.report",
	"vehicles.multiple",
}

// ContributorCapabilities is the PRO bundle plus the public contributor
// profile capability.
var ContributorCapabilities = append(append([]string{}, PremiumCapabilities...), "contributor.profile")

// Capabilities returns a defensive copy of the tier bundle for one policy
// version. Unknown tiers or versions are refused.
func Capabilities(tier Tier, policyVersion int) ([]string, bool) {
	if policyVersion != PolicyVersion {
		return nil, false
	}
	var source []string
	switch tier {
	case TierPRO, TierEnterprise:
		source = PremiumCapabilities
	case TierContributor:
		source = ContributorCapabilities
	default:
		return nil, false
	}
	return append([]string{}, source...), true
}

func ValidTier(tier Tier) bool {
	switch tier {
	case TierPRO, TierContributor, TierEnterprise:
		return true
	}
	return false
}

func ParseTier(raw string) (Tier, bool) {
	tier := Tier(raw)
	return tier, ValidTier(tier)
}

// EndAtBounds are the ENT-04 limits around the server start time.
const (
	MinGrantDuration = time.Hour
	MaxGrantDuration = 5 * 365 * 24 * time.Hour
)

// ValidEndAt accepts nil (open-ended) or a bounded future end time.
func ValidEndAt(now time.Time, end *time.Time) bool {
	if end == nil {
		return true
	}
	value := end.UTC()
	return !value.Before(now.Add(MinGrantDuration)) && !value.After(now.Add(MaxGrantDuration))
}
