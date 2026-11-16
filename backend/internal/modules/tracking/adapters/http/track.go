// Package http exposes the EMP-GPS-012 recorded-track read through the
// guarded company pipeline (EDR-281).
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// TrackReadPath is the recorded-track read of one trip.
const TrackReadPath = "/v1/orgs/{org_id}/trips/{trip_id}/track/read"

// TrackRecords accepts only the recorded-track policy.
type TrackRecords struct{}

func (TrackRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 || policy != app.TrackPolicy() || !facts.RecordedTrack {
		return access.ErrUnavailable
	}
	return nil
}

// TripAuthors reads the stored trip's author membership.
type TripAuthors interface {
	TripAuthor(context.Context, kernel.OrgScope, string) (string, error)
}

// trackResource resolves the path trip with its server facts: the author is
// the own record and the trip's driver, and the read is a recorded track
// (BR-AUTH-005, OWN_TRIPS_ONLY, RECORDED_TRACKS_ONLY). Structure comes from
// the TripStructure source, never from the request.
type trackResource struct{ authors TripAuthors }

func (r trackResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil || missing(r.authors) {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, err := tripPathID(ctx)
	if err != nil {
		return permissions.ResourceFacts{}, err
	}
	author, err := r.authors.TripAuthor(ctx, scope, id)
	if errors.Is(err, app.ErrNotFound) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	if err != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id, AuthoredBy: author, TripDriver: author, RecordedTrack: true}, nil
}

func tripPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "trip_id")
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

type trackRequest struct {
	orgapi.Page
}

func (trackRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v *trackRequest) UnmarshalJSON(raw []byte) error {
	type plain trackRequest
	var in plain
	if httpapi.DecodeJSON(raw, &in) != nil {
		return orgapi.Malformed
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&fields) != nil || fields == nil {
		return orgapi.Malformed
	}
	*v = trackRequest(in)
	return nil
}

func trackMapped(err error) error {
	var denied *permissions.PermissionDenial
	if errors.As(err, &denied) {
		return orgapi.MissingPermission(string(denied.Permission()))
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrMalformed, orgapi.Malformed}, {app.ErrTrackDisabled, orgapi.NotFound}, {app.ErrNotFound, orgapi.NotFound}, {app.ErrTrackUnauthorized, orgapi.Unauthenticated},
		{permissions.ErrResourceNotFound, orgapi.NotFound}, {permissions.ErrPermissionDenied, orgapi.Forbidden},
		{access.ErrNoMembership, orgapi.NotFound}, {access.ErrNotFound, orgapi.NotFound}, {access.ErrUnauthenticated, orgapi.Unauthenticated},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired}, {access.ErrMembershipSuspended, orgapi.MembershipSuspended}, {access.ErrOrgSuspended, orgapi.OrgSuspended},
		{access.ErrOrgArchived, orgapi.OrgArchived}, {access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete}, {access.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

func optionalInt32(v *int32) any {
	if v == nil {
		return nil
	}
	return *v
}

func optionalInt16(v *int16) any {
	if v == nil {
		return nil
	}
	return *v
}

func project(v app.TrackView, next string) map[string]any {
	var session any
	if s := v.Session; s != nil {
		transitions := make([]map[string]any, 0, len(s.Transitions))
		for _, t := range s.Transitions {
			transitions = append(transitions, map[string]any{"sequence": t.Sequence, "state": t.State, "cause": nullable(t.Cause), "actor": t.Actor, "permission": nullable(t.Permission), "occurred_at": orgtime.FormatInstant(t.At)})
		}
		session = map[string]any{"session_id": s.ID, "state": s.State, "policy": s.Policy, "opened_at": orgtime.FormatInstant(s.OpenedAt), "transitions": transitions}
	}
	segments := make([]map[string]any, 0, len(v.Segments))
	for _, g := range v.Segments {
		fixes := make([]map[string]any, 0, len(g.Fixes))
		for _, f := range g.Fixes {
			fixes = append(fixes, map[string]any{"collected_at": orgtime.FormatInstant(f.At), "lat_e7": f.Lat, "lon_e7": f.Lon, "accuracy_m": f.Accuracy, "speed_cm_s": optionalInt32(f.Speed), "bearing_deg": optionalInt16(f.Bearing), "coarse": f.Coarse})
		}
		segments = append(segments, map[string]any{"sequence": g.Sequence, "source": g.Source, "received_at": orgtime.FormatInstant(g.ReceivedAt), "received_count": g.Received, "stored_count": g.Stored,
			"duplicate_count": g.Duplicates, "outside_session_count": g.Outside, "mock_count": g.Mock, "fixes": fixes})
	}
	return map[string]any{"trip_id": v.TripID, "session": session, "segments": segments, "next_cursor": next, "data_mode": "SYNTHETIC"}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// RegisterTrack mounts the recorded-track read (EMP-GPS-012).
func RegisterTrack(router chi.Router, service app.TrackService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, authors TripAuthors) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || missing(authors) || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	policy := app.TrackPolicy()
	rule := orgapi.Rule{Permission: app.TrackPermission, Write: false, Fresh: policy.Fresh, CompanyOnly: true, Success: 200}
	return guarded.Register(owned, TrackReadPath, gateway, rule, authorization, policy, trackResource{authors: authors}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input trackRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		trip, err := tripPathID(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil || page.Limit > app.MaxTrackPage {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("tracking.track.v1:%s:%d", trip, page.Limit)
		after, err := gateway.OpenCursor(p, TrackReadPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		request := app.TrackRequest{TripID: trip, Limit: page.Limit}
		if after != "" {
			if request.AfterSequence, err = strconv.ParseInt(after, 10, 64); err != nil || request.AfterSequence < 1 {
				return nil, orgapi.Malformed
			}
		}
		view, err := service.Read(ctx, p.Scope, app.TrackActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, decision, request)
		if err != nil {
			return nil, trackMapped(err)
		}
		next := ""
		if view.NextSequence > 0 {
			if next, err = gateway.SealCursor(p, TrackReadPath, filters, strconv.FormatInt(view.NextSequence, 10)); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return project(view, next), nil
	})
}
