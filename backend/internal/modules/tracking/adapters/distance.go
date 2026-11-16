package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DistanceStore serves a trip's distance sources (EMP-GPS-006, EDR-318):
// the recorded-track authority, the trip and its latest summary in one read
// transaction, then the odometer through the METER-006 port, then the
// access record, then the answer (B-BR-EMP-491..494).
type DistanceStore struct {
	Pool      *pgxpool.Pool
	Authority TrackAuthority
	Trips     app.Trips
	Meters    fleetapp.MeterUsagePort
	Access    auditapp.AccessRecorder
	NewID     func() (string, error)
}

var _ app.DistanceRepository = (*DistanceStore)(nil)

// distanceBinding is the served answer's identity digest: the trip, the
// session and the summary revision it showed, never a value.
func distanceBinding(scope kernel.OrgScope, v app.DistanceView) string {
	raw, _ := json.Marshal([]string{"tracking.distance-read.v1", scope.OrgID().String(), v.TripID, v.GPS.SessionID, strconv.Itoa(v.GPS.Revision)})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// distanceError keeps the authority vocabulary and turns every way of not
// seeing the trip into NOT_FOUND; anything else is unavailable.
func distanceError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, app.ErrNotFound), errors.Is(err, tripapp.ErrNotFound), errors.Is(err, permissions.ErrResourceNotFound), errors.Is(err, access.ErrNoMembership),
		errors.Is(err, access.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		return app.ErrNotFound
	case errors.Is(err, app.ErrTrackUnauthorized), errors.Is(err, access.ErrUnauthenticated):
		return app.ErrTrackUnauthorized
	}
	for _, known := range []error{access.ErrEntitlementRequired, access.ErrFreshAuthRequired, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete} {
		if errors.Is(err, known) {
			return known
		}
	}
	return app.ErrUnavailable
}

// latestSummary reads the trip's session and its latest summary revision.
func latestSummary(ctx context.Context, tx pgx.Tx, org, trip string, facts *app.DistanceFacts) error {
	var session string
	var revision, coverage, gaps *int32
	var method, quality, distanceMethod *string
	var expected, gap, distance *int64
	var processed *time.Time
	err := tx.QueryRow(ctx, `SELECT s.id::text,x.revision,x.method,x.quality,x.coverage_permille,x.expected_seconds,x.gap_seconds,cardinality(x.gap_from),x.distance_m,x.distance_method,x.processed_at
 FROM org_tracking_sessions s LEFT JOIN LATERAL(SELECT * FROM org_tracking_summaries y WHERE y.org_id=s.org_id AND y.session_id=s.id ORDER BY y.revision DESC LIMIT 1) x ON true
 WHERE s.org_id=$1 AND s.trip_id=$2`, org, trip).Scan(&session, &revision, &method, &quality, &coverage, &expected, &gap, &gaps, &distance, &distanceMethod, &processed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	facts.SessionID = session
	if revision == nil {
		return nil
	}
	if method == nil || quality == nil || expected == nil || gap == nil || gaps == nil || processed == nil {
		return app.ErrUnavailable
	}
	summary := &app.SummaryFacts{Revision: int(*revision), Method: *method, Quality: domain.Quality(*quality), ExpectedSeconds: *expected, GapSeconds: *gap, Gaps: int(*gaps),
		DistanceMetres: distance, DistanceMethod: distanceMethod, ProcessedAt: processed.UTC()}
	if coverage != nil {
		v := int64(*coverage)
		summary.CoveragePermille = &v
	}
	facts.Summary = summary
	return nil
}

func (s *DistanceStore) ReadDistances(ctx context.Context, scope kernel.OrgScope, actor app.TrackActor, request app.DistanceRequest) (app.DistanceView, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) || absent(s.Trips) || absent(s.Meters) || absent(s.Access) || s.NewID == nil || scope.Validate() != nil {
		return app.DistanceView{}, app.ErrUnavailable
	}
	if err := actor.Validate(); err != nil {
		return app.DistanceView{}, err
	}
	if err := request.Validate(); err != nil {
		return app.DistanceView{}, err
	}
	var facts app.DistanceFacts
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		// The recorded-track grant is proved first, under the GPS-012 lock
		// order; only its absence withholds the GPS source, every other
		// refusal (proof, membership, unknown trip) ends the read.
		switch err := s.Authority.CheckTrackTx(ctx, tx, scope, actor, request.MembershipID, request.TripID); {
		case err == nil:
		case errors.Is(err, permissions.ErrPermissionDenied):
			facts.Withheld = true
		default:
			return err
		}
		record, err := s.Trips.ReadTripTx(ctx, syncapp.Borrow(tx), scope, request.TripID)
		if err != nil {
			return err
		}
		facts.Trip = record
		if facts.Withheld {
			return nil
		}
		return latestSummary(ctx, tx, scope.OrgID().String(), request.TripID, &facts)
	})
	if err != nil {
		return app.DistanceView{}, distanceError(err)
	}
	if facts.Trip.ID != request.TripID {
		return app.DistanceView{}, app.ErrUnavailable
	}
	odometer, err := app.ReadOdometer(ctx, scope, facts.Trip, s.Meters)
	if err != nil {
		return app.DistanceView{}, err
	}
	view, err := app.ComposeDistances(facts, odometer, domain.DefaultThresholds())
	if err != nil {
		return app.DistanceView{}, err
	}
	if facts.Withheld {
		// Nothing location-derived is served, so nothing is recorded.
		return view, nil
	}
	// B-BR-EMP-494 (EDR-282): the issuance and the served access are recorded
	// before the answer leaves; any failure serves nothing.
	issued, err := s.NewID()
	if err != nil {
		return app.DistanceView{}, app.ErrAccessUnrecorded
	}
	served, err := s.NewID()
	if err != nil {
		return app.DistanceView{}, app.ErrAccessUnrecorded
	}
	record := audit.SensitiveAccess{Category: audit.LocationTrack, Stage: audit.AccessIssued, AccountID: actor.AccountID, MembershipID: request.MembershipID, TargetID: request.TripID,
		AccessID: issued, RequestID: issued, Binding: distanceBinding(scope, view)}
	if err := s.Access.RecordAccess(ctx, scope, record); err != nil {
		return app.DistanceView{}, app.ErrAccessUnrecorded
	}
	record.Stage, record.RequestID = audit.AccessServed, served
	if err := s.Access.RecordAccess(ctx, scope, record); err != nil {
		return app.DistanceView{}, app.ErrAccessUnrecorded
	}
	return view, nil
}
