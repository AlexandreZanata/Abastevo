package adapters

import (
	"context"
	"errors"

	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// submitHandler owns trip.submit (EMP-TRIP-004, B-BR-EMP-342/343, EDR-220).
// It shares the trip owners' authority, readings port and store, and appends
// the SUBMIT entry through the review store port.
type submitHandler struct {
	ports
	reviews app.SubmitStore
}

// NewSubmitHandler composes the trip.submit owner from real ports only.
func NewSubmitHandler(authority app.Authority, readings app.Readings, store app.Store, reviews app.SubmitStore) (syncapp.Handler, error) {
	p := ports{authority: authority, readings: readings, store: store}
	if !p.ready() || absent(reviews) {
		return nil, syncapp.ErrUnavailable
	}
	return &submitHandler{ports: p, reviews: reviews}, nil
}

// Descriptor: no held path, so only ACCEPTED and REJECTED are final.
func (*submitHandler) Descriptor() syncapp.Descriptor {
	return syncapp.Descriptor{Type: app.SubmitOperationType, Schema: 1, WritePolicy: app.Policy(app.RecordAction), ReadPolicy: app.Policy(app.ReadAction),
		TerminalResults: []string{protocol.Accepted, protocol.Rejected}}
}

func (h *submitHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.SubmitCommand, app.SubmitInput, error) {
	if h == nil || !h.ready() || absent(h.reviews) {
		return app.SubmitCommand{}, app.SubmitInput{}, syncapp.ErrUnavailable
	}
	if err := bound(ctx, db, source, op, mode, app.SubmitOperationType); err != nil {
		return app.SubmitCommand{}, app.SubmitInput{}, err
	}
	command, err := app.ParseSubmit(op)
	if err != nil {
		return app.SubmitCommand{}, app.SubmitInput{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.SubmitCommand{}, app.SubmitInput{}, syncapp.ErrUnavailable
	}
	return command, v, nil
}

// trip reads the submitted trip and proves the current authority over it.
// A trip not stored here admits only the current device and is a pending
// dependency that reveals nothing (as the finish, EDR-202); only its author
// submits it.
func (h *submitHandler) trip(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.SubmitInput, mode syncapp.Mode) (app.Record, error) {
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
	if mode == syncapp.Write && record.AuthorMembershipID != source.MembershipID {
		return app.Record{}, syncapp.SourceDenied(orgapi.Forbidden)
	}
	return record, nil
}

func (h *submitHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	_, v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.root(ctx, db, source, mode)
	case err != nil:
		return err
	}
	record, err := h.trip(ctx, db, source, v, mode)
	if mode == syncapp.ReadReceipt {
		if errors.Is(err, syncapp.ErrDependencyPending) {
			return nil
		}
		return err
	}
	if err != nil {
		return err
	}
	// Only a completed trip is submitted; its finish is the dependency.
	if record.State != trip.Completed {
		return syncapp.ErrDependencyPending
	}
	_, err = h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt)
	return err
}

func (h *submitHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	command, v, err := h.request(ctx, db, source, op, syncapp.Write)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, syncapp.Write); err != nil {
			return syncapp.Outcome{}, err
		}
		return malformed(), nil
	case err != nil:
		return syncapp.Outcome{}, err
	}
	record, err := h.trip(ctx, db, source, v, syncapp.Write)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	state, err := h.reviews.LockTripTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID)
	if err != nil || state != record.State {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	entries, err := h.reviews.ReviewEntriesTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	for _, e := range entries {
		if e.OperationID == v.OperationID {
			// A stored entry without its stored receipt is never re-acknowledged.
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
	}
	current, err := app.ReviewCase(entries)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	// A case that is no longer a draft at the named revision, and changed
	// rights, store nothing until their owner exists (EDR-220).
	if app.SubmitGate(state, current, v.Revision) != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	disposition, err := h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if disposition != app.Apply {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	employee, err := h.store.AuthorEmployeeTx(ctx, syncapp.Borrow(db), source.Scope, source.MembershipID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	author := app.Person{MembershipID: record.AuthorMembershipID, Employees: []string{record.AuthorEmployeeID}}
	submitter := app.Person{MembershipID: source.MembershipID, Employees: []string{employee}}
	next, err := app.Submit(current, v.Revision, submitter, author)
	if errors.Is(err, review.ErrForbidden) {
		return syncapp.Outcome{}, syncapp.SourceDenied(orgapi.Forbidden)
	}
	if err != nil || next.State() != review.Submitted {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.reviews.InsertSubmitTx(ctx, syncapp.Borrow(db), source.Scope, source, command, record.AuthorEmployeeID); err != nil {
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

// storedSubmit proves the stored SUBMIT entry is exactly this operation's.
func storedSubmit(source syncapp.Source, v app.SubmitInput, record app.Record, entries []app.ReviewEntry) bool {
	for _, e := range entries {
		if e.OperationID == v.OperationID {
			return e.Sequence == 1 && e.Kind == review.Submit && e.State == review.Submitted && e.Revision == v.Revision && e.Reason == "" &&
				e.ActorMembershipID == source.MembershipID && e.ActorMembershipID == record.AuthorMembershipID && e.ActorEmployeeID == record.AuthorEmployeeID
		}
	}
	return false
}

func (h *submitHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	_, v, err := h.request(ctx, db, source, op, mode)
	if !confirmable(err, mode, outcome) {
		return syncapp.ErrUnavailable
	}
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.confirmMalformed(ctx, db, source, mode, outcome)
	case err != nil:
		return err
	}
	record, err := h.trip(ctx, db, source, v, mode)
	if errors.Is(err, syncapp.ErrDependencyPending) && outcome.Result != protocol.Accepted {
		return nil
	}
	if err != nil {
		return err
	}
	if mode == syncapp.Write && outcome.Result == protocol.Accepted {
		disposition, err := h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt)
		if err != nil {
			return err
		}
		if disposition != app.Apply {
			return syncapp.ErrUnavailable
		}
	}
	if outcome.Result != protocol.Accepted {
		return nil
	}
	if outcome.EntityRevision == nil || *outcome.EntityRevision != v.Revision || outcome.Code != "" || len(outcome.Warnings) != 0 || record.State != trip.Completed {
		return syncapp.ErrUnavailable
	}
	entries, err := h.reviews.ReviewEntriesTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID)
	if err != nil || !storedSubmit(source, v, record, entries) {
		return syncapp.ErrUnavailable
	}
	return nil
}
