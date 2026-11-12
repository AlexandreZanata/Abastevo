package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"time"
)

func (o Organization) SetSuspension(scope kernel.OrgScope, expected int64, reason Reason) (Organization, error) {
	if e := o.changing(scope, expected); e != nil {
		return Organization{}, e
	}
	if !validReason(reason) {
		return Organization{}, ErrInvalid
	}
	if o.value.SuspensionReason != "" {
		return Organization{}, ErrTransition
	}
	if o.value.State == Active {
		return o.Transition(scope, expected, Suspended, Conditions{SuspensionReason: reason})
	}
	if o.value.State != Archived {
		return Organization{}, ErrTransition
	}
	result := o
	result.value.SuspensionReason = reason
	result.value.Revision++
	return result, nil
}
func (o Organization) ResolveSuspension(scope kernel.OrgScope, expected int64, reason Reason) (Organization, error) {
	if e := o.changing(scope, expected); e != nil {
		return Organization{}, e
	}
	if !validReason(reason) {
		return Organization{}, ErrInvalid
	}
	if o.value.SuspensionReason != reason {
		return Organization{}, ErrConflict
	}
	if o.value.State == Suspended {
		return o.Transition(scope, expected, Active, Conditions{SuspensionResolved: true})
	}
	if o.value.State != Archived {
		return Organization{}, ErrTransition
	}
	result := o
	result.value.SuspensionReason = ""
	result.value.Revision++
	return result, nil
}

const SuspensionGrace = 7 * 24 * time.Hour
const MaxAssessedSuspensions = 100

type SuspensionPeriod struct {
	privateSettings
	StartedAt, EndedAt time.Time
}
type SuspensionDisposition string

const (
	SuspensionAllow  SuspensionDisposition = "ALLOW"
	SuspensionReview SuspensionDisposition = "REVIEW"
	SuspensionRefuse SuspensionDisposition = "REFUSE"
)

type SuspensionIntakeFacts struct {
	privateSettings
	DeclaredAt, ReceivedAt, ServerOpenedAt time.Time
}
type SuspensionAssessment struct {
	Disposition SuspensionDisposition
	Code        string
}

// AssessSuspension classifies only this lifecycle condition. The owning command
// still verifies device, membership, permission, replay, schema and intake window.
// ServerOpenedAt is trusted persisted entity history, never an HTTP DTO field.
func AssessSuspension(state State, periods []SuspensionPeriod, f SuspensionIntakeFacts) (SuspensionAssessment, error) {
	if !validState(state) || !settingsInstant(f.DeclaredAt) || !settingsInstant(f.ReceivedAt) || len(periods) > MaxAssessedSuspensions {
		return SuspensionAssessment{}, ErrInvalid
	}
	if !f.ServerOpenedAt.IsZero() && (!settingsInstant(f.ServerOpenedAt) || f.ServerOpenedAt.After(f.ReceivedAt)) {
		return SuspensionAssessment{}, ErrInvalid
	}
	open := false
	var previousEnd time.Time
	for i, p := range periods {
		if !settingsInstant(p.StartedAt) || p.StartedAt.After(f.ReceivedAt) || i > 0 && (previousEnd.IsZero() || p.StartedAt.Before(previousEnd)) {
			return SuspensionAssessment{}, ErrConditions
		}
		if !p.EndedAt.IsZero() && (!settingsInstant(p.EndedAt) || p.EndedAt.Before(p.StartedAt) || p.EndedAt.After(f.ReceivedAt)) {
			return SuspensionAssessment{}, ErrConditions
		}
		previousEnd = p.EndedAt
		open = p.EndedAt.IsZero()
	}
	if state == Suspended && !open || state == Active && open {
		return SuspensionAssessment{}, ErrConditions
	}
	switch state {
	case Closed:
		return SuspensionAssessment{SuspensionRefuse, "NOT_FOUND"}, nil
	case PendingSetup:
		return SuspensionAssessment{SuspensionRefuse, "ORG_SETUP_INCOMPLETE"}, nil
	case Archived:
		return SuspensionAssessment{SuspensionRefuse, "ORG_ARCHIVED"}, nil
	}
	result := SuspensionAssessment{Disposition: SuspensionAllow}
	for _, p := range periods {
		// Closed periods wholly before this fact do not constrain new work.
		if !p.EndedAt.IsZero() && !f.DeclaredAt.Before(p.EndedAt) && (f.ServerOpenedAt.IsZero() || !f.ServerOpenedAt.Before(p.EndedAt)) {
			continue
		}
		opened := !f.ServerOpenedAt.IsZero() && f.ServerOpenedAt.Before(p.StartedAt)
		inGrace := f.DeclaredAt.Before(p.StartedAt) && !f.ReceivedAt.After(p.StartedAt.Add(SuspensionGrace))
		// A fact recorded during a now-resolved interval still needs review; current
		// permission never certifies what happened while the cause was open.
		duringResolved := !p.EndedAt.IsZero() && !f.DeclaredAt.Before(p.StartedAt) && f.DeclaredAt.Before(p.EndedAt)
		if !opened && !inGrace && !duringResolved {
			return SuspensionAssessment{SuspensionRefuse, "ORG_SUSPENDED"}, nil
		}
		result = SuspensionAssessment{SuspensionReview, "ORG_SUSPENDED_GRACE"}
	}
	return result, nil
}
