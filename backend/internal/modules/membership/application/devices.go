package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/jackc/pgx/v5"
)

var (
	ErrDeviceRevoked  = errors.New("membership: organization device revoked")
	ErrDeviceLimit    = errors.New("membership: organization device limit")
	ErrDeviceConflict = errors.New("membership: organization device conflict")
)

// DeviceActor contains metadata established by the real signed account entry.
// Validation checks shape only; the source store rechecks live account/family,
// key ownership and proof expiry under its retained transaction locks.
type DeviceActor struct {
	privateRegister
	AccountID, FamilyID, AccessToken, ContributorID, KeyID, Fingerprint string
	ProofExpires                                                        time.Time
}

func (a DeviceActor) Validate() error {
	for _, id := range []string{a.AccountID, a.FamilyID, a.ContributorID, a.KeyID} {
		if !canonicalEmployeeID(id) {
			return d.ErrInvalid
		}
	}
	fingerprint, e := identity.ParseFingerprint(a.Fingerprint)
	if e != nil || fingerprint != a.Fingerprint || len(a.AccessToken) == 0 || len(a.AccessToken) > 512 || a.ProofExpires.IsZero() {
		return d.ErrInvalid
	}
	return nil
}

type DeviceRecord struct {
	privateRegister
	Enrollment            d.DeviceEnrollment
	Label                 string
	EnrolledAt            time.Time
	LastSyncAt, RevokedAt *time.Time
	SourceType, SourceID  string
	SourceRevision        int64
}

func (v DeviceRecord) Validate(scope kernel.OrgScope) error {
	state, e := v.Enrollment.Snapshot(scope)
	if e != nil {
		return e
	}
	if d.ValidateDeviceLabel(v.Label) != nil || v.EnrolledAt.IsZero() || v.LastSyncAt != nil {
		return ErrRegisterUnavailable
	}
	switch state.State {
	case d.DeviceEnrolled:
		if v.RevokedAt != nil || v.SourceType != "" || v.SourceID != "" || v.SourceRevision != 0 {
			return ErrRegisterUnavailable
		}
	case d.DeviceRevoked:
		if v.RevokedAt == nil || v.RevokedAt.IsZero() || v.RevokedAt.Before(v.EnrolledAt) || !canonicalEmployeeID(v.SourceID) {
			return ErrRegisterUnavailable
		}
		switch v.SourceType {
		case "COMMAND":
			if v.SourceRevision != 1 {
				return ErrRegisterUnavailable
			}
		case "MEMBERSHIP":
			if v.SourceID != state.MembershipID || v.SourceRevision < 1 || state.Revocation != d.AccessEnded {
				return ErrRegisterUnavailable
			}
		default:
			return ErrRegisterUnavailable
		}
	default:
		return ErrRegisterUnavailable
	}
	return nil
}

type DeviceCommand struct {
	privateRegister
	OperationID, DeviceID, Label, Reason string
	ExpectedRevision                     int64
	Class                                d.RevocationClass
}

func (c DeviceCommand) Validate(enroll bool) error {
	if !canonicalEmployeeID(c.OperationID) {
		return d.ErrInvalid
	}
	if enroll {
		if c.DeviceID != "" || c.ExpectedRevision != 0 || c.Class != "" || c.Reason != "" {
			return d.ErrInvalid
		}
		return d.ValidateDeviceLabel(c.Label)
	}
	if !canonicalEmployeeID(c.DeviceID) || c.ExpectedRevision < 1 || c.ExpectedRevision == math.MaxInt64 || c.Label != "" || (c.Class != d.LostOrCompromised && c.Class != d.AccessEnded) {
		return d.ErrInvalid
	}
	return d.ValidateAccessChangeReason(c.Reason)
}

func (c DeviceCommand) Digest(scope kernel.OrgScope, enroll bool) string {
	if scope.Validate() != nil || c.Validate(enroll) != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "idn.device.command", scope.OrgID().String(), enroll, c.OperationID, c.DeviceID, c.ExpectedRevision, c.Label, c.Class, c.Reason})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type DeviceResult struct {
	privateRegister
	Device              DeviceRecord
	OperationID, Reason string
	Replayed            bool
}

type DevicePage struct {
	privateRegister
	Devices []DeviceRecord
	NextID  string
}

func DevicePolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ReduceAccess
	}
	return access.Policy{Permission: "idn.credential.manage", Capability: "organizations", Effect: effect, Fresh: true}
}

type DeviceAuthority interface {
	CheckManagerDeviceTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string, bool) error
	CheckOwnDeviceTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string, bool) error
}

type DeviceRepository interface {
	PrepareDeviceEnrollment(context.Context, kernel.OrgScope, DeviceActor) (string, error)
	EnrollDevice(context.Context, kernel.OrgScope, DeviceActor, DeviceCommand) (DeviceResult, error)
	CheckDevice(context.Context, kernel.OrgScope, DeviceActor) (DeviceRecord, error)
	ListDevices(context.Context, kernel.OrgScope, EmployeeActor, string, string, bool, string, int) (DevicePage, error)
	RevokeDevice(context.Context, kernel.OrgScope, EmployeeActor, string, string, bool, DeviceCommand) (DeviceResult, error)
}
