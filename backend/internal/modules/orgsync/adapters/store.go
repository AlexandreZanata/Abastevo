// Package adapters owns the organization intake ledger and its scoped transaction.
package adapters

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgsync"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	events "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/application"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errPendingRollback = errors.New("orgsync: guarded dependency pending")

func nilPort(value any) bool {
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

type Config struct {
	Pool          *pgxpool.Pool
	ConfirmSource func(context.Context, pgx.Tx, app.Source) (time.Time, error)
	Audit         auditing.TransactionRunner
	AuditCatalog  audit.Catalog
	EventCatalog  event.Catalog
	EventWriter   events.Writer
	// Exact source-owned consumer plans indexed by handler type, then final result.
	Plans map[string]map[string][]string
}
type Store struct{ config Config }

func NewStore(config Config) *Store {
	plans := make(map[string]map[string][]string, len(config.Plans))
	for kind, results := range config.Plans {
		plans[kind] = make(map[string][]string, len(results))
		for result, consumers := range results {
			plans[kind][result] = append([]string(nil), consumers...)
		}
	}
	config.Plans = plans
	return &Store{config: config}
}
func AuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "offl.operation.accepted", TargetType: "offl.operation", Reasons: []string{"intake"}, RequireReason: true},
		{Action: "offl.operation.review", TargetType: "offl.operation", Reasons: []string{"intake"}, RequireReason: true},
		{Action: "offl.operation.refused", TargetType: "offl.operation", Reasons: []string{"intake"}, RequireReason: true},
	}
}
func EventDefinitions(consumers []string) []event.Definition {
	return []event.Definition{
		{Name: "offl.operation.accepted", TargetType: "offl.operation", Consumers: append([]string(nil), consumers...)},
		{Name: "offl.operation.review", TargetType: "offl.operation", Consumers: append([]string(nil), consumers...)},
		{Name: "offl.operation.refused", TargetType: "offl.operation", Consumers: append([]string(nil), consumers...)},
	}
}
func action(result string) string {
	switch result {
	case protocol.Accepted:
		return "offl.operation.accepted"
	case protocol.AcceptedForReview:
		return "offl.operation.review"
	case protocol.Rejected:
		return "offl.operation.refused"
	}
	return ""
}
func (s *Store) plan(source app.Source, op protocol.Operation, result string) (event.Plan, audit.Event, error) {
	name := action(result)
	a := audit.Event{OperationID: op.ID(), ActorID: source.Actor.AccountID, Action: name, TargetType: "offl.operation", TargetID: op.ID(), ReasonCode: "intake", RequestHash: strings.TrimPrefix(op.PayloadDigest(), "sha256:")}
	if s.config.AuditCatalog.Validate(a) != nil {
		return event.Plan{}, audit.Event{}, app.ErrUnavailable
	}
	plan, bound, e := s.config.EventCatalog.Prepare(source.Scope, a, event.Event{ID: op.ID(), Name: name, TargetType: "offl.operation", TargetID: op.ID(), Revision: 1}, s.config.Plans[op.Snapshot().Type][result])
	if e != nil {
		return event.Plan{}, audit.Event{}, app.ErrUnavailable
	}
	return plan, bound, nil
}
func ackFor(op protocol.Operation, o app.Outcome, at time.Time, limits protocol.Limits) (protocol.Acknowledgement, error) {
	received := ""
	if o.Result != protocol.Retry {
		received = orgtime.FormatInstant(at)
	}
	return protocol.NewAcknowledgement(op, protocol.AcknowledgementSnapshot{Result: o.Result, Code: o.Code, EntityRevision: o.EntityRevision, ReceivedAt: received, ReviewItemID: o.ReviewItemID, Warnings: o.Warnings}, limits)
}
func (s *Store) Process(ctx context.Context, source app.Source, op protocol.Operation, limits protocol.Limits, handler app.Handler) (protocol.Acknowledgement, error) {
	var result protocol.Acknowledgement
	if s == nil || s.config.Pool == nil || s.config.ConfirmSource == nil || nilPort(s.config.Audit) || nilPort(s.config.EventWriter) || nilPort(handler) || source.Validate() != nil {
		return result, app.ErrUnavailable
	}
	descriptor := handler.Descriptor()
	v := op.Snapshot()
	if descriptor.Validate() != nil || descriptor.Type != v.Type {
		return result, app.ErrUnavailable
	}
	binding, e := protocol.NewBinding(source.Actor.AccountID, source.Origin, source.Scope.OrgID().String())
	if e != nil {
		return result, app.SourceDenied(orgapi.NotFound)
	}
	if _, e = protocol.ParseOperation(op.CanonicalBytes(), binding, limits); e != nil || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return result, app.SourceDenied(orgapi.NotFound)
	}
	for _, outcome := range []string{protocol.Accepted, protocol.AcceptedForReview, protocol.Rejected} {
		if _, _, e := s.plan(source, op, outcome); e != nil {
			return result, app.ErrUnavailable
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	e = database.WithOrgTx(ctx, s.config.Pool, source.Scope, func(tx pgx.Tx) error {
		fresh := func() (time.Time, error) {
			at, e := s.config.ConfirmSource(ctx, tx, source)
			if e != nil {
				if errors.Is(e, app.ErrSourceDenied) {
					return time.Time{}, e
				}
				return time.Time{}, app.SourceDenied(e)
			}
			if at.IsZero() {
				return time.Time{}, app.SourceDenied(app.ErrUnavailable)
			}
			return at, nil
		}
		admittedAt, e := fresh()
		if e != nil {
			return e
		}
		pending := func() error {
			outcome := app.Outcome{Result: protocol.Retry, Code: "DEPENDENCY_PENDING"}
			if e := handler.Confirm(ctx, app.Borrow(tx), source, op, app.Write, outcome); e != nil {
				if errors.Is(e, app.ErrSourceDenied) || !errors.Is(e, app.ErrDependencyPending) {
					return e
				}
			}
			if _, e := fresh(); e != nil {
				return e
			}
			return errPendingRollback
		}
		guardError := func(e error) error {
			if errors.Is(e, app.ErrSourceDenied) {
				return e
			}
			if errors.Is(e, app.ErrDependencyPending) {
				return pending()
			}
			return e
		}
		queries := q.New(tx)
		db := app.Borrow(tx)
		original, e := queries.ReadOperation(ctx, q.ReadOperationParams{OrgID: v.OrgID, OperationID: op.ID()})
		exists := e == nil
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if exists && (original.OwnerAccountID != v.OwnerAccountID || original.ApiOrigin != v.APIOrigin || original.MembershipID != v.MembershipID || original.EnrollmentID != v.EnrollmentID) {
			return app.SourceDenied(orgapi.NotFound)
		}
		if exists && original.PayloadDigest != op.PayloadDigest() {
			if e := handler.Admit(ctx, db, source, op, app.Write); e != nil {
				return guardError(e)
			}
			outcome := app.Outcome{Result: protocol.Rejected, Code: "OPERATION_ID_REUSED"}
			if e := handler.Confirm(ctx, db, source, op, app.Write, outcome); e != nil {
				return guardError(e)
			}
			at, e := fresh()
			if e != nil {
				return e
			}
			result, e = ackFor(op, outcome, at, limits)
			return e
		}
		if exists {
			if original.OperationType != v.Type || original.OperationSchema != v.OperationSchema || original.EntityType != v.Entity.Type || original.EntityID != v.Entity.ID {
				return app.ErrUnavailable
			}
			stored, e := queries.ReadAcknowledgement(ctx, q.ReadAcknowledgementParams{OrgID: v.OrgID, OperationID: op.ID()})
			if e == nil {
				if e := handler.Admit(ctx, db, source, op, app.ReadReceipt); e != nil {
					if errors.Is(e, app.ErrDependencyPending) && !errors.Is(e, app.ErrSourceDenied) {
						return app.ErrUnavailable
					}
					return e
				}
				raw := stored.CanonicalAck
				ack, e := protocol.ParseAcknowledgement(raw, op, limits)
				if e != nil || !ack.IsFinal() || !bytes.Equal(raw, ack.CanonicalBytes()) {
					return app.ErrUnavailable
				}
				a := ack.Snapshot()
				received, e := orgtime.ParseInstant(a.ReceivedAt)
				if e != nil || !stored.ReceivedAt.Valid || !stored.ReceivedAt.Time.Equal(received) || stored.Result != a.Result || stored.Code != a.Code || stored.ReviewItemID != a.ReviewItemID || !slices.Equal(stored.Warnings, a.Warnings) || stored.EntityRevision.Valid != (a.EntityRevision != nil) || (a.EntityRevision != nil && stored.EntityRevision.Int64 != *a.EntityRevision) || stored.AuditActorID != source.Actor.AccountID || stored.AuditAction != action(a.Result) || stored.AuditTargetType != "offl.operation" || stored.AuditTargetID != op.ID() || stored.SourceAuditHash != stored.AuditHash {
					return app.ErrUnavailable
				}
				outcome := app.Outcome{Result: a.Result, Code: a.Code, EntityRevision: a.EntityRevision, ReviewItemID: a.ReviewItemID, Warnings: a.Warnings}
				if e := handler.Confirm(ctx, db, source, op, app.ReadReceipt, outcome); e != nil {
					if errors.Is(e, app.ErrDependencyPending) && !errors.Is(e, app.ErrSourceDenied) {
						return app.ErrUnavailable
					}
					return e
				}
				if _, e := fresh(); e != nil {
					return e
				}
				result = ack
				return nil
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
		}
		if e := handler.Admit(ctx, db, source, op, app.Write); e != nil {
			return guardError(e)
		}
		if !exists {
			n, e := queries.ReserveOperation(ctx, q.ReserveOperationParams{OrgID: v.OrgID, OperationID: op.ID(), OwnerAccountID: v.OwnerAccountID, ApiOrigin: v.APIOrigin, MembershipID: v.MembershipID, EnrollmentID: v.EnrollmentID, OperationType: v.Type, OperationSchema: v.OperationSchema, EntityType: v.Entity.Type, EntityID: v.Entity.ID, PayloadDigest: v.PayloadDigest})
			if e != nil {
				return e
			}
			if n != 1 {
				return app.ErrUnavailable
			}
		}
		if len(v.DependsOn) > 0 {
			ids := make([]pgtype.UUID, len(v.DependsOn))
			for i, id := range v.DependsOn {
				if e := ids[i].Scan(id); e != nil {
					return app.ErrUnavailable
				}
			}
			ready, e := queries.ReadyDependencies(ctx, q.ReadyDependenciesParams{OrgID: v.OrgID, OperationIds: ids})
			if e != nil {
				return e
			}
			if ready != int64(len(ids)) {
				outcome := app.Outcome{Result: protocol.Retry, Code: "DEPENDENCY_PENDING"}
				if e := handler.Confirm(ctx, db, source, op, app.Write, outcome); e != nil {
					return guardError(e)
				}
				if _, e := fresh(); e != nil {
					return e
				}
				result, e = ackFor(op, outcome, time.Time{}, limits)
				return e
			}
		}
		outcome := app.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}
		if op.SupportsSchema() {
			outcome, e = handler.Apply(ctx, db, source, op)
			if e != nil {
				return e
			}
		}
		if outcome.Result == protocol.Retry {
			return app.ErrUnavailable
		}
		// Validate shape without exposing or persisting this provisional time.
		if validation, e := ackFor(op, outcome, admittedAt, limits); e != nil || !validation.IsFinal() {
			return app.ErrUnavailable
		}
		var final protocol.Acknowledgement
		plan, bound, e := s.plan(source, op, outcome.Result)
		if e != nil {
			return e
		}
		replay, e := s.config.Audit.RecordChangeTx(ctx, tx, source.Scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			for _, consumer := range plan.Consumers {
				if e := s.config.EventWriter.Enqueue(ctx, source.Scope, tx, event.Envelope{Version: event.Version, OrgID: v.OrgID, Consumer: consumer, Event: plan.Event}, bound.RequestHash); e != nil {
					return e
				}
			}
			at, e := fresh()
			if e != nil {
				return e
			}
			final, e = ackFor(op, outcome, at, limits)
			if e != nil || !final.IsFinal() {
				return app.ErrUnavailable
			}
			snap := final.Snapshot()
			revision := pgtype.Int8{}
			if snap.EntityRevision != nil {
				revision = pgtype.Int8{Int64: *snap.EntityRevision, Valid: true}
			}
			if e := queries.InsertAcknowledgement(ctx, q.InsertAcknowledgementParams{OrgID: v.OrgID, OperationID: op.ID(), Result: snap.Result, Code: snap.Code, EntityRevision: revision, ReviewItemID: snap.ReviewItemID, ReceivedAt: pgtype.Timestamptz{Time: at, Valid: true}, Warnings: append([]string{}, snap.Warnings...), CanonicalAck: final.CanonicalBytes(), AuditHash: bound.RequestHash}); e != nil {
				return e
			}
			if outcome.EffectReady() {
				if snap.EntityRevision == nil {
					return app.ErrUnavailable
				}
				return queries.InsertEffect(ctx, q.InsertEffectParams{OrgID: v.OrgID, OperationID: op.ID(), EntityRevision: *snap.EntityRevision})
			}
			return nil
		})
		if e != nil {
			return e
		}
		if replay {
			return app.ErrUnavailable
		}
		if e := handler.Confirm(ctx, db, source, op, app.Write, outcome); e != nil {
			return guardError(e)
		}
		if _, e := fresh(); e != nil {
			return e
		}
		result = final
		return nil
	})
	if !errors.Is(e, app.ErrSourceDenied) && errors.Is(e, errPendingRollback) && ctx.Err() == nil {
		return ackFor(op, app.Outcome{Result: protocol.Retry, Code: "DEPENDENCY_PENDING"}, time.Time{}, limits)
	}

	if e != nil {
		if errors.Is(e, app.ErrSourceDenied) {
			return protocol.Acknowledgement{}, e
		}
		return protocol.Acknowledgement{}, app.ErrUnavailable
	}
	if ctx.Err() != nil {
		return protocol.Acknowledgement{}, app.ErrUnavailable
	}
	return result, nil
}

var _ app.Store = (*Store)(nil)
