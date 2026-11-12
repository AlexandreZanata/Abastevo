package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalid    = errors.New("membership: invalid input")
	ErrScope      = errors.New("membership: scope denied")
	ErrConflict   = errors.New("membership: stale revision")
	ErrRevision   = errors.New("membership: revision exhausted")
	ErrTransition = errors.New("membership: transition not allowed")
	ErrConditions = errors.New("membership: lifecycle conditions not confirmed")
	ErrEnded      = errors.New("membership: period ended")
	ErrMerged     = errors.New("membership: employee merged away")
	ErrRevoked    = errors.New("membership: device revoked")
)

// privateValue makes formatting (all verbs), nested JSON and slog safe even
// for snapshots. Explicit field mapping is required at a storage boundary.
type privateValue struct{}

func (privateValue) String() string                 { return "IdentityValue(redacted)" }
func (p privateValue) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p privateValue) LogValue() slog.Value         { return slog.StringValue(p.String()) }
func (p privateValue) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }

// canonicalID reuses the existing nonzero UUID parser for shape only. It
// neither interprets the value as an organization nor proves it exists.
func canonicalID(raw string) (string, error) {
	id, err := kernel.ParseOrgID(raw)
	if err != nil {
		return "", ErrInvalid
	}
	return id.String(), nil
}
func scoped(scope kernel.OrgScope, org kernel.OrgID, id string, revision int64) error {
	if scope.Validate() != nil || scope.OrgID() != org {
		return ErrScope
	}
	if id == "" || revision < 1 {
		return ErrInvalid
	}
	return nil
}
func changing(scope kernel.OrgScope, org kernel.OrgID, id string, revision, expected int64) error {
	if err := scoped(scope, org, id, revision); err != nil {
		return err
	}
	if expected != revision {
		return ErrConflict
	}
	if revision == math.MaxInt64 {
		return ErrRevision
	}
	return nil
}

type AccessModel string

const (
	ExistingAccount AccessModel = "EXISTING_ACCOUNT"
	ManagedAccess   AccessModel = "MANAGED"
)

func validModel(model AccessModel) bool { return model == ExistingAccount || model == ManagedAccess }

// AccountRef references an existing account; it has no account status, alias,
// secrets, sessions or grant. A managed reference cannot cross organizations.
type AccountRef struct {
	privateValue
	id    string
	model AccessModel
	org   kernel.OrgID
}

func PersonalAccount(raw string) (AccountRef, error) {
	id, err := canonicalID(raw)
	if err != nil {
		return AccountRef{}, err
	}
	return AccountRef{id: id, model: ExistingAccount}, nil
}
func ManagedAccount(scope kernel.OrgScope, raw string) (AccountRef, error) {
	if scope.Validate() != nil {
		return AccountRef{}, ErrScope
	}
	id, err := canonicalID(raw)
	if err != nil {
		return AccountRef{}, err
	}
	return AccountRef{id: id, model: ManagedAccess, org: scope.OrgID()}, nil
}
func (a AccountRef) check(scope kernel.OrgScope) error {
	if scope.Validate() != nil || (a.model == ManagedAccess && a.org != scope.OrgID()) {
		return ErrScope
	}
	if a.id == "" || !validModel(a.model) {
		return ErrInvalid
	}
	return nil
}

// Reference returns metadata only, after scope validation; it is not proof.
func (a AccountRef) Reference(scope kernel.OrgScope) (string, AccessModel, error) {
	if err := a.check(scope); err != nil {
		return "", "", err
	}
	return a.id, a.model, nil
}

type CredentialKind string

const (
	AccountKey        CredentialKind = "ACCOUNT_KEY"
	EmailCode         CredentialKind = "EMAIL_CODE"
	LinkedProvider    CredentialKind = "LINKED_PROVIDER"
	EnrolledDeviceKey CredentialKind = "ENROLLED_DEVICE_KEY"
)

type CredentialRef struct {
	privateValue
	account AccountRef
	kind    CredentialKind
}

// Credential describes the source without receiving any credential material.
// It performs no verification and must never be used as an authorization input.
func Credential(scope kernel.OrgScope, account AccountRef, kind CredentialKind) (CredentialRef, error) {
	if err := account.check(scope); err != nil {
		return CredentialRef{}, err
	}
	switch kind {
	case AccountKey, EmailCode, LinkedProvider, EnrolledDeviceKey:
	default:
		return CredentialRef{}, ErrInvalid
	}
	if account.model == ManagedAccess && kind != EnrolledDeviceKey {
		return CredentialRef{}, ErrInvalid
	}
	return CredentialRef{account: account, kind: kind}, nil
}
func (c CredentialRef) Reference(scope kernel.OrgScope) (AccountRef, CredentialKind, error) {
	if err := c.account.check(scope); err != nil {
		return AccountRef{}, "", err
	}
	return c.account, c.kind, nil
}

// DriverProfileRef represents the optional employee association only. Licence
// fields and eligibility are owned by IDN-014; no person/driver ID is invented.
type DriverProfileRef struct {
	privateValue
	org        kernel.OrgID
	employeeID string
}

func DriverProfile(scope kernel.OrgScope, employee Employee) (DriverProfileRef, error) {
	snap, err := employee.Snapshot(scope)
	if err != nil {
		return DriverProfileRef{}, err
	}
	return DriverProfileRef{org: scope.OrgID(), employeeID: snap.ID}, nil
}
func (p DriverProfileRef) EmployeeID(scope kernel.OrgScope) (string, error) {
	if err := scoped(scope, p.org, p.employeeID, 1); err != nil {
		return "", err
	}
	return p.employeeID, nil
}
