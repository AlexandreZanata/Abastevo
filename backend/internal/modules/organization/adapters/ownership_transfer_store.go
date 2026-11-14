package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	roles "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OwnershipTransferStore struct {
	Pool         *pgxpool.Pool
	Memberships  members.TransactionRepository
	Assignments  app.OwnershipAssignments
	Capabilities setupCapability
	Audit        auditing.TransactionRunner
	NewID        func() (string, error)
}

var _ app.OwnershipTransferRepository = (*OwnershipTransferStore)(nil)

func NewOwnershipTransferStore(pool *pgxpool.Pool, assignments app.OwnershipAssignments, runner auditing.TransactionRunner) *OwnershipTransferStore {
	return &OwnershipTransferStore{Pool: pool, Assignments: assignments, Audit: runner, NewID: jobs.NewUUIDv4}
}
func OwnershipTransferAuditDefinitions() []audit.Definition {
	result := []audit.Definition{}
	for _, kind := range []string{"proposed", "accepted", "cancelled", "declined"} {
		result = append(result, audit.Definition{Action: "org.ownership_transfer." + kind, TargetType: "ownership_transfer", Reasons: []string{kind}, RequireReason: true})
	}
	return result
}
func (s *OwnershipTransferStore) ready() bool {
	return s != nil && s.Pool != nil && s.Memberships != nil && s.Assignments != nil && s.Capabilities != nil && s.Audit != nil && s.NewID != nil
}
func transferError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "P8703":
			return d.ErrTransferNotPending
		case "P8701":
			return d.ErrInvalid
		case "P7501":
			return kernel.ErrLastOrgOwner
		case "P7505":
			return d.ErrOwnerRequiresPersonalAccount
		case "23505":
			if pe.ConstraintName == "org_ownership_transfers_open" {
				return d.ErrTransferPending
			}
		}
		return app.ErrUnavailable
	}
	// A role-period collision in the handover is a state conflict (409), never a
	// retryable outage; the transaction has already rolled back.
	if errors.Is(e, roles.ErrAssignmentConflict) {
		return roles.ErrAssignmentConflict
	}
	for _, known := range []error{app.ErrUnauthenticated, app.ErrNotFound, access.ErrNoMembership, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, d.ErrInvalid, d.ErrScope, d.ErrTransferPending, d.ErrTargetAlreadyOwner, d.ErrTransferNotPending, d.ErrTransferExpired, d.ErrTransferOperationReused, d.ErrSelfTransferForbidden, d.ErrOwnerRequiresPersonalAccount, d.ErrTargetNotActive, d.ErrTargetEmployeeNotActive, d.ErrOrganizationNotActive, kernel.ErrLastOrgOwner} {
		if errors.Is(e, known) {
			return e
		}
	}
	var denied *roles.PermissionDenial
	if errors.As(e, &denied) {
		return e
	}
	return legalError(ctx, e)
}

// Explicit private codec; generic domain formatting is redacted.
type transferReceipt struct {
	TransferID, OrgID, InitiatorMembershipID, TargetMembershipID string
	StepDownToAdmin                                              bool
	State                                                        d.OwnershipTransferState
	ExpiresAt, CreatedAt                                         time.Time
	AcceptedAt, CancelledAt, DeclinedAt                          *time.Time
	OperationID                                                  string
}

func encodeTransfer(t d.OwnershipTransfer) ([]byte, error) {
	return json.Marshal(transferReceipt{t.TransferID, t.OrgID, t.InitiatorMembershipID, t.TargetMembershipID, t.StepDownToAdmin, t.State, t.ExpiresAt, t.CreatedAt, t.AcceptedAt, t.CancelledAt, t.DeclinedAt, t.OperationID})
}
func decodeTransfer(scope kernel.OrgScope, raw []byte) (d.OwnershipTransfer, error) {
	var v transferReceipt
	if json.Unmarshal(raw, &v) != nil {
		return d.OwnershipTransfer{}, app.ErrUnavailable
	}
	t := d.OwnershipTransfer{TransferID: v.TransferID, OrgID: v.OrgID, InitiatorMembershipID: v.InitiatorMembershipID, TargetMembershipID: v.TargetMembershipID, StepDownToAdmin: v.StepDownToAdmin, State: v.State, ExpiresAt: v.ExpiresAt, CreatedAt: v.CreatedAt, AcceptedAt: v.AcceptedAt, CancelledAt: v.CancelledAt, DeclinedAt: v.DeclinedAt, OperationID: v.OperationID}
	if t.Validate(scope) != nil {
		return d.OwnershipTransfer{}, app.ErrUnavailable
	}
	return t, nil
}
func transferSnapshot(scope kernel.OrgScope, row q.GetTransferSnapshotRow) (d.OwnershipTransfer, error) {
	if !settingsInstant(row.CreatedAt) || !settingsInstant(row.ExpiresAt) {
		return d.OwnershipTransfer{}, app.ErrUnavailable
	}
	t := d.OwnershipTransfer{TransferID: row.TransferID, OrgID: row.OrgID, InitiatorMembershipID: row.InitiatorMembershipID, TargetMembershipID: row.TargetMembershipID, StepDownToAdmin: row.StepDownToAdmin, State: d.OwnershipTransferState(row.State), ExpiresAt: row.ExpiresAt.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC(), OperationID: row.OperationID}
	for _, field := range []struct {
		src  pgtype.Timestamptz
		dest **time.Time
	}{{row.AcceptedAt, &t.AcceptedAt}, {row.CancelledAt, &t.CancelledAt}, {row.DeclinedAt, &t.DeclinedAt}} {
		if field.src.Valid {
			if !settingsInstant(field.src) {
				return d.OwnershipTransfer{}, app.ErrUnavailable
			}
			at := field.src.Time.UTC()
			*field.dest = &at
		}
	}
	if t.Validate(scope) != nil {
		return d.OwnershipTransfer{}, app.ErrUnavailable
	}
	return t, nil
}
func loadTransfer(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.OwnershipTransfer, error) {
	row, e := q.New(tx).GetTransferSnapshot(ctx, q.GetTransferSnapshotParams{OrgID: scope.OrgID().String(), TransferID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return d.OwnershipTransfer{}, app.ErrNotFound
	}
	if e != nil {
		return d.OwnershipTransfer{}, e
	}
	return transferSnapshot(scope, row)
}
func transferID(raw string) pgtype.UUID { var v pgtype.UUID; _ = v.Scan(raw); return v }
func (s *OwnershipTransferStore) prelock(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, membersIDs []string) error {
	ids := map[string]bool{actor.AccountID: true}
	for _, member := range membersIDs {
		if member == "" {
			continue
		}
		account, e := q.New(tx).TransferMemberAccount(ctx, q.TransferMemberAccountParams{OrgID: scope.OrgID().String(), MemberID: member})
		if errors.Is(e, pgx.ErrNoRows) || e == nil && account == "" {
			continue
		}
		if e != nil {
			return e
		}
		ids[account] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		if _, e := q.New(tx).LockLegalAccount(ctx, id); e != nil {
			return e
		}
	}
	return nil
}
func (s *OwnershipTransferStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member string, policy access.Policy) error {
	return (companyAuthority{Memberships: s.Memberships, Roles: s.Assignments, Capabilities: s.Capabilities}).check(ctx, tx, scope, actor, member, scope.OrgID().String(), policy)
}
func transferParticipant(t d.OwnershipTransfer, member string) bool {
	return member == t.InitiatorMembershipID || member == t.TargetMembershipID
}
func validTransferMember(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member string) error {
	v, e := q.New(tx).TransferMemberFacts(ctx, q.TransferMemberFactsParams{OrgID: scope.OrgID().String(), MemberID: member})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	if e != nil {
		return e
	}
	if v.ManagedOrg != "" || v.AccessModel == "MANAGED" {
		return d.ErrOwnerRequiresPersonalAccount
	}
	if v.State != "ACTIVE" || v.AccountStatus != "active" || v.AccountID == "" || v.DeletionPending {
		return d.ErrTargetNotActive
	}
	if v.EmployeeState != "ACTIVE" {
		return d.ErrTargetEmployeeNotActive
	}
	return nil
}
func currentTransferOwner(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member string) (bool, error) {
	return q.New(tx).TransferCurrentOwner(ctx, q.TransferCurrentOwnerParams{OrgID: scope.OrgID().String(), MemberID: member})
}

// B-BR-EMP-173: a recipient with any unended, unexpired OWNER period, current
// or scheduled, is already an owner. The handover's open-ended OWNER period
// would overlap that plan, so it is refused instead of rewritten.
func (s *OwnershipTransferStore) transferTargetOwner(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member string) (bool, error) {
	if owner, e := currentTransferOwner(ctx, tx, scope, member); e != nil || owner {
		return owner, e
	}
	periods, e := s.Assignments.RemainingOwnershipAssignmentsTx(ctx, tx, scope, member)
	if e != nil {
		return false, e
	}
	for _, period := range periods {
		v, e := period.Snapshot(scope)
		if e != nil {
			return false, e
		}
		if v.Role == roles.Owner {
			return true, nil
		}
	}
	return false, nil
}
func transferDigest(scope kernel.OrgScope, kind, member, id, target string, step bool) string {
	raw, _ := json.Marshal([]any{1, scope.OrgID().String(), kind, member, id, target, step})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func (s *OwnershipTransferStore) Propose(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, t d.OwnershipTransfer) (d.OwnershipTransfer, bool, error) {
	if t.OrgID != scope.OrgID().String() || t.InitiatorMembershipID != member || legalIdentity(scope, t.TransferID) != nil || legalIdentity(scope, t.TargetMembershipID) != nil {
		return d.OwnershipTransfer{}, false, d.ErrInvalid
	}
	if member == t.TargetMembershipID {
		return d.OwnershipTransfer{}, false, d.ErrSelfTransferForbidden
	}
	return s.write(ctx, scope, actor, member, "PROPOSE", t.TransferID, t.TargetMembershipID, t.StepDownToAdmin, t.OperationID)
}
func (s *OwnershipTransferStore) Accept(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, id, op string) (d.OwnershipTransfer, bool, error) {
	return s.write(ctx, scope, actor, member, "ACCEPT", id, "", false, op)
}
func (s *OwnershipTransferStore) Cancel(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, id, op string) (d.OwnershipTransfer, bool, error) {
	return s.write(ctx, scope, actor, member, "CANCEL", id, "", false, op)
}
func (s *OwnershipTransferStore) write(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, kind, id, target string, step bool, op string) (d.OwnershipTransfer, bool, error) {
	if e := legalCall(scope, actor, member); e != nil {
		return d.OwnershipTransfer{}, false, e
	}
	if legalIdentity(scope, id) != nil || legalIdentity(scope, op) != nil {
		return d.OwnershipTransfer{}, false, d.ErrInvalid
	}
	if !s.ready() {
		return d.OwnershipTransfer{}, false, app.ErrUnavailable
	}
	digest := transferDigest(scope, kind, member, func() string {
		if kind == "PROPOSE" {
			return ""
		}
		return id
	}(), target, step)
	var result d.OwnershipTransfer
	replay, readyToCommit := false, false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var before d.OwnershipTransfer
		var lookupErr error
		ids := []string{member, target}
		if kind != "PROPOSE" {
			v, e := loadTransfer(ctx, tx, scope, id)
			lookupErr = e
			before = v
			if e == nil {
				ids = append(ids, v.InitiatorMembershipID, v.TargetMembershipID)
			}
		}
		if e := s.prelock(ctx, tx, scope, actor, ids); e != nil {
			return e
		}
		policy := app.OwnershipTransferProposePolicy()
		if kind == "ACCEPT" {
			policy = app.OwnershipTransferAcceptPolicy()
		}
		if kind == "CANCEL" {
			policy = app.OwnershipTransferCancelPolicy()
		}
		if e := s.authorize(ctx, tx, scope, actor, member, policy); e != nil {
			return e
		}
		if lookupErr != nil {
			return lookupErr
		}
		if kind != "PROPOSE" {
			var e error
			before, e = loadTransfer(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			if !transferParticipant(before, member) || kind == "ACCEPT" && member != before.TargetMembershipID {
				return app.ErrNotFound
			}
		}
		queries := q.New(tx)
		receipt, e := queries.GetOwnershipCommand(ctx, q.GetOwnershipCommandParams{OrgID: scope.OrgID().String(), OperationID: op})
		if e == nil {
			if receipt.ActorAccountID != actor.AccountID || receipt.Kind != kind || receipt.RequestHash != digest {
				return d.ErrTransferOperationReused
			}
			result, e = decodeTransfer(scope, receipt.Result)
			if e != nil {
				return e
			}
			replay = true
			readyToCommit = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		now, e := queries.CurrentOnboardingTime(ctx)
		if e != nil || !settingsInstant(now) {
			return app.ErrUnavailable
		}
		if kind == "PROPOSE" {
			var rootState string
			root, e := queries.GetRoot(ctx, scope.OrgID().String())
			rootState = root.State
			if e != nil {
				return e
			}
			if rootState != string(d.Active) {
				return d.ErrOrganizationNotActive
			}
			if e := validTransferMember(ctx, tx, scope, member); e != nil {
				return e
			}
			if e := validTransferMember(ctx, tx, scope, target); e != nil {
				return e
			}
			owner, e := currentTransferOwner(ctx, tx, scope, member)
			if e != nil {
				return e
			}
			if !owner {
				return roles.DenyPermission("org.ownership.transfer")
			}
			already, e := s.transferTargetOwner(ctx, tx, scope, target)
			if e != nil {
				return e
			}
			if already {
				return d.ErrTargetAlreadyOwner
			}
			if e := queries.ExpireTransfers(ctx, q.ExpireTransfersParams{OrgID: transferID(scope.OrgID().String()), ExpiresAt: now}); e != nil {
				return e
			}
			_, e = queries.GetPendingTransferForOrg(ctx, transferID(scope.OrgID().String()))
			if e == nil {
				return d.ErrTransferPending
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
		} else {
			if !before.IsPendingAt(now.Time) {
				if before.State == d.OwnershipTransferPending {
					return d.ErrTransferExpired
				}
				return d.ErrTransferNotPending
			}
			if kind == "ACCEPT" {
				if e := validTransferMember(ctx, tx, scope, before.InitiatorMembershipID); e != nil {
					return e
				}
				if e := validTransferMember(ctx, tx, scope, before.TargetMembershipID); e != nil {
					return e
				}
				owner, e := currentTransferOwner(ctx, tx, scope, before.InitiatorMembershipID)
				if e != nil {
					return e
				}
				if !owner {
					return d.ErrTransferNotPending
				}
				already, e := s.transferTargetOwner(ctx, tx, scope, before.TargetMembershipID)
				if e != nil {
					return e
				}
				if already {
					return d.ErrTargetAlreadyOwner
				}
			}
		}
		reason := "proposed"
		if kind == "ACCEPT" {
			reason = "accepted"
		}
		if kind == "CANCEL" {
			reason = "declined"
			if member == before.InitiatorMembershipID {
				reason = "cancelled"
			}
		}
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: op, ActorID: actor.AccountID, Action: "org.ownership_transfer." + reason, TargetType: "ownership_transfer", TargetID: id, ReasonCode: reason, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			if kind == "PROPOSE" {
				_, e := queries.CreateTransfer(ctx, q.CreateTransferParams{TransferID: transferID(id), OrgID: transferID(scope.OrgID().String()), InitiatorMembershipID: transferID(member), TargetMembershipID: transferID(target), StepDownToAdmin: step, ExpiresAt: pgtype.Timestamptz{Time: now.Time.Add(d.OwnershipTransferValidity), Valid: true}, CreatedAt: now, OperationID: transferID(op)})
				if e != nil {
					return e
				}
			} else {
				if kind == "ACCEPT" {
					if e := s.handover(ctx, tx, scope, before, now.Time); e != nil {
						return e
					}
				}
				state := d.OwnershipTransferAccepted
				if reason == "cancelled" {
					state = d.OwnershipTransferCancelled
				}
				if reason == "declined" {
					state = d.OwnershipTransferDeclined
				}
				if _, e := queries.UpdateTransferState(ctx, q.UpdateTransferStateParams{OrgID: transferID(scope.OrgID().String()), TransferID: transferID(id), State: string(state)}); e != nil {
					return e
				}
			}
			var e error
			result, e = loadTransfer(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			raw, e := encodeTransfer(result)
			if e != nil {
				return e
			}
			return queries.InsertOwnershipCommand(ctx, q.InsertOwnershipCommandParams{OrgID: scope.OrgID().String(), OperationID: op, ActorID: actor.AccountID, Kind: kind, RequestHash: digest, TransferID: id, Result: raw})
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrUnavailable
		}
		readyToCommit = true
		return nil
	})
	if e != nil {
		if readyToCommit && ctx.Err() == nil {
			return d.OwnershipTransfer{}, false, app.ErrUnavailable
		}
		return d.OwnershipTransfer{}, false, transferError(ctx, e)
	}
	return result, replay, nil
}
func (s *OwnershipTransferStore) handover(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, t d.OwnershipTransfer, now time.Time) error {
	create := func(member, by string, role roles.Role, until time.Time) error {
		id, e := s.NewID()
		if e != nil {
			return app.ErrUnavailable
		}
		v, e := roles.RehydrateAssignment(scope, roles.AssignmentSnapshot{OrgID: scope.OrgID(), ID: id, MembershipID: member, AssignedBy: by, Role: role, Scope: roles.OrgScope, TargetID: scope.OrgID().String(), ValidFrom: now, ValidTo: until, Reason: "Ownership transfer accepted", Revision: 1})
		if e != nil {
			return e
		}
		created, e := s.Assignments.CreateTx(ctx, tx, scope, v)
		if e != nil {
			return e
		}
		if !created {
			return app.ErrUnavailable
		}
		return nil
	}
	if e := create(t.TargetMembershipID, t.InitiatorMembershipID, roles.Owner, time.Time{}); e != nil {
		return e
	}
	if !t.StepDownToAdmin {
		return nil
	}
	candidates, e := s.Assignments.CandidatesTx(ctx, tx, scope, t.InitiatorMembershipID)
	if e != nil {
		return e
	}
	adminExists := false
	for _, v := range candidates {
		a, e := v.Snapshot(scope)
		if e != nil {
			return e
		}
		remaining := a.EndedAt.IsZero() && (a.ValidTo.IsZero() || a.ValidTo.After(now))
		if !remaining || a.Scope != roles.OrgScope {
			continue
		}
		if a.Role == roles.Admin && !a.ValidFrom.After(now) {
			adminExists = true
		}
	}
	periods, e := s.Assignments.RemainingOwnershipAssignmentsTx(ctx, tx, scope, t.InitiatorMembershipID)
	if e != nil {
		return e
	}
	var adminUntil time.Time
	for _, period := range periods {
		a, e := period.Snapshot(scope)
		if e != nil {
			return e
		}
		if a.Role == roles.Admin {
			if a.ValidFrom.After(now) && (adminUntil.IsZero() || a.ValidFrom.Before(adminUntil)) {
				adminUntil = a.ValidFrom
			}
			continue
		}
		if a.Role != roles.Owner {
			return app.ErrUnavailable
		}
		if _, changed, e := s.Assignments.EndTx(ctx, tx, scope, a.ID, a.Revision); e != nil {
			return e
		} else if !changed {
			return app.ErrUnavailable
		}
	}
	if !adminExists {
		// Preserve a scheduled ADMIN plan, filling only the gap before it starts.
		return create(t.InitiatorMembershipID, t.TargetMembershipID, roles.Admin, adminUntil)
	}
	return nil
}
func (s *OwnershipTransferStore) GetCurrent(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, id string) (d.OwnershipTransfer, error) {
	if e := legalCall(scope, actor, member); e != nil {
		return d.OwnershipTransfer{}, e
	}
	if id != "" && legalIdentity(scope, id) != nil {
		return d.OwnershipTransfer{}, d.ErrInvalid
	}
	if !s.ready() {
		return d.OwnershipTransfer{}, app.ErrUnavailable
	}
	var result d.OwnershipTransfer
	ready := false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.prelock(ctx, tx, scope, actor, []string{member}); e != nil {
			return e
		}
		if e := s.authorize(ctx, tx, scope, actor, member, app.OwnershipTransferReadPolicy()); e != nil {
			return e
		}
		var e error
		result, e = loadTransfer(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if !transferParticipant(result, member) {
			return app.ErrNotFound
		}
		now, e := q.New(tx).CurrentOnboardingTime(ctx)
		if e != nil || !settingsInstant(now) {
			return app.ErrUnavailable
		}
		if result.State == d.OwnershipTransferPending && !result.IsPendingAt(now.Time) {
			if _, e := q.New(tx).UpdateTransferState(ctx, q.UpdateTransferStateParams{OrgID: transferID(scope.OrgID().String()), TransferID: transferID(result.TransferID), State: "EXPIRED"}); e != nil {
				return e
			}
			result, e = loadTransfer(ctx, tx, scope, result.TransferID)
			if e != nil {
				return e
			}
		}
		ready = true
		return nil
	})
	if e != nil {
		if ready && ctx.Err() == nil {
			return d.OwnershipTransfer{}, app.ErrUnavailable
		}
		return d.OwnershipTransfer{}, transferError(ctx, e)
	}
	return result, nil
}
