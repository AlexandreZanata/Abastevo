package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EMP-TRIP-012 driver handover persistence (EDR-255).

// HandoverAuditDefinitions is the handover audit action on target type trip;
// it carries the command digest and no reason text (BR-AUD-004).
func HandoverAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: app.HandoverAuditAction, TargetType: ReviewTargetType}}
}

// HandoverAuthority proves the three handover policies inside the owning
// transaction (trip.trip.close_for_other over a stored trip or the
// organization, trip.trip.start over the requesting membership) and answers
// whether a receiver is eligible for a trip at a database instant.
type HandoverAuthority interface {
	CheckTripHandoverTx(context.Context, pgx.Tx, kernel.OrgScope, app.ReviewActor, string, string, app.HandoverAction) (app.Reviewer, error)
	EligibleReceiverTx(context.Context, pgx.Tx, kernel.OrgScope, string, app.Target, time.Time) (bool, error)
}

// HandoverStore is the company-session handover owner.
type HandoverStore struct {
	Pool      *pgxpool.Pool
	Authority HandoverAuthority
	Audit     auditing.TransactionRunner
}

var _ app.HandoverRepository = (*HandoverStore)(nil)

func (s *HandoverStore) ready() bool {
	return s != nil && s.Pool != nil && !absent(s.Authority) && !absent(s.Audit)
}

func handoverRecords(rows []q.ListTripHandoversRow) ([]app.HandoverRecord, error) {
	out := make([]app.HandoverRecord, 0, len(rows))
	for _, row := range rows {
		at, ok := exact(row.HandedOverAt, 0)
		if !ok {
			return nil, app.ErrUnavailable
		}
		out = append(out, app.HandoverRecord{Revision: row.Revision, OperationID: row.OperationID, RequestHash: row.RequestHash, FromMembershipID: row.FromMembershipID,
			FromEmployeeID: row.FromEmployeeID, ToMembershipID: row.ToMembershipID, ToEmployeeID: row.ToEmployeeID, RecorderMembershipID: row.RecorderMembershipID,
			RecorderEmployeeID: row.RecorderEmployeeID, Reason: row.Reason, HandedOverAt: at})
	}
	return out, nil
}

// handoverResult rebuilds the answer of one stored handover from the stored
// trip, so a replay returns exactly the first answer: the trip's state at the
// handover's revision (a handover never moves the state).
func handoverResult(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, tripID, operation string) (app.HandoverResult, error) {
	record, err := (&Store{}).ReadTripTx(ctx, tx, scope, tripID)
	if err != nil {
		return app.HandoverResult{}, app.ErrUnavailable
	}
	if _, err := app.StoredDomain(scope, record); err != nil {
		return app.HandoverResult{}, app.ErrUnavailable
	}
	for _, h := range record.Handovers {
		if h.OperationID != operation {
			continue
		}
		state := trip.InProgress
		for _, x := range record.Transitions {
			if x.Revision < h.Revision {
				state = x.To
			}
		}
		return app.HandoverResult{TripID: record.ID, Operational: state, Revision: h.Revision, Handover: h}, nil
	}
	return app.HandoverResult{}, app.ErrUnavailable
}

// samePerson compares two people by account, membership and employee
// records of their merge chains (BR-REV-003, B-BR-EMP-397).
func samePerson(a, b app.Person) bool {
	return a.AccountID != "" && a.AccountID == b.AccountID || a.MembershipID != "" && a.MembershipID == b.MembershipID || sharesEmployee(a, b)
}

// handoverDomainError maps a pure refusal at receipt to the command's answer.
func handoverDomainError(err error) error {
	switch {
	case errors.Is(err, trip.ErrConflict), errors.Is(err, trip.ErrInvalidTransition):
		return app.ErrHandoverConflict
	case errors.Is(err, trip.ErrForbidden):
		return app.ErrSelfHandover
	case errors.Is(err, trip.ErrInvalid):
		return app.ErrReceiverIneligible
	}
	return app.ErrUnavailable
}

// HandOver records one handover (B-BR-EMP-394..397): authority (which holds
// the organization root for a write), the trip row lock, replay by
// operation, the expected revision and open state (CONFLICT), the separation
// of the recorder from the current responsible member and the receiver
// (SELF_APPROVAL), the receiver's eligibility at the database instant
// (VALIDATION_FAILED), the pure handover, then the handover row and one audit
// event, and authority again.
func (s *HandoverStore) HandOver(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, command app.HandoverCommand) (app.HandoverResult, bool, error) {
	if !s.ready() {
		return app.HandoverResult{}, false, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.HandoverResult{}, false, app.ErrReviewUnauthorized
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.HandoverResult{}, false, err
	}
	digest := command.Digest(scope)
	if digest == "" {
		return app.HandoverResult{}, false, app.ErrUnavailable
	}
	bound := audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: app.HandoverAuditAction, TargetType: ReviewTargetType, TargetID: v.TripID, RequestHash: digest}
	org := scope.OrgID().String()
	var result app.HandoverResult
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		recorder, err := s.Authority.CheckTripHandoverTx(ctx, tx, scope, actor, member, v.TripID, app.HandoverWriteAction)
		if err != nil {
			return err
		}
		recheck := func() error {
			again, err := s.Authority.CheckTripHandoverTx(ctx, tx, scope, actor, member, v.TripID, app.HandoverWriteAction)
			if err != nil {
				return err
			}
			if again != recorder {
				return app.ErrUnavailable
			}
			return nil
		}
		queries := q.New(tx)
		if _, err := queries.LockTripForReview(ctx, q.LockTripForReviewParams{OrgID: org, ID: v.TripID}); err != nil {
			return err
		}
		if prior, err := queries.TripHandoverByOperation(ctx, q.TripHandoverByOperationParams{OrgID: org, OperationID: v.OperationID}); err == nil {
			repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(context.Context, pgx.Tx) error { return app.ErrUnavailable })
			if err != nil {
				return err
			}
			if !repeated || prior.TripID != v.TripID || prior.RequestHash != digest || prior.RecorderMembershipID != recorder.MembershipID {
				return app.ErrOperationReused
			}
			if result, err = handoverResult(ctx, tx, scope, v.TripID, v.OperationID); err != nil {
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
		// EMP-TRIP-011: an open trip is IN_PROGRESS or PAUSED; a closed one,
		// or a revision that is not the current one, is CONFLICT.
		if v.ExpectedRevision != record.Revision() || record.State != trip.InProgress && record.State != trip.Paused {
			return app.ErrHandoverConflict
		}
		self, err := person(ctx, queries, org, recorder.MembershipID)
		if err != nil {
			return err
		}
		if self.AccountID != recorder.AccountID || self.Employees[0] != recorder.EmployeeID {
			return app.ErrUnavailable
		}
		currentMember, currentEmployee := record.Responsible()
		current, err := person(ctx, queries, org, currentMember, currentEmployee)
		if err != nil {
			return err
		}
		// An unknown or another organization's membership has no row here
		// (forced RLS) and is answered like any ineligible receiver.
		var receiver *app.Person
		row, err := queries.TripHandoverReceiver(ctx, q.TripHandoverReceiverParams{OrgID: org, MembershipID: v.ReceiverMembershipID})
		switch {
		case err == nil:
			receiver = &app.Person{AccountID: row.AccountID, MembershipID: v.ReceiverMembershipID, Employees: []string{row.EmployeeID, row.CanonicalEmployeeID}}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		// B-BR-EMP-397: the recorder is neither the current responsible
		// member nor the receiver.
		if samePerson(self, current) || receiver != nil && samePerson(self, *receiver) {
			return app.ErrSelfHandover
		}
		if receiver == nil || row.MembershipState != "ACTIVE" || row.EmployeeState != "ACTIVE" || !row.Unmerged || !canonicalID(row.CanonicalEmployeeID) || samePerson(*receiver, current) {
			return app.ErrReceiverIneligible
		}
		now, err := queries.TripHandoverNow(ctx)
		at, ok := exact(now, 0)
		if err != nil || !ok {
			return app.ErrUnavailable
		}
		eligible, err := s.Authority.EligibleReceiverTx(ctx, tx, scope, v.ReceiverMembershipID,
			app.Target{TripID: record.ID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID}, at)
		if err != nil {
			return err
		}
		if !eligible {
			return app.ErrReceiverIneligible
		}
		if _, err := app.HandoverDomain(scope, record, recorder.MembershipID, v.ReceiverMembershipID, row.EmployeeID, v.ExpectedRevision, v.Reason, at); err != nil {
			return handoverDomainError(err)
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			if err := q.New(tx).InsertTripHandover(ctx, q.InsertTripHandoverParams{OrgID: org, TripID: v.TripID, Revision: record.Revision() + 1, OperationID: v.OperationID,
				RequestHash: digest, FromMembershipID: currentMember, FromEmployeeID: currentEmployee, ToMembershipID: v.ReceiverMembershipID, ToEmployeeID: row.EmployeeID,
				RecorderMembershipID: recorder.MembershipID, RecorderEmployeeID: recorder.EmployeeID, Reason: v.Reason}); err != nil {
				return err
			}
			result, err = handoverResult(ctx, tx, scope, v.TripID, v.OperationID)
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
		return app.HandoverResult{}, false, reviewFailure(ctx, err)
	}
	return result, replay, nil
}

// ListHandovers returns the organization's handovers in recording order
// after one handover operation (EDR-255).
func (s *HandoverStore) ListHandovers(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, filter app.QueueFilter) (app.HandoverPage, error) {
	if !s.ready() {
		return app.HandoverPage{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.HandoverPage{}, app.ErrReviewUnauthorized
	}
	if err := filter.Validate(); err != nil {
		return app.HandoverPage{}, err
	}
	org := scope.OrgID().String()
	var page app.HandoverPage
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reader, err := s.Authority.CheckTripHandoverTx(ctx, tx, scope, actor, member, org, app.HandoverReadAction)
		if err != nil {
			return err
		}
		rows, err := q.New(tx).OrganizationTripHandovers(ctx, q.OrganizationTripHandoversParams{OrgID: org, AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		page = app.HandoverPage{Items: make([]app.HandoverItem, 0, min(len(rows), filter.Limit))}
		for i, row := range rows {
			if i == filter.Limit {
				page.NextID = page.Items[len(page.Items)-1].Handover.OperationID
				break
			}
			records, err := handoverRecords([]q.ListTripHandoversRow{{Revision: row.Revision, OperationID: row.OperationID, RequestHash: row.RequestHash,
				FromMembershipID: row.FromMembershipID, FromEmployeeID: row.FromEmployeeID, ToMembershipID: row.ToMembershipID, ToEmployeeID: row.ToEmployeeID,
				RecorderMembershipID: row.RecorderMembershipID, RecorderEmployeeID: row.RecorderEmployeeID, Reason: row.Reason, HandedOverAt: row.HandedOverAt}})
			if err != nil {
				return err
			}
			page.Items = append(page.Items, app.HandoverItem{TripID: row.TripID, AssetID: row.AssetID, Handover: records[0]})
		}
		again, err := s.Authority.CheckTripHandoverTx(ctx, tx, scope, actor, member, org, app.HandoverReadAction)
		if err != nil {
			return err
		}
		if again != reader {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.HandoverPage{}, reviewFailure(ctx, err)
	}
	return page, nil
}

// ListOwnTrips returns the open trips the member is currently responsible
// for, by trip identity, with the latest handover to it (B-BR-EMP-398).
func (s *HandoverStore) ListOwnTrips(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, filter app.QueueFilter) (app.OwnTripPage, error) {
	if !s.ready() {
		return app.OwnTripPage{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.OwnTripPage{}, app.ErrReviewUnauthorized
	}
	if err := filter.Validate(); err != nil {
		return app.OwnTripPage{}, err
	}
	org := scope.OrgID().String()
	var page app.OwnTripPage
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reader, err := s.Authority.CheckTripHandoverTx(ctx, tx, scope, actor, member, member, app.OwnTripsAction)
		if err != nil {
			return err
		}
		queries := q.New(tx)
		rows, err := queries.OwnOpenTrips(ctx, q.OwnOpenTripsParams{OrgID: org, MembershipID: member, AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		page = app.OwnTripPage{Items: make([]app.OwnTrip, 0, min(len(rows), filter.Limit))}
		for i, row := range rows {
			if i == filter.Limit {
				page.NextID = page.Items[len(page.Items)-1].TripID
				break
			}
			started, ok := exact(row.OccurredAt, row.OccurredSubmicro)
			state, err := trip.ParseState(row.State)
			if !ok || err != nil {
				return app.ErrUnavailable
			}
			item := app.OwnTrip{TripID: row.ID, AssetID: row.AssetID, ReportingDay: row.ReportingDay, StartedAt: started, Revision: row.Revision, State: state}
			handovers, err := queries.ListTripHandovers(ctx, q.ListTripHandoversParams{OrgID: org, TripID: row.ID})
			if err != nil {
				return err
			}
			records, err := handoverRecords(handovers)
			if err != nil {
				return err
			}
			if n := len(records); n > 0 && records[n-1].ToMembershipID == member {
				item.Handover = &records[n-1]
			}
			page.Items = append(page.Items, item)
		}
		again, err := s.Authority.CheckTripHandoverTx(ctx, tx, scope, actor, member, member, app.OwnTripsAction)
		if err != nil {
			return err
		}
		if again != reader {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.OwnTripPage{}, reviewFailure(ctx, err)
	}
	return page, nil
}
