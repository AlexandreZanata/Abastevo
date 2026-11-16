package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
)

var (
	// ErrReservationConflict is the B-BR-EMP-295 refusal; the concrete error
	// is a ReservationConflict that carries only the conflicting window.
	ErrReservationConflict = errors.New("fleet: reservation window conflict")
	// ErrNotDue answers a time-driven settlement asked before its instant.
	ErrNotDue = errors.New("fleet: reservation outcome not due")
)

// ReservationConflict reveals the conflicting window and nothing else: no
// identifier, requester, employee, purpose, reason or kind of record.
type ReservationConflict struct {
	private
	From time.Time
	To   *time.Time
}

func (ReservationConflict) Error() string        { return ErrReservationConflict.Error() }
func (ReservationConflict) Is(target error) bool { return target == ErrReservationConflict }

// NewReservationConflict copies a valid window; an invalid one is unavailable.
func NewReservationConflict(from time.Time, to *time.Time) error {
	if !exactInstant(&from) || from.IsZero() || !exactInstant(to) || to != nil && !to.After(from) {
		return ErrUnavailable
	}
	return ReservationConflict{From: from.UTC(), To: copyInstant(to)}
}

// FleetReserve is fresh fleet.reservation.manage on one asset (EMP-FLEET-007).
const FleetReserve FleetAction = 5

// ReservePolicy is the source-owned policy of every reservation and block
// command (org-permissions-v5, EDR-195).
func ReservePolicy() access.Policy {
	return access.Policy{Permission: "fleet.reservation.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}

func digest(parts ...any) string {
	raw, _ := json.Marshal(parts)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type ReservationRequestInput struct {
	private
	OperationID, AssetID string
	Start, End           time.Time
	// EmployeeID is the reserved employee record, empty when nobody is named.
	EmployeeID string
	Purpose    string
	// Confirm creates the reservation CONFIRMED (the requester may confirm).
	Confirm bool
}
type ReservationRequest struct {
	private
	value ReservationRequestInput
}

// NewReservationRequest validates the exact B-BR-EMP-294 shape; the start
// against the server clock is the repository's check.
func NewReservationRequest(v ReservationRequestInput) (ReservationRequest, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || v.EmployeeID != "" && !canonicalID(v.EmployeeID) || !fleet.ValidReason(v.Purpose) {
		return ReservationRequest{}, fleet.ErrInvalid
	}
	window, err := fleet.NewReservationWindow(v.Start, v.End)
	if err != nil {
		return ReservationRequest{}, err
	}
	v.Start, v.End = window.From, window.To
	return ReservationRequest{value: v}, nil
}
func (c ReservationRequest) Snapshot() (ReservationRequestInput, error) {
	copy, err := NewReservationRequest(c.value)
	if err != nil {
		return ReservationRequestInput{}, err
	}
	return copy.value, nil
}
func (c ReservationRequest) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	return digest(1, "fleet.asset.reservation.request", scope.OrgID().String(), in.OperationID, in.AssetID, instantText(&in.Start), instantText(&in.End), in.EmployeeID, in.Purpose, in.Confirm)
}

// ReservationActInput confirms or cancels one reservation of the asset. A
// confirmation carries no reason.
type ReservationActInput struct {
	private
	OperationID, AssetID, ReservationID, Reason string
}
type ReservationAct struct {
	private
	command fleet.ReservationCommand
	value   ReservationActInput
}

func newReservationAct(command fleet.ReservationCommand, v ReservationActInput) (ReservationAct, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || !canonicalID(v.ReservationID) || !validOptionalReason(v.Reason) || command == fleet.ConfirmReservation && v.Reason != "" {
		return ReservationAct{}, fleet.ErrInvalid
	}
	return ReservationAct{command: command, value: v}, nil
}
func NewReservationConfirm(v ReservationActInput) (ReservationAct, error) {
	return newReservationAct(fleet.ConfirmReservation, v)
}
func NewReservationCancel(v ReservationActInput) (ReservationAct, error) {
	return newReservationAct(fleet.CancelReservation, v)
}
func (c ReservationAct) Command() fleet.ReservationCommand { return c.command }
func (c ReservationAct) Snapshot() (ReservationActInput, error) {
	if c.command != fleet.ConfirmReservation && c.command != fleet.CancelReservation {
		return ReservationActInput{}, fleet.ErrInvalid
	}
	copy, err := newReservationAct(c.command, c.value)
	if err != nil {
		return ReservationActInput{}, err
	}
	return copy.value, nil
}
func (c ReservationAct) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	return digest(1, "fleet.asset.reservation."+string(c.command), scope.OrgID().String(), in.OperationID, in.AssetID, in.ReservationID, in.Reason)
}

type BlockCreateInput struct {
	private
	OperationID, AssetID string
	Reason               fleet.BlockReason
	Note                 string
	// Start nil means the server instant of the command.
	Start, End *time.Time
}
type BlockCreate struct {
	private
	value BlockCreateInput
}

func NewBlockCreate(v BlockCreateInput) (BlockCreate, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || !validOptionalReason(v.Note) {
		return BlockCreate{}, fleet.ErrInvalid
	}
	if _, err := fleet.ParseBlockReason(string(v.Reason)); err != nil {
		return BlockCreate{}, err
	}
	if !exactInstant(v.Start) || !exactInstant(v.End) || v.Start != nil && v.End != nil && !v.End.After(*v.Start) {
		return BlockCreate{}, fleet.ErrInvalid
	}
	v.Start, v.End = copyInstant(v.Start), copyInstant(v.End)
	return BlockCreate{value: v}, nil
}
func (c BlockCreate) Snapshot() (BlockCreateInput, error) {
	copy, err := NewBlockCreate(c.value)
	if err != nil {
		return BlockCreateInput{}, err
	}
	return copy.value, nil
}
func (c BlockCreate) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	return digest(1, "fleet.asset.block.create", scope.OrgID().String(), in.OperationID, in.AssetID, in.Reason, in.Note, instantText(in.Start), instantText(in.End))
}

// BlockRemoveInput removes one block; the reason is mandatory (BR-FLEET-032).
type BlockRemoveInput struct {
	private
	OperationID, AssetID, BlockID, Reason string
}
type BlockRemove struct {
	private
	value BlockRemoveInput
}

func NewBlockRemove(v BlockRemoveInput) (BlockRemove, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || !canonicalID(v.BlockID) || !fleet.ValidReason(v.Reason) {
		return BlockRemove{}, fleet.ErrInvalid
	}
	return BlockRemove{value: v}, nil
}
func (c BlockRemove) Snapshot() (BlockRemoveInput, error) {
	copy, err := NewBlockRemove(c.value)
	if err != nil {
		return BlockRemoveInput{}, err
	}
	return copy.value, nil
}
func (c BlockRemove) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	return digest(1, "fleet.asset.block.remove", scope.OrgID().String(), in.OperationID, in.AssetID, in.BlockID, in.Reason)
}

// ReservationResult acknowledges one reservation command with the state the
// command left.
type ReservationResult struct {
	private
	AssetID, ReservationID string
	State                  fleet.ReservationState
}

// BlockResult acknowledges one block command.
type BlockResult struct {
	private
	AssetID, BlockID string
}

// ReservationEndCause records why a reservation left REQUESTED/CONFIRMED.
type ReservationEndCause string

const (
	CancelledByCommand ReservationEndCause = "CANCELLED"
	CancelledByService ReservationEndCause = "ASSET_LEFT_SERVICE"
	EndedByTime        ReservationEndCause = "TIME"
)

// ReservationView is one stored reservation with its effective state at the
// page instant (BR-FLEET-030 at read time). EndedAt is the cancellation
// instant, or the instant at which the time-driven outcome happened.
type ReservationView struct {
	private
	ID, AssetID, EmployeeID, Purpose string
	Window                           fleet.ReservationWindow
	Tolerance                        time.Duration
	State                            fleet.ReservationState
	RequestedAt                      time.Time
	ConfirmedAt                      *time.Time
	EndCause                         ReservationEndCause
	EndedAt                          *time.Time
	EndReason                        string
}

type ReservationPage struct {
	private
	Reservations []ReservationView
	At           time.Time
	NextFrom     *time.Time
	NextID       string
}

// BlockView is one stored block; a removed block keeps its removal facts.
type BlockView struct {
	private
	ID, AssetID   string
	Reason        fleet.BlockReason
	Note          string
	Period        fleet.AssignmentPeriod
	RecordedAt    time.Time
	RemovedAt     *time.Time
	RemovalReason string
}

type BlockPage struct {
	private
	Blocks   []BlockView
	At       time.Time
	NextFrom *time.Time
	NextID   string
}

// PeriodFilter is the keyset page of reservations or blocks by start.
type PeriodFilter struct {
	private
	AfterFrom *time.Time
	AfterID   string
	Limit     int
}

func (f PeriodFilter) Validate() error {
	return AssignmentFilter{AfterFrom: f.AfterFrom, AfterID: f.AfterID, Limit: f.Limit}.Validate()
}

// AvailabilitySource names a port that answered "no source" (B-BR-EMP-299).
type AvailabilitySource string

const (
	ActiveUseSource       AvailabilitySource = "ACTIVE_USE"
	MaintenanceHoldSource AvailabilitySource = "MAINTENANCE_HOLD"
	// ChecklistBlockSource is the BLOCKING checklist runs (EMP-CHECK-002).
	ChecklistBlockSource AvailabilitySource = "CHECKLIST_RUN"
)

// AvailabilityView is the derived answer at the server instant At and the
// sources that could not be consulted.
type AvailabilityView struct {
	private
	AssetID        string
	At             time.Time
	Assessment     fleet.Assessment
	MissingSources []AvailabilitySource
}

// UseAnswer is the active-use port's answer; Sourced is false when no trip
// source exists (the explicit NoActiveUse adapter, EDR-194).
type UseAnswer struct {
	private
	Sourced bool
	Uses    []fleet.UseFact
}

// ActiveUsePort answers trips in progress or paused holding the asset at an
// instant, inside the caller's organization transaction. EMP-TRIP-002
// replaces the explicit "no active use source" adapter.
type ActiveUsePort interface {
	ActiveUseDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time) (UseAnswer, error)
}

// HoldAnswer is the maintenance-hold port's answer; Sourced is false when no
// maintenance source exists.
type HoldAnswer struct {
	private
	Sourced bool
	Holds   []fleet.HoldFact
}

// MaintenanceHoldPort answers open maintenance holds of the asset at an
// instant. The maintenance epic replaces the explicit "no source" adapter.
type MaintenanceHoldPort interface {
	MaintenanceHoldsDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time) (HoldAnswer, error)
}

// ChecklistBlockAnswer is the checklist-block port's answer; Sourced is
// false when no checklist source is composed (EDR-285).
type ChecklistBlockAnswer struct {
	private
	Sourced bool
	Blocks  []fleet.ChecklistBlockFact
}

// ChecklistBlockPort answers the BLOCKING checklist runs of an asset whose
// period covers the instant, on the caller's transaction (BR-CHECK-010).
type ChecklistBlockPort interface {
	ChecklistBlocksDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time) (ChecklistBlockAnswer, error)
}

// StartAvailabilityPort is the intake check of a trip start (B-BR-EMP-300):
// the decision for a start of the asset by the membership at the declared
// instant, on the borrowed intake transaction. EMP-TRIP-002 calls it.
type StartAvailabilityPort interface {
	StartDecisionDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, string, time.Time) (fleet.StartDecision, error)
}

// ReservationOutcomes settles one due time-driven outcome (the job and the
// commands). It answers the state left, or ErrNotDue before the instant.
type ReservationOutcomes interface {
	SettleReservation(context.Context, kernel.OrgScope, string, fleet.ReservationCommand) (fleet.ReservationState, error)
}

type ReservationRepository interface {
	RequestReservation(context.Context, kernel.OrgScope, FleetActor, string, string, ReservationRequest) (ReservationResult, bool, error)
	ActOnReservation(context.Context, kernel.OrgScope, FleetActor, string, ReservationAct) (ReservationResult, bool, error)
	CreateBlock(context.Context, kernel.OrgScope, FleetActor, string, string, BlockCreate) (BlockResult, bool, error)
	RemoveBlock(context.Context, kernel.OrgScope, FleetActor, string, BlockRemove) (BlockResult, bool, error)
	ListReservations(context.Context, kernel.OrgScope, FleetActor, string, string, PeriodFilter) (ReservationPage, error)
	ListBlocks(context.Context, kernel.OrgScope, FleetActor, string, string, PeriodFilter) (BlockPage, error)
	ReadAvailability(context.Context, kernel.OrgScope, FleetActor, string, string) (AvailabilityView, error)
}

type ReservationService struct {
	Enabled    bool
	Repository ReservationRepository
	NewID      func() (string, error)
}

// guarded runs one repository call between two live decisions of exactly the
// policy and asset (the FLEET-006 pattern).
func (s ReservationService) guarded(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, asset string, policy access.Policy, call func() error) error {
	return guardedCall(ctx, s.Enabled, s.Repository, scope, actor, decision, asset, policy, call)
}

// guardedCall is the shared surface/decision/repository guard of the
// EMP-FLEET-007 and EMP-FLEET-011 services.
func guardedCall(ctx context.Context, enabled bool, repository any, scope kernel.OrgScope, actor FleetActor, decision access.Decision, asset string, policy access.Policy, call func() error) error {
	if !enabled {
		return ErrSurfaceDisabled
	}
	if err := authorized(ctx, scope, actor, decision, asset, policy); err != nil {
		return err
	}
	if unavailable(repository) {
		return ErrUnavailable
	}
	err := call()
	if guard := authorized(ctx, scope, actor, decision, asset, policy); guard != nil {
		return guard
	}
	return err
}

func (s ReservationService) newID() (string, error) {
	if s.NewID == nil {
		return "", ErrUnavailable
	}
	id, err := s.NewID()
	if err != nil || !canonicalID(id) {
		return "", ErrUnavailable
	}
	return id, nil
}

func validReservationResult(v ReservationResult, asset, id string, states ...fleet.ReservationState) bool {
	if v.AssetID != asset || !canonicalID(v.ReservationID) || id != "" && v.ReservationID != id {
		return false
	}
	for _, state := range states {
		if v.State == state {
			return true
		}
	}
	return false
}

func (s ReservationService) Request(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command ReservationRequest) (ReservationResult, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return ReservationResult{}, false, err
	}
	var v ReservationResult
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, in.AssetID, ReservePolicy(), func() error {
		id, err := s.newID()
		if err != nil {
			return err
		}
		v, replayed, err = s.Repository.RequestReservation(ctx, scope, actor, decision.Subject().MembershipID(), id, command)
		return err
	})
	if err != nil {
		return ReservationResult{}, false, err
	}
	want := fleet.ReservationRequested
	if in.Confirm {
		want = fleet.ReservationConfirmed
	}
	if !validReservationResult(v, in.AssetID, "", want) {
		return ReservationResult{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

// Act confirms or cancels one reservation.
func (s ReservationService) Act(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command ReservationAct) (ReservationResult, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return ReservationResult{}, false, err
	}
	var v ReservationResult
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, in.AssetID, ReservePolicy(), func() error {
		var err error
		v, replayed, err = s.Repository.ActOnReservation(ctx, scope, actor, decision.Subject().MembershipID(), command)
		return err
	})
	if err != nil {
		return ReservationResult{}, false, err
	}
	want, err := fleet.ReservationTransition(fleet.ReservationRequested, command.Command())
	if err != nil || !validReservationResult(v, in.AssetID, in.ReservationID, want) {
		return ReservationResult{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

func (s ReservationService) Block(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command BlockCreate) (BlockResult, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return BlockResult{}, false, err
	}
	var v BlockResult
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, in.AssetID, ReservePolicy(), func() error {
		id, err := s.newID()
		if err != nil {
			return err
		}
		v, replayed, err = s.Repository.CreateBlock(ctx, scope, actor, decision.Subject().MembershipID(), id, command)
		return err
	})
	if err != nil {
		return BlockResult{}, false, err
	}
	if v.AssetID != in.AssetID || !canonicalID(v.BlockID) {
		return BlockResult{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

func (s ReservationService) Unblock(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command BlockRemove) (BlockResult, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return BlockResult{}, false, err
	}
	var v BlockResult
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, in.AssetID, ReservePolicy(), func() error {
		var err error
		v, replayed, err = s.Repository.RemoveBlock(ctx, scope, actor, decision.Subject().MembershipID(), command)
		return err
	})
	if err != nil {
		return BlockResult{}, false, err
	}
	if v.AssetID != in.AssetID || v.BlockID != in.BlockID {
		return BlockResult{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

func validReservationView(v ReservationView, asset string, at time.Time) bool {
	if !canonicalID(v.ID) || v.AssetID != asset || v.EmployeeID != "" && !canonicalID(v.EmployeeID) || !fleet.ValidReason(v.Purpose) || !validInstant(v.RequestedAt) || !validOptionalReason(v.EndReason) {
		return false
	}
	if _, err := fleet.NewReservationWindow(v.Window.From, v.Window.To); err != nil || v.Tolerance < fleet.MinNoShowTolerance || v.Tolerance > fleet.MaxNoShowTolerance {
		return false
	}
	if _, err := fleet.ParseReservationState(string(v.State)); err != nil || v.ConfirmedAt != nil && !validInstant(*v.ConfirmedAt) {
		return false
	}
	switch v.State {
	case fleet.ReservationRequested, fleet.ReservationConfirmed, fleet.ReservationFulfilled:
		return v.EndCause == "" && v.EndedAt == nil && (v.State == fleet.ReservationRequested) == (v.ConfirmedAt == nil)
	case fleet.ReservationCancelled:
		return (v.EndCause == CancelledByCommand || v.EndCause == CancelledByService) && v.EndedAt != nil
	case fleet.ReservationExpired:
		return v.EndCause == EndedByTime && v.ConfirmedAt == nil && v.EndedAt != nil && v.EndedAt.Equal(v.Window.From) && !at.Before(v.Window.From)
	case fleet.ReservationNoShow:
		r := fleet.Reservation{State: fleet.ReservationConfirmed, Window: v.Window, Tolerance: v.Tolerance, Confirmed: true}
		return v.EndCause == EndedByTime && v.ConfirmedAt != nil && v.EndedAt != nil && v.EndedAt.Equal(r.NoShowDeadline()) && !at.Before(r.NoShowDeadline())
	}
	return false
}

// validPeriodOrder checks the start-then-identifier order and the cursor.
func validPeriodOrder(n, limit int, from func(int) time.Time, id func(int) string, nextFrom *time.Time, nextID string) bool {
	if n > limit {
		return false
	}
	for i := 1; i < n; i++ {
		if from(i).Before(from(i-1)) || from(i).Equal(from(i-1)) && id(i) <= id(i-1) {
			return false
		}
	}
	if nextID == "" {
		return nextFrom == nil
	}
	return n == limit && id(n-1) == nextID && nextFrom != nil && nextFrom.Equal(from(n-1))
}

func (s ReservationService) List(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, asset string, filter PeriodFilter) (ReservationPage, error) {
	if !canonicalID(asset) || filter.Validate() != nil {
		return ReservationPage{}, fleet.ErrInvalid
	}
	var v ReservationPage
	err := s.guarded(ctx, scope, actor, decision, asset, Policy(false), func() error {
		var err error
		v, err = s.Repository.ListReservations(ctx, scope, actor, decision.Subject().MembershipID(), asset, filter)
		return err
	})
	if err != nil {
		return ReservationPage{}, err
	}
	rows := v.Reservations
	if !validInstant(v.At) || !validPeriodOrder(len(rows), filter.Limit, func(i int) time.Time { return rows[i].Window.From }, func(i int) string { return rows[i].ID }, v.NextFrom, v.NextID) {
		return ReservationPage{}, ErrUnavailable
	}
	out := ReservationPage{Reservations: make([]ReservationView, 0, len(rows)), At: v.At, NextFrom: copyInstant(v.NextFrom), NextID: v.NextID}
	for _, row := range rows {
		if !validReservationView(row, asset, v.At) {
			return ReservationPage{}, ErrUnavailable
		}
		row.ConfirmedAt, row.EndedAt = copyInstant(row.ConfirmedAt), copyInstant(row.EndedAt)
		out.Reservations = append(out.Reservations, row)
	}
	return out, nil
}

func validBlockView(v BlockView, asset string) bool {
	if !canonicalID(v.ID) || v.AssetID != asset || !validOptionalReason(v.Note) || !validInstant(v.RecordedAt) {
		return false
	}
	if _, err := fleet.ParseBlockReason(string(v.Reason)); err != nil {
		return false
	}
	if _, err := fleet.NewAssignmentPeriod(v.Period.From, v.Period.To); err != nil {
		return false
	}
	if v.RemovedAt == nil {
		return v.RemovalReason == "" && !v.Period.Empty()
	}
	return validInstant(*v.RemovedAt) && fleet.ValidReason(v.RemovalReason) && v.Period.To != nil
}

func (s ReservationService) Blocks(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, asset string, filter PeriodFilter) (BlockPage, error) {
	if !canonicalID(asset) || filter.Validate() != nil {
		return BlockPage{}, fleet.ErrInvalid
	}
	var v BlockPage
	err := s.guarded(ctx, scope, actor, decision, asset, Policy(false), func() error {
		var err error
		v, err = s.Repository.ListBlocks(ctx, scope, actor, decision.Subject().MembershipID(), asset, filter)
		return err
	})
	if err != nil {
		return BlockPage{}, err
	}
	rows := v.Blocks
	if !validInstant(v.At) || !validPeriodOrder(len(rows), filter.Limit, func(i int) time.Time { return rows[i].Period.From }, func(i int) string { return rows[i].ID }, v.NextFrom, v.NextID) {
		return BlockPage{}, ErrUnavailable
	}
	out := BlockPage{Blocks: make([]BlockView, 0, len(rows)), At: v.At, NextFrom: copyInstant(v.NextFrom), NextID: v.NextID}
	for _, row := range rows {
		if !validBlockView(row, asset) {
			return BlockPage{}, ErrUnavailable
		}
		row.Period.To, row.RemovedAt = copyInstant(row.Period.To), copyInstant(row.RemovedAt)
		out.Blocks = append(out.Blocks, row)
	}
	return out, nil
}

// Availability reads the derived answer at the server instant (B-BR-EMP-299).
func (s ReservationService) Availability(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, asset string) (AvailabilityView, error) {
	if !canonicalID(asset) {
		return AvailabilityView{}, fleet.ErrInvalid
	}
	var v AvailabilityView
	err := s.guarded(ctx, scope, actor, decision, asset, Policy(false), func() error {
		var err error
		v, err = s.Repository.ReadAvailability(ctx, scope, actor, decision.Subject().MembershipID(), asset)
		return err
	})
	if err != nil {
		return AvailabilityView{}, err
	}
	if v.AssetID != asset || !validInstant(v.At) || len(v.MissingSources) > 3 {
		return AvailabilityView{}, ErrUnavailable
	}
	if (len(v.Assessment.Causes) == 0) != (v.Assessment.Availability == fleet.Available) || len(v.Assessment.Causes) > 0 && v.Assessment.Causes[0].Availability != v.Assessment.Availability {
		return AvailabilityView{}, ErrUnavailable
	}
	for i, source := range v.MissingSources {
		if source != ActiveUseSource && source != MaintenanceHoldSource && source != ChecklistBlockSource || i > 0 && source <= v.MissingSources[i-1] {
			return AvailabilityView{}, ErrUnavailable
		}
	}
	out := v
	out.MissingSources = append([]AvailabilitySource{}, v.MissingSources...)
	out.Assessment.Causes = make([]fleet.Cause, 0, len(v.Assessment.Causes))
	for _, cause := range v.Assessment.Causes {
		cause.To = copyInstant(cause.To)
		out.Assessment.Causes = append(out.Assessment.Causes, cause)
	}
	return out, nil
}

// SettlementDigest binds one time-driven settlement to the reservation and
// the stored revision it moves (the audit request hash, EDR-193).
func SettlementDigest(scope kernel.OrgScope, reservation string, revision int64, command fleet.ReservationCommand) string {
	if scope.Validate() != nil || !canonicalID(reservation) || revision < 1 {
		return ""
	}
	return digest(1, "fleet.asset.reservation.settle", scope.OrgID().String(), reservation, revision, command)
}
