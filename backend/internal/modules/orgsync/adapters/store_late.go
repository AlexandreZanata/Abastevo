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
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// lateInstant stores an exact declared instant as its microsecond floor and
// the 0..999 nanosecond remainder (EDR-105).
func lateInstant(at time.Time) (pgtype.Timestamptz, int16) {
	floor := at.UTC().Truncate(time.Microsecond)
	return pgtype.Timestamptz{Time: floor, Valid: true}, int16(at.UTC().Sub(floor))
}

func lateStamp(v pgtype.Timestamptz) (time.Time, bool) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return v.Time.UTC(), true
}

// lateWindow reads the window in force at the end instant through the same
// SQL function the decision guard uses (EDR-197).
func lateWindow(ctx context.Context, tx pgx.Tx, org string, access app.LateAccess) (app.LateEvidence, error) {
	var days int32
	var revision pgtype.Int8
	if err := tx.QueryRow(ctx, `SELECT window_days,settings_revision FROM org_sync_late_window($1,$2)`, org, access.EndedAt).Scan(&days, &revision); err != nil {
		return app.LateEvidence{}, err
	}
	evidence := app.LateEvidence{LateAccess: access, WindowDays: int(days)}
	if revision.Valid {
		evidence.SettingsRevision = revision.Int64
	}
	if evidence.Validate() != nil || revision.Valid && revision.Int64 < 1 {
		return app.LateEvidence{}, app.ErrUnavailable
	}
	return evidence, nil
}

// ProcessLate decides one operation of an exact ended period from server
// history only (EMP-OFFL-009, EDR-197..199). It never runs the owner's current
// authority, never applies an effect and replays only its own decisions.
func (s *Store) ProcessLate(ctx context.Context, source app.Source, op protocol.Operation, limits protocol.Limits, handler app.Handler) (protocol.Acknowledgement, error) {
	var result protocol.Acknowledgement
	if s == nil || s.config.Pool == nil || s.config.ConfirmLate == nil || nilPort(s.config.Audit) || nilPort(handler) || source.Validate() != nil {
		return result, app.ErrUnavailable
	}
	descriptor := handler.Descriptor().Copy()
	v := op.Snapshot()
	if descriptor.Validate() != nil || descriptor.Type != v.Type || !slices.Contains(descriptor.Results(), protocol.Rejected) {
		return result, app.ErrUnavailable
	}
	var proof app.ReviewProof
	if slices.Contains(descriptor.Results(), protocol.AcceptedForReview) {
		provider, ok := handler.(app.ReviewProofProvider)
		if !ok || nilPort(provider) {
			return result, app.ErrUnavailable
		}
		if proof = provider.ReviewProof(); nilPort(proof) {
			return result, app.ErrUnavailable
		}
	}
	holder := app.LateHolderOf(handler)
	binding, e := protocol.NewBinding(source.Actor.AccountID, source.Origin, source.Scope.OrgID().String())
	if e != nil {
		return result, app.SourceDenied(orgapi.NotFound)
	}
	if _, e = protocol.ParseOperation(op.CanonicalBytes(), binding, limits); e != nil || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return result, app.SourceDenied(orgapi.NotFound)
	}
	declared, e := orgtime.ParseInstant(v.OccurredAt)
	if e != nil {
		return result, app.ErrUnavailable
	}
	if _, _, e := s.plan(source, op, descriptor, protocol.Rejected, proof); e != nil {
		return result, app.ErrUnavailable
	}
	org := source.Scope.OrgID().String()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	e = database.WithOrgTx(ctx, s.config.Pool, source.Scope, func(tx pgx.Tx) error {
		fresh := func() (app.LateEvidence, time.Time, error) {
			access, at, e := s.config.ConfirmLate(ctx, tx, source)
			if e != nil {
				if errors.Is(e, app.ErrSourceDenied) {
					return app.LateEvidence{}, time.Time{}, e
				}
				return app.LateEvidence{}, time.Time{}, app.SourceDenied(e)
			}
			if at.IsZero() || access.Validate() != nil || at.Before(access.EndedAt) {
				return app.LateEvidence{}, time.Time{}, app.SourceDenied(app.ErrUnavailable)
			}
			evidence, e := lateWindow(ctx, tx, org, access)
			if e != nil {
				return app.LateEvidence{}, time.Time{}, app.SourceDenied(app.ErrUnavailable)
			}
			return evidence, at, nil
		}
		evidence, admittedAt, e := fresh()
		if e != nil {
			return e
		}
		same := func(again app.LateEvidence) bool {
			return again.End == evidence.End && again.EndedAt.Equal(evidence.EndedAt) && again.WindowDays == evidence.WindowDays && again.SettingsRevision == evidence.SettingsRevision
		}
		queries := q.New(tx)
		original, e := queries.ReadOperation(ctx, q.ReadOperationParams{OrgID: v.OrgID, OperationID: op.ID()})
		exists := e == nil
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if exists && (original.OwnerAccountID != v.OwnerAccountID || original.ApiOrigin != v.APIOrigin || original.MembershipID != v.MembershipID || original.EnrollmentID != v.EnrollmentID) {
			return app.SourceDenied(orgapi.NotFound)
		}
		if exists && original.PayloadDigest != op.PayloadDigest() {
			_, at, e := fresh()
			if e != nil {
				return e
			}
			result, e = ackFor(op, app.Outcome{Result: protocol.Rejected, Code: "OPERATION_ID_REUSED"}, at, limits)
			return e
		}
		var claimed *time.Time
		if exists {
			if original.OperationType != v.Type || original.OperationSchema != v.OperationSchema || original.EntityType != v.Entity.Type || original.EntityID != v.Entity.ID {
				return app.ErrUnavailable
			}
			stored, e := queries.ReadAcknowledgement(ctx, q.ReadAcknowledgementParams{OrgID: v.OrgID, OperationID: op.ID()})
			if e == nil {
				// Only this path's own decisions replay; a normal receipt needs
				// current read coverage an ended period never has (EDR-199).
				late, e := queries.ReadLateIntake(ctx, q.ReadLateIntakeParams{OrgID: v.OrgID, OperationID: op.ID()})
				if errors.Is(e, pgx.ErrNoRows) {
					return app.ErrUnavailable
				}
				if e != nil {
					return e
				}
				ack, outcome, e := storedAcknowledgement(stored, source, op, limits, descriptor)
				if e != nil {
					return e
				}
				received, ok := lateStamp(late.ReceivedAt)
				if !ok || late.MembershipID != v.MembershipID || late.EnrollmentID != v.EnrollmentID || late.Result != outcome.Result || late.Code != outcome.Code || !received.Equal(stored.ReceivedAt.Time) || outcome.EntityRevision != nil {
					return app.ErrUnavailable
				}
				if outcome.Result == protocol.AcceptedForReview {
					if nilPort(proof) {
						return app.SourceDenied(app.ErrUnavailable)
					}
					if e := proof.CheckHeldTx(ctx, app.Borrow(tx), source, op, outcome.Copy()); e != nil {
						if errors.Is(e, app.ErrSourceDenied) {
							return e
						}
						return app.SourceDenied(e)
					}
				}
				if _, _, e := fresh(); e != nil {
					return e
				}
				result = ack
				return nil
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			at, ok := lateStamp(original.ClaimedAt)
			if !ok {
				return app.ErrUnavailable
			}
			claimed = &at
		}
		decision, e := app.DecideLate(evidence, app.LateFacts{Declared: &declared, Claimed: claimed, Received: admittedAt})
		if e != nil {
			return app.ErrUnavailable
		}
		// An owner without a held path keeps eligible work retryable with no
		// new claim (fuelings until FUEL-003, EDR-177).
		if decision.Holds() && (holder == nil || nilPort(proof) || !slices.Contains(descriptor.Results(), protocol.AcceptedForReview)) {
			return app.ErrUnavailable
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
		outcome := app.Outcome{Result: decision.Result, Code: decision.Code}
		if decision.Holds() {
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
					if _, _, e := fresh(); e != nil {
						return e
					}
					result, e = ackFor(op, app.Outcome{Result: protocol.Retry, Code: "DEPENDENCY_PENDING"}, time.Time{}, limits)
					return e
				}
			}
			outcome, e = holder.HoldLate(ctx, app.Borrow(tx), source, op, app.LateHeldCode)
			if e != nil {
				if errors.Is(e, app.ErrSourceDenied) {
					return e
				}
				return app.ErrUnavailable
			}
			heldOutcome := outcome.Result == protocol.AcceptedForReview && outcome.Code == app.LateHeldCode && outcome.ReviewItemID != "" && outcome.EntityRevision == nil
			malformed := outcome.Result == protocol.Rejected && outcome.Code == "MALFORMED" && outcome.ReviewItemID == "" && outcome.EntityRevision == nil
			if !heldOutcome && !malformed || len(outcome.Warnings) != 0 {
				return app.ErrUnavailable
			}
		}
		if !slices.Contains(descriptor.Results(), outcome.Result) {
			return app.ErrUnavailable
		}
		if validation, e := ackFor(op, outcome, admittedAt, limits); e != nil || !validation.IsFinal() {
			return app.ErrUnavailable
		}
		plan, bound, e := s.plan(source, op, descriptor, outcome.Result, proof)
		if e != nil {
			return e
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
			again, at, e := fresh()
			if e != nil {
				return e
			}
			// The final receipt decides: a window closing during the waits
			// rolls back for a fresh decision, never a stale one.
			now, e := app.DecideLate(again, app.LateFacts{Declared: &declared, Claimed: claimed, Received: at})
			if e != nil || !same(again) || now != decision {
				return app.ErrUnavailable
			}
			final, e = ackFor(op, outcome, at, limits)
			if e != nil || !final.IsFinal() {
				return app.ErrUnavailable
			}
			declaredAt, sub := lateInstant(declared)
			if e := queries.InsertLateIntake(ctx, q.InsertLateIntakeParams{OrgID: v.OrgID, OperationID: op.ID(), MembershipID: v.MembershipID, EnrollmentID: v.EnrollmentID, EndSource: string(evidence.End), EndedAt: pgtype.Timestamptz{Time: evidence.EndedAt, Valid: true}, WindowDays: int16(evidence.WindowDays), SettingsRevision: evidence.SettingsRevision, DeclaredAt: declaredAt, DeclaredSubmicro: sub, ReceivedAt: pgtype.Timestamptz{Time: at, Valid: true}, Disposition: string(decision.Disposition), Result: outcome.Result, Code: outcome.Code}); e != nil {
				return e
			}
			snap := final.Snapshot()
			return queries.InsertAcknowledgement(ctx, q.InsertAcknowledgementParams{OrgID: v.OrgID, OperationID: op.ID(), Result: snap.Result, Code: snap.Code, ReviewItemID: snap.ReviewItemID, ReceivedAt: pgtype.Timestamptz{Time: at, Valid: true}, Warnings: append([]string{}, snap.Warnings...), CanonicalAck: final.CanonicalBytes(), AuditHash: bound.RequestHash})
		})
		if e != nil {
			return e
		}
		if replay {
			return app.ErrUnavailable
		}
		if _, _, e := fresh(); e != nil {
			return e
		}
		result = final
		return nil
	})
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

var _ app.LateStore = (*Store)(nil)
