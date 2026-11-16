// Package adapters owns the signed tracking intake of EMP-GPS-004: the six
// gps.session.* owners, the gps.track.batch owner and the PostgreSQL store
// of sessions, transitions and segments.
package adapters

import (
	"context"
	"errors"
	"reflect"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func absent(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// core holds the real ports shared by every tracking owner.
type core struct {
	authority app.Authority
	trips     app.Trips
	store     app.Store
}

func (c core) ready() bool { return !absent(c.authority) && !absent(c.trips) && !absent(c.store) }

type sessionHandler struct {
	core
	kind string
}

type batchHandler struct{ core }

// NewSessionHandlers composes the six gps.session.* owners from real ports
// only: the trip intake authority, the stored trips and the tracking store.
// They have no held form, review proof or late holder (EDR-278).
func NewSessionHandlers(authority app.Authority, trips app.Trips, store app.Store) ([]syncapp.Handler, error) {
	c := core{authority: authority, trips: trips, store: store}
	if !c.ready() {
		return nil, syncapp.ErrUnavailable
	}
	handlers := make([]syncapp.Handler, 0, 6)
	for _, kind := range app.SessionOperationTypes() {
		handlers = append(handlers, &sessionHandler{core: c, kind: kind})
	}
	return handlers, nil
}

// NewBatchHandler composes the gps.track.batch owner from real ports only.
func NewBatchHandler(authority app.Authority, trips app.Trips, store app.Store) (syncapp.Handler, error) {
	c := core{authority: authority, trips: trips, store: store}
	if !c.ready() {
		return nil, syncapp.ErrUnavailable
	}
	return &batchHandler{c}, nil
}

func descriptor(kind string) syncapp.Descriptor {
	return syncapp.Descriptor{Type: kind, Schema: 1, WritePolicy: tripapp.Policy(tripapp.RecordAction), ReadPolicy: tripapp.Policy(tripapp.ReadAction),
		TerminalResults: []string{protocol.Accepted, protocol.Rejected}}
}

func (h *sessionHandler) Descriptor() syncapp.Descriptor { return descriptor(h.kind) }
func (*batchHandler) Descriptor() syncapp.Descriptor     { return descriptor(app.BatchOperationType) }

// denied maps a current-source answer to a bounded transport refusal; an
// unknown cause stays an unavailable source denial.
func denied(err error) error {
	if errors.Is(err, syncapp.ErrSourceDenied) {
		return err
	}
	if code, ok := orgapi.Registered(err); ok {
		return syncapp.SourceDenied(code)
	}
	for _, pair := range []struct{ source, target error }{
		{permission.ErrPermissionDenied, orgapi.Forbidden},
		{permission.ErrResourceNotFound, orgapi.NotFound},
		{fleetapp.ErrNotFound, orgapi.NotFound},
		{fleetapp.ErrUnauthenticated, orgapi.Unauthenticated},
		{access.ErrEntitlementRequired, orgapi.EntitlementRequired},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired},
		{access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete},
	} {
		if errors.Is(err, pair.source) {
			return syncapp.SourceDenied(pair.target)
		}
	}
	return syncapp.SourceDenied(err)
}

// bound proves the envelope is this source's own operation of this type.
func bound(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, kind string) error {
	if ctx == nil || syncapp.Borrow(db) == nil || (mode != syncapp.Write && mode != syncapp.ReadReceipt) {
		return syncapp.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	v := op.Snapshot()
	if source.Validate() != nil || v.Type != kind || v.OwnerAccountID != source.Actor.AccountID || v.OrgID != source.Scope.OrgID().String() || v.APIOrigin != source.Origin || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return syncapp.SourceDenied(orgapi.NotFound)
	}
	return nil
}

func action(mode syncapp.Mode) tripapp.Action {
	if mode == syncapp.ReadReceipt {
		return tripapp.ReadAction
	}
	return tripapp.RecordAction
}

func malformed() syncapp.Outcome {
	return syncapp.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}
}

func target(record tripapp.Record) tripapp.Target {
	return tripapp.Target{TripID: record.ID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID, AuthorMembershipID: record.AuthorMembershipID}
}

func (c core) check(ctx context.Context, db syncapp.DBTX, source syncapp.Source, t tripapp.Target, a tripapp.Action) error {
	if err := c.authority.CheckTripTx(ctx, syncapp.Borrow(db), source.Scope, source, t, a); err != nil {
		return denied(err)
	}
	return nil
}

// root is the malformed-admission selector: current ORG authority only.
func (c core) root(ctx context.Context, db syncapp.DBTX, source syncapp.Source, mode syncapp.Mode) error {
	return c.check(ctx, db, source, tripapp.Target{}, action(mode))
}

// trip reads the named trip and proves current authority over it; a trip not
// stored here (another organization's included) admits only the current
// device and is a pending dependency that reveals nothing (EDR-202/278).
func (c core) trip(ctx context.Context, db syncapp.DBTX, source syncapp.Source, id string, mode syncapp.Mode) (tripapp.Record, error) {
	record, err := c.trips.ReadTripTx(ctx, syncapp.Borrow(db), source.Scope, id)
	if errors.Is(err, tripapp.ErrNotFound) {
		if err := c.check(ctx, db, source, tripapp.Target{TripID: id}, action(mode)); err != nil {
			return tripapp.Record{}, err
		}
		return tripapp.Record{}, syncapp.ErrDependencyPending
	}
	if err != nil {
		return tripapp.Record{}, syncapp.ErrUnavailable
	}
	if err := c.check(ctx, db, source, target(record), action(mode)); err != nil {
		return tripapp.Record{}, err
	}
	return record, nil
}

// locked takes the trip row lock and re-reads the trip under it.
func (c core) locked(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, id string) (tripapp.Record, error) {
	if err := c.store.LockTripTx(ctx, syncapp.Borrow(db), scope, id); err != nil {
		return tripapp.Record{}, syncapp.ErrUnavailable
	}
	record, err := c.trips.ReadTripTx(ctx, syncapp.Borrow(db), scope, id)
	if err != nil {
		return tripapp.Record{}, syncapp.ErrUnavailable
	}
	return record, nil
}

// session reads a stored session; a session not stored here is a pending
// dependency (BR-GPS-022).
func (c core) session(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, id string) (app.SessionRecord, error) {
	record, err := c.store.SessionTx(ctx, syncapp.Borrow(db), scope, id)
	if errors.Is(err, app.ErrNotFound) {
		return app.SessionRecord{}, syncapp.ErrDependencyPending
	}
	if err != nil {
		return app.SessionRecord{}, syncapp.ErrUnavailable
	}
	return record, nil
}

func confirmMalformed(outcome syncapp.Outcome) error {
	if outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" && outcome.Code != "OPERATION_ID_REUSED" || outcome.ReviewItemID != "" || outcome.EntityRevision != nil {
		return syncapp.ErrUnavailable
	}
	return nil
}

// ---- gps.session.* (B-BR-EMP-428..430) ----

func (h *sessionHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.SessionInput, error) {
	if h == nil || !h.ready() {
		return app.SessionInput{}, syncapp.ErrUnavailable
	}
	if err := bound(ctx, db, source, op, mode, h.kind); err != nil {
		return app.SessionInput{}, err
	}
	command, err := app.ParseSession(op)
	if err != nil {
		return app.SessionInput{}, err
	}
	v, err := command.Snapshot()
	if err != nil || v.Type != h.kind {
		return app.SessionInput{}, syncapp.ErrUnavailable
	}
	return v, nil
}

func (h *sessionHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.root(ctx, db, source, mode)
	case err != nil:
		return err
	}
	_, err = h.trip(ctx, db, source, v.TripID, mode)
	if mode == syncapp.ReadReceipt {
		if errors.Is(err, syncapp.ErrDependencyPending) {
			return nil
		}
		return err
	}
	if err != nil || h.kind == app.StartOperationType {
		return err
	}
	_, err = h.session(ctx, db, source.Scope, v.SessionID)
	return err
}

// previous resolves the stored session an operation acts on: for a start,
// the trip's session (if any) and a refusal of a session identity used by
// another trip; otherwise the named session of this trip.
func (h *sessionHandler) previous(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, v app.SessionInput) (*app.SessionRecord, error) {
	if h.kind != app.StartOperationType {
		record, err := h.session(ctx, db, scope, v.SessionID)
		if err != nil {
			return nil, err
		}
		if record.TripID != v.TripID {
			return nil, app.ErrMalformed
		}
		return &record, nil
	}
	record, err := h.store.SessionOfTripTx(ctx, syncapp.Borrow(db), scope, v.TripID)
	switch {
	case err == nil && record.ID == v.SessionID:
		return &record, nil
	case err != nil && !errors.Is(err, app.ErrNotFound):
		return nil, syncapp.ErrUnavailable
	}
	if _, other := h.store.SessionTx(ctx, syncapp.Borrow(db), scope, v.SessionID); other == nil {
		return nil, app.ErrMalformed
	} else if !errors.Is(other, app.ErrNotFound) {
		return nil, syncapp.ErrUnavailable
	}
	if err == nil {
		return &record, nil
	}
	return nil, nil
}

func (h *sessionHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	v, err := h.request(ctx, db, source, op, syncapp.Write)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, syncapp.Write); err != nil {
			return syncapp.Outcome{}, err
		}
		return malformed(), nil
	case err != nil:
		return syncapp.Outcome{}, err
	}
	if _, err := h.trip(ctx, db, source, v.TripID, syncapp.Write); err != nil {
		return syncapp.Outcome{}, err
	}
	record, err := h.locked(ctx, db, source.Scope, v.TripID)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	facts, err := app.TripFactsAt(source.Scope, record, v.Declared)
	if errors.Is(err, app.ErrMalformed) {
		return malformed(), nil
	}
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	previous, err := h.previous(ctx, db, source.Scope, v)
	if errors.Is(err, app.ErrMalformed) {
		return malformed(), nil
	}
	if err != nil {
		return syncapp.Outcome{}, err
	}
	var existing *domain.Session
	if previous != nil {
		for _, row := range previous.Transitions {
			if row.OperationID == v.OperationID {
				// A stored fact without its stored receipt is never re-acknowledged.
				return syncapp.Outcome{}, syncapp.ErrUnavailable
			}
		}
		rebuilt, err := app.Rebuild(source.Scope, *previous)
		if err != nil {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
		// The session follows the trip's stored pauses, cancellation and
		// finish before this operation's instant (GPS-001 FOLLOW); those rows
		// are stored with this operation.
		if previous.ID == v.SessionID {
			if rebuilt, err = app.FollowTrip(source.Scope, record, rebuilt, v.Declared); err != nil {
				return syncapp.Outcome{}, syncapp.ErrUnavailable
			}
		}
		existing = &rebuilt
	}
	policy := app.PolicyVersion{}
	if h.kind == app.StartOperationType && (previous == nil || previous.ID != v.SessionID) {
		if policy, err = h.store.PolicyTx(ctx, syncapp.Borrow(db), source.Scope); err != nil {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
	}
	next, err := app.Decide(source.Scope, v, facts, existing, policy)
	if err != nil {
		switch refusal := app.Refusal(err); {
		case errors.Is(refusal, app.ErrPolicyOff):
			return syncapp.Outcome{Result: protocol.Rejected, Code: "FORBIDDEN"}, nil
		case errors.Is(refusal, domain.ErrForbidden):
			return syncapp.Outcome{}, syncapp.SourceDenied(orgapi.Forbidden)
		case errors.Is(refusal, app.ErrMalformed):
			return malformed(), nil
		}
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	change, err := app.Change(previous, next, v, policy)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.store.RecordSessionTx(ctx, syncapp.Borrow(db), source.Scope, source, change); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.check(ctx, db, source, target(record), tripapp.RecordAction); err != nil {
		return syncapp.Outcome{}, err
	}
	revision := int64(len(next.History()))
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

// storedSession proves the stored rows of an accepted operation.
func storedSession(source syncapp.Source, v app.SessionInput, record app.SessionRecord, revision int64) bool {
	last := 0
	for _, row := range record.Transitions {
		if row.OperationID == v.OperationID {
			last = row.Seq
		}
	}
	return record.ID == v.SessionID && record.TripID == v.TripID && record.OwnerMembershipID == source.MembershipID && last > 0 && int64(last) == revision
}

func (h *sessionHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, mode); err != nil {
			return err
		}
		return confirmMalformed(outcome)
	case err != nil:
		return err
	}
	_, err = h.trip(ctx, db, source, v.TripID, mode)
	if errors.Is(err, syncapp.ErrDependencyPending) && outcome.Result != protocol.Accepted {
		return nil
	}
	if err != nil {
		return err
	}
	switch outcome.Result {
	case protocol.Retry:
		if mode != syncapp.Write || outcome.Code != "DEPENDENCY_PENDING" {
			return syncapp.ErrUnavailable
		}
		return nil
	case protocol.Rejected:
		if outcome.Code != "MALFORMED" && outcome.Code != "FORBIDDEN" && outcome.Code != "OPERATION_ID_REUSED" || outcome.EntityRevision != nil {
			return syncapp.ErrUnavailable
		}
		return nil
	case protocol.Accepted:
		if outcome.EntityRevision == nil || outcome.Code != "" || len(outcome.Warnings) != 0 {
			return syncapp.ErrUnavailable
		}
		record, err := h.store.SessionTx(ctx, syncapp.Borrow(db), source.Scope, v.SessionID)
		if err != nil || !storedSession(source, v, record, *outcome.EntityRevision) {
			return syncapp.ErrUnavailable
		}
		return nil
	}
	return syncapp.ErrUnavailable
}

// ---- gps.track.batch (B-BR-EMP-431..434) ----

func (h *batchHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.BatchInput, error) {
	if h == nil || !h.ready() {
		return app.BatchInput{}, syncapp.ErrUnavailable
	}
	if err := bound(ctx, db, source, op, mode, app.BatchOperationType); err != nil {
		return app.BatchInput{}, err
	}
	command, err := app.ParseBatch(op)
	if err != nil {
		return app.BatchInput{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.BatchInput{}, syncapp.ErrUnavailable
	}
	return v, nil
}

func (h *batchHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.root(ctx, db, source, mode)
	case err != nil:
		return err
	}
	_, err = h.trip(ctx, db, source, v.TripID, mode)
	if mode == syncapp.ReadReceipt {
		if errors.Is(err, syncapp.ErrDependencyPending) {
			return nil
		}
		return err
	}
	if err != nil {
		return err
	}
	_, err = h.session(ctx, db, source.Scope, v.SessionID)
	return err
}

func (h *batchHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	v, err := h.request(ctx, db, source, op, syncapp.Write)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, syncapp.Write); err != nil {
			return syncapp.Outcome{}, err
		}
		return malformed(), nil
	case err != nil:
		return syncapp.Outcome{}, err
	}
	if _, err := h.trip(ctx, db, source, v.TripID, syncapp.Write); err != nil {
		return syncapp.Outcome{}, err
	}
	record, err := h.locked(ctx, db, source.Scope, v.TripID)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	stored, err := h.session(ctx, db, source.Scope, v.SessionID)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if stored.TripID != v.TripID {
		return malformed(), nil
	}
	if stored.OwnerMembershipID != source.MembershipID {
		return syncapp.Outcome{}, syncapp.SourceDenied(orgapi.Forbidden)
	}
	if _, err := h.store.SegmentTx(ctx, syncapp.Borrow(db), source.Scope, v.OperationID); !errors.Is(err, app.ErrNotFound) {
		// A stored fact without its stored receipt is never re-acknowledged.
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	taken, err := h.store.SequenceTakenTx(ctx, syncapp.Borrow(db), source.Scope, v.SessionID, v.Seq)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if taken {
		return malformed(), nil
	}
	session, err := app.Rebuild(source.Scope, stored)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	first, last := v.Fixes[0].At, v.Fixes[len(v.Fixes)-1].At
	// The recording periods follow the trip's stored pauses, cancellation and
	// finish (GPS-001 FOLLOW) up to the batch's last fix.
	if session, err = app.FollowTrip(source.Scope, record, session, last.Add(time.Microsecond)); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	instants, err := h.store.StoredInstantsTx(ctx, syncapp.Borrow(db), source.Scope, v.SessionID, first, last)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	segment, err := app.Admit(v, session, app.Bounds(record), instants)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	segment.AuthorMembershipID, segment.EnrollmentID = source.MembershipID, source.EnrollmentID
	if err := h.store.InsertSegmentTx(ctx, syncapp.Borrow(db), source.Scope, source, segment); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.check(ctx, db, source, target(record), tripapp.RecordAction); err != nil {
		return syncapp.Outcome{}, err
	}
	revision := v.Seq
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

// storedSegment proves the stored segment is exactly this batch's effect.
func storedSegment(source syncapp.Source, v app.BatchInput, s app.SegmentRecord, revision int64) bool {
	return s.OperationID == v.OperationID && s.PayloadDigest == v.PayloadDigest && s.SessionID == v.SessionID && s.TripID == v.TripID && s.Seq == v.Seq && revision == v.Seq &&
		s.AuthorMembershipID == source.MembershipID && s.EnrollmentID == source.EnrollmentID && s.Received == len(v.Fixes) &&
		s.Received == s.Stored+s.Duplicates+s.Outside+s.Mock
}

func (h *batchHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, mode); err != nil {
			return err
		}
		return confirmMalformed(outcome)
	case err != nil:
		return err
	}
	_, err = h.trip(ctx, db, source, v.TripID, mode)
	if errors.Is(err, syncapp.ErrDependencyPending) && outcome.Result != protocol.Accepted {
		return nil
	}
	if err != nil {
		return err
	}
	switch outcome.Result {
	case protocol.Retry:
		if mode != syncapp.Write || outcome.Code != "DEPENDENCY_PENDING" {
			return syncapp.ErrUnavailable
		}
		return nil
	case protocol.Rejected:
		if outcome.Code != "MALFORMED" && outcome.Code != "OPERATION_ID_REUSED" || outcome.EntityRevision != nil {
			return syncapp.ErrUnavailable
		}
		return nil
	case protocol.Accepted:
		if outcome.EntityRevision == nil || outcome.Code != "" || len(outcome.Warnings) != 0 {
			return syncapp.ErrUnavailable
		}
		stored, err := h.store.SegmentTx(ctx, syncapp.Borrow(db), source.Scope, v.OperationID)
		if err != nil || !storedSegment(source, v, stored, *outcome.EntityRevision) {
			return syncapp.ErrUnavailable
		}
		return nil
	}
	return syncapp.ErrUnavailable
}
