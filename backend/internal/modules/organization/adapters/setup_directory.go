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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

var _ app.CreatedSetupReader = (*OnboardingStore)(nil)

func (s *OnboardingStore) ListCreatedSetups(ctx context.Context, actor app.CompanyActor, limit int, after string) (app.CreatedSetupPage, error) {
	if limit < 1 || limit > 100 || after != "" && !canonicalSetupID(after) {
		return app.CreatedSetupPage{}, d.ErrInvalid
	}
	if !s.ready() || s.Capabilities == nil {
		return app.CreatedSetupPage{}, app.ErrUnavailable
	}
	result := app.CreatedSetupPage{AccountID: actor.AccountID, Setups: []app.CreatedSetup{}}
	e := database.WithOrgBootstrapTx(ctx, s.Pool, func(tx pgx.Tx) error {
		// Settings-read authority upgrades its account/root lock for possible
		// settings bootstrap. Take that account lock first, so simultaneous
		// recovery reads never deadlock while upgrading shared account locks.
		if actor.Validate() != nil {
			return app.ErrUnauthenticated
		}
		account, e := q.New(tx).LockLegalAccount(ctx, actor.AccountID)
		if errors.Is(e, pgx.ErrNoRows) {
			return app.ErrUnauthenticated
		}
		if e != nil {
			return e
		}
		if account.Status != "active" || account.ManagedOrg != "" || account.DeletionPending {
			return app.ErrUnauthenticated
		}
		if _, e := contextProof(ctx, tx, actor); e != nil {
			return e
		}
		if e := q.New(tx).InstallCreationAccount(ctx, actor.AccountID); e != nil {
			return e
		}
		refs, e := q.New(tx).ListCreatedSetupRefs(ctx, q.ListCreatedSetupRefsParams{AccountID: actor.AccountID, AfterOrgID: after, RowLimit: int32(limit + 1)})
		if e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT set_config('anpfuel.creation_account','',true)`); e != nil {
			return e
		}
		// Advance over scanned owned receipts, including unauthorized entries.
		// A filtered empty page is not the end while its cursor is nonempty.
		if len(refs) > limit {
			result.NextAfterOrgID = refs[limit-1]
			refs = refs[:limit]
		}
		for _, ref := range refs {
			scope, e := contextScope(ctx, tx, ref)
			if e != nil {
				return e
			}
			current, e := s.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
			if e != nil {
				if errors.Is(e, pgx.ErrNoRows) || errors.Is(e, members.ErrNotFound) || errors.Is(e, app.ErrNotFound) || errors.Is(e, access.ErrNoMembership) {
					continue
				}
				return e
			}
			member, e := current.Snapshot(scope)
			if e != nil {
				return e
			}
			if e := (companyAuthority{Memberships: s.Memberships, Roles: s.Roles, Capabilities: s.Capabilities}).check(ctx, tx, scope, actor, member.ID, scope.OrgID().String(), app.SettingsPolicy(false)); e != nil {
				if errors.Is(e, permissions.ErrPermissionDenied) || errors.Is(e, access.ErrNoMembership) || errors.Is(e, app.ErrNotFound) {
					continue
				}
				return e
			}
			root, e := s.Roots.GetTx(ctx, tx, scope)
			if e != nil {
				return e
			}
			value, e := root.Snapshot(scope)
			if e != nil {
				return e
			}
			result.Setups = append(result.Setups, app.CreatedSetup{ID: ref, Name: value.Profile.Name, MembershipID: member.ID, State: string(value.State), RootRevision: value.Revision})
		}
		_, e = contextProof(ctx, tx, actor)
		return e
	})
	if e != nil {
		return app.CreatedSetupPage{}, contextError(e)
	}
	return result, nil
}

func canonicalSetupID(raw string) bool {
	value, e := kernel.ParseOrgID(raw)
	return e == nil && value.String() == raw
}
