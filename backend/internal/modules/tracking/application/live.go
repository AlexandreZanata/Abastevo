package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
)

// Live operations location reads (EMP-GPS-007, BR-GPS-030..033, VAL-16).
const (
	LivePermission = "gps.location.read_live"
	MaxLivePage    = 100
)

func LivePolicy() access.Policy {
	return access.Policy{Permission: LivePermission, Capability: "organizations", Effect: access.Read}
}

type LiveActor struct {
	private
	AccountID, SessionID string
}

func NewLiveActor(accountID, sessionID string) (LiveActor, error) {
	a := LiveActor{AccountID: accountID, SessionID: sessionID}
	if a.Validate() != nil {
		return LiveActor{}, ErrTrackUnauthorized
	}
	return a, nil
}

func (a LiveActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrTrackUnauthorized
	}
	return nil
}

type LiveReadRequest struct {
	TripID string
}

func (r LiveReadRequest) Validate() error {
	if !canonicalID(r.TripID) {
		return ErrMalformed
	}
	return nil
}

type LiveListRequest struct {
	UnitID        string
	SinceRevision int64
	Limit         int
}

func (r LiveListRequest) Validate() error {
	if (r.UnitID != "" && !canonicalID(r.UnitID)) || r.SinceRevision < 0 || r.Limit < 1 || r.Limit > MaxLivePage {
		return ErrMalformed
	}
	return nil
}

// LiveRecord holds raw DB state prior to read-time freshness derivation.
type LiveRecord struct {
	TripID              string
	SessionID           string
	SessionState        string
	TripState           string
	UnitID              string
	UnitName            string
	AssetID             string
	AssetLabel          string
	DriverMembershipID  string
	DriverName          string
	Revision            int64
	Fix                 *domain.LiveFix
	LastCommunicationAt *time.Time
	ExpiresAt           *time.Time
}

type LiveTripView struct {
	TripID              string
	SessionID           string
	SessionState        string
	TripState           string
	StatusNote          string
	UnitID              string
	UnitName            string
	AssetID             string
	AssetLabel          string
	DriverName          string
	Freshness           domain.Freshness
	Lat                 *int32
	Lon                 *int32
	Accuracy            *int32
	Speed               *int32
	Bearing             *int16
	Coarse              *bool
	CollectedAt         *time.Time
	ReceivedAt          *time.Time
	LastCommunicationAt *time.Time
	Revision            int64
}

type LiveListView struct {
	Items          []LiveTripView
	Alerts         []AlertView
	MaxRevision    int64
	CurrentSeconds int64
	StaleSeconds   int64
}

// ProjectLiveView derives read-time freshness and hides rows past their 24h
// post-trip expiration (BR-GPS-030..032).
func ProjectLiveView(rec LiveRecord, limits domain.FreshnessLimits, now time.Time) (LiveTripView, bool) {
	pos := domain.LivePosition{
		TripID:              rec.TripID,
		SessionID:           rec.SessionID,
		Fix:                 rec.Fix,
		LastCommunicationAt: rec.LastCommunicationAt,
		Revision:            rec.Revision,
	}
	if domain.LivePositionExpiryReached(rec.ExpiresAt, now) {
		return LiveTripView{}, false
	}
	fresh, _, err := domain.EvaluateFreshness(pos, now, limits)
	if err != nil {
		fresh = domain.FreshnessNoSignal
	}
	view := LiveTripView{
		TripID:              rec.TripID,
		SessionID:           rec.SessionID,
		SessionState:        rec.SessionState,
		TripState:           rec.TripState,
		UnitID:              rec.UnitID,
		UnitName:            rec.UnitName,
		AssetID:             rec.AssetID,
		AssetLabel:          rec.AssetLabel,
		DriverName:          rec.DriverName,
		Freshness:           fresh,
		LastCommunicationAt: rec.LastCommunicationAt,
		Revision:            rec.Revision,
	}
	if rec.SessionState == "SUSPENDED" || rec.TripState == "PAUSED" {
		view.StatusNote = "viagem pausada"
	}
	if rec.Fix != nil {
		lat, lon, acc, coarse := rec.Fix.Lat, rec.Fix.Lon, rec.Fix.Accuracy, rec.Fix.Coarse
		col, rcv := rec.Fix.CollectedAt.UTC(), rec.Fix.ReceivedAt.UTC()
		view.Lat, view.Lon, view.Accuracy, view.Coarse = &lat, &lon, &acc, &coarse
		view.CollectedAt, view.ReceivedAt = &col, &rcv
		view.Speed, view.Bearing = copyPointer(rec.Fix.Speed), copyPointer(rec.Fix.Bearing)
	}
	return view, true
}

type LiveRepository interface {
	ReadLive(ctx context.Context, scope kernel.OrgScope, actor LiveActor, decision access.Decision, request LiveReadRequest) (LiveTripView, error)
	ListLive(ctx context.Context, scope kernel.OrgScope, actor LiveActor, decision access.Decision, request LiveListRequest) (LiveListView, error)
}

type LiveService struct {
	Enabled    bool
	Repository LiveRepository
}

func (s LiveService) ReadLive(ctx context.Context, scope kernel.OrgScope, actor LiveActor, decision access.Decision, request LiveReadRequest) (LiveTripView, error) {
	if !s.Enabled {
		return LiveTripView{}, ErrTrackDisabled
	}
	if scope.Validate() != nil || request.Validate() != nil {
		return LiveTripView{}, ErrMalformed
	}
	if actor.Validate() != nil {
		return LiveTripView{}, ErrTrackUnauthorized
	}
	if absent(s.Repository) {
		return LiveTripView{}, ErrUnavailable
	}
	return s.Repository.ReadLive(ctx, scope, actor, decision, request)
}

func (s LiveService) ListLive(ctx context.Context, scope kernel.OrgScope, actor LiveActor, decision access.Decision, request LiveListRequest) (LiveListView, error) {
	if !s.Enabled {
		return LiveListView{}, ErrTrackDisabled
	}
	if scope.Validate() != nil || request.Validate() != nil {
		return LiveListView{}, ErrMalformed
	}
	if actor.Validate() != nil {
		return LiveListView{}, ErrTrackUnauthorized
	}
	if absent(s.Repository) {
		return LiveListView{}, ErrUnavailable
	}
	return s.Repository.ListLive(ctx, scope, actor, decision, request)
}
