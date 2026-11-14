package adapters

import (
	"context"
	"errors"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ResourceAssignee is the EMP-FLEET-006 assignee lookup of the SELF scope on
// assets (B-BR-EMP-246, EDR-165): an asset is the member's own record exactly
// while the Fleet assigned-asset port says it is assigned to the membership
// at the evaluated instant. It answers only for fleet.asset.read on an asset;
// the organization root, other permissions and other records are never
// assigned. Prepare uses the pool form; the company authority re-check uses
// the same lookup inside the owning transaction with database time.
type ResourceAssignee struct {
	Pool        *pgxpool.Pool
	Assignments fleet.AssignedAssetsPort
}

var _ access.AssigneeSource = (*ResourceAssignee)(nil)

func (r *ResourceAssignee) ResolveAssignee(ctx context.Context, scope kernel.OrgScope, policy access.Policy, facts permissions.ResourceFacts, member string, at time.Time) (bool, error) {
	if r == nil || r.Pool == nil {
		return false, access.ErrUnavailable
	}
	var assigned bool
	err := database.WithOrgTx(ctx, r.Pool, scope, func(tx pgx.Tx) error {
		var err error
		assigned, err = r.assignedTx(ctx, tx, scope, policy, facts, member, at)
		return err
	})
	switch {
	case err == nil:
		return assigned, nil
	case ctx.Err() != nil:
		return false, ctx.Err()
	}
	return false, access.ErrUnavailable
}

func (r *ResourceAssignee) assignedTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, policy access.Policy, facts permissions.ResourceFacts, member string, at time.Time) (bool, error) {
	if r == nil || tx == nil || at.IsZero() || facts.Validate(scope) != nil || legalIdentity(scope, member) != nil || meterMissing(r.Assignments) {
		return false, org.ErrUnavailable
	}
	if policy.Permission != "fleet.asset.read" || facts.ID == scope.OrgID().String() {
		return false, nil
	}
	assigned, err := r.Assignments.AssignedAtDBTX(ctx, tx, scope, member, facts.ID, at)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, org.ErrUnavailable
	}
	return assigned, nil
}

// assigneeTx adds the assignee fact to an in-transaction re-check only when
// the member holds a current SELF grant for the permission and the record
// states no own authorship or assignment.
func (r *ResourceAssignee) assigneeTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member string, assignments []permissions.Assignment, policy access.Policy, facts permissions.ResourceFacts, at time.Time) (permissions.ResourceFacts, error) {
	if r == nil || facts.AssignedTo != "" || facts.AuthoredBy == member {
		return facts, nil
	}
	self, err := permissions.SelfCandidate(scope, member, assignments, policy.Permission, at)
	if err != nil {
		return permissions.ResourceFacts{}, org.ErrUnavailable
	}
	if !self {
		return facts, nil
	}
	assigned, err := r.assignedTx(ctx, tx, scope, policy, facts, member, at)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return permissions.ResourceFacts{}, err
		}
		return permissions.ResourceFacts{}, org.ErrUnavailable
	}
	if assigned {
		facts.AssignedTo = member
	}
	return facts, nil
}
