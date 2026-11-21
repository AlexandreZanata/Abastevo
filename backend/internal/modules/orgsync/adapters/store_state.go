package adapters

import (
	"context"
	"errors"
	"slices"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgsync"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// stateRecorded reports whether the operation has an EMP-OFFL-012 state
// decision. It reads the state table only when the assessment is composed.
func (s *Store) stateRecorded(ctx context.Context, queries *q.Queries, org, operation string) (bool, error) {
	if s.config.AssessState == nil {
		return false, nil
	}
	_, e := queries.ReadStateIntake(ctx, q.ReadStateIntakeParams{OrgID: org, OperationID: operation})
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	return e == nil, e
}

// replayState returns a state decision's stored acknowledgement through its
// own row, never through owner read authority (B-BR-EMP-310). done=false
// leaves ordinary replay unchanged.
func (s *Store) replayState(ctx context.Context, tx pgx.Tx, source app.Source, op protocol.Operation, limits protocol.Limits, descriptor app.Descriptor, proof app.ReviewProof, stored q.ReadAcknowledgementRow, fresh func() (time.Time, error)) (protocol.Acknowledgement, bool, error) {
	if s.config.AssessState == nil {
		return protocol.Acknowledgement{}, false, nil
	}
	row, e := q.New(tx).ReadStateIntake(ctx, q.ReadStateIntakeParams{OrgID: source.Scope.OrgID().String(), OperationID: op.ID()})
	if errors.Is(e, pgx.ErrNoRows) {
		return protocol.Acknowledgement{}, false, nil
	}
	if e != nil {
		return protocol.Acknowledgement{}, true, e
	}
	ack, outcome, e := storedAcknowledgement(stored, source, op, limits, descriptor)
	if e != nil {
		return protocol.Acknowledgement{}, true, e
	}
	received, ok := lateStamp(row.ReceivedAt)
	if !ok || row.Result != outcome.Result || row.Code != outcome.Code || !received.Equal(stored.ReceivedAt.Time) || outcome.EntityRevision != nil || row.Policy != app.StatePolicy {
		return protocol.Acknowledgement{}, true, app.ErrUnavailable
	}
	if outcome.Result == protocol.AcceptedForReview {
		if nilPort(proof) {
			return protocol.Acknowledgement{}, true, app.SourceDenied(app.ErrUnavailable)
		}
		if e := proof.CheckHeldTx(ctx, app.Borrow(tx), source, op, outcome.Copy()); e != nil {
			if errors.Is(e, app.ErrSourceDenied) {
				return protocol.Acknowledgement{}, true, e
			}
			return protocol.Acknowledgement{}, true, app.SourceDenied(e)
		}
	}
	if _, e := fresh(); e != nil {
		return protocol.Acknowledgement{}, true, e
	}
	return ack, true, nil
}

// processState publishes one state decision (EMP-OFFL-012): a hold through the
// owner's typed held path or a final ORG_SUSPENDED refusal, with its row and
// acknowledgement; the decision is taken again at the final receipt.
func (s *Store) processState(ctx context.Context, tx pgx.Tx, source app.Source, op protocol.Operation, limits protocol.Limits, descriptor app.Descriptor, proof app.ReviewProof, handler app.Handler, decision app.StateDecision, declared time.Time, exists bool, fresh func() (time.Time, error)) (protocol.Acknowledgement, error) {
	v := op.Snapshot()
	queries := q.New(tx)
	holder := app.LateHolderOf(handler)
	if decision.Holds() && (holder == nil || nilPort(proof) || !slices.Contains(descriptor.Results(), protocol.AcceptedForReview)) {
		return protocol.Acknowledgement{}, app.ErrUnavailable
	}
	if !exists {
		n, e := queries.ReserveOperation(ctx, q.ReserveOperationParams{OrgID: v.OrgID, OperationID: op.ID(), OwnerAccountID: v.OwnerAccountID, ApiOrigin: v.APIOrigin, MembershipID: v.MembershipID, EnrollmentID: v.EnrollmentID, OperationType: v.Type, OperationSchema: v.OperationSchema, EntityType: v.Entity.Type, EntityID: v.Entity.ID, PayloadDigest: v.PayloadDigest})
		if e != nil {
			return protocol.Acknowledgement{}, e
		}
		if n != 1 {
			return protocol.Acknowledgement{}, app.ErrUnavailable
		}
	}
	outcome := app.Outcome{Result: decision.Result, Code: decision.Code}
	if decision.Holds() {
		if len(v.DependsOn) > 0 {
			ids := make([]pgtype.UUID, len(v.DependsOn))
			for i, id := range v.DependsOn {
				if e := ids[i].Scan(id); e != nil {
					return protocol.Acknowledgement{}, app.ErrUnavailable
				}
			}
			ready, e := queries.ReadyDependencies(ctx, q.ReadyDependenciesParams{OrgID: v.OrgID, OperationIds: ids})
			if e != nil {
				return protocol.Acknowledgement{}, e
			}
			if ready != int64(len(ids)) {
				if _, e := fresh(); e != nil {
					return protocol.Acknowledgement{}, e
				}
				return ackFor(op, app.Outcome{Result: protocol.Retry, Code: "DEPENDENCY_PENDING"}, time.Time{}, limits)
			}
		}
		held, e := holder.HoldLate(ctx, app.Borrow(tx), source, op, decision.Code)
		if e != nil {
			if errors.Is(e, app.ErrSourceDenied) {
				return protocol.Acknowledgement{}, e
			}
			return protocol.Acknowledgement{}, app.ErrUnavailable
		}
		heldOutcome := held.Result == protocol.AcceptedForReview && held.Code == decision.Code && held.ReviewItemID != "" && held.EntityRevision == nil
		malformed := held.Result == protocol.Rejected && held.Code == "MALFORMED" && held.ReviewItemID == "" && held.EntityRevision == nil
		if !heldOutcome && !malformed || len(held.Warnings) != 0 {
			return protocol.Acknowledgement{}, app.ErrUnavailable
		}
		outcome = held
	}
	if !slices.Contains(descriptor.Results(), outcome.Result) {
		return protocol.Acknowledgement{}, app.ErrUnavailable
	}
	plan, bound, e := s.plan(source, op, descriptor, outcome.Result, proof)
	if e != nil {
		return protocol.Acknowledgement{}, e
	}
	var final protocol.Acknowledgement
	replay, e := s.config.Audit.RecordChangeTx(ctx, tx, source.Scope, bound, func(ctx context.Context, tx pgx.Tx) error {
		for _, consumer := range plan.Consumers {
			if e := s.config.EventWriter.Enqueue(ctx, source.Scope, tx, event.Envelope{Version: event.Version, OrgID: v.OrgID, Consumer: consumer, Event: plan.Event}, bound.RequestHash); e != nil {
				return e
			}
		}
		if outcome.Result == protocol.AcceptedForReview {
			if e := proof.CheckHeldTx(ctx, app.Borrow(tx), source, op, outcome.Copy()); e != nil {
				if errors.Is(e, app.ErrSourceDenied) {
					return e
				}
				return app.SourceDenied(e)
			}
		}
		at, e := fresh()
		if e != nil {
			return e
		}
		// The decision taken at the final receipt must not have changed.
		again, e := s.config.AssessState(ctx, tx, source, descriptor, declared)
		if e != nil {
			if errors.Is(e, app.ErrSourceDenied) {
				return e
			}
			return app.SourceDenied(e)
		}
		if !again.Same(decision) {
			return app.ErrUnavailable
		}
		final, e = ackFor(op, outcome, at, limits)
		if e != nil || !final.IsFinal() {
			return app.ErrUnavailable
		}
		declaredAt, sub := lateInstant(declared)
		params := q.InsertStateIntakeParams{OrgID: v.OrgID, OperationID: op.ID(), Condition: decision.Condition, Disposition: decision.Disposition, Capability: decision.Capability, CapabilityRevision: decision.CapabilityRevision, SubscriptionID: decision.SubscriptionID,
			SubscriptionRevision: decision.SubscriptionRevision, DeclaredAt: declaredAt, DeclaredSubmicro: sub, ReceivedAt: pgtype.Timestamptz{Time: at, Valid: true}, Result: outcome.Result, Code: outcome.Code}
		if !decision.Instant.IsZero() {
			params.PlanChangedAt = pgtype.Timestamptz{Time: decision.Instant, Valid: true}
		}
		if e := queries.InsertStateIntake(ctx, params); e != nil {
			return e
		}
		snap := final.Snapshot()
		return queries.InsertAcknowledgement(ctx, q.InsertAcknowledgementParams{OrgID: v.OrgID, OperationID: op.ID(), Result: snap.Result, Code: snap.Code, ReviewItemID: snap.ReviewItemID, ReceivedAt: pgtype.Timestamptz{Time: at, Valid: true}, Warnings: append([]string{}, snap.Warnings...), CanonicalAck: final.CanonicalBytes(), AuditHash: bound.RequestHash})
	})
	if e != nil {
		return protocol.Acknowledgement{}, e
	}
	if replay {
		return protocol.Acknowledgement{}, app.ErrUnavailable
	}
	if _, e := fresh(); e != nil {
		return protocol.Acknowledgement{}, e
	}
	return final, nil
}
