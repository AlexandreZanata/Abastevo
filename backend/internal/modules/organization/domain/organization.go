// Package domain models scoped organization values, not actor authority or lifecycle side effects.
package domain

import (
	"errors"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalid    = errors.New("organization: invalid input")
	ErrScope      = errors.New("organization: scope denied")
	ErrConflict   = errors.New("organization: stale revision")
	ErrRevision   = errors.New("organization: revision exhausted")
	ErrClosed     = errors.New("organization: closed")
	ErrTransition = errors.New("organization: transition not allowed")
	ErrConditions = errors.New("organization: lifecycle conditions not confirmed")
	ErrSetup      = errors.New("organization: setup incomplete")
	zoneName      = regexp.MustCompile("^[A-Za-z0-9_+/-]{1,128}$")
)

type State string

const (
	PendingSetup State = "PENDING_SETUP"
	Active       State = "ACTIVE"
	Suspended    State = "SUSPENDED"
	Archived     State = "ARCHIVED"
	Closed       State = "CLOSED"
)

func validState(state State) bool {
	switch state {
	case PendingSetup, Active, Suspended, Archived, Closed:
		return true
	}
	return false
}

type Kind string

const (
	Delivery Kind = "DELIVERY"
	Carrier  Kind = "CARRIER"
	Services Kind = "SERVICES"
	Farm     Kind = "FARM"
	Other    Kind = "OTHER"
)

func validKind(kind Kind) bool {
	switch kind {
	case Delivery, Carrier, Services, Farm, Other:
		return true
	}
	return false
}

type Reason string

const (
	Billing      Reason = "BILLING"
	Terms        Reason = "TERMS"
	Security     Reason = "SECURITY"
	Legal        Reason = "LEGAL"
	OwnerRequest Reason = "OWNER_REQUEST"
)

func validReason(reason Reason) bool {
	switch reason {
	case Billing, Terms, Security, Legal, OwnerRequest:
		return true
	}
	return false
}

type ClosureDisposition string

const (
	ClosureUnknown   ClosureDisposition = "UNKNOWN"
	ClosureNone      ClosureDisposition = "NONE"
	ClosurePending   ClosureDisposition = "PENDING"
	ClosureElapsed   ClosureDisposition = "ELAPSED"
	ClosureCancelled ClosureDisposition = "CANCELLED"
)

// Profile carries descriptions only. Empty name/zone are representable during pending setup.
type Profile struct {
	Name     string
	TimeZone string
	Kinds    []Kind
}

func (Profile) String() string         { return "OrganizationProfile(redacted)" }
func (p Profile) GoString() string     { return p.String() }
func (p Profile) LogValue() slog.Value { return slog.StringValue(p.String()) }

func validZone(id string) bool {
	if !zoneName.MatchString(id) || id == "Local" || strings.Contains(id, "//") ||
		strings.HasPrefix(id, "posix/") || strings.HasPrefix(id, "right/") {
		return false
	}
	_, err := time.LoadLocation(id)
	return err == nil
}
func canonical(profile Profile) (Profile, error) {
	if !utf8.ValidString(profile.Name) || (profile.TimeZone != "" && !validZone(profile.TimeZone)) {
		return Profile{}, ErrInvalid
	}
	set := map[Kind]bool{}
	for _, kind := range profile.Kinds {
		if !validKind(kind) {
			return Profile{}, ErrInvalid
		}
		set[kind] = true
	}
	var kinds []Kind
	for kind := range set {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return Profile{Name: profile.Name, TimeZone: profile.TimeZone, Kinds: kinds}, nil
}
func clone(profile Profile) Profile {
	profile.Kinds = append([]Kind(nil), profile.Kinds...)
	return profile
}

// Snapshot is a scoped value, not a wire command, proof or permission decision.
type Snapshot struct {
	ID               kernel.OrgID
	Profile          Profile
	State            State
	Revision         int64
	SuspensionReason Reason
}

func (Snapshot) String() string         { return "OrganizationSnapshot(redacted)" }
func (s Snapshot) GoString() string     { return s.String() }
func (s Snapshot) LogValue() slog.Value { return slog.StringValue(s.String()) }

type Organization struct{ value Snapshot }

func (Organization) String() string         { return "Organization(redacted)" }
func (o Organization) GoString() string     { return o.String() }
func (o Organization) LogValue() slog.Value { return slog.StringValue(o.String()) }

// NewPending creates only the pure root. The owning command atomically creates unit/employee/owner.
func NewPending(scope kernel.OrgScope, profile Profile) (Organization, error) {
	return Rehydrate(scope, Snapshot{ID: scope.OrgID(), Profile: profile, State: PendingSetup, Revision: 1})
}

// Rehydrate accepts trusted storage state only. It must never be bound to a client DTO.
func Rehydrate(scope kernel.OrgScope, snapshot Snapshot) (Organization, error) {
	if scope.Validate() != nil || snapshot.ID != scope.OrgID() {
		return Organization{}, ErrScope
	}
	profile, err := canonical(snapshot.Profile)
	if err != nil || !validState(snapshot.State) || snapshot.Revision < 1 {
		return Organization{}, ErrInvalid
	}
	if (snapshot.SuspensionReason != "" && !validReason(snapshot.SuspensionReason)) ||
		(snapshot.State == Suspended && snapshot.SuspensionReason == "") ||
		((snapshot.State == PendingSetup || snapshot.State == Active) && snapshot.SuspensionReason != "") {
		return Organization{}, ErrInvalid
	}
	if snapshot.State != PendingSetup && snapshot.State != Closed &&
		(strings.TrimSpace(profile.Name) == "" || profile.TimeZone == "") {
		return Organization{}, ErrInvalid
	}
	snapshot.Profile = profile
	return Organization{value: snapshot}, nil
}
func (o Organization) check(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if o.value.ID == (kernel.OrgID{}) {
		return ErrInvalid
	}
	if scope.OrgID() != o.value.ID {
		return ErrScope
	}
	return nil
}
func (o Organization) Snapshot(scope kernel.OrgScope) (Snapshot, error) {
	if err := o.check(scope); err != nil {
		return Snapshot{}, err
	}
	snapshot := o.value
	snapshot.Profile = clone(snapshot.Profile)
	return snapshot, nil
}
func (o Organization) changing(scope kernel.OrgScope, expected int64) error {
	if err := o.check(scope); err != nil {
		return err
	}
	if o.value.State == Closed {
		return ErrClosed
	}
	if expected != o.value.Revision {
		return ErrConflict
	}
	if o.value.Revision == math.MaxInt64 {
		return ErrRevision
	}
	return nil
}

// ConfigurePending does not activate. Versioned active settings belong to their owning task.
func (o Organization) ConfigurePending(scope kernel.OrgScope, expected int64, profile Profile) (Organization, error) {
	if err := o.changing(scope, expected); err != nil {
		return Organization{}, err
	}
	if o.value.State != PendingSetup {
		return Organization{}, ErrTransition
	}
	profile, err := canonical(profile)
	if err != nil {
		return Organization{}, err
	}
	result := o
	result.value.Profile = profile
	result.value.Revision++
	return result, nil
}

// Conditions are facts resolved under current server authority, never caller permission claims.
// ArchiveClear means the owning command has checked the blocking-items report.
// Closure is a confirmed schedule result; this value acquires no clock or timer.
type Conditions struct {
	ActiveUnits          int64
	ActiveOwners         int64
	TermsRecorded        bool
	NoOperationalRecords bool
	SuspensionReason     Reason
	SuspensionResolved   bool
	ArchiveClear         bool
	Closure              ClosureDisposition
}

type Missing string

const (
	MissingName     Missing = "NAME"
	MissingTimeZone Missing = "TIME_ZONE"
	MissingUnit     Missing = "ACTIVE_UNIT"
	MissingOwner    Missing = "ACTIVE_OWNER"
	MissingTerms    Missing = "TERMS_ACCEPTANCE"
)

type SetupError struct{ missing []Missing }

func (*SetupError) Error() string        { return ErrSetup.Error() }
func (*SetupError) Unwrap() error        { return ErrSetup }
func (e *SetupError) Missing() []Missing { return append([]Missing(nil), e.missing...) }

func (o Organization) activation(facts Conditions) error {
	if facts.ActiveUnits < 0 || facts.ActiveOwners < 0 {
		return ErrInvalid
	}
	var missing []Missing
	if strings.TrimSpace(o.value.Profile.Name) == "" {
		missing = append(missing, MissingName)
	}
	if o.value.Profile.TimeZone == "" {
		missing = append(missing, MissingTimeZone)
	}
	if facts.ActiveUnits == 0 {
		missing = append(missing, MissingUnit)
	}
	if facts.ActiveOwners == 0 {
		missing = append(missing, MissingOwner)
	}
	if !facts.TermsRecorded {
		missing = append(missing, MissingTerms)
	}
	if len(missing) > 0 {
		return &SetupError{missing: missing}
	}
	return nil
}

// Transition returns a new value only. Application commands own permission, audit and side effects.
func (o Organization) Transition(scope kernel.OrgScope, expected int64, target State, facts Conditions) (Organization, error) {
	if err := o.changing(scope, expected); err != nil {
		return Organization{}, err
	}
	reason := o.value.SuspensionReason
	switch [2]State{o.value.State, target} {
	case [2]State{PendingSetup, Active}:
		if err := o.activation(facts); err != nil {
			return Organization{}, err
		}
	case [2]State{PendingSetup, Closed}:
		if !facts.NoOperationalRecords {
			return Organization{}, ErrConditions
		}
	case [2]State{Active, Suspended}:
		if !validReason(facts.SuspensionReason) {
			return Organization{}, ErrConditions
		}
		reason = facts.SuspensionReason
	case [2]State{Suspended, Active}:
		if !facts.SuspensionResolved {
			return Organization{}, ErrConditions
		}
		reason = ""
	case [2]State{Active, Archived}, [2]State{Suspended, Archived}:
		if !facts.ArchiveClear {
			return Organization{}, ErrConditions
		}
	case [2]State{Archived, Active}, [2]State{Archived, Suspended}:
		if facts.Closure != ClosureNone && facts.Closure != ClosureCancelled {
			return Organization{}, ErrConditions
		}
		if target == Active {
			if !facts.SuspensionResolved {
				return Organization{}, ErrConditions
			}
			reason = ""
		} else {
			if facts.SuspensionResolved || !validReason(facts.SuspensionReason) {
				return Organization{}, ErrConditions
			}
			reason = facts.SuspensionReason
		}
	case [2]State{Archived, Closed}:
		if facts.Closure != ClosureElapsed {
			return Organization{}, ErrConditions
		}
	default:
		return Organization{}, ErrTransition
	}
	result := o
	result.value.State = target
	result.value.SuspensionReason = reason
	result.value.Revision++
	return result, nil
}
