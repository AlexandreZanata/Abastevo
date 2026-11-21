package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

type LifecycleAction string

const (
	ArchiveOrganization LifecycleAction = "ARCHIVE"
	RestoreOrganization LifecycleAction = "RESTORE"
	ScheduleClosure     LifecycleAction = "REQUEST_CLOSURE"
	CancelClosure       LifecycleAction = "CANCEL_CLOSURE"
)

func LifecyclePolicy(write bool) access.Policy {
	policy := access.Policy{Permission: "org.lifecycle.manage", Capability: "organizations", Effect: access.Read, Fresh: true}
	if write {
		policy.Effect = access.ReduceAccess
	}
	return policy
}

type LifecycleCommand struct {
	Action           LifecycleAction
	OperationID      string
	ExpectedRevision int64
	Confirmation     string
	ClosureDays      int
}

func (c LifecycleCommand) Validate() error {
	if !canonicalID(c.OperationID) || c.ExpectedRevision < 1 {
		return d.ErrInvalid
	}
	switch c.Action {
	case ScheduleClosure:
		if c.Confirmation == "" || len(c.Confirmation) > 480 || c.ClosureDays != 0 && (c.ClosureDays < 7 || c.ClosureDays > 90) {
			return d.ErrInvalid
		}
	case ArchiveOrganization, RestoreOrganization, CancelClosure:
		if c.Confirmation != "" || c.ClosureDays != 0 {
			return d.ErrInvalid
		}
	default:
		return d.ErrInvalid
	}
	return nil
}

type OpenOrganizationItems struct {
	Trips              int64 `json:"trips"`
	Agreements         int64 `json:"agreements"`
	Reviews            int64 `json:"reviews"`
	HeldOperations     int64 `json:"held_operations"`
	OpenPayables       int64 `json:"open_payables"`
	StationAuthorities int64 `json:"station_authorities"`
	ActiveGrants       int64 `json:"active_grants"`
}

var ErrOrganizationOpenWork = errors.New("organization: open operations")

type OrganizationOpenWork struct{ Trips, Agreements int64 }

func (OrganizationOpenWork) Error() string { return ErrOrganizationOpenWork.Error() }
func (OrganizationOpenWork) Unwrap() error { return ErrOrganizationOpenWork }

type LifecycleStatus struct {
	State    d.State               `json:"state"`
	Revision int64                 `json:"revision"`
	ClosesAt *time.Time            `json:"closes_at"`
	Items    OpenOrganizationItems `json:"open_items"`
}

type LifecycleResult struct {
	LifecycleStatus
	Replayed bool `json:"replayed"`
}

type LifecycleRepository interface {
	LifecycleReport(context.Context, kernel.OrgScope, CompanyActor, string) (LifecycleStatus, error)
	ChangeLifecycle(context.Context, kernel.OrgScope, CompanyActor, string, LifecycleCommand) (LifecycleResult, error)
}
