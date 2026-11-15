package domain

import (
	"math/big"
	"regexp"
	"time"

	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

// HoldState is the closed Budget hold vocabulary of the state catalog.
type HoldState string

const (
	HoldActive   HoldState = "ACTIVE"
	HoldConsumed HoldState = "CONSUMED"
	HoldReleased HoldState = "RELEASED"
	HoldExpired  HoldState = "EXPIRED"
)

func ParseHoldState(raw string) (HoldState, error) {
	switch s := HoldState(raw); s {
	case HoldActive, HoldConsumed, HoldReleased, HoldExpired:
		return s, nil
	}
	return "", ErrInvalid
}

// HoldInput restores one stored hold. ReservedAt is the server reservation
// instant; Zones come from the resolved operating context. ConsumedBy is
// required exactly when State is CONSUMED.
type HoldInput struct {
	redacted
	Reference  HoldRef
	Line       LineRef
	Amount     *string
	ReservedAt time.Time
	Zones      orgtime.ReportingZones
	Category   string
	Tags       Tags
	State      HoldState
	ConsumedBy *expense.ExpenseRef
}

// Hold is an immutable reservation fact. Its zero value is not a hold.
type Hold struct {
	redacted
	ref        HoldRef
	line       LineRef
	amount     int64
	reservedAt time.Time
	reporting  orgtime.ReportingAttribution
	category   expense.Category
	tags       Tags
	state      HoldState
	consumedBy expense.ExpenseRef
	valid      bool
}

// NewHold validates, in order, the reference, the named line, the amount
// (1..1000000000000), the nonzero instant and its reporting day, the category,
// the tags, the state and the consuming expense.
func NewHold(scope kernel.OrgScope, in HoldInput) (Hold, error) {
	if e := in.Reference.validate(scope); e != nil {
		return Hold{}, e
	}
	if e := in.Line.validate(scope); e != nil {
		return Hold{}, e
	}
	if in.Amount == nil {
		return Hold{}, ErrInvalid
	}
	amount, e := exact.ParseCentavos(*in.Amount)
	if e != nil || amount == 0 || in.ReservedAt.IsZero() {
		return Hold{}, ErrInvalid
	}
	reporting, e := orgtime.AttributeReportingDay(in.ReservedAt, in.Zones)
	if e != nil {
		return Hold{}, ErrInvalid
	}
	category, e := expense.ParseCategory(in.Category)
	if e != nil {
		return Hold{}, ErrInvalid
	}
	if !in.Tags.within(scope.OrgID()) {
		return Hold{}, ErrScope
	}
	if _, e := ParseHoldState(string(in.State)); e != nil {
		return Hold{}, e
	}
	out := Hold{ref: in.Reference, line: in.Line, amount: amount, reservedAt: in.ReservedAt.UTC(), reporting: reporting,
		category: category, tags: in.Tags, state: in.State, valid: true}
	if (in.State == HoldConsumed) != (in.ConsumedBy != nil) {
		return Hold{}, ErrInvalid
	}
	if in.ConsumedBy != nil {
		switch org := in.ConsumedBy.OrganizationID(); {
		case org == (kernel.OrgID{}):
			return Hold{}, ErrInvalid
		case org != scope.OrgID():
			return Hold{}, ErrScope
		}
		out.consumedBy = *in.ConsumedBy
	}
	return out, nil
}

func (h Hold) Reference() HoldRef                      { return h.ref }
func (h Hold) Line() LineRef                           { return h.line }
func (h Hold) AmountCentavos() int64                   { return h.amount }
func (h Hold) ReservedAt() time.Time                   { return h.reservedAt }
func (h Hold) Reporting() orgtime.ReportingAttribution { return h.reporting }
func (h Hold) Category() expense.Category              { return h.category }
func (h Hold) Tags() Tags                              { return h.tags }
func (h Hold) State() HoldState                        { return h.state }

// ConsumedBy is the expense a CONSUMED hold converted into.
func (h Hold) ConsumedBy() (expense.ExpenseRef, bool) {
	return h.consumedBy, h.state == HoldConsumed
}

// Release and Expire close an ACTIVE hold once; consumption goes through Post.
func (h Hold) Release() (Hold, error) { return h.close(HoldReleased) }
func (h Hold) Expire() (Hold, error)  { return h.close(HoldExpired) }

func (h Hold) close(to HoldState) (Hold, error) {
	if !h.valid {
		return Hold{}, ErrInvalid
	}
	if h.state != HoldActive {
		return Hold{}, ErrHoldNotActive
	}
	h.state = to
	return h, nil
}

// coversHold applies the fact checks to a hold of a constructed line.
func (l Line) coversHold(h Hold) error {
	if !h.valid {
		return ErrInvalid
	}
	if h.ref.org != l.ref.org {
		return ErrScope
	}
	if !l.period.Contains(h.reporting.Day) {
		return ErrOutOfPeriod
	}
	if h.line.id != l.ref.id || !l.dimension.admits(h.tags, h.category.Code()) {
		return ErrDimension
	}
	return nil
}

// Capacity is the line's availability derived from the supplied facts. It
// stores no editable number: every figure is recomputed from those facts.
type Capacity struct {
	redacted
	line                                       Line
	consumed, pending, held                    big.Int
	consumedCount, pendingCount, excludedCount int
	activeHolds, closedHolds                   int
	expenses                                   map[string]bool
	origins                                    map[expense.OriginKey]bool
	holds                                      map[string]Hold
	valid                                      bool
}

// ComputeCapacity checks every consumption entry, then every hold, in input
// order (INVALID, SCOPE, OUT_OF_PERIOD, DIMENSION_MISMATCH, DUPLICATE) and
// returns no partial result. Sums are widened big integers.
func ComputeCapacity(scope kernel.OrgScope, line Line, consumption []Consumption, holds []Hold) (Capacity, error) {
	if scope.Validate() != nil {
		return Capacity{}, ErrScope
	}
	if !line.valid {
		return Capacity{}, ErrInvalid
	}
	if line.ref.org != scope.OrgID() {
		return Capacity{}, ErrScope
	}
	out := Capacity{line: line, expenses: make(map[string]bool, len(consumption)),
		origins: make(map[expense.OriginKey]bool, len(consumption)), holds: make(map[string]Hold, len(holds))}
	for _, c := range consumption {
		if e := line.Covers(c); e != nil {
			return Capacity{}, e
		}
		if e := out.fresh(c); e != nil {
			return Capacity{}, e
		}
		x := c.Entry.Expense
		out.expenses[x.Reference().ID()], out.origins[x.OriginKey()] = true, true
		company := big.NewInt(x.Amounts().CompanyCentavos())
		switch counting, _ := c.Entry.State.Counting(); counting {
		case expense.CountsApproved:
			out.consumed.Add(&out.consumed, company)
			out.consumedCount++
		case expense.CountsPending:
			out.pending.Add(&out.pending, company)
			out.pendingCount++
		default:
			out.excludedCount++
		}
	}
	for _, h := range holds {
		if e := line.coversHold(h); e != nil {
			return Capacity{}, e
		}
		if _, seen := out.holds[h.ref.id]; seen {
			return Capacity{}, ErrDuplicate
		}
		out.holds[h.ref.id] = h
		if h.state == HoldActive {
			out.held.Add(&out.held, big.NewInt(h.amount))
			out.activeHolds++
		} else {
			out.closedHolds++
		}
	}
	out.valid = true
	return out, nil
}

// fresh refuses an expense identity or origin key already among the facts.
func (c Capacity) fresh(x Consumption) error {
	e := x.Entry.Expense
	if c.expenses[e.Reference().ID()] || c.origins[e.OriginKey()] {
		return ErrDuplicate
	}
	return nil
}

// admit checks the decision scope against a constructed capacity.
func (c Capacity) admit(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if !c.valid {
		return ErrInvalid
	}
	if c.line.ref.org != scope.OrgID() {
		return ErrScope
	}
	return nil
}

func (c Capacity) Line() Line           { return c.line }
func (c Capacity) LimitCentavos() int64 { return c.line.limit }

// ConsumedCentavos is the committed consumption: APPROVED company amounts.
func (c Capacity) ConsumedCentavos() *big.Int { return new(big.Int).Set(&c.consumed) }

// PendingCentavos is the separately identified SUBMITTED/IN_REVIEW part; it is never deducted.
func (c Capacity) PendingCentavos() *big.Int { return new(big.Int).Set(&c.pending) }

// HeldCentavos is the sum of ACTIVE holds.
func (c Capacity) HeldCentavos() *big.Int { return new(big.Int).Set(&c.held) }

// AvailableCentavos is limit − consumed − held, exact and possibly negative.
func (c Capacity) AvailableCentavos() *big.Int {
	available := big.NewInt(c.line.limit)
	available.Sub(available, &c.consumed)
	return available.Sub(available, &c.held)
}

// OverCommitted reports a negative availability; it is never clamped.
func (c Capacity) OverCommitted() bool { return c.AvailableCentavos().Sign() < 0 }
func (c Capacity) ConsumedCount() int  { return c.consumedCount }
func (c Capacity) PendingCount() int   { return c.pendingCount }
func (c Capacity) ExcludedCount() int  { return c.excludedCount }
func (c Capacity) ActiveHolds() int    { return c.activeHolds }
func (c Capacity) ClosedHolds() int    { return c.closedHolds }

var claimPattern = regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`)

// Reconcile compares a claimed (stored, cached or displayed) available with
// the derived one; the claim is never used as truth.
func (c Capacity) Reconcile(claimed string) error {
	if !c.valid || len(claimed) > 64 || !claimPattern.MatchString(claimed) {
		return ErrInvalid
	}
	value, _ := new(big.Int).SetString(claimed, 10)
	if value.Cmp(c.AvailableCentavos()) != 0 {
		return ErrStaleAvailable
	}
	return nil
}

// Outcome is a capacity decision. EXCEEDS never approves: an overrun follows
// the owning exception policy (EMP-BUDG-004).
type Outcome string

const (
	OutcomeActivate Outcome = "ACTIVATE"
	OutcomePost     Outcome = "POST"
	OutcomeExceeds  Outcome = "EXCEEDS"
)

// HoldRequest asks to reserve an amount on a named line.
type HoldRequest struct {
	redacted
	Reference  HoldRef
	Line       LineRef
	Amount     *string
	ReservedAt time.Time
	Zones      orgtime.ReportingZones
	Category   string
	Tags       Tags
}

// Reservation is the result of Reserve; only ACTIVATE carries a hold.
type Reservation struct {
	redacted
	outcome                  Outcome
	hold                     Hold
	requested                int64
	before, after, shortfall big.Int
}

// Reserve checks the request like a stored ACTIVE hold of the capacity's line,
// refuses an identity already among its holds, then decides ACTIVATE when the
// amount fits the available capacity or EXCEEDS with the shortfall.
func Reserve(scope kernel.OrgScope, capacity Capacity, req HoldRequest) (Reservation, error) {
	if e := capacity.admit(scope); e != nil {
		return Reservation{}, e
	}
	h, e := NewHold(scope, HoldInput{Reference: req.Reference, Line: req.Line, Amount: req.Amount, ReservedAt: req.ReservedAt,
		Zones: req.Zones, Category: req.Category, Tags: req.Tags, State: HoldActive})
	if e != nil {
		return Reservation{}, e
	}
	if e := capacity.line.coversHold(h); e != nil {
		return Reservation{}, e
	}
	if _, seen := capacity.holds[h.ref.id]; seen {
		return Reservation{}, ErrDuplicate
	}
	available, amount := capacity.AvailableCentavos(), big.NewInt(h.amount)
	out := Reservation{requested: h.amount}
	out.before.Set(available)
	if amount.Cmp(available) <= 0 {
		out.outcome, out.hold = OutcomeActivate, h
		out.after.Sub(available, amount)
		return out, nil
	}
	out.outcome = OutcomeExceeds
	out.after.Set(available)
	out.shortfall.Sub(amount, available)
	return out, nil
}

func (r Reservation) Outcome() Outcome                  { return r.outcome }
func (r Reservation) Hold() (Hold, bool)                { return r.hold, r.outcome == OutcomeActivate }
func (r Reservation) RequestedCentavos() int64          { return r.requested }
func (r Reservation) AvailableBeforeCentavos() *big.Int { return new(big.Int).Set(&r.before) }
func (r Reservation) AvailableAfterCentavos() *big.Int  { return new(big.Int).Set(&r.after) }
func (r Reservation) ShortfallCentavos() *big.Int       { return new(big.Int).Set(&r.shortfall) }

// Posting is the result of Post; a converted hold is returned CONSUMED.
type Posting struct {
	redacted
	outcome                  Outcome
	amount                   int64
	converted                Hold
	hasConverted             bool
	before, after, shortfall big.Int
}

// Post decides whether one APPROVED expense of the line consumes capacity,
// optionally converting one ACTIVE hold of the facts. Order: scope and
// capacity, the expense fact checks, NOT_APPROVED, DUPLICATE, then the hold
// (INVALID, SCOPE, HOLD_NOT_ACTIVE). POST when the company amount is zero or
// fits available plus the converted hold; otherwise EXCEEDS and no change.
func Post(scope kernel.OrgScope, capacity Capacity, c Consumption, hold *HoldRef) (Posting, error) {
	if e := capacity.admit(scope); e != nil {
		return Posting{}, e
	}
	if e := capacity.line.Covers(c); e != nil {
		return Posting{}, e
	}
	if c.Entry.State != expense.Approved {
		return Posting{}, ErrNotApproved
	}
	if e := capacity.fresh(c); e != nil {
		return Posting{}, e
	}
	before := capacity.AvailableCentavos()
	headroom := new(big.Int).Set(before)
	var converted Hold
	if hold != nil {
		if e := hold.validate(scope); e != nil {
			return Posting{}, e
		}
		h, known := capacity.holds[hold.id]
		if !known || h.state != HoldActive {
			return Posting{}, ErrHoldNotActive
		}
		headroom.Add(headroom, big.NewInt(h.amount))
		converted = h
	}
	x := c.Entry.Expense
	out := Posting{amount: x.Amounts().CompanyCentavos()}
	out.before.Set(before)
	need := big.NewInt(out.amount)
	if out.amount == 0 || need.Cmp(headroom) <= 0 {
		out.outcome = OutcomePost
		out.after.Sub(headroom, need)
		if hold != nil {
			converted.state, converted.consumedBy = HoldConsumed, x.Reference()
			out.converted, out.hasConverted = converted, true
		}
		return out, nil
	}
	out.outcome = OutcomeExceeds
	out.after.Set(before)
	out.shortfall.Sub(need, headroom)
	return out, nil
}

func (p Posting) Outcome() Outcome                  { return p.outcome }
func (p Posting) AmountCentavos() int64             { return p.amount }
func (p Posting) ConsumedHold() (Hold, bool)        { return p.converted, p.hasConverted }
func (p Posting) AvailableBeforeCentavos() *big.Int { return new(big.Int).Set(&p.before) }
func (p Posting) AvailableAfterCentavos() *big.Int  { return new(big.Int).Set(&p.after) }
func (p Posting) ShortfallCentavos() *big.Int       { return new(big.Int).Set(&p.shortfall) }
