package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// EMP-TRIP-015 amendments of approved trips (B-BR-EMP-365..368, EDR-236/237).

// AmendmentReadings supersedes an approved amendment's readings inside the
// decision transaction through the METER-004 path (fleet MeterStore).
type AmendmentReadings interface {
	SupersedeApprovedTx(context.Context, pgx.Tx, kernel.OrgScope, fleetapp.FleetActor, string, fleetapp.AmendmentSupersession) error
}

// AmendAuthority proves meter.reading.correct over the stored trip with the
// real company proof (TripStructure) and returns the amendment's author.
type AmendAuthority interface {
	CheckTripAmendTx(context.Context, pgx.Tx, kernel.OrgScope, app.ReviewActor, string, string) (app.Reviewer, error)
}

func correctionRecord(meterID, readingID, newID string, from, to pgtype.Int8) (*app.CorrectionRecord, error) {
	if readingID == "" && meterID == "" && newID == "" && !from.Valid && !to.Valid {
		return nil, nil
	}
	if !canonicalID(meterID) || !canonicalID(readingID) || !canonicalID(newID) || !from.Valid || !to.Valid || from.Int64 == to.Int64 {
		return nil, app.ErrUnavailable
	}
	return &app.CorrectionRecord{MeterID: meterID, ReadingID: readingID, NewReadingID: newID, From: from.Int64, To: to.Int64}, nil
}

func amendmentRecords(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, tripID string) ([]app.AmendmentRecord, error) {
	rows, err := queries.TripAmendments(ctx, q.TripAmendmentsParams{OrgID: scope.OrgID().String(), TripID: tripID})
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]app.AmendmentRecord, 0, len(rows))
	for _, row := range rows {
		start, err := correctionRecord(row.StartMeterID, row.StartReadingID, row.StartNewReadingID, row.StartFrom, row.StartTo)
		if err != nil {
			return nil, err
		}
		end, err := correctionRecord(row.EndMeterID, row.EndReadingID, row.EndNewReadingID, row.EndFrom, row.EndTo)
		if err != nil {
			return nil, err
		}
		at, ok := exact(row.RecordedAt, 0)
		intent := amend.Intent(row.Intent)
		if !ok || start == nil && end == nil || intent != amend.IntentAmend && intent != amend.IntentEdit {
			return nil, app.ErrUnavailable
		}
		out = append(out, app.AmendmentRecord{Revision: row.Revision, Base: row.BaseRevision, Intent: intent, Start: start, End: end, ActorMembershipID: row.ActorMembershipID,
			ActorEmployeeID: row.ActorEmployeeID, OperationID: row.OperationID, RequestHash: row.RequestHash, RecordedAt: at})
	}
	return out, nil
}

// amendmentPerson is the amendment author of the current segment.
func amendmentPerson(ctx context.Context, queries *q.Queries, org string, r app.Revisions) (*app.Person, error) {
	e, ok := r.Amendment()
	if !ok {
		return nil, nil
	}
	p, err := person(ctx, queries, org, e.ActorMembershipID, e.ActorEmployeeID)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// pointCorrection checks one correction against the point's current reading
// of the effective revision and returns the stored correction.
func pointCorrection(c *app.Correction, meterID, readingID, kind string, value int64, newID string) (*app.CorrectionRecord, error) {
	if c == nil {
		return nil, nil
	}
	if !canonicalID(meterID) || !canonicalID(readingID) || string(c.Kind) != kind || c.Value == value {
		return nil, app.ErrInvalidAmendment
	}
	return &app.CorrectionRecord{MeterID: meterID, ReadingID: readingID, NewReadingID: newID, From: value, To: c.Value}, nil
}

func correctionParams(c *app.CorrectionRecord) (pgtype.Text, pgtype.Text, pgtype.Int8, pgtype.Int8, pgtype.Text) {
	if c == nil {
		return pgtype.Text{}, pgtype.Text{}, pgtype.Int8{}, pgtype.Int8{}, pgtype.Text{}
	}
	return id(c.MeterID), id(c.ReadingID), pgtype.Int8{Int64: c.From, Valid: true}, pgtype.Int8{Int64: c.To, Valid: true}, id(c.NewReadingID)
}

// ProposeTripAmendment records one amendment proposal (B-BR-EMP-365):
// authority, the trip row lock, replay by operation, the period of the
// stored start day, the orgamend kernel over the stored revisions, the
// current readings, then the amendment row, the AMEND entry and one audit
// event, and authority again.
func (s *ReviewStore) ProposeTripAmendment(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, command app.AmendmentCommand, newIDs [2]string) (app.AmendmentResult, bool, error) {
	if !s.ready() || absent(s.Amenders) {
		return app.AmendmentResult{}, false, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.AmendmentResult{}, false, app.ErrReviewUnauthorized
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.AmendmentResult{}, false, err
	}
	digest := command.Digest(scope)
	if digest == "" || !canonicalID(newIDs[0]) || !canonicalID(newIDs[1]) || newIDs[0] == newIDs[1] {
		return app.AmendmentResult{}, false, app.ErrUnavailable
	}
	bound := audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: app.AmendAuditAction, TargetType: ReviewTargetType, TargetID: v.TripID, RequestHash: digest}
	org := scope.OrgID().String()
	var result app.AmendmentResult
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		author, err := s.Amenders.CheckTripAmendTx(ctx, tx, scope, actor, member, v.TripID)
		if err != nil {
			return err
		}
		recheck := func() error {
			again, err := s.Amenders.CheckTripAmendTx(ctx, tx, scope, actor, member, v.TripID)
			if err != nil {
				return err
			}
			if again != author {
				return app.ErrUnavailable
			}
			return nil
		}
		queries := q.New(tx)
		if _, err := queries.LockTripForReview(ctx, q.LockTripForReviewParams{OrgID: org, ID: v.TripID}); err != nil {
			return err
		}
		if prior, err := queries.TripAmendmentByOperation(ctx, q.TripAmendmentByOperationParams{OrgID: org, OperationID: v.OperationID}); err == nil {
			repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(context.Context, pgx.Tx) error { return app.ErrUnavailable })
			if err != nil {
				return err
			}
			if !repeated || prior.TripID != v.TripID || prior.RequestHash != digest || prior.ActorMembershipID != author.MembershipID {
				return app.ErrOperationReused
			}
			if result, err = amendmentResult(ctx, queries, scope, v.TripID, prior.Revision); err != nil {
				return err
			}
			replay = true
			return recheck()
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		facts, err := queries.TripReviewFacts(ctx, q.TripReviewFactsParams{OrgID: org, ID: v.TripID})
		if err != nil {
			return err
		}
		entries, err := reviewEntries(ctx, queries, scope, v.TripID)
		if err != nil {
			return err
		}
		revisions, err := app.TripRevisions(entries)
		if err != nil {
			return err
		}
		fact, err := s.Periods.StateForDayTx(ctx, tx, scope, facts.ReportingDay)
		if err != nil || fact.Day != facts.ReportingDay {
			return app.ErrUnavailable
		}
		p, err := fact.Amendment()
		if err != nil {
			return app.ErrUnavailable
		}
		decision, err := app.ProposeAmendment(revisions, v.TripID, facts.ReportingDay, p, command, actor.AccountID, fact.CheckedAt)
		if err != nil {
			return err
		}
		points, err := queries.TripPointReadings(ctx, q.TripPointReadingsParams{OrgID: org, TripID: v.TripID})
		if err != nil {
			return err
		}
		start, err := pointCorrection(v.Start, points.StartMeterID, points.StartReadingID, points.StartKind, points.StartValue, newIDs[0])
		if err != nil {
			return err
		}
		end, err := pointCorrection(v.End, points.EndMeterID, points.EndReadingID, points.EndKind, points.EndValue, newIDs[1])
		if err != nil {
			return err
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			queries := q.New(tx)
			sm, sr, sf, st, sn := correctionParams(start)
			em, er, ef, et, en := correctionParams(end)
			if err := queries.InsertTripAmendment(ctx, q.InsertTripAmendmentParams{OrgID: org, TripID: v.TripID, Revision: decision.Revision, BaseRevision: decision.Base,
				Intent: string(intentOf(decision)), StartMeterID: sm, StartReadingID: sr, StartFrom: sf, StartTo: st, StartNewReadingID: sn,
				EndMeterID: em, EndReadingID: er, EndFrom: ef, EndTo: et, EndNewReadingID: en, ActorMembershipID: author.MembershipID, ActorEmployeeID: author.EmployeeID,
				OperationID: v.OperationID, RequestHash: digest}); err != nil {
				return err
			}
			if err := queries.InsertTripReviewEntry(ctx, q.InsertTripReviewEntryParams{OrgID: org, TripID: v.TripID, Sequence: int64(len(entries) + 1), Kind: string(app.Amend),
				Revision: decision.Revision, State: string(review.Submitted), Reason: decision.Reason, ActorMembershipID: author.MembershipID, ActorEmployeeID: author.EmployeeID,
				OperationID: v.OperationID}); err != nil {
				return err
			}
			var err error
			result, err = amendmentResult(ctx, queries, scope, v.TripID, decision.Revision)
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
		return app.AmendmentResult{}, false, reviewFailure(ctx, err)
	}
	return result, replay, nil
}

func intentOf(d amend.Decision) amend.Intent {
	if d.Outcome == amend.OutcomeEdit {
		return amend.IntentEdit
	}
	return amend.IntentAmend
}

// amendmentAt is the stored amendment of one revision.
func amendmentAt(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, tripID string, revision int64) (app.AmendmentRecord, error) {
	records, err := amendmentRecords(ctx, queries, scope, tripID)
	if err != nil {
		return app.AmendmentRecord{}, err
	}
	for _, r := range records {
		if r.Revision == revision {
			return r, nil
		}
	}
	return app.AmendmentRecord{}, app.ErrUnavailable
}

// amendmentResult rebuilds a proposal's answer from its stored amendment
// and AMEND entry, so a replay returns exactly the first result.
func amendmentResult(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, tripID string, revision int64) (app.AmendmentResult, error) {
	record, err := amendmentAt(ctx, queries, scope, tripID, revision)
	if err != nil {
		return app.AmendmentResult{}, err
	}
	entries, err := reviewEntries(ctx, queries, scope, tripID)
	if err != nil {
		return app.AmendmentResult{}, err
	}
	for _, e := range entries {
		if e.Kind == app.Amend && e.Revision == revision {
			if e.OperationID != record.OperationID || e.State != review.Submitted {
				return app.AmendmentResult{}, app.ErrUnavailable
			}
			return app.AmendmentResult{TripID: tripID, Revision: revision, Base: record.Base, Intent: record.Intent, State: e.State, RecordedAt: record.RecordedAt}, nil
		}
	}
	return app.AmendmentResult{}, app.ErrUnavailable
}

// revisionUsage completes a decision context with the effective revision's
// usage and every amendment with its state, reason, author and usage.
func revisionUsage(ctx context.Context, tx pgx.Tx, queries *q.Queries, scope kernel.OrgScope, tripID string, entries []app.ReviewEntry, r app.Revisions, view *app.ReviewView) error {
	record, err := (&Store{}).ReadTripTx(ctx, tx, scope, tripID)
	if err != nil {
		return app.ErrUnavailable
	}
	amendments, err := amendmentRecords(ctx, queries, scope, tripID)
	if err != nil {
		return err
	}
	if effective := r.Effective(); effective > 0 {
		start, end := app.Values(record, amendments, r, effective)
		usage, err := app.UsageOf(scope, record, effective, start, end)
		if err != nil {
			return err
		}
		view.EffectiveUsage = &usage
	}
	opened := map[int64]app.ReviewEntry{}
	for _, e := range entries {
		if e.Kind == app.Amend {
			opened[e.Revision] = e
		}
	}
	for _, a := range amendments {
		e, ok := opened[a.Revision]
		state, known := r.RevisionState(a.Revision)
		if !ok || !known || e.OperationID != a.OperationID || e.ActorMembershipID != a.ActorMembershipID {
			return app.ErrUnavailable
		}
		start, end := app.Values(record, amendments, r, a.Revision)
		usage, err := app.UsageOf(scope, record, a.Revision, start, end)
		if err != nil {
			return err
		}
		view.Amendments = append(view.Amendments, app.AmendmentView{Revision: a.Revision, Base: a.Base, Intent: a.Intent, State: state, Reason: e.Reason,
			AuthorMembershipID: a.ActorMembershipID, RecordedAt: a.RecordedAt, Start: a.Start, End: a.End, Usage: usage})
	}
	if len(view.Amendments) != len(opened) {
		return app.ErrUnavailable
	}
	return nil
}
