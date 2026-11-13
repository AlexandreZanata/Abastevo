package domain

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// EmployeeProfile holds organization-controlled descriptive data only. Work
// contacts are unverified notification details, never account proof or grants.
type EmployeeProfile struct {
	privateValue
	DisplayName, RegistrationNumber, WorkEmail, WorkPhone, JobTitle, HomeUnitID string
}

func profileText(raw string, max int, required bool) (string, error) {
	v := strings.TrimSpace(raw)
	if !utf8.ValidString(v) || len(v) > max || required && v == "" {
		return "", ErrInvalid
	}
	for _, r := range v {
		if unicode.IsControl(r) || unicode.IsSpace(r) && r != ' ' || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069 {
			return "", ErrInvalid
		}
	}
	return v, nil
}
func CanonicalEmployeeProfile(v EmployeeProfile) (EmployeeProfile, error) {
	var err error
	if v.DisplayName, err = profileText(v.DisplayName, 200, true); err != nil {
		return EmployeeProfile{}, err
	}
	if v.RegistrationNumber, err = profileText(v.RegistrationNumber, 64, false); err != nil {
		return EmployeeProfile{}, err
	}
	if v.JobTitle, err = profileText(v.JobTitle, 120, false); err != nil {
		return EmployeeProfile{}, err
	}
	v.WorkEmail = strings.ToLower(strings.TrimSpace(v.WorkEmail))
	v.WorkPhone = strings.TrimSpace(v.WorkPhone)
	if v.WorkEmail != "" {
		at := strings.IndexByte(v.WorkEmail, '@')
		if len(v.WorkEmail) > 254 || at < 1 || at >= len(v.WorkEmail)-1 || strings.Count(v.WorkEmail, "@") != 1 {
			return EmployeeProfile{}, ErrInvalid
		}
		for _, r := range v.WorkEmail {
			if r < 33 || r > 126 {
				return EmployeeProfile{}, ErrInvalid
			}
		}
	}
	if v.WorkPhone != "" {
		if len(v.WorkPhone) < 9 || len(v.WorkPhone) > 16 || v.WorkPhone[0] != '+' || v.WorkPhone[1] < '1' || v.WorkPhone[1] > '9' {
			return EmployeeProfile{}, ErrInvalid
		}
		for _, r := range v.WorkPhone[2:] {
			if r < '0' || r > '9' {
				return EmployeeProfile{}, ErrInvalid
			}
		}
	}
	if v.HomeUnitID != "" {
		id, e := canonicalID(v.HomeUnitID)
		if e != nil || id != v.HomeUnitID {
			return EmployeeProfile{}, ErrInvalid
		}
	}
	return v, nil
}

// EmployeeAccessState is descriptive read metadata, never membership authority.
// The zero value is unhydrated; employee lifecycle alone cannot imply access.
type EmployeeAccessState string

type EmployeeRecordSnapshot struct {
	privateValue
	Employee        Employee
	Profile         EmployeeProfile
	ProfileRevision int64
	AccessState     EmployeeAccessState
}
type EmployeeRecord struct {
	privateValue
	value EmployeeRecordSnapshot
}

func RehydrateEmployeeRecord(scope kernel.OrgScope, v EmployeeRecordSnapshot) (EmployeeRecord, error) {
	if _, err := v.Employee.Snapshot(scope); err != nil {
		return EmployeeRecord{}, err
	}
	switch v.AccessState {
	case "", "NONE", "INVITED", "ACTIVE", "SUSPENDED":
	default:
		return EmployeeRecord{}, ErrInvalid
	}
	if v.ProfileRevision == 0 {
		if v.Profile != (EmployeeProfile{}) {
			return EmployeeRecord{}, ErrInvalid
		}
	} else {
		p, err := CanonicalEmployeeProfile(v.Profile)
		if err != nil || p != v.Profile || v.ProfileRevision < 1 {
			return EmployeeRecord{}, ErrInvalid
		}
	}
	return EmployeeRecord{value: v}, nil
}
func (r EmployeeRecord) Snapshot(scope kernel.OrgScope) (EmployeeRecordSnapshot, error) {
	if _, err := RehydrateEmployeeRecord(scope, r.value); err != nil {
		return EmployeeRecordSnapshot{}, err
	}
	return r.value, nil
}
func (r EmployeeRecord) CompleteProfile(scope kernel.OrgScope, expectedIdentity int64, p EmployeeProfile) (EmployeeRecord, error) {
	v, err := r.Snapshot(scope)
	if err != nil {
		return EmployeeRecord{}, err
	}
	e, _ := v.Employee.Snapshot(scope)
	if v.ProfileRevision != 0 {
		return EmployeeRecord{}, ErrTransition
	}
	if e.Revision != expectedIdentity {
		return EmployeeRecord{}, ErrConflict
	}
	if e.MergedInto != "" {
		return EmployeeRecord{}, ErrMerged
	}
	p, err = CanonicalEmployeeProfile(p)
	if err != nil {
		return EmployeeRecord{}, err
	}
	v.Profile = p
	v.ProfileRevision = 1
	return RehydrateEmployeeRecord(scope, v)
}
func (r EmployeeRecord) ChangeProfile(scope kernel.OrgScope, expected int64, p EmployeeProfile) (EmployeeRecord, error) {
	v, err := r.Snapshot(scope)
	if err != nil {
		return EmployeeRecord{}, err
	}
	if v.ProfileRevision == 0 {
		return EmployeeRecord{}, ErrTransition
	}
	if expected != v.ProfileRevision {
		return EmployeeRecord{}, ErrConflict
	}
	if expected == math.MaxInt64 {
		return EmployeeRecord{}, ErrRevision
	}
	e, _ := v.Employee.Snapshot(scope)
	if e.MergedInto != "" {
		return EmployeeRecord{}, ErrMerged
	}
	p, err = CanonicalEmployeeProfile(p)
	if err != nil {
		return EmployeeRecord{}, err
	}
	v.Profile = p
	v.ProfileRevision++
	return RehydrateEmployeeRecord(scope, v)
}
