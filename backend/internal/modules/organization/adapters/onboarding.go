package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	accessapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type setupCapability interface {
	GuardWrite(context.Context, pgx.Tx, kernel.OrgScope, string) error
}
type OnboardingStore struct {
	Pool         *pgxpool.Pool
	Roots        app.TransactionRepository
	Memberships  memberapp.TransactionRepository
	Roles        accessapp.TransactionRepository
	Audit        auditapp.TransactionRunner
	Capabilities setupCapability
}

var _ app.OnboardingRepository = (*OnboardingStore)(nil)

func OnboardingAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "org.organization.created", TargetType: "organization"}, {Action: "org.organization.activated", TargetType: "organization"}}
}
func (s *OnboardingStore) ready() bool {
	return s != nil && s.Pool != nil && s.Roots != nil && s.Memberships != nil && s.Roles != nil && s.Audit != nil
}
func onboardingError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, audit.ErrReused) {
		return app.ErrOperationReused
	}
	for _, known := range []error{app.ErrUnauthenticated, app.ErrOperationReused, app.ErrNotFound, app.ErrUninitialized, d.ErrInvalid, d.ErrScope, d.ErrConflict, d.ErrSetup, d.ErrTransition, d.ErrTerms, kernel.ErrLastOrgOwner, kernel.ErrPendingOrgLimit, kernel.ErrOwnerRequiresPersonal, cap.ErrEntitlementRequired} {
		if errors.Is(err, known) {
			return err
		}
	}
	if errors.Is(err, access.ErrPermissionDenied) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P7501":
			return kernel.ErrLastOrgOwner
		case "P7503":
			return kernel.ErrPendingOrgLimit
		case "P7504":
			return d.ErrSetup
		case "P7505":
			return kernel.ErrOwnerRequiresPersonal
		case "23505":
			return d.ErrConflict
		case "23514", "22P02":
			return d.ErrInvalid
		case "23503":
			return app.ErrNotFound
		}
	}
	return app.ErrUnavailable
}
func lockPersonal(ctx context.Context, queries *q.Queries, account string) error {
	row, err := queries.LockOnboardingAccount(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if row.Status != "active" || row.DeletionPending {
		return app.ErrUnauthenticated
	}
	if row.Managed {
		return kernel.ErrOwnerRequiresPersonal
	}
	return nil
}
func creationResult(row q.CreationReceiptRow) (app.CreationResult, error) {
	id, err := kernel.ParseOrgID(row.OrgID)
	if err != nil || row.ResultRevision != 1 {
		return app.CreationResult{}, app.ErrUnavailable
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return app.CreationResult{}, app.ErrUnavailable
	}
	plan := app.Genesis{Scope: scope, UnitID: row.UnitID, EmployeeID: row.EmployeeID, MembershipID: row.MembershipID, AssignmentID: row.AssignmentID}
	if plan.Validate() != nil {
		return app.CreationResult{}, app.ErrUnavailable
	}
	return app.CreationResult{OrgID: id, UnitID: row.UnitID, EmployeeID: row.EmployeeID, MembershipID: row.MembershipID, AssignmentID: row.AssignmentID, State: d.PendingSetup, Revision: 1, DataMode: "SYNTHETIC"}, nil
}
func (s *OnboardingStore) CreateOrganization(ctx context.Context, actor app.CreationActor, plan app.Genesis, command d.Creation) (app.CreationResult, bool, error) {
	if err := actor.Validate(); err != nil {
		return app.CreationResult{}, false, err
	}
	if err := plan.Validate(); err != nil {
		return app.CreationResult{}, false, err
	}
	if err := command.Validate(); err != nil {
		return app.CreationResult{}, false, err
	}
	if !s.ready() {
		return app.CreationResult{}, false, app.ErrUnavailable
	}
	var result app.CreationResult
	var replay bool
	err := database.WithOrgTx(ctx, s.Pool, plan.Scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		if err := lockPersonal(ctx, queries, actor.AccountID); err != nil {
			return err
		}
		if _, err := queries.LockOnboardingProof(ctx, q.LockOnboardingProofParams{AccountID: actor.AccountID, SessionID: actor.SessionID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrUnauthenticated
			}
			return err
		}
		if err := queries.InstallCreationAccount(ctx, actor.AccountID); err != nil {
			return err
		}
		previous, err := queries.CreationReceipt(ctx, q.CreationReceiptParams{AccountID: actor.AccountID, OperationID: command.OperationID()})
		if err == nil {
			if previous.RequestHash != command.Digest() {
				return app.ErrOperationReused
			}
			result, err = creationResult(previous)
			replay = err == nil
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		auditReplay, err := s.Audit.RecordChangeTx(ctx, tx, plan.Scope, audit.Event{OperationID: command.OperationID(), ActorID: actor.AccountID, Action: "org.organization.created", TargetType: "organization", TargetID: plan.Scope.OrgID().String(), RequestHash: command.Digest()}, func(ctx context.Context, tx pgx.Tx) error {
			root, err := d.NewPending(plan.Scope, command.Profile())
			if err != nil {
				return err
			}
			if changed, err := s.Roots.CreateTx(ctx, tx, plan.Scope, root); err != nil {
				return err
			} else if !changed {
				return d.ErrConflict
			}
			unit, err := d.NewDefaultUnit(plan.Scope, plan.UnitID, command.UnitName(), command.Profile().TimeZone)
			if err != nil {
				return err
			}
			u, err := unit.Snapshot(plan.Scope)
			if err != nil {
				return err
			}
			if err := queries.InsertDefaultUnit(ctx, q.InsertDefaultUnitParams{OrgID: plan.Scope.OrgID().String(), ID: u.ID, Name: u.Name, TimeZone: u.TimeZone}); err != nil {
				return err
			}
			employee, err := member.NewEmployee(plan.Scope, plan.EmployeeID)
			if err != nil {
				return err
			}
			if changed, err := s.Memberships.CreateEmployeeTx(ctx, tx, plan.Scope, employee); err != nil {
				return err
			} else if !changed {
				return d.ErrConflict
			}
			membership, err := member.NewInvited(plan.Scope, plan.MembershipID, employee, member.ExistingAccount)
			if err != nil {
				return err
			}
			if changed, err := s.Memberships.CreateTx(ctx, tx, plan.Scope, membership); err != nil {
				return err
			} else if !changed {
				return d.ErrConflict
			}
			account, err := member.PersonalAccount(actor.AccountID)
			if err != nil {
				return err
			}
			membership, err = membership.BindAccount(plan.Scope, 1, account)
			if err != nil {
				return err
			}
			if changed, err := s.Memberships.SaveTx(ctx, tx, plan.Scope, 1, membership); err != nil {
				return err
			} else if !changed {
				return d.ErrConflict
			}
			now, err := queries.CurrentOnboardingTime(ctx)
			if err != nil {
				return err
			}
			if !now.Valid || now.InfinityModifier != pgtype.Finite {
				return app.ErrUnavailable
			}
			owner, err := access.RehydrateAssignment(plan.Scope, access.AssignmentSnapshot{OrgID: plan.Scope.OrgID(), ID: plan.AssignmentID, MembershipID: plan.MembershipID, AssignedBy: plan.MembershipID, Role: access.Owner, Scope: access.OrgScope, TargetID: plan.Scope.OrgID().String(), ValidFrom: now.Time, Reason: "organization creation", Revision: 1})
			if err != nil {
				return err
			}
			if changed, err := s.Roles.CreateTx(ctx, tx, plan.Scope, owner); err != nil {
				return err
			} else if !changed {
				return d.ErrConflict
			}
			membership, err = membership.Transition(plan.Scope, 2, member.MembershipActive, member.MembershipConditions{Employee: employee, AdmissionConfirmed: true, ConfirmationSatisfied: true, LiveRoleAssignments: 1})
			if err != nil {
				return err
			}
			if changed, err := s.Memberships.SaveTx(ctx, tx, plan.Scope, 2, membership); err != nil {
				return err
			} else if !changed {
				return d.ErrConflict
			}
			terms := command.Terms()
			if err := queries.InsertCreationReceipt(ctx, q.InsertCreationReceiptParams{OrgID: plan.Scope.OrgID().String(), OperationID: command.OperationID(), ActorID: actor.AccountID, RequestHash: command.Digest(), UnitID: plan.UnitID, EmployeeID: plan.EmployeeID, MembershipID: plan.MembershipID, AssignmentID: plan.AssignmentID, TermsVersion: terms.Version, TermsDigest: terms.Digest}); err != nil {
				return err
			}
			result = app.CreationResult{OrgID: plan.Scope.OrgID(), UnitID: plan.UnitID, EmployeeID: plan.EmployeeID, MembershipID: plan.MembershipID, AssignmentID: plan.AssignmentID, State: d.PendingSetup, Revision: 1, DataMode: "SYNTHETIC"}
			return nil
		})
		if err != nil {
			return err
		}
		if auditReplay {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.CreationResult{}, false, onboardingError(ctx, err)
	}
	return result, replay, nil
}

func (s *OnboardingStore) ActivateOrganization(ctx context.Context, scope kernel.OrgScope, actor app.CreationActor, memberID, operation string, expected int64) (app.ActivationResult, bool, error) {
	if scope.Validate() != nil {
		return app.ActivationResult{}, false, d.ErrScope
	}
	if actor.Validate() != nil {
		return app.ActivationResult{}, false, app.ErrUnauthenticated
	}
	account := actor.AccountID
	for _, raw := range []string{memberID, operation} {
		if id, err := kernel.ParseOrgID(raw); err != nil || id.String() != raw {
			return app.ActivationResult{}, false, d.ErrInvalid
		}
	}
	if expected < 1 || expected == 9223372036854775807 {
		return app.ActivationResult{}, false, d.ErrInvalid
	}
	if !s.ready() || s.Capabilities == nil {
		return app.ActivationResult{}, false, app.ErrUnavailable
	}
	var result app.ActivationResult
	var replay bool
	digest := app.ActivationDigest(scope, operation, expected)
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		if err := lockPersonal(ctx, queries, account); err != nil {
			return err
		}
		if _, err := queries.LockOnboardingProof(ctx, q.LockOnboardingProofParams{AccountID: account, SessionID: actor.SessionID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrUnauthenticated
			}
			return err
		}
		// Shared capability lock precedes the root: flag audit FKs may need that
		// root at commit. Reversing these locks can deadlock with a disable.
		if err := s.Capabilities.GuardWrite(ctx, tx, scope, "organizations"); err != nil {
			return err
		}
		if err := queries.LockSetupRoot(ctx, scope.OrgID().String()); err != nil {
			return err
		}
		current, err := s.Memberships.ActiveByAccountTx(ctx, tx, scope, account)
		if err != nil {
			return app.ErrUnauthenticated
		}
		m, err := current.Snapshot(scope)
		if err != nil || m.ID != memberID {
			return app.ErrUnauthenticated
		}
		roles, err := s.Roles.CandidatesTx(ctx, tx, scope, memberID)
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
		if _, err := access.CoveringRules(scope, memberID, roles, "org.settings.manage", access.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, now.Time); err != nil {
			return err
		}
		previous, err := queries.ActivationReceipt(ctx, q.ActivationReceiptParams{OrgID: scope.OrgID().String(), OperationID: operation})
		if err == nil {
			if previous.ActorID != account || previous.RequestHash != digest {
				return app.ErrOperationReused
			}
			result = app.ActivationResult{OrgID: scope.OrgID(), State: d.Active, Revision: previous.ResultRevision, DataMode: "SYNTHETIC"}
			replay = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		auditReplay, err := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: operation, ActorID: account, Action: "org.organization.activated", TargetType: "organization", TargetID: scope.OrgID().String(), RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			root, err := s.Roots.GetTx(ctx, tx, scope)
			if err != nil {
				return err
			}
			facts, err := queries.SetupFacts(ctx, q.SetupFactsParams{OrgID: scope.OrgID().String(), TermsVersion: d.SyntheticTermsVersion, TermsDigest: d.SyntheticTermsDigest})
			if err != nil {
				return err
			}
			active, err := root.Transition(scope, expected, d.Active, d.Conditions{ActiveUnits: facts.Units, ActiveOwners: facts.Owners, TermsRecorded: facts.Terms})
			if err != nil {
				return err
			}
			if changed, err := s.Roots.SaveTx(ctx, tx, scope, expected, active); err != nil {
				return err
			} else if !changed {
				return d.ErrConflict
			}
			if err := queries.InsertActivationReceipt(ctx, q.InsertActivationReceiptParams{OrgID: scope.OrgID().String(), OperationID: operation, ActorID: account, RequestHash: digest, Revision: expected + 1}); err != nil {
				return err
			}
			result = app.ActivationResult{OrgID: scope.OrgID(), State: d.Active, Revision: expected + 1, DataMode: "SYNTHETIC"}
			return nil
		})
		if err != nil {
			return err
		}
		if auditReplay {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.ActivationResult{}, false, onboardingError(ctx, err)
	}
	return result, replay, nil
}
