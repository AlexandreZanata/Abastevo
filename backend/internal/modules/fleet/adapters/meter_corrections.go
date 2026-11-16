package adapters

import (
	"context"
	"errors"
	"slices"
	"strings"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	events "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/application"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// Person decisions of org_meter_reading_states (EDR-184, EDR-187).
const (
	causeConfirmed  = "CONFIRMED"
	causeVoided     = "VOIDED"
	causeSuperseded = "SUPERSEDED"
	// MeterRecalculatedEvent is published only with composed consumers (EDR-189).
	MeterRecalculatedEvent = "meter.usage.recalculated"
	meterReadingTarget     = "meter_reading"
)

// MeterParentRecords answers whether a reading belongs to an APPROVED parent
// record (B-BR-EMP-290). The fueling store owns the only parent today.
type MeterParentRecords interface {
	ApprovedParentTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (bool, error)
}

// MeterRecalculation publishes meter.usage.recalculated through the PLAT-010
// outbox in the decision transaction. Nil publishes nothing (EDR-189).
type MeterRecalculation struct {
	Catalog   event.Catalog
	Writer    events.Writer
	Consumers []string
}

var meterDecisionActions = map[app.MeterDecision]string{app.MeterSupersede: "meter.reading.corrected", app.MeterVoid: "meter.reading.voided", app.MeterConfirm: "meter.reading.confirmed", app.MeterConfirmRollover: meterRolloverAction}

func MeterCorrectionEventDefinitions(consumers []string) []event.Definition {
	return []event.Definition{{Name: MeterRecalculatedEvent, TargetType: meterReadingTarget, Consumers: slices.Clone(consumers)}}
}

func flagsOf(raw []string) []meter.Flag {
	out := make([]meter.Flag, len(raw))
	for i, f := range raw {
		out[i] = meter.Flag(f)
	}
	return out
}

func (s *MeterStore) decisionEvent(scope kernel.OrgScope, actor app.FleetActor, v app.MeterDecisionInput, digest string) (audit.Event, *event.Plan, error) {
	bound := audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: meterDecisionActions[v.Decision], TargetType: meterReadingTarget, TargetID: v.ReadingID, RequestHash: digest}
	if s.Recalculation == nil {
		return bound, nil, nil
	}
	plan, bound, err := s.Recalculation.Catalog.Prepare(scope, bound, event.Event{ID: v.OperationID, Name: MeterRecalculatedEvent, TargetType: meterReadingTarget, TargetID: v.ReadingID, Revision: v.ExpectedRevision + 1}, s.Recalculation.Consumers)
	if err != nil || s.Recalculation.Writer == nil {
		return audit.Event{}, nil, app.ErrUnavailable
	}
	return bound, &plan, nil
}

// DecideMeterReading applies one person decision (B-BR-EMP-286..290) under
// the root, asset and meter locks, with its audit event, re-evaluation and
// optional recalculation intents in one transaction.
func (s *MeterStore) DecideMeterReading(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, newID string, command app.MeterDecisionCommand) (app.MeterDecisionResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.MeterDecisionResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.MeterDecisionResult{}, false, err
	}
	if !validID(newID) || newID == v.ReadingID {
		return app.MeterDecisionResult{}, false, meter.ErrInvalid
	}
	if !s.ready() || s.Audit == nil || s.Parents == nil {
		return app.MeterDecisionResult{}, false, app.ErrUnavailable
	}
	digest := command.Digest(scope)
	bound, plan, err := s.decisionEvent(scope, actor, v, digest)
	if err != nil {
		return app.MeterDecisionResult{}, false, err
	}
	source := app.MeterSource{Web: &actor}
	org := scope.OrgID().String()
	var result app.MeterDecisionResult
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterCorrectAction); err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.AssetRootLock(ctx, org); err != nil {
			return err
		}
		if _, err := queries.LockMeterAsset(ctx, q.LockMeterAssetParams{OrgID: org, AssetID: v.AssetID}); err != nil {
			return err
		}
		if _, err := queries.LockMeter(ctx, q.LockMeterParams{OrgID: org, AssetID: v.AssetID, ID: v.MeterID}); err != nil {
			return err
		}
		view, err := loadMeter(ctx, tx, scope, v.AssetID, v.MeterID, false)
		if err != nil {
			return err
		}
		used, err := queries.MeterOperationStateCount(ctx, q.MeterOperationStateCountParams{OrgID: org, OperationID: v.OperationID})
		if err != nil {
			return err
		}
		if used > 0 {
			repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(context.Context, pgx.Tx) error { return app.ErrUnavailable })
			if err != nil {
				return err
			}
			if !repeated {
				return app.ErrUnavailable
			}
			result, err = decisionResult(ctx, tx, scope, view, v)
			if err != nil {
				return err
			}
			if err := rolloverBoundary(ctx, tx, scope, v, &result); err != nil {
				return err
			}
			replay = true
			return s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterCorrectAction)
		}
		target, err := queries.MeterReadingCurrent(ctx, q.MeterReadingCurrentParams{OrgID: org, MeterID: v.MeterID, SegmentID: view.Segment.ID(), ID: v.ReadingID})
		if err != nil {
			return err
		}
		if target.State == string(meter.Voided) {
			return app.ErrReadingAlreadySuperseded
		}
		if target.Origin == string(meter.Installation) {
			return meter.ErrInvalid
		}
		// EMP-METER-005: every opening or closing reading of a segment is an
		// anchor (B-BR-EMP-486).
		if anchor, err := queries.MeterReadingIsAnchor(ctx, q.MeterReadingIsAnchorParams{OrgID: org, MeterID: v.MeterID, ReadingID: v.ReadingID}); err != nil {
			return err
		} else if anchor {
			return meter.ErrInvalid
		}
		approved, err := s.Parents.ApprovedParentTx(ctx, syncapp.Borrow(tx), scope, v.ReadingID)
		if err != nil {
			return app.ErrUnavailable
		}
		if approved {
			return app.ErrAmendmentRequired
		}
		if int64(target.Revision) != v.ExpectedRevision {
			return app.ErrConflict
		}
		if v.Decision == app.MeterConfirm || v.Decision == app.MeterConfirmRollover {
			if target.State != string(meter.Suspect) {
				return app.ErrConflict
			}
			for _, f := range target.Flags {
				if v.Decision == app.MeterConfirm && (f == string(meter.ReadingBelowPrevious) || f == string(meter.ReadingAboveNext) || f == string(meter.PossibleRollover)) {
					return meter.ErrInvalid
				}
			}
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
			if err := s.applyDecision(ctx, tx, scope, view, source, member, newID, v, digest, target); err != nil {
				return err
			}
			if plan != nil {
				for _, consumer := range plan.Consumers {
					envelope := event.Envelope{Version: event.Version, OrgID: org, Consumer: consumer, Event: plan.Event}
					if err := s.Recalculation.Writer.Enqueue(ctx, scope, tx, envelope, bound.RequestHash); err != nil {
						return app.ErrUnavailable
					}
				}
			}
			var err error
			if result, err = decisionResult(ctx, tx, scope, view, v); err != nil {
				return err
			}
			return rolloverBoundary(ctx, tx, scope, v, &result)
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.check(ctx, tx, scope, source, member, v.AssetID, app.MeterCorrectAction)
	})
	if err != nil {
		if errors.Is(err, audit.ErrReused) {
			return app.MeterDecisionResult{}, false, app.ErrOperationReused
		}
		return app.MeterDecisionResult{}, false, meterError(ctx, err)
	}
	return result.Clone(), replay, nil
}

func (s *MeterStore) applyDecision(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, view app.MeterView, source app.MeterSource, member, newID string, v app.MeterDecisionInput, digest string, target q.MeterReadingCurrentRow) error {
	queries := q.New(tx)
	org := scope.OrgID().String()
	state := q.InsertMeterReadingStateParams{OrgID: org, MeterID: v.MeterID, ReadingID: v.ReadingID, Revision: target.Revision + 1, Flags: []string{}, OperationID: v.OperationID, ActorMembershipID: member, Reason: v.Reason}
	switch v.Decision {
	case app.MeterVoid:
		state.State, state.Cause = string(meter.Voided), causeVoided
	case app.MeterConfirm:
		state.State, state.Cause = string(meter.Valid), causeConfirmed
	case app.MeterSupersede:
		return s.supersede(ctx, tx, scope, view, source, member, newID, v, digest, target, state)
	case app.MeterConfirmRollover:
		return s.confirmRollover(ctx, tx, scope, view, member, newID, v, target)
	default:
		return meter.ErrInvalid
	}
	if err := queries.InsertMeterReadingState(ctx, state); err != nil {
		return err
	}
	if s.Plausibility == nil {
		return nil
	}
	return reevaluateSegment(ctx, tx, scope, *s.Plausibility, view, v.OperationID)
}

func pointBefore(a, b meter.Point) bool {
	if !a.OccurredAt.Equal(b.OccurredAt) {
		return a.OccurredAt.Before(b.OccurredAt)
	}
	if !a.ReceivedAt.Equal(b.ReceivedAt) {
		return a.ReceivedAt.Before(b.ReceivedAt)
	}
	return a.ID < b.ID
}

// supersede records the corrected reading (the original's origin, method
// TYPED, the deciding member and web session) and voids the original with
// the link (B-BR-EMP-287). The new reading is classified against the series
// in which the original is VOIDED, then the whole segment is re-evaluated in
// memory, so the new reading's revision 1 is its final verdict and every
// other changed SUSPECT reading gets one REEVALUATION revision.
func (s *MeterStore) supersede(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, view app.MeterView, source app.MeterSource, member, newID string, v app.MeterDecisionInput, digest string, target q.MeterReadingCurrentRow, voided q.InsertMeterReadingStateParams) error {
	queries := q.New(tx)
	org := scope.OrgID().String()
	original, err := queries.GetMeterReading(ctx, q.GetMeterReadingParams{OrgID: org, MeterID: v.MeterID, SegmentID: view.Segment.ID(), ID: v.ReadingID})
	if err != nil {
		return err
	}
	declared := orgtime.Declaration{EntryZone: original.EntryZone, DeviceOffsetSeconds: original.DeviceOffsetSeconds}
	if declared.OccurredAt, err = exactMeterTime(original.OccurredAt, original.OccurredSubmicro); err != nil {
		return err
	}
	if v.Declared != nil {
		declared = *v.Declared
	}
	if declared.OccurredAt.Before(view.InstalledAt) || view.RemovedAt != nil && !declared.OccurredAt.Before(*view.RemovedAt) || v.Value.Kind() != view.Reference.Kind() {
		return meter.ErrInvalid
	}
	value, err := meter.NewValue(view.Reference.Kind(), v.Value.Count(), view.Reference.Modulus())
	if err != nil {
		return err
	}
	capture, err := source.Capture(scope)
	if err != nil || capture.Kind() != meter.CaptureWebSession {
		return app.ErrUnavailable
	}
	now, err := meterDBNow(ctx, queries)
	if err != nil {
		return err
	}
	verdict := meter.Verdict{State: meter.Valid}
	var changes []meter.Change
	revisions := map[string]int32{}
	if s.Plausibility != nil {
		members, err := meterSeries(ctx, tx, scope, v.MeterID, view.Segment.ID())
		if err != nil {
			return err
		}
		entries := seriesEntries(members)
		for i := range entries {
			revisions[entries[i].ID] = members[i].revision
			if entries[i].ID == v.ReadingID {
				entries[i].State, entries[i].Flags = meter.Voided, nil
			}
		}
		candidate := candidatePoint(newID, value.Count(), declared.OccurredAt, now)
		if verdict, err = s.Plausibility.CheckMeter(view.Reference.Kind(), view.Reference.Modulus(), entries, candidate); err != nil {
			return meter.ErrInvalid
		}
		with := make([]meter.Entry, 0, len(entries)+1)
		placed := false
		for _, e := range entries {
			if !placed && pointBefore(candidate, e.Point) {
				with = append(with, meter.Entry{Point: candidate, State: verdict.State, Flags: verdict.Flags})
				placed = true
			}
			with = append(with, e)
		}
		if !placed {
			with = append(with, meter.Entry{Point: candidate, State: verdict.State, Flags: verdict.Flags})
		}
		if changes, err = s.Plausibility.ReevaluateMeter(view.Reference.Kind(), view.Reference.Modulus(), with); err != nil {
			return app.ErrUnavailable
		}
		for _, c := range changes {
			if c.ID == newID {
				verdict = c.Verdict
			}
		}
	}
	at, sub := meterTime(declared.OccurredAt)
	if err := queries.InsertMeterReading(ctx, q.InsertMeterReadingParams{OrgID: org, ID: newID, MeterID: v.MeterID, SegmentID: view.Segment.ID(), Value: value.Count(), OccurredAt: at, OccurredSubmicro: sub, EntryZone: declared.EntryZone, DeviceOffsetSeconds: declared.DeviceOffsetSeconds, AuthorMembershipID: member, CaptureKind: string(capture.Kind()), WebSessionID: capture.ID(), OperationID: v.OperationID, RequestHash: digest, Origin: target.Origin, Method: string(meter.Typed), State: string(verdict.State)}); err != nil {
		return err
	}
	voided.State, voided.Cause, voided.SupersededBy = string(meter.Voided), causeSuperseded, newID
	if err := queries.InsertMeterReadingState(ctx, voided); err != nil {
		return err
	}
	if s.Plausibility == nil {
		return nil
	}
	if err := queries.InsertMeterReadingState(ctx, q.InsertMeterReadingStateParams{OrgID: org, MeterID: v.MeterID, ReadingID: newID, Revision: 1, State: string(verdict.State), Flags: flagStrings(verdict.Flags), Retroactive: verdict.Retroactive, Cause: causeIntake, OperationID: v.OperationID}); err != nil {
		return err
	}
	for _, c := range changes {
		if c.ID == newID {
			continue
		}
		if err := queries.InsertMeterReadingState(ctx, q.InsertMeterReadingStateParams{OrgID: org, MeterID: v.MeterID, ReadingID: c.ID, Revision: revisions[c.ID] + 1, State: string(c.Verdict.State), Flags: flagStrings(c.Verdict.Flags), Retroactive: c.Verdict.Retroactive, Cause: causeReevaluation, OperationID: v.OperationID}); err != nil {
			return err
		}
	}
	return nil
}

// decisionResult rebuilds the decision from the rows of its operation only.
func decisionResult(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, view app.MeterView, v app.MeterDecisionInput) (app.MeterDecisionResult, error) {
	rows, err := q.New(tx).MeterStatesByOperation(ctx, q.MeterStatesByOperationParams{OrgID: scope.OrgID().String(), MeterID: v.MeterID, OperationID: v.OperationID})
	if err != nil {
		return app.MeterDecisionResult{}, err
	}
	var result app.MeterDecisionResult
	replacement := ""
	for _, row := range rows {
		state := app.MeterReadingState{ReadingID: row.ReadingID, State: meter.State(row.State), Flags: flagsOf(row.Flags), Revision: int64(row.Revision)}
		switch {
		case row.ReadingID == v.ReadingID && row.Cause != causeReevaluation:
			result.Target = state
			replacement = row.SupersededBy
		case row.Cause == causeReevaluation:
			result.Reevaluated = append(result.Reevaluated, state)
		}
	}
	if result.Target.ReadingID == "" {
		return app.MeterDecisionResult{}, app.ErrUnavailable
	}
	if replacement != "" {
		r := app.MeterReadingState{ReadingID: replacement}
		for _, row := range rows {
			if row.ReadingID == replacement && row.Cause == causeIntake {
				r.State, r.Flags, r.Revision = meter.State(row.State), flagsOf(row.Flags), int64(row.Revision)
			}
		}
		if r.State == "" {
			// Validation switched off: the replacement has no history (EDR-185).
			stored, err := q.New(tx).GetMeterReading(ctx, q.GetMeterReadingParams{OrgID: scope.OrgID().String(), MeterID: v.MeterID, SegmentID: view.Segment.ID(), ID: replacement})
			if err != nil {
				return app.MeterDecisionResult{}, err
			}
			r.State, r.Flags = meter.State(stored.State), []meter.Flag{}
		}
		result.Replacement = &r
	}
	slices.SortFunc(result.Reevaluated, func(a, b app.MeterReadingState) int { return strings.Compare(a.ReadingID, b.ReadingID) })
	return result, nil
}

// MeterReadingHistory reads the open segment's history (B-BR-EMP-291); with
// suspects it lists the current SUSPECT readings and their valid neighbours.
func (s *MeterStore) MeterReadingHistory(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset, meterID string, filter app.MeterHistoryFilter, suspects bool) (app.MeterHistoryPage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.MeterHistoryPage{}, err
	}
	if !validID(asset) || !validID(meterID) {
		return app.MeterHistoryPage{}, meter.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return app.MeterHistoryPage{}, err
	}
	if !s.ready() {
		return app.MeterHistoryPage{}, app.ErrUnavailable
	}
	action := app.MeterReadAction
	if suspects {
		action = app.MeterCorrectReadAction
	}
	source := app.MeterSource{Web: &actor}
	page := app.MeterHistoryPage{Items: []app.MeterHistoryItem{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.check(ctx, tx, scope, source, member, asset, action); err != nil {
			return err
		}
		view, err := loadMeter(ctx, tx, scope, asset, meterID, false)
		if err != nil {
			return err
		}
		queries := q.New(tx)
		if page.At, err = meterDBNow(ctx, queries); err != nil {
			return err
		}
		rows, err := queries.MeterReadingHistoryPage(ctx, q.MeterReadingHistoryPageParams{OrgID: scope.OrgID().String(), MeterID: meterID, SegmentID: view.Segment.ID(), State: string(filter.State), AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		more := len(rows) > filter.Limit
		if more {
			rows = rows[:filter.Limit]
		}
		var neighbours map[string][2]string
		if suspects {
			members, err := meterSeries(ctx, tx, scope, meterID, view.Segment.ID())
			if err != nil {
				return err
			}
			neighbours = validNeighbours(members)
		}
		for _, row := range rows {
			reading, err := meterReading(scope, view, q.GetMeterReadingByOperationRow{ID: row.ID, MeterID: row.MeterID, SegmentID: row.SegmentID, Value: row.Value, OccurredAt: row.OccurredAt, OccurredSubmicro: row.OccurredSubmicro, ReceivedAt: row.ReceivedAt, EntryZone: row.EntryZone, DeviceOffsetSeconds: row.DeviceOffsetSeconds, AuthorMembershipID: row.AuthorMembershipID, CaptureKind: row.CaptureKind, CaptureID: row.CaptureID, OperationID: row.OperationID, RequestHash: row.RequestHash, Origin: row.Origin, Method: row.Method, State: row.State})
			if err != nil {
				return err
			}
			item := app.MeterHistoryItem{Reading: reading, State: meter.State(row.CurrentState), Flags: flagsOf(row.Flags), Retroactive: row.Retroactive, Revision: int64(row.Revision), SupersededBy: row.SupersededBy, Supersedes: row.Supersedes, Reason: row.Reason}
			if suspects {
				pair := neighbours[row.ID]
				item.Previous, item.Next = pair[0], pair[1]
			}
			page.Items = append(page.Items, item)
		}
		if more {
			page.NextID = rows[len(rows)-1].ID
		}
		return s.check(ctx, tx, scope, source, member, asset, action)
	})
	if err != nil {
		return app.MeterHistoryPage{}, meterError(ctx, err)
	}
	return page, nil
}

// validNeighbours names, for every member, the closest VALID members before
// and after it in series order.
func validNeighbours(members []seriesMember) map[string][2]string {
	out := make(map[string][2]string, len(members))
	previous := ""
	for _, m := range members {
		pair := out[m.entry.ID]
		pair[0] = previous
		out[m.entry.ID] = pair
		if m.entry.State == meter.Valid {
			previous = m.entry.ID
		}
	}
	next := ""
	for i := len(members) - 1; i >= 0; i-- {
		m := members[i]
		pair := out[m.entry.ID]
		pair[1] = next
		out[m.entry.ID] = pair
		if m.entry.State == meter.Valid {
			next = m.entry.ID
		}
	}
	return out
}

// MeterParentSet answers for every composed parent owner: a reading belongs
// to an APPROVED parent record when any owner says so (B-BR-EMP-290,
// EMP-TRIP-015 B-BR-EMP-368).
type MeterParentSet []MeterParentRecords

func (set MeterParentSet) ApprovedParentTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, reading string) (bool, error) {
	if len(set) == 0 {
		return false, app.ErrUnavailable
	}
	for _, owner := range set {
		if owner == nil {
			return false, app.ErrUnavailable
		}
		approved, err := owner.ApprovedParentTx(ctx, db, scope, reading)
		if err != nil {
			return false, err
		}
		if approved {
			return true, nil
		}
	}
	return false, nil
}

// SupersedeApprovedTx applies an approved amendment's corrected reading
// inside the parent owner's transaction (EMP-TRIP-015, B-BR-EMP-366): the
// METER-004 supersession under the asset and meter locks with the same
// classification and segment re-evaluation. It has no authority, audit event
// or approved-parent refusal of its own: the parent owner proved the review
// decision and records its audit event in the same transaction.
func (s *MeterStore) SupersedeApprovedTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.FleetActor, member string, in app.AmendmentSupersession) error {
	if s == nil || tx == nil {
		return app.ErrUnavailable
	}
	if err := validCall(scope, actor, member); err != nil {
		return err
	}
	if !validID(in.NewReadingID) || in.NewReadingID == in.ReadingID || in.Value < 0 || len(in.RequestHash) != 64 {
		return meter.ErrInvalid
	}
	if database.ValidateOrgTx(ctx, tx, scope) != nil {
		return app.ErrUnavailable
	}
	queries := q.New(tx)
	org := scope.OrgID().String()
	if _, err := queries.LockMeterAsset(ctx, q.LockMeterAssetParams{OrgID: org, AssetID: in.AssetID}); err != nil {
		return err
	}
	if _, err := queries.LockMeter(ctx, q.LockMeterParams{OrgID: org, AssetID: in.AssetID, ID: in.MeterID}); err != nil {
		return err
	}
	view, err := loadMeter(ctx, tx, scope, in.AssetID, in.MeterID, false)
	if err != nil {
		return err
	}
	target, err := queries.MeterReadingCurrent(ctx, q.MeterReadingCurrentParams{OrgID: org, MeterID: in.MeterID, SegmentID: view.Segment.ID(), ID: in.ReadingID})
	if err != nil {
		return err
	}
	if target.State == string(meter.Voided) {
		return app.ErrReadingAlreadySuperseded
	}
	if target.Origin == string(meter.Installation) {
		return meter.ErrInvalid
	}
	value, err := meter.NewValue(view.Reference.Kind(), in.Value, view.Reference.Modulus())
	if err != nil {
		return meter.ErrInvalid
	}
	// The METER-004 command validation (identifiers, reason, value).
	command, err := app.NewMeterDecisionCommand(app.MeterDecisionInput{OperationID: in.OperationID, AssetID: in.AssetID, MeterID: in.MeterID, ReadingID: in.ReadingID,
		ExpectedRevision: int64(target.Revision), Decision: app.MeterSupersede, Reason: in.Reason, Value: value})
	if err != nil {
		return err
	}
	v, err := command.Snapshot()
	if err != nil {
		return err
	}
	return s.applyDecision(ctx, tx, scope, view, app.MeterSource{Web: &actor}, member, in.NewReadingID, v, in.RequestHash, target)
}
