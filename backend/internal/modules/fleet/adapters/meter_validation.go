package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
)

// Stored causes of org_meter_reading_states (EDR-184).
const (
	causeIntake       = "INTAKE"
	causeReevaluation = "REEVALUATION"
)

// seriesMember is one stored reading of a segment with its current history
// revision (0 when it has none: recorded before 000216).
type seriesMember struct {
	entry    meter.Entry
	revision int32
}

// meterSeries reads a whole segment in BR-METER-008 order with each
// reading's current state. The caller holds the meter lock.
func meterSeries(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, meterID, segmentID string) ([]seriesMember, error) {
	rows, err := q.New(db).MeterSegmentSeries(ctx, q.MeterSegmentSeriesParams{OrgID: scope.OrgID().String(), MeterID: meterID, SegmentID: segmentID})
	if err != nil {
		return nil, err
	}
	out := make([]seriesMember, 0, len(rows))
	for _, row := range rows {
		occurred, err := exactMeterTime(row.OccurredAt, row.OccurredSubmicro)
		if err != nil {
			return nil, err
		}
		received, err := exactMeterTime(row.ReceivedAt, 0)
		if err != nil {
			return nil, err
		}
		entry := meter.Entry{Point: meter.Point{ID: row.ID, Value: row.Value, OccurredAt: occurred, ReceivedAt: received}, State: meter.State(row.State)}
		for _, flag := range row.Flags {
			entry.Flags = append(entry.Flags, meter.Flag(flag))
		}
		out = append(out, seriesMember{entry: entry, revision: row.Revision})
	}
	return out, nil
}

func seriesEntries(members []seriesMember) []meter.Entry {
	out := make([]meter.Entry, len(members))
	for i, m := range members {
		out[i] = m.entry
	}
	return out
}

func flagStrings(flags []meter.Flag) []string {
	out := make([]string, len(flags))
	for i, f := range flags {
		out[i] = string(f)
	}
	return out
}

// classifyCandidate decides the recorded classification of a new reading
// (B-BR-EMP-280..284). Without a configuration the reading is VALID with no
// flags (the rollback of EDR-185). A candidate already present in the series
// is left to the insert, which refuses the reused identity as before.
func classifyCandidate(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, policy *meter.Plausibility, view app.MeterView, candidate meter.Point) (meter.Verdict, error) {
	valid := meter.Verdict{State: meter.Valid}
	if policy == nil {
		return valid, nil
	}
	members, err := meterSeries(ctx, db, scope, view.Reference.ID(), view.Segment.ID())
	if err != nil {
		return meter.Verdict{}, err
	}
	for _, m := range members {
		if m.entry.ID == candidate.ID {
			return valid, nil
		}
	}
	verdict, err := policy.CheckMeter(view.Reference.Kind(), view.Reference.Modulus(), seriesEntries(members), candidate)
	if errors.Is(err, meter.ErrInvalid) {
		// Every candidate input was validated before; only stored evidence
		// that no longer satisfies the series contract can reach this.
		return meter.Verdict{}, app.ErrUnavailable
	}
	return verdict, err
}

// recordClassification stores revision 1 of a classified reading and, when
// it is VALID, re-evaluates the segment's SUSPECT readings in the same
// transaction (B-BR-EMP-285). operation names the transaction's operation.
// An unclassified reading (installation anchor, switch off) writes no
// history and keeps its recorded VALID state, exactly as before 000216.
func recordClassification(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, policy *meter.Plausibility, view app.MeterView, readingID, operation string, verdict meter.Verdict) error {
	if policy == nil {
		return nil
	}
	if err := q.New(db).InsertMeterReadingState(ctx, q.InsertMeterReadingStateParams{OrgID: scope.OrgID().String(), MeterID: view.Reference.ID(), ReadingID: readingID, Revision: 1, State: string(verdict.State), Flags: flagStrings(verdict.Flags), Retroactive: verdict.Retroactive, Cause: causeIntake, OperationID: operation}); err != nil {
		return err
	}
	if verdict.State != meter.Valid {
		return nil
	}
	return reevaluateSegment(ctx, db, scope, *policy, view, operation)
}

// reevaluateSegment appends one REEVALUATION revision for every SUSPECT
// reading whose final verdict changed.
func reevaluateSegment(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, policy meter.Plausibility, view app.MeterView, operation string) error {
	members, err := meterSeries(ctx, db, scope, view.Reference.ID(), view.Segment.ID())
	if err != nil {
		return err
	}
	changes, err := policy.ReevaluateMeter(view.Reference.Kind(), view.Reference.Modulus(), seriesEntries(members))
	if err != nil {
		return app.ErrUnavailable
	}
	revisions := make(map[string]int32, len(members))
	for _, m := range members {
		revisions[m.entry.ID] = m.revision
	}
	queries := q.New(db)
	for _, c := range changes {
		if err := queries.InsertMeterReadingState(ctx, q.InsertMeterReadingStateParams{OrgID: scope.OrgID().String(), MeterID: view.Reference.ID(), ReadingID: c.ID, Revision: revisions[c.ID] + 1, State: string(c.Verdict.State), Flags: flagStrings(c.Verdict.Flags), Retroactive: c.Verdict.Retroactive, Cause: causeReevaluation, OperationID: operation}); err != nil {
			return err
		}
	}
	return nil
}

// candidatePoint is the classification key of a new reading.
func candidatePoint(id string, value int64, occurred, received time.Time) meter.Point {
	return meter.Point{ID: id, Value: value, OccurredAt: occurred, ReceivedAt: received}
}
