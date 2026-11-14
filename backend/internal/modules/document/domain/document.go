package domain

import (
	"regexp"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

// digestPattern is the EMP-PLAT-006 private object SHA-256 identity format.
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func canonical(id string) bool { v, e := kernel.ParseOrgID(id); return e == nil && v.String() == id }

// Target is a same-organization operation or record a document may be
// linked to. Its shape proves neither existence nor read authority.
type Target struct {
	redacted
	kind TargetKind
	org  kernel.OrgID
	id   string
}

// NewTarget checks scope, then the closed kind, then the canonical identifier.
func NewTarget(scope kernel.OrgScope, kind TargetKind, id string) (Target, error) {
	if scope.Validate() != nil {
		return Target{}, ErrScope
	}
	if _, e := ParseTargetKind(string(kind)); e != nil || !canonical(id) {
		return Target{}, ErrInvalid
	}
	return Target{kind: kind, org: scope.OrgID(), id: id}, nil
}
func (t Target) Kind() TargetKind             { return t.kind }
func (t Target) ID() string                   { return t.id }
func (t Target) OrganizationID() kernel.OrgID { return t.org }
func (t Target) valid() bool {
	_, e := ParseTargetKind(string(t.kind))
	return e == nil && canonical(t.id) && t.org != (kernel.OrgID{})
}

// AllocationLine assigns exact centavos of the document amount to a target.
type AllocationLine struct {
	Target   Target
	Centavos int64
}

// Statuses are six separate answers; none is derived from another, and
// reconciliation alone is computed from exact amounts.
type Statuses struct {
	Storage, Extraction, Structure, Conference, Reconciliation, Fiscal State
}

// Transition is one command on one dimension. From is the expected current
// state; ConsultedAt is required for, and only for, a fiscal result.
type Transition struct {
	Dimension   Dimension
	From, To    State
	ConsultedAt *time.Time
}

// Document is an immutable value. Every command returns a new value; a
// refused command returns the unchanged receiver with its error. It never
// holds content bytes, file names, media details or personal data.
type Document struct {
	redacted
	org                   kernel.OrgID
	id, digest            string
	replaces, replacedBy  string
	storage, extraction   State
	structure, conference State
	fiscal                State
	consultedAt           *time.Time
	amount                *int64
	links                 []Target
	allocations           []AllocationLine
}

// New creates the container for one original at the initial states.
func New(scope kernel.OrgScope, id, digest string) (Document, error) {
	if scope.Validate() != nil {
		return Document{}, ErrScope
	}
	if !canonical(id) || !digestPattern.MatchString(digest) {
		return Document{}, ErrInvalid
	}
	return Document{org: scope.OrgID(), id: id, digest: digest, storage: Uploading, extraction: ExtractionNone,
		structure: NotApplicable, conference: Unconferred, fiscal: NotConsulted}, nil
}

func (d Document) valid() bool {
	return d.org != (kernel.OrgID{}) && canonical(d.id) && digestPattern.MatchString(d.digest)
}
func (d Document) OrganizationID() kernel.OrgID { return d.org }
func (d Document) ID() string                   { return d.id }
func (d Document) Digest() string               { return d.digest }
func (d Document) Replaces() string             { return d.replaces }
func (d Document) ReplacedBy() string           { return d.replacedBy }
func (d Document) Statuses() Statuses {
	if !d.valid() {
		return Statuses{}
	}
	return Statuses{Storage: d.storage, Extraction: d.extraction, Structure: d.structure,
		Conference: d.conference, Reconciliation: d.reconciliation(), Fiscal: d.fiscal}
}
func (d Document) ConsultedAt() *time.Time {
	if d.consultedAt == nil {
		return nil
	}
	at := *d.consultedAt
	return &at
}
func (d Document) Amount() *int64 {
	if d.amount == nil {
		return nil
	}
	n := *d.amount
	return &n
}

// Allocated is the exact sum of the current allocation set (at most
// MaxAllocationLines × MaxTotalCentavos, far below int64 overflow).
func (d Document) Allocated() int64 {
	var sum int64
	for _, line := range d.allocations {
		sum += line.Centavos
	}
	return sum
}

// Remaining is amount − allocated, negative only when over-allocated.
func (d Document) Remaining() *int64 {
	if d.amount == nil {
		return nil
	}
	n := *d.amount - d.Allocated()
	return &n
}
func (d Document) Links() []Target { return append([]Target(nil), d.links...) }
func (d Document) Allocations() []AllocationLine {
	return append([]AllocationLine(nil), d.allocations...)
}

// reconciliation derives the status from exact sums with zero tolerance (EDR-136).
func (d Document) reconciliation() State {
	var amount int64
	if d.amount != nil {
		amount = *d.amount
	}
	switch sum := d.Allocated(); {
	case sum == 0:
		return Unallocated
	case sum < amount:
		return PartiallyAllocated
	case sum == amount:
		return Reconciled
	}
	return OverAllocated
}

// MatchesOriginal only compares a supplied digest with the original. A match
// proves no fiscal authorization, conference or other status (BR-TRUTH-002).
func (d Document) MatchesOriginal(digest string) (bool, error) {
	if !d.valid() || !digestPattern.MatchString(digest) {
		return false, ErrInvalid
	}
	return digest == d.digest, nil
}

// field selects a commanded dimension; callers have excluded reconciliation.
func (d *Document) field(dimension Dimension) *State {
	switch dimension {
	case Storage:
		return &d.storage
	case Extraction:
		return &d.extraction
	case Structure:
		return &d.structure
	case Conference:
		return &d.conference
	}
	return &d.fiscal
}

// Apply runs one transition command and changes exactly one dimension.
func (d Document) Apply(t Transition) (Document, error) {
	_, fromErr := ParseState(t.Dimension, string(t.From))
	_, toErr := ParseState(t.Dimension, string(t.To))
	if !d.valid() || fromErr != nil || toErr != nil {
		return d, ErrInvalid
	}
	if t.Dimension.Derived() {
		return d, ErrDerived
	}
	if (t.Dimension == Fiscal) != (t.ConsultedAt != nil) {
		return d, ErrInvalid
	}
	var at time.Time
	if t.ConsultedAt != nil {
		if at = t.ConsultedAt.UTC(); at.Year() < 1 || at.Year() > 9999 {
			return d, ErrInvalid
		}
	}
	out := d
	current := out.field(t.Dimension)
	if *current != t.From {
		return d, ErrConflict
	}
	if !allowed(t.Dimension, t.From, t.To) {
		return d, ErrTransition
	}
	if t.Dimension == Fiscal {
		if d.consultedAt != nil && !at.After(*d.consultedAt) {
			return d, ErrConflict
		}
		out.consultedAt = &at
	}
	*current = t.To
	return out, nil
}

// WithAmount records or corrects the exact document amount. Allocations are
// kept; a correction below them is observed as OVER_ALLOCATED.
func (d Document) WithAmount(centavos int64) (Document, error) {
	if !d.valid() || centavos < 0 || centavos > metrics.MaxTotalCentavos {
		return d, ErrInvalid
	}
	out := d
	out.amount = &centavos
	return out, nil
}

func (d Document) scoped(t Target) error {
	if !t.valid() {
		return ErrInvalid
	}
	if t.org != d.org {
		return ErrScope
	}
	return nil
}

// Link adds an owned same-organization link. It grants no access and
// recognizes no cost.
func (d Document) Link(t Target) (Document, error) {
	if !d.valid() {
		return d, ErrInvalid
	}
	if e := d.scoped(t); e != nil {
		return d, e
	}
	for _, linked := range d.links {
		if linked == t {
			return d, ErrDuplicate
		}
	}
	out := d
	out.links = append(d.links[:len(d.links):len(d.links)], t)
	return out, nil
}

func (d Document) claim(t Target, seen map[Target]bool) error {
	if seen[t] {
		return ErrDuplicate
	}
	seen[t] = true
	if !t.kind.Allocatable() {
		return ErrNotAllocatable
	}
	for _, linked := range d.links {
		if linked == t {
			return nil
		}
	}
	return ErrNotLinked
}

// Allocate replaces the allocation set. The whole set is refused when any
// line is invalid or its exact sum exceeds the recorded amount.
func (d Document) Allocate(lines []AllocationLine) (Document, error) {
	if !d.valid() {
		return d, ErrInvalid
	}
	if d.amount == nil {
		return d, ErrAmountMissing
	}
	if len(lines) > exact.MaxAllocationLines {
		return d, ErrInvalid
	}
	seen := make(map[Target]bool, len(lines))
	var sum int64
	for _, line := range lines {
		if e := d.scoped(line.Target); e != nil {
			return d, e
		}
		if line.Centavos < 1 || line.Centavos > metrics.MaxTotalCentavos {
			return d, ErrInvalid
		}
		if e := d.claim(line.Target, seen); e != nil {
			return d, e
		}
		sum += line.Centavos
	}
	if sum > *d.amount {
		return d, ErrExceedsAmount
	}
	out := d
	out.allocations = append([]AllocationLine(nil), lines...)
	return out, nil
}

// SplitEvenly allocates the whole amount over linked expenses with the
// BR-MONEY-007 largest-remainder allocator (equal weights, ties to the
// earlier target) and omits zero shares.
func (d Document) SplitEvenly(targets []Target) (Document, error) {
	if !d.valid() {
		return d, ErrInvalid
	}
	if d.amount == nil {
		return d, ErrAmountMissing
	}
	if len(targets) == 0 || len(targets) > exact.MaxAllocationLines {
		return d, ErrInvalid
	}
	seen := make(map[Target]bool, len(targets))
	one := int64(1)
	weights := make([]exact.Line, len(targets))
	for i, t := range targets {
		if e := d.scoped(t); e != nil {
			return d, e
		}
		if e := d.claim(t, seen); e != nil {
			return d, e
		}
		weights[i] = exact.Line{Order: int64(i), Weight: &one}
	}
	split, e := exact.Allocate(d.amount, weights)
	if e != nil {
		return d, ErrInvalid
	}
	lines := make([]AllocationLine, 0, len(split.Parts))
	for _, part := range split.Parts {
		if part.Centavos > 0 {
			lines = append(lines, AllocationLine{Target: targets[part.Order], Centavos: part.Centavos})
		}
	}
	return d.Allocate(lines)
}

// Replace records a stored same-organization successor for an original. The
// old document keeps its digest, links and allocations and becomes REPLACED.
func Replace(old, successor Document) (Document, Document, error) {
	if !old.valid() || !successor.valid() {
		return old, successor, ErrInvalid
	}
	if old.org != successor.org {
		return old, successor, ErrScope
	}
	if old.id == successor.id {
		return old, successor, ErrInvalid
	}
	if successor.replaces != "" {
		return old, successor, ErrDuplicate
	}
	if (old.storage != Stored && old.storage != Quarantined) || successor.storage != Stored {
		return old, successor, ErrTransition
	}
	o, n := old, successor
	o.storage, o.replacedBy, n.replaces = Replaced, successor.id, old.id
	return o, n, nil
}
