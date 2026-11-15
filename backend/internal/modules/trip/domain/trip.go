package domain

import (
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

type scopedID struct {
	redacted
	org kernel.OrgID
	id  string
}

func (r scopedID) ID() string                   { return r.id }
func (r scopedID) OrganizationID() kernel.OrgID { return r.org }

// TripRef is a scoped trip identity; its shape proves no existence.
type TripRef struct{ scopedID }

// EmployeeRef is the scoped employee record of the driver.
type EmployeeRef struct{ scopedID }

// MembershipRef is the scoped membership of the member who records an event.
type MembershipRef struct{ scopedID }

func canonical(id string) bool { v, e := kernel.ParseOrgID(id); return e == nil && v.String() == id }

func scoped(scope kernel.OrgScope, id string) (scopedID, error) {
	if scope.Validate() != nil {
		return scopedID{}, ErrScope
	}
	if !canonical(id) {
		return scopedID{}, ErrInvalid
	}
	return scopedID{org: scope.OrgID(), id: id}, nil
}

// validate checks shape, then ownership, so an unconstructed value is INVALID.
// Callers check the scope itself first.
func (r scopedID) validate(scope kernel.OrgScope) error {
	if !canonical(r.id) || r.org == (kernel.OrgID{}) {
		return ErrInvalid
	}
	if r.org != scope.OrgID() {
		return ErrScope
	}
	return nil
}

func NewTripRef(scope kernel.OrgScope, id string) (TripRef, error) {
	r, e := scoped(scope, id)
	return TripRef{r}, e
}
func NewEmployeeRef(scope kernel.OrgScope, id string) (EmployeeRef, error) {
	r, e := scoped(scope, id)
	return EmployeeRef{r}, e
}
func NewMembershipRef(scope kernel.OrgScope, id string) (MembershipRef, error) {
	r, e := scoped(scope, id)
	return MembershipRef{r}, e
}

// MeterObservation is departure or final evidence: a reading reference with
// its exact typed value, or a stated absence. Its zero value is "not supplied".
type MeterObservation struct {
	redacted
	reading *meter.ReadingRef
	value   meter.Value
	absence *meter.Missing
}

// ObservedReading pairs a METER-001 reading reference with its exact value.
func ObservedReading(ref meter.ReadingRef, value meter.Value) (MeterObservation, error) {
	// A constructed METER-001 reference is fully scoped; only the zero value is not.
	m := ref.Segment().Meter()
	if ref == (meter.ReadingRef{}) || value.Kind() != m.Kind() {
		return MeterObservation{}, ErrInvalid
	}
	// Re-proves the kind bound and the reference's modulus for this exact value.
	if _, e := meter.NewValue(m.Kind(), value.Count(), m.Modulus()); e != nil {
		return MeterObservation{}, ErrInvalid
	}
	return MeterObservation{reading: &ref, value: value}, nil
}

// AbsentReading records the explicit reason why no reading exists.
func AbsentReading(reason meter.Missing) (MeterObservation, error) {
	if _, e := meter.NewMissing(reason.Reason()); e != nil {
		return MeterObservation{}, ErrInvalid
	}
	return MeterObservation{absence: &reason}, nil
}
func (o MeterObservation) Reading() *meter.ReadingRef { return copyPointer(o.reading) }
func (o MeterObservation) Value() (meter.Value, bool) { return o.value, o.reading != nil }
func (o MeterObservation) Absence() *meter.Missing    { return copyPointer(o.absence) }

func copyPointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func (o MeterObservation) supplied() bool { return o.reading != nil || o.absence != nil }

// check binds a supplied reading to the organization and, when known, to the trip's asset.
func (o MeterObservation) check(scope kernel.OrgScope, asset *fleet.AssetRef) error {
	if o.reading == nil {
		return nil
	}
	owner := o.reading.Segment().Meter().Asset()
	if owner.OrganizationID() != scope.OrgID() || asset != nil && owner != *asset {
		return ErrScope
	}
	return nil
}

// Missing names an absent part of the eligible context.
type Missing string

const (
	MissingAsset       Missing = "ASSET"
	MissingDriver      Missing = "DRIVER"
	MissingDeparture   Missing = "DEPARTURE_EVIDENCE"
	MissingDeclaration Missing = "DECLARATION"
	MissingFinal       Missing = "FINAL_EVIDENCE"
)

// IncompleteError lists every absent part, in order; it unwraps to ErrIncomplete.
type IncompleteError struct{ missing []Missing }

func (*IncompleteError) Error() string          { return ErrIncomplete.Error() }
func (*IncompleteError) Unwrap() error          { return ErrIncomplete }
func (err *IncompleteError) Missing() []Missing { return append([]Missing(nil), err.missing...) }

// StartInput is the driver's start. Zones come from the resolved operating
// context (unit, else organization), never from a device claim.
type StartInput struct {
	redacted
	Reference TripRef
	Author    MembershipRef
	Asset     fleet.AssetRef
	Driver    EmployeeRef
	Departure MeterObservation
	Declared  *orgtime.Declaration
	Zones     orgtime.ReportingZones
	Clock     *RunClock // optional device monotonic witness (EMP-TRIP-017)
}

// FinishInput is an operational completion; it cannot change the trip.
type FinishInput struct {
	redacted
	Author   MembershipRef
	Final    MeterObservation
	Declared *orgtime.Declaration
	Clock    *RunClock // optional device monotonic witness (EMP-TRIP-017)
}

// Event is one recorded lifecycle event: actor, declaration and evidence.
type Event struct {
	redacted
	author   MembershipRef
	declared orgtime.Declaration
	evidence MeterObservation
	clock    *RunClock
}

func (e Event) Author() MembershipRef         { return e.author }
func (e Event) Declared() orgtime.Declaration { return e.declared }
func (e Event) Evidence() MeterObservation    { return e.evidence }
func (e Event) Clock() *RunClock              { return copyPointer(e.clock) }

// Trip is an immutable operational trip; only Start and Finish build one.
type Trip struct {
	redacted
	reference TripRef
	asset     fleet.AssetRef
	driver    EmployeeRef
	state     State
	start     Event
	reporting orgtime.ReportingAttribution
	finish    *Event
	endDay    string
	revision  int64
	lifecycle []LifecycleFact
	closure   *Closure // EMP-TRIP-014: a supervisor closure, never beside a finish
}

// Start creates a C1 driver-initiated trip in IN_PROGRESS (B-BR-EMP-269/270).
// It checks the scope, identity and author, then the present parts of the
// eligible context in order, then reports every absent part, then attributes
// the start reporting day. It reads no clock and needs no server receipt.
func Start(scope kernel.OrgScope, in StartInput) (Trip, error) {
	if scope.Validate() != nil {
		return Trip{}, ErrScope
	}
	for _, r := range []scopedID{in.Reference.scopedID, in.Author.scopedID} {
		if e := r.validate(scope); e != nil {
			return Trip{}, e
		}
	}
	var missing []Missing
	var asset *fleet.AssetRef
	if in.Asset == (fleet.AssetRef{}) {
		missing = append(missing, MissingAsset)
	} else if in.Asset.OrganizationID() != scope.OrgID() {
		return Trip{}, ErrScope
	} else {
		asset = &in.Asset
	}
	if in.Driver == (EmployeeRef{}) {
		missing = append(missing, MissingDriver)
	} else if e := in.Driver.validate(scope); e != nil {
		return Trip{}, e
	}
	if !in.Departure.supplied() {
		missing = append(missing, MissingDeparture)
	} else if e := in.Departure.check(scope, asset); e != nil {
		return Trip{}, e
	}
	var declared orgtime.Declaration
	if in.Declared == nil {
		missing = append(missing, MissingDeclaration)
	} else {
		var e error
		if declared, e = orgtime.ValidateDeclaration(*in.Declared); e != nil {
			return Trip{}, ErrInvalid
		}
	}
	clock, e := optionalClock(in.Clock)
	if e != nil {
		return Trip{}, e
	}
	if len(missing) != 0 {
		return Trip{}, &IncompleteError{missing: missing}
	}
	reporting, e := orgtime.AttributeReportingDay(declared.OccurredAt, in.Zones)
	if e != nil {
		return Trip{}, ErrInvalid
	}
	return Trip{reference: in.Reference, asset: in.Asset, driver: in.Driver, state: InProgress,
		start: Event{author: in.Author, declared: declared, evidence: in.Departure, clock: clock}, reporting: reporting, revision: 1}, nil
}

// Finish completes the trip operationally, from IN_PROGRESS or PAUSED, and
// returns the next value at the next revision (B-BR-EMP-271/272/390). The receiver never changes; on error no trip is built.
func (t Trip) Finish(scope kernel.OrgScope, in FinishInput) (Trip, error) {
	if scope.Validate() != nil {
		return Trip{}, ErrScope
	}
	if !t.state.valid() {
		return Trip{}, ErrInvalid
	}
	if t.reference.org != scope.OrgID() {
		return Trip{}, ErrScope
	}
	if e := in.Author.validate(scope); e != nil {
		return Trip{}, e
	}
	var missing []Missing
	if !in.Final.supplied() {
		missing = append(missing, MissingFinal)
	} else if e := in.Final.check(scope, &t.asset); e != nil {
		return Trip{}, e
	} else if in.Final.reading != nil && t.start.evidence.reading != nil && in.Final.reading.ID() == t.start.evidence.reading.ID() {
		return Trip{}, ErrInvalid
	}
	var declared orgtime.Declaration
	if in.Declared == nil {
		missing = append(missing, MissingDeclaration)
	} else {
		var e error
		if declared, e = orgtime.ValidateDeclaration(*in.Declared); e != nil {
			return Trip{}, ErrInvalid
		}
	}
	clock, e := optionalClock(in.Clock)
	if e != nil {
		return Trip{}, e
	}
	if len(missing) != 0 {
		return Trip{}, &IncompleteError{missing: missing}
	}
	if in.Author != t.Responsible() {
		return Trip{}, ErrForbidden
	}
	next, e := Transition(t.state, CommandFinish)
	if e != nil {
		return Trip{}, e
	}
	if declared.OccurredAt.Before(t.start.declared.OccurredAt) {
		return Trip{}, ErrFinishBeforeStart
	}
	// The trip keeps its start attribution; the end day is informational,
	// in the stored reporting zone (B-BR-EMP-276).
	endDay, e := localDay(declared, t.reporting.Zone)
	if e != nil {
		return Trip{}, e
	}
	out := t
	out.state, out.endDay, out.revision = next, endDay, t.revision+1
	out.finish = &Event{author: in.Author, declared: declared, evidence: in.Final, clock: clock}
	return out, nil
}

func (t Trip) Reference() TripRef           { return t.reference }
func (t Trip) OrganizationID() kernel.OrgID { return t.reference.org }
func (t Trip) Asset() fleet.AssetRef        { return t.asset }
func (t Trip) Driver() EmployeeRef          { return t.driver }
func (t Trip) State() State                 { return t.state }
func (t Trip) Started() Event               { return t.start }
func (t Trip) Finished() (Event, bool) {
	if t.finish == nil {
		return Event{}, false
	}
	return *t.finish, true
}
func (t Trip) Reporting() orgtime.ReportingAttribution { return t.reporting }

// UsageStatus says whether the trip has meter evidence of its usage.
type UsageStatus string

const (
	Measured    UsageStatus = "MEASURED"
	NotMeasured UsageStatus = "NOT_MEASURED"
)

// UsageReason explains a NOT_MEASURED usage.
type UsageReason string

const (
	ReasonNotFinished      UsageReason = "NOT_FINISHED"
	ReasonReadingAbsent    UsageReason = "READING_ABSENT"
	ReasonDifferentSegment UsageReason = "DIFFERENT_SEGMENT"
	ReasonDecreasing       UsageReason = "DECREASING"
	// ReasonCancelled: a cancelled trip has no final evidence (EMP-TRIP-011).
	ReasonCancelled UsageReason = "CANCELLED"
)

// MeterUsage is the trip's own meter evidence; it is never invented.
type MeterUsage struct {
	redacted
	status UsageStatus
	reason UsageReason
	kind   meter.Kind
	delta  int64
}

func (u MeterUsage) Status() UsageStatus  { return u.status }
func (u MeterUsage) Reason() UsageReason  { return u.reason }
func (u MeterUsage) Kind() meter.Kind     { return u.kind }
func (u MeterUsage) Delta() (int64, bool) { return u.delta, u.status == Measured }

// MeterUsage reports the trip's meter evidence (BR-TRIP-004): the exact
// difference of its own two readings on one segment, or why there is none.
func (t Trip) MeterUsage() MeterUsage {
	if t.state == Cancelled {
		return MeterUsage{status: NotMeasured, reason: ReasonCancelled}
	}
	if t.closure != nil {
		// EMP-TRIP-014 (B-BR-EMP-400): a closure has no final reading.
		return MeterUsage{status: NotMeasured, reason: ReasonReadingAbsent}
	}
	if t.finish == nil {
		return MeterUsage{status: NotMeasured, reason: ReasonNotFinished}
	}
	from, to := t.start.evidence, t.finish.evidence
	switch {
	case from.reading == nil || to.reading == nil:
		return MeterUsage{status: NotMeasured, reason: ReasonReadingAbsent}
	case from.reading.Segment() != to.reading.Segment():
		return MeterUsage{status: NotMeasured, reason: ReasonDifferentSegment}
	case to.value.Count() < from.value.Count():
		return MeterUsage{status: NotMeasured, reason: ReasonDecreasing}
	}
	return MeterUsage{status: Measured, kind: from.value.Kind(), delta: to.value.Count() - from.value.Count()}
}
