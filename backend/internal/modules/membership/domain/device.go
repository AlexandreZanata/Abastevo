package domain

import (
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

type DeviceState string

const (
	DeviceEnrolled DeviceState = "ENROLLED"
	DeviceRevoked  DeviceState = "REVOKED"
)

type RevocationClass string

const (
	LostOrCompromised RevocationClass = "LOST_OR_COMPROMISED"
	AccessEnded       RevocationClass = "ACCESS_ENDED"
)

func validRevocation(c RevocationClass) bool { return c == LostOrCompromised || c == AccessEnded }

type DeviceSnapshot struct {
	privateValue
	OrgID                                   kernel.OrgID
	ID, MembershipID, DeviceID, Fingerprint string
	State                                   DeviceState
	Revision                                int64
	Revocation                              RevocationClass
}
type DeviceEnrollment struct {
	privateValue
	value DeviceSnapshot
}

// A count/limit is resolved under the owning membership lock, not from a client.
type EnrollmentConditions struct {
	ProofConfirmed               bool
	EnrolledDevices, DeviceLimit int64
}

func EnrollDevice(scope kernel.OrgScope, id, deviceID, fingerprint string, member Membership, facts EnrollmentConditions) (DeviceEnrollment, error) {
	m, err := member.Snapshot(scope)
	if err != nil {
		return DeviceEnrollment{}, err
	}
	if m.State != MembershipActive || !facts.ProofConfirmed || facts.EnrolledDevices < 0 || facts.DeviceLimit < 1 || facts.DeviceLimit > 5 || facts.EnrolledDevices >= facts.DeviceLimit || (m.Model == ManagedAccess && facts.DeviceLimit != 1) {
		return DeviceEnrollment{}, ErrConditions
	}
	return RehydrateDevice(scope, DeviceSnapshot{OrgID: scope.OrgID(), ID: id, MembershipID: m.ID, DeviceID: deviceID, Fingerprint: fingerprint, State: DeviceEnrolled, Revision: 1})
}
func RehydrateDevice(scope kernel.OrgScope, snapshot DeviceSnapshot) (DeviceEnrollment, error) {
	if scope.Validate() != nil || snapshot.OrgID != scope.OrgID() {
		return DeviceEnrollment{}, ErrScope
	}
	var err error
	snapshot.ID, err = canonicalID(snapshot.ID)
	if err != nil {
		return DeviceEnrollment{}, err
	}
	snapshot.MembershipID, err = canonicalID(snapshot.MembershipID)
	if err != nil {
		return DeviceEnrollment{}, err
	}
	snapshot.DeviceID, err = canonicalID(snapshot.DeviceID)
	if err != nil {
		return DeviceEnrollment{}, err
	}
	fp, err := identity.ParseFingerprint(snapshot.Fingerprint)
	if err != nil || fp != snapshot.Fingerprint || snapshot.Revision < 1 {
		return DeviceEnrollment{}, ErrInvalid
	}
	switch snapshot.State {
	case DeviceEnrolled:
		if snapshot.Revocation != "" {
			return DeviceEnrollment{}, ErrInvalid
		}
	case DeviceRevoked:
		if !validRevocation(snapshot.Revocation) {
			return DeviceEnrollment{}, ErrInvalid
		}
	default:
		return DeviceEnrollment{}, ErrInvalid
	}
	return DeviceEnrollment{value: snapshot}, nil
}
func (d DeviceEnrollment) Snapshot(scope kernel.OrgScope) (DeviceSnapshot, error) {
	if err := scoped(scope, d.value.OrgID, d.value.ID, d.value.Revision); err != nil {
		return DeviceSnapshot{}, err
	}
	return d.value, nil
}
func (d DeviceEnrollment) Transition(scope kernel.OrgScope, expected int64, to DeviceState, class RevocationClass) (DeviceEnrollment, error) {
	v := d.value
	if err := changing(scope, v.OrgID, v.ID, v.Revision, expected); err != nil {
		return DeviceEnrollment{}, err
	}
	if v.State == DeviceRevoked {
		return DeviceEnrollment{}, ErrRevoked
	}
	if to != DeviceRevoked {
		return DeviceEnrollment{}, ErrTransition
	}
	if !validRevocation(class) {
		return DeviceEnrollment{}, ErrConditions
	}
	v.State = to
	v.Revision++
	v.Revocation = class
	return DeviceEnrollment{value: v}, nil
}
