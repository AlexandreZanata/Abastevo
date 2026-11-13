package application

import (
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
)

// DeviceContactRequest selects one previously enrolled period. Identifiers are
// selectors only; the retained source transaction must prove their ownership.
type DeviceContactRequest struct {
	privateRegister
	RequestID, MembershipID, EnrollmentID string
}

func (r DeviceContactRequest) Validate() error {
	for _, id := range []string{r.RequestID, r.MembershipID, r.EnrollmentID} {
		if !canonicalEmployeeID(id) {
			return d.ErrInvalid
		}
	}
	return nil
}

type ContactVerdict string

const (
	ContactActive                ContactVerdict = "ACTIVE"
	ContactLock                  ContactVerdict = "LOCK"
	ContactErase                 ContactVerdict = "ERASE"
	DeviceOfflineValiditySeconds int64          = 30 * 24 * 60 * 60
)

// DeviceContact is a current custody observation, never a business permission or
// an immutable command receipt. Terminal observations contain no renewal policy.
// Managed custody remains unavailable until its owning policy source exists.
type DeviceContact struct {
	privateRegister
	RequestID, AccountID, FamilyID, MembershipID, EnrollmentID string
	Revision                                                   int64
	ServerTime                                                 time.Time
	Verdict                                                    ContactVerdict
	AccessModel                                                d.AccessModel
	ValiditySeconds                                            int64
	RevocationClass                                            d.RevocationClass
}

func (v DeviceContact) Validate(a DeviceActor, r DeviceContactRequest) error {
	if a.Validate() != nil || r.Validate() != nil || v.RequestID != r.RequestID || v.AccountID != a.AccountID || v.FamilyID != a.FamilyID || v.MembershipID != r.MembershipID || v.EnrollmentID != r.EnrollmentID || v.Revision < 1 || v.ServerTime.IsZero() || v.ServerTime.UTC().Year() < 1 || v.ServerTime.UTC().Year() > 9999 {
		return ErrRegisterUnavailable
	}
	switch v.Verdict {
	case ContactActive:
		if v.AccessModel != d.ExistingAccount || v.ValiditySeconds != DeviceOfflineValiditySeconds || v.RevocationClass != "" {
			return ErrRegisterUnavailable
		}
	case ContactErase, ContactLock:
		if v.AccessModel != "" || v.ValiditySeconds != 0 || v.Verdict == ContactErase && v.RevocationClass != d.LostOrCompromised || v.Verdict == ContactLock && v.RevocationClass != d.AccessEnded {
			return ErrRegisterUnavailable
		}
	default:
		return ErrRegisterUnavailable
	}
	return nil
}
