// Package incident is the pure damage and incident report rule of
// EMP-CHECK-003 (B-BR-EMP-369, B-BR-EMP-373): the bounded report a member
// records offline and the handling state machine of the operational states
// REPORTED, ACKNOWLEDGED, IN_TREATMENT, RESOLVED and DISMISSED. It never
// holds a cost, a document or a person's health. It is standard-library only
// and mirrored in Kotlin (com.anpfuel.domain.enterprise.maintenance.incident)
// through the shared incident-v1 oracle.
package incident

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrInvalid is a malformed report, current fact or handling command.
	ErrInvalid = errors.New("incident: invalid")
	// ErrConflict is an expected revision other than the current one.
	ErrConflict = errors.New("incident: stale revision")
	// ErrTransition is a handling step not allowed from the current state.
	ErrTransition = errors.New("incident: transition not allowed")
	// ErrSelfApproval is the reporter resolving or dismissing a blocking report.
	ErrSelfApproval = errors.New("incident: self approval")
)

// Bounds of B-BR-EMP-369/373 (BR-CHECK-015: description 1 000, reason 500).
const (
	MaxDescriptionScalars = 1000
	MaxLocationScalars    = 140
	MaxNoteScalars        = 500
	// MaxRevision is the longest handling path: REPORTED (1), ACKNOWLEDGED
	// (2), IN_TREATMENT (3), RESOLVED or DISMISSED (4).
	MaxRevision = 4
)

// ReportType distinguishes a damage of the asset from an incident.
type ReportType string

const (
	Damage   ReportType = "DAMAGE"
	Incident ReportType = "INCIDENT"
)

func ParseReportType(raw string) (ReportType, error) {
	switch t := ReportType(raw); t {
	case Damage, Incident:
		return t, nil
	}
	return "", ErrInvalid
}

// Kind is the closed kind of an incident; a damage has none.
type Kind string

const (
	Breakdown  Kind = "BREAKDOWN"
	Accident   Kind = "ACCIDENT"
	Theft      Kind = "THEFT"
	FineNotice Kind = "FINE_NOTICE"
	Other      Kind = "OTHER"
)

var kinds = []Kind{Breakdown, Accident, Theft, FineNotice, Other}

// Kinds returns the closed incident kinds in their fixed order.
func Kinds() []Kind { return slices.Clone(kinds) }

func ParseKind(raw string) (Kind, error) {
	if !slices.Contains(kinds, Kind(raw)) {
		return "", ErrInvalid
	}
	return Kind(raw), nil
}

// State is the operational state shared by damages and incidents.
type State string

const (
	Reported     State = "REPORTED"
	Acknowledged State = "ACKNOWLEDGED"
	InTreatment  State = "IN_TREATMENT"
	Resolved     State = "RESOLVED"
	Dismissed    State = "DISMISSED"
)

var states = []State{Reported, Acknowledged, InTreatment, Resolved, Dismissed}

// States returns the closed states in their fixed order.
func States() []State { return slices.Clone(states) }

func ParseState(raw string) (State, error) {
	if !slices.Contains(states, State(raw)) {
		return "", ErrInvalid
	}
	return State(raw), nil
}

// Terminal reports RESOLVED and DISMISSED.
func (s State) Terminal() bool { return s == Resolved || s == Dismissed }

// Action is one handling command of UC-CHECK-04.
type Action string

const (
	Acknowledge Action = "ACKNOWLEDGE"
	Treat       Action = "TREAT"
	Resolve     Action = "RESOLVE"
	Dismiss     Action = "DISMISS"
)

var actions = []Action{Acknowledge, Treat, Resolve, Dismiss}

// Actions returns the closed handling actions in their fixed order.
func Actions() []Action { return slices.Clone(actions) }

func ParseAction(raw string) (Action, error) {
	if !slices.Contains(actions, Action(raw)) {
		return "", ErrInvalid
	}
	return Action(raw), nil
}

// Target is the state an action leads to.
func (a Action) Target() State {
	switch a {
	case Acknowledge:
		return Acknowledged
	case Treat:
		return InTreatment
	case Resolve:
		return Resolved
	case Dismiss:
		return Dismissed
	}
	return ""
}

// NeedsNote reports whether the action carries a note (the stated action,
// the resolution note or the dismissal reason); an acknowledgement has none.
func (a Action) NeedsNote() bool { return a == Treat || a == Resolve || a == Dismiss }

// Allowed reports the 08-state-model transitions.
func Allowed(from State, action Action) bool {
	switch action {
	case Acknowledge:
		return from == Reported
	case Treat:
		return from == Reported || from == Acknowledged
	case Resolve, Dismiss:
		return from == Reported || from == Acknowledged || from == InTreatment
	}
	return false
}

// CanonicalID reports a lowercase, dashed, non-nil UUID.
func CanonicalID(raw string) bool {
	if len(raw) != 36 || raw == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		switch i {
		case 8, 13, 18, 23:
			if b != '-' {
				return false
			}
		default:
			if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
				return false
			}
		}
	}
	return true
}

// ValidText is the shared text profile (SQL org_asset_text_valid with
// required = true): valid UTF-8, 1..maximum Unicode scalars, no C0/C1
// control character and not only white space.
func ValidText(raw string, maximum int) bool {
	if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > maximum {
		return false
	}
	says := false
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false
		}
		if !unicode.IsSpace(r) {
			says = true
		}
	}
	return says
}

// private redacts free text on every formatting surface.
type private struct{}

func (private) String() string               { return "incident.Report(redacted)" }
func (p private) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, p.String()) }
func (p private) GoString() string           { return p.String() }
func (private) MarshalJSON() ([]byte, error) { return []byte(`"incident.Report(redacted)"`), nil }
func (p private) LogValue() slog.Value       { return slog.StringValue(p.String()) }

// Draft is one report as its author's device recorded it. Kind is empty for
// a damage; Location is nil when absent.
type Draft struct {
	private
	Type        ReportType
	Kind        Kind
	Description string
	Location    *string
	BlocksUse   bool
}

// Report is a validated report at its initial state and revision.
type Report struct {
	private
	Type        ReportType
	Kind        Kind
	Description string
	Location    *string
	BlocksUse   bool
	State       State
	Revision    int64
}

// NewReport validates one draft (B-BR-EMP-369): the type, the kind (one of
// the closed kinds for an incident, none for a damage), the description and
// the optional location. A report starts REPORTED at revision 1.
func NewReport(d Draft) (Report, error) {
	t, err := ParseReportType(string(d.Type))
	if err != nil {
		return Report{}, err
	}
	switch t {
	case Incident:
		if _, err := ParseKind(string(d.Kind)); err != nil {
			return Report{}, err
		}
	case Damage:
		if d.Kind != "" {
			return Report{}, ErrInvalid
		}
	}
	if !ValidText(d.Description, MaxDescriptionScalars) {
		return Report{}, ErrInvalid
	}
	var location *string
	if d.Location != nil {
		if !ValidText(*d.Location, MaxLocationScalars) {
			return Report{}, ErrInvalid
		}
		v := *d.Location
		location = &v
	}
	return Report{Type: t, Kind: d.Kind, Description: d.Description, Location: location, BlocksUse: d.BlocksUse, State: Reported, Revision: 1}, nil
}

// Current is the stored fact a handling command is decided against. The
// reporter is the set of its identity keys (account, membership, employee
// record).
type Current struct {
	State     State
	Revision  int64
	BlocksUse bool
	ReportID  string
	Reporter  []string
}

// Command is one handling command. Note is nil when absent; DuplicateOf is
// empty when absent. Handler is the set of the handler's identity keys.
type Command struct {
	private
	Action           Action
	ExpectedRevision int64
	Note             *string
	DuplicateOf      string
	Handler          []string
}

// Step is the accepted transition.
type Step struct {
	Action   Action
	From, To State
	Revision int64
}

func validIdentity(keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	for _, k := range keys {
		if !CanonicalID(k) {
			return false
		}
	}
	return true
}

func validCurrent(c Current) bool {
	if _, err := ParseState(string(c.State)); err != nil {
		return false
	}
	if c.Revision < 1 || c.Revision > MaxRevision || (c.State == Reported) != (c.Revision == 1) {
		return false
	}
	return CanonicalID(c.ReportID) && validIdentity(c.Reporter)
}

func shares(a, b []string) bool {
	for _, k := range a {
		if slices.Contains(b, k) {
			return true
		}
	}
	return false
}

// Decide applies one handling command (B-BR-EMP-373), first failure first:
// ErrInvalid (current fact, action, expected revision, handler, note policy,
// duplicate), ErrConflict (expected revision differs), ErrTransition (not
// allowed from the current state), ErrSelfApproval (the reporter resolves or
// dismisses a blocking report).
func Decide(current Current, command Command) (Step, error) {
	if !validCurrent(current) {
		return Step{}, ErrInvalid
	}
	action, err := ParseAction(string(command.Action))
	if err != nil {
		return Step{}, err
	}
	if command.ExpectedRevision < 1 || !validIdentity(command.Handler) {
		return Step{}, ErrInvalid
	}
	if action.NeedsNote() != (command.Note != nil) || command.Note != nil && !ValidText(*command.Note, MaxNoteScalars) {
		return Step{}, ErrInvalid
	}
	if command.DuplicateOf != "" && (action != Dismiss || !CanonicalID(command.DuplicateOf) || command.DuplicateOf == current.ReportID) {
		return Step{}, ErrInvalid
	}
	if command.ExpectedRevision != current.Revision {
		return Step{}, ErrConflict
	}
	if !Allowed(current.State, action) {
		return Step{}, ErrTransition
	}
	if current.BlocksUse && (action == Resolve || action == Dismiss) && shares(command.Handler, current.Reporter) {
		return Step{}, ErrSelfApproval
	}
	return Step{Action: action, From: current.State, To: action.Target(), Revision: current.Revision + 1}, nil
}
