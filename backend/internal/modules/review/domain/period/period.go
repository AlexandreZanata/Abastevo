// Package period decides organization period locks (EMP-APPR-006,
// B-BR-EMP-211/212). It is pure: the owning store supplies the current head,
// appends the returned transition with actor, receipt time and audit event,
// and keeps the history. It performs no I/O, clock read or authorization.
package period

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Closed refusals; the first failing check wins in this order.
var (
	ErrInvalid        = errors.New("period: invalid")
	ErrConflict       = errors.New("period: stale revision")
	ErrTransition     = errors.New("period: transition not allowed")
	ErrReasonRequired = errors.New("period: reason required")
	ErrReasonInvalid  = errors.New("period: reason invalid")
)

// State is the lock state of a period (08-state-model Period row).
type State string

const (
	Open     State = "OPEN"
	Locked   State = "LOCKED"
	Reopened State = "REOPENED"
)

// Kind names one appended transition.
type Kind string

const (
	Lock   Kind = "LOCK"
	Reopen Kind = "REOPEN"
	Relock Kind = "RELOCK"
)

// Action is what the actor asks for. Close covers lock and re-lock.
type Action string

const (
	ActionClose  Action = "CLOSE"
	ActionReopen Action = "REOPEN"
)

// The permissions that authorize each action (07-permissions catalog).
const (
	ClosePermission  = "fin.period.close"
	ReopenPermission = "fin.period.reopen"
)

// MaxReasonScalars is the existing private reason bound (B-BR-EMP-163).
const MaxReasonScalars = 280

// Month is one canonical calendar month. The zero value is invalid.
type Month struct{ year, month int }

// ParseMonth accepts exactly `YYYY-MM` from 0001-01 to 9999-12.
func ParseMonth(raw string) (Month, error) {
	if len(raw) != 7 || raw[4] != '-' {
		return Month{}, ErrInvalid
	}
	for i, b := range []byte(raw) {
		if i != 4 && (b < '0' || b > '9') {
			return Month{}, ErrInvalid
		}
	}
	year := int(raw[0]-'0')*1000 + int(raw[1]-'0')*100 + int(raw[2]-'0')*10 + int(raw[3]-'0')
	month := int(raw[5]-'0')*10 + int(raw[6]-'0')
	if year < 1 || month < 1 || month > 12 {
		return Month{}, ErrInvalid
	}
	return Month{year, month}, nil
}

// MonthOf returns the period holding a canonical `YYYY-MM-DD` reporting day.
func MonthOf(day string) (Month, error) {
	if !validDay(day) {
		return Month{}, ErrInvalid
	}
	return ParseMonth(day[:7])
}

// validDay accepts a canonical YYYY-MM-DD reporting day (BR-TIME-004 format).
func validDay(day string) bool {
	parsed, err := time.Parse(time.DateOnly, day)
	return err == nil && parsed.Year() >= 1 && parsed.Format(time.DateOnly) == day
}

func (m Month) Valid() bool { return m.year >= 1 && m.year <= 9999 && m.month >= 1 && m.month <= 12 }

func (m Month) String() string {
	if !m.Valid() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d", m.year, m.month)
}

// Start is the first reporting day, inclusive.
func (m Month) Start() string {
	if !m.Valid() {
		return ""
	}
	return m.String() + "-01"
}

// End is the first day of the next month, exclusive (half-open period). The
// end of 9999-12 is the first day of year 10000, outside every reporting day.
func (m Month) End() string {
	if !m.Valid() {
		return ""
	}
	year, month := m.year, m.month+1
	if month == 13 {
		year, month = year+1, 1
	}
	return fmt.Sprintf("%04d-%02d-01", year, month)
}

// Contains reports whether a canonical reporting day belongs to the period.
func (m Month) Contains(day string) bool {
	holder, err := MonthOf(day)
	return err == nil && m.Valid() && holder == m
}

// Head is the current state of a period. A month without history is OPEN at
// the implicit revision 1; LOCKED revisions are even and REOPENED ones odd.
type Head struct {
	State    State
	Revision int64
}

// Initial is the implicit, never stored head of every month.
func Initial() Head { return Head{State: Open, Revision: 1} }

// Validate checks the state/revision parity invariant.
func (h Head) Validate() error {
	switch {
	case h.State == Open && h.Revision == 1,
		h.State == Locked && h.Revision >= 2 && h.Revision%2 == 0,
		h.State == Reopened && h.Revision >= 3 && h.Revision%2 == 1:
		return nil
	}
	return ErrInvalid
}

// Command is the actor's request with the revision it saw.
type Command struct {
	Action   Action
	Expected int64
	Reason   string
}

// Transition is what the store appends. Prior transitions never change.
type Transition struct {
	Kind      Kind
	From, To  State
	Revision  int64
	Authority string
	Reason    string
}

// Validate checks a stored transition's internal consistency.
func (t Transition) Validate() error {
	var ok bool
	switch t.Kind {
	case Lock:
		ok = t.From == Open && t.To == Locked && t.Revision == 2 && t.Authority == ClosePermission && t.Reason == ""
	case Reopen:
		ok = t.From == Locked && t.To == Reopened && t.Revision >= 3 && t.Revision%2 == 1 && t.Authority == ReopenPermission && CheckReason(t.Reason) == nil
	case Relock:
		ok = t.From == Reopened && t.To == Locked && t.Revision >= 4 && t.Revision%2 == 0 && t.Authority == ClosePermission && t.Reason == ""
	}
	if !ok {
		return ErrInvalid
	}
	return nil
}

// After returns the head a valid transition produces.
func (t Transition) After() Head {
	if t.Validate() != nil {
		return Head{}
	}
	return Head{State: t.To, Revision: t.Revision}
}

// CheckReason applies the B-BR-EMP-163 reason profile for a required reason
// in the amendment kernel's order: empty or blank is REASON_REQUIRED; invalid
// UTF-8, more than 280 scalars or a control character is REASON_INVALID. A
// reason is kept verbatim, including surrounding spaces.
func CheckReason(raw string) error {
	if raw == "" {
		return ErrReasonRequired
	}
	if !utf8.ValidString(raw) {
		return ErrReasonInvalid
	}
	if strings.TrimSpace(raw) == "" {
		return ErrReasonRequired
	}
	if utf8.RuneCountInString(raw) > MaxReasonScalars {
		return ErrReasonInvalid
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return ErrReasonInvalid
		}
	}
	return nil
}

// Decide classifies one command against the current head or refuses it:
// INVALID, CONFLICT, TRANSITION, then the reopening reason.
func Decide(h Head, c Command) (Transition, error) {
	if h.Validate() != nil || c.Expected < 1 || h.Revision == math.MaxInt64 {
		return Transition{}, ErrInvalid
	}
	switch c.Action {
	case ActionClose:
		if c.Reason != "" {
			return Transition{}, ErrInvalid
		}
	case ActionReopen:
	default:
		return Transition{}, ErrInvalid
	}
	if c.Expected != h.Revision {
		return Transition{}, ErrConflict
	}
	next := Transition{From: h.State, Revision: h.Revision + 1}
	switch {
	case c.Action == ActionClose && h.State == Open:
		next.Kind, next.To, next.Authority = Lock, Locked, ClosePermission
	case c.Action == ActionClose && h.State == Reopened:
		next.Kind, next.To, next.Authority = Relock, Locked, ClosePermission
	case c.Action == ActionReopen && h.State == Locked:
		if err := CheckReason(c.Reason); err != nil {
			return Transition{}, err
		}
		next.Kind, next.To, next.Authority, next.Reason = Reopen, Reopened, ReopenPermission, c.Reason
	default:
		return Transition{}, ErrTransition
	}
	return next, nil
}
