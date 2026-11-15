package adapters

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	checklists "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// ChecklistAuthority supplies current company proof through the checklist
// port (EMP-CHECK-001, EDR-215). Templates are organization-level, so the
// template actions target the organization itself and a unit-, cost-centre-,
// asset-, team- or SELF-scoped grant never covers them; template writes also
// require an ACTIVE root. The applicable read targets the asset with the
// AUTH-006 structure and the FLEET-006 SELF assignee lookups.
type ChecklistAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Structure    *ResourceStructure
	Assignee     *ResourceAssignee
}

var _ checklists.ChecklistAuthority = (*ChecklistAuthority)(nil)

func (g *ChecklistAuthority) CheckChecklistTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor checklists.ChecklistActor, member, target string, action checklists.ChecklistAction) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || legalIdentity(scope, target) != nil {
		return checklists.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return checklists.ErrUnauthenticated
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}
	switch action {
	case checklists.TemplateRead, checklists.TemplateManage:
		if target != scope.OrgID().String() {
			return checklists.ErrUnavailable
		}
	case checklists.ApplicableRead:
		if target == scope.OrgID().String() {
			return checklists.ErrUnavailable
		}
		guard.Structure, guard.Assignee = g.Structure, g.Assignee
	default:
		return checklists.ErrUnavailable
	}
	policy := checklists.Policy(action)
	if policy.Validate() != nil {
		return checklists.ErrUnavailable
	}
	if database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return checklists.ErrUnavailable
	}
	err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, policy)
	switch {
	case err == nil:
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnauthenticated):
		return checklists.ErrUnauthenticated
	case errors.Is(err, org.ErrUnavailable):
		return checklists.ErrUnavailable
	case errors.Is(err, org.ErrNotFound):
		return checklists.ErrNotFound
	default:
		return err
	}
	if action != checklists.TemplateManage {
		return nil
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
		return checklists.ErrUnavailable
	}
	if state != "ACTIVE" {
		return access.ErrSetupIncomplete
	}
	return nil
}
