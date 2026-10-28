// Package domain owns the frozen staff role catalog. Roles grant no
// permission by themselves; consumers must implement their own checks.
package domain

import (
	"errors"
	"time"
)

const CatalogVersion = "staff-roles-v1"

var (
	ErrInvalid        = errors.New("staff: invalid input")
	ErrIneligible     = errors.New("staff: account-ineligible")
	ErrConflict       = errors.New("staff: exists")
	ErrNotFound       = errors.New("staff: not-found")
	ErrAlreadyRevoked = errors.New("staff: already-revoked")
	ErrUnavailable    = errors.New("staff: unavailable")
)

type Role string

const (
	RoleCommunityModerator Role = "community_moderator"
	RoleStationReviewer    Role = "station_reviewer"
	RoleContributorCurator Role = "contributor_curator"
	RoleSupportAgent       Role = "support_agent"
)

type RoleInfo struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

var catalog = []RoleInfo{
	{Code: string(RoleCommunityModerator), Label: "Moderação da comunidade"},
	{Code: string(RoleStationReviewer), Label: "Revisão de postos"},
	{Code: string(RoleContributorCurator), Label: "Curadoria de contribuidores"},
	{Code: string(RoleSupportAgent), Label: "Suporte"},
}

func Catalog() []RoleInfo { return append([]RoleInfo{}, catalog...) }

func ValidRole(raw string) bool {
	for _, info := range catalog {
		if info.Code == raw {
			return true
		}
	}
	return false
}

func Label(raw string) string {
	for _, info := range catalog {
		if info.Code == raw {
			return info.Label
		}
	}
	return ""
}

const (
	MinAssignmentDuration = time.Hour
	MaxAssignmentDuration = 5 * 365 * 24 * time.Hour
)

func ValidEndAt(now time.Time, end *time.Time) bool {
	if end == nil {
		return true
	}
	value := end.UTC()
	return !value.Before(now.Add(MinAssignmentDuration)) && !value.After(now.Add(MaxAssignmentDuration))
}

type State string

const (
	StateActive  State = "ACTIVE"
	StateRevoked State = "REVOKED"
)

type Assignment struct {
	ID               string     `json:"id"`
	AccountID        string     `json:"account_id"`
	Role             string     `json:"role"`
	Label            string     `json:"label"`
	State            string     `json:"state"`
	StartAt          time.Time  `json:"start_at"`
	EndAt            *time.Time `json:"end_at"`
	Reason           string     `json:"reason"`
	CreatedAt        time.Time  `json:"created_at"`
	CreatedSessionID string     `json:"-"`
	RevokedAt        *time.Time `json:"revoked_at"`
	RevokeReason     string     `json:"revoke_reason,omitempty"`
}

// ExpiredRevokeReason marks a lapsed ACTIVE assignment revoked because the
// same role is assigned again.
const ExpiredRevokeReason = "expired"
