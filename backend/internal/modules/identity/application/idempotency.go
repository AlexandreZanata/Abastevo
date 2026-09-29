package application

// Action is the idempotency decision for one attempt.
type Action int

const (
	// ActionExecute runs the business write: no usable record exists.
	ActionExecute Action = iota
	// ActionReplay returns the stored outcome without executing.
	ActionReplay
	// ActionConflict refuses: same key, different body.
	ActionConflict
	// ActionReserve retries the reservation: the stored record expired or
	// belonged to an aborted attempt.
	ActionReserve
)

// Decide maps a stored attempt to an action. Expired and unfinished rows
// behave as absent; only a completed row with the same body hash replays,
// and a completed row with a different hash conflicts.
func Decide(found, completed, expired, sameHash bool) Action {
	if !found {
		return ActionExecute
	}
	if expired || !completed {
		return ActionReserve
	}
	if !sameHash {
		return ActionConflict
	}
	return ActionReplay
}
