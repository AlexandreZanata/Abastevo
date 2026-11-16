package adapters

import (
	"context"
	"errors"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

type startHandler struct {
	ports
	availability app.StartAvailability
}
type finishHandler struct{ ports }

// NewStartHandler composes the trip.start owner from real ports only; the
// FLEET-007 start decision is consulted under the asset lock and the linked
// start consumes its mission's reservation (EMP-TRIP-023).
func NewStartHandler(authority app.Authority, readings app.Readings, availability app.StartAvailability, store app.Store) (syncapp.Handler, error) {
	p := ports{authority: authority, readings: readings, store: store}
	if !p.ready() || absent(availability) {
		return nil, syncapp.ErrUnavailable
	}
	return &startHandler{ports: p, availability: availability}, nil
}

// NewFinishHandler composes the trip.finish owner from real ports only.
func NewFinishHandler(authority app.Authority, readings app.Readings, store app.Store) (syncapp.Handler, error) {
	p := ports{authority: authority, readings: readings, store: store}
	if !p.ready() {
		return nil, syncapp.ErrUnavailable
	}
	return &finishHandler{p}, nil
}

func descriptor(kind string) syncapp.Descriptor {
	return syncapp.Descriptor{Type: kind, Schema: 1, WritePolicy: app.Policy(app.RecordAction), ReadPolicy: app.Policy(app.ReadAction),
		TerminalResults: []string{protocol.Accepted, protocol.AcceptedForReview, protocol.Rejected}}
}

func (*startHandler) Descriptor() syncapp.Descriptor  { return descriptor(app.StartOperationType) }
func (*finishHandler) Descriptor() syncapp.Descriptor { return descriptor(app.FinishOperationType) }

// ReviewProof is the held-fact proof of both owners (OFFL-008).
func (h *startHandler) ReviewProof() syncapp.ReviewProof  { return h.store }
func (h *finishHandler) ReviewProof() syncapp.ReviewProof { return h.store }

// ---- trip.start (B-BR-EMP-312/313) ----

func (h *startHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.StartCommand, app.StartInput, error) {
	if h == nil || !h.ready() || absent(h.availability) {
		return app.StartCommand{}, app.StartInput{}, syncapp.ErrUnavailable
	}
	if err := bound(ctx, db, source, op, mode, app.StartOperationType); err != nil {
		return app.StartCommand{}, app.StartInput{}, err
	}
	command, err := app.ParseStart(op)
	if err != nil {
		return app.StartCommand{}, app.StartInput{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.StartCommand{}, app.StartInput{}, syncapp.ErrUnavailable
	}
	return command, v, nil
}

// readTarget resolves the record facts of a receipt read: the stored trip of
// this operation, or the would-be own trip of a refused or held one.
func (h *startHandler) readTarget(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.StartInput) (app.Target, *app.Record, error) {
	record, err := h.store.ReadTripTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID)
	switch {
	case err == nil && record.OperationID == v.OperationID:
		return app.Target{TripID: v.TripID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID, AuthorMembershipID: record.AuthorMembershipID}, &record, nil
	case err == nil, errors.Is(err, app.ErrNotFound):
		return app.Target{TripID: v.TripID, AssetID: v.AssetID}, nil, nil
	}
	return app.Target{}, nil, syncapp.ErrUnavailable
}

func (h *startHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	_, v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.root(ctx, db, source, mode)
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.ErrUnavailable
	case err != nil:
		return err
	}
	if mode == syncapp.ReadReceipt {
		target, _, err := h.readTarget(ctx, db, source, v)
		if err != nil {
			return err
		}
		if err := h.authority.CheckTripTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.ReadAction); err != nil {
			return denied(err)
		}
		return nil
	}
	_, _, err = h.meterTarget(ctx, db, source, v.AssetID, v.DriverID, v.Event)
	return err
}

// mission answers the mission an admissible start executes (EMP-TRIP-023,
// B-BR-EMP-412): the referenced mission locked under the root and asset
// locks and eligible for this author, driver and asset, else "".
func (h *startHandler) mission(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.StartInput) (string, error) {
	if v.MissionID == "" {
		return "", nil
	}
	m, err := h.store.MissionForStartTx(ctx, syncapp.Borrow(db), source.Scope, v.MissionID)
	switch {
	case errors.Is(err, app.ErrNotFound):
		return "", nil
	case err != nil:
		return "", syncapp.ErrUnavailable
	case !app.MissionEligible(m, source.MembershipID, v.DriverID, v.AssetID):
		return "", nil
	}
	return v.MissionID, nil
}

// startHold decides the held condition of an admissible start under the
// root and asset locks; "" applies it (B-BR-EMP-313). The FLEET-007 start
// decision (block, reservation for someone else, lifecycle, maintenance,
// missing assignment, a trip holding the declared instant) follows the
// lifecycle and history checks; a trip in progress now is always busy.
func startHold(disposition app.Disposition, facts app.StartFacts, decision fleet.StartDecision, v app.StartInput) string {
	switch {
	case disposition == app.HoldPermissionChanged:
		return app.PermissionChanged
	case facts.AssetState != string(fleet.Active) || facts.DisposedAt != nil && !v.Declared.OccurredAt.Before(*facts.DisposedAt):
		return app.AssetUnavailable
	case disposition == app.HoldDisposalEnded:
		return app.PermissionChanged
	case !decision.Admitted && decision.Code == fleet.AssetBusy:
		return app.AssetBusy
	case !decision.Admitted:
		return app.AssetUnavailable
	case facts.ActiveTripID != "":
		return app.AssetBusy
	}
	return ""
}

func (h *startHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
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
	disposition, view, err := h.meterTarget(ctx, db, source, v.AssetID, v.DriverID, v.Event)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if _, err := h.lock(ctx, db, source.Scope, v.AssetID); err != nil {
		return syncapp.Outcome{}, err
	}
	facts, err := h.store.PrepareStartTx(ctx, syncapp.Borrow(db), source.Scope, source, command)
	if errors.Is(err, app.ErrNotFound) {
		return syncapp.Outcome{}, denied(err)
	}
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if facts.ExistingOperation == v.OperationID || facts.HeldOperation == v.OperationID {
		// A stored fact without its stored receipt is never re-acknowledged.
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	// EMP-FLEET-011 AC4 (EDR-219): an IMPLEMENT is never a trip's main asset.
	if facts.AssetState == "DRAFT" || fleet.MainAssetAllowed(fleet.Kind(facts.AssetKind)) != nil || !facts.DriverActive || facts.ExistingOperation != "" || facts.HeldOperation != "" || !readingFits(v.Event, view) {
		return malformed(), nil
	}
	mission, err := h.mission(ctx, db, source, v)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	decision, err := h.availability.StartDecisionDBTX(ctx, syncapp.Borrow(db), source.Scope, v.AssetID, source.MembershipID, mission, v.Declared.OccurredAt)
	if err != nil {
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return syncapp.Outcome{}, err
		}
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	code := startHold(disposition, facts, decision, v)
	// EMP-TRIP-023 (B-BR-EMP-412): an otherwise accepted start naming a
	// missing, foreign or ineligible mission is held with the link removed.
	if code == "" && v.MissionID != "" && mission == "" {
		code = app.OrphanReference
	}
	if code != "" {
		review, err := h.store.HoldStartTx(ctx, syncapp.Borrow(db), source, op, command, code)
		return held(review, code, err)
	}
	started, err := app.StartDomain(op.Snapshot(), v, facts.Zones)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.record(ctx, db, source, v.AssetID, v.Event, view, meter.TripStart); err != nil {
		return syncapp.Outcome{}, err
	}
	if err := h.store.InsertTripTx(ctx, syncapp.Borrow(db), source.Scope, source, command, facts, started); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if disposition, err := h.classify(ctx, db, source, v.AssetID, v.DriverID, v.Declared.OccurredAt); err != nil || disposition != app.Apply {
		if err == nil {
			err = syncapp.ErrUnavailable
		}
		return syncapp.Outcome{}, err
	}
	if mission != "" {
		// EMP-TRIP-023 (B-BR-EMP-413/414): the link, the mission's START and
		// the consumption of its qualifying reservation, in this transaction.
		if err := h.store.LinkMissionTx(ctx, syncapp.Borrow(db), source, command); err != nil {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
		if _, err := h.availability.FulfilMissionReservationDBTX(ctx, syncapp.Borrow(db), source.Scope, mission, v.TripID, v.AssetID, v.Declared.OccurredAt); err != nil {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
	}
	revision := int64(1)
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

// storedStart proves the stored trip is exactly this operation's fact.
func storedStart(source syncapp.Source, v app.StartInput, record app.Record) bool {
	return record.ID == v.TripID && record.OperationID == v.OperationID && record.PayloadDigest == v.PayloadDigest && record.AssetID == v.AssetID &&
		record.DriverID == v.DriverID && record.AuthorMembershipID == source.MembershipID && record.EnrollmentID == source.EnrollmentID &&
		record.Declared.OccurredAt.Equal(v.Declared.OccurredAt) && record.Declared.EntryZone == v.Declared.EntryZone &&
		record.Declared.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds && sameClock(record.Clock, v.Clock) && sameText(record.MissingReason, v.MissingReason) &&
		sameReading(v.Reading, record.MeterID, record.SegmentID, record.ReadingID) && !record.ReceivedAt.IsZero() && record.Reporting.Day != ""
}

func (h *startHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
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
	if mode == syncapp.Write {
		disposition, _, err := h.meterTarget(ctx, db, source, v.AssetID, v.DriverID, v.Event)
		if err != nil || outcome.Result != protocol.Accepted {
			if err == nil && outcome.Result == protocol.AcceptedForReview {
				return h.checkHeld(ctx, db, source, op, outcome, app.StartHeldCode(outcome.Code))
			}
			return err
		}
		if disposition != app.Apply {
			return syncapp.ErrUnavailable
		}
	}
	target, record, err := h.readTarget(ctx, db, source, v)
	if err != nil {
		return err
	}
	if mode == syncapp.ReadReceipt {
		if err := h.authority.CheckTripTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.ReadAction); err != nil {
			return denied(err)
		}
		if outcome.Result == protocol.AcceptedForReview {
			return h.checkHeld(ctx, db, source, op, outcome, app.StartHeldCode(outcome.Code))
		}
	}
	if outcome.Result != protocol.Accepted {
		return nil
	}
	if outcome.EntityRevision == nil || *outcome.EntityRevision != 1 || outcome.Code != "" || len(outcome.Warnings) != 0 || record == nil || !storedStart(source, v, *record) {
		return syncapp.ErrUnavailable
	}
	return nil
}

// ---- trip.finish (B-BR-EMP-315) ----

func (h *finishHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.FinishCommand, app.FinishInput, error) {
	if h == nil || !h.ready() {
		return app.FinishCommand{}, app.FinishInput{}, syncapp.ErrUnavailable
	}
	if err := bound(ctx, db, source, op, mode, app.FinishOperationType); err != nil {
		return app.FinishCommand{}, app.FinishInput{}, err
	}
	command, err := app.ParseFinish(op)
	if err != nil {
		return app.FinishCommand{}, app.FinishInput{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.FinishCommand{}, app.FinishInput{}, syncapp.ErrUnavailable
	}
	return command, v, nil
}

// trip reads the finished trip; a trip not stored here (missing, held or
// another organization's) admits only the current device and is a pending
// dependency that reveals nothing (EDR-202).
func (h *finishHandler) trip(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.FinishInput, mode syncapp.Mode) (app.Record, error) {
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
	// EMP-TRIP-012: the member currently responsible for the trip.
	if err := h.authority.CheckTripTx(ctx, syncapp.Borrow(db), source.Scope, source, tripTarget(record, source, mode, nil), action(mode)); err != nil {
		return app.Record{}, denied(err)
	}
	return record, nil
}

func (h *finishHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
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
	_, _, err = h.meterTarget(ctx, db, source, record.AssetID, record.Driver(), v.Event)
	return err
}

// finishRefusal maps an EMP-TRIP-001 refusal at receipt (EDR-202).
func finishRefusal(err error) (syncapp.Outcome, error) {
	switch {
	case errors.Is(err, trip.ErrForbidden):
		return syncapp.Outcome{}, syncapp.SourceDenied(orgapi.Forbidden)
	case errors.Is(err, trip.ErrFinishBeforeStart), errors.Is(err, trip.ErrInvalid), errors.Is(err, trip.ErrScope), errors.Is(err, meter.ErrInvalid), errors.Is(err, meter.ErrScope):
		return malformed(), nil
	}
	return syncapp.Outcome{}, syncapp.ErrUnavailable
}

func (h *finishHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
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
	disposition, view, err := h.meterTarget(ctx, db, source, record.AssetID, record.Driver(), v.Event)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if _, err := h.lock(ctx, db, source.Scope, record.AssetID); err != nil {
		return syncapp.Outcome{}, err
	}
	// Re-read under the locks: a concurrent finish may have completed it.
	if record, err = h.store.ReadTripTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if record.Finish != nil && record.Finish.OperationID == v.OperationID {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	finished, err := app.FinishDomain(source.Scope, record, v)
	if err != nil {
		return finishRefusal(err)
	}
	if !readingFits(v.Event, view) {
		return malformed(), nil
	}
	// EMP-TRIP-011: a cancelled trip is closed like a completed one; so is a
	// trip a supervisor closed (EMP-TRIP-014).
	if record.Finish != nil || record.State == trip.Cancelled || record.Closure != nil {
		review, err := h.store.HoldFinishTx(ctx, syncapp.Borrow(db), source, op, command, app.TripAlreadyClosed)
		return held(review, app.TripAlreadyClosed, err)
	}
	// Changed rights on a finish store nothing until a held owner exists.
	if disposition != app.Apply {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.record(ctx, db, source, record.AssetID, v.Event, view, meter.TripEnd); err != nil {
		return syncapp.Outcome{}, err
	}
	if err := h.store.InsertFinishTx(ctx, syncapp.Borrow(db), source.Scope, source, command, finished.Crossing().EndDay()); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if disposition, err := h.classify(ctx, db, source, record.AssetID, record.Driver(), v.Declared.OccurredAt); err != nil || disposition != app.Apply {
		if err == nil {
			err = syncapp.ErrUnavailable
		}
		return syncapp.Outcome{}, err
	}
	// EMP-TRIP-011: the finish is acknowledged at the trip's next revision (2
	// without lifecycle commands).
	revision := finished.Revision()
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

// storedFinish proves the stored finish is exactly this operation's fact.
func storedFinish(source syncapp.Source, v app.FinishInput, record app.Record) bool {
	f := record.Finish
	return f != nil && record.State == trip.Completed && f.OperationID == v.OperationID && f.PayloadDigest == v.PayloadDigest && f.AuthorMembershipID == source.MembershipID &&
		f.EnrollmentID == source.EnrollmentID && f.Declared.OccurredAt.Equal(v.Declared.OccurredAt) && f.Declared.EntryZone == v.Declared.EntryZone &&
		f.Declared.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds && sameClock(f.Clock, v.Clock) && sameText(f.MissingReason, v.MissingReason) &&
		sameReading(v.Reading, f.MeterID, f.SegmentID, f.ReadingID) && !f.ReceivedAt.IsZero() && f.EndDay != ""
}

func (h *finishHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
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
		disposition, _, err := h.meterTarget(ctx, db, source, record.AssetID, record.Driver(), v.Event)
		if err != nil {
			return err
		}
		if outcome.Result == protocol.Accepted && disposition != app.Apply {
			return syncapp.ErrUnavailable
		}
	}
	switch outcome.Result {
	case protocol.AcceptedForReview:
		return h.checkHeld(ctx, db, source, op, outcome, outcome.Code == app.TripAlreadyClosed)
	case protocol.Accepted:
		if outcome.EntityRevision == nil || *outcome.EntityRevision != record.Revision() || outcome.Code != "" || len(outcome.Warnings) != 0 || !storedFinish(source, v, record) {
			return syncapp.ErrUnavailable
		}
	}
	return nil
}
