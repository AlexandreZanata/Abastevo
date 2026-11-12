// Package adapters implements scoped role persistence, queries, and audit integration.
package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaccess"
	qorgaudit "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaudit"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleStore struct {
	Pool        *pgxpool.Pool
	Assignments *Store
	Audit       auditing.TransactionRunner
}

var _ app.RoleManagementRepository = (*RoleStore)(nil)

func NewRoleStore(pool *pgxpool.Pool, assignments *Store, auditRunner auditing.TransactionRunner) *RoleStore {
	return &RoleStore{
		Pool:        pool,
		Assignments: assignments,
		Audit:       auditRunner,
	}
}

func RoleAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "auth.assignment.created", TargetType: "assignment", Reasons: []string{"granted", "changed"}, RequireReason: true},
		{Action: "auth.assignment.ended", TargetType: "assignment", Reasons: []string{"ended", "changed"}, RequireReason: true},
	}
}

func (s *RoleStore) ready() bool {
	return s != nil && s.Pool != nil && s.Assignments != nil && s.Audit != nil
}

func roleStoreError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, kernel.ErrInvalidOrgScope) {
		return d.ErrAssignmentScope
	}
	for _, known := range []error{
		d.ErrAssignmentScope, d.ErrAssignmentInvalid, d.ErrAssignmentConflict, d.ErrAssignmentEnded,
		d.ErrUnknownRole, d.ErrCannotAssignOwner, d.ErrSelfModificationForbidden, d.ErrSelfAssignmentForbidden,
		app.ErrNotFound, app.ErrUnavailable, app.ErrRoleDisabled, app.ErrRoleInvalid,
		app.ErrRoleTargetNotFound, app.ErrRoleTargetNotActive, app.ErrRoleOperationReused,
		app.ErrOrgSuspended, app.ErrOrgArchived, app.ErrSetupIncomplete,
		kernel.ErrLastOrgOwner, kernel.ErrOrgAccessPolicyUnavailable, kernel.ErrPendingOrgLimit, kernel.ErrOwnerRequiresPersonal,
	} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	if errors.Is(err, audit.ErrReused) {
		return app.ErrRoleOperationReused
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P7501":
			return kernel.ErrLastOrgOwner
		case "P7502":
			return kernel.ErrOrgAccessPolicyUnavailable
		case "P7503":
			return kernel.ErrPendingOrgLimit
		case "P7505":
			return kernel.ErrOwnerRequiresPersonal
		case "23505":
			return d.ErrAssignmentConflict
		case "23503":
			return app.ErrRoleTargetNotFound
		case "23514", "22P02", "22008":
			return d.ErrAssignmentInvalid
		case "42501":
			return d.ErrAssignmentScope
		case "55000":
			return d.ErrAssignmentEnded
		}
	}
	return app.ErrUnavailable
}

func (s *RoleStore) actorRolesTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actorMemberID string) ([]d.Role, error) {
	candidates, err := s.Assignments.CandidatesTx(ctx, tx, scope, actorMemberID)
	if err != nil {
		return nil, err
	}
	roles := make([]d.Role, 0, len(candidates))
	for _, c := range candidates {
		snap, err := c.Snapshot(scope)
		if err != nil {
			return nil, err
		}
		roles = append(roles, snap.Role)
	}
	return roles, nil
}

func (s *RoleStore) checkOrgState(ctx context.Context, queries *q.Queries, orgID string, write bool) error {
	orgRow, err := queries.GetOrganizationStateForUpdate(ctx, orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrNotFound
		}
		return err
	}
	state := orgRow.State.String
	if state == "ARCHIVED" {
		return app.ErrOrgArchived
	}
	if write && state == "SUSPENDED" {
		return app.ErrOrgSuspended
	}
	if !orgRow.HasSnapshot {
		return app.ErrSetupIncomplete
	}
	return nil
}

func (s *RoleStore) checkTarget(ctx context.Context, queries *q.Queries, orgID, targetMemberID string, targetRole d.Role) error {
	targetRow, err := queries.CheckTargetMembership(ctx, q.CheckTargetMembershipParams{
		OrgID:        orgID,
		MembershipID: targetMemberID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrRoleTargetNotFound
		}
		return err
	}
	if targetRow.MembershipState != "ACTIVE" && targetRow.MembershipState != "INVITED" {
		return app.ErrRoleTargetNotActive
	}
	if targetRow.EmployeeState != "ACTIVE" {
		return app.ErrRoleTargetNotActive
	}
	if targetRole == d.Owner && targetRow.AccessModel != "EXISTING_ACCOUNT" {
		return kernel.ErrOwnerRequiresPersonal
	}
	return nil
}

func (s *RoleStore) AssignRole(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, actorMemberID, newID string, input app.RoleAssignInput) (app.RoleResult, error) {
	if !s.ready() {
		return app.RoleResult{}, app.ErrUnavailable
	}
	digest := input.Digest(scope)
	var result app.RoleResult

	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		auditQueries := qorgaudit.New(tx)

		// Check prior execution for replay idempotency
		prior, err := auditQueries.GetEvent(ctx, qorgaudit.GetEventParams{
			OrgID:       scope.OrgID().String(),
			OperationID: input.OperationID,
		})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.RequestHash != digest {
				return app.ErrRoleOperationReused
			}
			assignment, err := s.Assignments.GetTx(ctx, tx, scope, prior.TargetID)
			if err != nil {
				return err
			}
			result = app.RoleResult{Assignment: assignment, Replayed: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// Root organization state check
		if err := s.checkOrgState(ctx, queries, scope.OrgID().String(), true); err != nil {
			return err
		}

		// Actor active roles check
		actorRoles, err := s.actorRolesTx(ctx, tx, scope, actorMemberID)
		if err != nil {
			return err
		}

		// BR-AUTH-007: No escalation & self-modification rules
		if !d.CanModifyAssignment(actorMemberID, input.MembershipID, actorRoles, false) {
			return d.ErrSelfAssignmentForbidden
		}
		if !d.CanAssignRole(d.SystemCatalog(), actorRoles, input.Role) {
			return d.ErrCannotAssignOwner
		}

		// Target membership check
		if err := s.checkTarget(ctx, queries, scope.OrgID().String(), input.MembershipID, input.Role); err != nil {
			return err
		}

		validFrom := input.ValidFrom
		if validFrom.IsZero() {
			validFrom = time.Now().UTC()
		}

		assignmentSnapshot := d.AssignmentSnapshot{
			OrgID:        scope.OrgID(),
			ID:           newID,
			MembershipID: input.MembershipID,
			AssignedBy:   actorMemberID,
			Role:         input.Role,
			Scope:        input.Scope,
			TargetID:     input.TargetID,
			ValidFrom:    validFrom,
			ValidTo:      input.ValidTo,
			Reason:       input.Reason,
			Revision:     1,
		}
		assignment, err := d.RehydrateAssignment(scope, assignmentSnapshot)
		if err != nil {
			return err
		}

		event := audit.Event{
			OperationID: input.OperationID,
			ActorID:     actor.AccountID,
			Action:      "auth.assignment.created",
			TargetType:  "assignment",
			TargetID:    newID,
			ReasonCode:  "granted",
			RequestHash: digest,
		}

		replayed, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			changed, err := s.Assignments.CreateTx(ctx, tx, scope, assignment)
			if err != nil {
				return err
			}
			if !changed {
				return d.ErrAssignmentConflict
			}
			return nil
		})
		if err != nil {
			return err
		}

		loaded, err := s.Assignments.GetTx(ctx, tx, scope, newID)
		if err != nil {
			return err
		}
		result = app.RoleResult{Assignment: loaded, Replayed: replayed}
		return nil
	})

	if err != nil {
		return app.RoleResult{}, roleStoreError(ctx, err)
	}
	return result, nil
}

func (s *RoleStore) EndRole(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, actorMemberID string, input app.RoleEndInput) (app.RoleResult, error) {
	if !s.ready() {
		return app.RoleResult{}, app.ErrUnavailable
	}
	digest := input.Digest(scope)
	var result app.RoleResult

	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		auditQueries := qorgaudit.New(tx)

		// Check prior execution for replay idempotency
		prior, err := auditQueries.GetEvent(ctx, qorgaudit.GetEventParams{
			OrgID:       scope.OrgID().String(),
			OperationID: input.OperationID,
		})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.RequestHash != digest {
				return app.ErrRoleOperationReused
			}
			assignment, err := s.Assignments.GetTx(ctx, tx, scope, prior.TargetID)
			if err != nil {
				return err
			}
			result = app.RoleResult{Assignment: assignment, Replayed: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// Root organization state check (end is ReduceAccess: allowed even when suspended)
		if err := s.checkOrgState(ctx, queries, scope.OrgID().String(), false); err != nil {
			return err
		}

		// Fetch existing assignment to end
		existing, err := s.Assignments.GetTx(ctx, tx, scope, input.AssignmentID)
		if err != nil {
			return err
		}
		existingSnap, err := existing.Snapshot(scope)
		if err != nil {
			return err
		}

		// Actor active roles check
		actorRoles, err := s.actorRolesTx(ctx, tx, scope, actorMemberID)
		if err != nil {
			return err
		}

		// BR-AUTH-007: Only Owner can end their own role (self-reduction)
		if !d.CanModifyAssignment(actorMemberID, existingSnap.MembershipID, actorRoles, true) {
			return d.ErrSelfModificationForbidden
		}

		event := audit.Event{
			OperationID: input.OperationID,
			ActorID:     actor.AccountID,
			Action:      "auth.assignment.ended",
			TargetType:  "assignment",
			TargetID:    input.AssignmentID,
			ReasonCode:  "ended",
			RequestHash: digest,
		}

		var endedAssignment d.Assignment
		replayed, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			a, changed, err := s.Assignments.EndTx(ctx, tx, scope, input.AssignmentID, input.ExpectedRevision)
			if err != nil {
				return err
			}
			if !changed {
				return d.ErrAssignmentConflict
			}
			endedAssignment = a
			return nil
		})
		if err != nil {
			return err
		}

		if replayed {
			endedAssignment, err = s.Assignments.GetTx(ctx, tx, scope, input.AssignmentID)
			if err != nil {
				return err
			}
		}
		result = app.RoleResult{Assignment: endedAssignment, Replayed: replayed}
		return nil
	})

	if err != nil {
		return app.RoleResult{}, roleStoreError(ctx, err)
	}
	return result, nil
}

func (s *RoleStore) ChangeRole(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, actorMemberID, newID string, input app.RoleChangeInput) (app.RoleChangeResult, error) {
	if !s.ready() {
		return app.RoleChangeResult{}, app.ErrUnavailable
	}
	digest := input.Digest(scope)
	var result app.RoleChangeResult

	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		auditQueries := qorgaudit.New(tx)

		// Check prior execution for replay idempotency
		prior, err := auditQueries.GetEvent(ctx, qorgaudit.GetEventParams{
			OrgID:       scope.OrgID().String(),
			OperationID: input.OperationID,
		})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.RequestHash != digest {
				return app.ErrRoleOperationReused
			}
			newA, err := s.Assignments.GetTx(ctx, tx, scope, prior.TargetID)
			if err != nil {
				return err
			}
			endedA, err := s.Assignments.GetTx(ctx, tx, scope, input.AssignmentID)
			if err != nil {
				return err
			}
			result = app.RoleChangeResult{EndedAssignment: endedA, NewAssignment: newA, Replayed: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// Root organization state check
		if err := s.checkOrgState(ctx, queries, scope.OrgID().String(), true); err != nil {
			return err
		}

		// Fetch existing assignment to end
		existing, err := s.Assignments.GetTx(ctx, tx, scope, input.AssignmentID)
		if err != nil {
			return err
		}
		existingSnap, err := existing.Snapshot(scope)
		if err != nil {
			return err
		}

		// Actor active roles check
		actorRoles, err := s.actorRolesTx(ctx, tx, scope, actorMemberID)
		if err != nil {
			return err
		}

		// BR-AUTH-007: Nobody modifies their own assignment
		if !d.CanModifyAssignment(actorMemberID, existingSnap.MembershipID, actorRoles, false) {
			return d.ErrSelfModificationForbidden
		}
		if !d.CanAssignRole(d.SystemCatalog(), actorRoles, input.Role) {
			return d.ErrCannotAssignOwner
		}

		// Target membership check
		if err := s.checkTarget(ctx, queries, scope.OrgID().String(), existingSnap.MembershipID, input.Role); err != nil {
			return err
		}

		validFrom := input.ValidFrom
		if validFrom.IsZero() {
			validFrom = time.Now().UTC()
		}

		newSnapshot := d.AssignmentSnapshot{
			OrgID:        scope.OrgID(),
			ID:           newID,
			MembershipID: existingSnap.MembershipID,
			AssignedBy:   actorMemberID,
			Role:         input.Role,
			Scope:        input.Scope,
			TargetID:     input.TargetID,
			ValidFrom:    validFrom,
			ValidTo:      input.ValidTo,
			Reason:       input.Reason,
			Revision:     1,
		}
		newAssignment, err := d.RehydrateAssignment(scope, newSnapshot)
		if err != nil {
			return err
		}

		event := audit.Event{
			OperationID: input.OperationID,
			ActorID:     actor.AccountID,
			Action:      "auth.assignment.created",
			TargetType:  "assignment",
			TargetID:    newID,
			ReasonCode:  "changed",
			RequestHash: digest,
		}

		var endedAssignment d.Assignment
		replayed, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			// 1. End existing assignment
			ended, changed, err := s.Assignments.EndTx(ctx, tx, scope, input.AssignmentID, input.ExpectedRevision)
			if err != nil {
				return err
			}
			if !changed {
				return d.ErrAssignmentConflict
			}
			endedAssignment = ended

			// 2. Create new assignment
			created, err := s.Assignments.CreateTx(ctx, tx, scope, newAssignment)
			if err != nil {
				return err
			}
			if !created {
				return d.ErrAssignmentConflict
			}
			return nil
		})
		if err != nil {
			return err
		}

		if replayed {
			endedAssignment, err = s.Assignments.GetTx(ctx, tx, scope, input.AssignmentID)
			if err != nil {
				return err
			}
		}
		loadedNew, err := s.Assignments.GetTx(ctx, tx, scope, newID)
		if err != nil {
			return err
		}
		result = app.RoleChangeResult{EndedAssignment: endedAssignment, NewAssignment: loadedNew, Replayed: replayed}
		return nil
	})

	if err != nil {
		return app.RoleChangeResult{}, roleStoreError(ctx, err)
	}
	return result, nil
}

func (s *RoleStore) ReadRole(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, actorMemberID, assignmentID string) (d.Assignment, error) {
	if !s.ready() {
		return d.Assignment{}, app.ErrUnavailable
	}
	var result d.Assignment
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		if err := s.checkOrgState(ctx, queries, scope.OrgID().String(), false); err != nil {
			return err
		}
		a, err := s.Assignments.GetTx(ctx, tx, scope, assignmentID)
		if err != nil {
			return err
		}
		result = a
		return nil
	})
	if err != nil {
		return d.Assignment{}, roleStoreError(ctx, err)
	}
	return result, nil
}

func (s *RoleStore) ListRoles(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, actorMemberID string, filter app.RoleListFilter) (app.RolePage, error) {
	if !s.ready() {
		return app.RolePage{}, app.ErrUnavailable
	}
	var result app.RolePage
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		if err := s.checkOrgState(ctx, queries, scope.OrgID().String(), false); err != nil {
			return err
		}

		limit := int32(filter.Limit)
		if limit <= 0 || limit > 100 {
			limit = 20
		}

		rows, err := queries.ListAssignments(ctx, q.ListAssignmentsParams{
			OrgID:            scope.OrgID().String(),
			MembershipFilter: filter.MembershipID,
			RoleFilter:       string(filter.Role),
			StateFilter:      filter.State,
			AfterID:          filter.AfterID,
			RowLimit:         limit,
		})
		if err != nil {
			return err
		}

		items := make([]d.Assignment, 0, len(rows))
		for _, row := range rows {
			snap := d.AssignmentSnapshot{
				OrgID:        scope.OrgID(),
				ID:           row.ID,
				MembershipID: row.MembershipID,
				AssignedBy:   row.AssignedBy,
				Role:         d.Role(row.Role),
				Scope:        d.ScopeType(row.ScopeType),
				TargetID:     row.ScopeTarget,
				ValidFrom:    row.ValidFrom.Time,
				Reason:       row.Reason,
				Revision:     row.Revision,
			}
			if row.ValidTo.Valid {
				snap.ValidTo = row.ValidTo.Time
			}
			if row.EndedAt.Valid {
				snap.EndedAt = row.EndedAt.Time
			}
			a, err := d.RehydrateAssignment(scope, snap)
			if err != nil {
				return err
			}
			items = append(items, a)
		}

		var nextID string
		if len(rows) == int(limit) {
			nextID = rows[len(rows)-1].ID
		}

		result = app.RolePage{
			Assignments: items,
			NextID:      nextID,
		}
		return nil
	})
	if err != nil {
		return app.RolePage{}, roleStoreError(ctx, err)
	}
	return result, nil
}
