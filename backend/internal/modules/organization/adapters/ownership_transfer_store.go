package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	accessd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OwnershipTransferStore struct {
	Pool        *pgxpool.Pool
	Assignments *accesspg.Store
	Audit       auditing.TransactionRunner
	NewID       func() (string, error)
}

var _ app.OwnershipTransferRepository = (*OwnershipTransferStore)(nil)

func NewOwnershipTransferStore(pool *pgxpool.Pool, assignments *accesspg.Store, auditRunner auditing.TransactionRunner) *OwnershipTransferStore {
	return &OwnershipTransferStore{
		Pool:        pool,
		Assignments: assignments,
		Audit:       auditRunner,
		NewID:       jobs.NewUUIDv4,
	}
}

func OwnershipTransferAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "org.ownership_transfer.proposed", TargetType: "ownership_transfer", Reasons: []string{"proposed"}, RequireReason: false},
		{Action: "org.ownership_transfer.accepted", TargetType: "ownership_transfer", Reasons: []string{"accepted"}, RequireReason: false},
		{Action: "org.ownership_transfer.cancelled", TargetType: "ownership_transfer", Reasons: []string{"cancelled"}, RequireReason: false},
		{Action: "org.ownership_transfer.declined", TargetType: "ownership_transfer", Reasons: []string{"declined"}, RequireReason: false},
	}
}

func (s *OwnershipTransferStore) ready() bool {
	return s != nil && s.Pool != nil && s.Assignments != nil && s.Audit != nil
}

func (s *OwnershipTransferStore) Propose(ctx context.Context, scope kernel.OrgScope, t d.OwnershipTransfer) (d.OwnershipTransfer, bool, error) {
	if !s.ready() {
		return d.OwnershipTransfer{}, false, errors.New("ownership transfer store not ready")
	}
	if err := scope.Validate(); err != nil {
		return d.OwnershipTransfer{}, false, err
	}

	orgUUID, err := parseUUID(scope.OrgID().String())
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	opUUID, err := parseUUID(t.OperationID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	initUUID, err := parseUUID(t.InitiatorMembershipID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	targetUUID, err := parseUUID(t.TargetMembershipID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	transferUUID, err := parseUUID(t.TransferID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}

	requestPayload := fmt.Sprintf("%s|%s|%s|%t", t.InitiatorMembershipID, t.TargetMembershipID, t.OperationID, t.StepDownToAdmin)
	h := sha256.Sum256([]byte(requestPayload))
	digest := hex.EncodeToString(h[:])

	event := audit.Event{
		OperationID: t.OperationID,
		ActorID:     t.InitiatorMembershipID,
		Action:      "org.ownership_transfer.proposed",
		TargetType:  "ownership_transfer",
		TargetID:    t.TransferID,
		ReasonCode:  "proposed",
		RequestHash: digest,
	}

	var result d.OwnershipTransfer
	var replayed bool

	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, setErr := tx.Exec(ctx, "SELECT set_config('anpfuel.org_id', $1, true)", scope.OrgID().String()); setErr != nil {
			return setErr
		}

		queries := q.New(tx)

		// 1. Check replay by operation ID
		existingByOp, opErr := queries.GetTransferByOperationID(ctx, q.GetTransferByOperationIDParams{
			OrgID:       orgUUID,
			OperationID: opUUID,
		})
		if opErr == nil {
			// Found existing operation
			tExisting := mapTransferRow(existingByOp.TransferID, existingByOp.OrgID, existingByOp.InitiatorMembershipID, existingByOp.TargetMembershipID, existingByOp.StepDownToAdmin, existingByOp.State, existingByOp.ExpiresAt, existingByOp.AcceptedAt, existingByOp.CancelledAt, existingByOp.DeclinedAt, existingByOp.CreatedAt, existingByOp.OperationID)
			if tExisting.InitiatorMembershipID != t.InitiatorMembershipID ||
				tExisting.TargetMembershipID != t.TargetMembershipID ||
				tExisting.StepDownToAdmin != t.StepDownToAdmin {
				return d.ErrTransferOperationReused
			}
			result = tExisting
			replayed = true
			return nil
		} else if !errors.Is(opErr, pgx.ErrNoRows) {
			return opErr
		}

		// 2. Check root state is ACTIVE
		var rootState string
		if stateErr := tx.QueryRow(ctx, "SELECT state FROM organizations WHERE org_id = $1", scope.OrgID().String()).Scan(&rootState); stateErr != nil {
			return stateErr
		}
		if rootState != "ACTIVE" {
			return d.ErrOrganizationNotActive
		}

		now := time.Now()

		// 3. Expire stale pending transfers
		_ = queries.ExpireTransfers(ctx, q.ExpireTransfersParams{
			OrgID:     orgUUID,
			ExpiresAt: pgtype.Timestamptz{Time: now, Valid: true},
		})

		// 4. Check if an open transfer already exists
		pending, pendingErr := queries.GetPendingTransferForOrg(ctx, orgUUID)
		if pendingErr == nil && pending.ExpiresAt.Time.After(now) {
			return d.ErrTransferPending
		} else if pendingErr != nil && !errors.Is(pendingErr, pgx.ErrNoRows) {
			return pendingErr
		}

		// 5. Check target membership and personal account status
		targetRow, targetErr := queries.CheckTransferTarget(ctx, q.CheckTransferTargetParams{
			OrgID: orgUUID,
			ID:    targetUUID,
		})
		if targetErr != nil {
			if errors.Is(targetErr, pgx.ErrNoRows) {
				return d.ErrTargetNotActive
			}
			return targetErr
		}
		if targetRow.MembershipState != "ACTIVE" {
			return d.ErrTargetNotActive
		}
		if targetRow.EmployeeState != "ACTIVE" {
			return d.ErrTargetEmployeeNotActive
		}
		if targetRow.EnterpriseManagedOrgID.Valid {
			return d.ErrOwnerRequiresPersonalAccount
		}

		// 6. Check target is not already an owner
		targetIsOwner, ownerErr := queries.CheckTargetIsOwner(ctx, q.CheckTargetIsOwnerParams{
			OrgID:        orgUUID,
			MembershipID: targetUUID,
		})
		if ownerErr != nil {
			return ownerErr
		}
		if targetIsOwner {
			return d.ErrTargetAlreadyOwner
		}

		// 7. Check initiator is currently an owner
		initIsOwner, initErr := queries.CheckTargetIsOwner(ctx, q.CheckTargetIsOwnerParams{
			OrgID:        orgUUID,
			MembershipID: initUUID,
		})
		if initErr != nil {
			return initErr
		}
		if !initIsOwner {
			return errors.New("initiator is not an active owner")
		}

		// 8. Record audit and insert transfer
		rAudit, auditErr := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			created, createErr := queries.CreateTransfer(ctx, q.CreateTransferParams{
				TransferID:            transferUUID,
				OrgID:                 orgUUID,
				InitiatorMembershipID: initUUID,
				TargetMembershipID:    targetUUID,
				StepDownToAdmin:       t.StepDownToAdmin,
				ExpiresAt:             pgtype.Timestamptz{Time: t.ExpiresAt, Valid: true},
				CreatedAt:             pgtype.Timestamptz{Time: t.CreatedAt, Valid: true},
				OperationID:           opUUID,
			})
			if createErr != nil {
				return createErr
			}
			result = mapTransferRow(created.TransferID, created.OrgID, created.InitiatorMembershipID, created.TargetMembershipID, created.StepDownToAdmin, created.State, created.ExpiresAt, created.AcceptedAt, created.CancelledAt, created.DeclinedAt, created.CreatedAt, created.OperationID)
			return nil
		})
		if auditErr != nil {
			return auditErr
		}
		replayed = rAudit
		return nil
	})

	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	return result, replayed, nil
}

func (s *OwnershipTransferStore) Accept(ctx context.Context, scope kernel.OrgScope, transferID string, targetMemberID string, opID string) (d.OwnershipTransfer, bool, error) {
	if !s.ready() {
		return d.OwnershipTransfer{}, false, errors.New("ownership transfer store not ready")
	}
	if err := scope.Validate(); err != nil {
		return d.OwnershipTransfer{}, false, err
	}

	orgUUID, err := parseUUID(scope.OrgID().String())
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	trUUID, err := parseUUID(transferID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	targetUUID, err := parseUUID(targetMemberID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	opUUID, err := parseUUID(opID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	_ = opUUID

	requestPayload := fmt.Sprintf("ACCEPT|%s|%s|%s", transferID, targetMemberID, opID)
	h := sha256.Sum256([]byte(requestPayload))
	digest := hex.EncodeToString(h[:])

	event := audit.Event{
		OperationID: opID,
		ActorID:     targetMemberID,
		Action:      "org.ownership_transfer.accepted",
		TargetType:  "ownership_transfer",
		TargetID:    transferID,
		ReasonCode:  "accepted",
		RequestHash: digest,
	}

	var result d.OwnershipTransfer
	var replayed bool

	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, setErr := tx.Exec(ctx, "SELECT set_config('anpfuel.org_id', $1, true)", scope.OrgID().String()); setErr != nil {
			return setErr
		}

		queries := q.New(tx)

		// 1. Lock transfer row for update
		locked, lockErr := queries.LockTransferForUpdate(ctx, q.LockTransferForUpdateParams{
			OrgID:      orgUUID,
			TransferID: trUUID,
		})
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return d.ErrTransferNotFound
			}
			return lockErr
		}

		// Check replay
		if locked.State == "ACCEPTED" {
			existing, getErr := queries.GetTransferByID(ctx, q.GetTransferByIDParams{
				OrgID:      orgUUID,
				TransferID: trUUID,
			})
			if getErr != nil {
				return getErr
			}
			result = mapTransferRow(existing.TransferID, existing.OrgID, existing.InitiatorMembershipID, existing.TargetMembershipID, existing.StepDownToAdmin, existing.State, existing.ExpiresAt, existing.AcceptedAt, existing.CancelledAt, existing.DeclinedAt, existing.CreatedAt, existing.OperationID)
			replayed = true
			return nil
		}

		if locked.State != "PENDING" {
			return d.ErrTransferNotPending
		}

		// Must be the target member accepting
		if locked.TargetMembershipID != targetUUID {
			return d.ErrNotTransferTarget
		}

		now := time.Now()
		if !locked.ExpiresAt.Time.After(now) {
			_, _ = queries.UpdateTransferState(ctx, q.UpdateTransferStateParams{
				OrgID:      orgUUID,
				TransferID: trUUID,
				State:      "EXPIRED",
			})
			return d.ErrTransferExpired
		}

		// Re-check target member personal account
		targetRow, targetErr := queries.CheckTransferTarget(ctx, q.CheckTransferTargetParams{
			OrgID: orgUUID,
			ID:    targetUUID,
		})
		if targetErr != nil {
			return targetErr
		}
		if targetRow.MembershipState != "ACTIVE" || targetRow.EmployeeState != "ACTIVE" {
			return d.ErrTargetNotActive
		}
		if targetRow.EnterpriseManagedOrgID.Valid {
			return d.ErrOwnerRequiresPersonalAccount
		}

		// Execute role transitions and audit record
		rAudit, auditErr := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			// A. Create OWNER role assignment for target member
			newOwnerID, idErr := s.newID()
			if idErr != nil {
				return idErr
			}
			targetOwnerSnapshot := accessd.AssignmentSnapshot{
				OrgID:        scope.OrgID(),
				ID:           newOwnerID,
				MembershipID: targetMemberID,
				AssignedBy:   uuidToString(locked.InitiatorMembershipID),
				Role:         accessd.Owner,
				Scope:        accessd.OrgScope,
				TargetID:     scope.OrgID().String(),
				ValidFrom:    now,
				Reason:       "ownership transfer accepted",
				Revision:     1,
			}
			targetAssignment, rehydrateErr := accessd.RehydrateAssignment(scope, targetOwnerSnapshot)
			if rehydrateErr != nil {
				return rehydrateErr
			}
			created, createErr := s.Assignments.CreateTx(ctx, tx, scope, targetAssignment)
			if createErr != nil {
				return createErr
			}
			if !created {
				return d.ErrTargetAlreadyOwner
			}

			// B. If step_down_to_admin is true, step down initiator
			if locked.StepDownToAdmin {
				initMemberStr := uuidToString(locked.InitiatorMembershipID)
				candidates, candErr := s.Assignments.CandidatesTx(ctx, tx, scope, initMemberStr)
				if candErr != nil {
					return candErr
				}
				for _, cand := range candidates {
					snap, snapErr := cand.Snapshot(scope)
					if snapErr == nil && snap.Role == accessd.Owner && snap.EndedAt.IsZero() && (snap.ValidTo.IsZero() || snap.ValidTo.After(now)) {
						// End initiator owner role
						_, _, endErr := s.Assignments.EndTx(ctx, tx, scope, snap.ID, snap.Revision)
						if endErr != nil {
							return endErr
						}
					}
				}

				// Assign ADMIN role to initiator
				newAdminID, adminIDErr := s.newID()
				if adminIDErr != nil {
					return adminIDErr
				}
				adminSnapshot := accessd.AssignmentSnapshot{
					OrgID:        scope.OrgID(),
					ID:           newAdminID,
					MembershipID: initMemberStr,
					AssignedBy:   targetMemberID,
					Role:         accessd.Admin,
					Scope:        accessd.OrgScope,
					TargetID:     scope.OrgID().String(),
					ValidFrom:    now,
					Reason:       "stepped down to admin upon ownership transfer",
					Revision:     1,
				}
				adminAssignment, adminRehydrateErr := accessd.RehydrateAssignment(scope, adminSnapshot)
				if adminRehydrateErr != nil {
					return adminRehydrateErr
				}
				_, adminCreateErr := s.Assignments.CreateTx(ctx, tx, scope, adminAssignment)
				if adminCreateErr != nil {
					return adminCreateErr
				}
			}

			// C. Mark transfer accepted
			updated, upErr := queries.UpdateTransferState(ctx, q.UpdateTransferStateParams{
				OrgID:      orgUUID,
				TransferID: trUUID,
				State:      "ACCEPTED",
			})
			if upErr != nil {
				return upErr
			}
			result = mapTransferRow(updated.TransferID, updated.OrgID, updated.InitiatorMembershipID, updated.TargetMembershipID, updated.StepDownToAdmin, updated.State, updated.ExpiresAt, updated.AcceptedAt, updated.CancelledAt, updated.DeclinedAt, updated.CreatedAt, updated.OperationID)
			return nil
		})
		if auditErr != nil {
			return auditErr
		}
		replayed = rAudit
		return nil
	})

	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	return result, replayed, nil
}

func (s *OwnershipTransferStore) Cancel(ctx context.Context, scope kernel.OrgScope, transferID string, actorMemberID string, opID string, isInitiator bool) (d.OwnershipTransfer, bool, error) {
	if !s.ready() {
		return d.OwnershipTransfer{}, false, errors.New("ownership transfer store not ready")
	}
	if err := scope.Validate(); err != nil {
		return d.OwnershipTransfer{}, false, err
	}

	orgUUID, err := parseUUID(scope.OrgID().String())
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	trUUID, err := parseUUID(transferID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	actorUUID, err := parseUUID(actorMemberID)
	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}

	action := "org.ownership_transfer.declined"
	newState := "DECLINED"
	reasonCode := "declined"
	if isInitiator {
		action = "org.ownership_transfer.cancelled"
		newState = "CANCELLED"
		reasonCode = "cancelled"
	}

	requestPayload := fmt.Sprintf("%s|%s|%s|%s", action, transferID, actorMemberID, opID)
	h := sha256.Sum256([]byte(requestPayload))
	digest := hex.EncodeToString(h[:])

	event := audit.Event{
		OperationID: opID,
		ActorID:     actorMemberID,
		Action:      action,
		TargetType:  "ownership_transfer",
		TargetID:    transferID,
		ReasonCode:  reasonCode,
		RequestHash: digest,
	}

	var result d.OwnershipTransfer
	var replayed bool

	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, setErr := tx.Exec(ctx, "SELECT set_config('anpfuel.org_id', $1, true)", scope.OrgID().String()); setErr != nil {
			return setErr
		}

		queries := q.New(tx)

		locked, lockErr := queries.LockTransferForUpdate(ctx, q.LockTransferForUpdateParams{
			OrgID:      orgUUID,
			TransferID: trUUID,
		})
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return d.ErrTransferNotFound
			}
			return lockErr
		}

		if locked.State == newState {
			existing, getErr := queries.GetTransferByID(ctx, q.GetTransferByIDParams{
				OrgID:      orgUUID,
				TransferID: trUUID,
			})
			if getErr != nil {
				return getErr
			}
			result = mapTransferRow(existing.TransferID, existing.OrgID, existing.InitiatorMembershipID, existing.TargetMembershipID, existing.StepDownToAdmin, existing.State, existing.ExpiresAt, existing.AcceptedAt, existing.CancelledAt, existing.DeclinedAt, existing.CreatedAt, existing.OperationID)
			replayed = true
			return nil
		}

		if locked.State != "PENDING" {
			return d.ErrTransferNotPending
		}

		if isInitiator {
			if locked.InitiatorMembershipID != actorUUID {
				return d.ErrNotTransferParticipant
			}
		} else {
			if locked.TargetMembershipID != actorUUID {
				return d.ErrNotTransferParticipant
			}
		}

		rAudit, auditErr := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			updated, upErr := queries.UpdateTransferState(ctx, q.UpdateTransferStateParams{
				OrgID:      orgUUID,
				TransferID: trUUID,
				State:      newState,
			})
			if upErr != nil {
				return upErr
			}
			result = mapTransferRow(updated.TransferID, updated.OrgID, updated.InitiatorMembershipID, updated.TargetMembershipID, updated.StepDownToAdmin, updated.State, updated.ExpiresAt, updated.AcceptedAt, updated.CancelledAt, updated.DeclinedAt, updated.CreatedAt, updated.OperationID)
			return nil
		})
		if auditErr != nil {
			return auditErr
		}
		replayed = rAudit
		return nil
	})

	if err != nil {
		return d.OwnershipTransfer{}, false, err
	}
	return result, replayed, nil
}

func (s *OwnershipTransferStore) GetCurrent(ctx context.Context, scope kernel.OrgScope) (d.OwnershipTransfer, error) {
	if !s.ready() {
		return d.OwnershipTransfer{}, errors.New("ownership transfer store not ready")
	}
	if err := scope.Validate(); err != nil {
		return d.OwnershipTransfer{}, err
	}

	orgUUID, err := parseUUID(scope.OrgID().String())
	if err != nil {
		return d.OwnershipTransfer{}, err
	}

	var result d.OwnershipTransfer
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, setErr := tx.Exec(ctx, "SELECT set_config('anpfuel.org_id', $1, true)", scope.OrgID().String()); setErr != nil {
			return setErr
		}
		queries := q.New(tx)
		row, getErr := queries.GetPendingTransferForOrg(ctx, orgUUID)
		if getErr != nil {
			if errors.Is(getErr, pgx.ErrNoRows) {
				return d.ErrTransferNotFound
			}
			return getErr
		}

		now := time.Now()
		if !row.ExpiresAt.Time.After(now) {
			_ = queries.ExpireTransfers(ctx, q.ExpireTransfersParams{
				OrgID:     orgUUID,
				ExpiresAt: pgtype.Timestamptz{Time: now, Valid: true},
			})
			return d.ErrTransferNotFound
		}

		result = mapTransferRow(row.TransferID, row.OrgID, row.InitiatorMembershipID, row.TargetMembershipID, row.StepDownToAdmin, row.State, row.ExpiresAt, row.AcceptedAt, row.CancelledAt, row.DeclinedAt, row.CreatedAt, row.OperationID)
		return nil
	})

	if err != nil {
		return d.OwnershipTransfer{}, err
	}
	return result, nil
}

func (s *OwnershipTransferStore) newID() (string, error) {
	if s.NewID != nil {
		return s.NewID()
	}
	return jobs.NewUUIDv4()
}

func mapTransferRow(
	transferID, orgID, initiatorID, targetID pgtype.UUID,
	stepDown bool, state string,
	expiresAt, acceptedAt, cancelledAt, declinedAt, createdAt pgtype.Timestamptz,
	opID pgtype.UUID,
) d.OwnershipTransfer {
	t := d.OwnershipTransfer{
		TransferID:            uuidToString(transferID),
		OrgID:                 uuidToString(orgID),
		InitiatorMembershipID: uuidToString(initiatorID),
		TargetMembershipID:    uuidToString(targetID),
		StepDownToAdmin:       stepDown,
		State:                 d.OwnershipTransferState(state),
		CreatedAt:             createdAt.Time,
		ExpiresAt:             expiresAt.Time,
		OperationID:           uuidToString(opID),
	}
	if acceptedAt.Valid {
		tm := acceptedAt.Time
		t.AcceptedAt = &tm
	}
	if cancelledAt.Valid {
		tm := cancelledAt.Time
		t.CancelledAt = &tm
	}
	if declinedAt.Valid {
		tm := declinedAt.Time
		t.DeclinedAt = &tm
	}
	return t
}

func parseUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}
