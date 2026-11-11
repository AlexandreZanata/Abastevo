package kernel

import (
	"encoding/hex"
	"errors"
	"strings"
)

var (
	// ErrInvalidOrgID covers malformed and nil UUIDs without exposing input.
	ErrInvalidOrgID = errors.New("kernel: invalid organization ID")
	// ErrInvalidOrgScope covers an absent organization selection.
	ErrInvalidOrgScope = errors.New("kernel: missing organization scope")
)

// OrgID identifies a private Abastevo Empresas organization with a non-nil UUID.
// Its zero value is invalid.
type OrgID struct {
	normalized string
}

// ParseOrgID accepts dashed UUIDs in either hex case and canonicalizes to lowercase.
func ParseOrgID(raw string) (OrgID, error) {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return OrgID{}, ErrInvalidOrgID
	}
	compact := strings.ReplaceAll(raw, "-", "")
	if len(compact) != 32 {
		return OrgID{}, ErrInvalidOrgID
	}
	if _, err := hex.DecodeString(compact); err != nil {
		return OrgID{}, ErrInvalidOrgID
	}
	normalized := strings.ToLower(raw)
	if normalized == "00000000-0000-0000-0000-000000000000" {
		return OrgID{}, ErrInvalidOrgID
	}
	return OrgID{normalized: normalized}, nil
}

// String returns the canonical UUID, or an empty string for the zero value.
func (id OrgID) String() string { return id.normalized }

// OrgScope carries an explicitly selected organization through private ports.
// The application must verify current actor authority before constructing it.
// The value contains no membership, role or capability decision.
type OrgScope struct {
	orgID OrgID
}

// NewOrgScope constructs a scope from a validated, nonzero organization ID.
func NewOrgScope(id OrgID) (OrgScope, error) {
	if id == (OrgID{}) {
		return OrgScope{}, ErrInvalidOrgScope
	}
	return OrgScope{orgID: id}, nil
}

// OrgID returns the selected organization ID.
func (scope OrgScope) OrgID() OrgID { return scope.orgID }

// Validate rejects the zero scope before an adapter accesses private data.
func (scope OrgScope) Validate() error {
	if scope.orgID == (OrgID{}) {
		return ErrInvalidOrgScope
	}
	return nil
}
