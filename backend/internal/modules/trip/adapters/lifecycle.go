package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// ---- trip.pause, trip.resume, trip.cancel (EMP-TRIP-011, B-BR-EMP-391) ----

type transitionHandler struct {
	ports
	lifecycle app.LifecycleStore
	kind      string
}

// NewTransitionHandler composes one lifecycle owner (trip.pause, trip.resume
// or trip.cancel) from real ports only: the trip authority, the fleet
// lifecycle lookup that takes the root and asset locks, the trip store and the
// lifecycle store.
func NewTransitionHandler(kind string, authority app.Authority, readings app.Readings, trips app.Store, lifecycle app.LifecycleStore) (syncapp.Handler, error) {
	p := ports{authority: authority, readings: readings, store: trips}
	if _, ok := app.LifecycleCommandOf(kind); !ok || !p.ready() || absent(lifecycle) {
		return nil, syncapp.ErrUnavailable
	}
	return &transitionHandler{ports: p, lifecycle: lifecycle, kind: kind}, nil
}

func (h *transitionHandler) Descriptor() syncapp.Descriptor { return descriptor(h.kind) }

// ReviewProof is the held lifecycle proof (OFFL-008).
func (h *transitionHandler) ReviewProof() syncapp.ReviewProof { return h.lifecycle }

func (h *transitionHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.TransitionCommand, app.TransitionInput, error) {
	if h == nil || !h.ready() || absent(h.lifecycle) {
		return app.TransitionCommand{}, app.TransitionInput{}, syncapp.ErrUnavailable
	}
	if err := bound(ctx, db, source, op, mode, h.kind); err != nil {
		return app.TransitionCommand{}, app.TransitionInput{}, err
	}
	command, err := app.ParseTransition(op)
	if err != nil {
		return app.TransitionCommand{}, app.TransitionInput{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.TransitionCommand{}, app.TransitionInput{}, syncapp.ErrUnavailable
	}
	return command, v, nil
}

// trip reads the command's trip; a trip not stored here (undelivered, held or
// another organization's) admits only the current device and is a pending
// dependency that reveals nothing (EDR-202).
func (h *transitionHandler) trip(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.TransitionInput, mode syncapp.Mode) (app.Record, error) {
	record, err := h.store.ReadTripTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID)
	if errors.Is(err, app.ErrNotFound) {
		if err := h.authority.CheckTripTx(ctx, syncapp.Borrow(db), source.Scope, source, app.Target{TripID: v.TripID}, action(mode)); err != nil {
			return app.Record{}, denied(err)
		}
		return app.Record{}, syncapp.ErrDependencyPending
	}
	if err != nil {
		return app.Record{}, syncapp.ErrUnavailable
	}
	target := app.Target{TripID: record.ID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID, AuthorMembershipID: record.AuthorMembershipID}
	if err := h.authority.CheckTripTx(ctx, syncapp.Borrow(db), source.Scope, source, target, action(mode)); err != nil {
		return app.Record{}, denied(err)
	}
	return record, nil
}

func (h *transitionHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	_, v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.root(ctx, db, source, mode)
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.ErrUnavailable
	case err != nil:
		return err
	}
	record, err := h.trip(ctx, db, source, v, mode)
	if err != nil || mode == syncapp.ReadReceipt {
		if mode == syncapp.ReadReceipt && errors.Is(err, syncapp.ErrDependencyPending) {
			return nil
		}
		return err
	}
	_, err = h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt)
	return err
}

// transitionHold maps a refusal at receipt to its held code (B-BR-EMP-391):
// a closed trip is TRIP_ALREADY_CLOSED; a stale revision, or a state that no
// longer admits the command, is STALE_REVISION; "" is not a hold.
func transitionHold(record app.Record, err error) string {
	switch {
	case errors.Is(err, trip.ErrInvalidTransition) && record.State.Terminal():
		return app.TripAlreadyClosed
	case errors.Is(err, trip.ErrConflict), errors.Is(err, trip.ErrInvalidTransition):
		return app.StaleRevision
	}
	return ""
}

// transitionRefusal maps a final refusal at receipt (EDR-253).
func transitionRefusal(err error) (syncapp.Outcome, error) {
	switch {
	case errors.Is(err, trip.ErrForbidden):
		return syncapp.Outcome{}, syncapp.SourceDenied(orgapi.Forbidden)
	case errors.Is(err, trip.ErrBeforeStart), errors.Is(err, trip.ErrInvalid), errors.Is(err, trip.ErrScope), errors.Is(err, trip.ErrIncomplete):
		return malformed(), nil
	}
	return syncapp.Outcome{}, syncapp.ErrUnavailable
}

func (h *transitionHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	command, v, err := h.request(ctx, db, source, op, syncapp.Write)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, syncapp.Write); err != nil {
			return syncapp.Outcome{}, err
		}
		return malformed(), nil
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	case err != nil:
		return syncapp.Outcome{}, err
	}
	record, err := h.trip(ctx, db, source, v, syncapp.Write)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	disposition, err := h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if _, err := h.lock(ctx, db, source.Scope, record.AssetID); err != nil {
		return syncapp.Outcome{}, err
	}
	// Re-read under the locks: a concurrent command may have moved the trip.
	if record, err = h.store.ReadTripTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	facts, err := h.lifecycle.TransitionFactsTx(ctx, syncapp.Borrow(db), source.Scope, v.OperationID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if facts.ExistingOperation != "" || facts.HeldOperation != "" {
		// A stored fact without its stored receipt is never re-acknowledged.
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	next, err := app.TransitionDomain(source.Scope, record, v)
	if code := transitionHold(record, err); code != "" {
		review, err := h.lifecycle.HoldTransitionTx(ctx, syncapp.Borrow(db), source, op, command, code)
		return held(review, code, err)
	}
	if err != nil {
		return transitionRefusal(err)
	}
	// Changed rights store nothing until a held owner exists (EMP-TRIP-007).
	if disposition != app.Apply {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	employee, err := h.store.AuthorEmployeeTx(ctx, syncapp.Borrow(db), source.Scope, source.MembershipID)
	if err != nil || employee != record.AuthorEmployeeID {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.lifecycle.InsertTransitionTx(ctx, syncapp.Borrow(db), source.Scope, source, command, next, employee); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if disposition, err := h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt); err != nil || disposition != app.Apply {
		if err == nil {
			err = syncapp.ErrUnavailable
		}
		return syncapp.Outcome{}, err
	}
	revision := next.Revision()
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

// storedTransition proves the stored lifecycle row is exactly this operation's.
func storedTransition(source syncapp.Source, v app.TransitionInput, record app.Record, revision int64) bool {
	for _, x := range record.Transitions {
		if x.OperationID == v.OperationID {
			return x.Revision == revision && x.Revision == v.Expected+1 && x.Command == v.Command && x.PayloadDigest == v.PayloadDigest &&
				sameText(x.Reason, v.Reason) && x.AuthorMembershipID == source.MembershipID && x.AuthorEmployeeID == record.AuthorEmployeeID &&
				x.EnrollmentID == source.EnrollmentID && x.Declared.OccurredAt.Equal(v.Declared.OccurredAt) && x.Declared.EntryZone == v.Declared.EntryZone &&
				x.Declared.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds && !x.ReceivedAt.IsZero()
		}
	}
	return false
}

func (h *transitionHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	_, v, err := h.request(ctx, db, source, op, mode)
	if !confirmable(err, mode, outcome) {
		return syncapp.ErrUnavailable
	}
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.confirmMalformed(ctx, db, source, mode, outcome)
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.ErrUnavailable
	case err != nil:
		return err
	}
	record, err := h.trip(ctx, db, source, v, mode)
	if errors.Is(err, syncapp.ErrDependencyPending) && outcome.Result != protocol.Accepted && outcome.Result != protocol.AcceptedForReview {
		return nil
	}
	if err != nil {
		return err
	}
	if mode == syncapp.Write {
		disposition, err := h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt)
		if err != nil {
			return err
		}
		if outcome.Result == protocol.Accepted && disposition != app.Apply {
			return syncapp.ErrUnavailable
		}
	}
	switch outcome.Result {
	case protocol.AcceptedForReview:
		if outcome.Code != app.TripAlreadyClosed && outcome.Code != app.StaleRevision || outcome.EntityRevision != nil {
			return syncapp.ErrUnavailable
		}
		if err := h.lifecycle.CheckHeldTx(ctx, syncapp.Borrow(db), source, op, outcome.Copy()); err != nil {
			if errors.Is(err, syncapp.ErrSourceDenied) {
				return err
			}
			return syncapp.SourceDenied(err)
		}
	case protocol.Accepted:
		if outcome.EntityRevision == nil || outcome.Code != "" || len(outcome.Warnings) != 0 || !storedTransition(source, v, record, *outcome.EntityRevision) {
			return syncapp.ErrUnavailable
		}
	}
	return nil
}

// ---- lifecycle store (EDR-253) ----

var _ app.LifecycleStore = (*Store)(nil)

func transitionRecords(rows []q.ListTripTransitionsRow) ([]app.TransitionRecord, error) {
	out := make([]app.TransitionRecord, 0, len(rows))
	for _, row := range rows {
		at, ok := exact(row.OccurredAt, row.OccurredSubmicro)
		cmd, errCmd := trip.ParseCommand(row.Command)
		from, errFrom := trip.ParseState(row.FromState)
		to, errTo := trip.ParseState(row.ToState)
		if !ok || errCmd != nil || errFrom != nil || errTo != nil || !row.ReceivedAt.Valid {
			return nil, app.ErrUnavailable
		}
		out = append(out, app.TransitionRecord{Revision: row.Revision, Command: cmd, From: from, To: to, Reason: optionalText(row.Reason),
			OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, AuthorMembershipID: row.AuthorMembershipID, AuthorEmployeeID: row.AuthorEmployeeID,
			EnrollmentID: row.EnrollmentID, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds},
			ReceivedAt: row.ReceivedAt.Time.UTC()})
	}
	return out, nil
}

// TransitionFactsTx answers the operations already holding the operation identity.
func (s *Store) TransitionFactsTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, operation string) (app.TransitionFacts, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil || !canonicalID(operation) {
		return app.TransitionFacts{}, app.ErrUnavailable
	}
	row, err := queries.TripTransitionFacts(ctx, q.TripTransitionFactsParams{OrgID: scope.OrgID().String(), OperationID: operation})
	if err != nil {
		return app.TransitionFacts{}, storeError(err)
	}
	return app.TransitionFacts{ExistingOperation: row.ExistingOperation, HeldOperation: row.HeldOperation}, nil
}

// InsertTransitionTx writes the lifecycle row of an accepted command; its
// insert moves the trip's state (migration 000233).
func (s *Store) InsertTransitionTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.TransitionCommand, next trip.Trip, employee string) error {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	facts := next.Lifecycle()
	if err != nil || len(facts) == 0 || source.MembershipID != v.AuthorMembershipID || !canonicalID(employee) || next.Reference().ID() != v.TripID {
		return app.ErrUnavailable
	}
	last := facts[len(facts)-1]
	if last.Command() != v.Command || last.Revision() != next.Revision() || last.Revision() != v.Expected+1 || last.To() != next.State() {
		return app.ErrUnavailable
	}
	at, sub := instant(v.Declared.OccurredAt)
	if err := queries.InsertTripTransition(ctx, q.InsertTripTransitionParams{OrgID: scope.OrgID().String(), TripID: v.TripID, Revision: last.Revision(),
		Command: string(v.Command), FromState: string(last.From()), ToState: string(last.To()), Reason: text(v.Reason), OperationID: v.OperationID,
		PayloadDigest: v.PayloadDigest, AuthorMembershipID: source.MembershipID, AuthorEmployeeID: employee, EnrollmentID: source.EnrollmentID,
		OccurredAt: at, OccurredSubmicro: sub, EntryZone: v.Declared.EntryZone, DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds}); err != nil {
		return storeError(err)
	}
	return nil
}

func heldTransitionCode(code string) bool {
	return code == app.TripAlreadyClosed || code == app.StaleRevision
}

// heldTransitionProof compares a stored held command with the operation's parsed command.
func heldTransitionProof(source syncapp.Source, op protocol.Operation, row q.ReadTripTransitionHeldProofRow, v app.TransitionInput) bool {
	e := op.Snapshot()
	at, ok := exact(row.OccurredAt, row.OccurredSubmicro)
	return ok && heldBinding(source, op) && row.ProofVersion == 1 && canonicalID(row.ID) && canonicalID(row.AuthorEmployeeID) &&
		row.OperationType == e.Type && row.OperationSchema == e.OperationSchema && row.PayloadDigest == op.PayloadDigest() &&
		row.OwnerAccountID == source.Actor.AccountID && row.ApiOrigin == source.Origin && row.MembershipID == source.MembershipID &&
		row.EnrollmentID == source.EnrollmentID && row.EntityType == app.EntityType && row.EntityID == v.TripID && row.TripID == v.TripID &&
		row.Command == string(v.Command) && row.ExpectedRevision == v.Expected && sameText(optionalText(row.Reason), v.Reason) && heldTransitionCode(row.Code) &&
		at.Equal(v.Declared.OccurredAt) && row.EntryZone == v.Declared.EntryZone && row.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds
}

func parsedTransition(op protocol.Operation) (app.TransitionInput, error) {
	command, err := app.ParseTransition(op)
	if err != nil {
		return app.TransitionInput{}, app.ErrUnavailable
	}
	return command.Snapshot()
}

// HoldTransitionTx keeps a command on a closed trip or naming a stale
// revision as one held fact (B-BR-EMP-391).
func (s *Store) HoldTransitionTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, command app.TransitionCommand, code string) (string, error) {
	queries, _, err := s.port(ctx, db, source.Scope)
	v, errCommand := command.Snapshot()
	if err != nil || errCommand != nil || s.NewID == nil || !heldTransitionCode(code) || v.OperationID != op.ID() || !heldBinding(source, op) {
		return "", app.ErrUnavailable
	}
	if _, ok := app.LifecycleCommandOf(op.Snapshot().Type); !ok {
		return "", app.ErrUnavailable
	}
	org := source.Scope.OrgID().String()
	row, err := queries.ReadTripTransitionHeldProof(ctx, q.ReadTripTransitionHeldProofParams{OrgID: org, OperationID: op.ID()})
	if err == nil {
		if !heldTransitionProof(source, op, row, v) || row.Code != code {
			return "", app.ErrUnavailable
		}
		return row.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrUnavailable
	}
	review, err := s.NewID()
	if err != nil || !canonicalID(review) {
		return "", app.ErrUnavailable
	}
	if n, err := queries.InsertTripReviewItem(ctx, q.InsertTripReviewItemParams{OrgID: org, ID: review, OperationID: op.ID(), Code: code}); err != nil || n != 1 {
		return "", app.ErrUnavailable
	}
	at, sub := instant(v.Declared.OccurredAt)
	if err := queries.InsertTripHeldTransition(ctx, q.InsertTripHeldTransitionParams{OrgID: org, ReviewItemID: review, OperationID: op.ID(), TripID: v.TripID,
		Command: string(v.Command), ExpectedRevision: v.Expected, Reason: text(v.Reason), OccurredAt: at, OccurredSubmicro: sub,
		EntryZone: v.Declared.EntryZone, DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds}); err != nil {
		return "", storeError(err)
	}
	return review, nil
}

func heldTransitionReceipt(op protocol.Operation, row q.ReadTripTransitionHeldProofRow) error {
	return heldReceipt(op, q.ReadTripHeldProofRow{ID: row.ID, Code: row.Code, AcknowledgementResult: row.AcknowledgementResult, AcknowledgementCode: row.AcknowledgementCode,
		AcknowledgementReviewItemID: row.AcknowledgementReviewItemID, AcknowledgementProofVersion: row.AcknowledgementProofVersion, ReceivedAt: row.ReceivedAt, CanonicalAck: row.CanonicalAck})
}

// lifecycleOperation reports whether the operation is one of the three owners.
func lifecycleOperation(op protocol.Operation) bool {
	_, ok := app.LifecycleCommandOf(op.Snapshot().Type)
	return ok
}

// CheckHeldTx proves a held outcome: of a lifecycle command against its held
// row and, once written, its exact receipt, or of a start or finish through
// the EMP-TRIP-002 proof (OFFL-008 ReviewProof).
func (s *Store) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	if !lifecycleOperation(op) {
		return s.checkHeldStartOrFinish(ctx, db, source, op, outcome)
	}
	queries, _, err := s.port(ctx, db, source.Scope)
	if err != nil || !heldBinding(source, op) {
		return app.ErrUnavailable
	}
	v, err := parsedTransition(op)
	if err != nil {
		return app.ErrUnavailable
	}
	row, err := queries.ReadTripTransitionHeldProof(ctx, q.ReadTripTransitionHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if err != nil || !heldTransitionProof(source, op, row, v) || outcome.Result != protocol.AcceptedForReview || outcome.Code != row.Code || outcome.ReviewItemID != row.ID || outcome.EntityRevision != nil {
		return app.ErrUnavailable
	}
	if len(row.CanonicalAck) != 0 {
		return heldTransitionReceipt(op, row)
	}
	if row.AcknowledgementResult != "" || row.AcknowledgementProofVersion != 0 || row.ReceivedAt.Valid {
		return app.ErrUnavailable
	}
	return nil
}

// CheckOwnReceiptTx lets the author re-read its own held receipt without the
// record read permission (OFFL-008), for every trip owner of this store.
func (s *Store) CheckOwnReceiptTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (bool, error) {
	if !lifecycleOperation(op) {
		return s.checkOwnStartOrFinish(ctx, db, source, op)
	}
	db = syncapp.Borrow(db)
	if s == nil || db == nil || source.Validate() != nil || database.ValidateOrgDBTX(ctx, db, source.Scope) != nil {
		return false, app.ErrUnavailable
	}
	row, err := q.New(db).ReadTripTransitionHeldProof(ctx, q.ReadTripTransitionHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, app.ErrUnavailable
	}
	v, err := parsedTransition(op)
	if err != nil {
		return false, app.ErrUnavailable
	}
	if !heldTransitionProof(source, op, row, v) {
		return false, app.ErrNotFound
	}
	if err := heldTransitionReceipt(op, row); err != nil {
		return false, err
	}
	return true, nil
}
