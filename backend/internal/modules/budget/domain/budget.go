// Package domain models pure enterprise budgets: a line over one organization,
// a half-open reporting-day period, one closed dimension and an exact limit;
// capacity derived from approved consumption and active holds; and the
// reservation, posting and hold-lifecycle decisions that the owning
// transaction runs under the line's row lock. It never authorizes, persists,
// applies an overrun policy or reads a clock.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

var (
	ErrInvalid        = errors.New("budget: invalid value")
	ErrScope          = errors.New("budget: organization scope denied")
	ErrOutOfPeriod    = errors.New("budget: reporting day outside the line period")
	ErrDimension      = errors.New("budget: fact outside the line dimension")
	ErrDuplicate      = errors.New("budget: duplicate fact for one line")
	ErrHoldNotActive  = errors.New("budget: hold is not active")
	ErrNotApproved    = errors.New("budget: only an approved company amount posts as consumption")
	ErrStaleAvailable = errors.New("budget: claimed available differs from the derived amount")
)

type redacted struct{}

func (redacted) String() string                 { return "EnterpriseBudgetValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

// DimensionKind is the closed list of budget dimensions (EDR-141).
type DimensionKind string

const (
	DimensionOrganization DimensionKind = "ORGANIZATION"
	DimensionUnit         DimensionKind = "UNIT"
	DimensionCostCentre   DimensionKind = "COST_CENTRE"
	DimensionAsset        DimensionKind = "ASSET"
	DimensionEmployee     DimensionKind = "EMPLOYEE"
)

func ParseDimensionKind(raw string) (DimensionKind, error) {
	switch k := DimensionKind(raw); k {
	case DimensionOrganization, DimensionUnit, DimensionCostCentre, DimensionAsset, DimensionEmployee:
		return k, nil
	}
	return "", ErrInvalid
}

// tagIndex places each targetable kind; ORGANIZATION is never a tag.
func tagIndex(kind DimensionKind) (int, bool) {
	switch kind {
	case DimensionUnit:
		return 0, true
	case DimensionCostCentre:
		return 1, true
	case DimensionAsset:
		return 2, true
	case DimensionEmployee:
		return 3, true
	}
	return 0, false
}

type scopedID struct {
	redacted
	org kernel.OrgID
	id  string
}

func (r scopedID) ID() string                   { return r.id }
func (r scopedID) OrganizationID() kernel.OrgID { return r.org }

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

// validate checks scope, then shape, then ownership, so an unconstructed value is INVALID.
func (r scopedID) validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if !canonical(r.id) || r.org == (kernel.OrgID{}) {
		return ErrInvalid
	}
	if r.org != scope.OrgID() {
		return ErrScope
	}
	return nil
}

// LineRef is a scoped budget line identity; its shape proves no existence.
type LineRef struct{ scopedID }

// HoldRef is a scoped hold identity; its shape proves no existence.
type HoldRef struct{ scopedID }

func NewLineRef(scope kernel.OrgScope, id string) (LineRef, error) {
	r, e := scoped(scope, id)
	return LineRef{r}, e
}
func NewHoldRef(scope kernel.OrgScope, id string) (HoldRef, error) {
	r, e := scoped(scope, id)
	return HoldRef{r}, e
}

// Target is the scoped identity of a unit, cost centre, asset or employee.
// Adapters bind it to the owning register; its shape proves no existence.
type Target struct {
	scopedID
	kind DimensionKind
}

func NewTarget(scope kernel.OrgScope, kind DimensionKind, id string) (Target, error) {
	if scope.Validate() != nil {
		return Target{}, ErrScope
	}
	if _, ok := tagIndex(kind); !ok {
		return Target{}, ErrInvalid
	}
	r, e := scoped(scope, id)
	if e != nil {
		return Target{}, e
	}
	return Target{scopedID: r, kind: kind}, nil
}
func (t Target) Kind() DimensionKind { return t.kind }

// Tags are the unit, cost centre, asset and employee stored on a record when
// it was created (BR-AUTH-005), at most one per kind. The zero value has none.
type Tags struct {
	redacted
	targets [4]Target
}

func NewTags(scope kernel.OrgScope, targets ...Target) (Tags, error) {
	if scope.Validate() != nil {
		return Tags{}, ErrScope
	}
	var out Tags
	for _, t := range targets {
		i, ok := tagIndex(t.kind)
		if !ok || !canonical(t.id) || out.targets[i].id != "" {
			return Tags{}, ErrInvalid
		}
		if t.org != scope.OrgID() {
			return Tags{}, ErrScope
		}
		out.targets[i] = t
	}
	return out, nil
}

func (t Tags) Get(kind DimensionKind) (Target, bool) {
	i, ok := tagIndex(kind)
	if !ok || t.targets[i].id == "" {
		return Target{}, false
	}
	return t.targets[i], true
}

func (t Tags) within(org kernel.OrgID) bool {
	for _, x := range t.targets {
		if x.id != "" && x.org != org {
			return false
		}
	}
	return true
}

// Dimension is one closed kind, the target required by every kind except
// ORGANIZATION, and an optional exact category filter.
type Dimension struct {
	redacted
	org      kernel.OrgID
	kind     DimensionKind
	target   Target
	filter   expense.Category
	filtered bool
	valid    bool
}

// NewDimension refuses a target on ORGANIZATION, a missing or other-kind
// target elsewhere, a foreign target and a malformed filter.
func NewDimension(scope kernel.OrgScope, kind DimensionKind, target *Target, filter *string) (Dimension, error) {
	if scope.Validate() != nil {
		return Dimension{}, ErrScope
	}
	if _, e := ParseDimensionKind(string(kind)); e != nil {
		return Dimension{}, e
	}
	out := Dimension{org: scope.OrgID(), kind: kind}
	if kind == DimensionOrganization {
		if target != nil {
			return Dimension{}, ErrInvalid
		}
	} else {
		if target == nil || target.kind != kind || !canonical(target.id) {
			return Dimension{}, ErrInvalid
		}
		if target.org != scope.OrgID() {
			return Dimension{}, ErrScope
		}
		out.target = *target
	}
	if filter != nil {
		category, e := expense.ParseCategory(*filter)
		if e != nil {
			return Dimension{}, ErrInvalid
		}
		out.filter, out.filtered = category, true
	}
	out.valid = true
	return out, nil
}

func (d Dimension) Kind() DimensionKind { return d.kind }
func (d Dimension) Target() (Target, bool) {
	return d.target, d.target.id != ""
}
func (d Dimension) Filter() (expense.Category, bool) { return d.filter, d.filtered }

// admits compares recorded tags and a category exactly; an absent tag never matches.
func (d Dimension) admits(tags Tags, category string) bool {
	if d.kind != DimensionOrganization {
		t, ok := tags.Get(d.kind)
		if !ok || t.id != d.target.id {
			return false
		}
	}
	return !d.filtered || d.filter.Code() == category
}

// LineInput gathers the required line facts; Limit nil is absence, never zero
// and never unlimited.
type LineInput struct {
	redacted
	Reference  LineRef
	Start, End string
	Dimension  Dimension
	Limit      *string
}

// Line is an immutable budget line. Its zero value is not a budget.
type Line struct {
	redacted
	ref       LineRef
	period    expense.ReportingPeriod
	dimension Dimension
	limit     int64
	valid     bool
}

// NewLine validates, in order, the reference, the half-open period, the
// dimension and the required exact limit (the accepted orgvalue bounds).
func NewLine(scope kernel.OrgScope, in LineInput) (Line, error) {
	if e := in.Reference.validate(scope); e != nil {
		return Line{}, e
	}
	period, e := expense.NewReportingPeriod(in.Start, in.End)
	if e != nil || !in.Dimension.valid {
		return Line{}, ErrInvalid
	}
	if in.Dimension.org != scope.OrgID() {
		return Line{}, ErrScope
	}
	if in.Limit == nil {
		return Line{}, ErrInvalid
	}
	limit, e := exact.ParseCentavos(*in.Limit)
	if e != nil {
		return Line{}, ErrInvalid
	}
	return Line{ref: in.Reference, period: period, dimension: in.Dimension, limit: limit, valid: true}, nil
}

func (l Line) Reference() LineRef              { return l.ref }
func (l Line) Period() expense.ReportingPeriod { return l.period }
func (l Line) Dimension() Dimension            { return l.dimension }
func (l Line) LimitCentavos() int64            { return l.limit }

// Consumption is one expense with its current review state and recorded tags.
type Consumption struct {
	redacted
	Entry expense.Entry
	Tags  Tags
}

// Covers checks one consumption entry against the line: INVALID for an
// unconstructed line or expense or an unknown review state, then SCOPE,
// OUT_OF_PERIOD and DIMENSION_MISMATCH. Any review state may be covered.
func (l Line) Covers(c Consumption) error {
	x := c.Entry.Expense
	org := x.Reference().OrganizationID()
	if !l.valid || org == (kernel.OrgID{}) {
		return ErrInvalid
	}
	if _, e := c.Entry.State.Counting(); e != nil {
		return ErrInvalid
	}
	if org != l.ref.org || !c.Tags.within(l.ref.org) {
		return ErrScope
	}
	if !l.period.Contains(x.Reporting().Day) {
		return ErrOutOfPeriod
	}
	if !l.dimension.admits(c.Tags, x.Category().Code()) {
		return ErrDimension
	}
	return nil
}
