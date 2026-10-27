package domain

import "time"

// Grant is one admin-created tier assignment. Capabilities are never stored;
// they are derived from Tier and PolicyVersion.
type Grant struct {
	ID               string
	AccountID        string
	Tier             Tier
	PolicyVersion    int
	State            State
	StartAt          time.Time
	EndAt            *time.Time
	Reason           string
	CreatedAt        time.Time
	CreatedSessionID string
	RevokedAt        *time.Time
	RevokeReason     string
}

// NewGrant is one validated creation intent. StartAt is always server time.
type NewGrant struct {
	ID               string
	AccountID        string
	Tier             Tier
	EndAt            *time.Time
	Reason           string
	CreatedSessionID string
	Now              time.Time
}

// ActiveAt reports the ENT-06 evaluation window.
func (g Grant) ActiveAt(now time.Time) bool {
	return g.State == StateActive && !g.StartAt.After(now) && (g.EndAt == nil || g.EndAt.After(now))
}

// GrantFilter bounds one admin list read.
type GrantFilter struct {
	Tier          Tier
	State         State
	BeforeCreated time.Time
	BeforeID      string
	Limit         int
}

// ExpiredRevokeReason marks a lapsed ACTIVE grant revoked because a new
// grant for the same tier supersedes it.
const ExpiredRevokeReason = "expired"
