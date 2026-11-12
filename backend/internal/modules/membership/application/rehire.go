package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/jackc/pgx/v5"
	"math"
	"time"
)

const RehireProvisioning = "SEPARATE_INVITATION_REQUIRED"

func RehirePolicy() access.Policy { return MembershipAccessPolicy(MembershipReactivate) }

type RehireAuthority interface {
	CheckRehireTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string) error
}
type RehireCommand struct {
	privateRegister
	OperationID, EmployeeID, Reason string
	ExpectedRevision                int64
}

func (c RehireCommand) Validate() error {
	if !canonicalEmployeeID(c.OperationID) || !canonicalEmployeeID(c.EmployeeID) || c.ExpectedRevision < 1 || c.ExpectedRevision == math.MaxInt64 {
		return d.ErrInvalid
	}
	return d.ValidateAccessChangeReason(c.Reason)
}
func (c RehireCommand) Digest(scope kernel.OrgScope) string {
	if scope.Validate() != nil || c.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "idn.employee.rehire", scope.OrgID().String(), c.OperationID, c.EmployeeID, c.ExpectedRevision, c.Reason})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type RehireResult struct {
	privateRegister
	EmployeeID, OperationID, Reason string
	State                           d.EmployeeState
	Revision                        int64
	OccurredAt                      time.Time
	Replayed                        bool
}

func (r RehireResult) Validate(c RehireCommand) error {
	if c.Validate() != nil || r.EmployeeID != c.EmployeeID || r.OperationID != c.OperationID || r.Reason != c.Reason || r.State != d.EmployeeActive || r.Revision != c.ExpectedRevision+1 || r.OccurredAt.IsZero() {
		return ErrRegisterUnavailable
	}
	return nil
}

type RehireRepository interface {
	Rehire(context.Context, kernel.OrgScope, EmployeeActor, string, RehireCommand) (RehireResult, error)
}
type RehireService struct {
	Enabled    bool
	Repository RehireRepository
}

func (s RehireService) Rehire(ctx context.Context, scope kernel.OrgScope, actor EmployeeActor, decision access.Decision, c RehireCommand) (RehireResult, error) {
	if !s.Enabled {
		return RehireResult{}, ErrRegisterDisabled
	}
	if scope.Validate() != nil {
		return RehireResult{}, d.ErrScope
	}
	p := RehirePolicy()
	if actor.Validate() != nil || decision.Policy() != p || decision.Check(ctx, scope, actor.AccountID, p.Permission, scope.OrgID().String()) != nil {
		return RehireResult{}, ErrEmployeeUnauthenticated
	}
	if e := c.Validate(); e != nil {
		return RehireResult{}, e
	}
	if s.Repository == nil {
		return RehireResult{}, ErrRegisterUnavailable
	}
	out, e := s.Repository.Rehire(ctx, scope, actor, decision.Subject().MembershipID(), c)
	if e != nil {
		return RehireResult{}, e
	}
	if e := out.Validate(c); e != nil {
		return RehireResult{}, e
	}
	return out, nil
}
