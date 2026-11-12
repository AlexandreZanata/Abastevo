package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	idn "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	auth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

type EmployeeMergeAuthority struct{ MembershipAccessAuthority }

func (g *EmployeeMergeAuthority) CheckEmployeeMergeTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a members.EmployeeActor, member string, write bool) error {
	if g == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil || a.Validate() != nil || legalIdentity(scope, member) != nil {
		return members.ErrRegisterUnavailable
	}
	e := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, app.CompanyActor{AccountID: a.AccountID, SessionID: a.SessionID}, member, scope.OrgID().String(), members.EmployeeMergePolicy(write))
	switch {
	case errors.Is(e, app.ErrUnauthenticated):
		return members.ErrEmployeeUnauthenticated
	case errors.Is(e, app.ErrUnavailable):
		return members.ErrRegisterUnavailable
	case errors.Is(e, app.ErrNotFound):
		return members.ErrEmployeeNotFound
	case errors.Is(e, cap.ErrEntitlementRequired):
		return auth.ErrEntitlementRequired
	}
	return e
}

type EmployeeMergeTeamProjector struct{ NewID func() (string, error) }

func (p EmployeeMergeTeamProjector) PreviewEmployeeMergeTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, source, survivor string) ([]members.MergeTeamPlan, error) {
	if legalIdentity(scope, source) != nil || legalIdentity(scope, survivor) != nil {
		return nil, idn.ErrInvalid
	}
	if e := database.ValidateOrgTx(ctx, tx, scope); e != nil {
		return nil, e
	}
	rows, e := q.New(tx).EmployeeMergeTeamPlans(ctx, q.EmployeeMergeTeamPlansParams{OrgID: scope.OrgID().String(), SourceID: source, SurvivorID: survivor})
	if e != nil {
		return nil, e
	}
	if len(rows) > idn.MaxEmployeeMergeTeams {
		return nil, idn.ErrMergeConflict
	}
	out := make([]members.MergeTeamPlan, 0, len(rows))
	for _, r := range rows {
		out = append(out, members.MergeTeamPlan{TeamID: r.TeamID, SourceLinkID: r.SourceLinkID, ExistingSurvivorLinkID: r.SurvivorLinkID, Name: r.Name, TeamRevision: r.TeamRevision, Admissible: r.Admissible})
	}
	return out, nil
}
func (p EmployeeMergeTeamProjector) TransferEmployeeTeamsTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, source, survivor, operation string, plans []members.MergeTeamPlan) ([]members.MergeTeamEffect, error) {
	if p.NewID == nil || legalIdentity(scope, source) != nil || legalIdentity(scope, survivor) != nil || legalIdentity(scope, operation) != nil || len(plans) > idn.MaxEmployeeMergeTeams {
		return nil, idn.ErrInvalid
	}
	if e := database.ValidateOrgTx(ctx, tx, scope); e != nil {
		return nil, e
	}
	queries := q.New(tx)
	out := make([]members.MergeTeamEffect, 0, len(plans))
	for _, v := range plans {
		if !v.Admissible {
			return nil, idn.ErrMergeConflict
		}
		target, disposition := v.ExistingSurvivorLinkID, "EXISTING"
		if target == "" {
			var e error
			target, e = p.NewID()
			if e != nil {
				return nil, e
			}
			if legalIdentity(scope, target) != nil {
				return nil, idn.ErrInvalid
			}
			if e := queries.InsertTeamLink(ctx, q.InsertTeamLinkParams{OrgID: scope.OrgID().String(), ID: target, TeamID: v.TeamID, EmployeeID: survivor, OperationID: operation}); e != nil {
				return nil, e
			}
			disposition = "CREATED"
		}
		n, e := queries.EndTeamLink(ctx, q.EndTeamLinkParams{OrgID: scope.OrgID().String(), ID: v.SourceLinkID, OperationID: operation})
		if e != nil {
			return nil, e
		}
		if n != 1 {
			return nil, idn.ErrConflict
		}
		out = append(out, members.MergeTeamEffect{TeamID: v.TeamID, SourceLinkID: v.SourceLinkID, SurvivorLinkID: target, Disposition: disposition})
	}
	return out, nil
}
