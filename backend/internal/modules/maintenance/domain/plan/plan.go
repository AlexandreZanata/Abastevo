// Package plan implements pure maintenance plan trigger computation, work-order
// line reconciliation, unavailability window merging, and expiry evaluation
// (EMP-MAINT-001, EMP-MAINT-002, EMP-MAINT-004..007, BR-MAINT-001..046, VAL-31).
package plan

import (
	"errors"
	"math/big"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidInput    = errors.New("maintenance: validation failed")
	ErrStateTransition = errors.New("maintenance: transition not allowed")
	ErrSelfApproval    = errors.New("maintenance: requester or selector cannot review")
	ErrOverrunDenied   = errors.New("maintenance: overrun exceeds approved ceiling")
	ErrHoldOpen        = errors.New("maintenance: asset hold is still open")
	ErrQuoteExpired    = errors.New("maintenance: selected quote has expired")
	ErrInvoiceRequired = errors.New("maintenance: supplier invoice required before closing")
)

type DueState string

const (
	StateUpcoming DueState = "UPCOMING"
	StateDue      DueState = "DUE"
	StateOverdue  DueState = "OVERDUE"
	StateDone     DueState = "DONE"
	StateSkipped  DueState = "SKIPPED"
)

func severity(s DueState) int {
	switch s {
	case StateOverdue:
		return 3
	case StateDue:
		return 2
	case StateUpcoming:
		return 1
	default:
		return 0
	}
}

type TriggerKind string

const (
	TriggerNone     TriggerKind = "NONE"
	TriggerDistance TriggerKind = "DISTANCE"
	TriggerHours    TriggerKind = "HOURS"
	TriggerTime     TriggerKind = "TIME"
)

type RebaseRule string

const (
	RebaseActual    RebaseRule = "ACTUAL"
	RebaseScheduled RebaseRule = "SCHEDULED"
)

type PlanSpec struct {
	DistanceIntervalM  *int64
	DistanceLeadM      *int64
	HourIntervalS      *int64
	HourLeadS          *int64
	TimeIntervalDays   *int
	TimeIntervalMonths *int
	TimeLeadDays       *int
	BaselineDay        time.Time
	BaselineOdometerM  *int64
	BaselineHourS      *int64
	Rebase             RebaseRule
}

type PlanEvalInput struct {
	Plan            PlanSpec
	EvalDay         time.Time
	CurrentOdoM     *int64
	CurrentHourS    *int64
	WindowStartDay  *time.Time
	WindowStartOdoM *int64
}

type PlanEvalResult struct {
	State           DueState
	DecidingTrigger TriggerKind
	UsedM           *int64
	RemainingM      *int64
	UsedS           *int64
	RemainingS      *int64
	DueDay          *time.Time
	DaysToDue       *int
	ProjectedDueDay *time.Time
}

func (s PlanSpec) Validate() error {
	hasTrigger := s.DistanceIntervalM != nil || s.HourIntervalS != nil || s.TimeIntervalDays != nil || s.TimeIntervalMonths != nil
	if !hasTrigger || s.BaselineDay.IsZero() {
		return ErrInvalidInput
	}
	if s.DistanceIntervalM != nil {
		if s.DistanceLeadM == nil || *s.DistanceIntervalM <= 0 || *s.DistanceLeadM <= 0 || *s.DistanceLeadM > *s.DistanceIntervalM {
			return ErrInvalidInput
		}
		if s.BaselineOdometerM == nil {
			return ErrInvalidInput
		}
	}
	if s.HourIntervalS != nil {
		if s.HourLeadS == nil || *s.HourIntervalS <= 0 || *s.HourLeadS <= 0 || *s.HourLeadS > *s.HourIntervalS {
			return ErrInvalidInput
		}
		if s.BaselineHourS == nil {
			return ErrInvalidInput
		}
	}
	if (s.TimeIntervalDays != nil || s.TimeIntervalMonths != nil) && (s.TimeLeadDays == nil || *s.TimeLeadDays <= 0) {
		return ErrInvalidInput
	}
	return nil
}

func AddIntervalDay(base time.Time, months *int, days *int) time.Time {
	d := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.UTC)
	if months != nil && *months > 0 {
		d = d.AddDate(0, *months, 0)
	}
	if days != nil && *days > 0 {
		d = d.AddDate(0, 0, *days)
	}
	return d
}

// Evaluate computes the earliest trigger across distance, hours, and calendar time.
func Evaluate(in PlanEvalInput) (PlanEvalResult, error) {
	if err := in.Plan.Validate(); err != nil || in.EvalDay.IsZero() {
		return PlanEvalResult{}, ErrInvalidInput
	}
	out := PlanEvalResult{State: StateUpcoming, DecidingTrigger: TriggerNone}
	evalDate := time.Date(in.EvalDay.Year(), in.EvalDay.Month(), in.EvalDay.Day(), 0, 0, 0, 0, time.UTC)

	advance := func(st DueState, trig TriggerKind) {
		if severity(st) > severity(out.State) {
			out.State = st
			if st == StateUpcoming {
				out.DecidingTrigger = TriggerNone
			} else {
				out.DecidingTrigger = trig
			}
		}
	}

	if in.Plan.DistanceIntervalM != nil && in.CurrentOdoM != nil && in.Plan.BaselineOdometerM != nil {
		used := *in.CurrentOdoM - *in.Plan.BaselineOdometerM
		rem := *in.Plan.DistanceIntervalM - used
		out.UsedM = &used
		out.RemainingM = &rem
		st := StateUpcoming
		if used > *in.Plan.DistanceIntervalM {
			st = StateOverdue
		} else if rem <= *in.Plan.DistanceLeadM {
			st = StateDue
		}
		advance(st, TriggerDistance)
		if rem > 0 && in.WindowStartDay != nil && in.WindowStartOdoM != nil {
			winDate := time.Date(in.WindowStartDay.Year(), in.WindowStartDay.Month(), in.WindowStartDay.Day(), 0, 0, 0, 0, time.UTC)
			elapsedDays := int64(evalDate.Sub(winDate) / (24 * time.Hour))
			winUsage := *in.CurrentOdoM - *in.WindowStartOdoM
			if elapsedDays > 0 && winUsage > 0 {
				num := new(big.Int).Mul(big.NewInt(rem), big.NewInt(elapsedDays))
				den := big.NewInt(winUsage)
				ceilDays := new(big.Int).Add(num, new(big.Int).Sub(den, big.NewInt(1)))
				ceilDays.Quo(ceilDays, den)
				if ceilDays.IsInt64() {
					proj := evalDate.AddDate(0, 0, int(ceilDays.Int64()))
					out.ProjectedDueDay = &proj
				}
			}
		}
	}

	if in.Plan.HourIntervalS != nil && in.CurrentHourS != nil && in.Plan.BaselineHourS != nil {
		used := *in.CurrentHourS - *in.Plan.BaselineHourS
		rem := *in.Plan.HourIntervalS - used
		out.UsedS = &used
		out.RemainingS = &rem
		st := StateUpcoming
		if used > *in.Plan.HourIntervalS {
			st = StateOverdue
		} else if rem <= *in.Plan.HourLeadS {
			st = StateDue
		}
		advance(st, TriggerHours)
	}

	if in.Plan.TimeIntervalDays != nil || in.Plan.TimeIntervalMonths != nil {
		due := AddIntervalDay(in.Plan.BaselineDay, in.Plan.TimeIntervalMonths, in.Plan.TimeIntervalDays)
		out.DueDay = &due
		daysToDue := int(due.Sub(evalDate) / (24 * time.Hour))
		out.DaysToDue = &daysToDue
		st := StateUpcoming
		if daysToDue < 0 {
			st = StateOverdue
		} else if daysToDue <= *in.Plan.TimeLeadDays {
			st = StateDue
		}
		advance(st, TriggerTime)
	}

	return out, nil
}

// RebaseNextMeter returns the next due value under ACTUAL vs SCHEDULED rebase (BR-MAINT-006).
func RebaseNextMeter(scheduledDue, completedMeter, interval int64, rule RebaseRule) int64 {
	if rule == RebaseScheduled {
		return scheduledDue + interval
	}
	return completedMeter + interval
}

type LineKind string

const (
	LinePart    LineKind = "PART"
	LineService LineKind = "SERVICE"
	LineLabor   LineKind = "LABOR"
)

type WorkOrderLine struct {
	Kind           LineKind
	QuantityMilli  int64 // parts/service milli-units OR labor seconds
	UnitPriceMilli int64 // milli-BRL per unit or per hour
}

type WorkOrderReconciliation struct {
	PartsCentavos      int64
	ServiceCentavos    int64
	SumCentavos        int64
	OverrunCeiling     int64
	ArithmeticMismatch bool
	OverrunExceeded    bool
}

func roundHalfUp(num, den int64) int64 {
	bNum := big.NewInt(num)
	bDen := big.NewInt(den)
	half := new(big.Int).Rsh(bDen, 1)
	return new(big.Int).Quo(new(big.Int).Add(bNum, half), bDen).Int64()
}

// EvaluateWorkOrder reconciles executed parts/service/labor lines, discount, paid total,
// and overrun tolerance (BR-MAINT-024..029, maintenance.md Example 3).
func EvaluateWorkOrder(lines []WorkOrderLine, approvedCents int64, tolerancePct int, discountCents, paidCents int64) (WorkOrderReconciliation, error) {
	if approvedCents < 0 || tolerancePct < 0 || tolerancePct > 100 || discountCents < 0 || paidCents < 0 {
		return WorkOrderReconciliation{}, ErrInvalidInput
	}
	var partsCents, serviceCents int64
	for _, l := range lines {
		if l.QuantityMilli <= 0 || l.UnitPriceMilli < 0 {
			return WorkOrderReconciliation{}, ErrInvalidInput
		}
		switch l.Kind {
		case LinePart:
			partsCents += roundHalfUp(l.QuantityMilli*l.UnitPriceMilli, 10000)
		case LineService:
			serviceCents += roundHalfUp(l.QuantityMilli*l.UnitPriceMilli, 10000)
		case LineLabor:
			serviceCents += roundHalfUp(l.QuantityMilli*l.UnitPriceMilli, 36000)
		default:
			return WorkOrderReconciliation{}, ErrInvalidInput
		}
	}
	sumCents := partsCents + serviceCents
	ceiling := approvedCents + (approvedCents*int64(tolerancePct))/100
	return WorkOrderReconciliation{
		PartsCentavos:      partsCents,
		ServiceCentavos:    serviceCents,
		SumCentavos:        sumCents,
		OverrunCeiling:     ceiling,
		ArithmeticMismatch: paidCents+discountCents != sumCents,
		OverrunExceeded:    paidCents > ceiling,
	}, nil
}

type WorkOrderState string

const (
	WOStateDraft       WorkOrderState = "DRAFT"
	WOStateRequested   WorkOrderState = "REQUESTED"
	WOStateQuoted      WorkOrderState = "QUOTED"
	WOStateApproved    WorkOrderState = "APPROVED"
	WOStateScheduled   WorkOrderState = "SCHEDULED"
	WOStateInExecution WorkOrderState = "IN_EXECUTION"
	WOStateCompleted   WorkOrderState = "COMPLETED"
	WOStateClosed      WorkOrderState = "CLOSED"
	WOStateRejected    WorkOrderState = "REJECTED"
	WOStateCancelled   WorkOrderState = "CANCELLED"
)

var allowedWOTransitions = map[WorkOrderState][]WorkOrderState{
	WOStateDraft:       {WOStateRequested, WOStateCancelled},
	WOStateRequested:   {WOStateQuoted, WOStateApproved, WOStateRejected, WOStateCancelled},
	WOStateQuoted:      {WOStateQuoted, WOStateApproved, WOStateRejected, WOStateCancelled},
	WOStateApproved:    {WOStateScheduled, WOStateInExecution, WOStateCancelled},
	WOStateScheduled:   {WOStateApproved, WOStateInExecution, WOStateCancelled},
	WOStateInExecution: {WOStateCompleted},
	WOStateCompleted:   {WOStateInExecution, WOStateClosed},
}

func CanTransitionWorkOrder(from, to WorkOrderState) bool {
	for _, allowed := range allowedWOTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// ValidateReview checks separation of duties on work order approvals (BR-REV-003).
func ValidateReview(requesterID, reviewerID string, quoteValidUntil *time.Time, now time.Time) error {
	if strings.TrimSpace(reviewerID) == "" || requesterID == reviewerID {
		return ErrSelfApproval
	}
	if quoteValidUntil != nil {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		valid := time.Date(quoteValidUntil.Year(), quoteValidUntil.Month(), quoteValidUntil.Day(), 0, 0, 0, 0, time.UTC)
		if today.After(valid) {
			return ErrQuoteExpired
		}
	}
	return nil
}

type HoldWindow struct {
	Start time.Time
	End   time.Time
}

// UnavailableSeconds merges overlapping maintenance hold intervals so each second
// of unavailability counts at most once (BR-MAINT-032).
func UnavailableSeconds(windows []HoldWindow, periodStart, periodEnd time.Time) int64 {
	if !periodEnd.After(periodStart) || len(windows) == 0 {
		return 0
	}
	var clipped []HoldWindow
	for _, w := range windows {
		s := w.Start
		if s.Before(periodStart) {
			s = periodStart
		}
		e := w.End
		if e.After(periodEnd) {
			e = periodEnd
		}
		if e.After(s) {
			clipped = append(clipped, HoldWindow{Start: s, End: e})
		}
	}
	if len(clipped) == 0 {
		return 0
	}
	sort.Slice(clipped, func(i, j int) bool {
		return clipped[i].Start.Before(clipped[j].Start)
	})
	total := int64(0)
	cur := clipped[0]
	for _, next := range clipped[1:] {
		if !next.Start.After(cur.End) {
			if next.End.After(cur.End) {
				cur.End = next.End
			}
		} else {
			total += int64(cur.End.Sub(cur.Start) / time.Second)
			cur = next
		}
	}
	total += int64(cur.End.Sub(cur.Start) / time.Second)
	return total
}

// EvaluateCompliance evaluates expiry state for asset documents or driver licences (BR-MAINT-038).
func EvaluateCompliance(expiryDay, evalDay time.Time, leadDays int) (DueState, int) {
	exp := time.Date(expiryDay.Year(), expiryDay.Month(), expiryDay.Day(), 0, 0, 0, 0, time.UTC)
	ev := time.Date(evalDay.Year(), evalDay.Month(), evalDay.Day(), 0, 0, 0, 0, time.UTC)
	daysLeft := int(exp.Sub(ev) / (24 * time.Hour))
	switch {
	case daysLeft < 0:
		return StateOverdue, daysLeft
	case daysLeft <= leadDays:
		return StateDue, daysLeft
	default:
		return StateUpcoming, daysLeft
	}
}
