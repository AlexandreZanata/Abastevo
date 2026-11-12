package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrAssignmentInvalid  = errors.New("orgaccess: invalid assignment")
	ErrAssignmentScope    = errors.New("orgaccess: assignment scope denied")
	ErrAssignmentConflict = errors.New("orgaccess: assignment conflict")
	ErrAssignmentEnded    = errors.New("orgaccess: assignment ended")
)

type ScopeType string

const (
	OrgScope  ScopeType = "ORG"
	SelfScope ScopeType = "SELF"
)

type privateAssignment struct{}

func (privateAssignment) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "[private role assignment]")
}
func (privateAssignment) MarshalJSON() ([]byte, error) {
	return json.Marshal("[private role assignment]")
}
func (privateAssignment) LogValue() slog.Value { return slog.StringValue("[private role assignment]") }

// AssignmentSnapshot is trusted persistence data, never a wire DTO or proof.
// Zero ValidTo means unbounded. EndedAt cancels even a not-yet-started period.
type AssignmentSnapshot struct {
	privateAssignment
	OrgID                        kernel.OrgID
	ID, MembershipID, AssignedBy string
	Role                         Role
	Scope                        ScopeType
	TargetID                     string
	ValidFrom, ValidTo, EndedAt  time.Time
	Reason                       string
	Revision                     int64
}
type Assignment struct {
	privateAssignment
	value AssignmentSnapshot
}

func RehydrateAssignment(scope kernel.OrgScope, v AssignmentSnapshot) (Assignment, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return Assignment{}, ErrAssignmentScope
	}
	for _, raw := range []string{v.ID, v.MembershipID, v.AssignedBy, v.TargetID} {
		if _, err := kernel.ParseOrgID(raw); err != nil {
			return Assignment{}, ErrAssignmentInvalid
		}
	}
	if _, err := SystemCatalog().Rules(v.Role); err != nil {
		return Assignment{}, ErrUnknownRole
	}
	if v.Scope != OrgScope && v.Scope != SelfScope {
		return Assignment{}, ErrAssignmentScope
	}
	if (v.Scope == OrgScope && v.TargetID != scope.OrgID().String()) || (v.Scope == SelfScope && v.TargetID != v.MembershipID) || (v.Role == Owner && v.Scope != OrgScope) {
		return Assignment{}, ErrAssignmentScope
	}
	if v.Role == StationAdmin || v.Role == StationManager || v.Role == StationAttendant {
		return Assignment{}, ErrAssignmentScope
	}
	if v.ValidFrom.IsZero() || (!v.ValidTo.IsZero() && !v.ValidFrom.Before(v.ValidTo)) || v.Revision < 1 || (v.EndedAt.IsZero() != (v.Revision == 1)) || !utf8.ValidString(v.Reason) || strings.TrimSpace(v.Reason) == "" || utf8.RuneCountInString(v.Reason) > 280 {
		return Assignment{}, ErrAssignmentInvalid
	}
	for _, r := range v.Reason {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return Assignment{}, ErrAssignmentInvalid
		}
	}
	v.ValidFrom = v.ValidFrom.UTC()
	if !v.ValidTo.IsZero() {
		v.ValidTo = v.ValidTo.UTC()
	}
	if !v.EndedAt.IsZero() {
		v.EndedAt = v.EndedAt.UTC()
	}
	return Assignment{value: v}, nil
}
func (a Assignment) Snapshot(scope kernel.OrgScope) (AssignmentSnapshot, error) {
	if _, err := RehydrateAssignment(scope, a.value); err != nil {
		return AssignmentSnapshot{}, err
	}
	return a.value, nil
}

// ValidAt checks only this period. Membership, account, organization and actor
// checks remain server policy; this result never authorizes a request.
func (a Assignment) ValidAt(scope kernel.OrgScope, at time.Time) (bool, error) {
	v, err := a.Snapshot(scope)
	if err != nil {
		return false, err
	}
	if at.IsZero() {
		return false, ErrAssignmentInvalid
	}
	return !at.Before(v.ValidFrom) && (v.ValidTo.IsZero() || at.Before(v.ValidTo)) && (v.EndedAt.IsZero() || at.Before(v.EndedAt)), nil
}

// End creates a proposed terminal revision; storage supplies its server time.
func (a Assignment) End(scope kernel.OrgScope, expected int64, at time.Time) (Assignment, error) {
	v, err := a.Snapshot(scope)
	if err != nil {
		return Assignment{}, err
	}
	if !v.EndedAt.IsZero() {
		return Assignment{}, ErrAssignmentEnded
	}
	if expected != v.Revision {
		return Assignment{}, ErrAssignmentConflict
	}
	if at.IsZero() || expected == math.MaxInt64 {
		return Assignment{}, ErrAssignmentInvalid
	}
	v.EndedAt = at
	v.Revision++
	return RehydrateAssignment(scope, v)
}
