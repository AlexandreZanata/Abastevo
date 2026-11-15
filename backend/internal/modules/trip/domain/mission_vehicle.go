package domain

import "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"

// EMP-TRIP-013 vehicle change within a mission (B-BR-EMP-387, EDR-250). It is
// a server-only operation of the mission, not a lifecycle command: the
// seven-command vocabulary and MissionTransition are unchanged.

// VehicleChangeAllowed reports the states whose vehicle may change: PLANNED,
// ASSIGNED, ACCEPTED and DECLINED. A DRAFT mission exists only at revision 1
// (B-BR-EMP-353), so it keeps its creation vehicle until it is planned;
// IN_PROGRESS belongs to the trip link (EMP-TRIP-021/023) and the terminal
// states never change.
func VehicleChangeAllowed(s MissionState) error {
	if _, err := ParseMissionState(string(s)); err != nil {
		return err
	}
	if s.Terminal() || s == MissionDraft {
		return ErrInvalidTransition
	}
	if s == MissionInProgress {
		return ErrUnsupportedCommand
	}
	return nil
}

// ChangeVehicle names the mission's new vehicle with a reason: scope, the
// vehicle reference, the reason, a positive expected revision, CONFLICT for a
// stale revision, the state, then INVALID_TRANSITION for the current vehicle
// again. The result keeps state, assignee and answer at the next revision.
func (m Mission) ChangeVehicle(scope kernel.OrgScope, expected int64, to MissionAssetRef, reason string) (Mission, error) {
	if err := m.subject(scope); err != nil {
		return Mission{}, err
	}
	if err := to.validate(scope); err != nil {
		return Mission{}, err
	}
	if err := ValidMissionText(reason); err != nil {
		return Mission{}, err
	}
	if expected < 1 {
		return Mission{}, ErrInvalid
	}
	if expected != m.value.Revision {
		return Mission{}, ErrConflict
	}
	if err := VehicleChangeAllowed(m.value.State); err != nil {
		return Mission{}, err
	}
	if m.value.Asset != nil && *m.value.Asset == to {
		return Mission{}, ErrInvalidTransition
	}
	out := m.value.copy()
	out.Revision, out.Asset = m.value.Revision+1, &to
	return Mission{valid: true, value: out}, nil
}
