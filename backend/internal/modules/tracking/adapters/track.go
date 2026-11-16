package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditDefinitions registers the recorded-track access vocabulary
// (EMP-PRIV-005 LOCATION_TRACK, EMP-GPS-012).
func AuditDefinitions() []audit.Definition { return audit.AccessDefinitions(audit.LocationTrack) }

// TrackAuthority re-proves gps.track.read over one stored trip inside the
// read transaction with the company proof (EMP-GPS-012).
type TrackAuthority interface {
	CheckTrackTx(context.Context, pgx.Tx, kernel.OrgScope, app.TrackActor, string, string) error
}

// TrackStore serves recorded tracks: authority in the read transaction,
// then the access record, then the bytes (B-BR-EMP-435..437).
type TrackStore struct {
	Pool      *pgxpool.Pool
	Authority TrackAuthority
	Access    auditapp.AccessRecorder
	NewID     func() (string, error)
}

var _ app.TrackRepository = (*TrackStore)(nil)

// TripAuthor returns the stored trip's author membership, the own-record and
// trip-driver fact of the route resolver.
func (s *TrackStore) TripAuthor(ctx context.Context, scope kernel.OrgScope, trip string) (string, error) {
	if s == nil || s.Pool == nil {
		return "", app.ErrUnavailable
	}
	var author string
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT author_membership_id::text FROM org_trips WHERE org_id=$1 AND id=$2`, scope.OrgID().String(), trip).Scan(&author)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrNotFound
	}
	if err != nil {
		return "", storeError(err)
	}
	return author, nil
}

// binding is the served page's identity digest, never its content.
func binding(scope kernel.OrgScope, v app.TrackView) string {
	parts := []string{"tracking.track-read.v1", scope.OrgID().String(), v.TripID}
	if v.Session != nil {
		parts = append(parts, v.Session.ID, strconv.Itoa(len(v.Session.Transitions)))
	}
	for _, s := range v.Segments {
		parts = append(parts, strconv.FormatInt(s.Sequence, 10))
	}
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *TrackStore) ReadTrack(ctx context.Context, scope kernel.OrgScope, actor app.TrackActor, decision access.Decision, request app.TrackRequest) (app.TrackView, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) || absent(s.Access) || s.NewID == nil {
		return app.TrackView{}, app.ErrUnavailable
	}
	if err := decision.Check(ctx, scope, actor.AccountID, app.TrackPermission, request.TripID); err != nil {
		return app.TrackView{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	view := app.TrackView{TripID: request.TripID}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckTrackTx(ctx, tx, scope, actor, member, request.TripID); err != nil {
			return err
		}
		db := syncapp.Borrow(tx)
		session, err := (&Store{}).SessionOfTripTx(ctx, db, scope, request.TripID)
		switch {
		case err == nil:
			served := &app.TrackSession{ID: session.ID, Policy: string(session.Policy), OpenedAt: session.OpenedAt}
			for _, t := range session.Transitions {
				served.Transitions = append(served.Transitions, app.TrackTransition{Sequence: t.Seq, State: string(t.State), Cause: string(t.Cause), Actor: string(t.Actor), Permission: string(t.Permission), At: t.At})
			}
			if n := len(session.Transitions); n > 0 {
				served.State = string(session.Transitions[n-1].State)
			}
			view.Session = served
		case !errors.Is(err, app.ErrNotFound):
			return err
		}
		rows, err := tx.Query(ctx, `SELECT seq,source,received_at,received_count,stored_count,duplicate_count,outside_session_count,mock_count,collected_at,lat_e7,lon_e7,accuracy_m,speed_cm_s,bearing_deg,coarse
 FROM org_tracking_segments WHERE org_id=$1 AND trip_id=$2 AND seq>$3 ORDER BY seq LIMIT $4`, scope.OrgID().String(), request.TripID, request.AfterSequence, request.Limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var g app.TrackSegment
			var at []time.Time
			var lat, lon, accuracy []int32
			var speed []*int32
			var bearing []*int16
			var coarse []bool
			if err := rows.Scan(&g.Sequence, &g.Source, &g.ReceivedAt, &g.Received, &g.Stored, &g.Duplicates, &g.Outside, &g.Mock, &at, &lat, &lon, &accuracy, &speed, &bearing, &coarse); err != nil {
				return err
			}
			if len(at) != g.Stored || len(lat) != g.Stored || len(lon) != g.Stored || len(accuracy) != g.Stored || len(speed) != g.Stored || len(bearing) != g.Stored || len(coarse) != g.Stored {
				return app.ErrUnavailable
			}
			g.ReceivedAt = g.ReceivedAt.UTC()
			for i := range at {
				g.Fixes = append(g.Fixes, app.TrackFix{At: at[i].UTC(), Lat: lat[i], Lon: lon[i], Accuracy: accuracy[i], Speed: speed[i], Bearing: bearing[i], Coarse: coarse[i]})
			}
			view.Segments = append(view.Segments, g)
		}
		return rows.Err()
	})
	if err != nil {
		// The route maps the authority vocabulary; anything else is unavailable.
		return app.TrackView{}, err
	}
	if len(view.Segments) > request.Limit {
		view.Segments = view.Segments[:request.Limit]
		view.NextSequence = view.Segments[request.Limit-1].Sequence
	}
	// B-BR-EMP-436: the issuance and the served access are recorded before any
	// byte leaves; any failure serves nothing.
	issued, err := s.NewID()
	if err != nil {
		return app.TrackView{}, app.ErrAccessUnrecorded
	}
	served, err := s.NewID()
	if err != nil {
		return app.TrackView{}, app.ErrAccessUnrecorded
	}
	digest := binding(scope, view)
	record := audit.SensitiveAccess{Category: audit.LocationTrack, Stage: audit.AccessIssued, AccountID: actor.AccountID, MembershipID: member, TargetID: request.TripID, AccessID: issued, RequestID: issued, Binding: digest}
	if err := s.Access.RecordAccess(ctx, scope, record); err != nil {
		return app.TrackView{}, app.ErrAccessUnrecorded
	}
	record.Stage, record.RequestID = audit.AccessServed, served
	if err := s.Access.RecordAccess(ctx, scope, record); err != nil {
		return app.TrackView{}, app.ErrAccessUnrecorded
	}
	return view, nil
}
