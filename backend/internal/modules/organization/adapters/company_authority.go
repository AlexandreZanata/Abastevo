package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
)

// Reuse the same account/proof -> capability -> root -> current authority
// lock order for the concrete legal and structure producers. This accepts only
// source-owned policies, never an arbitrary caller-selected permission.
type companyAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

func (guard companyAuthority) check(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member, target string, policy access.Policy, requestedRole ...permissions.Role) error {
	if policy != fleet.Policy(false) && policy != fleet.Policy(true) && policy != app.SettingsMembersPolicy() && policy != members.DevicePolicy(false) && policy != members.DevicePolicy(true) && policy != members.EmployeeMergePolicy(false) && policy != members.EmployeeMergePolicy(true) && policy != members.MembershipAccessPolicy(members.MembershipAccessRead) && policy != members.MembershipAccessPolicy(members.MembershipSuspend) && policy != members.MembershipAccessPolicy(members.MembershipReactivate) && policy != members.InvitationPolicy(members.InvitationIssue) && policy != members.InvitationPolicy(members.InvitationRead) && policy != members.InvitationPolicy(members.InvitationRevoke) && policy != access.RolePolicy(access.RoleAssignAction) && policy != access.RolePolicy(access.RoleEndAction) && policy != access.RolePolicy(access.RoleReadAction) && policy != app.OwnershipTransferProposePolicy() && policy != app.OwnershipTransferReadPolicy() && policy != app.OwnershipTransferAcceptPolicy() && policy != app.OwnershipTransferCancelPolicy() && policy != app.SettingsPolicy(false) && policy != app.SettingsPolicy(true) && policy != app.LegalPolicy(false, false) && policy != app.LegalPolicy(false, true) && policy != app.LegalPolicy(true, true) && policy != app.StructurePolicy(false) && policy != app.StructurePolicy(true) && policy != members.EmployeePolicy(members.EmployeeRead) && policy != members.EmployeePolicy(members.EmployeeReview) && policy != members.EmployeePolicy(members.EmployeeManage) {
		return app.ErrUnavailable
	}
	write := policy.Effect == access.ConfigurationWrite
	// Plain reads share the account and root rows, so they run concurrently
	// while writers still wait for them. A settings read can initialize the
	// first version under the root (org_settings_bootstrap locks it FOR
	// UPDATE); two readers holding FOR SHARE would then deadlock on upgrade.
	exclusive := policy.Effect != access.Read || policy == app.SettingsPolicy(false) || policy == app.OwnershipTransferReadPolicy() || policy == members.InvitationPolicy(members.InvitationRead)
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
	facts := permissions.ResourceFacts{OrgID: scope.OrgID(), ID: target}
	// Response permission admits a SELF-scoped member to attempt consent.
	// The source store checks the concrete transfer participants under this
	// retained root lock before any read, receipt or state change.
	if policy.Permission == "org.ownership.respond" {
		facts.AssignedTo = member
	}
	if len(requestedRole) == 1 {
		facts.RequestedRole = requestedRole[0]
	}
	if _, err := permissions.CoveringRules(scope, member, assignments, policy.Permission, facts, now.Time); err != nil {
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

// RoleAuthority supplies current company proof through the access module port.
type RoleAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

func (g *RoleAuthority) CheckRoleTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor access.CompanyActor, member, target string, action access.RoleAction, requested permissions.Role) ([]permissions.Assignment, error) {
	if g == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil || actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return nil, access.ErrUnavailable
	}
	policy := access.RolePolicy(action)
	if policy.Validate() != nil {
		return nil, access.ErrUnavailable
	}
	err := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, app.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, policy, requested)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthenticated):
			return nil, access.ErrRoleUnauthenticated
		case errors.Is(err, app.ErrNotFound):
			return nil, access.ErrRoleTargetNotFound
		case errors.Is(err, capability.ErrEntitlementRequired):
			return nil, access.ErrEntitlementRequired
		}
		return nil, err
	}
	return g.Roles.CandidatesTx(ctx, tx, scope, member)
}

// InvitationAuthority retains the normal company proof/root/roles and captures
// actual settings in the same owning transaction. No public redemption occurs.
type InvitationAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Settings     app.SettingsIntakeRepository
}

var _ members.InvitationAuthority = (*InvitationAuthority)(nil)

func (g *InvitationAuthority) CheckInvitationTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, member string, action members.InvitationAction) (members.InvitationAuthorityFacts, error) {
	if g == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil || g.Settings == nil || actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return members.InvitationAuthorityFacts{}, members.ErrRegisterUnavailable
	}
	policy := members.InvitationPolicy(action)
	if policy.Validate() != nil {
		return members.InvitationAuthorityFacts{}, members.ErrRegisterUnavailable
	}
	e := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, app.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, scope.OrgID().String(), policy)
	if e != nil {
		switch {
		case errors.Is(e, app.ErrUnauthenticated):
			e = members.ErrEmployeeUnauthenticated
		case errors.Is(e, app.ErrNotFound):
			e = members.ErrEmployeeNotFound
		case errors.Is(e, capability.ErrEntitlementRequired):
			e = access.ErrEntitlementRequired
		}
		return members.InvitationAuthorityFacts{}, e
	}
	current, e := g.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
	if e != nil {
		return members.InvitationAuthorityFacts{}, e
	}
	m, e := current.Snapshot(scope)
	if e != nil {
		return members.InvitationAuthorityFacts{}, e
	}
	capture, e := g.Settings.CaptureSettingsTx(ctx, tx, scope)
	if e != nil {
		return members.InvitationAuthorityFacts{}, e
	}
	settings, e := capture.Settings.Snapshot(scope)
	if e != nil {
		return members.InvitationAuthorityFacts{}, e
	}
	candidates, e := g.Roles.CandidatesTx(ctx, tx, scope, member)
	if e != nil {
		return members.InvitationAuthorityFacts{}, e
	}
	return members.InvitationAuthorityFacts{Assignments: candidates, EmployeeID: m.EmployeeID, ValidityDays: settings.Values.Policies.InvitationValidityDays, SettingsRevision: settings.Revision, Now: capture.ReceivedAt}, nil
}

// MembershipAccessAuthority owns the fixed manager policies; it retains actual
// company proof and current authority through the caller's scoped transaction.
type MembershipAccessAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

func (g *MembershipAccessAuthority) CheckMembershipAccessTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, member string, action members.MembershipAccessAction) (time.Time, error) {
	if g == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil || actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return time.Time{}, members.ErrRegisterUnavailable
	}
	policy := members.MembershipAccessPolicy(action)
	if policy.Validate() != nil {
		return time.Time{}, members.ErrRegisterUnavailable
	}
	e := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, app.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, scope.OrgID().String(), policy)
	if e != nil {
		switch {
		case errors.Is(e, app.ErrUnauthenticated):
			e = members.ErrEmployeeUnauthenticated
		case errors.Is(e, app.ErrNotFound):
			e = members.ErrEmployeeNotFound
		case errors.Is(e, app.ErrUnavailable):
			e = members.ErrRegisterUnavailable
		case errors.Is(e, capability.ErrEntitlementRequired):
			e = access.ErrEntitlementRequired
		}
		return time.Time{}, e
	}
	now, e := q.New(tx).CurrentOnboardingTime(ctx)
	if e != nil {
		return time.Time{}, e
	}
	if !now.Valid || now.InfinityModifier != pgtype.Finite {
		return time.Time{}, members.ErrRegisterUnavailable
	}
	return now.Time.UTC(), nil
}

// OffboardingAuthority treats self-leave as an account-bound personal action,
// including suspended/no-role membership. It cannot grant organization access.
type OffboardingAuthority struct{ MembershipAccessAuthority }

func (g *OffboardingAuthority) CheckOffboardingTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, member string, self, write bool) error {
	if g == nil || scope.Validate() != nil || actor.Validate() != nil {
		return members.ErrEmployeeUnauthenticated
	}
	if !self {
		action := members.MembershipAccessRead
		if write {
			action = members.MembershipSuspend
		}
		_, e := g.CheckMembershipAccessTx(ctx, tx, scope, actor, member, action)
		return e
	}
	queries := q.New(tx)
	account, e := queries.ShareLegalAccount(ctx, actor.AccountID)
	if e != nil {
		return e
	}
	if account.Status != "active" || account.DeletionPending {
		return members.ErrEmployeeUnauthenticated
	}
	if account.ManagedOrg != "" {
		return members.ErrManagedRestricted
	}
	proof := q.LockLegalProofParams{AccountID: actor.AccountID, SessionID: actor.SessionID, RequireFresh: true}
	if _, e = queries.LockLegalProof(ctx, proof); e != nil {
		return members.ErrEmployeeUnauthenticated
	}
	// Prove the selected account binding before exposing any private root facts.
	if g.Memberships == nil {
		return members.ErrRegisterUnavailable
	}
	own, e := g.Memberships.GetTx(ctx, tx, scope, member)
	if errors.Is(e, members.ErrNotFound) {
		return members.ErrEmployeeNotFound
	}
	if e != nil {
		return e
	}
	v, e := own.Snapshot(scope)
	if e != nil {
		return e
	}
	bound, model, e := v.Account.Reference(scope)
	if e != nil || bound != actor.AccountID || model != "EXISTING_ACCOUNT" {
		return members.ErrEmployeeNotFound
	}

	params := q.LockLegalOrganizationParams{OrgID: scope.OrgID().String(), TermsVersion: d.SyntheticTermsVersion, TermsDigest: d.SyntheticTermsDigest}
	var root q.LockLegalOrganizationRow
	if write {
		root, e = queries.LockLegalOrganization(ctx, params)
	} else {
		v, err := queries.ShareLegalOrganization(ctx, q.ShareLegalOrganizationParams(params))
		root = q.LockLegalOrganizationRow(v)
		e = err
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return members.ErrEmployeeNotFound
	}
	if e != nil {
		return e
	}
	if !root.HasSnapshot || !root.State.Valid || !root.SyntheticSetup {
		return members.ErrRegisterUnavailable
	}
	// A wait on root must not prolong fresh proof.
	if _, e = queries.LockLegalProof(ctx, proof); e != nil {
		return members.ErrEmployeeUnauthenticated
	}
	return nil
}
