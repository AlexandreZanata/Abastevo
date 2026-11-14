package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	exports "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	exportdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ExportAuthority supplies the export permission port (EMP-RPT-001). Web
// calls retain the company pipeline for the organization target (an
// organization-wide rpt.export.run grant; a unit- or SELF-scoped grant never
// covers it); a request also needs an ACTIVE root. The worker re-evaluates a
// recorded requester without a session (BR-RPT-002).
type ExportAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

var _ exports.Authority = (*ExportAuthority)(nil)

func (g *ExportAuthority) CheckExportTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor exports.Actor, member string, action exports.Action) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) {
		return exports.ErrUnavailable
	}
	policy := exports.Policy(action)
	if policy.Validate() != nil {
		return exports.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return exports.ErrUnauthenticated
	}
	if database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return exports.ErrUnavailable
	}
	err := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, scope.OrgID().String(), policy)
	switch {
	case err == nil:
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnauthenticated):
		return exports.ErrUnauthenticated
	case errors.Is(err, org.ErrUnavailable):
		return exports.ErrUnavailable
	case errors.Is(err, org.ErrNotFound):
		return exports.ErrNotFound
	default:
		return err
	}
	if action != exports.ActionRequest {
		return nil
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
		return exports.ErrUnavailable
	}
	if state != string(d.Active) {
		return access.ErrSetupIncomplete
	}
	return nil
}

// CheckDataTx proves, after CheckExportTx in the same transaction, that the
// actor's same ACTIVE membership holds a current grant covering the kind's
// declared read permission for the whole organization (EMP-AUTH-011). A
// missing grant is the bounded permission denial of that key.
func (g *ExportAuthority) CheckDataTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor exports.Actor, member string, read permissions.Permission) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || !exports.ExportableRead(read) {
		return exports.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return exports.ErrUnauthenticated
	}
	if database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return exports.ErrUnavailable
	}
	current, err := g.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, members.ErrNotFound) {
			return access.ErrNoMembership
		}
		return exports.ErrUnavailable
	}
	if membership, err := current.Snapshot(scope); err != nil || membership.ID != member {
		return access.ErrNoMembership
	}
	return g.covers(ctx, tx, scope, member, read)
}

// covers reports, under database time, whether the member's current
// assignments cover permission for the whole organization. A missing grant
// is the bounded permission denial; structural and own-records grants never
// cover the organization target.
func (g *ExportAuthority) covers(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member string, permission permissions.Permission) error {
	assignments, err := g.Roles.CandidatesTx(ctx, tx, scope, member)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return exports.ErrUnavailable
	}
	now, err := q.New(tx).CurrentOnboardingTime(ctx)
	if err != nil || !now.Valid || now.InfinityModifier != pgtype.Finite {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return exports.ErrUnavailable
	}
	facts := permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}
	if _, err := permissions.CoveringRules(scope, member, assignments, permission, facts, now.Time); err != nil {
		if errors.Is(err, permissions.ErrPermissionDenied) {
			return err
		}
		return exports.ErrUnavailable
	}
	return nil
}

// CheckRequesterTx returns ErrRightsLost when the recorded requester no
// longer holds export authority now: inactive or deleting account, a
// membership that is not the same ACTIVE one, an organization that is not
// ACTIVE, or no current role covering rpt.export.run and the kind's declared
// read permission for the whole organization. Only an unavailable source is
// ErrUnavailable.
func (g *ExportAuthority) CheckRequesterTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, requester exports.Requester, kind string, read permissions.Permission) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) {
		return exports.ErrUnavailable
	}
	if requester.Validate() != nil || legalIdentity(scope, requester.MembershipID) != nil || !exportdomain.ValidKind(kind) || !exports.ExportableRead(read) {
		return exports.ErrUnavailable
	}
	if database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return exports.ErrUnavailable
	}
	unavailable := func(err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return exports.ErrRightsLost
		}
		return exports.ErrUnavailable
	}
	queries := q.New(tx)
	account, err := queries.ShareLegalAccount(ctx, requester.AccountID)
	if err != nil {
		return unavailable(err)
	}
	orgID := scope.OrgID().String()
	if account.Status != "active" || account.DeletionPending || account.ManagedOrg != "" && account.ManagedOrg != orgID {
		return exports.ErrRightsLost
	}
	root, err := queries.ShareLegalOrganization(ctx, q.ShareLegalOrganizationParams{OrgID: orgID, TermsVersion: d.SyntheticTermsVersion, TermsDigest: d.SyntheticTermsDigest})
	if err != nil {
		return unavailable(err)
	}
	if !root.HasSnapshot || !root.State.Valid || !root.SyntheticSetup || d.State(root.State.String) != d.Active {
		return exports.ErrRightsLost
	}
	current, err := g.Memberships.ActiveByAccountTx(ctx, tx, scope, requester.AccountID)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, members.ErrNotFound) {
			return exports.ErrRightsLost
		}
		return exports.ErrUnavailable
	}
	membership, err := current.Snapshot(scope)
	if err != nil || membership.ID != requester.MembershipID {
		return exports.ErrRightsLost
	}
	// rpt.export.run and the kind's data permission must both still cover the
	// whole organization (B-BR-EMP-249).
	for _, permission := range []permissions.Permission{exportdomain.Permission, read} {
		if err := g.covers(ctx, tx, scope, requester.MembershipID, permission); err != nil {
			if errors.Is(err, permissions.ErrPermissionDenied) {
				return exports.ErrRightsLost
			}
			return err
		}
	}
	return nil
}
