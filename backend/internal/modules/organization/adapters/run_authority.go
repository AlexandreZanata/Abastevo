package adapters

import (
	"context"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	checklists "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/jackc/pgx/v5"
)

// RunAuthority is the signed check.run.complete authority (EMP-CHECK-002,
// EDR-284). It shares the meter authority's real sources and the report
// authority's own-record checks; errors use the fleet vocabulary.
type RunAuthority MeterAuthority

var _ checklists.RunAuthority = (*RunAuthority)(nil)

// runHistoryRule is the EDR-177 rule applied to check.checklist.execute: the
// cell is S for every holder, so a positive ORG rule covers the member's own
// new run on any asset of the organization, and SELF covers only the assets
// assigned to the member at the declared instant.
var runHistoryRule = historyRule{permission: checklists.ExecutePermission, orgCovers: func(c permission.Coverage) bool { return c != permission.Denied }, selfCovers: true}

// CheckRunTx proves current authority on the borrowed intake port over the
// server-resolved run facts (the stored author when given); an empty RunID
// is the root selector and considers ORG grants only.
func (f *RunAuthority) CheckRunTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, target checklists.RunTarget, action checklists.RunAction) error {
	policy := checklists.RunPolicy(action)
	if policy.Permission == "" {
		return fleet.ErrUnavailable
	}
	return (*ReportAuthority)(f).checkRecordTx(ctx, db, scope, source, recordTarget{ID: target.RunID, AssetID: target.AssetID, UnitID: target.UnitID,
		CostCentreID: target.CostCentreID, AuthorMembershipID: target.AuthorMembershipID}, policy, action == checklists.RunWrite)
}

// ClassifyRunTx decides a new run from retained role and assignment history
// with the unchanged EDR-118 continuity.
func (f *RunAuthority) ClassifyRunTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, asset string, declared time.Time) (checklists.ReportDisposition, error) {
	return (*ReportAuthority)(f).classifyRecordTx(ctx, db, scope, source, asset, declared, runHistoryRule)
}

// RunCompanyAuthority proves the company run policies (EDR-285): the read on
// the asset (AUTH-006 structure and FLEET-006 SELF assignee, as the
// applicable read) and the release on the stored run (fresh proof, ACTIVE
// root, structure through the run's stored asset, unit and cost centre); the
// queue on the organization root exactly as the incident queue (EDR-328).
type RunCompanyAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	// AssetStructure and Assignee serve the read; RunStructure the release.
	AssetStructure *ResourceStructure
	Assignee       *ResourceAssignee
	RunStructure   *ResourceStructure
}

var _ checklists.RunCompanyAuthority = (*RunCompanyAuthority)(nil)

func (g *RunCompanyAuthority) CheckRunCompanyTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor checklists.IncidentActor, member, target string, action checklists.RunCompanyAction) (checklists.Handler, error) {
	if g == nil || meterMissing(tx) || g.AssetStructure == nil || g.AssetStructure.Kind != AssetStructure || g.RunStructure == nil || g.RunStructure.Kind != ChecklistRunStructure {
		return checklists.Handler{}, checklists.ErrUnavailable
	}
	inner := IncidentAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities, Structure: g.RunStructure}
	switch action {
	case checklists.RunRead:
		return inner.checkTx(ctx, tx, scope, actor, member, target, checklists.RunCompanyPolicy(action), false, &companyAuthority{Structure: g.AssetStructure, Assignee: g.Assignee})
	case checklists.RunRelease:
		return inner.checkTx(ctx, tx, scope, actor, member, target, checklists.RunCompanyPolicy(action), true, &companyAuthority{Structure: g.RunStructure})
	case checklists.RunQueue:
		// EDR-328: exactly the incident queue, on the organization root only.
		if target != scope.OrgID().String() {
			return checklists.Handler{}, checklists.ErrUnavailable
		}
		return inner.checkTx(ctx, tx, scope, actor, member, target, checklists.RunCompanyPolicy(action), false, nil)
	}
	return checklists.Handler{}, checklists.ErrUnavailable
}
