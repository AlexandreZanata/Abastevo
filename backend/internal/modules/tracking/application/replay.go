package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
)

// Trip track replay with logbook correlation and retention/trimming enforcement
// (EMP-GPS-009, EMP-GPS-011, BR-GPS-023..024, BR-GPS-033, BR-GPS-035..036).
type ReplayRequest struct {
	TripID string
}

func (r ReplayRequest) Validate() error {
	if !canonicalID(r.TripID) {
		return ErrMalformed
	}
	return nil
}

type ReplayPoint struct {
	Sequence   int64
	At         time.Time
	ReceivedAt time.Time
	Lat        int32
	Lon        int32
	Accuracy   int32
	Speed      *int32
	Bearing    *int16
	Coarse     bool
}

type ReplayGap struct {
	From       time.Time
	To         time.Time
	Reason     string
	DetectedBy string
}

type ReplayLogbookEvent struct {
	Kind    string
	At      time.Time
	Summary string
}

type ReplaySummary struct {
	Revision         int
	Quality          string
	CoveragePermille *int
	DistanceMetres   *int64
	ExpectedSeconds  int64
	GapSeconds       int64
	Coarse           bool
	ProcessedAt      time.Time
}

type ReplayView struct {
	TripID          string
	TripState       string
	RawExpired      bool
	TrimmedCount    int
	Session         *TrackSession
	Summary         *ReplaySummary
	Points          []ReplayPoint
	SimplifiedLines [][]domain.Point
	Gaps            []ReplayGap
	Logbook         []ReplayLogbookEvent
}

type ReplayRepository interface {
	ReadReplay(ctx context.Context, scope kernel.OrgScope, actor TrackActor, decision access.Decision, request ReplayRequest) (ReplayView, error)
}

type ReplayService struct {
	Enabled    bool
	Repository ReplayRepository
}

func (s ReplayService) ReadReplay(ctx context.Context, scope kernel.OrgScope, actor TrackActor, decision access.Decision, request ReplayRequest) (ReplayView, error) {
	if !s.Enabled {
		return ReplayView{}, ErrTrackDisabled
	}
	if scope.Validate() != nil || request.Validate() != nil {
		return ReplayView{}, ErrMalformed
	}
	if actor.Validate() != nil {
		return ReplayView{}, ErrTrackUnauthorized
	}
	if absent(s.Repository) {
		return ReplayView{}, ErrUnavailable
	}
	return s.Repository.ReadReplay(ctx, scope, actor, decision, request)
}
