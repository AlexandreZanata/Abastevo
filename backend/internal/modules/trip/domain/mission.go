package domain

import (
	"errors"
	"slices"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// ErrConflict is a command decided on a stale mission revision (EMP-TRIP-008,
// B-BR-EMP-353) or a trip lifecycle command naming a stale trip revision
// (EMP-TRIP-011, B-BR-EMP-390); identity, scope, author and table failures keep ErrInvalid,
// ErrScope, ErrForbidden, ErrInvalidTransition and ErrUnsupportedCommand.
var ErrConflict = errors.New("trip: stale revision")

// MissionState is a mission operational state of the canonical catalog
// (08-state-model §3).
type MissionState string

const (
	MissionDraft      MissionState = "DRAFT"
	MissionPlanned    MissionState = "PLANNED"
	MissionAssigned   MissionState = "ASSIGNED"
	MissionAccepted   MissionState = "ACCEPTED"
	MissionDeclined   MissionState = "DECLINED"
	MissionInProgress MissionState = "IN_PROGRESS"
	MissionCompleted  MissionState = "COMPLETED"
	MissionCancelled  MissionState = "CANCELLED"
)

var missionStates = []MissionState{MissionDraft, MissionPlanned, MissionAssigned, MissionAccepted, MissionDeclined, MissionInProgress, MissionCompleted, MissionCancelled}

// MissionStates returns the closed vocabulary in its fixed order.
func MissionStates() []MissionState { return slices.Clone(missionStates) }

// ParseMissionState accepts only an exact catalog token.
func ParseMissionState(raw string) (MissionState, error) {
	if !slices.Contains(missionStates, MissionState(raw)) {
		return "", ErrInvalid
	}
	return MissionState(raw), nil
}

// Terminal reports COMPLETED and CANCELLED.
func (s MissionState) Terminal() bool { return s == MissionCompleted || s == MissionCancelled }

// MissionCommand is one mission command.
type MissionCommand string

const (
	MissionPlan     MissionCommand = "PLAN"
	MissionAssign   MissionCommand = "ASSIGN"
	MissionAccept   MissionCommand = "ACCEPT"
	MissionDecline  MissionCommand = "DECLINE"
	MissionStart    MissionCommand = "START"
	MissionComplete MissionCommand = "COMPLETE"
	MissionCancel   MissionCommand = "CANCEL"
)

var missionCommands = []MissionCommand{MissionPlan, MissionAssign, MissionAccept, MissionDecline, MissionStart, MissionComplete, MissionCancel}

// MissionCommands returns the closed command vocabulary in its fixed order.
func MissionCommands() []MissionCommand { return slices.Clone(missionCommands) }

// ParseMissionCommand accepts only an exact command token.
func ParseMissionCommand(raw string) (MissionCommand, error) {
	if !slices.Contains(missionCommands, MissionCommand(raw)) {
		return "", ErrInvalid
	}
	return MissionCommand(raw), nil
}

// MissionTransition is the EDR-228 table with the EDR-246 PLAN rows and the
// EDR-265 trip rows, first failure first: a terminal state; from
// IN_PROGRESS only COMPLETE (the linked trip completed) and CANCEL; START
// only from ACCEPTED (the linked trip started) and COMPLETE only from
// IN_PROGRESS; ACCEPT and DECLINE only from ASSIGNED; PLAN never from
// ACCEPTED (an accepted plan is not rewritten), else to PLANNED; ASSIGN and
// CANCEL from DRAFT, PLANNED, ASSIGNED, ACCEPTED and DECLINED. The table
// answers INVALID_TRANSITION for every refused pair (EMP-TRIP-023).
func MissionTransition(from MissionState, command MissionCommand) (MissionState, error) {
	if _, err := ParseMissionState(string(from)); err != nil {
		return "", err
	}
	if _, err := ParseMissionCommand(string(command)); err != nil {
		return "", err
	}
	if from.Terminal() {
		return "", ErrInvalidTransition
	}
	if from == MissionInProgress {
		switch command {
		case MissionComplete:
			return MissionCompleted, nil
		case MissionCancel:
			return MissionCancelled, nil
		}
		return "", ErrInvalidTransition
	}
	switch command {
	case MissionStart:
		if from != MissionAccepted {
			return "", ErrInvalidTransition
		}
		return MissionInProgress, nil
	case MissionComplete:
		return "", ErrInvalidTransition
	case MissionPlan:
		if from == MissionAccepted {
			return "", ErrInvalidTransition
		}
		return MissionPlanned, nil
	case MissionAccept, MissionDecline:
		if from != MissionAssigned {
			return "", ErrInvalidTransition
		}
		if command == MissionAccept {
			return MissionAccepted, nil
		}
		return MissionDeclined, nil
	case MissionAssign:
		return MissionAssigned, nil
	}
	return MissionCancelled, nil
}

// MissionResponse is the assignee's answer.
type MissionResponse string

const (
	ResponseAccept  MissionResponse = "ACCEPT"
	ResponseDecline MissionResponse = "DECLINE"
)

var missionResponses = []MissionResponse{ResponseAccept, ResponseDecline}

// MissionResponses returns the closed response vocabulary.
func MissionResponses() []MissionResponse { return slices.Clone(missionResponses) }

// ParseMissionResponse accepts only an exact response token.
func ParseMissionResponse(raw string) (MissionResponse, error) {
	if !slices.Contains(missionResponses, MissionResponse(raw)) {
		return "", ErrInvalid
	}
	return MissionResponse(raw), nil
}

// Command is the mission command of a response.
func (r MissionResponse) Command() MissionCommand {
	if r == ResponseDecline {
		return MissionDecline
	}
	return MissionAccept
}

// MissionRef is a scoped mission identity; its shape proves no existence.
type MissionRef struct{ scopedID }

// UnitRef is the scoped operating unit of a mission.
type UnitRef struct{ scopedID }

// MissionAssetRef is the scoped asset a mission names.
type MissionAssetRef struct{ scopedID }

func NewMissionRef(scope kernel.OrgScope, id string) (MissionRef, error) {
	r, e := scoped(scope, id)
	return MissionRef{r}, e
}
func NewUnitRef(scope kernel.OrgScope, id string) (UnitRef, error) {
	r, e := scoped(scope, id)
	return UnitRef{r}, e
}
func NewMissionAssetRef(scope kernel.OrgScope, id string) (MissionAssetRef, error) {
	r, e := scoped(scope, id)
	return MissionAssetRef{r}, e
}

// MissionWindow is the planned half-open window [From, To).
type MissionWindow struct{ From, To time.Time }

func wholeMicrosecond(at time.Time) bool {
	return !at.IsZero() && at.Nanosecond()%1000 == 0 && at.UTC().Year() >= 1 && at.UTC().Year() <= 9999
}

// NewMissionWindow requires whole-microsecond bounds and To after From; it
// has no maximum span (EDR-228).
func NewMissionWindow(from, to time.Time) (MissionWindow, error) {
	if !wholeMicrosecond(from) || !wholeMicrosecond(to) || !to.After(from) {
		return MissionWindow{}, ErrInvalid
	}
	return MissionWindow{From: from.UTC(), To: to.UTC()}, nil
}

// ValidMissionText is the METER-001 text profile shared by titles and
// reasons: nonblank, free of control characters, at most 280 scalars.
func ValidMissionText(raw string) error {
	if _, err := meter.NewMissing(raw); err != nil {
		return ErrInvalid
	}
	return nil
}

// MissionInput is a new mission as the planner submitted it.
type MissionInput struct {
	redacted
	Reference MissionRef
	Unit      UnitRef
	Asset     *MissionAssetRef
	Title     string
	Window    MissionWindow
}

// MissionAssignee is the employee record and the membership that may answer.
type MissionAssignee struct {
	redacted
	Employee EmployeeRef
	Member   MembershipRef
}

// MissionAnswer is the assignee's stored response.
type MissionAnswer struct {
	redacted
	Kind   MissionResponse
	Member MembershipRef
	Reason *string
}

// MissionSnapshot is the stored form of a mission.
type MissionSnapshot struct {
	redacted
	Reference MissionRef
	Unit      UnitRef
	Asset     *MissionAssetRef
	Title     string
	Window    MissionWindow
	State     MissionState
	Revision  int64
	Assignee  *MissionAssignee
	Answer    *MissionAnswer
}

func (s MissionSnapshot) copy() MissionSnapshot {
	s.Asset = copyPointer(s.Asset)
	s.Assignee = copyPointer(s.Assignee)
	if s.Answer != nil {
		a := *s.Answer
		a.Reason = copyPointer(a.Reason)
		s.Answer = &a
	}
	return s
}

// Mission is one organization mission; commands return a new value and never
// change the receiver.
type Mission struct {
	redacted
	value MissionSnapshot
	valid bool
}

// Snapshot returns an independent copy of the mission.
func (m Mission) Snapshot() MissionSnapshot { return m.value.copy() }

// content checks identity, unit, optional asset, title and window, first
// failure first.
func content(scope kernel.OrgScope, reference MissionRef, unit UnitRef, asset *MissionAssetRef, title string, window MissionWindow) (MissionWindow, error) {
	if scope.Validate() != nil {
		return MissionWindow{}, ErrScope
	}
	if err := reference.validate(scope); err != nil {
		return MissionWindow{}, err
	}
	if err := unit.validate(scope); err != nil {
		return MissionWindow{}, err
	}
	if asset != nil {
		if err := asset.validate(scope); err != nil {
			return MissionWindow{}, err
		}
	}
	if err := ValidMissionText(title); err != nil {
		return MissionWindow{}, err
	}
	return NewMissionWindow(window.From, window.To)
}

// NewMission validates a new mission: DRAFT at revision 1, no assignee
// (B-BR-EMP-354).
func NewMission(scope kernel.OrgScope, in MissionInput) (Mission, error) {
	window, err := content(scope, in.Reference, in.Unit, in.Asset, in.Title, in.Window)
	if err != nil {
		return Mission{}, err
	}
	return Mission{valid: true, value: MissionSnapshot{Reference: in.Reference, Unit: in.Unit, Asset: copyPointer(in.Asset), Title: in.Title, Window: window, State: MissionDraft, Revision: 1}}, nil
}

func validAnswer(scope kernel.OrgScope, a MissionAnswer) error {
	if err := a.Member.validate(scope); err != nil {
		return err
	}
	kind, err := ParseMissionResponse(string(a.Kind))
	if err != nil {
		return err
	}
	return validReason(kind, a.Reason)
}

// validReason requires a reason for DECLINE and refuses one for ACCEPT.
func validReason(kind MissionResponse, reason *string) error {
	if kind == ResponseAccept {
		if reason != nil {
			return ErrInvalid
		}
		return nil
	}
	if reason == nil {
		return ErrInvalid
	}
	return ValidMissionText(*reason)
}

// invariant is the stored shape of each state (EDR-228).
func invariant(s MissionSnapshot) bool {
	if (s.State == MissionDraft) != (s.Revision == 1) {
		return false
	}
	if s.Answer != nil && (s.Assignee == nil || s.Answer.Member != s.Assignee.Member) {
		return false
	}
	switch s.State {
	case MissionDraft, MissionPlanned:
		return s.Assignee == nil && s.Answer == nil
	case MissionAssigned:
		return s.Assignee != nil && s.Answer == nil
	case MissionAccepted, MissionInProgress, MissionCompleted:
		return s.Answer != nil && s.Answer.Kind == ResponseAccept
	case MissionDeclined:
		return s.Answer != nil && s.Answer.Kind == ResponseDecline
	}
	return s.State == MissionCancelled
}

// RestoreMission proves the stored invariants of a mission.
func RestoreMission(scope kernel.OrgScope, s MissionSnapshot) (Mission, error) {
	window, err := content(scope, s.Reference, s.Unit, s.Asset, s.Title, s.Window)
	if err != nil {
		return Mission{}, err
	}
	if _, err := ParseMissionState(string(s.State)); err != nil {
		return Mission{}, err
	}
	if s.Revision < 1 {
		return Mission{}, ErrInvalid
	}
	if s.Assignee != nil {
		if err := s.Assignee.Employee.validate(scope); err != nil {
			return Mission{}, err
		}
		if err := s.Assignee.Member.validate(scope); err != nil {
			return Mission{}, err
		}
	}
	if s.Answer != nil {
		if err := validAnswer(scope, *s.Answer); err != nil {
			return Mission{}, err
		}
	}
	if !invariant(s) {
		return Mission{}, ErrInvalid
	}
	out := s.copy()
	out.Window = window
	return Mission{valid: true, value: out}, nil
}

// subject checks the scope and the restored mission.
func (m Mission) subject(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if !m.valid {
		return ErrInvalid
	}
	return m.value.Reference.validate(scope)
}

// decide applies the stale-revision rule then the table.
func (m Mission) decide(expected int64, command MissionCommand) (MissionState, error) {
	if expected != m.value.Revision {
		return "", ErrConflict
	}
	return MissionTransition(m.value.State, command)
}

// Assign names a new assignee (B-BR-EMP-355): scope, the new employee and
// membership, a positive expected revision, CONFLICT for a stale revision,
// the table, then the current membership again while ASSIGNED or ACCEPTED.
func (m Mission) Assign(scope kernel.OrgScope, expected int64, to MissionAssignee) (Mission, error) {
	if err := m.subject(scope); err != nil {
		return Mission{}, err
	}
	if err := to.Employee.validate(scope); err != nil {
		return Mission{}, err
	}
	if err := to.Member.validate(scope); err != nil {
		return Mission{}, err
	}
	if expected < 1 {
		return Mission{}, ErrInvalid
	}
	next, err := m.decide(expected, MissionAssign)
	if err != nil {
		return Mission{}, err
	}
	current := m.value.Assignee
	if (m.value.State == MissionAssigned || m.value.State == MissionAccepted) && current != nil && current.Member == to.Member {
		return Mission{}, ErrInvalidTransition
	}
	out := m.value.copy()
	out.State, out.Revision, out.Answer = next, m.value.Revision+1, nil
	out.Assignee = &MissionAssignee{Employee: to.Employee, Member: to.Member}
	return Mission{valid: true, value: out}, nil
}

// Respond records the current assignee's answer (B-BR-EMP-356): scope, the
// responding membership, the response token and reason, a positive expected
// revision, FORBIDDEN unless the membership is the current assignee's, then
// CONFLICT for a stale revision, then the table.
func (m Mission) Respond(scope kernel.OrgScope, expected int64, member MembershipRef, kind string, reason *string) (Mission, error) {
	if err := m.subject(scope); err != nil {
		return Mission{}, err
	}
	if err := member.validate(scope); err != nil {
		return Mission{}, err
	}
	response, err := ParseMissionResponse(kind)
	if err != nil {
		return Mission{}, err
	}
	if err := validReason(response, reason); err != nil {
		return Mission{}, err
	}
	if expected < 1 {
		return Mission{}, ErrInvalid
	}
	if m.value.Assignee == nil || m.value.Assignee.Member != member {
		return Mission{}, ErrForbidden
	}
	next, err := m.decide(expected, response.Command())
	if err != nil {
		return Mission{}, err
	}
	out := m.value.copy()
	out.State, out.Revision = next, m.value.Revision+1
	out.Answer = &MissionAnswer{Kind: response, Member: member, Reason: copyPointer(reason)}
	return Mission{valid: true, value: out}, nil
}

// Cancel ends the mission with a reason (B-BR-EMP-353), keeping the assignee
// and answer: scope, the reason, a positive expected revision, CONFLICT, then
// the table.
func (m Mission) Cancel(scope kernel.OrgScope, expected int64, reason string) (Mission, error) {
	if err := m.subject(scope); err != nil {
		return Mission{}, err
	}
	if err := ValidMissionText(reason); err != nil {
		return Mission{}, err
	}
	if expected < 1 {
		return Mission{}, ErrInvalid
	}
	next, err := m.decide(expected, MissionCancel)
	if err != nil {
		return Mission{}, err
	}
	out := m.value.copy()
	out.State, out.Revision = next, m.value.Revision+1
	return Mission{valid: true, value: out}, nil
}
