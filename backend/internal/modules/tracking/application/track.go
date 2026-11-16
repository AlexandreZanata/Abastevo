package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

// Recorded-track reads (EMP-GPS-012, B-BR-EMP-435..437, EDR-281/282).
const (
	// TrackPermission is the only permission of a recorded-track read; it is
	// never derived from trip.trip.read (BR-GPS-033, BR-AUTH-011).
	TrackPermission = "gps.track.read"
	// MaxTrackPage bounds the segments of one read page.
	MaxTrackPage = 20
)

var (
	ErrTrackDisabled     = errors.New("tracking: track read disabled")
	ErrTrackUnauthorized = errors.New("tracking: company proof required")
	// ErrAccessUnrecorded means the access log refused or failed: nothing is
	// served (B-BR-EMP-436).
	ErrAccessUnrecorded = errors.New("tracking: access not recorded")
)

// TrackPolicy is the source-owned read policy of a recorded track.
func TrackPolicy() access.Policy {
	return access.Policy{Permission: TrackPermission, Capability: "organizations", Effect: access.Read}
}

// TrackActor is the verified company proof of the reader.
type TrackActor struct {
	private
	AccountID, SessionID string
}

func (a TrackActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrTrackUnauthorized
	}
	return nil
}

// TrackRequest names the trip and the segment page.
type TrackRequest struct {
	private
	TripID        string
	AfterSequence int64
	Limit         int
}

func (r TrackRequest) Validate() error {
	if !canonicalID(r.TripID) || r.AfterSequence < 0 || r.AfterSequence > MaxSequence || r.Limit < 1 || r.Limit > MaxTrackPage {
		return ErrMalformed
	}
	return nil
}

// TrackTransition is one stored session row as served.
type TrackTransition struct {
	private
	Sequence                        int
	State, Cause, Actor, Permission string
	At                              time.Time
}

// TrackSession is the trip's session as served.
type TrackSession struct {
	private
	ID, State, Policy string
	OpenedAt          time.Time
	Transitions       []TrackTransition
}

// TrackFix is one stored fix as served.
type TrackFix struct {
	private
	At                 time.Time
	Lat, Lon, Accuracy int32
	Speed              *int32
	Bearing            *int16
	Coarse             bool
}

// TrackSegment is one stored segment as served.
type TrackSegment struct {
	private
	Sequence                                    int64
	Source                                      string
	ReceivedAt                                  time.Time
	Received, Stored, Duplicates, Outside, Mock int
	Fixes                                       []TrackFix
}

// TrackView is one page of a trip's recorded track.
type TrackView struct {
	private
	TripID       string
	Session      *TrackSession
	Segments     []TrackSegment
	NextSequence int64
}

// TrackRepository reads a recorded track under a completed decision,
// re-proves authority in its transaction and records the access before
// returning anything.
type TrackRepository interface {
	ReadTrack(context.Context, kernel.OrgScope, TrackActor, access.Decision, TrackRequest) (TrackView, error)
}

// TrackService exposes the read when enabled.
type TrackService struct {
	Enabled    bool
	Repository TrackRepository
}

func (s TrackService) Read(ctx context.Context, scope kernel.OrgScope, actor TrackActor, decision access.Decision, request TrackRequest) (TrackView, error) {
	if !s.Enabled || s.Repository == nil {
		return TrackView{}, ErrTrackDisabled
	}
	if err := actor.Validate(); err != nil {
		return TrackView{}, err
	}
	if err := request.Validate(); err != nil {
		return TrackView{}, err
	}
	if err := decision.Check(ctx, scope, actor.AccountID, TrackPermission, request.TripID); err != nil {
		return TrackView{}, ErrUnavailable
	}
	return s.Repository.ReadTrack(ctx, scope, actor, decision, request)
}
