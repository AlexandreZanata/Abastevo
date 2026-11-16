package adapters

import (
	"bytes"
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EMP-TRIP-007 conflict queue and resolution (B-BR-EMP-407..411, EDR-262..264).

// ConflictAuditDefinitions is the OFFL-008 resolution event: it carries the
// command digest, never the reason (BR-AUD-004).
func ConflictAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: conflictAuditAction, TargetType: conflictAuditTarget}}
}

const (
	conflictAuditAction = "offl.review.resolved"
	conflictAuditTarget = "offl.review"
)

// ConflictAuthority proves trip.trip.review over a held trip item, or the
// organization root for the queue, inside the owning transaction (EDR-263).
type ConflictAuthority interface {
	CheckTripConflictTx(context.Context, pgx.Tx, kernel.OrgScope, app.ReviewActor, string, string, bool) (app.Reviewer, error)
}

// ConflictStore is the company-session conflict owner. Allocations is the
// FLEET-005 snapshot of an accepted start's declared instant; Readings
// stores its reviewed TRIP_START reading at the original receipt.
type ConflictStore struct {
	Pool        *pgxpool.Pool
	Authority   ConflictAuthority
	Audit       auditing.TransactionRunner
	Allocations fleetapp.AllocationSnapshotPort
	Readings    fleetapp.ReviewedReadingRepository
}

var _ app.ConflictRepository = (*ConflictStore)(nil)

func (s *ConflictStore) ready() bool {
	return s != nil && s.Pool != nil && !absent(s.Authority) && !absent(s.Audit) && !absent(s.Allocations) && !absent(s.Readings)
}

// conflictFailure keeps domain, authority and application refusals and
// turns storage failures into not-found or unavailable answers.
func conflictFailure(ctx context.Context, err error) error {
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

func conflictDeclaration(at pgtype.Timestamptz, sub int16, zone string, offset int64) (orgtime.Declaration, bool) {
	occurred, ok := exact(at, sub)
	return orgtime.Declaration{OccurredAt: occurred, EntryZone: zone, DeviceOffsetSeconds: offset}, ok
}

func conflictTrip(id, state string, at pgtype.Timestamptz, sub int16, finished pgtype.Timestamptz, finishedSub int16) (app.ConflictTrip, error) {
	parsed, err := trip.ParseState(state)
	started, ok := exact(at, sub)
	if err != nil || !ok {
		return app.ConflictTrip{}, app.ErrUnavailable
	}
	out := app.ConflictTrip{TripID: id, State: parsed, StartedAt: started}
	if finished.Valid {
		end, ok := exact(finished, finishedSub)
		if !ok {
			return app.ConflictTrip{}, app.ErrUnavailable
		}
		out.FinishedAt = &end
	}
	return out, nil
}

// heldReading builds the held start or finish fact's shared part.
func heldReading(tripID string, declared orgtime.Declaration, boot pgtype.Text, elapsed pgtype.Int8, meterID, segmentID, readingID, kind string, value pgtype.Int8, reason pgtype.Text) (*app.HeldStartFact, bool) {
	clock, ok := clockOf(boot, elapsed)
	if !ok || value.Valid != (readingID != "") || (readingID != "") == reason.Valid {
		return nil, false
	}
	fact := &app.HeldStartFact{TripID: tripID, Declared: declared, Clock: clock, MeterID: meterID, SegmentID: segmentID, ReadingID: readingID, ReadingKind: kind, MissingReason: optionalText(reason)}
	if value.Valid {
		v := value.Int64
		fact.ReadingValue = &v
	}
	return fact, true
}

// conflictView reads one held trip item, its exact held fact, the trips it
// conflicts with and its resolution.
func conflictView(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, id string, now time.Time) (app.ConflictView, error) {
	org := scope.OrgID().String()
	row, err := queries.ReadTripConflictItem(ctx, q.ReadTripConflictItemParams{OrgID: org, ID: id})
	if err != nil {
		return app.ConflictView{}, err
	}
	received, ok := exact(row.ReceivedAt, 0)
	expires, okExpires := exact(row.ExpiresAt, 0)
	if !ok || !okExpires || received.IsZero() {
		return app.ConflictView{}, app.ErrUnavailable
	}
	view := app.ConflictView{ReviewItemID: row.ID, OperationID: row.OperationID, OperationType: row.OperationType, PayloadDigest: row.PayloadDigest, Code: row.Code, State: app.ConflictOpen,
		Revision: 1, AuthorAccountID: row.OwnerAccountID, AuthorMembershipID: row.MembershipID, AuthorEmployeeID: row.AuthorEmployeeID, EnrollmentID: row.EnrollmentID,
		ReceivedAt: received, ExpiresAt: expires, Trips: []app.ConflictTrip{}}
	if !now.Before(expires) {
		view.State = app.ConflictExpired
	}
	named := ""
	switch row.OperationType {
	case app.StartOperationType:
		h, err := queries.ReadTripConflictStart(ctx, q.ReadTripConflictStartParams{OrgID: org, ReviewItemID: id})
		if err != nil {
			return app.ConflictView{}, err
		}
		declared, ok := conflictDeclaration(h.OccurredAt, h.OccurredSubmicro, h.EntryZone, h.DeviceOffsetSeconds)
		start, okFact := heldReading(h.TripID, declared, h.BootID, h.ElapsedMillis, h.MeterID, h.SegmentID, h.ReadingID, h.ReadingKind, h.ReadingValue, h.MissingReadingReason)
		if !ok || !okFact {
			return app.ConflictView{}, app.ErrUnavailable
		}
		start.AssetID, start.DriverID = h.AssetID, h.DriverEmployeeID
		view.Fact = app.ConflictFact{TripID: h.TripID, Declared: declared, Start: start}
		at, sub := instant(declared.OccurredAt)
		rows, err := queries.TripConflictTripsOfAsset(ctx, q.TripConflictTripsOfAssetParams{OrgID: org, AssetID: h.AssetID, TripID: h.TripID, At: at, AtSubmicro: sub})
		if err != nil {
			return app.ConflictView{}, err
		}
		for _, r := range rows {
			t, err := conflictTrip(r.ID, r.State, r.OccurredAt, r.OccurredSubmicro, r.FinishedAt, r.FinishedSubmicro)
			if err != nil {
				return app.ConflictView{}, err
			}
			view.Trips = append(view.Trips, t)
		}
	case app.FinishOperationType:
		h, err := queries.ReadTripConflictFinish(ctx, q.ReadTripConflictFinishParams{OrgID: org, ReviewItemID: id})
		if err != nil {
			return app.ConflictView{}, err
		}
		declared, ok := conflictDeclaration(h.OccurredAt, h.OccurredSubmicro, h.EntryZone, h.DeviceOffsetSeconds)
		finish, okFact := heldReading(h.TripID, declared, h.BootID, h.ElapsedMillis, h.MeterID, h.SegmentID, h.ReadingID, h.ReadingKind, h.ReadingValue, h.MissingReadingReason)
		if !ok || !okFact {
			return app.ConflictView{}, app.ErrUnavailable
		}
		view.Fact, named = app.ConflictFact{TripID: h.TripID, Declared: declared, Finish: finish}, h.TripID
	case app.EventOperationType:
		h, err := queries.ReadTripConflictEvent(ctx, q.ReadTripConflictEventParams{OrgID: org, ReviewItemID: id})
		if err != nil {
			return app.ConflictView{}, err
		}
		declared, ok := conflictDeclaration(h.OccurredAt, h.OccurredSubmicro, h.EntryZone, h.DeviceOffsetSeconds)
		if !ok {
			return app.ConflictView{}, app.ErrUnavailable
		}
		view.Fact = app.ConflictFact{TripID: h.TripID, Declared: declared, Event: &app.HeldEventFact{EventID: h.EventID, EventType: h.EventType, Text: optionalText(h.EventText)}}
		named = h.TripID
	case app.SubmitOperationType:
		h, err := queries.ReadTripConflictSubmission(ctx, q.ReadTripConflictSubmissionParams{OrgID: org, ReviewItemID: id})
		if err != nil {
			return app.ConflictView{}, err
		}
		declared, ok := conflictDeclaration(h.OccurredAt, h.OccurredSubmicro, h.EntryZone, h.DeviceOffsetSeconds)
		if !ok {
			return app.ConflictView{}, app.ErrUnavailable
		}
		revision := h.Revision
		view.Fact, named = app.ConflictFact{TripID: h.TripID, Declared: declared, SubmittedRevision: &revision}, h.TripID
	case app.PauseOperationType, app.ResumeOperationType, app.CancelOperationType:
		// EMP-TRIP-011: a held lifecycle command names its trip.
		h, err := queries.ReadTripConflictTransition(ctx, q.ReadTripConflictTransitionParams{OrgID: org, ReviewItemID: id})
		if err != nil {
			return app.ConflictView{}, err
		}
		declared, ok := conflictDeclaration(h.OccurredAt, h.OccurredSubmicro, h.EntryZone, h.DeviceOffsetSeconds)
		if !ok {
			return app.ConflictView{}, app.ErrUnavailable
		}
		view.Fact = app.ConflictFact{TripID: h.TripID, Declared: declared, Transition: &app.HeldTransitionFact{Command: h.Command, ExpectedRevision: h.ExpectedRevision, Reason: optionalText(h.Reason)}}
		named = h.TripID
	default:
		return app.ConflictView{}, app.ErrNotFound
	}
	if named != "" {
		r, err := queries.TripConflictTrip(ctx, q.TripConflictTripParams{OrgID: org, ID: named})
		if err != nil {
			return app.ConflictView{}, err
		}
		t, err := conflictTrip(r.ID, r.State, r.OccurredAt, r.OccurredSubmicro, r.FinishedAt, r.FinishedSubmicro)
		if err != nil {
			return app.ConflictView{}, err
		}
		view.Trips = append(view.Trips, t)
	}
	if len(row.CanonicalResolution) > 0 {
		resolution, err := protocol.ParseReviewResolution(row.CanonicalResolution)
		if err != nil || !bytes.Equal(resolution.CanonicalBytes(), row.CanonicalResolution) {
			return app.ConflictView{}, app.ErrUnavailable
		}
		v := resolution.Snapshot()
		if v.OperationID != row.OperationID || v.PayloadDigest != row.PayloadDigest || v.ReviewItemID != row.ID || v.Outcome != row.Outcome {
			return app.ConflictView{}, app.ErrUnavailable
		}
		view.Resolution, view.Revision, view.State = &resolution, 2, app.ConflictRefused
		if v.Outcome == protocol.ReviewAccepted {
			view.State = app.ConflictAccepted
		}
	}
	if view.Validate() != nil {
		return app.ConflictView{}, app.ErrUnavailable
	}
	return view, nil
}

func (s *ConflictStore) now(ctx context.Context, queries *q.Queries) (time.Time, error) {
	at, err := queries.TripConflictNow(ctx)
	now, ok := exact(at, 0)
	if err != nil || !ok {
		return time.Time{}, app.ErrUnavailable
	}
	return now, nil
}

func (s *ConflictStore) call(scope kernel.OrgScope, actor app.ReviewActor, member string) error {
	if !s.ready() {
		return app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.ErrReviewUnauthorized
	}
	return nil
}

// ListTripConflicts returns the organization queue (B-BR-EMP-407): only an
// ORG grant covers the organization target.
func (s *ConflictStore) ListTripConflicts(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, filter app.ConflictFilter) (app.ConflictPage, error) {
	if err := s.call(scope, actor, member); err != nil {
		return app.ConflictPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.ConflictPage{}, err
	}
	org := scope.OrgID().String()
	page := app.ConflictPage{Items: []app.ConflictView{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reviewer, err := s.Authority.CheckTripConflictTx(ctx, tx, scope, actor, member, org, false)
		if err != nil {
			return err
		}
		queries := q.New(tx)
		now, err := s.now(ctx, queries)
		if err != nil {
			return err
		}
		ids, err := queries.ListTripConflictIDs(ctx, q.ListTripConflictIDsParams{OrgID: org, AfterID: filter.AfterID, State: filter.State, At: pgtype.Timestamptz{Time: now, Valid: true}, RowLimit: int32(filter.Limit)})
		if err != nil {
			return err
		}
		for _, id := range ids {
			view, err := conflictView(ctx, queries, scope, id, now)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, view)
		}
		if len(ids) == filter.Limit {
			page.NextID = ids[len(ids)-1]
		}
		again, err := s.Authority.CheckTripConflictTx(ctx, tx, scope, actor, member, org, false)
		if err != nil {
			return err
		}
		if again != reviewer {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.ConflictPage{}, conflictFailure(ctx, err)
	}
	return page, nil
}

// ReadTripConflict returns one item to a reviewer covering its structure.
func (s *ConflictStore) ReadTripConflict(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member, id string) (app.ConflictView, error) {
	if err := s.call(scope, actor, member); err != nil {
		return app.ConflictView{}, err
	}
	if !canonicalID(id) {
		return app.ConflictView{}, app.ErrNotFound
	}
	var view app.ConflictView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reviewer, err := s.Authority.CheckTripConflictTx(ctx, tx, scope, actor, member, id, false)
		if err != nil {
			return err
		}
		queries := q.New(tx)
		now, err := s.now(ctx, queries)
		if err != nil {
			return err
		}
		if view, err = conflictView(ctx, queries, scope, id, now); err != nil {
			return err
		}
		again, err := s.Authority.CheckTripConflictTx(ctx, tx, scope, actor, member, id, false)
		if err != nil {
			return err
		}
		if again != reviewer {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.ConflictView{}, conflictFailure(ctx, err)
	}
	return view.Clone(), nil
}

// party is one canonical person for the self-review rule.
func party(ctx context.Context, queries *q.Queries, org, account, membership string, employees ...string) (app.ConflictParty, error) {
	out := app.ConflictParty{AccountID: account, MembershipID: membership}
	for _, e := range employees {
		canonical, err := queries.TripReviewEmployee(ctx, q.TripReviewEmployeeParams{OrgID: org, ID: e})
		if err != nil || !canonicalID(canonical) {
			return app.ConflictParty{}, app.ErrUnavailable
		}
		out.Employees = append(out.Employees, e, canonical)
	}
	return out, nil
}

// ResolveTripConflict applies one decision (B-BR-EMP-408..410): authority,
// the item lock, replay by operation, the revision, self-review, the
// acceptance facts under the asset lock, then the resolution, an accepted
// start's reading, trip and readiness and one audit event, and authority
// again.
func (s *ConflictStore) ResolveTripConflict(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member string, command app.ConflictDecision) (protocol.ReviewResolution, bool, error) {
	if err := s.call(scope, actor, member); err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	digest := command.Digest(scope)
	if digest == "" {
		return protocol.ReviewResolution{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	bound := audit.Event{OperationID: in.OperationID, ActorID: actor.AccountID, Action: conflictAuditAction, TargetType: conflictAuditTarget, TargetID: in.ReviewItemID, RequestHash: digest}
	var result protocol.ReviewResolution
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reviewer, err := s.Authority.CheckTripConflictTx(ctx, tx, scope, actor, member, in.ReviewItemID, true)
		if err != nil {
			return err
		}
		recheck := func(tx pgx.Tx) error {
			again, err := s.Authority.CheckTripConflictTx(ctx, tx, scope, actor, member, in.ReviewItemID, true)
			if err != nil {
				return err
			}
			if again != reviewer {
				return app.ErrUnavailable
			}
			return nil
		}
		queries := q.New(tx)
		if _, err := queries.LockTripConflict(ctx, q.LockTripConflictParams{OrgID: org, ID: in.ReviewItemID}); err != nil {
			return err
		}
		old, err := queries.TripConflictDecisionReceipt(ctx, q.TripConflictDecisionReceiptParams{OrgID: org, ID: in.OperationID})
		if err == nil {
			if old.RequestHash != digest || old.ReviewerAccountID != actor.AccountID || old.ReviewItemID != in.ReviewItemID {
				return app.ErrOperationReused
			}
			if result, err = protocol.ParseReviewResolution(old.CanonicalResolution); err != nil || !bytes.Equal(result.CanonicalBytes(), old.CanonicalResolution) {
				return app.ErrUnavailable
			}
			replay = true
			return recheck(tx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		now, err := s.now(ctx, queries)
		if err != nil {
			return err
		}
		view, err := conflictView(ctx, queries, scope, in.ReviewItemID, now)
		if err != nil {
			return err
		}
		if view.Revision != 1 || view.Resolution != nil || in.ExpectedRevision != view.Revision {
			return review.ErrConflict
		}
		self, err := party(ctx, queries, org, reviewer.AccountID, reviewer.MembershipID, reviewer.EmployeeID)
		if err != nil {
			return err
		}
		author, err := party(ctx, queries, org, view.AuthorAccountID, view.AuthorMembershipID, view.AuthorEmployeeID)
		if err != nil {
			return err
		}
		parties := []app.ConflictParty{author}
		var facts app.ConflictFacts
		zone := ""
		if start := view.Fact.Start; start != nil {
			driver, err := party(ctx, queries, org, "", "", start.DriverID)
			if err != nil {
				return err
			}
			parties = append(parties, driver)
			row, err := queries.TripConflictFacts(ctx, q.TripConflictFactsParams{OrgID: org, AssetID: start.AssetID, OperationID: view.OperationID, AuthorEmployeeID: view.AuthorEmployeeID, DriverID: start.DriverID})
			if err != nil {
				return err
			}
			facts = app.ConflictFacts{AssetActive: row.AssetActive, AuthorCurrent: row.AuthorCurrent, DriverActive: row.DriverActive}
			if row.DisposedAt.Valid {
				at, ok := exact(row.DisposedAt, row.DisposedSubmicro)
				if !ok {
					return app.ErrUnavailable
				}
				facts.DisposedAt = &at
			}
			zone = row.OrganizationZone
		}
		if app.SelfReview(self, parties...) {
			return review.ErrSelfApproval
		}
		if err := app.DecideConflict(view, in, facts); err != nil {
			return err
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			if err := recheck(tx); err != nil {
				return err
			}
			queries := q.New(tx)
			at, err := s.now(ctx, queries)
			if err != nil {
				return err
			}
			value := protocol.ReviewResolutionSnapshot{OperationID: view.OperationID, PayloadDigest: view.PayloadDigest, ReviewItemID: view.ReviewItemID, ResolutionID: in.OperationID,
				ReviewRevision: 2, Outcome: protocol.ReviewRefused, Reason: in.Reason, ResolvedAt: orgtime.FormatInstant(at)}
			params := q.InsertTripConflictResolutionParams{OrgID: org, ID: in.OperationID, ReviewItemID: view.ReviewItemID, ExpectedRevision: in.ExpectedRevision, Outcome: protocol.ReviewRefused,
				Reason: in.Reason, ReviewerAccountID: reviewer.AccountID, ReviewerMembershipID: reviewer.MembershipID, ReviewerEmployeeID: reviewer.EmployeeID, ReviewerSessionID: actor.SessionID,
				RequestHash: digest, ResolvedAt: pgtype.Timestamptz{Time: at, Valid: true}}
			if in.Decision == app.ConflictAccept {
				revision := int64(1)
				value.Outcome, value.EntityRevision = protocol.ReviewAccepted, &revision
				params.Outcome, params.EntityRevision, params.ReadingID = protocol.ReviewAccepted, pgtype.Int8{Int64: 1, Valid: true}, view.Fact.Start.ReadingID
			}
			if result, err = protocol.NewReviewResolution(value); err != nil {
				return app.ErrUnavailable
			}
			params.CanonicalResolution = result.CanonicalBytes()
			if err := queries.InsertTripConflictResolution(ctx, params); err != nil {
				return err
			}
			if in.Decision == app.ConflictAccept {
				if err := s.acceptStart(ctx, tx, scope, view, in.OperationID, zone); err != nil {
					return err
				}
			}
			return recheck(tx)
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return recheck(tx)
	})
	if err != nil {
		return protocol.ReviewResolution{}, false, conflictFailure(ctx, err)
	}
	return result, replay, nil
}

// acceptStart stores an accepted held start as the trip it declared
// (B-BR-EMP-408): its reviewed TRIP_START reading at the original receipt,
// the trip with the FLEET-005 snapshot and reporting day of its declared
// instant, and its readiness effect, so a pending finish proceeds.
func (s *ConflictStore) acceptStart(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, view app.ConflictView, resolution, organizationZone string) error {
	start := view.Fact.Start
	if start == nil || organizationZone == "" {
		return app.ErrUnavailable
	}
	org := scope.OrgID().String()
	queries := q.New(tx)
	if start.ReadingID != "" {
		value, err := meter.NewValue(meter.Kind(start.ReadingKind), *start.ReadingValue, nil)
		if err != nil {
			return app.ErrUnavailable
		}
		command, err := fleetapp.NewMeterReadingCommand(fleetapp.MeterReadingInput{OperationID: view.OperationID, AssetID: start.AssetID, MeterID: start.MeterID, SegmentID: start.SegmentID,
			ReadingID: start.ReadingID, Value: value, Declared: start.Declared, Origin: meter.TripStart, Method: meter.Typed})
		if err != nil {
			return app.ErrUnavailable
		}
		reading, err := s.Readings.ApplyReviewedEmbeddedTx(ctx, syncapp.Borrow(tx), scope, fleetapp.ReviewedEmbeddedReading{ResolutionID: resolution, AuthorMembershipID: view.AuthorMembershipID,
			EnrollmentID: view.EnrollmentID, ReceivedAt: view.ReceivedAt, Command: command})
		if err != nil {
			return err
		}
		if got := reading.Snapshot(); got.Reference.ID() != start.ReadingID || got.Origin != meter.TripStart || got.Value.Count() != value.Count() || !got.ReceivedAt.Equal(view.ReceivedAt) {
			return app.ErrUnavailable
		}
	}
	zones := orgtime.ReportingZones{Organization: organizationZone}
	unit, centre := "", ""
	snapshot, err := s.Allocations.UnitAndCostCentreAtDBTX(ctx, syncapp.Borrow(tx), scope, start.AssetID, start.Declared.OccurredAt)
	switch {
	case err == nil:
		if snapshot.AssetID != start.AssetID {
			return app.ErrUnavailable
		}
		unit, centre = snapshot.UnitID, snapshot.CostCentreID
	case errors.Is(err, fleetapp.ErrUnallocated):
	default:
		return app.ErrUnavailable
	}
	if unit != "" {
		zone, err := queries.TripUnitZone(ctx, q.TripUnitZoneParams{OrgID: org, UnitID: unit})
		if err != nil {
			return err
		}
		zones.Unit = &zone
	}
	started, err := app.HeldStartTrip(scope, view, zones)
	if err != nil {
		return app.ErrUnavailable
	}
	rep := started.Reporting()
	reportingDay, ok := day(rep.Day)
	if !ok || started.Reference().ID() != start.TripID || started.State() != trip.InProgress {
		return app.ErrUnavailable
	}
	at, sub := instant(start.Declared.OccurredAt)
	boot, elapsed := clockColumns(start.Clock)
	params := q.InsertTripParams{OrgID: org, ID: start.TripID, OperationID: view.OperationID, PayloadDigest: view.PayloadDigest, AssetID: start.AssetID,
		DriverEmployeeID: start.DriverID, AuthorMembershipID: view.AuthorMembershipID, AuthorEmployeeID: view.AuthorEmployeeID, EnrollmentID: view.EnrollmentID,
		OccurredAt: at, OccurredSubmicro: sub, EntryZone: start.Declared.EntryZone, DeviceOffsetSeconds: start.Declared.DeviceOffsetSeconds, BootID: boot, ElapsedMillis: elapsed,
		MissingReadingReason: text(start.MissingReason), UnitID: id(unit), CostCentreID: id(centre), ReportingDay: reportingDay, ReportingZone: rep.Zone, ReportingSource: rep.Source}
	if start.ReadingID != "" {
		params.MeterID, params.SegmentID, params.ReadingID = id(start.MeterID), id(start.SegmentID), id(start.ReadingID)
	}
	if err := queries.InsertTrip(ctx, params); err != nil {
		return err
	}
	return queries.InsertTripConflictEffect(ctx, q.InsertTripConflictEffectParams{OrgID: org, OperationID: view.OperationID})
}
