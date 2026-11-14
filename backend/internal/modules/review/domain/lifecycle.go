// Package domain owns the pure single-level (C1) review lifecycle shared by
// trips, fuelings, expenses and work orders (EMP-APPR-001, EDR-012,
// B-BR-EMP-175..179). It holds no persistence, organization scope, clock,
// audit actor, approval level or amount; owners bind those.
package domain

import (
	"errors"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Closed failure vocabulary; the first failure wins in this order.
var (
	ErrInvalid           = errors.New("review: invalid input")
	ErrReasonRequired    = errors.New("review: reason required")
	ErrForbidden         = errors.New("review: permission not held at receipt")
	ErrSelfApproval      = errors.New("review: reviewer is the author or beneficiary")
	ErrConflict          = errors.New("review: command names a stale revision")
	ErrInvalidTransition = errors.New("review: transition not allowed")
)

// MaxReasonScalars reuses the existing private reason bound (B-BR-EMP-163).
const MaxReasonScalars = 280

// State is a server review state of review-v1.
type State string

const (
	Draft     State = "DRAFT"
	Submitted State = "SUBMITTED"
	InReview  State = "IN_REVIEW"
	Approved  State = "APPROVED"
	Returned  State = "RETURNED"
	Rejected  State = "REJECTED"
	Cancelled State = "CANCELLED"
)

// Kind is a review command.
type Kind string

const (
	Submit   Kind = "SUBMIT"
	Cancel   Kind = "CANCEL"
	Resubmit Kind = "RESUBMIT"
	Open     Kind = "OPEN"
	Approve  Kind = "APPROVE"
	Return   Kind = "RETURN"
	Reject   Kind = "REJECT"
)

// Role says who may issue a command.
type Role string

const (
	RoleAuthor   Role = "AUTHOR"
	RoleReviewer Role = "REVIEWER"
)

// ReasonPolicy says whether a command carries a reason.
type ReasonPolicy string

const (
	ReasonNone     ReasonPolicy = "NONE"
	ReasonOptional ReasonPolicy = "OPTIONAL"
	ReasonRequired ReasonPolicy = "REQUIRED"
)

// ParseState accepts only an exact review-v1 state token.
func ParseState(raw string) (State, error) {
	s := State(raw)
	if !s.valid() {
		return "", ErrInvalid
	}
	return s, nil
}

func (s State) valid() bool {
	switch s {
	case Draft, Submitted, InReview, Approved, Returned, Rejected, Cancelled:
		return true
	}
	return false
}

// Terminal reports a valid state that accepts no command.
func (s State) Terminal() bool { return s.valid() && len(transitions[s]) == 0 }

// ParseKind accepts only an exact review-v1 command token.
func ParseKind(raw string) (Kind, error) {
	k := Kind(raw)
	if _, ok := policies[k]; !ok {
		return "", ErrInvalid
	}
	return k, nil
}

// Role returns the issuing role of a valid command, or "".
func (k Kind) Role() Role { return policies[k].role }

// ReasonPolicy returns the reason policy of a valid command, or "".
func (k Kind) ReasonPolicy() ReasonPolicy { return policies[k].reason }

// Identity is one canonical person as resolved by the server at receipt,
// following the actual employee merge chain. Identifiers are canonical
// lowercase non-nil UUIDs; at least one is required.
type Identity struct {
	Accounts    []string
	Memberships []string
	Employees   []string
}

// Facts are supplied by the server at receipt. Permitted is the authority
// port's answer for the command's permission on the record scope; its zero
// value is "not granted".
type Facts struct {
	Actor       Identity
	Author      Identity
	Beneficiary *Identity
	Permitted   bool
}

// Command names the revision it acts on or judges.
type Command struct {
	Kind     Kind
	Revision int64
	Reason   string
}

// Entry is one accepted command: resulting revision and state, exact reason.
type Entry struct {
	Kind     Kind
	Revision int64
	State    State
	Reason   string
}

// Case is an immutable review case; only NewDraft, Replay and Apply build one.
type Case struct {
	state    State
	revision int64
	history  []Entry
}

// NewDraft starts a case as DRAFT revision 1 with no history.
func NewDraft() Case { return Case{state: Draft, revision: 1} }

// Replay rebuilds a case from stored history, refusing any entry that the
// lifecycle could not have produced.
func Replay(history []Entry) (Case, error) {
	c := NewDraft()
	c.history = make([]Entry, 0, len(history))
	for _, e := range history {
		if checkReason(e.Kind, e.Reason) != nil {
			return Case{}, ErrInvalid
		}
		next, ok := c.next(e.Kind)
		if !ok || next.State != e.State || next.Revision != e.Revision {
			return Case{}, ErrInvalid
		}
		next.Reason = e.Reason
		// c.history is a fresh slice owned only by this replay.
		c.state, c.revision, c.history = next.State, next.Revision, append(c.history, next)
	}
	return c, nil
}

// State returns the current review state.
func (c Case) State() State { return c.state }

// Revision returns the current revision.
func (c Case) Revision() int64 { return c.revision }

// History returns a copy of the append-only history.
func (c Case) History() []Entry {
	if len(c.history) == 0 {
		return nil
	}
	return append([]Entry(nil), c.history...)
}

// Apply validates one command at receipt and returns the next case. On any
// error no case is produced and the receiver is unchanged.
func (c Case) Apply(cmd Command, facts Facts) (Case, error) {
	if !c.state.valid() || cmd.Revision < 1 || !facts.valid() {
		return Case{}, ErrInvalid
	}
	if err := checkReason(cmd.Kind, cmd.Reason); err != nil {
		return Case{}, err
	}
	if !facts.Permitted {
		return Case{}, ErrForbidden
	}
	if cmd.Kind.Role() == RoleAuthor && !facts.Actor.shares(facts.Author) {
		return Case{}, ErrForbidden
	}
	if cmd.Kind.Role() == RoleReviewer && (facts.Actor.shares(facts.Author) || facts.Beneficiary != nil && facts.Actor.shares(*facts.Beneficiary)) {
		return Case{}, ErrSelfApproval
	}
	if cmd.Revision != c.revision {
		return Case{}, ErrConflict
	}
	next, ok := c.next(cmd.Kind)
	if !ok {
		return Case{}, ErrInvalidTransition
	}
	next.Reason = cmd.Reason
	return c.append(next), nil
}

type policy struct {
	role   Role
	reason ReasonPolicy
}

var policies = map[Kind]policy{
	Submit:   {RoleAuthor, ReasonNone},
	Cancel:   {RoleAuthor, ReasonOptional},
	Resubmit: {RoleAuthor, ReasonNone},
	Open:     {RoleReviewer, ReasonNone},
	Approve:  {RoleReviewer, ReasonOptional},
	Return:   {RoleReviewer, ReasonRequired},
	Reject:   {RoleReviewer, ReasonRequired},
}

// transitions is the BR-REV diagram for one level (EDR-128). Decisions are
// accepted only from IN_REVIEW; terminal states have no entry.
var transitions = map[State]map[Kind]State{
	Draft:     {Submit: Submitted, Cancel: Cancelled},
	Submitted: {Open: InReview, Cancel: Cancelled},
	InReview:  {Approve: Approved, Return: Returned, Reject: Rejected},
	Returned:  {Resubmit: Submitted, Cancel: Cancelled},
}

// next returns the entry a valid command would append. Only RESUBMIT creates
// a revision; the revision is bounded by the history length, so it cannot
// overflow.
func (c Case) next(kind Kind) (Entry, bool) {
	state, ok := transitions[c.state][kind]
	if !ok {
		return Entry{}, false
	}
	revision := c.revision
	if kind == Resubmit {
		revision++
	}
	return Entry{Kind: kind, Revision: revision, State: state}, true
}

// append never writes into storage shared with the receiver.
func (c Case) append(e Entry) Case {
	history := make([]Entry, len(c.history), len(c.history)+1)
	copy(history, c.history)
	return Case{state: e.State, revision: e.Revision, history: append(history, e)}
}

// checkReason applies the command's reason policy with the existing private
// 280-scalar, control-free rule (B-BR-EMP-163). Reasons are kept verbatim.
func checkReason(kind Kind, raw string) error {
	p, ok := policies[kind]
	if !ok {
		return ErrInvalid
	}
	says, ok := reasonShape(raw)
	switch {
	case !ok:
		return ErrInvalid
	case p.reason == ReasonRequired && !says:
		return ErrReasonRequired
	case p.reason == ReasonOptional && raw != "" && !says, p.reason == ReasonNone && raw != "":
		return ErrInvalid
	}
	return nil
}

// reasonShape reports whether a well-formed reason says something: empty and
// whitespace-only reasons say nothing.
func reasonShape(raw string) (says bool, ok bool) {
	if !utf8.ValidString(raw) {
		return false, false
	}
	n := 0
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false, false
		}
		if !unicode.IsSpace(r) {
			says = true
		}
		n++
	}
	return says, n <= MaxReasonScalars
}

func (f Facts) valid() bool {
	return f.Actor.valid() && f.Author.valid() && (f.Beneficiary == nil || f.Beneficiary.valid())
}

func (i Identity) all() [][]string { return [][]string{i.Accounts, i.Memberships, i.Employees} }

func (i Identity) valid() bool {
	n := 0
	for _, ids := range i.all() {
		for _, id := range ids {
			if v, e := kernel.ParseOrgID(id); e != nil || v.String() != id {
				return false
			}
			n++
		}
	}
	return n > 0
}

// shares compares canonical identifiers regardless of kind (EDR-129): UUIDs
// are unique, so any equality means the same person, and the check fails
// closed.
func (i Identity) shares(other Identity) bool {
	seen := map[string]bool{}
	for _, ids := range other.all() {
		for _, id := range ids {
			seen[id] = true
		}
	}
	for _, ids := range i.all() {
		for _, id := range ids {
			if seen[id] {
				return true
			}
		}
	}
	return false
}
