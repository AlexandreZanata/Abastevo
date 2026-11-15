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

// Descriptor: a stale or duplicate submission is held STALE_REVISION
// (EMP-TRIP-015, EDR-235), so ACCEPTED_FOR_REVIEW is final too.
func (*submitHandler) Descriptor() syncapp.Descriptor {
	return syncapp.Descriptor{Type: app.SubmitOperationType, Schema: 1, WritePolicy: app.Policy(app.RecordAction), ReadPolicy: app.Policy(app.ReadAction),
		TerminalResults: []string{protocol.Accepted, protocol.Rejected, protocol.AcceptedForReview}}
}

// ReviewProof is the held-submission proof (OFFL-008).
func (*submitHandler) ReviewProof() syncapp.ReviewProof { return SubmitProof{} }

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
	// Only a completed trip is submitted; its finish is the dependency. A
	// cancelled trip is never submitted (EMP-TRIP-011): Apply refuses it.
	if record.State != trip.Completed && record.State != trip.Cancelled {
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
	// B-BR-EMP-392: a cancelled trip has no field revision to submit.
	if record.State == trip.Cancelled {
		return malformed(), nil
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
	revisions, err := app.TripRevisions(entries)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	// A named revision that is no longer the current draft or returned field
	// revision is a late or duplicate submission: one held fact that changes
	// nothing (B-BR-EMP-364, EDR-235).
	kind, err := app.SubmitKind(state, revisions, v.Revision)
	if errors.Is(err, app.ErrSubmitStale) {
		review, err := h.reviews.HoldSubmitTx(ctx, syncapp.Borrow(db), source, op, command, app.StaleRevision)
		return held(review, app.StaleRevision, err)
	}
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	// Changed rights store nothing until their owner exists (EMP-TRIP-007).
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
	entry, err := app.SubmitEntry(revisions, kind, v.Revision, submitter, author)
	if errors.Is(err, review.ErrForbidden) {
		return syncapp.Outcome{}, syncapp.SourceDenied(orgapi.Forbidden)
	}
	if err != nil || entry.State != review.Submitted {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.reviews.InsertSubmitTx(ctx, syncapp.Borrow(db), source.Scope, source, command, entry, int64(len(entries)+1), record.AuthorEmployeeID); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if disposition, err := h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt); err != nil || disposition != app.Apply {
		if err == nil {
			err = syncapp.ErrUnavailable
		}
		return syncapp.Outcome{}, err
	}
	revision := entry.Revision
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

// storedSubmit proves the stored SUBMIT (first, at the named revision) or
// RESUBMIT (at the named revision + 1) entry is exactly this operation's,
// and returns the revision it produced.
func storedSubmit(source syncapp.Source, v app.SubmitInput, record app.Record, entries []app.ReviewEntry) (int64, bool) {
	for _, e := range entries {
		if e.OperationID == v.OperationID {
			shape := e.Kind == review.Submit && e.Sequence == 1 && e.Revision == v.Revision || e.Kind == review.Resubmit && e.Sequence > 1 && e.Revision == v.Revision+1
			return e.Revision, shape && e.State == review.Submitted && e.Reason == "" &&
				e.ActorMembershipID == source.MembershipID && e.ActorMembershipID == record.AuthorMembershipID && e.ActorEmployeeID == record.AuthorEmployeeID
		}
	}
	return 0, false
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
	if errors.Is(err, syncapp.ErrDependencyPending) && outcome.Result != protocol.Accepted && outcome.Result != protocol.AcceptedForReview {
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
	if outcome.Result == protocol.AcceptedForReview {
		if outcome.Code != app.StaleRevision || outcome.EntityRevision != nil {
			return syncapp.ErrUnavailable
		}
		if err := (SubmitProof{}).CheckHeldTx(ctx, syncapp.Borrow(db), source, op, outcome.Copy()); err != nil {
			if errors.Is(err, syncapp.ErrSourceDenied) {
				return err
			}
			return syncapp.SourceDenied(err)
		}
		return nil
	}
	if outcome.Result != protocol.Accepted {
		return nil
	}
	if outcome.EntityRevision == nil || outcome.Code != "" || len(outcome.Warnings) != 0 || record.State != trip.Completed {
		return syncapp.ErrUnavailable
	}
	entries, err := h.reviews.ReviewEntriesTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID)
	if err != nil {
		return syncapp.ErrUnavailable
	}
	if revision, ok := storedSubmit(source, v, record, entries); !ok || revision != *outcome.EntityRevision {
		return syncapp.ErrUnavailable
	}
	return nil
}
