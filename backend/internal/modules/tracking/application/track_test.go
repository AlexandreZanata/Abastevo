package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
)

type recordingRepository struct{ calls int }

func (r *recordingRepository) ReadTrack(context.Context, kernel.OrgScope, app.TrackActor, access.Decision, app.TrackRequest) (app.TrackView, error) {
	r.calls++
	return app.TrackView{}, nil
}

// B-BR-EMP-435: the read policy is gps.track.read only, a read, never
// trip.trip.read; requests are bounded and a service without a completed
// decision reaches no repository.
func TestTrackReadPolicyRequestAndDecisionGate(t *testing.T) {
	policy := app.TrackPolicy()
	if policy.Permission != "gps.track.read" || policy.Effect != access.Read || policy.Fresh || policy.Capability != "organizations" || policy.Validate() != nil {
		t.Fatal("track policy", policy.Permission)
	}
	valid := app.TrackRequest{TripID: id(21), Limit: 20}
	if valid.Validate() != nil {
		t.Fatal("a valid page request is refused")
	}
	for name, r := range map[string]app.TrackRequest{
		"no trip": {Limit: 1}, "limit zero": {TripID: id(21)}, "limit 21": {TripID: id(21), Limit: app.MaxTrackPage + 1},
		"negative cursor": {TripID: id(21), Limit: 1, AfterSequence: -1}, "upper trip": {TripID: strings.ToUpper(id(21)), Limit: 1},
	} {
		if !errors.Is(r.Validate(), app.ErrMalformed) {
			t.Fatal("an invalid page request was admitted", name)
		}
	}
	if !errors.Is((app.TrackActor{AccountID: id(1)}).Validate(), app.ErrTrackUnauthorized) {
		t.Fatal("an actor without a session was admitted")
	}
	repository := &recordingRepository{}
	actor := app.TrackActor{AccountID: id(1), SessionID: id(2)}
	if _, err := (app.TrackService{Repository: repository}).Read(t.Context(), scope(t), actor, access.Decision{}, valid); !errors.Is(err, app.ErrTrackDisabled) {
		t.Fatal("a disabled service read", err)
	}
	if _, err := (app.TrackService{Enabled: true, Repository: repository}).Read(t.Context(), scope(t), actor, access.Decision{}, valid); !errors.Is(err, app.ErrUnavailable) || repository.calls != 0 {
		t.Fatal("a read without a completed decision reached the repository", err)
	}
	if _, err := (app.TrackService{Enabled: true, Repository: repository}).Read(t.Context(), scope(t), actor, access.Decision{}, app.TrackRequest{TripID: id(21)}); !errors.Is(err, app.ErrMalformed) {
		t.Fatal("an unbounded page was read", err)
	}
}

// B-BR-EMP-437: served values print redacted.
func TestTrackViewsAreRedacted(t *testing.T) {
	speed := int32(1389)
	values := []any{
		app.TrackView{TripID: id(21), Segments: []app.TrackSegment{{Fixes: []app.TrackFix{{Lat: -100000000, Lon: -500000000, Speed: &speed}}}}},
		app.TrackFix{Lat: -100000000, Lon: -500000000}, app.TrackSegment{Sequence: 7}, app.TrackSession{ID: id(11)}, app.TrackTransition{State: "RECORDING"},
		app.TrackRequest{TripID: id(21)}, app.TrackActor{AccountID: id(1)},
	}
	for _, v := range values {
		raw, _ := json.Marshal(v)
		for _, s := range []string{fmt.Sprintf("%v %+v %#v %s", v, v, v, v), string(raw), loggedValue(v)} {
			for _, secret := range []string{"100000000", "500000000", id(1), id(11), id(21), "1389"} {
				if strings.Contains(s, secret) {
					t.Fatalf("%T leaked %q", v, secret)
				}
			}
		}
	}
}
