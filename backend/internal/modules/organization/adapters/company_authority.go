package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Reuse the same account/proof -> capability -> root -> current authority
// lock order for the concrete legal and structure producers. This accepts only
// source-owned policies, never an arbitrary caller-selected permission.
type companyAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

func (guard companyAuthority) check(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member, target string, policy access.Policy) error {
	if policy != app.SettingsPolicy(false) && policy != app.SettingsPolicy(true) && policy != app.LegalPolicy(false, false) && policy != app.LegalPolicy(false, true) && policy != app.LegalPolicy(true, true) && policy != app.StructurePolicy(false) && policy != app.StructurePolicy(true) && policy != members.EmployeePolicy(members.EmployeeRead) && policy != members.EmployeePolicy(members.EmployeeReview) && policy != members.EmployeePolicy(members.EmployeeManage) {
		return app.ErrUnavailable
	}
	write := policy.Effect == access.ConfigurationWrite
	// Plain reads share the account and root rows, so they run concurrently
	// while writers still wait for them. A settings read can initialize the
	// first version under the root (org_settings_bootstrap locks it FOR
	// UPDATE); two readers holding FOR SHARE would then deadlock on upgrade.
	exclusive := write || policy == app.SettingsPolicy(false)
	queries := q.New(tx)
	var account q.LockLegalAccountRow
	var err error
	if exclusive {
		account, err = queries.LockLegalAccount(ctx, actor.AccountID)
	} else {
		var shared q.ShareLegalAccountRow
		shared, err = queries.ShareLegalAccount(ctx, actor.AccountID)
		account = q.LockLegalAccountRow(shared)
	}
	if err != nil {
		return err
	}
	if account.Status != "active" || account.DeletionPending || (account.ManagedOrg != "" && account.ManagedOrg != scope.OrgID().String()) {
		return app.ErrUnauthenticated
	}
	proof := q.LockLegalProofParams{AccountID: actor.AccountID, SessionID: actor.SessionID, RequireFresh: policy.Fresh}
	if _, err := queries.LockLegalProof(ctx, proof); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrUnauthenticated
		}
		return err
	}
	if write {
		if err := guard.Capabilities.GuardWrite(ctx, tx, scope, "organizations"); err != nil {
			return err
		}
	}
	rootParams := q.LockLegalOrganizationParams{OrgID: scope.OrgID().String(), TermsVersion: d.SyntheticTermsVersion, TermsDigest: d.SyntheticTermsDigest}
	var root q.LockLegalOrganizationRow
	if exclusive {
		root, err = queries.LockLegalOrganization(ctx, rootParams)
	} else {
		var shared q.ShareLegalOrganizationRow
		shared, err = queries.ShareLegalOrganization(ctx, q.ShareLegalOrganizationParams(rootParams))
		root = q.LockLegalOrganizationRow(shared)
	}
	if err != nil {
		return err
	}
	if !root.HasSnapshot || !root.State.Valid || !root.SyntheticSetup {
		return app.ErrUnavailable
	}
	switch d.State(root.State.String) {
	case d.PendingSetup, d.Active:
	case d.Suspended:
		if write {
			return access.ErrOrgSuspended
		}
	case d.Archived:
		if write {
			return access.ErrOrgArchived
		}
	default:
		return app.ErrNotFound
	}
	current, err := guard.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
	if err != nil {
		return access.ErrNoMembership
	}
	membership, err := current.Snapshot(scope)
	if err != nil || membership.ID != member {
		return access.ErrNoMembership
	}
	assignments, err := guard.Roles.CandidatesTx(ctx, tx, scope, member)
	if err != nil {
		return err
	}
	now, err := queries.CurrentOnboardingTime(ctx)
	if err != nil {
		return err
	}
	if !now.Valid || now.InfinityModifier != pgtype.Finite {
		return app.ErrUnavailable
	}
	if _, err := permissions.CoveringRules(scope, member, assignments, policy.Permission, permissions.ResourceFacts{OrgID: scope.OrgID(), ID: target}, now.Time); err != nil {
		return err
	}
	// The root/capability locks could have waited: recheck freshness immediately
	// before current reads/mutations, while the original proof rows stay locked.
	if _, err := queries.LockLegalProof(ctx, proof); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrUnauthenticated
		}
		return err
	}
	return nil
}

// EmployeeAuthority implements the identity module's explicit transaction port
// without sharing generated SQL or accepting arbitrary permissions.
type EmployeeAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

var _ members.EmployeeAuthority = (*EmployeeAuthority)(nil)

func (g *EmployeeAuthority) CheckEmployeeTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, member, target string, action members.EmployeeAction) error {
	if g == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil || actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return members.ErrRegisterUnavailable
	}
	if action != members.EmployeeRead && action != members.EmployeeReview && action != members.EmployeeManage {
		return members.ErrRegisterUnavailable
	}
	err := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, app.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, members.EmployeePolicy(action))
	switch {
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, app.ErrUnauthenticated):
		return members.ErrEmployeeUnauthenticated
	case errors.Is(err, app.ErrUnavailable):
		return members.ErrRegisterUnavailable
	case errors.Is(err, app.ErrNotFound):
		return members.ErrEmployeeNotFound
	}
	return err
}
