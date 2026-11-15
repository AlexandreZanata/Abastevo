package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	reviewapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EMP-TRIP-004 review persistence (EDR-221, EDR-222).
const ReviewTargetType = "trip"

var reviewActions = map[review.Kind]string{review.Approve: "trip.trip.approved", review.Return: "trip.trip.returned", review.Reject: "trip.trip.rejected"}

// ReviewAuditDefinitions are the decision audit actions on target type trip;
// they carry the command digest and no reason text (BR-AUD-004).
func ReviewAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: reviewActions[review.Approve], TargetType: ReviewTargetType}, {Action: reviewActions[review.Return], TargetType: ReviewTargetType},
		{Action: reviewActions[review.Reject], TargetType: ReviewTargetType}}
}

var _ app.SubmitStore = (*Store)(nil)

func entryOf(sequence, revision int64, kind, state, reason, member, employee, operation string, recorded pgtype.Timestamptz) (app.ReviewEntry, error) {
	k, err := review.ParseKind(kind)
	if err != nil {
		return app.ReviewEntry{}, app.ErrUnavailable
	}
	s, err := review.ParseState(state)
	if err != nil {
		return app.ReviewEntry{}, app.ErrUnavailable
	}
	at, ok := exact(recorded, 0)
	if !ok {
		return app.ReviewEntry{}, app.ErrUnavailable
	}
	return app.ReviewEntry{Sequence: sequence, Kind: k, Revision: revision, State: s, Reason: reason, ActorMembershipID: member, ActorEmployeeID: employee, OperationID: operation, RecordedAt: at}, nil
}

// LockTripTx takes the trip row lock that serializes its review entries and
// returns the operational state; a trip not stored here is ErrNotFound.
func (s *Store) LockTripTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, tripID string) (trip.State, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return "", err
	}
	state, err := queries.LockTripForReview(ctx, q.LockTripForReviewParams{OrgID: scope.OrgID().String(), ID: tripID})
	if err != nil {
		return "", storeError(err)
	}
	parsed, err := trip.ParseState(state)
	if err != nil {
		return "", app.ErrUnavailable
	}
	return parsed, nil
}

// ReviewEntriesTx reads one trip's history in sequence order.
func (s *Store) ReviewEntriesTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, tripID string) ([]app.ReviewEntry, error) {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return nil, err
	}
	return reviewEntries(ctx, queries, scope, tripID)
}

func reviewEntries(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, tripID string) ([]app.ReviewEntry, error) {
	rows, err := queries.TripReviewEntries(ctx, q.TripReviewEntriesParams{OrgID: scope.OrgID().String(), TripID: tripID})
	if err != nil {
		return nil, storeError(err)
	}
	entries := make([]app.ReviewEntry, 0, len(rows))
	for _, row := range rows {
		e, err := entryOf(row.Sequence, row.Revision, row.Kind, row.State, row.Reason, row.ActorMembershipID, row.ActorEmployeeID, row.OperationID, row.RecordedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// InsertSubmitTx appends the author's SUBMIT entry (B-BR-EMP-342); the
// database re-proves the transition, the author and, at commit, the receipt.
func (s *Store) InsertSubmitTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command app.SubmitCommand, authorEmployee string) error {
	queries, _, err := s.port(ctx, db, scope)
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	if err != nil || source.MembershipID != v.AuthorMembershipID || !canonicalID(authorEmployee) {
		return app.ErrUnavailable
	}
	if err := queries.InsertTripReviewEntry(ctx, q.InsertTripReviewEntryParams{OrgID: scope.OrgID().String(), TripID: v.TripID, Sequence: 1, Kind: string(review.Submit),
		Revision: v.Revision, State: string(review.Submitted), Reason: "", ActorMembershipID: source.MembershipID, ActorEmployeeID: authorEmployee, OperationID: v.OperationID}); err != nil {
		return storeError(err)
	}
	return nil
}

// ReviewAuthority proves the reviewer's current company authority over one
// trip inside the owning transaction (EMP-AUTH-006 trip structure).
type ReviewAuthority interface {
	CheckTripReviewTx(context.Context, pgx.Tx, kernel.OrgScope, app.ReviewActor, string, string, bool) (app.Reviewer, error)
}

// ReviewStore is the company-session review owner (EDR-222). Periods is the
// APPR-006 gate read inside the decision transaction (EMP-TRIP-016,
// EDR-232).
type ReviewStore struct {
	Pool      *pgxpool.Pool
	Authority ReviewAuthority
	Audit     auditing.TransactionRunner
	Periods   reviewapp.PeriodGate
}

var _ app.ReviewRepository = (*ReviewStore)(nil)

func (s *ReviewStore) ready() bool {
	return s != nil && s.Pool != nil && !absent(s.Authority) && !absent(s.Audit) && !absent(s.Periods)
}

// reviewFailure keeps domain, authority and application refusals and turns
// storage failures into not-found or unavailable answers.
func reviewFailure(ctx context.Context, err error) error {
	var pg *pgconn.PgError
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, audit.ErrReused):
		return app.ErrOperationReused
	case errors.Is(err, pgx.ErrNoRows):
		return app.ErrNotFound
	case errors.As(err, &pg):
		return app.ErrUnavailable
	}
	return err
}

// person reads one membership's account and the employee records of its
// merge chain; an unresolvable chain fails closed.
func person(ctx context.Context, queries *q.Queries, org, membership string, employees ...string) (app.Person, error) {
	row, err := queries.TripReviewPerson(ctx, q.TripReviewPersonParams{OrgID: org, MembershipID: membership})
	if err != nil || !canonicalID(row.CanonicalEmployeeID) {
		return app.Person{}, app.ErrUnavailable
	}
	out := app.Person{AccountID: row.AccountID, MembershipID: membership, Employees: []string{row.EmployeeID, row.CanonicalEmployeeID}}
	for _, e := range employees {
		chain, err := employee(ctx, queries, org, e)
		if err != nil {
			return app.Person{}, err
		}
		out.Employees = append(out.Employees, chain.Employees...)
	}
	return out, nil
}

func employee(ctx context.Context, queries *q.Queries, org, id string) (app.Person, error) {
	canonical, err := queries.TripReviewEmployee(ctx, q.TripReviewEmployeeParams{OrgID: org, ID: id})
	if err != nil || !canonicalID(canonical) {
		return app.Person{}, app.ErrUnavailable
	}
	return app.Person{Employees: []string{id, canonical}}, nil
}

// decisionOf rebuilds the result of one operation from its stored entries.
func decisionOf(rows []q.TripReviewOperationEntriesRow, v app.DecisionInput) (app.DecisionResult, error) {
	for _, row := range rows {
		if row.TripID != v.TripID || row.Kind != string(v.Decision) {
			continue
		}
		e, err := entryOf(row.Sequence, row.Revision, row.Kind, row.State, row.Reason, row.ActorMembershipID, row.ActorEmployeeID, v.OperationID, row.RecordedAt)
		if err != nil {
			return app.DecisionResult{}, err
		}
		return app.DecisionResult{TripID: row.TripID, Decision: e.Kind, State: e.State, Revision: e.Revision, Reason: e.Reason, RecordedAt: e.RecordedAt}, nil
	}
	return app.DecisionResult{}, app.ErrOperationReused
}

// DecideTripReview applies one decision (B-BR-EMP-344/345): authority, the
// trip row lock, replay by operation, the APPR-001 lifecycle over the stored
// history, then the entries and one audit event, and authority again.
func (s *ReviewStore) DecideTripReview(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, command app.DecisionCommand) (app.DecisionResult, bool, error) {
	if !s.ready() {
		return app.DecisionResult{}, false, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.DecisionResult{}, false, app.ErrReviewUnauthorized
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.DecisionResult{}, false, err
	}
	digest := command.Digest(scope)
	if digest == "" {
		return app.DecisionResult{}, false, app.ErrUnavailable
	}
	bound := audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: reviewActions[v.Decision], TargetType: ReviewTargetType, TargetID: v.TripID, RequestHash: digest}
	org := scope.OrgID().String()
	var result app.DecisionResult
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reviewer, err := s.Authority.CheckTripReviewTx(ctx, tx, scope, actor, member, v.TripID, true)
		if err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.LockTripForReview(ctx, q.LockTripForReviewParams{OrgID: org, ID: v.TripID}); err != nil {
			return err
		}
		recheck := func() error {
			again, err := s.Authority.CheckTripReviewTx(ctx, tx, scope, actor, member, v.TripID, true)
			if err != nil {
				return err
			}
			if again != reviewer {
				return app.ErrUnavailable
			}
			return nil
		}
		used, err := queries.TripReviewOperationEntries(ctx, q.TripReviewOperationEntriesParams{OrgID: org, OperationID: v.OperationID})
		if err != nil {
			return err
		}
		if len(used) > 0 {
			repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(context.Context, pgx.Tx) error { return app.ErrUnavailable })
			if err != nil {
				return err
			}
			if !repeated {
				return app.ErrUnavailable
			}
			if result, err = decisionOf(used, v); err != nil {
				return err
			}
			replay = true
			return recheck()
		}
		facts, err := queries.TripReviewFacts(ctx, q.TripReviewFactsParams{OrgID: org, ID: v.TripID})
		if err != nil {
			return err
		}
		entries, err := reviewEntries(ctx, queries, scope, v.TripID)
		if err != nil {
			return err
		}
		current, err := app.ReviewCase(entries)
		if err != nil {
			return err
		}
		self, err := person(ctx, queries, org, reviewer.MembershipID)
		if err != nil {
			return err
		}
		if self.AccountID != reviewer.AccountID || self.Employees[0] != reviewer.EmployeeID {
			return app.ErrUnavailable
		}
		author, err := person(ctx, queries, org, facts.AuthorMembershipID, facts.AuthorEmployeeID)
		if err != nil {
			return err
		}
		driver, err := employee(ctx, queries, org, facts.DriverEmployeeID)
		if err != nil {
			return err
		}
		next, err := app.Decide(current, v, self, author, driver)
		if err != nil {
			return err
		}
		// B-BR-EMP-359: the period of the stored start reporting day, read
		// under the root lock the authority already holds (EDR-232).
		fact, err := s.Periods.StateForDayTx(ctx, tx, scope, facts.ReportingDay)
		if err != nil {
			return app.ErrUnavailable
		}
		if err := app.DecisionPeriod(facts.ReportingDay, fact); err != nil {
			return err
		}
		added := next.History()[len(entries):]
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			queries := q.New(tx)
			for i, e := range added {
				if err := queries.InsertTripReviewEntry(ctx, q.InsertTripReviewEntryParams{OrgID: org, TripID: v.TripID, Sequence: int64(len(entries) + i + 1), Kind: string(e.Kind),
					Revision: e.Revision, State: string(e.State), Reason: e.Reason, ActorMembershipID: reviewer.MembershipID, ActorEmployeeID: reviewer.EmployeeID, OperationID: v.OperationID}); err != nil {
					return err
				}
			}
			rows, err := queries.TripReviewOperationEntries(ctx, q.TripReviewOperationEntriesParams{OrgID: org, OperationID: v.OperationID})
			if err != nil {
				return err
			}
			result, err = decisionOf(rows, v)
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
		return app.DecisionResult{}, false, reviewFailure(ctx, err)
	}
	return result, replay, nil
}

// ReadTripReview returns the minimal decision context (B-BR-EMP-346).
func (s *ReviewStore) ReadTripReview(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member, tripID string) (app.ReviewView, error) {
	if !s.ready() {
		return app.ReviewView{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.ReviewView{}, app.ErrReviewUnauthorized
	}
	if !canonicalID(tripID) {
		return app.ReviewView{}, app.ErrNotFound
	}
	org := scope.OrgID().String()
	var view app.ReviewView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reviewer, err := s.Authority.CheckTripReviewTx(ctx, tx, scope, actor, member, tripID, false)
		if err != nil {
			return err
		}
		queries := q.New(tx)
		facts, err := queries.TripReviewFacts(ctx, q.TripReviewFactsParams{OrgID: org, ID: tripID})
		if err != nil {
			return err
		}
		entries, err := reviewEntries(ctx, queries, scope, tripID)
		if err != nil {
			return err
		}
		current, err := app.ReviewCase(entries)
		if err != nil {
			return err
		}
		operational, err := trip.ParseState(facts.State)
		started, ok := exact(facts.OccurredAt, facts.OccurredSubmicro)
		if err != nil || !ok || facts.FinishedAt.Valid != facts.FinishedSubmicro.Valid {
			return app.ErrUnavailable
		}
		view = app.ReviewView{TripID: facts.ID, AssetID: facts.AssetID, DriverEmployeeID: facts.DriverEmployeeID, ReportingDay: facts.ReportingDay, Operational: operational,
			StartedAt: started, Review: trip.ReviewState(current.State()), Revision: current.Revision(), History: entries}
		if facts.FinishedAt.Valid {
			finished, ok := exact(facts.FinishedAt, facts.FinishedSubmicro.Int16)
			if !ok {
				return app.ErrUnavailable
			}
			view.FinishedAt = &finished
		}
		again, err := s.Authority.CheckTripReviewTx(ctx, tx, scope, actor, member, tripID, false)
		if err != nil {
			return err
		}
		if again != reviewer {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.ReviewView{}, reviewFailure(ctx, err)
	}
	return view.Clone(), nil
}

// ListTripReviews returns the organization queue of trips awaiting a
// decision, in submission order, after one trip (EDR-222).
func (s *ReviewStore) ListTripReviews(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, filter app.QueueFilter) (app.QueuePage, error) {
	if !s.ready() {
		return app.QueuePage{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.QueuePage{}, app.ErrReviewUnauthorized
	}
	if err := filter.Validate(); err != nil {
		return app.QueuePage{}, err
	}
	org := scope.OrgID().String()
	var page app.QueuePage
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reviewer, err := s.Authority.CheckTripReviewTx(ctx, tx, scope, actor, member, org, false)
		if err != nil {
			return err
		}
		rows, err := q.New(tx).TripReviewQueue(ctx, q.TripReviewQueueParams{OrgID: org, AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		page = app.QueuePage{Items: make([]app.QueueItem, 0, min(len(rows), filter.Limit))}
		for i, row := range rows {
			if i == filter.Limit {
				page.NextID = page.Items[len(page.Items)-1].TripID
				break
			}
			state, err := review.ParseState(row.State)
			submitted, ok := exact(row.SubmittedAt, 0)
			if err != nil || !ok {
				return app.ErrUnavailable
			}
			page.Items = append(page.Items, app.QueueItem{TripID: row.ID, AssetID: row.AssetID, DriverEmployeeID: row.DriverEmployeeID, ReportingDay: row.ReportingDay,
				Review: trip.ReviewState(state), Revision: row.Revision, SubmittedAt: submitted})
		}
		again, err := s.Authority.CheckTripReviewTx(ctx, tx, scope, actor, member, org, false)
		if err != nil {
			return err
		}
		if again != reviewer {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.QueuePage{}, reviewFailure(ctx, err)
	}
	return page, nil
}
