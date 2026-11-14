package domain

import "time"

type AdmissionState string

const (
	AdmissionWaiting   AdmissionState = "WAITING"
	AdmissionActive    AdmissionState = "ACTIVE"
	AdmissionRejected  AdmissionState = "REJECTED"
	AdmissionExpired   AdmissionState = "EXPIRED"
	AdmissionCancelled AdmissionState = "ENDED"
)

type Admission struct {
	privateValue
	state      AdmissionState
	acceptedAt time.Time
}

func InvitationNeedsConfirmation(grants []InvitationGrant, required bool) (bool, error) {
	values, e := CanonicalInvitationGrants(grants)
	if e != nil {
		return false, e
	}
	for _, g := range values {
		if g.Role != "DRIVER" && g.Role != "OPERATIONS_STAFF" {
			required = true
		}
	}
	return required, nil
}
func NewAdmission(state AdmissionState, at time.Time) (Admission, error) {
	switch state {
	case AdmissionWaiting, AdmissionActive, AdmissionRejected, AdmissionExpired, AdmissionCancelled:
	default:
		return Admission{}, ErrInvalid
	}
	if !invitationInstant(at) {
		return Admission{}, ErrInvalid
	}
	return Admission{state: state, acceptedAt: at.UTC()}, nil
}
func (a Admission) CanConfirmAt(now time.Time) bool {
	return a.state == AdmissionWaiting && !now.Before(a.acceptedAt) && now.Before(a.acceptedAt.Add(7*24*time.Hour))
}
