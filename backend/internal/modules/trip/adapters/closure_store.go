package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EMP-TRIP-014 supervisor closure persistence (EDR-258/259).

// ClosureAuditDefinitions is the closure audit action on target type trip;
// it carries the command digest and no reason text (BR-AUD-004).
func ClosureAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: app.CloseAuditAction, TargetType: ReviewTargetType}}
}

// CloseAuthority proves trip.trip.close_for_other over one stored trip (or
// the organization for the open-trip list) inside the owning transaction and
// returns the closer.
type CloseAuthority interface {
	CheckTripCloseTx(context.Context, pgx.Tx, kernel.OrgScope, app.ReviewActor, string, string, bool) (app.Reviewer, error)
}

// ClosureStore is the company-session closure owner.
type ClosureStore struct {
	Pool      *pgxpool.Pool
	Authority CloseAuthority
	Audit     auditing.TransactionRunner
}

var _ app.ClosureRepository = (*ClosureStore)(nil)

func (s *ClosureStore) ready() bool {
	return s != nil && s.Pool != nil && !absent(s.Authority) && !absent(s.Audit)
}

func closureRecordOf(operation, hash, member, employee, reason, missing string, at pgtype.Timestamptz) (*app.ClosureRecord, error) {
	closed, ok := exact(at, 0)
	if !ok || !canonicalID(operation) || !canonicalID(member) || !canonicalID(employee) {
		return nil, app.ErrUnavailable
	}
	return &app.ClosureRecord{OperationID: operation, RequestHash: hash, CloserMembershipID: member, CloserEmployeeID: employee, Reason: reason, MissingReason: missing, ClosedAt: closed}, nil
}

// tripClosure reads the stored closure of a completed trip without a finish.
func tripClosure(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, tripID string) (*app.ClosureRecord, error) {
	row, err := queries.GetTripClosure(ctx, q.GetTripClosureParams{OrgID: scope.OrgID().String(), TripID: tripID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrUnavailable
		}
		return nil, storeError(err)
	}
	if row.TripID != tripID {
		return nil, app.ErrUnavailable
	}
	return closureRecordOf(row.OperationID, row.RequestHash, row.CloserMembershipID, row.CloserEmployeeID, row.Reason, row.MissingReadingReason, row.ClosedAt)
}

// closeResult rebuilds the answer from the stored trip and its closure, so a
// replay returns exactly the first result; the usage is the pure trip's.
func closeResult(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, tripID string) (app.CloseResult, error) {
	record, err := (&Store{}).ReadTripTx(ctx, tx, scope, tripID)
	if err != nil || record.Closure == nil {
		return app.CloseResult{}, app.ErrUnavailable
	}
	closed, err := app.StoredDomain(scope, record)
	if err != nil {
		return app.CloseResult{}, app.ErrUnavailable
	}
	usage := closed.MeterUsage()
	return app.CloseResult{TripID: record.ID, Operational: closed.State(), Revision: app.OpenRevision + 1, Closure: *record.Closure, UsageStatus: usage.Status(), UsageReason: usage.Reason()}, nil
}

// CloseTrip records one supervisor closure (B-BR-EMP-399..403): authority
// (which holds the organization root for a write), the trip row lock, replay
// by operation, the expected revision and state, the separation of the
// closer from the author and driver, the pure closure, then the closure row
// (its insert completes the trip) and one audit event, and authority again.
func (s *ClosureStore) CloseTrip(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, command app.CloseCommand) (app.CloseResult, bool, error) {
	if !s.ready() {
		return app.CloseResult{}, false, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.CloseResult{}, false, app.ErrReviewUnauthorized
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.CloseResult{}, false, err
	}
	digest := command.Digest(scope)
	if digest == "" {
		return app.CloseResult{}, false, app.ErrUnavailable
	}
	bound := audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: app.CloseAuditAction, TargetType: ReviewTargetType, TargetID: v.TripID, RequestHash: digest}
	org := scope.OrgID().String()
	var result app.CloseResult
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		closer, err := s.Authority.CheckTripCloseTx(ctx, tx, scope, actor, member, v.TripID, true)
		if err != nil {
			return err
		}
		recheck := func() error {
			again, err := s.Authority.CheckTripCloseTx(ctx, tx, scope, actor, member, v.TripID, true)
			if err != nil {
				return err
			}
			if again != closer {
				return app.ErrUnavailable
			}
			return nil
		}
		queries := q.New(tx)
		if _, err := queries.LockTripForReview(ctx, q.LockTripForReviewParams{OrgID: org, ID: v.TripID}); err != nil {
			return err
		}
		if prior, err := queries.TripClosureByOperation(ctx, q.TripClosureByOperationParams{OrgID: org, OperationID: v.OperationID}); err == nil {
			repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(context.Context, pgx.Tx) error { return app.ErrUnavailable })
			if err != nil {
				return err
			}
			if !repeated || prior.TripID != v.TripID || prior.RequestHash != digest || prior.CloserMembershipID != closer.MembershipID {
				return app.ErrOperationReused
			}
			if result, err = closeResult(ctx, tx, scope, v.TripID); err != nil {
				return err
			}
			replay = true
			return recheck()
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		record, err := (&Store{}).ReadTripTx(ctx, tx, scope, v.TripID)
		if err != nil {
			return err
		}
		current := app.OpenRevision
		if record.State != trip.InProgress {
			current++
		}
		if v.ExpectedRevision != current || record.State != trip.InProgress {
			return app.ErrClosureConflict
		}
		// B-BR-EMP-401: account, membership and canonical employee of the
		// closer against the author's and the driver's.
		self, err := person(ctx, queries, org, closer.MembershipID)
		if err != nil {
			return err
		}
		if self.AccountID != closer.AccountID || self.Employees[0] != closer.EmployeeID {
			return app.ErrUnavailable
		}
		author, err := person(ctx, queries, org, record.AuthorMembershipID, record.AuthorEmployeeID)
		if err != nil {
			return err
		}
		driver, err := employee(ctx, queries, org, record.DriverID)
		if err != nil {
			return err
		}
		if self.AccountID == author.AccountID || self.MembershipID == author.MembershipID || sharesEmployee(self, author) || sharesEmployee(self, driver) {
			return trip.ErrSelfClosure
		}
		if _, err := app.CloseDomain(scope, record, closer.MembershipID, closer.EmployeeID, v.MissingReason); err != nil {
			return err
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := q.New(tx).InsertTripClosure(ctx, q.InsertTripClosureParams{OrgID: org, TripID: v.TripID, OperationID: v.OperationID, RequestHash: digest,
				CloserMembershipID: closer.MembershipID, CloserEmployeeID: closer.EmployeeID, Reason: v.Reason, MissingReadingReason: v.MissingReason}); err != nil {
				return err
			}
			result, err = closeResult(ctx, tx, scope, v.TripID)
			return err
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return recheck()
	})
	if err != nil {
		return app.CloseResult{}, false, reviewFailure(ctx, err)
	}
	return result, replay, nil
}

func sharesEmployee(a, b app.Person) bool {
	for _, x := range a.Employees {
		for _, y := range b.Employees {
			if x == y {
				return true
			}
		}
	}
	return false
}

// ListOpenTrips returns the organization's trips in progress, oldest start
// first, after one trip (EDR-259).
func (s *ClosureStore) ListOpenTrips(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, filter app.QueueFilter) (app.OpenTripPage, error) {
	if !s.ready() {
		return app.OpenTripPage{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.OpenTripPage{}, app.ErrReviewUnauthorized
	}
	if err := filter.Validate(); err != nil {
		return app.OpenTripPage{}, err
	}
	org := scope.OrgID().String()
	var page app.OpenTripPage
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		closer, err := s.Authority.CheckTripCloseTx(ctx, tx, scope, actor, member, org, false)
		if err != nil {
			return err
		}
		rows, err := q.New(tx).OpenTrips(ctx, q.OpenTripsParams{OrgID: org, AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		page = app.OpenTripPage{Items: make([]app.OpenTrip, 0, min(len(rows), filter.Limit))}
		for i, row := range rows {
			if i == filter.Limit {
				page.NextID = page.Items[len(page.Items)-1].TripID
				break
			}
			started, ok := exact(row.OccurredAt, row.OccurredSubmicro)
			if !ok {
				return app.ErrUnavailable
			}
			page.Items = append(page.Items, app.OpenTrip{TripID: row.ID, AssetID: row.AssetID, DriverEmployeeID: row.DriverEmployeeID, AuthorMembershipID: row.AuthorMembershipID,
				ReportingDay: row.ReportingDay, StartedAt: started, Revision: app.OpenRevision})
		}
		again, err := s.Authority.CheckTripCloseTx(ctx, tx, scope, actor, member, org, false)
		if err != nil {
			return err
		}
		if again != closer {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.OpenTripPage{}, reviewFailure(ctx, err)
	}
	return page, nil
}
