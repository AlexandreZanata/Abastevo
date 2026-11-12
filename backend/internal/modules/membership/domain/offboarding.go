package domain

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"slices"
	"strings"
)

var ErrOpenResponsibilities = errors.New("membership: responsibilities require review")

const OffboardingScopeVersion = "ORG_F1"
const MaxHandoverItems = 500

// Source facts only. A pending manager flag never closes or reassigns work.
type HandoverItem struct {
	Key      string `json:"key"`
	Revision int64  `json:"source_revision"`
}

func (i HandoverItem) Validate() error {
	if i.Revision < 1 {
		return ErrInvalid
	}
	parts := strings.Split(i.Key, ":")
	if len(parts) == 2 && parts[0] == "TEAM_LINK" {
		id, e := canonicalID(parts[1])
		if e == nil && id == parts[1] {
			return nil
		}
	}
	if len(parts) == 3 && parts[0] == "SETTING" && slices.Contains([]string{"review_fallback", "platform_notices", "data_protection", "billing"}, parts[1]) && slices.Contains([]string{"primary", "substitute"}, parts[2]) {
		return nil
	}
	return ErrInvalid
}
func HandoverDigest(scope kernel.OrgScope, m Membership, employee Employee, items []HandoverItem) (string, error) {
	v, e := m.Snapshot(scope)
	if e != nil {
		return "", e
	}
	emp, e := employee.Snapshot(scope)
	if e != nil || emp.ID != v.EmployeeID {
		return "", ErrScope
	}
	if len(items) > MaxHandoverItems {
		return "", ErrOpenResponsibilities
	}
	last := ""
	for _, i := range items {
		if i.Validate() != nil || i.Key <= last {
			return "", ErrInvalid
		}
		last = i.Key
	}
	raw, _ := json.Marshal([]any{OffboardingScopeVersion, scope.OrgID().String(), v.ID, v.Revision, v.State, v.Suspension, emp.ID, emp.Revision, emp.State, items})
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}
func PlanOffboarding(scope kernel.OrgScope, m Membership, employee Employee, expected int64, terminate, self bool, digest string, flags []string, items []HandoverItem, ownerProtected bool) (Membership, error) {
	v, e := m.Snapshot(scope)
	if e != nil {
		return Membership{}, e
	}
	if v.Model != ExistingAccount || terminate && self {
		return Membership{}, ErrConditions
	}
	actual, e := HandoverDigest(scope, m, employee, items)
	if e != nil {
		return Membership{}, e
	}
	if digest != actual || len(flags) != len(items) {
		return Membership{}, ErrOpenResponsibilities
	}
	for n, i := range items {
		if flags[n] != i.Key {
			return Membership{}, ErrOpenResponsibilities
		}
	}
	reason := Offboarded
	if self {
		reason = Left
	}
	return m.Transition(scope, expected, MembershipEnded, MembershipConditions{OwnershipProtected: ownerProtected, ResponsibilitiesReviewed: true, EndReason: reason})
}
