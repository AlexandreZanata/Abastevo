package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EMP-TRIP-010 (B-BR-EMP-476..478, EDR-308..310): the member's
// planned-versus-observed read of one own trip. The mission link, plan,
// planned stops and held observations are read with raw parametrized
// statements (the trip and mission sqlc stanzas do not include those tables,
// the EMP-TRIP-023 precedent).

// StopsStore reads one own trip under the EMP-TRIP-012 own-trips authority.
type StopsStore struct {
	Pool      *pgxpool.Pool
	Authority HandoverAuthority
}

func (s *StopsStore) ready() bool { return s != nil && s.Pool != nil && !absent(s.Authority) }

// ReadOwnTripStops proves trip.trip.start over the requesting membership,
// reads the trip only when the member started it or is its current
// responsible member (anything else is ErrNotFound), its accepted and held
// STOP, ARRIVAL and DEPARTURE events and, for a trip linked to a mission,
// that mission's current planning revision with its stops, then proves the
// authority again, in one forced-RLS transaction.
func (s *StopsStore) ReadOwnTripStops(ctx context.Context, scope kernel.OrgScope, actor app.ReviewActor, member, tripID string) (app.StopsRecord, error) {
	if !s.ready() {
		return app.StopsRecord{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !canonicalID(member) {
		return app.StopsRecord{}, app.ErrReviewUnauthorized
	}
	if !canonicalID(tripID) {
		return app.StopsRecord{}, app.ErrNotFound
	}
	var record app.StopsRecord
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		reader, err := s.Authority.CheckTripHandoverTx(ctx, tx, scope, actor, member, member, app.OwnTripsAction)
		if err != nil {
			return err
		}
		if record, err = readOwnTripStops(ctx, tx, scope, member, tripID); err != nil {
			return err
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
		return app.StopsRecord{}, reviewFailure(ctx, err)
	}
	return record, nil
}

// stopMarks maps the logbook types that bound stops and legs.
var stopMarks = map[trip.LogbookType]trip.StopMark{trip.LogbookStop: trip.MarkStop, trip.LogbookArrival: trip.MarkArrival, trip.LogbookDeparture: trip.MarkDeparture}

func readOwnTripStops(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member, tripID string) (app.StopsRecord, error) {
	org := scope.OrgID().String()
	var own bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_trips t WHERE t.org_id=$1::uuid AND t.id=$2::uuid
 AND(t.author_membership_id=$3::uuid OR org_trip_responsible_member(t.org_id,t.id)=$3::uuid))`, org, tripID, member).Scan(&own); err != nil {
		return app.StopsRecord{}, err
	}
	if !own {
		return app.StopsRecord{}, app.ErrNotFound
	}
	db := syncapp.Borrow(tx)
	stored, err := (&Store{}).ReadTripTx(ctx, db, scope, tripID)
	if err != nil {
		return app.StopsRecord{}, err
	}
	r := app.StopsRecord{TripID: stored.ID, AssetID: stored.AssetID, State: stored.State,
		Start: app.ObservedFact{Mark: trip.MarkStart, FactID: stored.ID, Declared: stored.Declared, ReceivedAt: stored.ReceivedAt}}
	if f := stored.Finish; f != nil {
		r.End = &app.ObservedFact{Mark: trip.MarkFinish, FactID: stored.ID, Declared: f.Declared, ReceivedAt: f.ReceivedAt}
	}
	for _, x := range stored.Transitions {
		if x.Command == trip.CommandCancel {
			r.End = &app.ObservedFact{Mark: trip.MarkCancel, FactID: x.OperationID, Declared: x.Declared, ReceivedAt: x.ReceivedAt}
		}
	}
	rows, err := q.New(db).ListTripEvents(ctx, q.ListTripEventsParams{OrgID: org, TripID: tripID})
	if err != nil {
		return app.StopsRecord{}, err
	}
	for _, row := range rows {
		e, err := eventRecord(q.GetTripEventRow(row))
		if err != nil || e.TripID != tripID {
			return app.StopsRecord{}, app.ErrUnavailable
		}
		if mark, ok := stopMarks[e.Type]; ok {
			r.Events = append(r.Events, app.ObservedFact{Mark: mark, FactID: e.ID, Declared: e.Declared, ReceivedAt: e.ReceivedAt})
		}
	}
	if r.Held, err = heldObservations(ctx, tx, org, tripID); err != nil {
		return app.StopsRecord{}, err
	}
	if r.Plan, err = linkedPlan(ctx, tx, org, tripID); err != nil {
		return app.StopsRecord{}, err
	}
	return r, nil
}

// heldObservations reads the trip's held STOP, ARRIVAL and DEPARTURE events
// with their review code (EMP-TRIP-003/015).
func heldObservations(ctx context.Context, tx pgx.Tx, org, tripID string) ([]app.HeldObservation, error) {
	rows, err := tx.Query(ctx, `SELECT h.id::text,h.event_type,h.occurred_at,h.occurred_submicro,h.entry_zone,h.device_offset_seconds,i.code
FROM org_trip_held_events h JOIN org_sync_review_items i ON(i.org_id,i.id,i.operation_id)=(h.org_id,h.review_item_id,h.operation_id)
WHERE h.org_id=$1::uuid AND h.trip_id=$2::uuid AND h.event_type IN('STOP','ARRIVAL','DEPARTURE') ORDER BY h.occurred_at,h.occurred_submicro,h.id`, org, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []app.HeldObservation
	for rows.Next() {
		var id, kind, zone, code string
		var occurred pgtype.Timestamptz
		var sub int16
		var offset int64
		if err := rows.Scan(&id, &kind, &occurred, &sub, &zone, &offset, &code); err != nil {
			return nil, err
		}
		at, ok := exact(occurred, sub)
		if !ok {
			return nil, app.ErrUnavailable
		}
		out = append(out, app.HeldObservation{EventID: id, Type: trip.LogbookType(kind), Declared: orgtime.Declaration{OccurredAt: at, EntryZone: zone, DeviceOffsetSeconds: offset}, Code: code})
	}
	return out, rows.Err()
}

// linkedPlan reads the current planning revision, window and planned stops
// of the mission the trip executes (EMP-TRIP-023 link); nil for none.
func linkedPlan(ctx context.Context, tx pgx.Tx, org, tripID string) (*app.PlannedRecord, error) {
	var p app.PlannedRecord
	var planned, starts, ends time.Time
	err := tx.QueryRow(ctx, `SELECT m.id::text,p.revision,p.recorded_at,m.starts_at,m.ends_at,p.destination_text,COALESCE(p.destination_unit_id::text,'')
FROM org_trip_mission_links l JOIN org_trip_missions m ON(m.org_id,m.id)=(l.org_id,l.mission_id)
JOIN org_trip_mission_plans p ON(p.org_id,p.mission_id)=(m.org_id,m.id)
WHERE l.org_id=$1::uuid AND l.trip_id=$2::uuid ORDER BY p.revision DESC LIMIT 1`, org, tripID).Scan(&p.MissionID, &p.Revision, &planned, &starts, &ends, &p.DestinationText, &p.DestinationUnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.PlannedAt, p.StartsAt, p.EndsAt = planned.UTC(), starts.UTC(), ends.UTC()
	rows, err := tx.Query(ctx, `SELECT sequence,place_text FROM org_trip_mission_plan_stops WHERE org_id=$1::uuid AND mission_id=$2::uuid AND revision=$3 ORDER BY sequence`,
		org, p.MissionID, p.Revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sequence int16
		var place string
		if err := rows.Scan(&sequence, &place); err != nil {
			return nil, err
		}
		if int(sequence) != len(p.Stops)+1 {
			return nil, app.ErrUnavailable
		}
		p.Stops = append(p.Stops, place)
	}
	return &p, rows.Err()
}
