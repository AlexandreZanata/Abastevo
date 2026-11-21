package domain

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalidSubscription = errors.New("orgbilling: invalid subscription")
	ErrInvalidTransition   = errors.New("orgbilling: invalid subscription transition")
	ErrUnboundEvidence     = errors.New("orgbilling: unbound lifecycle evidence")
	ErrStaleSubscription   = errors.New("orgbilling: stale subscription revision")
)

type SubscriptionState string

const (
	Trialing   SubscriptionState = "TRIALING"
	Active     SubscriptionState = "ACTIVE"
	PastDue    SubscriptionState = "PAST_DUE"
	Suspended  SubscriptionState = "SUSPENDED"
	Cancelling SubscriptionState = "CANCELLING"
	Cancelled  SubscriptionState = "CANCELLED"
)

type EventKind string

const (
	StartTrial           EventKind = "START_TRIAL"
	ConfirmSettlement    EventKind = "CONFIRM_SETTLEMENT"
	ReportPastDue        EventKind = "REPORT_PAST_DUE"
	SuspendAccess        EventKind = "SUSPEND_ACCESS"
	RestoreAccess        EventKind = "RESTORE_ACCESS"
	ScheduleCancellation EventKind = "SCHEDULE_CANCELLATION"
	ConfirmCancellation  EventKind = "CONFIRM_CANCELLATION"
	FinishTerm           EventKind = "FINISH_TERM"
)

type EvidenceSource string

const (
	TrialPolicy      EvidenceSource = "TRIAL_POLICY"
	VerifiedBilling  EvidenceSource = "VERIFIED_BILLING"
	ReviewedOperator EvidenceSource = "REVIEWED_OPERATOR"
	ServerClock      EvidenceSource = "SERVER_CLOCK"
)

type Period struct {
	Start time.Time
	End   time.Time
}

func (period Period) contains(at time.Time) bool {
	return !period.Start.IsZero() && period.Start.Before(period.End) &&
		!at.Before(period.Start) && at.Before(period.End)
}

func (period Period) equal(other Period) bool {
	return period.Start.Equal(other.Start) && period.End.Equal(other.End)
}

// LifecycleEvent is a fact from a server authority. The application must
// verify its provenance and binding before constructing a fact for Apply.
type LifecycleEvent struct {
	Scope            kernel.OrgScope
	SubscriptionID   string
	ExpectedRevision int64
	Kind             EventKind
	Source           EvidenceSource
	At               time.Time
	Reference        string
	Period           Period
	PlanChange       PlanKey
}

type SubscriptionSnapshot struct {
	Scope          kernel.OrgScope
	SubscriptionID string
	Plan           PlanKey
	State          SubscriptionState
	PriorState     SubscriptionState
	Period         Period
	Revision       int64
	LastEventAt    time.Time
}

type Transition struct {
	Scope          kernel.OrgScope
	SubscriptionID string
	Revision       int64
	Kind           EventKind
	From           SubscriptionState
	To             SubscriptionState
	Source         EvidenceSource
	At             time.Time
	Reference      string
	Period         Period
}

type Subscription struct{ current SubscriptionSnapshot }

func NewSubscription(scope kernel.OrgScope, id string, plan PlanKey) (Subscription, error) {
	parsed, err := kernel.ParseOrgID(id)
	if scope.Validate() != nil || err != nil || parsed.String() != id || plan.Validate() != nil {
		return Subscription{}, ErrInvalidSubscription
	}
	return Subscription{current: SubscriptionSnapshot{Scope: scope, SubscriptionID: id, Plan: plan}}, nil
}

// RestoreSubscription validates a current snapshot read from the scoped ledger.
func RestoreSubscription(state SubscriptionSnapshot) (Subscription, error) {
	if _, err := NewSubscription(state.Scope, state.SubscriptionID, state.Plan); err != nil {
		return Subscription{}, err
	}
	if state.Revision < 1 || state.LastEventAt.IsZero() || state.Period.Start.IsZero() ||
		!state.Period.Start.Before(state.Period.End) || state.LastEventAt.Before(state.Period.Start) ||
		!state.validPriorState() {
		return Subscription{}, ErrInvalidSubscription
	}
	return Subscription{current: state}, nil
}

func (current SubscriptionSnapshot) validPriorState() bool {
	prior := current.PriorState
	switch current.State {
	case Trialing, Active, PastDue, Cancelled:
		return prior == ""
	case Suspended:
		return prior == Trialing || prior == Active || prior == PastDue
	case Cancelling:
		return prior == Trialing || prior == Active || prior == PastDue || prior == Suspended
	default:
		return false
	}
}

func (sub Subscription) Snapshot() SubscriptionSnapshot { return sub.current }

func (sub Subscription) Apply(event LifecycleEvent) (Subscription, Transition, error) {
	current := sub.current
	if current.Scope.Validate() != nil {
		return Subscription{}, Transition{}, ErrInvalidSubscription
	}
	if event.Scope.Validate() != nil || event.Scope.OrgID() != current.Scope.OrgID() ||
		event.SubscriptionID != current.SubscriptionID || !event.admittedSource() ||
		!validReference(event.Reference) || event.At.IsZero() || event.At.Before(current.LastEventAt) {
		return Subscription{}, Transition{}, ErrUnboundEvidence
	}
	if event.ExpectedRevision != current.Revision || current.Revision == math.MaxInt64 {
		return Subscription{}, Transition{}, ErrStaleSubscription
	}
	if event.Kind == StartTrial || event.Kind == ConfirmSettlement {
		if !event.Period.contains(event.At) {
			return Subscription{}, Transition{}, ErrInvalidTransition
		}
	} else if event.Period != (Period{}) {
		return Subscription{}, Transition{}, ErrInvalidTransition
	}
	if plan := event.PlanChange; plan != (PlanKey{}) {
		if event.Kind != ConfirmSettlement || current.Revision == 0 || plan.Validate() != nil ||
			plan.Product != current.Plan.Product || plan == current.Plan || !event.Period.Start.Equal(current.Period.End) {
			return Subscription{}, Transition{}, ErrInvalidTransition
		}
	}
	next, err := current.next(event)
	if err != nil {
		return Subscription{}, Transition{}, err
	}
	if event.PlanChange != (PlanKey{}) {
		next.Plan = event.PlanChange
	}
	next.Revision++
	next.LastEventAt = event.At.UTC()
	return Subscription{current: next}, Transition{
		Scope: next.Scope, SubscriptionID: next.SubscriptionID, Revision: next.Revision,
		Kind: event.Kind, From: current.State, To: next.State, Source: event.Source,
		At: next.LastEventAt, Reference: event.Reference, Period: next.Period,
	}, nil
}

func validReference(raw string) bool {
	return len(raw) > 0 && len(raw) <= 200 && !strings.ContainsFunc(raw, func(r rune) bool {
		return r < '!' || r > '~'
	})
}

func (event LifecycleEvent) admittedSource() bool {
	switch event.Kind {
	case StartTrial:
		return event.Source == TrialPolicy
	case ConfirmSettlement, ReportPastDue, ConfirmCancellation:
		return event.Source == VerifiedBilling
	case SuspendAccess, RestoreAccess:
		return event.Source == ReviewedOperator
	case ScheduleCancellation:
		return event.Source == ReviewedOperator || event.Source == VerifiedBilling
	case FinishTerm:
		return event.Source == ServerClock
	default:
		return false
	}
}

func (current SubscriptionSnapshot) next(event LifecycleEvent) (SubscriptionSnapshot, error) {
	next := current
	state := current.State
	switch event.Kind {
	case StartTrial:
		if state == "" {
			next.State, next.Period = Trialing, event.Period
			return next, nil
		}
	case ConfirmSettlement:
		if current.canSettle(event.Period) {
			next.State, next.Period = Active, event.Period
			return next, nil
		}
	case ReportPastDue:
		if state == Active {
			next.State = PastDue
			return next, nil
		}
	case SuspendAccess:
		if state == Trialing || state == Active || state == PastDue {
			next.State, next.PriorState = Suspended, state
			return next, nil
		}
	case RestoreAccess:
		if state == Suspended {
			next.State, next.PriorState = current.PriorState, ""
			return next, nil
		}
	case ScheduleCancellation:
		if state != "" && state != Cancelled && state != Cancelling && event.At.Before(current.Period.End) {
			next.State, next.PriorState = Cancelling, state
			return next, nil
		}
	case ConfirmCancellation:
		if state != "" && state != Cancelled {
			next.State, next.PriorState = Cancelled, ""
			return next, nil
		}
	case FinishTerm:
		if (state == Trialing || state == Cancelling) && !event.At.Before(current.Period.End) {
			next.State, next.PriorState = Cancelled, ""
			return next, nil
		}
	}
	return SubscriptionSnapshot{}, ErrInvalidTransition
}

func (current SubscriptionSnapshot) canSettle(paid Period) bool {
	switch current.State {
	case "":
		return true
	case Trialing:
		return !paid.Start.Before(current.Period.Start)
	case Active:
		return !paid.Start.Before(current.Period.End)
	case PastDue:
		return current.Period.equal(paid) || !paid.Start.Before(current.Period.End)
	default:
		return false
	}
}
