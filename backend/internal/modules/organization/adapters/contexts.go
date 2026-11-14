package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	memberd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// contextCapability is the advisory, non-locking current capability read.
// A context read never takes the capability write/intake lock.
type contextCapability interface {
	StateTx(context.Context, pgx.Tx, kernel.OrgScope, string) (capability.State, error)
}

type AccountContextStore struct {
	Pool         *pgxpool.Pool
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities contextCapability
	Settings     app.SettingsIntakeRepository
}

var _ app.AccountContextReader = (*AccountContextStore)(nil)

func (s *AccountContextStore) ready() bool {
	return s != nil && s.Pool != nil && s.Memberships != nil && s.Roles != nil && s.Capabilities != nil && s.Settings != nil
}
func contextError(e error) error {
	if errors.Is(e, pgx.ErrNoRows) || errors.Is(e, members.ErrNotFound) {
		return app.ErrNotFound
	}
	for _, known := range []error{app.ErrNotFound, app.ErrUnauthenticated, app.ErrUnavailable, capability.ErrEntitlementRequired} {
		if errors.Is(e, known) {
			return known
		}
	}
	return app.ErrUnavailable
}
func contextProof(ctx context.Context, tx pgx.Tx, actor app.CompanyActor) (bool, error) {
	row, e := q.New(tx).ContextProof(ctx, q.ContextProofParams{AccountID: actor.AccountID, SessionID: actor.SessionID})
	if errors.Is(e, pgx.ErrNoRows) {
		return false, app.ErrUnauthenticated
	}
	return row, e
}
func contextAccount(ctx context.Context, tx pgx.Tx, actor app.CompanyActor) error {
	if actor.Validate() != nil {
		return app.ErrUnauthenticated
	}
	row, e := q.New(tx).ShareLegalAccount(ctx, actor.AccountID)
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrUnauthenticated
	}
	if e != nil {
		return e
	}
	if row.Status != "active" || row.ManagedOrg != "" || row.DeletionPending {
		return app.ErrUnauthenticated
	}
	_, e = contextProof(ctx, tx, actor)
	return e
}
func contextScope(ctx context.Context, tx pgx.Tx, org string) (kernel.OrgScope, error) {
	id, e := kernel.ParseOrgID(org)
	if e != nil || id.String() != org {
		return kernel.OrgScope{}, app.ErrNotFound
	}
	scope, e := kernel.NewOrgScope(id)
	if e != nil {
		return kernel.OrgScope{}, e
	}
	if _, e = tx.Exec(ctx, `SELECT set_config('anpfuel.account_context','',true),set_config('anpfuel.org_id',$1,true)`, org); e != nil {
		return kernel.OrgScope{}, e
	}
	return scope, database.ValidateOrgTx(ctx, tx, scope)
}
func (s *AccountContextStore) current(ctx context.Context, tx pgx.Tx, actor app.CompanyActor, scope kernel.OrgScope, member string, details bool) (app.AccountContextView, error) {
	var empty app.AccountContextView
	queries := q.New(tx)
	params := q.LockLegalOrganizationParams{OrgID: scope.OrgID().String(), TermsVersion: d.SyntheticTermsVersion, TermsDigest: d.SyntheticTermsDigest}
	var root q.LockLegalOrganizationRow
	var e error
	if details {
		root, e = queries.LockLegalOrganization(ctx, params)
	} else {
		var shared q.ShareLegalOrganizationRow
		shared, e = queries.ShareLegalOrganization(ctx, q.ShareLegalOrganizationParams(params))
		root = q.LockLegalOrganizationRow(shared)
	}
	if e != nil {
		return empty, e
	}
	if !root.HasSnapshot || !root.State.Valid || !root.SyntheticSetup {
		return empty, app.ErrNotFound
	}
	// BR-ORG-030/033: suspended and archived organizations stay readable for
	// their members; only these lifecycle states accept writes.
	writable := false
	switch d.State(root.State.String) {
	case d.PendingSetup, d.Active:
		writable = true
	case d.Suspended, d.Archived:
	default:
		return empty, app.ErrNotFound
	}
	// Membership/employee/role changes retain this root lock, so the following
	// reads observe the state after any wait and remain stable until commit.
	current, e := s.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
	if e != nil {
		if errors.Is(e, members.ErrNotFound) || errors.Is(e, pgx.ErrNoRows) {
			return empty, app.ErrNotFound
		}
		return empty, e
	}
	m, e := current.Snapshot(scope)
	if e != nil {
		return empty, e
	}
	if m.ID != member || m.Model != memberd.ExistingAccount || m.State != memberd.MembershipActive {
		return empty, app.ErrNotFound
	}
	employee, e := s.Memberships.GetEmployeeTx(ctx, tx, scope, m.EmployeeID)
	if e != nil {
		return empty, e
	}
	ev, e := employee.Snapshot(scope)
	if e != nil {
		return empty, e
	}
	if ev.State != memberd.EmployeeActive {
		return empty, app.ErrNotFound
	}
	// Only a bound current member learns the capability outcome, so a
	// non-member cannot tell capability state from NOT_FOUND. Plain read.
	enabled, e := s.Capabilities.StateTx(ctx, tx, scope, "organizations")
	if e != nil {
		return empty, e
	}
	if e := enabled.CheckWrite(); e != nil {
		return empty, e
	}
	meta, e := queries.GetRoot(ctx, scope.OrgID().String())
	if e != nil {
		return empty, e
	}
	if meta.State != root.State.String {
		return empty, app.ErrUnavailable
	}
	result := app.AccountContextView{Organization: app.AccountContext{ID: scope.OrgID().String(), Name: meta.DisplayName, MembershipID: member, State: meta.State, SuspensionReason: meta.SuspensionReason}, Permissions: []permissions.Permission{}, InvitationGrants: []permissions.ContextGrant{}}
	if !details {
		if _, e := contextProof(ctx, tx, actor); e != nil {
			return empty, e
		}
		return result, nil
	}
	candidates, e := s.Roles.CandidatesTx(ctx, tx, scope, member)
	if e != nil {
		return empty, e
	}
	capture, e := s.Settings.CaptureSettingsTx(ctx, tx, scope)
	if e != nil {
		return empty, e
	}
	values, e := capture.Settings.Snapshot(scope)
	if e != nil {
		return empty, e
	}
	result.InvitationValidityDays = values.Values.Policies.InvitationValidityDays
	// Locks on proof/root/settings may have waited; capture clock and proof again.
	fresh, e := contextProof(ctx, tx, actor)
	if e != nil {
		return empty, e
	}
	now, e := queries.CurrentOnboardingTime(ctx)
	if e != nil || !settingsInstant(now) {
		return empty, app.ErrUnavailable
	}
	// Management hints are write controls; a read-only organization shows
	// reading hints only, exactly like an expired-freshness session.
	result.Permissions, result.InvitationGrants, e = permissions.ContextHints(scope, member, candidates, now.Time, fresh && writable)
	if e != nil {
		return empty, e
	}
	return result, nil
}
func (s *AccountContextStore) ListAccountContexts(ctx context.Context, actor app.CompanyActor, limit int, after string) (app.AccountContextPage, error) {
	result := app.AccountContextPage{Organizations: []app.AccountContext{}}
	if !s.ready() {
		return result, app.ErrUnavailable
	}
	if limit < 1 || limit > 100 {
		return result, d.ErrInvalid
	}
	if after != "" {
		if id, e := kernel.ParseOrgID(after); e != nil || id.String() != after {
			return result, d.ErrInvalid
		}
	}
	e := database.WithOrgBootstrapTx(ctx, s.Pool, func(tx pgx.Tx) error {
		if e := contextAccount(ctx, tx, actor); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT set_config('anpfuel.account_context',$1,true)`, actor.AccountID); e != nil {
			return e
		}
		refs, e := q.New(tx).ListAccountContextRefs(ctx, q.ListAccountContextRefsParams{AccountID: actor.AccountID, AfterOrgID: after, RowLimit: int32(limit + 1)})
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT set_config('anpfuel.account_context','',true)`); e != nil {
			return e
		}
		if len(refs) > limit {
			result.NextAfterOrgID = refs[limit-1].OrgID
			refs = refs[:limit]
		}
		for _, ref := range refs {
			scope, e := contextScope(ctx, tx, ref.OrgID)
			if e != nil {
				return e
			}
			view, e := s.current(ctx, tx, actor, scope, ref.MembershipID, false)
			if errors.Is(e, app.ErrNotFound) || errors.Is(e, capability.ErrEntitlementRequired) {
				continue
			}
			if e != nil {
				return e
			}
			result.Organizations = append(result.Organizations, view.Organization)
		}
		_, e = contextProof(ctx, tx, actor)
		return e
	})
	if e != nil {
		return app.AccountContextPage{}, contextError(e)
	}
	return result, nil
}
func (s *AccountContextStore) ReadAccountContext(ctx context.Context, actor app.CompanyActor, scope kernel.OrgScope, member string) (app.AccountContextView, error) {
	var result app.AccountContextView
	if !s.ready() {
		return result, app.ErrUnavailable
	}
	if e := legalIdentity(scope, member); e != nil {
		return result, app.ErrNotFound
	}
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := contextAccount(ctx, tx, actor); e != nil {
			return e
		}
		var e error
		result, e = s.current(ctx, tx, actor, scope, member, true)
		return e
	})
	if e != nil {
		return app.AccountContextView{}, contextError(e)
	}
	return result, nil
}
