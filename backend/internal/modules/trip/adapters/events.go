package adapters

import (
	"context"
	"errors"
	"slices"

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

// ---- trip.event.record (EMP-TRIP-003, B-BR-EMP-321..324) ----

type eventHandler struct {
	ports
	events app.EventStore
}

// NewEventHandler composes the trip.event.record owner from real ports only:
// the trip authority, the fleet lifecycle lookup that takes the root and
// asset locks, the trip store and the logbook store.
func NewEventHandler(authority app.Authority, readings app.Readings, trips app.Store, events app.EventStore) (syncapp.Handler, error) {
	p := ports{authority: authority, readings: readings, store: trips}
	if !p.ready() || absent(events) {
		return nil, syncapp.ErrUnavailable
	}
	return &eventHandler{ports: p, events: events}, nil
}

func (*eventHandler) Descriptor() syncapp.Descriptor { return descriptor(app.EventOperationType) }

// ReviewProof is the held-event proof (OFFL-008).
func (h *eventHandler) ReviewProof() syncapp.ReviewProof { return h.events }

func (h *eventHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.EventCommand, app.EventInput, error) {
	if h == nil || !h.ready() || absent(h.events) {
		return app.EventCommand{}, app.EventInput{}, syncapp.ErrUnavailable
	}
	if err := bound(ctx, db, source, op, mode, app.EventOperationType); err != nil {
		return app.EventCommand{}, app.EventInput{}, err
	}
	command, err := app.ParseEvent(op)
	if err != nil {
		return app.EventCommand{}, app.EventInput{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.EventCommand{}, app.EventInput{}, syncapp.ErrUnavailable
	}
	return command, v, nil
}

// trip reads the event's trip; a trip not stored here (undelivered, held or
// another organization's) admits only the current device and is a pending
// dependency that reveals nothing (EDR-207).
func (h *eventHandler) trip(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.EventInput, mode syncapp.Mode) (app.Record, error) {
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

func (h *eventHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
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

// eventRefusal maps a logbook refusal at receipt (EDR-207).
func eventRefusal(err error) (syncapp.Outcome, error) {
	switch {
	case errors.Is(err, trip.ErrForbidden):
		return syncapp.Outcome{}, syncapp.SourceDenied(orgapi.Forbidden)
	case errors.Is(err, trip.ErrEventBeforeStart), errors.Is(err, trip.ErrInvalid), errors.Is(err, trip.ErrScope):
		return malformed(), nil
	}
	return syncapp.Outcome{}, syncapp.ErrUnavailable
}

func (h *eventHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
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
	// Re-read under the locks: a concurrent finish may have completed it.
	if record, err = h.store.ReadTripTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	facts, err := h.events.EventFactsTx(ctx, syncapp.Borrow(db), source.Scope, v.EventID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if facts.ExistingOperation == v.OperationID || facts.HeldOperation == v.OperationID {
		// A stored fact without its stored receipt is never re-acknowledged.
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if facts.ExistingOperation != "" || facts.HeldOperation != "" {
		return malformed(), nil
	}
	_, err = app.EventDomain(source.Scope, record, v)
	if errors.Is(err, trip.ErrEventAfterFinish) {
		review, err := h.events.HoldEventTx(ctx, syncapp.Borrow(db), source, op, command, app.TripAlreadyClosed)
		return held(review, app.TripAlreadyClosed, err)
	}
	if err != nil {
		return eventRefusal(err)
	}
	// Changed rights on an event store nothing until a held owner exists.
	if disposition != app.Apply {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	employee, err := h.store.AuthorEmployeeTx(ctx, syncapp.Borrow(db), source.Scope, source.MembershipID)
	if err != nil || employee != record.AuthorEmployeeID {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.events.InsertEventTx(ctx, syncapp.Borrow(db), source.Scope, source, command, employee); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if disposition, err := h.classify(ctx, db, source, record.AssetID, record.DriverID, v.Declared.OccurredAt); err != nil || disposition != app.Apply {
		if err == nil {
			err = syncapp.ErrUnavailable
		}
		return syncapp.Outcome{}, err
	}
	revision := int64(1)
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

// storedEvent proves the stored event is exactly this operation's fact.
func storedEvent(source syncapp.Source, v app.EventInput, e app.EventRecord) bool {
	return e.ID == v.EventID && e.TripID == v.TripID && e.OperationID == v.OperationID && e.PayloadDigest == v.PayloadDigest && e.Type == v.Type &&
		sameText(e.Text, v.Text) && e.AuthorMembershipID == source.MembershipID && e.EnrollmentID == source.EnrollmentID &&
		e.Declared.OccurredAt.Equal(v.Declared.OccurredAt) && e.Declared.EntryZone == v.Declared.EntryZone &&
		e.Declared.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds && !e.ReceivedAt.IsZero()
}

func (h *eventHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
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
		if outcome.Code != app.TripAlreadyClosed || outcome.EntityRevision != nil {
			return syncapp.ErrUnavailable
		}
		if err := h.events.CheckHeldTx(ctx, syncapp.Borrow(db), source, op, outcome.Copy()); err != nil {
			if errors.Is(err, syncapp.ErrSourceDenied) {
				return err
			}
			return syncapp.SourceDenied(err)
		}
	case protocol.Accepted:
		if outcome.EntityRevision == nil || *outcome.EntityRevision != 1 || outcome.Code != "" || len(outcome.Warnings) != 0 {
			return syncapp.ErrUnavailable
		}
		stored, err := h.events.ReadEventTx(ctx, syncapp.Borrow(db), source.Scope, v.EventID)
		if err != nil || !storedEvent(source, v, stored) {
			return syncapp.ErrUnavailable
		}
	}
	return nil
}

// ---- logbook store (EDR-208) ----

// EventStore persists accepted and held logbook events; NewID names the
// review items of held events. Reports is the EMP-CHECK-003 source of the
// timeline's INCIDENT and DAMAGE facts (EDR-240); the timeline is
// unavailable without it, never silently incomplete.
type EventStore struct {
	NewID   func() (string, error)
	Reports app.TimelineReports
}

var _ app.EventStore = (*EventStore)(nil)

func (s *EventStore) port(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (*q.Queries, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return nil, app.ErrUnavailable
	}
	return q.New(db), nil
}

// EventFactsTx answers the operations already holding the event identity.
func (s *EventStore) EventFactsTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, event string) (app.EventFacts, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || !canonicalID(event) {
		return app.EventFacts{}, app.ErrUnavailable
	}
	row, err := queries.TripEventFacts(ctx, q.TripEventFactsParams{OrgID: scope.OrgID().String(), ID: event})
	if err != nil {
		return app.EventFacts{}, storeError(err)
	}
	return app.EventFacts{ExistingOperation: row.ExistingOperation, HeldOperation: row.HeldOperation}, nil
}

// InsertEventTx writes the logbook fact of an accepted event.
func (s *EventStore) InsertEventTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.EventCommand, employee string) error {
	queries, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID || !canonicalID(employee) {
		return app.ErrUnavailable
	}
	at, sub := instant(v.Declared.OccurredAt)
	if err := queries.InsertTripEvent(ctx, q.InsertTripEventParams{OrgID: scope.OrgID().String(), ID: v.EventID, TripID: v.TripID, OperationID: v.OperationID,
		PayloadDigest: v.PayloadDigest, EventType: string(v.Type), EventText: text(v.Text), AuthorMembershipID: source.MembershipID, AuthorEmployeeID: employee,
		EnrollmentID: source.EnrollmentID, OccurredAt: at, OccurredSubmicro: sub, EntryZone: v.Declared.EntryZone, DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds}); err != nil {
		return storeError(err)
	}
	return nil
}

func eventRecord(row q.GetTripEventRow) (app.EventRecord, error) {
	at, ok := exact(row.OccurredAt, row.OccurredSubmicro)
	kind, err := trip.ParseLogbookType(row.EventType)
	if !ok || err != nil || !kind.Supported() || !row.ReceivedAt.Valid {
		return app.EventRecord{}, app.ErrUnavailable
	}
	return app.EventRecord{ID: row.ID, TripID: row.TripID, OperationID: row.OperationID, PayloadDigest: row.PayloadDigest, AuthorMembershipID: row.AuthorMembershipID,
		AuthorEmployeeID: row.AuthorEmployeeID, EnrollmentID: row.EnrollmentID, Type: kind, Text: optionalText(row.EventText),
		Declared: orgtime.Declaration{OccurredAt: at, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds}, ReceivedAt: row.ReceivedAt.Time.UTC()}, nil
}

// ReadEventTx reads one accepted logbook fact of the scope.
func (s *EventStore) ReadEventTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, event string) (app.EventRecord, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil {
		return app.EventRecord{}, err
	}
	row, err := queries.GetTripEvent(ctx, q.GetTripEventParams{OrgID: scope.OrgID().String(), ID: event})
	if err != nil {
		return app.EventRecord{}, storeError(err)
	}
	return eventRecord(row)
}

// TimelineTx reads one stored trip's timeline (B-BR-EMP-324): its start, its
// accepted logbook facts, the reports made inside it (EMP-CHECK-003, as
// INCIDENT or DAMAGE facts without text) and its finish, each with fact
// type, actor, declaration and server receipt, in the shared domain order.
// It computes no working time (EDR-028) and lists no held event.
func (s *EventStore) TimelineTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, tripID string) ([]app.TimelineEntry, error) {
	queries, err := s.port(ctx, db, scope)
	if err != nil || absent(s.Reports) {
		return nil, app.ErrUnavailable
	}
	record, err := (&Store{}).ReadTripTx(ctx, db, scope, tripID)
	if err != nil {
		return nil, err
	}
	entries := []app.TimelineEntry{{Item: trip.TimelineItem{Kind: trip.TimelineStart, ID: record.ID, At: record.Declared.OccurredAt}, FactType: string(trip.TimelineStart),
		AuthorMembershipID: record.AuthorMembershipID, Declared: record.Declared, ReceivedAt: record.ReceivedAt}}
	rows, err := queries.ListTripEvents(ctx, q.ListTripEventsParams{OrgID: scope.OrgID().String(), TripID: tripID})
	if err != nil {
		return nil, storeError(err)
	}
	for _, row := range rows {
		e, err := eventRecord(q.GetTripEventRow(row))
		if err != nil || e.TripID != record.ID {
			return nil, app.ErrUnavailable
		}
		entries = append(entries, app.TimelineEntry{Item: trip.TimelineItem{Kind: trip.TimelineEvent, ID: e.ID, At: e.Declared.OccurredAt}, FactType: string(e.Type),
			AuthorMembershipID: e.AuthorMembershipID, Declared: e.Declared, ReceivedAt: e.ReceivedAt, Text: e.Text})
	}
	reports, err := s.Reports.TripReportsTx(ctx, syncapp.Borrow(db), scope, record.ID)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	for _, r := range reports {
		if !canonicalID(r.ID) || r.Type != trip.LogbookIncident && r.Type != trip.LogbookDamage || !canonicalID(r.AuthorMembershipID) || r.ReceivedAt.IsZero() {
			return nil, app.ErrUnavailable
		}
		if _, err := orgtime.ValidateDeclaration(r.Declared); err != nil {
			return nil, app.ErrUnavailable
		}
		entries = append(entries, app.TimelineEntry{Item: trip.TimelineItem{Kind: trip.TimelineEvent, ID: r.ID, At: r.Declared.OccurredAt}, FactType: string(r.Type),
			AuthorMembershipID: r.AuthorMembershipID, Declared: r.Declared, ReceivedAt: r.ReceivedAt})
	}
	if f := record.Finish; f != nil {
		entries = append(entries, app.TimelineEntry{Item: trip.TimelineItem{Kind: trip.TimelineFinish, ID: record.ID, At: f.Declared.OccurredAt}, FactType: string(trip.TimelineFinish),
			AuthorMembershipID: f.AuthorMembershipID, Declared: f.Declared, ReceivedAt: f.ReceivedAt})
	}
	slices.SortStableFunc(entries, func(a, b app.TimelineEntry) int { return trip.CompareTimeline(a.Item, b.Item) })
	return entries, nil
}

// heldEventProof compares a stored held event with the operation's parsed event.
func heldEventProof(source syncapp.Source, op protocol.Operation, row q.ReadTripEventHeldProofRow, v app.EventInput) bool {
	e := op.Snapshot()
	at, ok := exact(row.OccurredAt, row.OccurredSubmicro)
	return ok && heldBinding(source, op) && row.ProofVersion == 1 && canonicalID(row.ID) && canonicalID(row.AuthorEmployeeID) &&
		row.OperationType == app.EventOperationType && row.OperationSchema == e.OperationSchema && row.PayloadDigest == op.PayloadDigest() &&
		row.OwnerAccountID == source.Actor.AccountID && row.ApiOrigin == source.Origin && row.MembershipID == source.MembershipID &&
		row.EnrollmentID == source.EnrollmentID && row.EntityType == app.EventEntityType && row.EntityID == v.EventID && row.EventID == v.EventID &&
		row.TripID == v.TripID && row.EventType == string(v.Type) && sameText(optionalText(row.EventText), v.Text) && row.Code == app.TripAlreadyClosed &&
		at.Equal(v.Declared.OccurredAt) && row.EntryZone == v.Declared.EntryZone && row.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds
}

func parsedEvent(op protocol.Operation) (app.EventInput, error) {
	command, err := app.ParseEvent(op)
	if err != nil {
		return app.EventInput{}, app.ErrUnavailable
	}
	return command.Snapshot()
}

// HoldEventTx keeps an event declared after its trip's finish (B-BR-EMP-323).
func (s *EventStore) HoldEventTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, command app.EventCommand, code string) (string, error) {
	queries, err := s.port(ctx, db, source.Scope)
	v, errCommand := command.Snapshot()
	if err != nil || errCommand != nil || s.NewID == nil || code != app.TripAlreadyClosed || op.Snapshot().Type != app.EventOperationType || v.OperationID != op.ID() || !heldBinding(source, op) {
		return "", app.ErrUnavailable
	}
	org := source.Scope.OrgID().String()
	row, err := queries.ReadTripEventHeldProof(ctx, q.ReadTripEventHeldProofParams{OrgID: org, OperationID: op.ID()})
	if err == nil {
		if !heldEventProof(source, op, row, v) {
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
	if err := queries.InsertTripHeldEvent(ctx, q.InsertTripHeldEventParams{OrgID: org, ReviewItemID: review, OperationID: op.ID(), ID: v.EventID, TripID: v.TripID,
		EventType: string(v.Type), EventText: text(v.Text), OccurredAt: at, OccurredSubmicro: sub, EntryZone: v.Declared.EntryZone,
		DeviceOffsetSeconds: v.Declared.DeviceOffsetSeconds}); err != nil {
		return "", storeError(err)
	}
	return review, nil
}

// CheckHeldTx proves a held event outcome against the stored held fact and,
// once written, its exact receipt (OFFL-008 ReviewProof).
func (s *EventStore) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	queries, err := s.port(ctx, db, source.Scope)
	if err != nil || !heldBinding(source, op) {
		return app.ErrUnavailable
	}
	v, err := parsedEvent(op)
	if err != nil {
		return app.ErrUnavailable
	}
	row, err := queries.ReadTripEventHeldProof(ctx, q.ReadTripEventHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if err != nil || !heldEventProof(source, op, row, v) || outcome.Result != protocol.AcceptedForReview || outcome.Code != row.Code || outcome.ReviewItemID != row.ID || outcome.EntityRevision != nil {
		return app.ErrUnavailable
	}
	if len(row.CanonicalAck) != 0 {
		return heldEventReceipt(op, row)
	}
	if row.AcknowledgementResult != "" || row.AcknowledgementProofVersion != 0 || row.ReceivedAt.Valid {
		return app.ErrUnavailable
	}
	return nil
}

func heldEventReceipt(op protocol.Operation, row q.ReadTripEventHeldProofRow) error {
	return heldReceipt(op, q.ReadTripHeldProofRow{ID: row.ID, Code: row.Code, AcknowledgementResult: row.AcknowledgementResult, AcknowledgementCode: row.AcknowledgementCode,
		AcknowledgementReviewItemID: row.AcknowledgementReviewItemID, AcknowledgementProofVersion: row.AcknowledgementProofVersion, ReceivedAt: row.ReceivedAt, CanonicalAck: row.CanonicalAck})
}

// CheckOwnReceiptTx lets the author re-read its own held event receipt
// without the record read permission (OFFL-008).
func (s *EventStore) CheckOwnReceiptTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (bool, error) {
	queries, err := s.port(ctx, db, source.Scope)
	if err != nil || source.Validate() != nil {
		return false, app.ErrUnavailable
	}
	row, err := queries.ReadTripEventHeldProof(ctx, q.ReadTripEventHeldProofParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, app.ErrUnavailable
	}
	v, err := parsedEvent(op)
	if err != nil {
		return false, app.ErrUnavailable
	}
	if !heldEventProof(source, op, row, v) {
		return false, app.ErrNotFound
	}
	if err := heldEventReceipt(op, row); err != nil {
		return false, err
	}
	return true, nil
}
