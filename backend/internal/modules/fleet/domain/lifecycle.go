package domain

import "errors"

// ErrTransition reports a lifecycle command from a state its §6.1 row does not
// name. Adapters map it to CONFLICT; it never changes anything by itself.
var ErrTransition = errors.New("fleet: lifecycle transition unavailable")

// LifecycleCommand names one EMP-FLEET-003 command. ACTIVATE stays with the
// FLEET-002 register command and its activation checklist.
type LifecycleCommand string

const (
	RecordAcquisition LifecycleCommand = "RECORD_ACQUISITION"
	Deactivate        LifecycleCommand = "DEACTIVATE"
	Reactivate        LifecycleCommand = "REACTIVATE"
	Dispose           LifecycleCommand = "DISPOSE"
	RevertDisposal    LifecycleCommand = "REVERT_DISPOSAL"
	Archive           LifecycleCommand = "ARCHIVE"
)

// LifecycleCommands lists every command in specification order.
func LifecycleCommands() []LifecycleCommand {
	return []LifecycleCommand{RecordAcquisition, Deactivate, Reactivate, Dispose, RevertDisposal, Archive}
}

// DisposalKind is the closed sale/write-off/return vocabulary of BR-FLEET-009.
type DisposalKind string

const (
	Sale           DisposalKind = "SALE"
	WriteOff       DisposalKind = "WRITE_OFF"
	ReturnToHolder DisposalKind = "RETURN_TO_HOLDER"
)

func ParseDisposalKind(raw string) (DisposalKind, error) {
	switch kind := DisposalKind(raw); kind {
	case Sale, WriteOff, ReturnToHolder:
		return kind, nil
	}
	return "", ErrInvalid
}

// lifecycleRows is fleet.md §6.1 for the EMP-FLEET-003 commands. Recording the
// acquisition keeps the state; ARCHIVED has no outgoing row (terminal).
var lifecycleRows = map[LifecycleCommand]map[State]State{
	RecordAcquisition: {Draft: Draft, Active: Active, Inactive: Inactive},
	Deactivate:        {Active: Inactive},
	Reactivate:        {Inactive: Active},
	Dispose:           {Active: Disposed, Inactive: Disposed},
	RevertDisposal:    {Disposed: Inactive},
	Archive:           {Disposed: Archived},
}

// Transition answers the target state of command from the given source state.
func Transition(from State, command LifecycleCommand) (State, error) {
	if to, ok := lifecycleRows[command][from]; ok {
		return to, nil
	}
	return "", ErrTransition
}

// RequiresReason reports whether the §6.1 row requires a free-text reason.
func RequiresReason(command LifecycleCommand) bool {
	return command == Deactivate || command == Dispose || command == RevertDisposal
}

// ChecksOpenWork reports whether BR-FLEET-010 applies to the command target.
func ChecksOpenWork(command LifecycleCommand) bool {
	return command == Deactivate || command == Dispose || command == Archive
}

// ValidReason applies the shared 280-scalar free-text rule (fleet §10.1).
func ValidReason(raw string) bool {
	return validMetadata(raw, 280) && !blank(raw)
}
