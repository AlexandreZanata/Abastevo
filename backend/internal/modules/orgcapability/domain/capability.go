// Package domain defines source-registered organization capability switches.
// A switch never carries membership or permission authority.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"regexp"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalid             = errors.New("orgcapability: invalid input")
	ErrDenied              = errors.New("orgcapability: authority denied")
	ErrUnavailable         = errors.New("orgcapability: unavailable")
	ErrConflict            = errors.New("orgcapability: stale revision")
	ErrReused              = errors.New("orgcapability: operation ID reused")
	ErrSurfaceDisabled     = errors.New("orgcapability: surface disabled")
	ErrEntitlementRequired = errors.New("orgcapability: entitlement required")
	codePattern            = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$`)
)

type Catalog struct{ codes map[string]struct{} }

func NewCatalog(codes []string) (Catalog, error) {
	if len(codes) == 0 {
		return Catalog{}, ErrInvalid
	}
	catalog := Catalog{codes: make(map[string]struct{}, len(codes))}
	for _, code := range codes {
		if len(code) > 64 || !codePattern.MatchString(code) {
			return Catalog{}, ErrInvalid
		}
		if _, exists := catalog.codes[code]; exists {
			return Catalog{}, ErrInvalid
		}
		catalog.codes[code] = struct{}{}
	}
	return catalog, nil
}

func (catalog Catalog) Validate(code string) error {
	if _, exists := catalog.codes[code]; !exists {
		return ErrInvalid
	}
	return nil
}

type Intent uint8

const (
	Read Intent = iota
	Export
	Write
	Manage
)

// Actor is server-verified attribution, never a client authority claim.
type Actor struct{ AccountID, DelegationID string }

func (actor Actor) Validate() error {
	if !validID(actor.AccountID) || (actor.DelegationID != "" && !validID(actor.DelegationID)) {
		return ErrInvalid
	}
	return nil
}
func validID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

type State struct {
	Enabled  bool
	Revision int64
}

func (state State) CheckWrite() error {
	if !state.Enabled {
		return ErrEntitlementRequired
	}
	return nil
}

// ExpectedRevision is zero only for the first persisted state.
type Change struct {
	OperationID      string
	Capability       string
	Enabled          bool
	ExpectedRevision int64
}

func (change Change) Validate(catalog Catalog) error {
	if catalog.Validate(change.Capability) != nil || !validID(change.OperationID) ||
		change.ExpectedRevision < 0 || change.ExpectedRevision == math.MaxInt64 {
		return ErrInvalid
	}
	return nil
}

// Digest excludes transport credentials. Actor attribution is compared separately
// by the audit runner. Fixed array positions pin the canonical command version.
func (change Change) Digest(scope kernel.OrgScope) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	body, _ := json.Marshal([6]any{1, scope.OrgID().String(), change.OperationID, change.Capability, change.Enabled, change.ExpectedRevision})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
