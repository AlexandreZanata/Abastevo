package domain

import "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"

// EMP-TRIP-023 reservation request of a mission's vehicle (B-BR-EMP-414,
// EDR-266). Like the EMP-TRIP-013 vehicle change it is a server-only
// operation of the mission, not a lifecycle command: the seven-command
// vocabulary is unchanged and the reservation itself belongs to Fleet.

// ReservationRequestAllowed reports the states whose vehicle may be
// reserved for the mission: PLANNED, ASSIGNED, ACCEPTED and DECLINED. A
// DRAFT mission exists only at revision 1 (B-BR-EMP-353); an IN_PROGRESS
// mission is executing and the terminal states never change.
func ReservationRequestAllowed(s MissionState) error {
	if _, err := ParseMissionState(string(s)); err != nil {
		return err
	}
	if s.Terminal() || s == MissionDraft || s == MissionInProgress {
		return ErrInvalidTransition
	}
	return nil
}

// Reserve records a reservation request at the next revision, keeping state,
// assignee and answer: scope, a positive expected revision, CONFLICT for a
// stale revision, the state, then INVALID for a mission without a vehicle.
func (m Mission) Reserve(scope kernel.OrgScope, expected int64) (Mission, error) {
	if err := m.subject(scope); err != nil {
		return Mission{}, err
	}
	if expected < 1 {
		return Mission{}, ErrInvalid
	}
	if expected != m.value.Revision {
		return Mission{}, ErrConflict
	}
	if err := ReservationRequestAllowed(m.value.State); err != nil {
		return Mission{}, err
	}
	if m.value.Asset == nil {
		return Mission{}, ErrInvalid
	}
	out := m.value.copy()
	out.Revision = m.value.Revision + 1
	return Mission{valid: true, value: out}, nil
}
