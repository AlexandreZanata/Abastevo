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
	"github.com/jackc/pgx/v5"
	"slices"
)

type RedemptionAuthority struct {
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Settings     app.SettingsIntakeRepository
}

func (g *RedemptionAuthority) CheckPersonalTx(ctx context.Context, tx pgx.Tx, actor members.EmployeeActor) error {
	if actor.Validate() != nil {
		return members.ErrEmployeeUnauthenticated
	}
	a, e := q.New(tx).PersonalRedemptionProof(ctx, q.PersonalRedemptionProofParams{AccountID: actor.AccountID, SessionID: actor.SessionID})
	if errors.Is(e, pgx.ErrNoRows) {
		return members.ErrEmployeeUnauthenticated
	}
	if e != nil {
		return e
	}
	if a.Status != "active" || a.DeletionPending || !a.Proof {
		return members.ErrEmployeeUnauthenticated
	}
	if a.Managed {
		return members.ErrManagedRestricted
	}
	return nil
}
func (g *RedemptionAuthority) LockRedemptionTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, invitation, targetAccount string, write bool) error {
	if e := g.CheckPersonalTx(ctx, tx, actor); e != nil {
		return e
	}
	queries := q.New(tx)
	issuer, e := queries.RedemptionIssuerAccount(ctx, q.RedemptionIssuerAccountParams{OrgID: scope.OrgID().String(), ID: invitation})
	if e != nil {
		return e
	}
	ids := []string{actor.AccountID}
	if issuer != "" {
		ids = append(ids, issuer)
	}
	if targetAccount != "" {
		ids = append(ids, targetAccount)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		if _, e := queries.LockLegalAccount(ctx, id); e != nil {
			return e
		}
	}
	proof := q.LockLegalProofParams{AccountID: actor.AccountID, SessionID: actor.SessionID, RequireFresh: true}
	if _, e := queries.LockLegalProof(ctx, proof); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return members.ErrEmployeeUnauthenticated
		}
		return e
	}
	if write {
		if e := g.Capabilities.GuardWrite(ctx, tx, scope, "organizations"); e != nil {
			return members.ErrTokenInvalid
		}
	}
	// The root is locked after all accounts, proof and capability, exactly as
	// account erasure and existing company writes require.
	if e := queries.LockSetupRoot(ctx, scope.OrgID().String()); e != nil {
		return e
	}
	if e := g.CheckPersonalTx(ctx, tx, actor); e != nil {
		return e
	}
	return nil
}
func (g *RedemptionAuthority) CheckRedemptionTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, invitation string) (members.RedemptionFacts, error) {
	if g == nil || g.Roles == nil || g.Settings == nil || g.Capabilities == nil {
		return members.RedemptionFacts{}, members.ErrRegisterUnavailable
	}
	if e := g.CheckPersonalTx(ctx, tx, actor); e != nil {
		return members.RedemptionFacts{}, e
	}
	queries := q.New(tx)
	root, e := queries.RedemptionOrganizationFacts(ctx, scope.OrgID().String())
	if e != nil {
		return members.RedemptionFacts{}, e
	}
	capture, e := g.Settings.CaptureSettingsTx(ctx, tx, scope)
	if e != nil {
		return members.RedemptionFacts{}, e
	}
	settings, e := capture.Settings.Snapshot(scope)
	if e != nil {
		return members.RedemptionFacts{}, e
	}
	alias, e := queries.RedemptionAccountAlias(ctx, actor.AccountID)
	if e != nil {
		return members.RedemptionFacts{}, e
	}
	f := members.RedemptionFacts{Name: root.DisplayName.String, Alias: alias, Verified: root.Verified, RootRevision: root.Revision, SettingsRevision: settings.Revision, RequireConfirmation: settings.Values.Policies.RequireInvitationConfirmation, Now: capture.ReceivedAt}
	if !root.HasSnapshot || !root.State.Valid || (root.State.String != "ACTIVE" && root.State.String != "PENDING_SETUP") {
		return f, nil
	}
	source, e := queries.RedemptionSourceFacts(ctx, q.RedemptionSourceFactsParams{OrgID: scope.OrgID().String(), ID: invitation})
	if errors.Is(e, pgx.ErrNoRows) {
		return f, nil
	}
	if e != nil {
		return f, e
	}
	if source.IssuerState != "ACTIVE" || source.IssuerEmployeeState != "ACTIVE" || source.EmployeeState != "ACTIVE" || source.IssuerAccountState != "active" || source.IssuerDeletion {
		return f, nil
	}
	f.Assignments, e = g.Roles.CandidatesTx(ctx, tx, scope, source.MemberID)
	if e != nil {
		return f, e
	}
	_, e = permissions.CoveringRules(scope, source.MemberID, f.Assignments, "idn.invitation.manage", permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, f.Now)
	f.Eligible = e == nil
	return f, nil
}
